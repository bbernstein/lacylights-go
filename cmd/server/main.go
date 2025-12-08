// Package main is the entry point for the LacyLights Go server.
package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/99designs/gqlgen/graphql/handler"
	"github.com/99designs/gqlgen/graphql/handler/extension"
	"github.com/99designs/gqlgen/graphql/handler/lru"
	"github.com/99designs/gqlgen/graphql/handler/transport"
	"github.com/99designs/gqlgen/graphql/playground"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gorilla/websocket"
	"github.com/joho/godotenv"
	"github.com/rs/cors"
	"github.com/vektah/gqlparser/v2/ast"

	"github.com/bbernstein/lacylights-go/internal/config"
	"github.com/bbernstein/lacylights-go/internal/database"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/graphql/resolvers"
	"github.com/bbernstein/lacylights-go/internal/services/dmx"
	"github.com/bbernstein/lacylights-go/internal/services/fade"
	"github.com/bbernstein/lacylights-go/internal/services/playback"
)

// Version information (set at build time)
var (
	Version   = "0.1.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// Load .env file if present
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Load configuration
	cfg := config.Load()

	// Print startup banner
	printBanner(cfg)

	// Connect to database
	db, err := database.Connect(database.Config{
		URL:         cfg.DatabaseURL,
		MaxIdleConn: 5,
		MaxOpenConn: 10,
		Debug:       cfg.IsDevelopment(),
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Auto-migrate database schema
	log.Println("Running database migrations...")
	if err := db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.ProjectUser{},
		&models.FixtureDefinition{},
		&models.ChannelDefinition{},
		&models.FixtureMode{},
		&models.ModeChannel{},
		&models.FixtureInstance{},
		&models.InstanceChannel{},
		&models.Scene{},
		&models.FixtureValue{},
		&models.CueList{},
		&models.Cue{},
		&models.PreviewSession{},
		&models.Setting{},
		&models.SceneBoard{},
		&models.SceneBoardButton{},
	); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
	log.Println("Database migrations complete")

	// Create and initialize DMX service
	dmxService := dmx.NewService(dmx.Config{
		Enabled:          cfg.ArtNetEnabled,
		BroadcastAddr:    cfg.ArtNetBroadcast,
		Port:             cfg.ArtNetPort,
		RefreshRateHz:    cfg.DMXRefreshRate,
		IdleRateHz:       cfg.DMXIdleRate,
		HighRateDuration: cfg.DMXHighRateDuration,
	})
	if err := dmxService.Initialize(); err != nil {
		log.Printf("Warning: DMX service initialization failed: %v", err)
		// Continue anyway - DMX may be disabled or broadcast address unavailable
	}

	// Load saved broadcast address from database
	settingRepo := repositories.NewSettingRepository(db)
	if savedAddr, err := settingRepo.FindByKey(context.Background(), "artnet_broadcast_address"); err == nil && savedAddr != nil && savedAddr.Value != "" {
		log.Printf("📡 Loading saved Art-Net broadcast address: %s", savedAddr.Value)
		if err := dmxService.ReloadBroadcastAddress(savedAddr.Value); err != nil {
			log.Printf("Warning: failed to load saved broadcast address: %v", err)
		}
	}

	// Create and start fade engine
	fadeEngine := fade.NewEngine(dmxService)
	fadeEngine.Start()

	// Create playback service
	playbackService := playback.NewService(db, dmxService, fadeEngine)

	// Create router
	router := chi.NewRouter()

	// Middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Timeout(60 * time.Second))

	// CORS
	corsMiddleware := cors.New(cors.Options{
		AllowedOrigins:   []string{cfg.CORSOrigin, "http://localhost:3000", "http://localhost:4000"},
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token"},
		AllowCredentials: true,
		Debug:            cfg.IsDevelopment(),
	})
	router.Use(corsMiddleware.Handler)

	// Create resolver with dependencies
	resolver := resolvers.NewResolver(db, dmxService, fadeEngine, playbackService)

	// Create GraphQL server
	srv := handler.New(generated.NewExecutableSchema(generated.Config{
		Resolvers: resolver,
	}))

	// Configure transport handlers
	srv.AddTransport(transport.Websocket{
		Upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for WebSocket
			},
			ReadBufferSize:  1024,
			WriteBufferSize: 1024,
		},
		KeepAlivePingInterval: 10 * time.Second,
	})
	srv.AddTransport(transport.Options{})
	srv.AddTransport(transport.GET{})
	srv.AddTransport(transport.POST{})
	srv.AddTransport(transport.MultipartForm{})

	// Configure extensions
	srv.SetQueryCache(lru.New[*ast.QueryDocument](1000))
	srv.Use(extension.Introspection{})
	srv.Use(extension.AutomaticPersistedQuery{
		Cache: lru.New[string](100),
	})

	// Routes
	router.Get("/health", healthCheckHandler)
	router.Handle("/graphql", srv)

	// GraphQL Playground (only in development)
	if cfg.IsDevelopment() {
		router.Handle("/", playground.Handler("LacyLights GraphQL Playground", "/graphql"))
		log.Printf("GraphQL Playground available at http://localhost:%s/\n", cfg.Port)
	}

	// Create HTTP server
	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Server listening on http://localhost:%s\n", cfg.Port)
		log.Printf("GraphQL endpoint: http://localhost:%s/graphql\n", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	log.Println("Shutting down server...")

	// Cleanup services in reverse order
	playbackService.Cleanup()
	fadeEngine.Stop()
	dmxService.Stop()

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := httpServer.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

// healthCheckHandler returns the server health status.
func healthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	response := fmt.Sprintf(`{
  "status": "ok",
  "timestamp": "%s",
  "version": "%s",
  "uptime": "N/A"
}`, time.Now().UTC().Format(time.RFC3339), Version)

	_, _ = w.Write([]byte(response))
}

// printBanner prints the startup banner.
func printBanner(cfg *config.Config) {
	fmt.Println("============================================")
	fmt.Println("  LacyLights Go Server")
	fmt.Printf("  Version: %s\n", Version)
	fmt.Printf("  Build:   %s\n", BuildTime)
	fmt.Printf("  Commit:  %s\n", GitCommit)
	fmt.Println("============================================")
	fmt.Printf("  Environment: %s\n", cfg.Env)
	fmt.Printf("  Port:        %s\n", cfg.Port)
	fmt.Printf("  Database:    %s\n", cfg.DatabaseURL)
	fmt.Printf("  Art-Net:     %v\n", cfg.ArtNetEnabled)
	fmt.Println("============================================")
}
