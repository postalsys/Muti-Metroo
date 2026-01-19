package sleep

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/postalsys/muti-metroo/internal/config"
	"github.com/postalsys/muti-metroo/internal/identity"
	"github.com/postalsys/muti-metroo/internal/logging"
	"github.com/postalsys/muti-metroo/internal/protocol"
)

func TestState_String(t *testing.T) {
	tests := []struct {
		state State
		want  string
	}{
		{StateAwake, "AWAKE"},
		{StateSleeping, "SLEEPING"},
		{StatePolling, "POLLING"},
		{State(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		got := tt.state.String()
		if got != tt.want {
			t.Errorf("State(%d).String() = %q, want %q", tt.state, got, tt.want)
		}
	}
}

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:            true,
		PollInterval:       5 * time.Minute,
		PollIntervalJitter: 0.3,
		PollDuration:       30 * time.Second,
		PersistState:       false,
		MaxQueuedMessages:  1000,
	}
	logger := logging.NewLogger("error", "text")

	mgr := NewManager(cfg, tmpDir, logger)
	if mgr == nil {
		t.Fatal("NewManager returned nil")
	}

	if mgr.GetState() != StateAwake {
		t.Errorf("Initial state = %v, want AWAKE", mgr.GetState())
	}

	mgr.Stop()
}

func TestManager_SleepWakeCycle(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:            true,
		PollInterval:       100 * time.Millisecond,
		PollIntervalJitter: 0.1,
		PollDuration:       50 * time.Millisecond,
		PersistState:       false,
		MaxQueuedMessages:  100,
	}
	logger := logging.NewLogger("error", "text")

	mgr := NewManager(cfg, tmpDir, logger)
	defer mgr.Stop()

	var sleepCalled, wakeCalled bool
	mgr.SetCallbacks(Callbacks{
		OnSleep: func() error {
			sleepCalled = true
			return nil
		},
		OnWake: func() error {
			wakeCalled = true
			return nil
		},
	})

	// Test sleep
	if err := mgr.Sleep(); err != nil {
		t.Fatalf("Sleep() error = %v", err)
	}
	if !sleepCalled {
		t.Error("OnSleep callback not called")
	}
	if mgr.GetState() != StateSleeping {
		t.Errorf("State after Sleep() = %v, want SLEEPING", mgr.GetState())
	}

	// Test wake
	if err := mgr.Wake(); err != nil {
		t.Fatalf("Wake() error = %v", err)
	}
	if !wakeCalled {
		t.Error("OnWake callback not called")
	}
	if mgr.GetState() != StateAwake {
		t.Errorf("State after Wake() = %v, want AWAKE", mgr.GetState())
	}
}

func TestManager_SleepErrors(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:      true,
		PollInterval: 1 * time.Minute,
		PollDuration: 30 * time.Second,
	}
	logger := logging.NewLogger("error", "text")

	mgr := NewManager(cfg, tmpDir, logger)
	defer mgr.Stop()

	mgr.SetCallbacks(Callbacks{
		OnSleep: func() error { return nil },
		OnWake:  func() error { return nil },
	})

	// First sleep should succeed
	if err := mgr.Sleep(); err != nil {
		t.Fatalf("First Sleep() error = %v", err)
	}

	// Second sleep should fail (already sleeping)
	if err := mgr.Sleep(); err != ErrAlreadySleeping {
		t.Errorf("Second Sleep() error = %v, want ErrAlreadySleeping", err)
	}
}

func TestManager_WakeErrors(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:      true,
		PollInterval: 1 * time.Minute,
		PollDuration: 30 * time.Second,
	}
	logger := logging.NewLogger("error", "text")

	mgr := NewManager(cfg, tmpDir, logger)
	defer mgr.Stop()

	mgr.SetCallbacks(Callbacks{
		OnSleep: func() error { return nil },
		OnWake:  func() error { return nil },
	})

	// Wake while awake should fail
	if err := mgr.Wake(); err != ErrNotSleeping {
		t.Errorf("Wake() while awake error = %v, want ErrNotSleeping", err)
	}
}

func TestManager_HandleSleepCommand(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:      true,
		PollInterval: 1 * time.Minute,
		PollDuration: 30 * time.Second,
	}
	logger := logging.NewLogger("error", "text")

	mgr := NewManager(cfg, tmpDir, logger)
	defer mgr.Stop()

	originID, err := identity.NewAgentID()
	if err != nil {
		t.Fatalf("NewAgentID() error = %v", err)
	}
	cmd := &protocol.SleepCommand{
		OriginAgent: originID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		SeenBy:      []identity.AgentID{originID},
	}

	// First time should return true
	if !mgr.HandleSleepCommand(cmd) {
		t.Error("First HandleSleepCommand() = false, want true")
	}

	// Same command again should return false (deduplicated)
	if mgr.HandleSleepCommand(cmd) {
		t.Error("Second HandleSleepCommand() = true, want false (deduplicated)")
	}

	// Different command ID should return true
	cmd2 := &protocol.SleepCommand{
		OriginAgent: originID,
		CommandID:   2,
		Timestamp:   uint64(time.Now().Unix()),
		SeenBy:      []identity.AgentID{originID},
	}
	if !mgr.HandleSleepCommand(cmd2) {
		t.Error("HandleSleepCommand with different ID = false, want true")
	}
}

func TestManager_HandleWakeCommand(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:      true,
		PollInterval: 1 * time.Minute,
		PollDuration: 30 * time.Second,
	}
	logger := logging.NewLogger("error", "text")

	mgr := NewManager(cfg, tmpDir, logger)
	defer mgr.Stop()

	originID, err := identity.NewAgentID()
	if err != nil {
		t.Fatalf("NewAgentID() error = %v", err)
	}
	cmd := &protocol.WakeCommand{
		OriginAgent: originID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
		SeenBy:      []identity.AgentID{originID},
	}

	// First time should return true
	if !mgr.HandleWakeCommand(cmd) {
		t.Error("First HandleWakeCommand() = false, want true")
	}

	// Same command again should return false (deduplicated)
	if mgr.HandleWakeCommand(cmd) {
		t.Error("Second HandleWakeCommand() = true, want false (deduplicated)")
	}
}

func TestManager_GetStatus(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:            true,
		PollInterval:       5 * time.Minute,
		PollIntervalJitter: 0.3,
		PollDuration:       30 * time.Second,
	}
	logger := logging.NewLogger("error", "text")

	mgr := NewManager(cfg, tmpDir, logger)
	defer mgr.Stop()

	status := mgr.GetStatus()
	if status.State != StateAwake {
		t.Errorf("Status.State = %v, want AWAKE", status.State)
	}
	if !status.Enabled {
		t.Error("Status.Enabled = false, want true")
	}
}

func TestManager_Persistence(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:       true,
		PollInterval:  5 * time.Minute,
		PollDuration:  30 * time.Second,
		PersistState:  true,
		MaxQueuedMessages: 100,
	}
	logger := logging.NewLogger("error", "text")

	// Create first manager and put it to sleep
	mgr1 := NewManager(cfg, tmpDir, logger)
	mgr1.SetCallbacks(Callbacks{
		OnSleep: func() error { return nil },
		OnWake:  func() error { return nil },
	})

	if err := mgr1.Sleep(); err != nil {
		t.Fatalf("Sleep() error = %v", err)
	}
	mgr1.Stop()

	// Verify state file exists
	stateFile := filepath.Join(tmpDir, "sleep_state.json")
	if _, err := os.Stat(stateFile); os.IsNotExist(err) {
		t.Error("State file not created")
	}

	// Create second manager and verify it loads the sleeping state
	mgr2 := NewManager(cfg, tmpDir, logger)
	if err := mgr2.LoadState(); err != nil {
		t.Fatalf("LoadState() error = %v", err)
	}
	if mgr2.GetState() != StateSleeping {
		t.Errorf("Loaded state = %v, want SLEEPING", mgr2.GetState())
	}
	mgr2.Stop()
}

func TestManager_NotEnabled(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled: false,
	}
	logger := logging.NewLogger("error", "text")

	mgr := NewManager(cfg, tmpDir, logger)
	defer mgr.Stop()

	originID, err := identity.NewAgentID()
	if err != nil {
		t.Fatalf("NewAgentID() error = %v", err)
	}

	// Operations should fail when not enabled
	cmd := &protocol.SleepCommand{
		OriginAgent: originID,
		CommandID:   1,
		Timestamp:   uint64(time.Now().Unix()),
	}
	if mgr.HandleSleepCommand(cmd) {
		t.Error("HandleSleepCommand when disabled = true, want false")
	}
}

func TestJitteredInterval(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:            true,
		PollInterval:       5 * time.Minute,
		PollIntervalJitter: 0.3,
		PollDuration:       30 * time.Second,
	}
	logger := logging.NewLogger("error", "text")
	mgr := NewManager(cfg, tmpDir, logger)
	defer mgr.Stop()

	baseInterval := 5 * time.Minute
	jitter := 0.3 // 30%

	// Run multiple times to test randomness
	for i := 0; i < 100; i++ {
		result := mgr.jitteredInterval(baseInterval, jitter)
		minExpected := time.Duration(float64(baseInterval) * (1 - jitter))
		maxExpected := time.Duration(float64(baseInterval) * (1 + jitter))

		if result < minExpected || result > maxExpected {
			t.Errorf("Jittered interval %v out of range [%v, %v]", result, minExpected, maxExpected)
		}
	}
}

func TestIsSleeping(t *testing.T) {
	tmpDir := t.TempDir()
	cfg := config.SleepConfig{
		Enabled:      true,
		PollInterval: 1 * time.Minute,
		PollDuration: 30 * time.Second,
	}
	logger := logging.NewLogger("error", "text")

	mgr := NewManager(cfg, tmpDir, logger)
	defer mgr.Stop()

	mgr.SetCallbacks(Callbacks{
		OnSleep: func() error { return nil },
		OnWake:  func() error { return nil },
	})

	if mgr.IsSleeping() {
		t.Error("IsSleeping() = true when awake, want false")
	}

	if err := mgr.Sleep(); err != nil {
		t.Fatalf("Sleep() error = %v", err)
	}

	if !mgr.IsSleeping() {
		t.Error("IsSleeping() = false when sleeping, want true")
	}
}
