package resolver

import (
	"bytes"
	"context"
	"encoding/csv"
	"reflect"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/resolver/repology"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func TestResolve_ExactMatch(t *testing.T) {
	result := resolveFixture(t, "htop", []repology.ProjectPackage{{Repo: "debian_13", Name: "htop", Version: "3.4"}})
	assertResolution(t, result, "exact", "high")
}

func TestResolve_Equivalent(t *testing.T) {
	result := resolveFixture(t, "firefox", []repology.ProjectPackage{{Repo: "debian_13", Name: "firefox-esr", Version: "128"}})
	assertResolution(t, result, "equivalent", "medium")
}

func TestResolve_Ambiguous(t *testing.T) {
	result := resolveFixture(t, "python", []repology.ProjectPackage{
		{Repo: "debian_13", Name: "python3", Version: "3.13"},
		{Repo: "debian_13", Name: "python3-minimal", Version: "3.13"},
	})
	assertResolution(t, result, "ambiguous", "low")
}

func TestResolve_NotFound(t *testing.T) {
	result := resolveFixture(t, "unknown", nil)
	assertResolution(t, result, "not_found", "low")
}

func TestResolve_SubpackageUsesNormalizedProject(t *testing.T) {
	db, err := repology.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := &repology.Project{
		Name: "NetworkManager",
		Packages: []repology.ProjectPackage{{
			Repo: "debian_13", Name: "network-manager", Version: "1.44",
		}},
	}
	if _, err := repology.StoreProject(context.Background(), db, project, "https://networkmanager.dev", "debian_13"); err != nil {
		t.Fatal(err)
	}
	source := &manifest.Manifest{Packages: []manifest.Package{{Name: "NetworkManager-adsl", Manager: "dnf"}}}
	results, err := ResolveManifest(context.Background(), db, source, "debian_13", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].TargetName != "network-manager" ||
		results[0].Status != "equivalent" || results[0].Confidence != "medium" {
		t.Fatalf("resolution = %#v", results)
	}
}

func TestResolve_SubpackageSuffixUsesBaseProject(t *testing.T) {
	db, err := repology.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := &repology.Project{
		Name: "foo",
		Packages: []repology.ProjectPackage{{
			Repo: "debian_13", Name: "foo", Version: "1",
		}},
	}
	if _, err := repology.StoreProject(context.Background(), db, project, "", "debian_13"); err != nil {
		t.Fatal(err)
	}
	source := &manifest.Manifest{Packages: []manifest.Package{{Name: "foo-devel", Manager: "dnf"}}}
	results, err := ResolveManifest(context.Background(), db, source, "debian_13", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "equivalent" || results[0].TargetName != "foo" {
		t.Fatalf("resolution = %#v", results)
	}
}

func TestResolve_FuzzyFallback(t *testing.T) {
	db, err := repology.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, target := range []string{"htop", "htopa", "htopb"} {
		project := &repology.Project{
			Name: target,
			Packages: []repology.ProjectPackage{{
				Repo: "debian_13", Name: target, Version: "1",
			}},
		}
		if _, err := repology.StoreProject(context.Background(), db, project, "", "debian_13"); err != nil {
			t.Fatal(err)
		}
	}
	source := &manifest.Manifest{Packages: []manifest.Package{{Name: "htopx", Manager: "dnf"}}}
	results, err := ResolveManifest(context.Background(), db, source, "debian_13", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "ambiguous" || results[0].Confidence != "low" {
		t.Fatalf("resolution = %#v, want fuzzy ambiguous result", results)
	}
}

func TestResolve_FuzzySingleCandidateIsEquivalent(t *testing.T) {
	db, err := repology.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := &repology.Project{
		Name: "htop",
		Packages: []repology.ProjectPackage{{
			Repo: "debian_13", Name: "htop", Version: "3.4",
		}},
	}
	if _, err := repology.StoreProject(context.Background(), db, project, "", "debian_13"); err != nil {
		t.Fatal(err)
	}
	source := &manifest.Manifest{Packages: []manifest.Package{{Name: "htopx", Manager: "dnf"}}}
	results, err := ResolveManifest(context.Background(), db, source, "debian_13", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("got %d resolutions, want 1", len(results))
	}
	result := results[0]
	if result.Status != "equivalent" || result.Confidence != "medium" ||
		!strings.Contains(result.Notes, "fuzzy match") {
		t.Fatalf("resolution = %#v, want one fuzzy equivalent", result)
	}
}

func TestResolve_CSVFormat(t *testing.T) {
	var output bytes.Buffer
	resolutions := []RepologyResolution{{
		Index: 1, SourceName: "name,with comma", SourceVersion: "1",
		Status: "not_found", Confidence: "low", Notes: "reason, and detail",
	}}
	if err := WriteCSV(&output, resolutions); err != nil {
		t.Fatal(err)
	}
	csvText := output.String()
	records, err := csv.NewReader(strings.NewReader(csvText)).ReadAll()
	if err != nil {
		t.Fatalf("CSV parse error = %v", err)
	}
	if !reflect.DeepEqual(records[0], CSVHeader) {
		t.Errorf("CSV header = %v, want %v", records[0], CSVHeader)
	}
	if records[1][1] != resolutions[0].SourceName || records[1][7] != resolutions[0].Notes {
		t.Errorf("CSV failed to escape commas: %v", records[1])
	}
	if !strings.Contains(csvText, `"name,with comma"`) {
		t.Errorf("CSV output lacks quoting: %s", csvText)
	}
}

func resolveFixture(t *testing.T, sourceName string, target []repology.ProjectPackage) RepologyResolution {
	t.Helper()
	db, err := repology.OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := &repology.Project{Name: sourceName, Packages: target}
	if _, err := repology.StoreProject(context.Background(), db, project, "https://example.test/upstream", "debian_13"); err != nil {
		t.Fatal(err)
	}
	source := &manifest.Manifest{Packages: []manifest.Package{{Name: sourceName, Manager: "dnf", Version: "1"}}}
	result, err := ResolveManifest(context.Background(), db, source, "debian_13", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d resolutions, want 1", len(result))
	}
	return result[0]
}

func assertResolution(t *testing.T, result RepologyResolution, status, confidence string) {
	t.Helper()
	if result.Status != status || result.Confidence != confidence {
		t.Errorf("resolution = status %q confidence %q, want %q/%q", result.Status, result.Confidence, status, confidence)
	}
}
