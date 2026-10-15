package planner

import (
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/transpose-project/transpose/internal/adapter"
	"github.com/transpose-project/transpose/internal/adapter/flatpak"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func TestBuildManifest_FromDnfAndFlatpak(t *testing.T) {
	results := []*adapter.CaptureResult{
		{
			Manager:     "dnf",
			Level:       adapter.Level2,
			Reliability: adapter.ReliabilityMedium,
			Packages: []adapter.Package{
				{Name: "htop", Version: "3.3.0"},
				{Name: "vim", Version: "9.1"},
			},
		},
		{
			Manager: "flatpak",
			Level:   adapter.Level1,
			Raw: map[string]any{
				"remotes": map[string][]flatpak.Remote{
					"system": {{Name: "flathub", URL: "https://flathub.org"}},
					"user":   {},
				},
				"apps": []flatpak.App{{Ref: "org.example.App", Scope: "user"}},
				"overrides": map[string]string{
					"user": "--filesystem=home",
				},
			},
		},
	}
	capturedAt := time.Date(2026, 10, 15, 11, 0, 0, 0, time.FixedZone("test", 3600))
	m, err := BuildManifestWithSource(results, strings.NewReader("ID=fedora\nVERSION_ID=\"44\"\n"), "host", capturedAt)
	if err != nil {
		t.Fatalf("BuildManifestWithSource() error = %v", err)
	}
	if m.Schema != manifest.SchemaVersion || m.Source.Distro != "fedora" || m.Source.Version != "44" {
		t.Errorf("metadata = schema %d, distro %q, version %q", m.Schema, m.Source.Distro, m.Source.Version)
	}
	if m.Source.CaptureLevel["dnf"] != 2 || m.Source.CaptureLevel["flatpak"] != 1 {
		t.Errorf("capture levels = %v", m.Source.CaptureLevel)
	}
	if len(m.Packages) != 2 || m.Packages[0].Name != "htop" || m.Packages[0].Manager != "dnf" {
		t.Errorf("native packages = %#v", m.Packages)
	}
	if !reflect.DeepEqual(m.Flatpak.Remotes, []manifest.FlatpakRemote{{Name: "flathub", URL: "https://flathub.org", Scope: "system"}}) {
		t.Errorf("Flatpak remotes = %#v", m.Flatpak.Remotes)
	}
	if !reflect.DeepEqual(m.Flatpak.Apps, []manifest.FlatpakApp{{Ref: "org.example.App", Scope: "user"}}) {
		t.Errorf("Flatpak apps = %#v", m.Flatpak.Apps)
	}
	if len(m.Flatpak.Overrides) != 1 || m.Flatpak.Overrides[0].Args[0] != "--filesystem=home" {
		t.Errorf("Flatpak overrides = %#v", m.Flatpak.Overrides)
	}
	if !m.Source.CapturedAt.Equal(capturedAt.UTC()) {
		t.Errorf("CapturedAt = %s, want %s", m.Source.CapturedAt, capturedAt.UTC())
	}
}

func TestBuildManifest_EmptyResults(t *testing.T) {
	m, err := BuildManifestWithSource(nil, strings.NewReader("ID=debian\nVERSION_ID=13\n"), "host", time.Time{})
	if err != nil {
		t.Fatalf("BuildManifestWithSource() error = %v", err)
	}
	if m.Schema != 1 || m.Source.Distro != "debian" {
		t.Errorf("manifest metadata = %#v", m)
	}
	if len(m.Packages) != 0 || len(m.Repos) != 0 || len(m.Flatpak.Apps) != 0 || len(m.Flatpak.Remotes) != 0 || len(m.Flatpak.Overrides) != 0 {
		t.Errorf("expected empty package lists: %#v", m)
	}
}

func TestBuildManifest_SourceMetadata(t *testing.T) {
	m, err := BuildManifestWithSource(nil, strings.NewReader("NAME=TestOS\nID=\"test-os\"\nVERSION_ID=\"1.2\"\n"), "fake-host", time.Now())
	if err != nil {
		t.Fatalf("BuildManifestWithSource() error = %v", err)
	}
	if m.Source.Distro != "test-os" || m.Source.Version != "1.2" {
		t.Errorf("distro/version = %q/%q, want test-os/1.2", m.Source.Distro, m.Source.Version)
	}
	if m.Source.Hostname != "fake-host" {
		t.Errorf("Hostname = %q, want fake-host", m.Source.Hostname)
	}
}
