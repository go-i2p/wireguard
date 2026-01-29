package version

import (
	"strings"
	"testing"
)

func TestVersion_Default(t *testing.T) {
	// Default version should be "dev"
	if Version != "dev" {
		// Version may be set by ldflags in CI, so just check it's not empty
		if Version == "" {
			t.Error("Version should not be empty")
		}
	}
}

func TestFull_DefaultVersion(t *testing.T) {
	// Save original values
	origVersion := Version
	origCommit := GitCommit
	origBuildTime := BuildTime
	defer func() {
		Version = origVersion
		GitCommit = origCommit
		BuildTime = origBuildTime
	}()

	// Test with just version
	Version = "1.0.0"
	GitCommit = ""
	BuildTime = ""

	result := Full()
	if result != "1.0.0" {
		t.Errorf("Full() = %q, want %q", result, "1.0.0")
	}
}

func TestFull_WithCommit(t *testing.T) {
	origVersion := Version
	origCommit := GitCommit
	origBuildTime := BuildTime
	defer func() {
		Version = origVersion
		GitCommit = origCommit
		BuildTime = origBuildTime
	}()

	Version = "1.0.0"
	GitCommit = "abc1234"
	BuildTime = ""

	result := Full()
	if result != "1.0.0-abc1234" {
		t.Errorf("Full() = %q, want %q", result, "1.0.0-abc1234")
	}
}

func TestFull_WithBuildTime(t *testing.T) {
	origVersion := Version
	origCommit := GitCommit
	origBuildTime := BuildTime
	defer func() {
		Version = origVersion
		GitCommit = origCommit
		BuildTime = origBuildTime
	}()

	Version = "1.0.0"
	GitCommit = ""
	BuildTime = "2026-01-29T12:00:00Z"

	result := Full()
	if result != "1.0.0 (2026-01-29T12:00:00Z)" {
		t.Errorf("Full() = %q, want %q", result, "1.0.0 (2026-01-29T12:00:00Z)")
	}
}

func TestFull_Complete(t *testing.T) {
	origVersion := Version
	origCommit := GitCommit
	origBuildTime := BuildTime
	defer func() {
		Version = origVersion
		GitCommit = origCommit
		BuildTime = origBuildTime
	}()

	Version = "1.0.0"
	GitCommit = "abc1234"
	BuildTime = "2026-01-29T12:00:00Z"

	result := Full()
	expected := "1.0.0-abc1234 (2026-01-29T12:00:00Z)"
	if result != expected {
		t.Errorf("Full() = %q, want %q", result, expected)
	}

	// Verify all parts are present
	if !strings.Contains(result, Version) {
		t.Error("Full() should contain Version")
	}
	if !strings.Contains(result, GitCommit) {
		t.Error("Full() should contain GitCommit")
	}
	if !strings.Contains(result, BuildTime) {
		t.Error("Full() should contain BuildTime")
	}
}
