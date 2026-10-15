package repology

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/transpose-project/transpose/pkg/manifest"
)

func TestDB_InsertIdempotent(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := &Project{
		Name: "htop",
		Packages: []ProjectPackage{
			{Repo: "debian_13", Name: "htop", Version: "3.4", Category: "admin"},
		},
	}
	for range 2 {
		if _, err := StoreProject(context.Background(), db, project, "https://example.test", "debian_13"); err != nil {
			t.Fatalf("StoreProject() error = %v", err)
		}
	}
	count, err := CountPackages(context.Background(), db)
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Errorf("package count = %d, want 1", count)
	}
}

func TestDB_QueryByProject(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	project := &Project{
		Name: "firefox",
		Packages: []ProjectPackage{
			{Repo: "debian_13", Name: "firefox-esr", Version: "128"},
			{Repo: "ubuntu_24_04", Name: "firefox", Version: "1"},
		},
	}
	if _, err := StoreProject(context.Background(), db, project, "https://mozilla.org", "debian_13"); err != nil {
		t.Fatal(err)
	}
	packages, upstream, err := QueryByProject(context.Background(), db, "firefox", "debian_13")
	if err != nil {
		t.Fatal(err)
	}
	if upstream != "https://mozilla.org" || len(packages) != 1 || packages[0].Name != "firefox-esr" {
		t.Errorf("QueryByProject() = %#v, upstream %q", packages, upstream)
	}
}

func TestPopulate_UsesNormalizedProjectCandidate(t *testing.T) {
	db, err := OpenDB(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/project/NetworkManager-adsl" {
			fmt.Fprintln(w, `[]`)
			return
		}
		if r.URL.Path != "/api/v1/project/NetworkManager" {
			http.NotFound(w, r)
			return
		}
		json.NewEncoder(w).Encode([]map[string]string{{
			"repo": "debian_13", "binname": "network-manager", "version": "1.52.1",
		}})
	}))
	defer server.Close()
	client := NewClientWithOptions(ClientOptions{
		BaseURL:      server.URL,
		CacheDir:     t.TempDir(),
		HTTPClient:   server.Client(),
		RateLimit:    time.Millisecond,
		RetryBackoff: []time.Duration{},
	})
	source := &manifest.Manifest{Packages: []manifest.Package{
		{Name: "NetworkManager-adsl", Manager: "dnf"},
	}}
	stats, err := PopulateWithObserver(context.Background(), db, client, source, "debian_13", nil)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Processed != 1 || stats.StoredPackages != 1 || stats.NotFound != 0 {
		t.Errorf("stats = %#v", stats)
	}
	packages, _, err := QueryByProject(context.Background(), db, "NetworkManager", "debian_13")
	if err != nil {
		t.Fatal(err)
	}
	if len(packages) != 1 || packages[0].Name != "network-manager" {
		t.Errorf("normalized project packages = %#v", packages)
	}
}
