// Package main provides a one-time migration tool to convert fixture layout
// coordinates from normalized 0-1 values to pixel-based coordinates.
//
// Usage: go run ./cmd/migrate-layout
//
// This script:
// 1. Finds all fixtures with layoutX/layoutY values in the 0-1 range
// 2. Converts them to pixel coordinates (normalized * canvasSize)
// 3. Uses the project's layoutCanvasWidth/Height (defaults to 2000x2000)
//
// The script is idempotent - it will only convert values that appear to be
// normalized (both x and y are between 0 and 1 inclusive).
package main

import (
	"fmt"
	"log"
	"os"

	"github.com/bbernstein/lacylights-go/internal/database"
	"github.com/bbernstein/lacylights-go/internal/database/models"
	"github.com/joho/godotenv"
)

const (
	defaultCanvasWidth  = 2000
	defaultCanvasHeight = 2000
)

// isNormalized checks if a value appears to be a normalized 0-1 coordinate
func isNormalized(val *float64) bool {
	if val == nil {
		return false
	}
	return *val >= 0 && *val <= 1
}

func main() {
	// Load .env file if present
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Get database URL from environment
	dbURL := os.Getenv("DATABASE_URL")
	if dbURL == "" {
		dbURL = "file:./data/lacylights.db?_fk=1"
	}

	log.Printf("Connecting to database: %s", dbURL)

	// Connect to database
	db, err := database.Connect(database.Config{
		URL:         dbURL,
		MaxIdleConn: 1,
		MaxOpenConn: 1,
		Debug:       true,
	})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	defer func() { _ = database.Close() }()

	// Load all projects to get their canvas dimensions
	var projects []models.Project
	if err := db.Find(&projects).Error; err != nil {
		log.Fatalf("Failed to load projects: %v", err)
	}

	// Build a map of project ID to canvas dimensions
	projectCanvas := make(map[string]struct{ width, height int })
	for _, p := range projects {
		width := p.LayoutCanvasWidth
		height := p.LayoutCanvasHeight
		if width == 0 {
			width = defaultCanvasWidth
		}
		if height == 0 {
			height = defaultCanvasHeight
		}
		projectCanvas[p.ID] = struct{ width, height int }{width, height}
	}

	// Find all fixtures with layout positions
	var fixtures []models.FixtureInstance
	if err := db.Where("layout_x IS NOT NULL AND layout_y IS NOT NULL").Find(&fixtures).Error; err != nil {
		log.Fatalf("Failed to load fixtures: %v", err)
	}

	log.Printf("Found %d fixtures with layout positions", len(fixtures))

	// Count statistics
	converted := 0
	skipped := 0
	errored := 0
	var erroredFixtures []string

	for _, fixture := range fixtures {
		// Check if both coordinates appear to be normalized (0-1 range)
		if !isNormalized(fixture.LayoutX) || !isNormalized(fixture.LayoutY) {
			// Already in pixel coordinates or invalid
			skipped++
			continue
		}

		// Get canvas dimensions for this fixture's project
		canvas, ok := projectCanvas[fixture.ProjectID]
		if !ok {
			log.Printf("Warning: Fixture %s has no project, using default canvas size", fixture.ID)
			canvas = struct{ width, height int }{defaultCanvasWidth, defaultCanvasHeight}
		}

		// Convert normalized to pixel coordinates
		oldX, oldY := *fixture.LayoutX, *fixture.LayoutY
		newX := oldX * float64(canvas.width)
		newY := oldY * float64(canvas.height)

		fixture.LayoutX = &newX
		fixture.LayoutY = &newY

		// Save the updated fixture
		if err := db.Save(&fixture).Error; err != nil {
			log.Printf("Error updating fixture %s: %v", fixture.ID, err)
			errored++
			erroredFixtures = append(erroredFixtures, fixture.ID)
			continue
		}

		log.Printf("Converted fixture %s (%s): (%.4f, %.4f) -> (%.1f, %.1f)",
			fixture.ID, fixture.Name, oldX, oldY, newX, newY)
		converted++
	}

	fmt.Printf("\nMigration complete:\n")
	fmt.Printf("  Total fixtures with positions: %d\n", len(fixtures))
	fmt.Printf("  Converted (normalized -> pixels): %d\n", converted)
	fmt.Printf("  Skipped (already pixels or invalid): %d\n", skipped)
	fmt.Printf("  Errors: %d\n", errored)

	if errored > 0 {
		fmt.Printf("\nFailed fixture IDs:\n")
		for _, id := range erroredFixtures {
			fmt.Printf("  - %s\n", id)
		}
	}

	if converted > 0 {
		fmt.Println("\nNote: Fixture positions have been converted from 0-1 normalized")
		fmt.Println("coordinates to pixel-based coordinates using the project's canvas size")
		fmt.Println("(default 2000x2000 if not set).")
	}

	// Exit with non-zero status if there were errors
	if errored > 0 {
		os.Exit(1)
	}
}
