package sysinfo

import (
	"strings"
	"testing"
)

func TestVersion(t *testing.T) {
	// Version should either be a release version (set via ldflags)
	// or an enhanced dev version (dev-<commit> or dev-<timestamp>)
	t.Logf("Version: %s", Version)

	if Version == "dev" {
		t.Error("Version should not be plain 'dev' - enhanceDevVersion should have been called")
	}

	// Check valid version formats
	validFormats := []string{
		"dev-",   // Enhanced dev version (dev-abc1234, dev-abc1234-dirty, dev-20060102-150405)
		"v",      // Release version (v1.0.0)
		"latest", // Latest tag
	}

	hasValidFormat := false
	for _, prefix := range validFormats {
		if strings.HasPrefix(Version, prefix) {
			hasValidFormat = true
			break
		}
	}

	if !hasValidFormat {
		t.Errorf("Version %q has unexpected format", Version)
	}
}

func TestEnhanceDevVersion(t *testing.T) {
	version := enhanceDevVersion()
	t.Logf("Enhanced dev version: %s", version)

	if !strings.HasPrefix(version, "dev-") {
		t.Errorf("Enhanced version %q should start with 'dev-'", version)
	}

	// Should have something after "dev-"
	suffix := strings.TrimPrefix(version, "dev-")
	if suffix == "" {
		t.Error("Enhanced version should have content after 'dev-'")
	}
}
