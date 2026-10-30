package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
)

func TestHealthLive(t *testing.T) {
	app := fiber.New()
	h := &Handler{}
	app.Get("/health/live", h.HealthLive)

	req := httptest.NewRequest("GET", "/health/live", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if body["status"] != "alive" {
		t.Errorf("expected status alive, got %v", body["status"])
	}
	if body["timestamp"] == "" {
		t.Errorf("expected non-empty timestamp")
	}
}
