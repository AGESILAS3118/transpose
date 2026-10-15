package flatpak

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/adapter"
)

func TestFlatpak_HealthyPath(t *testing.T) {
	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "flatpak" && len(args) == 1 && args[0] == "--version":
			return []byte("Flatpak 1.15\n"), nil
		case name == "flatpak" && len(args) > 0 && args[0] == "list":
			return []byte("org.example.App\t1.2\tflathub\tsystem\n"), nil
		case name == "flatpak" && len(args) > 0 && args[0] == "remotes" && args[1] == "--system":
			return []byte("flathub\thttps://example.org/system\n"), nil
		case name == "flatpak" && len(args) > 0 && args[0] == "remotes" && args[1] == "--user":
			return []byte("flathub\thttps://example.org/user\n"), nil
		case name == "flatpak" && len(args) > 0 && args[0] == "override":
			return []byte(""), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}
	a := NewWithAppDirs(runner, filepath.Join(t.TempDir(), "system"), filepath.Join(t.TempDir(), "user"))

	result, err := a.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if result.Level != adapter.Level1 || result.Reliability != adapter.ReliabilityHigh {
		t.Errorf("got level %d reliability %q, want level 1/high", result.Level, result.Reliability)
	}
	if len(result.Packages) != 1 {
		t.Fatalf("got %d packages, want 1", len(result.Packages))
	}
	if _, ok := result.Raw["remotes"]; !ok {
		t.Error("Raw is missing remotes")
	}
	if _, ok := result.Raw["overrides"]; !ok {
		t.Error("Raw is missing overrides")
	}
}

func TestFlatpak_FallbackToDirectoryScan(t *testing.T) {
	root := t.TempDir()
	systemDir := filepath.Join(root, "system")
	userDir := filepath.Join(root, "user")
	refDir := filepath.Join(systemDir, "org.example.App", "x86_64", "stable")
	if err := os.MkdirAll(refDir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(userDir, "org.example.Other", "aarch64", "beta"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := func(name string, args ...string) ([]byte, error) {
		if name == "flatpak" && len(args) > 0 && args[0] == "list" {
			return nil, fmt.Errorf("flatpak CLI failed")
		}
		if name == "flatpak" && len(args) == 1 && args[0] == "--version" {
			return []byte("Flatpak\n"), nil
		}
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	}
	a := NewWithAppDirs(runner, systemDir, userDir)

	result, err := a.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if result.Level != adapter.Level2 {
		t.Errorf("Level = %d, want %d", result.Level, adapter.Level2)
	}
	if len(result.Packages) != 2 {
		t.Errorf("got %d packages, want 2", len(result.Packages))
	}
}

func TestFlatpak_DivergenceWarning(t *testing.T) {
	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "flatpak" && len(args) == 1 && args[0] == "--version":
			return []byte("Flatpak\n"), nil
		case name == "flatpak" && len(args) > 0 && args[0] == "list":
			return []byte("org.example.App\t1.2\tflathub\tsystem\n"), nil
		case name == "flatpak" && len(args) > 0 && args[0] == "remotes" && args[1] == "--system":
			return []byte("flathub\thttps://example.org\n"), nil
		case name == "flatpak" && len(args) > 0 && args[0] == "remotes" && args[1] == "--user":
			return nil, nil
		case name == "flatpak" && len(args) > 0 && args[0] == "override":
			return nil, nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}
	a := NewWithAppDirs(runner, filepath.Join(t.TempDir(), "system"), filepath.Join(t.TempDir(), "user"))

	result, err := a.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if len(result.Warnings) != 1 || !strings.Contains(result.Warnings[0], "présent en système mais absent en utilisateur") {
		t.Errorf("Warnings = %v, want system/user remote divergence warning", result.Warnings)
	}
}
