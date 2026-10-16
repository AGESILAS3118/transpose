package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	repoadapter "github.com/transpose-project/transpose/internal/adapter/repos"
)

func TestReposList_ProducesYAML(t *testing.T) {
	aptDirectory := filepath.Join(t.TempDir(), "apt")
	if err := os.MkdirAll(aptDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	collectors := []repoadapter.RepoCollector{
		repoadapter.NewRPMRepoCollector(filepath.Join(t.TempDir(), "missing-rpm")),
		repoadapter.NewAPTRepoCollector(aptDirectory),
	}
	root := commandRoot(newReposCmdWithCollectors(collectors))
	root.SetArgs([]string{"repos", "list"})
	var output bytes.Buffer
	root.SetOut(&output)
	if err := root.Execute(); err != nil {
		t.Fatalf("repos list error = %v", err)
	}
	if strings.TrimSpace(output.String()) != "[]" {
		t.Fatalf("YAML output = %q, want empty YAML list", output.String())
	}
}

func TestReposCheck_DetectsDuplicateNames(t *testing.T) {
	path := filepath.Join(t.TempDir(), "repos.yaml")
	content := `repos:
  - kind: rpm
    name: duplicate
    file_name: first.repo
    url: https://example.com/one
  - kind: rpm
    name: DUPLICATE
    file_name: second.repo
    url: https://example.com/two
`
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	root := commandRoot(newReposCmdWithCollectors(nil))
	root.SetArgs([]string{"repos", "check", path})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "dupliqué") {
		t.Fatalf("repos check error = %v, want duplicate-name error", err)
	}
}

func TestReposExport_RefusesEtcOutput(t *testing.T) {
	root := commandRoot(newReposCmdWithCollectors(nil))
	root.SetArgs([]string{"repos", "export", "-o", "/etc/xpose-repos-must-not-be-written.yaml"})
	err := root.Execute()
	if err == nil || !strings.Contains(err.Error(), "écriture interdite sous /etc") {
		t.Fatalf("repos export error = %v, want /etc path rejection", err)
	}
}

func TestReposExportWritesOnlyReposSection(t *testing.T) {
	directory := t.TempDir()
	repoFile := filepath.Join(directory, "fedora.repo")
	content := "[fedora]\nname=Fedora\nbaseurl=https://download.fedoraproject.org/pub/fedora/linux\n"
	if err := os.WriteFile(repoFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "repos.yaml")
	root := commandRoot(newReposCmdWithCollectors([]repoadapter.RepoCollector{
		repoadapter.NewRPMRepoCollector(directory),
	}))
	root.SetArgs([]string{"repos", "export", "-o", out})
	if err := root.Execute(); err != nil {
		t.Fatalf("repos export error = %v", err)
	}
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "repos:") || strings.Contains(string(data), "packages:") {
		t.Fatalf("export contents = %s", data)
	}
}
