package cmd

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/resolver/repology"
)

func TestUpdateDBCommand_ImportsFilteredDump(t *testing.T) {
	rootDir := t.TempDir()
	dumpPath := filepath.Join(rootDir, "repo.jsonl")
	dump := `{"project_name":"htop","repo":"debian_13","name":"htop","version":"3.4"}
`
	if err := os.WriteFile(dumpPath, []byte(dump), 0o600); err != nil {
		t.Fatal(err)
	}
	dbPath := filepath.Join(rootDir, "data", "repology.db")
	cachePath := filepath.Join(rootDir, "cache")
	command := newUpdateDBCmdWithPaths(func() (string, string, error) {
		return dbPath, cachePath, nil
	})
	root := commandRoot(command)
	root.SetArgs([]string{"update-db", "--from-dump", dumpPath, "--repo", "debian_13"})
	var output bytes.Buffer
	root.SetOut(&output)
	if err := root.Execute(); err != nil {
		t.Fatalf("update-db error = %v", err)
	}
	if !strings.Contains(output.String(), "Paquets importés: 1") {
		t.Errorf("output = %q", output.String())
	}
	db, err := repology.OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	packages, _, err := repology.QueryByProject(context.Background(), db, "htop", "debian_13")
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 {
		t.Errorf("got %d imported packages, want 1", len(packages))
	}
}
