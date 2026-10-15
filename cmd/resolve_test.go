package cmd

import (
	"bytes"
	"context"
	"encoding/csv"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/transpose-project/transpose/internal/resolver/repology"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func TestResolveCommand_OutputCSV(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "repology.db")
	db, err := repology.OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	project := &repology.Project{
		Name: "htop",
		Packages: []repology.ProjectPackage{{
			Repo: "debian_13", Name: "htop", Version: "3.4.0",
		}},
	}
	if _, err := repology.StoreProject(context.Background(), db, project, "", "debian_13"); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	manifestPath := writeCommandManifest(t, []manifest.Package{{Name: "htop", Manager: "dnf", Version: "3.3.0"}})
	outputPath := filepath.Join(t.TempDir(), "sample.csv")
	root := commandRoot(newResolveCmdWithDBPath(dbPath))
	root.SetArgs([]string{"resolve", "--from-manifest", manifestPath, "--target", "debian_13", "--output", outputPath})
	var stdout bytes.Buffer
	root.SetOut(&stdout)
	if err := root.Execute(); err != nil {
		t.Fatalf("resolve execution error = %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error = %v", err)
	}
	wantHeader := []string{"index", "source_name", "source_version", "target_name", "target_version", "status", "confidence", "notes"}
	if !reflect.DeepEqual(records[0], wantHeader) || len(records) != 2 || records[1][5] != "exact" {
		t.Errorf("CSV records = %#v", records)
	}
}

func TestResolveCommand_SampleLimit(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "repology.db")
	db, err := repology.OpenDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"first", "second", "third"} {
		project := &repology.Project{Name: name, Packages: []repology.ProjectPackage{
			{Repo: "debian_13", Name: name, Version: "1"},
		}}
		if _, err := repology.StoreProject(context.Background(), db, project, "", "debian_13"); err != nil {
			t.Fatal(err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	manifestPath := writeCommandManifest(t, []manifest.Package{
		{Name: "first", Manager: "dnf"},
		{Name: "second", Manager: "dnf"},
		{Name: "third", Manager: "dnf"},
	})
	outputPath := filepath.Join(t.TempDir(), "limited.csv")
	root := commandRoot(newResolveCmdWithDBPath(dbPath))
	root.SetArgs([]string{"resolve", "--from-manifest", manifestPath, "--sample", "2", "--output", outputPath})
	if err := root.Execute(); err != nil {
		t.Fatalf("resolve execution error = %v", err)
	}
	data, err := os.ReadFile(outputPath)
	if err != nil {
		t.Fatal(err)
	}
	records, err := csv.NewReader(bytes.NewReader(data)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 3 {
		t.Errorf("got %d CSV rows including header, want 3", len(records))
	}
}

func writeCommandManifest(t *testing.T, packages []manifest.Package) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "manifest.yaml")
	m := &manifest.Manifest{
		Schema:   manifest.SchemaVersion,
		Source:   manifest.Source{Distro: "fedora"},
		Packages: packages,
	}
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := manifest.WriteYAML(m, file); err != nil {
		file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
