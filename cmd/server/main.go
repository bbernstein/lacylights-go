// Package main is the entry point for the LacyLights Go server.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
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

	"github.com/lucsky/cuid"

	"github.com/bbernstein/lacylights-go/internal/auth"
	"github.com/bbernstein/lacylights-go/internal/config"
	"github.com/bbernstein/lacylights-go/internal/database"
	"github.com/bbernstein/lacylights-go/internal/database/migrations"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/bbernstein/lacylights-go/internal/database/repositories"
	"github.com/bbernstein/lacylights-go/internal/graphql/generated"
	"github.com/bbernstein/lacylights-go/internal/graphql/resolvers"
	authmiddleware "github.com/bbernstein/lacylights-go/internal/middleware"
	"github.com/bbernstein/lacylights-go/internal/services/dmx"
	"github.com/bbernstein/lacylights-go/internal/services/modulator"
	"github.com/bbernstein/lacylights-go/internal/services/ofl"
	"github.com/bbernstein/lacylights-go/internal/services/playback"
	"github.com/bbernstein/lacylights-go/internal/services/version"
	"gorm.io/gorm"
)

// Version information (set at build time)
var (
	Version   = "0.1.0"
	BuildTime = "unknown"
	GitCommit = "unknown"
)

func main() {
	// Set build info for the version package (used by GraphQL resolver)
	version.SetBuildInfo(Version, GitCommit, BuildTime)

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

	// Tables without FOREIGN KEY constraints in the database can be migrated normally
	if err := db.AutoMigrate(
		&models.User{},
		&models.Project{},
		&models.ProjectUser{},
		&models.PreviewSession{},
		&models.Setting{},
		&models.OFLImportMeta{},
		&models.Operation{},
		&models.OperationPointer{},
		// Auth tables without FK constraints
		&models.UserGroup{},
		&models.AuthSetting{},
	); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}

	// GORM SQLite migrator bug workaround:
	// When a table has FOREIGN KEY constraints in its schema, the migrator incorrectly
	// parses "FOREIGN" as a column name, causing migration failures.
	// For tables with FK constraints, we skip AutoMigrate if the table already exists
	// with the required columns.
	tablesWithFK := []struct {
		table      string
		model      interface{}
		keyColumns []string // columns that must exist for schema to be considered complete
	}{
		{"fixture_definitions", &models.FixtureDefinition{}, []string{"id"}},
		{"channel_definitions", &models.ChannelDefinition{}, []string{"id", "definition_id"}},
		{"fixture_modes", &models.FixtureMode{}, []string{"id", "definition_id"}},
		{"mode_channels", &models.ModeChannel{}, []string{"id", "mode_id", "channel_id"}},
		{"fixture_instances", &models.FixtureInstance{}, []string{"id", "project_id"}},
		{"instance_channels", &models.InstanceChannel{}, []string{"id", "fixture_id"}},
		{"fixture_groups", &models.FixtureGroup{}, []string{"id", "project_id"}},
		{"fixture_group_members", &models.FixtureGroupMember{}, []string{"group_id", "fixture_id"}},
		{"looks", &models.Look{}, []string{"id", "project_id"}},
		{"cue_lists", &models.CueList{}, []string{"id", "project_id"}},
		{"cues", &models.Cue{}, []string{"id", "look_id", "cue_list_id"}},
		{"look_boards", &models.LookBoard{}, []string{"id", "project_id"}},
		{"look_board_buttons", &models.LookBoardButton{}, []string{"id", "look_board_id", "look_id"}},
		{"fixture_values", &models.FixtureValue{}, []string{"id", "look_id", "fixture_id"}},
		{"effects", &models.Effect{}, []string{"id", "project_id"}},
		{"effect_fixtures", &models.EffectFixture{}, []string{"id", "effect_id", "fixture_id"}},
		{"effect_channels", &models.EffectChannel{}, []string{"id", "effect_fixture_id"}},
		{"cue_effects", &models.CueEffect{}, []string{"id", "cue_id", "effect_id"}},
		// Auth tables with FK constraints
		{"user_credentials", &models.UserCredential{}, []string{"id", "user_id"}},
		{"user_oauth", &models.UserOAuth{}, []string{"id", "user_id", "provider"}},
		{"devices", &models.Device{}, []string{"id", "fingerprint"}},
		{"sessions", &models.Session{}, []string{"id", "user_id", "token_hash"}},
		{"verification_tokens", &models.VerificationToken{}, []string{"id", "token_hash"}},
		{"user_group_members", &models.UserGroupMember{}, []string{"id", "user_id", "group_id", "role"}},
		{"device_group_members", &models.DeviceGroupMember{}, []string{"id", "device_id", "group_id"}},
		{"group_invitations", &models.GroupInvitation{}, []string{"id", "group_id", "email", "role"}},
	}

	for _, t := range tablesWithFK {
		needsMigration := false
		if !db.Migrator().HasTable(t.table) {
			needsMigration = true
		} else {
			// Check if all key columns exist
			for _, col := range t.keyColumns {
				if !db.Migrator().HasColumn(t.table, col) {
					needsMigration = true
					break
				}
			}
		}

		if needsMigration {
			if err := db.AutoMigrate(t.model); err != nil {
				log.Fatalf("Failed to migrate %s: %v", t.table, err)
			}
		}
	}

	log.Println("Database migrations complete")

	// Migrate scene terminology to look terminology FIRST. This legacy
	// migration inserts rows into looks/look_boards without setting
	// ref_id (it relies on the column default '') — we must run it
	// before CreateRefIDIndexes so those rows exist before BackfillRefIDs
	// populates them, otherwise the unique index would reject them.
	if err := migrateSceneToLook(db); err != nil {
		log.Printf("Warning: scene to look migration failed: %v", err)
	}

	// Migrate old channelValues to sparse Channels format
	if err := migrateChannelValuesToSparse(db); err != nil {
		log.Printf("Warning: sparse channel migration failed: %v", err)
	}

	// Backfill ref_id columns then create unique indexes. Order matters:
	// backfill must populate empty ref_ids (including those just inserted
	// by migrateSceneToLook above) before the unique index goes up,
	// otherwise duplicate empty strings would violate the constraint.
	// Both functions are idempotent.
	if err := migrations.BackfillRefIDs(db); err != nil {
		log.Fatalf("Failed to backfill ref_ids: %v", err)
	}
	if err := migrations.CreateRefIDIndexes(db); err != nil {
		log.Fatalf("Failed to create ref_id indexes: %v", err)
	}

	// Backfill default layout canvas dimensions for existing projects
	if err := backfillLayoutCanvasDimensions(db); err != nil {
		log.Printf("Warning: layout canvas backfill failed: %v", err)
	}

	// Add min_value and max_value columns to effect_channels table
	if err := migrateEffectChannelMinMax(db); err != nil {
		log.Printf("Warning: effect channel min/max migration failed: %v", err)
	}

	// Note: group ownership migration runs after auth setup (EnsureDefaultAdmin)
	// so the admin user exists when assigning orphaned projects.

	// Load Open Fixture Library if enabled and database is empty
	if cfg.OFLImportEnabled {
		fixtureRepo := repositories.NewFixtureRepository(db)
		oflLoader := ofl.NewLoader(db, fixtureRepo, cfg.OFLCachePath)

		needsImport, err := oflLoader.NeedsImport(context.Background())
		if err != nil {
			log.Printf("Warning: failed to check if OFL import needed: %v", err)
		} else if needsImport {
			log.Println("📦 No fixture definitions found, importing Open Fixture Library...")
			status, err := oflLoader.LoadAll(context.Background())
			if err != nil {
				log.Printf("Warning: OFL import failed: %v", err)
				log.Println("You can still use the app but will need to import fixtures manually")
			} else {
				log.Printf("✅ OFL import complete: %d fixtures imported", status.SuccessfulImports)
			}
		} else {
			count, _ := fixtureRepo.CountDefinitions(context.Background())
			log.Printf("📋 Found %d fixture definitions in database", count)
		}
	}

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

	// Check ArtNet settings from database
	settingRepo := repositories.NewSettingRepository(db)

	// First check if ArtNet was disabled (blackout mode) before shutdown
	// We check this BEFORE loading broadcast address to avoid brief transmission on startup
	artnetDisabled := false
	if savedEnabled, err := settingRepo.FindByKey(context.Background(), "artnet_enabled"); err == nil && savedEnabled != nil && savedEnabled.Value == "false" {
		artnetDisabled = true
		log.Printf("🔌 Art-Net will start disabled based on saved setting (blackout mode)")
	}

	// Load saved broadcast address from database (only if ArtNet is not disabled)
	if !artnetDisabled {
		if savedAddr, err := settingRepo.FindByKey(context.Background(), "artnet_broadcast_address"); err == nil && savedAddr != nil && savedAddr.Value != "" {
			log.Printf("📡 Loading saved Art-Net broadcast address: %s", savedAddr.Value)
			if err := dmxService.ReloadBroadcastAddress(savedAddr.Value); err != nil {
				log.Printf("Warning: failed to load saved broadcast address: %v", err)
			}
		}
	} else {
		// Disable ArtNet now that DMX service is initialized
		dmxService.DisableArtNet()
	}

	// Create fade engine with configured update rate (or saved rate from database)
	fadeUpdateRate := cfg.FadeUpdateRateHz
	if savedRate, err := settingRepo.FindByKey(context.Background(), "fade_update_rate_hz"); err == nil && savedRate != nil && savedRate.Value != "" {
		if rateHz, err := strconv.Atoi(savedRate.Value); err == nil && rateHz > 0 {
			log.Printf("⚡ Loading saved fade update rate: %d Hz", rateHz)
			fadeUpdateRate = rateHz
		} else {
			log.Printf("Warning: invalid saved fade update rate: %s", savedRate.Value)
		}
	}
	fadeEngine := modulator.NewEngine(dmxService, fadeUpdateRate)
	_ = fadeEngine.Start()

	// Create playback service
	playbackService := playback.NewService(db, dmxService, fadeEngine)

	// Create router
	router := chi.NewRouter()

	// Middleware
	router.Use(middleware.RequestID)
	router.Use(middleware.RealIP)
	router.Use(middleware.Logger)
	router.Use(middleware.Recoverer)
	// Note: We intentionally do NOT use middleware.Timeout here because:
	// 1. WebSocket connections are long-lived and need to stay open indefinitely
	// 2. The timeout middleware cancels the request context, which kills subscriptions
	// 3. HTTP server timeouts (ReadTimeout, WriteTimeout, IdleTimeout) protect against slow clients
	// 4. WebSocket keepalive (10s ping interval) handles connection health

	// CORS - prevent CORS_ALLOW_ALL in production for security
	// Note: This check intentionally uses log.Fatal which calls os.Exit(1).
	// Testing this path requires subprocess testing which is complex for this simple safeguard.
	// The behavior is verified by: 1) code review, 2) CI would fail if accidentally deployed.
	if cfg.CORSAllowAll && cfg.IsProduction() {
		log.Fatal("CORS_ALLOW_ALL cannot be enabled in production - this is a security risk")
	}

	corsOptions := cors.Options{
		AllowedMethods:   []string{"GET", "POST", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type", "X-CSRF-Token", "X-Device-Fingerprint"},
		AllowCredentials: true,
		Debug:            cfg.IsDevelopment(),
	}
	if cfg.CORSAllowAll {
		// Allow all non-empty origins (for E2E testing only)
		log.Println("⚠️  CORS_ALLOW_ALL is enabled - allowing all origins (E2E testing mode)")
		corsOptions.AllowOriginFunc = func(origin string) bool { return origin != "" }
	} else {
		corsOptions.AllowedOrigins = []string{cfg.CORSOrigin, "http://localhost:3000", "http://localhost:4000"}
	}
	corsMiddleware := cors.New(corsOptions)
	router.Use(corsMiddleware.Handler)

	// Initialize auth service if enabled
	var authService *auth.Service
	if cfg.AuthEnabled {
		var err error
		authService, err = auth.NewService(auth.Config{
			DB:                   db,
			JWTSecret:            cfg.JWTSecret,
			JWTIssuer:            cfg.JWTIssuer,
			JWTAccessTokenTTL:    cfg.JWTAccessTokenTTL,
			JWTRefreshTokenTTL:   cfg.JWTRefreshTokenTTL,
			SessionDurationHours: cfg.SessionDurationHours,
			CacheMaxSize:         1000, // Default session cache size
			CacheTTL:             5 * time.Minute,
			PasswordMinLength:    cfg.PasswordMinLength,
			Enabled:              cfg.AuthEnabled,
			DeviceAuthEnabled:    cfg.DeviceAuthEnabled,
		})
		if err != nil {
			log.Fatalf("Failed to initialize auth service: %v", err)
		}
		log.Println("🔐 Auth service initialized")

		// Ensure default admin user exists (creates one if no users exist)
		if err := authService.EnsureDefaultAdmin(context.Background(), cfg.DefaultAdminEmail, cfg.DefaultAdminPassword); err != nil {
			log.Fatalf("Failed to ensure default admin: %v", err)
		}

		// Repair orphaned personal groups: if a personal group's owner has no membership, add them
		if err := repairOrphanedGroupMemberships(db); err != nil {
			log.Printf("Warning: failed to repair orphaned group memberships: %v", err)
		}

		// Add auth middleware with DB access for device authentication
		authMiddleware := authmiddleware.NewAuthMiddlewareWithDB(authService, db)
		router.Use(authMiddleware.Authenticate)
	} else {
		log.Println("Auth service disabled")
	}

	// Migrate group ownership fields after auth setup so admin user exists for orphaned project assignment
	if err := migrateGroupOwnership(db, cfg.AuthEnabled); err != nil {
		log.Printf("Warning: group ownership migration failed: %v", err)
	}

	// Create resolver with dependencies
	resolver := resolvers.NewResolver(db, dmxService, fadeEngine, playbackService, cfg.OFLCachePath)

	// Wire auth service to resolver if enabled
	if authService != nil {
		resolver.SetAuthService(authService)
	}

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
	// Timeouts increased to handle long-running mutations like repository updates
	httpServer := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      router,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 120 * time.Second, // Allows for long-running update operations (npm install ~25s)
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
	_ = fadeEngine.Stop()
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
  "gitCommit": "%s",
  "buildTime": "%s"
}`, time.Now().UTC().Format(time.RFC3339), Version, GitCommit, BuildTime)

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
	fmt.Printf("  OFL Import:  %v\n", cfg.OFLImportEnabled)
	fmt.Println("============================================")
}

// migrateChannelValuesToSparse migrates old channelValues arrays to the new sparse Channels format.
// This is a one-time migration that runs on startup to convert existing data.
// Uses GORM's migrator to check column existence (database-agnostic), then raw SQL for the
// actual migration since the ChannelValues field has been removed from the Go model.
func migrateChannelValuesToSparse(db *gorm.DB) error {
	// Check if the old channelValues column exists using GORM's migrator (database-agnostic)
	// We use the table name directly since the field was removed from the Go model
	if !db.Migrator().HasColumn("fixture_values", "channelValues") {
		return nil // Column doesn't exist, nothing to migrate
	}

	// Find all fixture_values where Channels is empty but channelValues is not (using raw SQL)
	type oldFixtureValue struct {
		ID            string
		ChannelValues string
	}
	var oldValues []oldFixtureValue
	result := db.Raw(`
		SELECT id, channelValues
		FROM fixture_values
		WHERE (channels IS NULL OR channels = '' OR channels = '[]')
		  AND channelValues IS NOT NULL
		  AND channelValues != ''
		  AND channelValues != '[]'
	`).Scan(&oldValues)
	if result.Error != nil {
		return fmt.Errorf("failed to query fixture values: %w", result.Error)
	}

	if len(oldValues) == 0 {
		return nil // Nothing to migrate
	}

	log.Printf("🔄 Migrating %d fixture values from channelValues to sparse Channels format...", len(oldValues))

	migratedCount := 0
	for _, fv := range oldValues {
		// Parse old channelValues array
		var values []int
		if err := json.Unmarshal([]byte(fv.ChannelValues), &values); err != nil {
			log.Printf("  Warning: failed to parse channelValues for fixture value %s: %v", fv.ID, err)
			continue
		}

		// Convert to sparse format
		// Note: Preserving all values including zeros for data fidelity during migration.
		// This ensures exact migration of existing data without loss.
		// New scenes created via API will only store explicitly set channels.
		// The sparse format allows storing any subset of channels, including zeros when intended.
		var channels []models.ChannelValue
		for i, v := range values {
			channels = append(channels, models.ChannelValue{
				Offset: i,
				Value:  v,
			})
		}

		// Serialize to JSON
		channelsJSON, err := json.Marshal(channels)
		if err != nil {
			log.Printf("  Warning: failed to serialize channels for fixture value %s: %v", fv.ID, err)
			continue
		}

		// Update the record using raw SQL
		if err := db.Exec("UPDATE fixture_values SET channels = ? WHERE id = ?", string(channelsJSON), fv.ID).Error; err != nil {
			log.Printf("  Warning: failed to update fixture value %s: %v", fv.ID, err)
			continue
		}
		migratedCount++
	}

	log.Printf("✅ Migrated %d fixture values to sparse Channels format", migratedCount)
	return nil
}

// backfillLayoutCanvasDimensions updates existing projects that have 0 layout canvas dimensions
// to use the default value of 2000. This is needed because SQLite/GORM doesn't apply default
// values to existing rows when new columns are added.
func backfillLayoutCanvasDimensions(db *gorm.DB) error {
	const defaultCanvasSize = 2000

	// Check if any projects need backfilling (0 or NULL values)
	var count int64
	if err := db.Model(&models.Project{}).
		Where("layout_canvas_width IS NULL OR layout_canvas_width = 0 OR layout_canvas_height IS NULL OR layout_canvas_height = 0").
		Count(&count).Error; err != nil {
		return fmt.Errorf("failed to count projects: %w", err)
	}

	if count == 0 {
		return nil // Nothing to backfill
	}

	log.Printf("🔄 Backfilling layout canvas dimensions for %d projects...", count)

	// Update projects with 0 or NULL width
	if err := db.Model(&models.Project{}).
		Where("layout_canvas_width IS NULL OR layout_canvas_width = 0").
		Update("layout_canvas_width", defaultCanvasSize).Error; err != nil {
		return fmt.Errorf("failed to backfill canvas width: %w", err)
	}

	// Update projects with 0 or NULL height
	if err := db.Model(&models.Project{}).
		Where("layout_canvas_height IS NULL OR layout_canvas_height = 0").
		Update("layout_canvas_height", defaultCanvasSize).Error; err != nil {
		return fmt.Errorf("failed to backfill canvas height: %w", err)
	}

	log.Printf("✅ Backfilled layout canvas dimensions for %d projects", count)
	return nil
}

// migrateEffectChannelMinMax adds min_value and max_value columns to the effect_channels table.
// These columns allow per-channel min/max range specification for waveform effects.
func migrateEffectChannelMinMax(db *gorm.DB) error {
	// Check if the min_value column already exists
	if db.Migrator().HasColumn("effect_channels", "min_value") {
		return nil // Columns already exist, nothing to migrate
	}

	log.Println("🔄 Adding min_value and max_value columns to effect_channels table...")

	// Add min_value column
	if err := db.Exec("ALTER TABLE effect_channels ADD COLUMN min_value REAL").Error; err != nil {
		return fmt.Errorf("failed to add min_value column: %w", err)
	}

	// Add max_value column
	if err := db.Exec("ALTER TABLE effect_channels ADD COLUMN max_value REAL").Error; err != nil {
		return fmt.Errorf("failed to add max_value column: %w", err)
	}

	log.Println("✅ Added min_value and max_value columns to effect_channels table")
	return nil
}

// migrateSceneToLook handles the schema migration from "scene" terminology to "look" terminology.
// This is a one-time migration that runs on startup to rename tables and columns.
// SQLite doesn't support renaming columns directly, so we use ALTER TABLE RENAME TO for tables
// and recreate tables with new column names where needed.
//
// Note: AutoMigrate runs BEFORE this function, so the new tables (looks, look_boards, etc.)
// may already exist. This function handles both cases:
// - If old tables exist and new tables don't: rename old to new
// - If both exist: copy data from old to new and drop old tables
func migrateSceneToLook(db *gorm.DB) error {
	// Check if migration is needed by looking for old table names
	if !db.Migrator().HasTable("scenes") {
		return nil // Already migrated or fresh database
	}

	log.Println("🔄 Migrating scene terminology to look terminology...")

	// Wrap entire migration in a transaction for atomicity
	return db.Transaction(func(tx *gorm.DB) error {
		return doMigrateSceneToLook(tx)
	})
}

// doMigrateSceneToLook performs the actual migration within a transaction.
// This is separated from migrateSceneToLook to allow proper transaction wrapping.
func doMigrateSceneToLook(db *gorm.DB) error {

	// SQLite-specific migration using raw SQL
	// We need to:
	// 1. Migrate scenes -> looks
	// 2. Migrate fixture_values columns (scene_id -> look_id, scene_order -> look_order)
	// 3. Migrate cues columns (scene_id -> look_id)
	// 4. Migrate scene_boards -> look_boards
	// 5. Migrate scene_board_buttons -> look_board_buttons

	// Step 1: Migrate scenes table to looks table
	// AutoMigrate may have created an empty "looks" table, so we need to handle both cases
	if db.Migrator().HasTable("looks") {
		// Both tables exist - copy data from scenes to looks, then drop scenes
		// First check if looks is empty and scenes has data
		var looksCount, scenesCount int64
		if err := db.Raw("SELECT COUNT(*) FROM looks").Scan(&looksCount).Error; err != nil {
			return fmt.Errorf("failed to count looks: %w", err)
		}
		if err := db.Raw("SELECT COUNT(*) FROM scenes").Scan(&scenesCount).Error; err != nil {
			return fmt.Errorf("failed to count scenes: %w", err)
		}

		if scenesCount > 0 && looksCount == 0 {
			// Copy data from scenes to looks
			if err := db.Exec(`
				INSERT INTO looks (id, name, description, project_id, created_at, updated_at)
				SELECT id, name, description, project_id, created_at, updated_at FROM scenes
			`).Error; err != nil {
				return fmt.Errorf("failed to copy scenes to looks: %w", err)
			}
			log.Printf("  Copied %d scenes to looks table", scenesCount)
		} else if scenesCount > 0 && looksCount > 0 {
			// CRITICAL: Both tables have data - this is an inconsistent state that requires manual intervention
			return fmt.Errorf("migration conflict: both scenes (%d records) and looks (%d records) tables contain data; manual data reconciliation required before migration can proceed", scenesCount, looksCount)
		}

		// Only drop the old scenes table if we successfully copied or it was empty
		if err := db.Exec("DROP TABLE scenes").Error; err != nil {
			return fmt.Errorf("failed to drop old scenes table: %w", err)
		}
		log.Println("  Dropped old scenes table")
	} else {
		// Only scenes table exists - rename it to looks
		if err := db.Exec("ALTER TABLE scenes RENAME TO looks").Error; err != nil {
			return fmt.Errorf("failed to rename scenes table: %w", err)
		}
		log.Println("  Renamed table: scenes -> looks")
	}

	// Step 2: Handle fixture_values table - need to migrate scene_id to look_id and scene_order to look_order
	// Check if old columns exist
	if db.Migrator().HasColumn("fixture_values", "scene_id") {
		// Old schema has scene_id - we need to migrate the data
		// Check if new columns also exist (from AutoMigrate)
		hasLookId := db.Migrator().HasColumn("fixture_values", "look_id")

		if hasLookId {
			// Both old and new columns exist - copy data from scene_id to look_id where look_id is null
			var countToMigrate int64
			if err := db.Raw("SELECT COUNT(*) FROM fixture_values WHERE scene_id IS NOT NULL AND (look_id IS NULL OR look_id = '')").Scan(&countToMigrate).Error; err != nil {
				return fmt.Errorf("failed to count fixture_values to migrate: %w", err)
			}

			if countToMigrate > 0 {
				if err := db.Exec(`
					UPDATE fixture_values
					SET look_id = scene_id, look_order = scene_order
					WHERE scene_id IS NOT NULL AND (look_id IS NULL OR look_id = '')
				`).Error; err != nil {
					return fmt.Errorf("failed to migrate fixture_values data: %w", err)
				}
				log.Printf("  Migrated %d fixture_values records (scene_id -> look_id)", countToMigrate)
			}

			// Now drop the old columns by recreating the table without them
			// First check the current table structure to preserve any additional columns
			if err := db.Exec(`
				CREATE TABLE fixture_values_new (
					id TEXT PRIMARY KEY,
					look_id TEXT,
					fixture_id TEXT,
					channels TEXT DEFAULT '[]',
					look_order INTEGER,
					FOREIGN KEY (look_id) REFERENCES looks(id),
					FOREIGN KEY (fixture_id) REFERENCES fixture_instances(id)
				)
			`).Error; err != nil {
				return fmt.Errorf("failed to create fixture_values_new: %w", err)
			}

			if err := db.Exec(`
				INSERT INTO fixture_values_new (id, look_id, fixture_id, channels, look_order)
				SELECT id, look_id, fixture_id, channels, look_order FROM fixture_values
			`).Error; err != nil {
				return fmt.Errorf("failed to copy fixture_values data: %w", err)
			}

			if err := db.Exec("DROP TABLE fixture_values").Error; err != nil {
				return fmt.Errorf("failed to drop old fixture_values: %w", err)
			}
			if err := db.Exec("ALTER TABLE fixture_values_new RENAME TO fixture_values").Error; err != nil {
				return fmt.Errorf("failed to rename fixture_values_new: %w", err)
			}

			// Recreate indexes (log warnings but don't fail migration for index errors)
			if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_fixture_values_look_id ON fixture_values(look_id)").Error; err != nil {
				log.Printf("  Warning: failed to create idx_fixture_values_look_id: %v", err)
			}
			if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_fixture_values_fixture_id ON fixture_values(fixture_id)").Error; err != nil {
				log.Printf("  Warning: failed to create idx_fixture_values_fixture_id: %v", err)
			}

			log.Println("  Cleaned up fixture_values table (removed scene_id, scene_order columns)")
		} else {
			// Only old columns exist - do a full table rebuild
			if err := db.Exec(`
				CREATE TABLE fixture_values_new (
					id TEXT PRIMARY KEY,
					look_id TEXT,
					fixture_id TEXT,
					channels TEXT DEFAULT '[]',
					look_order INTEGER,
					FOREIGN KEY (look_id) REFERENCES looks(id),
					FOREIGN KEY (fixture_id) REFERENCES fixture_instances(id)
				)
			`).Error; err != nil {
				return fmt.Errorf("failed to create fixture_values_new: %w", err)
			}

			if err := db.Exec(`
				INSERT INTO fixture_values_new (id, look_id, fixture_id, channels, look_order)
				SELECT id, scene_id, fixture_id, channels, scene_order FROM fixture_values
			`).Error; err != nil {
				return fmt.Errorf("failed to copy fixture_values data: %w", err)
			}

			if err := db.Exec("DROP TABLE fixture_values").Error; err != nil {
				return fmt.Errorf("failed to drop old fixture_values: %w", err)
			}
			if err := db.Exec("ALTER TABLE fixture_values_new RENAME TO fixture_values").Error; err != nil {
				return fmt.Errorf("failed to rename fixture_values_new: %w", err)
			}

			// Recreate indexes (log warnings but don't fail migration for index errors)
			if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_fixture_values_look_id ON fixture_values(look_id)").Error; err != nil {
				log.Printf("  Warning: failed to create idx_fixture_values_look_id: %v", err)
			}
			if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_fixture_values_fixture_id ON fixture_values(fixture_id)").Error; err != nil {
				log.Printf("  Warning: failed to create idx_fixture_values_fixture_id: %v", err)
			}

			log.Println("  Migrated table: fixture_values (scene_id -> look_id, scene_order -> look_order)")
		}
	}

	// Step 3: Handle cues table - need to rename scene_id to look_id
	if db.Migrator().HasColumn("cues", "scene_id") {
		// Check if new column also exists (from AutoMigrate)
		hasLookId := db.Migrator().HasColumn("cues", "look_id")

		if hasLookId {
			// Both old and new columns exist - copy data from scene_id to look_id where look_id is null
			var countToMigrate int64
			if err := db.Raw("SELECT COUNT(*) FROM cues WHERE scene_id IS NOT NULL AND (look_id IS NULL OR look_id = '')").Scan(&countToMigrate).Error; err != nil {
				return fmt.Errorf("failed to count cues to migrate: %w", err)
			}

			if countToMigrate > 0 {
				if err := db.Exec(`
					UPDATE cues
					SET look_id = scene_id
					WHERE scene_id IS NOT NULL AND (look_id IS NULL OR look_id = '')
				`).Error; err != nil {
					return fmt.Errorf("failed to migrate cues data: %w", err)
				}
				log.Printf("  Migrated %d cue records (scene_id -> look_id)", countToMigrate)
			}
		}

		// Rebuild table without the old scene_id column
		if err := db.Exec(`
			CREATE TABLE cues_new (
				id TEXT PRIMARY KEY,
				name TEXT,
				cue_number REAL,
				cue_list_id TEXT,
				look_id TEXT,
				fade_in_time REAL DEFAULT 0,
				fade_out_time REAL DEFAULT 0,
				follow_time REAL,
				easing_type TEXT,
				notes TEXT,
				skip INTEGER DEFAULT 0,
				created_at DATETIME,
				updated_at DATETIME,
				FOREIGN KEY (cue_list_id) REFERENCES cue_lists(id),
				FOREIGN KEY (look_id) REFERENCES looks(id)
			)
		`).Error; err != nil {
			return fmt.Errorf("failed to create cues_new: %w", err)
		}

		// Use look_id if it exists (post-update), otherwise use scene_id
		var insertSQL string
		if hasLookId {
			insertSQL = `
				INSERT INTO cues_new (id, name, cue_number, cue_list_id, look_id, fade_in_time, fade_out_time, follow_time, easing_type, notes, skip, created_at, updated_at)
				SELECT id, name, cue_number, cue_list_id, look_id, fade_in_time, fade_out_time, follow_time, easing_type, notes, skip, created_at, updated_at FROM cues
			`
		} else {
			insertSQL = `
				INSERT INTO cues_new (id, name, cue_number, cue_list_id, look_id, fade_in_time, fade_out_time, follow_time, easing_type, notes, skip, created_at, updated_at)
				SELECT id, name, cue_number, cue_list_id, scene_id, fade_in_time, fade_out_time, follow_time, easing_type, notes, skip, created_at, updated_at FROM cues
			`
		}

		if err := db.Exec(insertSQL).Error; err != nil {
			return fmt.Errorf("failed to copy cues data: %w", err)
		}

		if err := db.Exec("DROP TABLE cues").Error; err != nil {
			return fmt.Errorf("failed to drop old cues: %w", err)
		}
		if err := db.Exec("ALTER TABLE cues_new RENAME TO cues").Error; err != nil {
			return fmt.Errorf("failed to rename cues_new: %w", err)
		}

		// Recreate indexes (log warnings but don't fail migration for index errors)
		if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_cues_cue_list_id ON cues(cue_list_id)").Error; err != nil {
			log.Printf("  Warning: failed to create idx_cues_cue_list_id: %v", err)
		}
		if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_cues_look_id ON cues(look_id)").Error; err != nil {
			log.Printf("  Warning: failed to create idx_cues_look_id: %v", err)
		}

		log.Println("  Migrated table: cues (scene_id -> look_id)")
	}

	// Step 4: Rename scene_boards to look_boards
	if db.Migrator().HasTable("scene_boards") {
		// Check if look_boards already exists (from AutoMigrate)
		if db.Migrator().HasTable("look_boards") {
			// Both tables exist - copy data from scene_boards to look_boards, then drop scene_boards
			var lookBoardsCount, sceneBoardsCount int64
			if err := db.Raw("SELECT COUNT(*) FROM look_boards").Scan(&lookBoardsCount).Error; err != nil {
				return fmt.Errorf("failed to count look_boards: %w", err)
			}
			if err := db.Raw("SELECT COUNT(*) FROM scene_boards").Scan(&sceneBoardsCount).Error; err != nil {
				return fmt.Errorf("failed to count scene_boards: %w", err)
			}

			if sceneBoardsCount > 0 && lookBoardsCount == 0 {
				// Copy data from scene_boards to look_boards
				if err := db.Exec(`
					INSERT INTO look_boards (id, name, description, project_id, grid_size, default_fade_time, canvas_width, canvas_height, created_at, updated_at)
					SELECT id, name, description, project_id, grid_size, default_fade_time, canvas_width, canvas_height, created_at, updated_at FROM scene_boards
				`).Error; err != nil {
					return fmt.Errorf("failed to copy scene_boards to look_boards: %w", err)
				}
				log.Printf("  Copied %d scene_boards to look_boards table", sceneBoardsCount)
			} else if sceneBoardsCount > 0 && lookBoardsCount > 0 {
				// CRITICAL: Both tables have data - this is an inconsistent state that requires manual intervention
				return fmt.Errorf("migration conflict: both scene_boards (%d records) and look_boards (%d records) tables contain data; manual data reconciliation required before migration can proceed", sceneBoardsCount, lookBoardsCount)
			}

			// Only drop the old scene_boards table if we successfully copied or it was empty
			if err := db.Exec("DROP TABLE scene_boards").Error; err != nil {
				return fmt.Errorf("failed to drop old scene_boards table: %w", err)
			}
			log.Println("  Dropped old scene_boards table")
		} else {
			// Only scene_boards table exists - rename it to look_boards
			if err := db.Exec("ALTER TABLE scene_boards RENAME TO look_boards").Error; err != nil {
				return fmt.Errorf("failed to rename scene_boards table: %w", err)
			}
			log.Println("  Renamed table: scene_boards -> look_boards")
		}
	}

	// Step 5: Handle scene_board_buttons - rename to look_board_buttons and update columns
	if db.Migrator().HasTable("scene_board_buttons") {
		// Check if look_board_buttons already exists (from AutoMigrate)
		if db.Migrator().HasTable("look_board_buttons") {
			// Both tables exist - copy data from scene_board_buttons to look_board_buttons, then drop old table
			var lookBoardButtonsCount, sceneBoardButtonsCount int64
			if err := db.Raw("SELECT COUNT(*) FROM look_board_buttons").Scan(&lookBoardButtonsCount).Error; err != nil {
				return fmt.Errorf("failed to count look_board_buttons: %w", err)
			}
			if err := db.Raw("SELECT COUNT(*) FROM scene_board_buttons").Scan(&sceneBoardButtonsCount).Error; err != nil {
				return fmt.Errorf("failed to count scene_board_buttons: %w", err)
			}

			if sceneBoardButtonsCount > 0 && lookBoardButtonsCount == 0 {
				// Copy data from scene_board_buttons to look_board_buttons (translating column names)
				if err := db.Exec(`
					INSERT INTO look_board_buttons (id, look_board_id, look_id, layout_x, layout_y, width, height, color, label, created_at, updated_at)
					SELECT id, scene_board_id, scene_id, layout_x, layout_y, width, height, color, label, created_at, updated_at FROM scene_board_buttons
				`).Error; err != nil {
					return fmt.Errorf("failed to copy scene_board_buttons to look_board_buttons: %w", err)
				}
				log.Printf("  Copied %d scene_board_buttons to look_board_buttons table", sceneBoardButtonsCount)
			} else if sceneBoardButtonsCount > 0 && lookBoardButtonsCount > 0 {
				// CRITICAL: Both tables have data - this is an inconsistent state that requires manual intervention
				return fmt.Errorf("migration conflict: both scene_board_buttons (%d records) and look_board_buttons (%d records) tables contain data; manual data reconciliation required before migration can proceed", sceneBoardButtonsCount, lookBoardButtonsCount)
			}

			// Only drop the old scene_board_buttons table if we successfully copied or it was empty
			if err := db.Exec("DROP TABLE scene_board_buttons").Error; err != nil {
				return fmt.Errorf("failed to drop old scene_board_buttons: %w", err)
			}
			log.Println("  Dropped old scene_board_buttons table")
		} else {
			// Only scene_board_buttons table exists - create new table and copy data
			if err := db.Exec(`
				CREATE TABLE look_board_buttons (
					id TEXT PRIMARY KEY,
					look_board_id TEXT,
					look_id TEXT,
					layout_x INTEGER,
					layout_y INTEGER,
					width INTEGER DEFAULT 200,
					height INTEGER DEFAULT 120,
					color TEXT,
					label TEXT,
					created_at DATETIME,
					updated_at DATETIME,
					FOREIGN KEY (look_board_id) REFERENCES look_boards(id),
					FOREIGN KEY (look_id) REFERENCES looks(id)
				)
			`).Error; err != nil {
				return fmt.Errorf("failed to create look_board_buttons: %w", err)
			}

			if err := db.Exec(`
				INSERT INTO look_board_buttons (id, look_board_id, look_id, layout_x, layout_y, width, height, color, label, created_at, updated_at)
				SELECT id, scene_board_id, scene_id, layout_x, layout_y, width, height, color, label, created_at, updated_at FROM scene_board_buttons
			`).Error; err != nil {
				return fmt.Errorf("failed to copy scene_board_buttons data: %w", err)
			}

			if err := db.Exec("DROP TABLE scene_board_buttons").Error; err != nil {
				return fmt.Errorf("failed to drop old scene_board_buttons: %w", err)
			}

			log.Println("  Migrated table: scene_board_buttons -> look_board_buttons")
		}

		// Recreate indexes (log warnings but don't fail migration for index errors)
		if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_look_board_buttons_look_board_id ON look_board_buttons(look_board_id)").Error; err != nil {
			log.Printf("  Warning: failed to create idx_look_board_buttons_look_board_id: %v", err)
		}
		if err := db.Exec("CREATE INDEX IF NOT EXISTS idx_look_board_buttons_look_id ON look_board_buttons(look_id)").Error; err != nil {
			log.Printf("  Warning: failed to create idx_look_board_buttons_look_id: %v", err)
		}
	}

	log.Println("✅ Scene to look migration complete")
	return nil
}

// repairOrphanedGroupMemberships finds personal groups whose owner has no membership
// record and adds them as GROUP_ADMIN. This repairs data from before the createUserGroup
// resolver was fixed to add creators as members.
func repairOrphanedGroupMemberships(db *gorm.DB) error {
	// Find all personal groups with an owner
	var personalGroups []models.UserGroup
	if err := db.Where("is_personal = ? AND owner_id IS NOT NULL", true).Find(&personalGroups).Error; err != nil {
		return fmt.Errorf("failed to find personal groups: %w", err)
	}

	repaired := 0
	for _, group := range personalGroups {
		ownerID := *group.OwnerID

		// Check if owner already has a membership
		var count int64
		if err := db.Model(&models.UserGroupMember{}).
			Where("user_id = ? AND group_id = ?", ownerID, group.ID).
			Count(&count).Error; err != nil {
			log.Printf("  Warning: failed to check membership for group %s: %v", group.ID, err)
			continue
		}

		if count == 0 {
			// Owner is missing from their own personal group - repair it
			member := models.UserGroupMember{
				ID:      cuid.New(),
				UserID:  ownerID,
				GroupID: group.ID,
				Role:    models.GroupRoleGroupAdmin,
			}
			if err := db.Create(&member).Error; err != nil {
				log.Printf("  Warning: failed to repair membership for group %s: %v", group.ID, err)
				continue
			}
			log.Printf("  Repaired: added owner %s to personal group %s (%s)", ownerID, group.ID, group.Name)
			repaired++
		}
	}

	if repaired > 0 {
		log.Printf("Repaired %d orphaned group memberships", repaired)
	}
	return nil
}

// migrateGroupOwnership handles the migration to add group ownership fields.
// This is idempotent and runs on every startup.
// It handles:
// 1. Backfilling: setting role=MEMBER for existing user_group_members without a role
// 2. When auth enabled: assigning orphaned projects (group_id IS NULL) to a target group
//    - By default, assigns to the admin user's personal group
//    - Override with MIGRATION_TARGET_GROUP_ID env var
//
// Environment variables:
//   - MIGRATION_TARGET_GROUP_ID: Override the target group for orphaned projects
//   - MIGRATION_DRY_RUN: Set to "true" to log what would happen without making changes
func migrateGroupOwnership(db *gorm.DB, authEnabled bool) error {
	dryRun := os.Getenv("MIGRATION_DRY_RUN") == "true"

	// Step 1: Backfill role for existing user_group_members that don't have one
	if db.Migrator().HasColumn("user_group_members", "role") {
		if dryRun {
			var count int64
			db.Model(&models.UserGroupMember{}).Where("role IS NULL OR role = ''").Count(&count)
			if count > 0 {
				log.Printf("[DRY RUN] Would backfill role for %d user_group_members", count)
			}
		} else {
			result := db.Exec(`UPDATE user_group_members SET role = 'MEMBER' WHERE role IS NULL OR role = ''`)
			if result.Error != nil {
				return fmt.Errorf("failed to backfill user_group_member roles: %w", result.Error)
			}
			if result.RowsAffected > 0 {
				log.Printf("Backfilled role for %d user_group_members", result.RowsAffected)
			}
		}
	}

	// Step 2: If auth is disabled, nothing more to do (group_id stays NULL)
	if !authEnabled {
		return nil
	}

	// Step 3: Find orphaned projects (auth enabled, projects without group_id)
	var orphanedProjects []models.Project
	if err := db.Where("group_id IS NULL AND deleted_at IS NULL").
		Find(&orphanedProjects).Error; err != nil {
		return fmt.Errorf("failed to find orphaned projects: %w", err)
	}

	if len(orphanedProjects) == 0 {
		return nil // Nothing to migrate
	}

	log.Printf("Found %d orphaned projects to migrate to group ownership", len(orphanedProjects))
	for _, p := range orphanedProjects {
		log.Printf("  Orphaned project: id=%s name=%q", p.ID, p.Name)
	}

	// Check for target group override via environment variable
	targetGroupID := os.Getenv("MIGRATION_TARGET_GROUP_ID")
	if targetGroupID != "" {
		var targetGroup models.UserGroup
		if err := db.First(&targetGroup, "id = ?", targetGroupID).Error; err != nil {
			return fmt.Errorf("MIGRATION_TARGET_GROUP_ID=%s not found: %w", targetGroupID, err)
		}
		log.Printf("Using target group from MIGRATION_TARGET_GROUP_ID: %s (%s)", targetGroup.ID, targetGroup.Name)
		return assignOrphanedProjects(db, orphanedProjects, targetGroup.ID, dryRun)
	}

	// Find admin user (first ADMIN user)
	var adminUser models.User
	if err := db.Where("role = 'ADMIN'").First(&adminUser).Error; err != nil {
		log.Printf("  No admin user found, skipping project group assignment")
		return nil
	}

	// Find or create admin's personal group
	var personalGroup models.UserGroup
	err := db.Where("owner_id = ? AND is_personal = ?", adminUser.ID, true).First(&personalGroup).Error
	if err != nil {
		if err != gorm.ErrRecordNotFound {
			return fmt.Errorf("failed to find personal group: %w", err)
		}
		if dryRun {
			log.Printf("[DRY RUN] Would create personal group for admin: %s", adminUser.Email)
			log.Printf("[DRY RUN] Would assign %d orphaned projects to new group", len(orphanedProjects))
			return nil
		}
		// Create personal group for admin
		personalGroup = models.UserGroup{
			ID:         cuid.New(),
			Name:       fmt.Sprintf("%s's Projects", adminUser.Email),
			IsPersonal: true,
			OwnerID:    &adminUser.ID,
		}
		if err := db.Create(&personalGroup).Error; err != nil {
			return fmt.Errorf("failed to create admin personal group: %w", err)
		}
		// Add admin as GROUP_ADMIN member
		member := models.UserGroupMember{
			ID:      cuid.New(),
			UserID:  adminUser.ID,
			GroupID: personalGroup.ID,
			Role:    models.GroupRoleGroupAdmin,
		}
		if err := db.Create(&member).Error; err != nil {
			return fmt.Errorf("failed to add admin to personal group: %w", err)
		}
		log.Printf("  Created personal group for admin: %s", personalGroup.Name)
	}

	return assignOrphanedProjects(db, orphanedProjects, personalGroup.ID, dryRun)
}

// assignOrphanedProjects assigns orphaned projects to the given group, logging each one.
func assignOrphanedProjects(db *gorm.DB, projects []models.Project, groupID string, dryRun bool) error {
	for _, p := range projects {
		if dryRun {
			log.Printf("[DRY RUN] Would assign project id=%s name=%q to group %s", p.ID, p.Name, groupID)
			continue
		}
		if err := db.Model(&models.Project{}).
			Where("id = ?", p.ID).
			Update("group_id", groupID).Error; err != nil {
			return fmt.Errorf("failed to assign project %s to group: %w", p.ID, err)
		}
		log.Printf("  Assigned project id=%s name=%q to group %s", p.ID, p.Name, groupID)
	}
	if !dryRun {
		log.Printf("Assigned %d orphaned projects to group %s", len(projects), groupID)
	}
	return nil
}

