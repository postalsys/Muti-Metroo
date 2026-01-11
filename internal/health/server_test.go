package health

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/protocol"
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

// ============================================================================
// Helper Function Tests
// ============================================================================

func TestWriteJSON(t *testing.T) {
	tests := []struct {
		name           string
		status         int
		data           interface{}
		expectedStatus int
		expectedKeys   []string
	}{
		{
			name:           "simple map",
			status:         http.StatusOK,
			data:           map[string]string{"key": "value"},
			expectedStatus: http.StatusOK,
			expectedKeys:   []string{"key"},
		},
		{
			name:           "status with message",
			status:         http.StatusBadRequest,
			data:           map[string]interface{}{"error": "bad request", "code": 400},
			expectedStatus: http.StatusBadRequest,
			expectedKeys:   []string{"error", "code"},
		},
		{
			name:           "stats struct",
			status:         http.StatusOK,
			data:           Stats{PeerCount: 5, StreamCount: 10},
			expectedStatus: http.StatusOK,
			expectedKeys:   []string{"peer_count", "stream_count"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			writeJSON(rec, tc.status, tc.data)

			if rec.Code != tc.expectedStatus {
				t.Errorf("status = %d, want %d", rec.Code, tc.expectedStatus)
			}

			contentType := rec.Header().Get("Content-Type")
			if contentType != "application/json" {
				t.Errorf("Content-Type = %q, want %q", contentType, "application/json")
			}

			var result map[string]interface{}
			if err := json.NewDecoder(rec.Body).Decode(&result); err != nil {
				t.Fatalf("failed to decode JSON: %v", err)
			}

			for _, key := range tc.expectedKeys {
				if _, ok := result[key]; !ok {
					t.Errorf("expected key %q in response", key)
				}
			}
		})
	}
}

func TestRequireGET(t *testing.T) {
	tests := []struct {
		method   string
		expected bool
		status   int
	}{
		{http.MethodGet, true, http.StatusOK},
		{http.MethodPost, false, http.StatusMethodNotAllowed},
		{http.MethodPut, false, http.StatusMethodNotAllowed},
		{http.MethodDelete, false, http.StatusMethodNotAllowed},
		{http.MethodPatch, false, http.StatusMethodNotAllowed},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "/test", nil)

			result := requireGET(rec, req)

			if result != tc.expected {
				t.Errorf("requireGET() = %v, want %v", result, tc.expected)
			}

			if !tc.expected && rec.Code != tc.status {
				t.Errorf("status = %d, want %d", rec.Code, tc.status)
			}
		})
	}
}

func TestRequirePOST(t *testing.T) {
	tests := []struct {
		method   string
		expected bool
		status   int
	}{
		{http.MethodPost, true, http.StatusOK},
		{http.MethodGet, false, http.StatusMethodNotAllowed},
		{http.MethodPut, false, http.StatusMethodNotAllowed},
		{http.MethodDelete, false, http.StatusMethodNotAllowed},
	}

	for _, tc := range tests {
		t.Run(tc.method, func(t *testing.T) {
			rec := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, "/test", nil)

			result := requirePOST(rec, req)

			if result != tc.expected {
				t.Errorf("requirePOST() = %v, want %v", result, tc.expected)
			}

			if !tc.expected && rec.Code != tc.status {
				t.Errorf("status = %d, want %d", rec.Code, tc.status)
			}
		})
	}
}

func TestCalculateUptimeHours(t *testing.T) {
	tests := []struct {
		name      string
		startTime int64
		minHours  float64
		maxHours  float64
	}{
		{
			name:      "zero start time",
			startTime: 0,
			minHours:  0,
			maxHours:  0,
		},
		{
			name:      "negative start time",
			startTime: -100,
			minHours:  0,
			maxHours:  0,
		},
		{
			name:      "one hour ago",
			startTime: time.Now().Unix() - 3600,
			minHours:  0.9,
			maxHours:  1.1,
		},
		{
			name:      "24 hours ago",
			startTime: time.Now().Unix() - 86400,
			minHours:  23.9,
			maxHours:  24.1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := calculateUptimeHours(tc.startTime)

			if result < tc.minHours || result > tc.maxHours {
				t.Errorf("calculateUptimeHours(%d) = %f, want between %f and %f",
					tc.startTime, result, tc.minHours, tc.maxHours)
			}
		})
	}
}

func TestPopulateNodeInfo(t *testing.T) {
	t.Run("nil nodeInfo", func(t *testing.T) {
		agent := &TopologyAgentInfo{
			ShortID:     "abc123",
			DisplayName: "abc123",
		}
		populateNodeInfo(agent, nil)

		// Should not modify agent when nodeInfo is nil
		if agent.Hostname != "" {
			t.Errorf("Hostname should be empty, got %q", agent.Hostname)
		}
	})

	t.Run("populates all fields", func(t *testing.T) {
		agent := &TopologyAgentInfo{
			ShortID:     "abc123",
			DisplayName: "abc123",
		}
		nodeInfo := &protocol.NodeInfo{
			DisplayName: "test-agent",
			Hostname:    "server1.example.com",
			OS:          "linux",
			Arch:        "amd64",
			Version:     "2.0.0",
			StartTime:   time.Now().Unix() - 7200, // 2 hours ago
			IPAddresses: []string{"192.168.1.10", "10.0.0.5"},
			UDPEnabled:  true,
		}

		populateNodeInfo(agent, nodeInfo)

		if agent.Hostname != "server1.example.com" {
			t.Errorf("Hostname = %q, want %q", agent.Hostname, "server1.example.com")
		}
		if agent.OS != "linux" {
			t.Errorf("OS = %q, want %q", agent.OS, "linux")
		}
		if agent.Arch != "amd64" {
			t.Errorf("Arch = %q, want %q", agent.Arch, "amd64")
		}
		if agent.Version != "2.0.0" {
			t.Errorf("Version = %q, want %q", agent.Version, "2.0.0")
		}
		if len(agent.IPAddresses) != 2 {
			t.Errorf("IPAddresses len = %d, want 2", len(agent.IPAddresses))
		}
		if !agent.UDPEnabled {
			t.Error("UDPEnabled should be true")
		}
		// Uptime should be approximately 2 hours
		if agent.UptimeHours < 1.9 || agent.UptimeHours > 2.1 {
			t.Errorf("UptimeHours = %f, want approximately 2", agent.UptimeHours)
		}
		// DisplayName should be updated from nodeInfo when it matches ShortID
		if agent.DisplayName != "test-agent" {
			t.Errorf("DisplayName = %q, want %q", agent.DisplayName, "test-agent")
		}
	})

	t.Run("does not override custom display name", func(t *testing.T) {
		agent := &TopologyAgentInfo{
			ShortID:     "abc123",
			DisplayName: "custom-name", // Not matching ShortID
		}
		nodeInfo := &protocol.NodeInfo{
			DisplayName: "nodeinfo-name",
		}

		populateNodeInfo(agent, nodeInfo)

		// Should keep custom name since it doesn't match ShortID
		if agent.DisplayName != "custom-name" {
			t.Errorf("DisplayName = %q, want %q", agent.DisplayName, "custom-name")
		}
	})
}

func TestBuildAgentRoles(t *testing.T) {
	cfg := DefaultServerConfig()
	s := NewServer(cfg, nil)

	tests := []struct {
		name                string
		isLocal             bool
		hasSOCKS5           bool
		hasExitRoutes       bool
		hasForwardListeners bool
		hasForwardEndpoints bool
		expected            []string
	}{
		{
			name:                "transit only",
			isLocal:             false,
			hasSOCKS5:           false,
			hasExitRoutes:       false,
			hasForwardListeners: false,
			hasForwardEndpoints: false,
			expected:            []string{"transit"},
		},
		{
			name:                "ingress only",
			isLocal:             true,
			hasSOCKS5:           true,
			hasExitRoutes:       false,
			hasForwardListeners: false,
			hasForwardEndpoints: false,
			expected:            []string{"ingress"},
		},
		{
			name:                "exit only",
			isLocal:             false,
			hasSOCKS5:           false,
			hasExitRoutes:       true,
			hasForwardListeners: false,
			hasForwardEndpoints: false,
			expected:            []string{"exit"},
		},
		{
			name:                "ingress and exit",
			isLocal:             true,
			hasSOCKS5:           true,
			hasExitRoutes:       true,
			hasForwardListeners: false,
			hasForwardEndpoints: false,
			expected:            []string{"ingress", "exit"},
		},
		{
			name:                "forward ingress only",
			isLocal:             true,
			hasSOCKS5:           false,
			hasExitRoutes:       false,
			hasForwardListeners: true,
			hasForwardEndpoints: false,
			expected:            []string{"forward_ingress"},
		},
		{
			name:                "forward exit only",
			isLocal:             false,
			hasSOCKS5:           false,
			hasExitRoutes:       false,
			hasForwardListeners: false,
			hasForwardEndpoints: true,
			expected:            []string{"forward_exit"},
		},
		{
			name:                "all roles",
			isLocal:             true,
			hasSOCKS5:           true,
			hasExitRoutes:       true,
			hasForwardListeners: true,
			hasForwardEndpoints: true,
			expected:            []string{"ingress", "exit", "forward_ingress", "forward_exit"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			roles := s.buildAgentRoles(tc.isLocal, tc.hasSOCKS5, tc.hasExitRoutes, tc.hasForwardListeners, tc.hasForwardEndpoints)

			if len(roles) != len(tc.expected) {
				t.Errorf("roles len = %d, want %d", len(roles), len(tc.expected))
				return
			}

			for i, role := range roles {
				if role != tc.expected[i] {
					t.Errorf("roles[%d] = %q, want %q", i, role, tc.expected[i])
				}
			}
		})
	}
}

// ============================================================================
// Splash Page Handler Tests
// ============================================================================

func TestServer_handleSplash(t *testing.T) {
	t.Run("root path with dashboard enabled", func(t *testing.T) {
		cfg := DefaultServerConfig()
		cfg.EnableDashboard = true
		s := NewServer(cfg, nil)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		body := rec.Body.String()
		if !strings.Contains(body, "Muti Metroo") {
			t.Error("expected body to contain 'Muti Metroo'")
		}
		if !strings.Contains(body, "Open Dashboard") {
			t.Error("expected body to contain dashboard link when enabled")
		}
	})

	t.Run("root path with dashboard disabled", func(t *testing.T) {
		cfg := DefaultServerConfig()
		cfg.EnableDashboard = false
		s := NewServer(cfg, nil)

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		body := rec.Body.String()
		if strings.Contains(body, "Open Dashboard") {
			t.Error("expected no dashboard link when disabled")
		}
	})

	t.Run("non-root path returns 404", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)

		req := httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("POST to root not allowed", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)

		req := httptest.NewRequest(http.MethodPost, "/", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})
}

// ============================================================================
// Disabled Endpoint Tests
// ============================================================================

func TestServer_DisabledEndpoints(t *testing.T) {
	t.Run("pprof disabled", func(t *testing.T) {
		cfg := DefaultServerConfig()
		cfg.EnablePprof = false
		s := NewServer(cfg, nil)

		req := httptest.NewRequest(http.MethodGet, "/debug/pprof/", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
		}
	})

	t.Run("dashboard disabled", func(t *testing.T) {
		cfg := DefaultServerConfig()
		cfg.EnableDashboard = false
		s := NewServer(cfg, nil)

		endpoints := []string{"/ui/", "/api/topology", "/api/dashboard"}
		for _, endpoint := range endpoints {
			req := httptest.NewRequest(http.MethodGet, endpoint, nil)
			rec := httptest.NewRecorder()

			s.server.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("%s: status = %d, want %d", endpoint, rec.Code, http.StatusNotFound)
			}
		}
	})

	t.Run("remote API disabled", func(t *testing.T) {
		cfg := DefaultServerConfig()
		cfg.EnableRemoteAPI = false
		s := NewServer(cfg, nil)

		endpoints := []string{"/agents", "/agents/", "/routes/advertise"}
		for _, endpoint := range endpoints {
			method := http.MethodGet
			if endpoint == "/routes/advertise" {
				method = http.MethodPost
			}
			req := httptest.NewRequest(method, endpoint, nil)
			rec := httptest.NewRecorder()

			s.server.Handler.ServeHTTP(rec, req)

			if rec.Code != http.StatusNotFound {
				t.Errorf("%s: status = %d, want %d", endpoint, rec.Code, http.StatusNotFound)
			}
		}
	})
}

// ============================================================================
// Route Trigger Tests
// ============================================================================

// mockRouteAdvertiseTrigger implements RouteAdvertiseTrigger for testing.
type mockRouteAdvertiseTrigger struct {
	triggered bool
}

func (m *mockRouteAdvertiseTrigger) TriggerRouteAdvertise() {
	m.triggered = true
}

func TestServer_handleTriggerAdvertise(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)
		trigger := &mockRouteAdvertiseTrigger{}
		s.SetRouteAdvertiseTrigger(trigger)

		req := httptest.NewRequest(http.MethodPost, "/routes/advertise", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		if !trigger.triggered {
			t.Error("expected route trigger to be called")
		}

		var response map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response["status"] != "triggered" {
			t.Errorf("status = %v, want %q", response["status"], "triggered")
		}
	})

	t.Run("GET not allowed", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)
		s.SetRouteAdvertiseTrigger(&mockRouteAdvertiseTrigger{})

		req := httptest.NewRequest(http.MethodGet, "/routes/advertise", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("no trigger configured", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)
		// Don't set route trigger

		req := httptest.NewRequest(http.MethodPost, "/routes/advertise", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

// ============================================================================
// Remote Provider Tests
// ============================================================================

// mockRemoteStatusProvider implements RemoteStatusProvider for testing.
type mockRemoteStatusProvider struct {
	id                  identity.AgentID
	displayName         string
	peerIDs             []identity.AgentID
	knownAgentIDs       []identity.AgentID
	peerDetails         []PeerDetails
	routeDetails        []RouteDetails
	domainRoutesList    []DomainRouteDetails
	forwardRoutesList    []PortForwardRouteDetails
	displayNames        map[identity.AgentID]string
	allNodeInfo         map[identity.AgentID]*protocol.NodeInfo
	localNodeInfo       *protocol.NodeInfo
	socks5Info          SOCKS5Info
	udpInfo             UDPInfo
	forwardInfo          PortForwardInfo
}

func (m *mockRemoteStatusProvider) ID() identity.AgentID {
	return m.id
}

func (m *mockRemoteStatusProvider) DisplayName() string {
	return m.displayName
}

func (m *mockRemoteStatusProvider) SendControlRequest(ctx context.Context, targetID identity.AgentID, controlType uint8) (*protocol.ControlResponse, error) {
	return &protocol.ControlResponse{Success: true, Data: []byte("{}")}, nil
}

func (m *mockRemoteStatusProvider) SendControlRequestWithData(ctx context.Context, targetID identity.AgentID, controlType uint8, data []byte) (*protocol.ControlResponse, error) {
	return &protocol.ControlResponse{Success: true, Data: []byte("{}")}, nil
}

func (m *mockRemoteStatusProvider) GetPeerIDs() []identity.AgentID {
	return m.peerIDs
}

func (m *mockRemoteStatusProvider) GetKnownAgentIDs() []identity.AgentID {
	return m.knownAgentIDs
}

func (m *mockRemoteStatusProvider) GetPeerDetails() []PeerDetails {
	return m.peerDetails
}

func (m *mockRemoteStatusProvider) GetRouteDetails() []RouteDetails {
	return m.routeDetails
}

func (m *mockRemoteStatusProvider) GetDomainRouteDetails() []DomainRouteDetails {
	return m.domainRoutesList
}

func (m *mockRemoteStatusProvider) GetAllDisplayNames() map[identity.AgentID]string {
	return m.displayNames
}

func (m *mockRemoteStatusProvider) GetAllNodeInfo() map[identity.AgentID]*protocol.NodeInfo {
	return m.allNodeInfo
}

func (m *mockRemoteStatusProvider) GetLocalNodeInfo() *protocol.NodeInfo {
	return m.localNodeInfo
}

func (m *mockRemoteStatusProvider) GetSOCKS5Info() SOCKS5Info {
	return m.socks5Info
}

func (m *mockRemoteStatusProvider) GetUDPInfo() UDPInfo {
	return m.udpInfo
}

func (m *mockRemoteStatusProvider) GetPortForwardInfo() PortForwardInfo {
	return m.forwardInfo
}

func (m *mockRemoteStatusProvider) GetPortForwardRouteDetails() []PortForwardRouteDetails {
	return m.forwardRoutesList
}

func (m *mockRemoteStatusProvider) UploadFile(ctx context.Context, targetID identity.AgentID, localPath, remotePath string, opts TransferOptions, progress FileTransferProgress) error {
	return nil
}

func (m *mockRemoteStatusProvider) DownloadFile(ctx context.Context, targetID identity.AgentID, remotePath, localPath string, opts TransferOptions, progress FileTransferProgress) error {
	return nil
}

func (m *mockRemoteStatusProvider) DownloadFileStream(ctx context.Context, targetID identity.AgentID, remotePath string, opts TransferOptions) (*DownloadStreamResult, error) {
	return nil, nil
}

func TestServer_handleListAgents(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cfg := DefaultServerConfig()
		provider := &mockStatsProvider{running: true}
		s := NewServer(cfg, provider)

		localID, _ := identity.NewAgentID()
		peerID, _ := identity.NewAgentID()

		remoteProvider := &mockRemoteStatusProvider{
			id:            localID,
			displayName:   "local-agent",
			knownAgentIDs: []identity.AgentID{localID, peerID},
		}
		s.SetRemoteProvider(remoteProvider)

		req := httptest.NewRequest(http.MethodGet, "/agents", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var agents []map[string]interface{}
		if err := json.NewDecoder(rec.Body).Decode(&agents); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(agents) != 2 {
			t.Errorf("expected 2 agents, got %d", len(agents))
		}

		// First agent should be local
		if agents[0]["local"] != true {
			t.Error("first agent should be local")
		}
	})

	t.Run("no remote provider", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)
		// Don't set remote provider

		req := httptest.NewRequest(http.MethodGet, "/agents", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

func TestServer_handleTopology(t *testing.T) {
	t.Run("success with peers and routes", func(t *testing.T) {
		cfg := DefaultServerConfig()
		provider := &mockStatsProvider{
			running: true,
			stats: Stats{
				SOCKS5Running:  true,
				ExitHandlerRun: false,
			},
		}
		s := NewServer(cfg, provider)

		localID, _ := identity.NewAgentID()
		peerID, _ := identity.NewAgentID()

		remoteProvider := &mockRemoteStatusProvider{
			id:          localID,
			displayName: "local-agent",
			peerIDs:     []identity.AgentID{peerID},
			peerDetails: []PeerDetails{
				{
					ID:          peerID,
					DisplayName: "peer-agent",
					State:       "connected",
					RTT:         50 * time.Millisecond,
					Transport:   "quic",
				},
			},
			routeDetails: []RouteDetails{
				{
					Network: "10.0.0.0/8",
					NextHop: peerID,
					Origin:  peerID,
					Metric:  1,
					Path:    []identity.AgentID{peerID},
				},
			},
			displayNames: map[identity.AgentID]string{
				peerID: "peer-agent",
			},
			allNodeInfo:   map[identity.AgentID]*protocol.NodeInfo{},
			localNodeInfo: &protocol.NodeInfo{},
			socks5Info: SOCKS5Info{
				Enabled: true,
				Address: "127.0.0.1:1080",
			},
		}
		s.SetRemoteProvider(remoteProvider)

		req := httptest.NewRequest(http.MethodGet, "/api/topology", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var response TopologyResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if !response.LocalAgent.IsLocal {
			t.Error("expected local agent to have IsLocal=true")
		}

		if len(response.Agents) < 1 {
			t.Error("expected at least 1 agent in response")
		}

		if len(response.Connections) < 1 {
			t.Error("expected at least 1 connection in response")
		}
	})

	t.Run("no remote provider", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/topology", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

func TestServer_handleDashboard(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cfg := DefaultServerConfig()
		provider := &mockStatsProvider{
			running: true,
			stats: Stats{
				PeerCount:   2,
				StreamCount: 5,
				RouteCount:  3,
			},
		}
		s := NewServer(cfg, provider)

		localID, _ := identity.NewAgentID()
		peerID, _ := identity.NewAgentID()

		remoteProvider := &mockRemoteStatusProvider{
			id:          localID,
			displayName: "local-agent",
			peerDetails: []PeerDetails{
				{
					ID:          peerID,
					DisplayName: "peer-agent",
					State:       "connected",
					RTT:         30 * time.Millisecond,
				},
			},
			routeDetails: []RouteDetails{
				{
					Network: "0.0.0.0/0",
					NextHop: peerID,
					Origin:  peerID,
					Metric:  1,
					Path:    []identity.AgentID{peerID},
				},
			},
			displayNames: map[identity.AgentID]string{
				peerID: "peer-agent",
			},
			allNodeInfo: map[identity.AgentID]*protocol.NodeInfo{},
		}
		s.SetRemoteProvider(remoteProvider)

		req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var response DashboardResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response.Stats.PeerCount != 2 {
			t.Errorf("PeerCount = %d, want 2", response.Stats.PeerCount)
		}

		if len(response.Peers) != 1 {
			t.Errorf("expected 1 peer, got %d", len(response.Peers))
		}

		if len(response.Routes) != 1 {
			t.Errorf("expected 1 route, got %d", len(response.Routes))
		}
	})

	t.Run("no providers configured", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

func TestServer_handleNodes(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		cfg := DefaultServerConfig()
		provider := &mockStatsProvider{running: true}
		s := NewServer(cfg, provider)

		localID, _ := identity.NewAgentID()
		peerID, _ := identity.NewAgentID()

		remoteProvider := &mockRemoteStatusProvider{
			id:          localID,
			displayName: "local-agent",
			peerIDs:     []identity.AgentID{peerID},
			localNodeInfo: &protocol.NodeInfo{
				Hostname: "local-host",
				OS:       "linux",
			},
			allNodeInfo: map[identity.AgentID]*protocol.NodeInfo{
				peerID: {
					DisplayName: "peer-agent",
					Hostname:    "peer-host",
					OS:          "darwin",
				},
			},
		}
		s.SetRemoteProvider(remoteProvider)

		req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
		}

		var response NodesResponse
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if len(response.Nodes) != 2 {
			t.Errorf("expected 2 nodes, got %d", len(response.Nodes))
		}

		// First node should be local
		if !response.Nodes[0].IsLocal {
			t.Error("first node should be local")
		}
	})

	t.Run("no remote provider", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)

		req := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

// ============================================================================
// Agent Info Handler Tests
// ============================================================================

func TestServer_handleAgentInfo(t *testing.T) {
	t.Run("invalid agent ID", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)
		s.SetRemoteProvider(&mockRemoteStatusProvider{})

		req := httptest.NewRequest(http.MethodGet, "/agents/invalid-id", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("empty agent ID", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)
		s.SetRemoteProvider(&mockRemoteStatusProvider{})

		req := httptest.NewRequest(http.MethodGet, "/agents/", nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusBadRequest)
		}
	})

	t.Run("no remote provider", func(t *testing.T) {
		cfg := DefaultServerConfig()
		s := NewServer(cfg, nil)
		// Don't set remote provider

		agentID, _ := identity.NewAgentID()
		req := httptest.NewRequest(http.MethodGet, "/agents/"+agentID.String(), nil)
		rec := httptest.NewRecorder()

		s.server.Handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("status = %d, want %d", rec.Code, http.StatusServiceUnavailable)
		}
	})
}

// ============================================================================
// BuildLocalAgentInfo Tests
// ============================================================================

func TestServer_buildLocalAgentInfo(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	localID, _ := identity.NewAgentID()

	remoteProvider := &mockRemoteStatusProvider{
		id:          localID,
		displayName: "test-agent",
		localNodeInfo: &protocol.NodeInfo{
			Hostname:    "test-host",
			OS:          "linux",
			Arch:        "amd64",
			Version:     "2.0.0",
			StartTime:   time.Now().Unix() - 3600,
			IPAddresses: []string{"192.168.1.10"},
		},
	}
	s.SetRemoteProvider(remoteProvider)

	stats := Stats{
		SOCKS5Running:  true,
		ExitHandlerRun: true,
	}
	socks5Info := SOCKS5Info{
		Enabled: true,
		Address: "127.0.0.1:1080",
	}
	udpInfo := UDPInfo{
		Enabled: true,
	}
	forwardInfo := PortForwardInfo{
		ListenerKeys: []string{"web"},
		EndpointKeys: []string{"api"},
	}

	agent := s.buildLocalAgentInfo(localID, "test-agent", stats, socks5Info, udpInfo, forwardInfo)

	if agent.ID != localID.String() {
		t.Errorf("ID = %q, want %q", agent.ID, localID.String())
	}
	if agent.DisplayName != "test-agent" {
		t.Errorf("DisplayName = %q, want %q", agent.DisplayName, "test-agent")
	}
	if !agent.IsLocal {
		t.Error("expected IsLocal to be true")
	}
	if !agent.IsConnected {
		t.Error("expected IsConnected to be true")
	}
	if agent.SOCKS5Addr != "127.0.0.1:1080" {
		t.Errorf("SOCKS5Addr = %q, want %q", agent.SOCKS5Addr, "127.0.0.1:1080")
	}
	if !agent.UDPEnabled {
		t.Error("expected UDPEnabled to be true")
	}
	// Should have ingress, exit, forward_ingress, and forward_exit roles
	if len(agent.Roles) != 4 {
		t.Errorf("expected 4 roles, got %d: %v", len(agent.Roles), agent.Roles)
	}
	// Check port forward info
	if len(agent.ForwardListeners) != 1 || agent.ForwardListeners[0] != "web" {
		t.Errorf("ForwardListeners = %v, want [web]", agent.ForwardListeners)
	}
	if len(agent.ForwardEndpoints) != 1 || agent.ForwardEndpoints[0] != "api" {
		t.Errorf("ForwardEndpoints = %v, want [api]", agent.ForwardEndpoints)
	}
}

// ============================================================================
// Server Configuration Tests
// ============================================================================

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.Address != ":8080" {
		t.Errorf("Address = %q, want %q", cfg.Address, ":8080")
	}
	if cfg.ReadTimeout != 10*time.Second {
		t.Errorf("ReadTimeout = %v, want %v", cfg.ReadTimeout, 10*time.Second)
	}
	if cfg.WriteTimeout != 10*time.Second {
		t.Errorf("WriteTimeout = %v, want %v", cfg.WriteTimeout, 10*time.Second)
	}
	if !cfg.EnablePprof {
		t.Error("expected EnablePprof to be true by default")
	}
	if !cfg.EnableDashboard {
		t.Error("expected EnableDashboard to be true by default")
	}
	if !cfg.EnableRemoteAPI {
		t.Error("expected EnableRemoteAPI to be true by default")
	}
}

func TestServer_Handler(t *testing.T) {
	cfg := DefaultServerConfig()
	s := NewServer(cfg, nil)

	handler := s.Handler()
	if handler == nil {
		t.Fatal("Handler() returned nil")
	}
}

func TestServer_Address_NotStarted(t *testing.T) {
	cfg := DefaultServerConfig()
	s := NewServer(cfg, nil)

	addr := s.Address()
	if addr != nil {
		t.Errorf("expected nil address before start, got %v", addr)
	}
}

// ============================================================================
// UI Redirect Test
// ============================================================================

func TestServer_UIRedirect(t *testing.T) {
	cfg := DefaultServerConfig()
	cfg.EnableDashboard = true
	s := NewServer(cfg, nil)

	req := httptest.NewRequest(http.MethodGet, "/ui", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusMovedPermanently {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusMovedPermanently)
	}

	location := rec.Header().Get("Location")
	if location != "/ui/" {
		t.Errorf("Location = %q, want %q", location, "/ui/")
	}
}

// ============================================================================
// ShellStreamAdapter Tests
// ============================================================================

func TestNewShellStreamAdapter(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	closeCalled := false
	closeFunc := func() { closeCalled = true }

	adapter := NewShellStreamAdapter(123, targetID, closeFunc)

	if adapter == nil {
		t.Fatal("NewShellStreamAdapter returned nil")
	}
	if adapter.streamID != 123 {
		t.Errorf("streamID = %d, want 123", adapter.streamID)
	}
	if adapter.targetID != targetID {
		t.Error("targetID mismatch")
	}
	if adapter.send == nil {
		t.Error("send channel should not be nil")
	}
	if adapter.receive == nil {
		t.Error("receive channel should not be nil")
	}
	if adapter.done == nil {
		t.Error("done channel should not be nil")
	}

	// Test that close calls closeFunc
	adapter.Close()
	if !closeCalled {
		t.Error("closeFunc should have been called")
	}
}

func TestShellStreamAdapter_ToSession(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(456, targetID, nil)

	session := adapter.ToSession()

	if session == nil {
		t.Fatal("ToSession returned nil")
	}
	if session.StreamID != 456 {
		t.Errorf("StreamID = %d, want 456", session.StreamID)
	}
	if session.TargetID != targetID {
		t.Error("TargetID mismatch")
	}
	if session.Send == nil {
		t.Error("Send channel should not be nil")
	}
	if session.Receive == nil {
		t.Error("Receive channel should not be nil")
	}
	if session.Done == nil {
		t.Error("Done channel should not be nil")
	}
	if session.Close == nil {
		t.Error("Close func should not be nil")
	}
}

func TestShellStreamAdapter_PushReceive(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	// Push data
	testData := []byte("test data")
	adapter.PushReceive(testData)

	// Receive data
	select {
	case received := <-adapter.receive:
		if string(received) != "test data" {
			t.Errorf("received = %q, want %q", received, "test data")
		}
	default:
		t.Error("expected data in receive channel")
	}
}

func TestShellStreamAdapter_PushReceive_Closed(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	adapter.Close()

	// Should not panic when pushing to closed adapter
	adapter.PushReceive([]byte("data"))
}

func TestShellStreamAdapter_PopSend(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	// Send data
	go func() {
		adapter.send <- []byte("sent data")
	}()

	// Pop data
	data, ok := adapter.PopSend()
	if !ok {
		t.Error("expected ok to be true")
	}
	if string(data) != "sent data" {
		t.Errorf("data = %q, want %q", data, "sent data")
	}
}

func TestShellStreamAdapter_PopSend_Closed(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	adapter.Close()

	data, ok := adapter.PopSend()
	if ok {
		t.Error("expected ok to be false after close")
	}
	if data != nil {
		t.Errorf("expected nil data, got %v", data)
	}
}

func TestShellStreamAdapter_SetExitCode(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	adapter.SetExitCode(42)

	if adapter.exitCode != 42 {
		t.Errorf("exitCode = %d, want 42", adapter.exitCode)
	}
}

func TestShellStreamAdapter_SetError(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	testErr := io.EOF
	adapter.SetError(testErr)

	if adapter.err != testErr {
		t.Errorf("err = %v, want %v", adapter.err, testErr)
	}
}

func TestShellStreamAdapter_Close_Idempotent(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	closeCount := 0
	closeFunc := func() { closeCount++ }

	adapter := NewShellStreamAdapter(1, targetID, closeFunc)

	// Close multiple times
	adapter.Close()
	adapter.Close()
	adapter.Close()

	// closeFunc should only be called once
	if closeCount != 1 {
		t.Errorf("closeFunc called %d times, want 1", closeCount)
	}
}

func TestShellStreamAdapter_Read(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	// Push data to receive channel
	go func() {
		adapter.receive <- []byte("hello world")
	}()

	buf := make([]byte, 1024)
	n, err := adapter.Read(buf)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 11 {
		t.Errorf("n = %d, want 11", n)
	}
	if string(buf[:n]) != "hello world" {
		t.Errorf("data = %q, want %q", buf[:n], "hello world")
	}
}

func TestShellStreamAdapter_Read_Closed(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	adapter.Close()

	buf := make([]byte, 1024)
	_, err := adapter.Read(buf)

	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestShellStreamAdapter_Read_ChannelClosed(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	close(adapter.receive)

	buf := make([]byte, 1024)
	_, err := adapter.Read(buf)

	if err != io.EOF {
		t.Errorf("expected io.EOF, got %v", err)
	}
}

func TestShellStreamAdapter_Write(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	go func() {
		data := <-adapter.send
		if string(data) != "write test" {
			t.Errorf("data = %q, want %q", data, "write test")
		}
	}()

	n, err := adapter.Write([]byte("write test"))

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if n != 10 {
		t.Errorf("n = %d, want 10", n)
	}
}

func TestShellStreamAdapter_Write_Closed(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	// Fill the buffer first (buffer size is 64)
	for i := 0; i < 64; i++ {
		adapter.send <- []byte("fill")
	}

	adapter.Close()

	// Now write should fail because buffer is full and done is closed
	_, err := adapter.Write([]byte("data"))

	if err != io.ErrClosedPipe {
		t.Errorf("expected io.ErrClosedPipe, got %v", err)
	}
}

func TestShellStreamAdapter_GetStreamID(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(789, targetID, nil)

	if adapter.GetStreamID() != 789 {
		t.Errorf("GetStreamID() = %d, want 789", adapter.GetStreamID())
	}
}

func TestShellStreamAdapter_SetNextHop(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	nextHopID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	// Create a mock peer sender (nil is acceptable for this test)
	adapter.SetNextHop(nextHopID, nil)

	if adapter.nextHop != nextHopID {
		t.Error("nextHop not set correctly")
	}
}

func TestShellStreamAdapter_SessionKey(t *testing.T) {
	targetID, _ := identity.NewAgentID()
	adapter := NewShellStreamAdapter(1, targetID, nil)

	// Initially nil
	if adapter.GetSessionKey() != nil {
		t.Error("expected nil session key initially")
	}

	// Set and get (using nil as test value since we can't easily create a real SessionKey)
	adapter.SetSessionKey(nil)
	if adapter.GetSessionKey() != nil {
		t.Error("expected nil after setting nil")
	}
}

// ============================================================================
// Domain Routes in Dashboard Tests
// ============================================================================

func TestServer_handleDashboard_WithDomainRoutes(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{
		running: true,
		stats: Stats{
			PeerCount:   1,
			StreamCount: 2,
			RouteCount:  2,
		},
	}
	s := NewServer(cfg, provider)

	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()

	remoteProvider := &mockRemoteStatusProvider{
		id:          localID,
		displayName: "local-agent",
		peerDetails: []PeerDetails{
			{
				ID:          peerID,
				DisplayName: "peer-agent",
				State:       "connected",
				RTT:         20 * time.Millisecond,
			},
		},
		routeDetails: []RouteDetails{
			{
				Network: "10.0.0.0/8",
				NextHop: peerID,
				Origin:  peerID,
				Metric:  1,
				Path:    []identity.AgentID{peerID},
			},
		},
		domainRoutesList: []DomainRouteDetails{
			{
				Pattern:    "*.example.com",
				IsWildcard: true,
				NextHop:    peerID,
				Origin:     peerID,
				Metric:     1,
				Path:       []identity.AgentID{peerID},
			},
		},
		displayNames: map[identity.AgentID]string{
			peerID: "peer-agent",
		},
		allNodeInfo: map[identity.AgentID]*protocol.NodeInfo{
			peerID: {
				UDPEnabled: true,
			},
		},
	}
	s.SetRemoteProvider(remoteProvider)

	req := httptest.NewRequest(http.MethodGet, "/api/dashboard", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response DashboardResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Should have both CIDR and domain routes
	if len(response.Routes) != 2 {
		t.Errorf("expected 2 routes (1 CIDR + 1 domain), got %d", len(response.Routes))
	}

	// Should have domain routes in legacy field
	if len(response.DomainRoutes) != 1 {
		t.Errorf("expected 1 domain route, got %d", len(response.DomainRoutes))
	}

	// Verify domain route details
	if response.DomainRoutes[0].Pattern != "*.example.com" {
		t.Errorf("Pattern = %q, want %q", response.DomainRoutes[0].Pattern, "*.example.com")
	}
	if !response.DomainRoutes[0].IsWildcard {
		t.Error("expected IsWildcard to be true")
	}
	if !response.DomainRoutes[0].UDP {
		t.Error("expected UDP to be true (exit has UDP enabled)")
	}
}

// ============================================================================
// Topology with Unresponsive Peer Tests
// ============================================================================

func TestServer_handleTopology_UnresponsivePeer(t *testing.T) {
	cfg := DefaultServerConfig()
	provider := &mockStatsProvider{running: true}
	s := NewServer(cfg, provider)

	localID, _ := identity.NewAgentID()
	peerID, _ := identity.NewAgentID()

	remoteProvider := &mockRemoteStatusProvider{
		id:          localID,
		displayName: "local-agent",
		peerIDs:     []identity.AgentID{peerID},
		peerDetails: []PeerDetails{
			{
				ID:          peerID,
				DisplayName: "slow-peer",
				State:       "connected",
				RTT:         120 * time.Second, // Very slow - should be marked unresponsive
				Transport:   "quic",
			},
		},
		displayNames:  map[identity.AgentID]string{},
		allNodeInfo:   map[identity.AgentID]*protocol.NodeInfo{},
		localNodeInfo: &protocol.NodeInfo{},
	}
	s.SetRemoteProvider(remoteProvider)

	req := httptest.NewRequest(http.MethodGet, "/api/topology", nil)
	rec := httptest.NewRecorder()

	s.server.Handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var response TopologyResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Connection should be marked as unresponsive
	if len(response.Connections) < 1 {
		t.Fatal("expected at least 1 connection")
	}

	found := false
	for _, conn := range response.Connections {
		if conn.ToAgent == peerID.ShortString() {
			found = true
			if !conn.Unresponsive {
				t.Error("expected connection to be marked as unresponsive")
			}
		}
	}
	if !found {
		t.Error("connection to peer not found")
	}
}

// ============================================================================
// SetShellProvider Test
// ============================================================================

func TestServer_SetShellProvider(t *testing.T) {
	cfg := DefaultServerConfig()
	s := NewServer(cfg, nil)

	// Initially nil
	if s.shellProvider != nil {
		t.Error("expected nil shell provider initially")
	}

	// Set provider (using nil as we don't have a real implementation for unit tests)
	s.SetShellProvider(nil)

	// This test just verifies the method exists and doesn't panic
}

// ============================================================================
// CanDecryptManagement Tests
// ============================================================================

func TestServer_CanDecryptManagement(t *testing.T) {
	cfg := DefaultServerConfig()
	s := NewServer(cfg, nil)

	// Without sealed box
	if s.CanDecryptManagement() {
		t.Error("expected CanDecryptManagement to return false without sealed box")
	}
}

// ============================================================================
// ShouldRestrictTopology Tests
// ============================================================================

func TestServer_shouldRestrictTopology(t *testing.T) {
	cfg := DefaultServerConfig()
	s := NewServer(cfg, nil)

	// Without sealed box - no restriction
	if s.shouldRestrictTopology() {
		t.Error("expected no restriction without sealed box")
	}
}
