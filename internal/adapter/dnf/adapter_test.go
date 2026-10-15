package dnf

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/adapter"
)

func TestDnf_HealthyPath(t *testing.T) {
	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "dnf" && len(args) == 1 && args[0] == "--version":
			return []byte("dnf 5\n"), nil
		case name == "dnf" && len(args) > 0 && args[0] == "repoquery":
			return packageLines("user", 200), nil
		case name == "rpm" && len(args) == 1 && args[0] == "--version":
			return []byte("rpm\n"), nil
		case name == "rpm" && len(args) == 1 && args[0] == "-qa":
			return packageLines("installed", 300), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}
	a := NewWithHistoryPath(runner, filepath.Join(t.TempDir(), "history.sqlite"))

	result, err := a.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if result.Level != adapter.Level1 {
		t.Errorf("Level = %d, want %d", result.Level, adapter.Level1)
	}
	if result.Reliability != adapter.ReliabilityHigh {
		t.Errorf("Reliability = %q, want high", result.Reliability)
	}
	if len(result.Packages) != 200 {
		t.Errorf("got %d packages, want 200", len(result.Packages))
	}
	if len(result.Warnings) != 0 {
		t.Errorf("Warnings = %v, want none", result.Warnings)
	}
}

func TestDnf_FallbackOnEmpty(t *testing.T) {
	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "dnf" && len(args) == 1 && args[0] == "--version":
			return []byte("dnf 5\n"), nil
		case name == "dnf" && len(args) > 0 && args[0] == "repoquery":
			return nil, nil
		case name == "rpm" && len(args) == 1 && args[0] == "-qa":
			return packageLines("installed", 3455), nil
		case name == "rpm" && len(args) == 3 && args[0] == "-qa" && args[1] == "--qf" && args[2] == "%{name}\t%{version}\n":
			return []byte("base\t1\napp-a\t2\napp-b\t3\n"), nil
		case name == "rpm" && len(args) == 3 && args[0] == "-qa" && args[1] == "--qf" && args[2] == "[%{REQUIRENAME}\n]":
			return []byte("base\n"), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}
	a := NewWithHistoryPath(runner, filepath.Join(t.TempDir(), "history.sqlite"))

	result, err := a.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if result.Level != adapter.Level2 {
		t.Errorf("Level = %d, want %d", result.Level, adapter.Level2)
	}
	if result.Reliability != adapter.ReliabilityMedium {
		t.Errorf("Reliability = %q, want medium", result.Reliability)
	}
	if len(result.Packages) != 2 || result.Packages[0].Name != "app-a" || result.Packages[1].Name != "app-b" {
		t.Errorf("Packages = %#v, want top-level app-a and app-b", result.Packages)
	}
	if len(result.Warnings) == 0 || !strings.Contains(strings.ToLower(result.Warnings[0]), "incohérent") {
		t.Errorf("Warnings = %v, want an inconsistency warning", result.Warnings)
	}
}

func TestDnf_AllLevelsFail(t *testing.T) {
	historyPath := filepath.Join(t.TempDir(), "missing-history.sqlite")
	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "dnf" && len(args) == 1 && args[0] == "--version":
			return []byte("dnf 5\n"), nil
		case name == "dnf" && len(args) > 0 && args[0] == "repoquery":
			return nil, fmt.Errorf("repoquery failed")
		case name == "rpm" && len(args) == 1 && args[0] == "-qa":
			return packageLines("installed", 3455), nil
		case name == "rpm":
			return nil, fmt.Errorf("rpm query failed")
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}
	a := NewWithHistoryPath(runner, historyPath)

	_, err := a.Capture()
	if err == nil {
		t.Fatal("Capture() error = nil, want all-levels failure")
	}
	if !strings.Contains(err.Error(), historyPath) {
		t.Errorf("error %q does not include tested history path %q", err, historyPath)
	}
}

func TestDnf_HealthDetectsInconsistency(t *testing.T) {
	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "dnf" && len(args) == 1 && args[0] == "--version":
			return []byte("dnf 5\n"), nil
		case name == "dnf" && len(args) > 0 && args[0] == "repoquery":
			return nil, nil
		case name == "rpm" && len(args) == 1 && args[0] == "--version":
			return []byte("rpm\n"), nil
		case name == "rpm" && len(args) == 1 && args[0] == "-qa":
			return packageLines("installed", 3455), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}
	a := NewWithHistoryPath(runner, filepath.Join(t.TempDir(), "history.sqlite"))

	status, err := a.Health()
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if status.Functional {
		t.Error("Functional = true, want false")
	}
	if !strings.Contains(status.Message, "⚠ INCOHÉRENCE DÉTECTÉE — repli recommandé") {
		t.Errorf("Message = %q, want inconsistency notice", status.Message)
	}
}

func packageLines(prefix string, count int) []byte {
	var builder strings.Builder
	for i := 0; i < count; i++ {
		fmt.Fprintf(&builder, "%s-%d\t1.0\n", prefix, i)
	}
	return []byte(builder.String())
}

func TestDnf_HistoryReadsFileWithoutChangingIt(t *testing.T) {
	historyPath := filepath.Join(t.TempDir(), "history.sqlite")
	if err := os.WriteFile(historyPath, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("unexpected command: %s %v", name, args)
	}
	a := NewWithHistoryPath(runner, historyPath)
	if _, err := a.captureHistory(); err == nil || !strings.Contains(err.Error(), historyPath) {
		t.Errorf("captureHistory() error = %v, want read-only sqlite error including path", err)
	}
	data, err := os.ReadFile(historyPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "test" {
		t.Fatal("history file changed")
	}
}
