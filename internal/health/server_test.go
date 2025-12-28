package health

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// mockStatsProvider implements StatsProvider for testing.
type mockStatsProvider struct {
	running bool
	stats   Stats
}

func (m *mockStatsProvider) IsRunning() bool {
	return m.running
}

func (m *mockStatsProvider) Stats() Stats {
	return m.stats
}

func TestNewServer(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: true}

	s := NewServer(cfg, provider)
	if s == nil {
		t.Fatal("NewServer returned nil")
	}
}

func TestServer_handleHealth(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()
	if body != "OK\n" {
		t.Errorf("expected body 'OK\\n', got %q", body)
	}
}

func TestServer_handleHealth_MethodNotAllowed(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	req := httptest.NewRequest(http.MethodPost, "/health", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status %d, got %d", http.StatusMethodNotAllowed, rec.Code)
	}
}

func TestServer_handleHealthz_Running(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{
		running: true,
		stats: Stats{
			PeerCount:      5,
			StreamCount:    10,
			RouteCount:     3,
			SOCKS5Running:  true,
			ExitHandlerRun: true,
		},
	}
	s := NewServer(cfg, provider)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["status"] != "healthy" {
		t.Errorf("expected status 'healthy', got %v", response["status"])
	}
	if response["running"] != true {
		t.Errorf("expected running true, got %v", response["running"])
	}
	if int(response["peer_count"].(float64)) != 5 {
		t.Errorf("expected peer_count 5, got %v", response["peer_count"])
	}
	if int(response["stream_count"].(float64)) != 10 {
		t.Errorf("expected stream_count 10, got %v", response["stream_count"])
	}
}

func TestServer_handleHealthz_NotRunning(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: false}
	s := NewServer(cfg, provider)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["status"] != "unavailable" {
		t.Errorf("expected status 'unavailable', got %v", response["status"])
	}
}

func TestServer_handleReady_Ready(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	body := rec.Body.String()
	if body != "READY\n" {
		t.Errorf("expected body 'READY\\n', got %q", body)
	}
}

func TestServer_handleReady_NotReady(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: false}
	s := NewServer(cfg, provider)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}

	body := rec.Body.String()
	if body != "NOT READY\n" {
		t.Errorf("expected body 'NOT READY\\n', got %q", body)
	}
}

func TestServer_StartStop(t *testing.T) {
	cfg := ServerConfig{
		Address:      "127.0.0.1:0", // Dynamic port
		ReadTimeout:  5 * time.Second,
		WriteTimeout: 5 * time.Second,
	}
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	if err := s.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	if !s.IsRunning() {
		t.Error("expected server to be running")
	}

	addr := s.Address()
	if addr == nil {
		t.Fatal("expected non-nil address")
	}

	// Give the server time to start accepting connections
	// Use retry loop to handle race between Start() and Serve()
	var resp *http.Response
	var err error
	for i := 0; i < 10; i++ {
		time.Sleep(10 * time.Millisecond)
		resp, err = http.Get("http://" + addr.String() + "/health")
		if err == nil {
			break
		}
	}
	if err != nil {
		t.Fatalf("request failed after retries: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "OK\n" {
		t.Errorf("expected body 'OK\\n', got %q", body)
	}

	if err := s.Stop(); err != nil {
		t.Errorf("failed to stop: %v", err)
	}

	if s.IsRunning() {
		t.Error("expected server to be stopped")
	}
}

func TestServer_DoubleStop(t *testing.T) {
	cfg := ServerConfig{
		Address: "127.0.0.1:0",
	}
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	if err := s.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}

	// Stop twice should not error
	if err := s.Stop(); err != nil {
		t.Errorf("first stop failed: %v", err)
	}
	if err := s.Stop(); err != nil {
		t.Errorf("second stop failed: %v", err)
	}
}

func TestServer_NilProvider(t *testing.T) {
	cfg := DefaultServerConfig()
	s := NewServer(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status %d, got %d", http.StatusServiceUnavailable, rec.Code)
	}
}

func TestServer_PprofIndex(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	// Check that the response contains pprof content
	body := rec.Body.String()
	if len(body) == 0 {
		t.Error("expected non-empty body for pprof index")
	}
}

func TestServer_PprofCmdline(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/cmdline", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}

func TestServer_PprofSymbol(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	req := httptest.NewRequest(http.MethodGet, "/debug/pprof/symbol", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	// Symbol accepts both GET and POST
	if rec.Code != http.StatusOK {
		t.Errorf("expected status %d, got %d", http.StatusOK, rec.Code)
	}
}
