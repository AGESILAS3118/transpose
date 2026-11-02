package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func samplePackManifestYAML() string {
	return `schema: 1
source:
  distro: fedora
  version: "44"
  arch: amd64
packages:
  - name: htop
    manager: dnf
  - name: tree
    manager: dnf
repos:
  - kind: rpm
    name: custom
    file_name: custom.repo
    content: "[custom]\nname=custom\nenabled=1\n"
`
}

func TestPackCmd_ProducesArchive(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(samplePackManifestYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	root := newRootCmd()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"pack", manifestPath})

	// By default, outputs to manifest.tar.zst in current or target dir
	expectedArchive := strings.TrimSuffix(manifestPath, ".yaml") + ".tar.zst"
	defer os.Remove(expectedArchive)

	if err := root.Execute(); err != nil {
		t.Fatalf("pack execution failed: %v", err)
	}

	if _, err := os.Stat(expectedArchive); err != nil {
		t.Fatalf("expected archive %s to be created: %v", expectedArchive, err)
	}

	out := stdout.String()
	if !strings.Contains(out, "Archive créée") {
		t.Errorf("stdout = %q, expected 'Archive créée'", out)
	}
	if !strings.Contains(out, "Manifeste    : 2 paquets") {
		t.Errorf("stdout = %q, expected 2 paquets", out)
	}
}

func TestPackCmd_CustomOutput(t *testing.T) {
	tmpDir := t.TempDir()
	manifestPath := filepath.Join(tmpDir, "manifest.yaml")
	if err := os.WriteFile(manifestPath, []byte(samplePackManifestYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	customOut := filepath.Join(tmpDir, "custom-bundle.tar.zst")
	root := newRootCmd()
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	root.SetArgs([]string{"pack", manifestPath, "-o", customOut, "--verbose"})

	if err := root.Execute(); err != nil {
		t.Fatalf("pack execution failed: %v", err)
	}

	if _, err := os.Stat(customOut); err != nil {
		t.Fatalf("expected custom archive %s to exist: %v", customOut, err)
	}

	out := stdout.String()
	if !strings.Contains(out, customOut) {
		t.Errorf("stdout = %q, expected mention of %s", out, customOut)
	}
	if !strings.Contains(out, "tar.zst (version 1)") {
		t.Errorf("stdout = %q, expected format version", out)
	}
}
