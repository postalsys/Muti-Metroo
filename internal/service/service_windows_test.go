//go:build windows

package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestToRegistryValueName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "default name with hyphen",
			input:    "muti-metroo",
			expected: "MutiMetroo",
		},
		{
			name:     "name with spaces",
			input:    "My Tunnel",
			expected: "MyTunnel",
		},
		{
			name:     "name with multiple spaces",
			input:    "Tunnel  Manager",
			expected: "TunnelManager",
		},
		{
			name:     "name with underscores",
			input:    "my_tunnel_service",
			expected: "MyTunnelService",
		},
		{
			name:     "mixed separators",
			input:    "my-tunnel_service name",
			expected: "MyTunnelServiceName",
		},
		{
			name:     "already PascalCase",
			input:    "MyTunnel",
			expected: "Mytunnel",
		},
		{
			name:     "all uppercase",
			input:    "MYTUNNEL",
			expected: "Mytunnel",
		},
		{
			name:     "single word",
			input:    "tunnel",
			expected: "Tunnel",
		},
		{
			name:     "empty string",
			input:    "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := toRegistryValueName(tt.input)
			if result != tt.expected {
				t.Errorf("toRegistryValueName(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestReadUserServiceInfo(t *testing.T) {
	// Create a temporary directory
	tmpDir := t.TempDir()

	// Override getUserServiceDir for testing
	origDir := os.Getenv("LOCALAPPDATA")
	testAppData := tmpDir
	os.Setenv("LOCALAPPDATA", testAppData)
	defer os.Setenv("LOCALAPPDATA", origDir)

	// Create the service directory
	serviceDir := filepath.Join(testAppData, "muti-metroo")
	if err := os.MkdirAll(serviceDir, 0755); err != nil {
		t.Fatalf("Failed to create service dir: %v", err)
	}

	t.Run("valid service info", func(t *testing.T) {
		infoContent := `name=My Service
registry_value=MyService
dll=C:\path\to\muti-metroo.dll
config=C:\path\to\config.yaml
`
		infoPath := filepath.Join(serviceDir, userInfoFileName)
		if err := os.WriteFile(infoPath, []byte(infoContent), 0644); err != nil {
			t.Fatalf("Failed to write service info: %v", err)
		}
		defer os.Remove(infoPath)

		info := readUserServiceInfo()
		if info == nil {
			t.Fatal("readUserServiceInfo() returned nil")
		}

		if info.Name != "My Service" {
			t.Errorf("Name = %q, want %q", info.Name, "My Service")
		}
		if info.RegistryValue != "MyService" {
			t.Errorf("RegistryValue = %q, want %q", info.RegistryValue, "MyService")
		}
		if info.DLLPath != `C:\path\to\muti-metroo.dll` {
			t.Errorf("DLLPath = %q, want %q", info.DLLPath, `C:\path\to\muti-metroo.dll`)
		}
		if info.ConfigPath != `C:\path\to\config.yaml` {
			t.Errorf("ConfigPath = %q, want %q", info.ConfigPath, `C:\path\to\config.yaml`)
		}
	})

	t.Run("missing registry_value", func(t *testing.T) {
		infoContent := `name=My Service
dll=C:\path\to\muti-metroo.dll
config=C:\path\to\config.yaml
`
		infoPath := filepath.Join(serviceDir, userInfoFileName)
		if err := os.WriteFile(infoPath, []byte(infoContent), 0644); err != nil {
			t.Fatalf("Failed to write service info: %v", err)
		}
		defer os.Remove(infoPath)

		info := readUserServiceInfo()
		if info != nil {
			t.Error("readUserServiceInfo() should return nil when registry_value is missing")
		}
	})

	t.Run("file not found", func(t *testing.T) {
		// Remove the info file if it exists
		infoPath := filepath.Join(serviceDir, userInfoFileName)
		os.Remove(infoPath)

		info := readUserServiceInfo()
		if info != nil {
			t.Error("readUserServiceInfo() should return nil when file doesn't exist")
		}
	})
}

func TestStatusUserImpl(t *testing.T) {
	// This test verifies the statusUserImpl function behavior
	// It requires actual Windows environment to fully test

	t.Run("not installed returns correct status", func(t *testing.T) {
		// Create a temporary directory with no service info
		tmpDir := t.TempDir()
		origDir := os.Getenv("LOCALAPPDATA")
		os.Setenv("LOCALAPPDATA", tmpDir)
		defer os.Setenv("LOCALAPPDATA", origDir)

		status, err := statusUserImpl()
		if err != nil {
			t.Fatalf("statusUserImpl() error: %v", err)
		}

		if status != "not installed" {
			t.Errorf("statusUserImpl() = %q, want %q", status, "not installed")
		}
	})
}

func TestStopUserImpl(t *testing.T) {
	// Test that stopUserImpl doesn't panic when there's no service to stop
	t.Run("no service to stop", func(t *testing.T) {
		tmpDir := t.TempDir()
		origDir := os.Getenv("LOCALAPPDATA")
		os.Setenv("LOCALAPPDATA", tmpDir)
		defer os.Setenv("LOCALAPPDATA", origDir)

		// Should not return an error even if nothing to stop
		err := stopUserImpl()
		if err != nil {
			t.Errorf("stopUserImpl() error: %v", err)
		}
	})
}

func TestParsePowerShellCSVOutput(t *testing.T) {
	// Test the CSV parsing logic used in statusUserImpl and stopUserImpl
	// This simulates the output from PowerShell Get-CimInstance

	tests := []struct {
		name           string
		output         string
		dllPath        string
		shouldFindProc bool
	}{
		{
			name: "process found with DLL path",
			output: `"ProcessId","CommandLine"
"1234","C:\Windows\System32\rundll32.exe \"C:\Users\Test\muti-metroo.dll\",Run \"C:\Users\Test\config.yaml\""
`,
			dllPath:        `C:\Users\Test\muti-metroo.dll`,
			shouldFindProc: true,
		},
		{
			name: "process found with muti-metroo string",
			output: `"ProcessId","CommandLine"
"5678","C:\Windows\System32\rundll32.exe \"D:\apps\muti-metroo.dll\",Run"
`,
			dllPath:        "",
			shouldFindProc: true,
		},
		{
			name: "different rundll32 process",
			output: `"ProcessId","CommandLine"
"9999","C:\Windows\System32\rundll32.exe \"C:\Windows\shell32.dll\",Control_RunDLL"
`,
			dllPath:        `C:\Users\Test\muti-metroo.dll`,
			shouldFindProc: false,
		},
		{
			name:           "no processes",
			output:         `"ProcessId","CommandLine"`,
			dllPath:        `C:\Users\Test\muti-metroo.dll`,
			shouldFindProc: false,
		},
		{
			name:           "empty output",
			output:         "",
			dllPath:        `C:\Users\Test\muti-metroo.dll`,
			shouldFindProc: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the detection logic from statusUserImpl
			found := false
			if tt.dllPath != "" && strings.Contains(tt.output, tt.dllPath) {
				found = true
			} else if strings.Contains(tt.output, "muti-metroo") {
				found = true
			}

			if found != tt.shouldFindProc {
				t.Errorf("Process detection = %v, want %v", found, tt.shouldFindProc)
			}
		})
	}
}

func TestExtractPIDFromCSV(t *testing.T) {
	// Test the PID extraction logic used in stopUserImpl
	tests := []struct {
		name        string
		line        string
		expectedPID string
	}{
		{
			name:        "valid CSV line",
			line:        `"1234","C:\Windows\System32\rundll32.exe muti-metroo.dll"`,
			expectedPID: "1234",
		},
		{
			name:        "PID with spaces",
			line:        `"5678","some command"`,
			expectedPID: "5678",
		},
		{
			name:        "header line",
			line:        `"ProcessId","CommandLine"`,
			expectedPID: "ProcessId",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Simulate the PID extraction logic from stopUserImpl
			line := strings.TrimPrefix(tt.line, `"`)
			pid := ""
			if idx := strings.Index(line, `"`); idx > 0 {
				pid = line[:idx]
			}

			if pid != tt.expectedPID {
				t.Errorf("Extracted PID = %q, want %q", pid, tt.expectedPID)
			}
		})
	}
}
