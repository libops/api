package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"

	"github.com/libops/api/internal/config"
	"github.com/libops/api/internal/db"
	"github.com/libops/api/internal/events"
	"github.com/libops/api/internal/middleware"
)

// TestNew tests the New function to ensure it returns a non-nil HTTP handler.
func TestNew(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock DB: %v", err)
	}
	defer func() { _ = mockDB.Close() }()

	queries := db.New(mockDB)
	ceClient := events.NewNoOpClient()
	emitter := events.NewEmitter(ceClient, events.EventSourceLibOpsAPI, queries)

	deps := &Dependencies{
		Config:         &config.Config{},
		Queries:        queries,
		Emitter:        emitter,
		Authorizer:     nil,
		JWTValidator:   nil,
		AuthHandler:    nil,
		AllowedOrigins: []string{"*"},
	}

	handler := New(deps)
	if handler == nil {
		t.Fatal("New() returned nil handler")
	}

}

// TestHealthEndpoint tests the /health endpoint to ensure it returns HTTP 200 OK and the expected body.
func TestHealthEndpoint(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock DB: %v", err)
	}
	defer func() { _ = mockDB.Close() }()

	queries := db.New(mockDB)
	ceClient := events.NewNoOpClient()
	emitter := events.NewEmitter(ceClient, events.EventSourceLibOpsAPI, queries)

	deps := &Dependencies{
		Config:         &config.Config{},
		Queries:        queries,
		Emitter:        emitter,
		AllowedOrigins: []string{"*"},
	}

	handler := New(deps)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if body := w.Body.String(); body != "OK" {
		t.Errorf("Expected body 'OK', got %q", body)
	}
}

// TestVersionEndpoint tests the /version endpoint to ensure it returns HTTP 200 OK and the correct Content-Type.
func TestVersionEndpoint(t *testing.T) {
	mockDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("Failed to create mock DB: %v", err)
	}
	defer func() { _ = mockDB.Close() }()

	queries := db.New(mockDB)
	ceClient := events.NewNoOpClient()
	emitter := events.NewEmitter(ceClient, events.EventSourceLibOpsAPI, queries)

	deps := &Dependencies{
		Config:         &config.Config{},
		Queries:        queries,
		Emitter:        emitter,
		AllowedOrigins: []string{"*"},
	}

	handler := New(deps)

	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %q", contentType)
	}

	body := w.Body.String()
	if body == "" {
		t.Error("Expected non-empty version response")
	}
}

// TestHandleHealth tests the internal handleHealth HTTP handler.
func TestHandleHealth(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	w := httptest.NewRecorder()

	handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if body := w.Body.String(); body != "OK" {
		t.Errorf("Expected body 'OK', got %q", body)
	}
}

// TestHandleVersion tests the internal handleVersion HTTP handler.
func TestHandleVersion(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/version", nil)
	w := httptest.NewRecorder()

	handleVersion(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected Content-Type 'application/json', got %q", contentType)
	}
}

// TestCORSMiddleware tests the CORS middleware functionality.
func TestCORSMiddleware(t *testing.T) {
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("test"))
	})

	handler := middleware.CorsMiddleware(testHandler, []string{"*"})

	req := httptest.NewRequest(http.MethodOptions, "/", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	allowOrigin := w.Header().Get("Access-Control-Allow-Origin")
	if allowOrigin == "" {
		t.Error("Expected Access-Control-Allow-Origin header to be set")
	}
}

// BenchmarkHealthEndpoint benchmarks the performance of the /health endpoint.
func BenchmarkHealthEndpoint(b *testing.B) {
	mockDB, _, _ := sqlmock.New()
	defer func() { _ = mockDB.Close() }()

	queries := db.New(mockDB)
	ceClient := events.NewNoOpClient()
	emitter := events.NewEmitter(ceClient, events.EventSourceLibOpsAPI, queries)

	deps := &Dependencies{
		Config:         &config.Config{},
		Queries:        queries,
		Emitter:        emitter,
		AllowedOrigins: []string{"*"},
	}

	handler := New(deps)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, req)
	}
}
