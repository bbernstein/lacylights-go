package main

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/bbernstein/lacylights-go/internal/config"
)

func TestHealthCheckHandler(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	healthCheckHandler(w, req)

	resp := w.Result()
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(body)
	if !strings.Contains(bodyStr, `"status": "ok"`) {
		t.Error("Expected status ok in response")
	}
	if !strings.Contains(bodyStr, `"version":`) {
		t.Error("Expected version in response")
	}
	if !strings.Contains(bodyStr, `"timestamp":`) {
		t.Error("Expected timestamp in response")
	}
}

func TestPrintBanner(t *testing.T) {
	// Capture stdout
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	cfg := &config.Config{
		Env:         "test",
		Port:        "4000",
		DatabaseURL: "test.db",
	}

	printBanner(cfg)

	_ = w.Close()
	os.Stdout = oldStdout

	var buf bytes.Buffer
	_, _ = io.Copy(&buf, r)
	output := buf.String()

	// Verify banner contains expected elements
	if !strings.Contains(output, "LacyLights Go Server") {
		t.Error("Expected 'LacyLights Go Server' in banner")
	}
	if !strings.Contains(output, "Version:") {
		t.Error("Expected 'Version:' in banner")
	}
	if !strings.Contains(output, "Environment: test") {
		t.Error("Expected 'Environment: test' in banner")
	}
	if !strings.Contains(output, "Port:        4000") {
		t.Error("Expected 'Port: 4000' in banner")
	}
	if !strings.Contains(output, "Database:    test.db") {
		t.Error("Expected 'Database: test.db' in banner")
	}
}

func TestVersionVariables(t *testing.T) {
	// These are set at build time, but we can verify they have default values
	if Version == "" {
		t.Error("Version should have a default value")
	}
	if BuildTime == "" {
		t.Error("BuildTime should have a default value")
	}
	if GitCommit == "" {
		t.Error("GitCommit should have a default value")
	}
}
