package cmd

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/adapter"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func TestCapture_ProducesValidManifest(t *testing.T) {
	mock := manifestMockAdapter()
	outputPath := filepath.Join(t.TempDir(), "nested", "manifest.yaml")
	root := commandRoot(newCaptureCmdWithAdapters([]adapter.Adapter{mock}))
	root.SetArgs([]string{"capture", "--only", "dnf", "-o", outputPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("capture execution error = %v", err)
	}
	file, err := os.Open(outputPath)
	if err != nil {
		t.Fatalf("open manifest: %v", err)
	}
	defer file.Close()
	result, err := manifest.ReadYAML(file)
	if err != nil {
		t.Fatalf("ReadYAML() error = %v", err)
	}
	if result.Schema != manifest.SchemaVersion || result.Source.Distro == "" {
		t.Errorf("invalid manifest metadata: %#v", result)
	}
	if len(result.Packages) != 1 || result.Packages[0].Name != "htop" {
		t.Errorf("manifest packages = %#v, want htop", result.Packages)
	}
}

func TestCapture_JSONFormat(t *testing.T) {
	mock := manifestMockAdapter()
	root := commandRoot(newCaptureCmdWithAdapters([]adapter.Adapter{mock}))
	root.SetArgs([]string{"capture", "--only", "dnf", "--format", "json"})
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	if err := root.Execute(); err != nil {
		t.Fatalf("capture execution error = %v", err)
	}
	var result manifest.Manifest
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
	}
	if result.Schema != manifest.SchemaVersion || len(result.Packages) != 1 {
		t.Errorf("JSON manifest = %#v", result)
	}
}

func TestCapture_WarningsGoToStderr(t *testing.T) {
	mock := manifestMockAdapter()
	mock.result.Warnings = []string{"warning-for-test"}
	root := commandRoot(newCaptureCmdWithAdapters([]adapter.Adapter{mock}))
	root.SetArgs([]string{"capture", "--only", "dnf"})
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	if err := root.Execute(); err != nil {
		t.Fatalf("capture execution error = %v", err)
	}
	if strings.Contains(stdout.String(), "warning-for-test") {
		t.Errorf("warning leaked to stdout: %s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "warning-for-test") {
		t.Errorf("stderr %q does not contain capture warning", stderr.String())
	}
}

func manifestMockAdapter() mockAdapter {
	return mockAdapter{
		name:  "dnf",
		found: true,
		result: &adapter.CaptureResult{
			Manager:     "dnf",
			Level:       adapter.Level1,
			Reliability: adapter.ReliabilityHigh,
			Packages:    []adapter.Package{{Name: "htop", Version: "3.3.0", Source: adapter.Level1}},
			Warnings:    []string{},
		},
	}
}
