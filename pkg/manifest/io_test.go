package manifest

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestManifest_RoundTrip(t *testing.T) {
	want := Manifest{
		Schema: SchemaVersion,
		Source: Source{
			Distro:       "fedora",
			Version:      "44",
			Arch:         "amd64",
			Hostname:     "test-host",
			CapturedAt:   time.Date(2026, 10, 15, 14, 23, 11, 0, time.UTC),
			CaptureLevel: map[string]int{"dnf": 2, "flatpak": 1},
		},
		Repos: []Repo{{Kind: "rpm", Name: "fedora", URL: "https://example.test", FileName: "fedora.repo", Enabled: true, ThirdParty: true, Extra: map[string]any{"priority": 10}}},
		Packages: []Package{
			{Name: "htop", Manager: "dnf", Version: "3.3.0", Extra: map[string]any{"reason": "manual"}},
		},
		Flatpak: FlatpakBlock{
			Remotes:   []FlatpakRemote{{Name: "flathub", URL: "https://flathub.org", Scope: "system"}},
			Apps:      []FlatpakApp{{Ref: "org.example.App", Scope: "user"}},
			Overrides: []FlatpakOverride{{Ref: "org.example.App", Scope: "user", Args: []string{"--filesystem=home"}}},
		},
		Snaps:            []Snap{{Name: "hello", Channel: "stable", Scope: "system"}},
		LanguagePackages: LanguagePackages{Pipx: []string{"black"}, Cargo: []string{"ripgrep"}, Npm: []string{"typescript"}},
		ServicesEnabled:  []string{"sshd.service"},
		Cron:             []CronEntry{{Schedule: "0 2 * * *", Command: "/usr/bin/backup", User: "test"}},
		Configs:          ConfigsBlock{Dotfiles: []string{".bashrc"}, AppConfigs: []string{".config/app/config"}},
		Unmanaged:        []UnmanagedEntry{{Kind: "appimage", Path: "~/Applications/App.AppImage"}},
		Extra:            map[string]any{"future_field": "future-value"},
	}

	var output bytes.Buffer
	if err := WriteYAML(&want, &output); err != nil {
		t.Fatalf("WriteYAML() error = %v", err)
	}
	got, err := ReadYAML(bytes.NewReader(output.Bytes()))
	if err != nil {
		t.Fatalf("ReadYAML() error = %v\n%s", err, output.String())
	}
	if !reflect.DeepEqual(got, &want) {
		t.Errorf("round trip mismatch:\n got: %#v\nwant: %#v", got, &want)
	}
}

func TestManifest_PreservesUnknownFields(t *testing.T) {
	input := `schema: 1
source:
  distro: fedora
  capture_level: {}
repos: []
packages:
  - name: htop
    manager: dnf
    future_package_field: retained
flatpak:
  remotes: []
  apps: []
  overrides: []
future_field: foo
`
	m, err := ReadYAML(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ReadYAML() error = %v", err)
	}
	var output bytes.Buffer
	if err := WriteYAML(m, &output); err != nil {
		t.Fatalf("WriteYAML() error = %v", err)
	}
	for _, expected := range []string{"future_field: foo", "future_package_field: retained"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("round-trip YAML is missing %q:\n%s", expected, output.String())
		}
	}
}

func TestManifest_JSONRoundTripPreservesUnknownFields(t *testing.T) {
	input := `{
  "schema": 1,
  "source": {"distro": "fedora", "capture_level": {}, "future_source_field": "retained"},
  "repos": [{"kind": "rpm", "name": "third-party", "file_name": "third-party.repo", "future_repo_field": "retained"}],
  "packages": [{"name": "htop", "manager": "dnf", "future_package_field": 10}],
  "flatpak": {
    "remotes": [{"name": "flathub", "scope": "user", "future_remote_field": "retained"}],
    "apps": [{"ref": "org.example.App", "scope": "user", "future_app_field": "retained"}],
    "overrides": [],
    "future_flatpak_field": "retained"
  },
  "configs": {"dotfiles": [".bashrc"], "future_configs_field": "retained"},
  "future_field": "retained"
}`
	got, err := ReadJSON(strings.NewReader(input))
	if err != nil {
		t.Fatalf("ReadJSON() error = %v", err)
	}

	var output bytes.Buffer
	if err := WriteJSON(got, &output); err != nil {
		t.Fatalf("WriteJSON() error = %v", err)
	}
	for _, expected := range []string{
		`"future_field": "retained"`,
		`"future_source_field": "retained"`,
		`"future_repo_field": "retained"`,
		`"future_package_field": 10`,
		`"future_remote_field": "retained"`,
		`"future_app_field": "retained"`,
		`"future_flatpak_field": "retained"`,
		`"future_configs_field": "retained"`,
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("JSON round trip is missing %q:\n%s", expected, output.String())
		}
	}
	if _, err := ReadJSON(bytes.NewReader(output.Bytes())); err != nil {
		t.Errorf("ReadJSON() rejected its own output: %v", err)
	}
}

func TestManifest_ReadJSONRejectsInvalidDocument(t *testing.T) {
	for _, input := range []string{
		`{"schema": 1,}`,
		`{"schema": 1} {"schema": 1}`,
	} {
		if _, err := ReadJSON(strings.NewReader(input)); err == nil {
			t.Errorf("ReadJSON(%q) succeeded, want invalid-document error", input)
		}
	}
}

func TestManifest_ValidateRejectsUnknownSchema(t *testing.T) {
	err := Validate(&Manifest{Schema: 99, Source: Source{Distro: "fedora"}})
	if err == nil || !strings.Contains(err.Error(), "schéma inconnu : 99, attendu 1") {
		t.Errorf("Validate() error = %v, want explicit unknown schema error", err)
	}
}

func TestManifest_ValidateRejectsEmptyDistro(t *testing.T) {
	err := Validate(&Manifest{Schema: SchemaVersion})
	if err == nil || !strings.Contains(err.Error(), "source.distro") {
		t.Errorf("Validate() error = %v, want empty distro error", err)
	}
}

func TestManifest_ValidateWarnsOnSecrets(t *testing.T) {
	m := &Manifest{
		Schema: SchemaVersion,
		Source: Source{Distro: "fedora"},
		Packages: []Package{{
			Name: "test", Manager: "dnf",
			Extra: map[string]any{"note": "-----BEGIN PRIVATE KEY-----"},
		}},
	}
	warnings, err := ValidateWithWarnings(m)
	if err != nil {
		t.Fatalf("ValidateWithWarnings() error = %v, want warning only", err)
	}
	if len(warnings) == 0 || !strings.Contains(warnings[0], "secret") {
		t.Errorf("warnings = %v, want secret warning", warnings)
	}
}

func TestValidate_RejectsPrivateKeyInRepoContent(t *testing.T) {
	for _, content := range []string{
		"password=secret",
		"token=abc",
		"apiKey=abc",
		"private_key=abc",
		"-----BEGIN PGP PRIVATE KEY-----",
	} {
		m := &Manifest{
			Schema: SchemaVersion,
			Source: Source{Distro: "fedora"},
			Repos:  []Repo{{Kind: "rpm", Name: "test", FileName: "test.repo", Content: content}},
		}
		if err := Validate(m); err == nil {
			t.Errorf("Validate() accepted sensitive repo content %q", content)
		}
	}
	m := &Manifest{
		Schema: SchemaVersion,
		Source: Source{Distro: "fedora"},
		Repos: []Repo{{
			Kind: "rpm", Name: "test", FileName: "test.repo",
			Extra: map[string]any{"nested": map[string]any{"private_key": "hidden"}},
		}},
	}
	if err := Validate(m); err == nil {
		t.Error("Validate() accepted a sensitive field nested in Repo.Extra")
	}
}

func TestValidate_WarnsOnThirdPartyEnabled(t *testing.T) {
	m := &Manifest{
		Schema: SchemaVersion,
		Source: Source{Distro: "fedora"},
		Repos: []Repo{{
			Kind: "rpm", Name: "external", FileName: "external.repo",
			Enabled: true, ThirdParty: true,
		}},
	}
	warnings, err := ValidateWithWarnings(m)
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "confirmation manuelle requise") {
		t.Fatalf("warnings = %#v", warnings)
	}
}
