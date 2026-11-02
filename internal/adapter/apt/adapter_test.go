package apt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/adapter"
)

func packageLines(prefix string, count int) []byte {
	var b strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&b, "%s-%d\n", prefix, i)
	}
	return []byte(b.String())
}

func dpkgLines(prefix string, count int) []byte {
	var b strings.Builder
	for i := 1; i <= count; i++ {
		fmt.Fprintf(&b, "%s-%d\t1.0.%d\tii \n", prefix, i, i)
	}
	return []byte(b.String())
}

// TestApt_HealthyPath : niveau 1 renvoie 200 paquets, dpkg-query -W renvoie 1500 -> Level == 1, Reliability == "high"
func TestApt_HealthyPath(t *testing.T) {
	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "apt-mark" && len(args) == 1 && args[0] == "showmanual":
			return packageLines("pkg", 200), nil
		case name == "dpkg-query" && len(args) == 3 && args[0] == "-W":
			return dpkgLines("all", 1500), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}

	a := NewWithStatePath(runner, filepath.Join(t.TempDir(), "extended_states"))
	res, err := a.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if res.Level != adapter.Level1 {
		t.Errorf("Level = %d, want %d", res.Level, adapter.Level1)
	}
	if res.Reliability != adapter.ReliabilityHigh {
		t.Errorf("Reliability = %s, want %s", res.Reliability, adapter.ReliabilityHigh)
	}
	if len(res.Packages) != 200 {
		t.Errorf("len(Packages) = %d, want 200", len(res.Packages))
	}
	if len(res.Warnings) != 0 {
		t.Errorf("Warnings = %v, want empty", res.Warnings)
	}
}

// TestApt_FallbackOnInconsistent : niveau 1 renvoie 0, dpkg-query -W renvoie 1200 -> Level == 2, warning "incohérent"
func TestApt_FallbackOnInconsistent(t *testing.T) {
	tmpDir := t.TempDir()
	extPath := filepath.Join(tmpDir, "extended_states")
	// Package pkg-1 is manual (Auto-Installed: 0), pkg-2 is auto (Auto-Installed: 1)
	extContent := `Package: pkg-1
Architecture: amd64
Auto-Installed: 0

Package: pkg-2
Architecture: amd64
Auto-Installed: 1
`
	if err := os.WriteFile(extPath, []byte(extContent), 0644); err != nil {
		t.Fatal(err)
	}

	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "apt-mark" && len(args) == 1 && args[0] == "showmanual":
			return []byte(""), nil // 0 paquet
		case name == "dpkg-query" && len(args) == 3 && args[0] == "-W":
			// Renvoie pkg-1, pkg-2 et 1200 autres
			var b strings.Builder
			b.WriteString("pkg-1\t1.0\tii \npkg-2\t1.0\tii \n")
			b.Write(dpkgLines("extra", 1200))
			return []byte(b.String()), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}

	a := NewWithStatePath(runner, extPath)
	res, err := a.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if res.Level != adapter.Level2 {
		t.Errorf("Level = %d, want %d (Level 2)", res.Level, adapter.Level2)
	}
	if res.Reliability != adapter.ReliabilityMedium {
		t.Errorf("Reliability = %s, want %s", res.Reliability, adapter.ReliabilityMedium)
	}
	if len(res.Packages) != 1 || res.Packages[0].Name != "pkg-1" {
		t.Errorf("Packages = %v, want [pkg-1]", res.Packages)
	}
	foundIncoherentWarning := false
	for _, w := range res.Warnings {
		if strings.Contains(w, "incohérence") {
			foundIncoherentWarning = true
			break
		}
	}
	if !foundIncoherentWarning {
		t.Errorf("Warnings = %v, expected warning containing 'incohérence'", res.Warnings)
	}
}

// TestApt_FallbackOnError : niveau 1 échoue (exit != 0), niveau 2 OK -> Level == 2, warning "repli" / "échoué"
func TestApt_FallbackOnError(t *testing.T) {
	tmpDir := t.TempDir()
	extPath := filepath.Join(tmpDir, "extended_states")
	extContent := `Package: vim
Auto-Installed: 0
`
	if err := os.WriteFile(extPath, []byte(extContent), 0644); err != nil {
		t.Fatal(err)
	}

	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "apt-mark" && len(args) == 1 && args[0] == "showmanual":
			return nil, fmt.Errorf("apt-mark error: command failed")
		case name == "dpkg-query" && len(args) == 3 && args[0] == "-W":
			return []byte("vim\t2:9.1\tii \n"), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}

	a := NewWithStatePath(runner, extPath)
	res, err := a.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if res.Level != adapter.Level2 {
		t.Errorf("Level = %d, want %d", res.Level, adapter.Level2)
	}
	if res.Reliability != adapter.ReliabilityMedium {
		t.Errorf("Reliability = %s, want %s", res.Reliability, adapter.ReliabilityMedium)
	}
	if len(res.Packages) != 1 || res.Packages[0].Name != "vim" {
		t.Errorf("Packages = %v, want [vim]", res.Packages)
	}
}

// TestApt_AllLevelsFail : tous les niveaux échouent -> erreur retournée, pas de panic
func TestApt_AllLevelsFail(t *testing.T) {
	runner := func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("execution failed: %s", name)
	}

	a := NewWithStatePath(runner, "/nonexistent/extended_states")
	res, err := a.Capture()
	if err == nil {
		t.Fatalf("expected error when all levels fail, got res = %v", res)
	}
	if !strings.Contains(err.Error(), "échec de tous les niveaux") {
		t.Errorf("error message = %q, want mention of échec de tous les niveaux", err.Error())
	}
}

// TestApt_HealthDetectsInconsistency : Health() retourne Functional: false si niveau 1 = 0 et dpkg-query -W > 100
func TestApt_HealthDetectsInconsistency(t *testing.T) {
	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "apt-get" && len(args) == 1 && args[0] == "--version":
			return []byte("apt 2.6\n"), nil
		case name == "dpkg" && len(args) == 1 && args[0] == "--version":
			return []byte("dpkg 1.22\n"), nil
		case name == "apt-mark" && len(args) == 1 && args[0] == "showmanual":
			return []byte(""), nil // 0 paquet
		case name == "dpkg-query" && len(args) == 3 && args[0] == "-W":
			return dpkgLines("pkg", 150), nil // 150 paquets
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}

	a := New(runner)
	health, err := a.Health()
	if err != nil {
		t.Fatalf("Health() error = %v", err)
	}
	if !health.Present {
		t.Errorf("Present = false, want true")
	}
	if health.Functional {
		t.Errorf("Functional = true, want false when inconsistent")
	}
	if !strings.Contains(health.Message, "INCOHÉRENCE DÉTECTÉE") {
		t.Errorf("health.Message = %q, want mention of INCOHÉRENCE DÉTECTÉE", health.Message)
	}
}

// TestApt_ExtendedStatesParser : parser correctement le format Auto-Installed: 0/1, gérer blocs vides/commentaires
func TestApt_ExtendedStatesParser(t *testing.T) {
	content := []byte(`# APT extended states file
Package: curl
Architecture: amd64
Auto-Installed: 0

# Another block
Package: libcurl4
Architecture: amd64
Auto-Installed: 1

Package: jq
Auto-Installed: 0
`)

	res := ParseExtendedStates(content)
	if len(res) != 3 {
		t.Fatalf("ParseExtendedStates parsed %d packages, want 3", len(res))
	}
	if res["curl"] != false {
		t.Errorf("curl Auto-Installed = %v, want false", res["curl"])
	}
	if res["libcurl4"] != true {
		t.Errorf("libcurl4 Auto-Installed = %v, want true", res["libcurl4"])
	}
	if res["jq"] != false {
		t.Errorf("jq Auto-Installed = %v, want false", res["jq"])
	}
}

// TestApt_CaptureRespectsAutoInstalled : niveau 2 exclut les paquets avec Auto-Installed: 1
func TestApt_CaptureRespectsAutoInstalled(t *testing.T) {
	tmpDir := t.TempDir()
	extPath := filepath.Join(tmpDir, "extended_states")
	extContent := `Package: htop
Auto-Installed: 0

Package: libhtop-dev
Auto-Installed: 1
`
	if err := os.WriteFile(extPath, []byte(extContent), 0644); err != nil {
		t.Fatal(err)
	}

	runner := func(name string, args ...string) ([]byte, error) {
		switch {
		case name == "apt-mark" && len(args) == 1 && args[0] == "showmanual":
			return nil, fmt.Errorf("apt-mark not available")
		case name == "dpkg-query" && len(args) == 3 && args[0] == "-W":
			return []byte("htop\t3.3.0\tii \nlibhtop-dev\t3.3.0\tii \n"), nil
		default:
			return nil, fmt.Errorf("unexpected command: %s %v", name, args)
		}
	}

	a := NewWithStatePath(runner, extPath)
	res, err := a.Capture()
	if err != nil {
		t.Fatalf("Capture() error = %v", err)
	}
	if len(res.Packages) != 1 {
		t.Fatalf("len(Packages) = %d, want 1", len(res.Packages))
	}
	if res.Packages[0].Name != "htop" {
		t.Errorf("Package[0] = %q, want htop", res.Packages[0].Name)
	}
}

// TestApt_Detect : vérifie le comportement de Detect()
func TestApt_Detect(t *testing.T) {
	aPresent := New(func(name string, args ...string) ([]byte, error) {
		if name == "apt-get" {
			return []byte("apt 2.0\n"), nil
		}
		return nil, fmt.Errorf("not found")
	})
	detected, err := aPresent.Detect()
	if err != nil || !detected {
		t.Errorf("Detect() = %v, %v; want true, nil", detected, err)
	}

	aAbsent := New(func(name string, args ...string) ([]byte, error) {
		return nil, fmt.Errorf("not found")
	})
	detected, err = aAbsent.Detect()
	if err != nil || detected {
		t.Errorf("Detect() = %v, %v; want false, nil", detected, err)
	}
}
