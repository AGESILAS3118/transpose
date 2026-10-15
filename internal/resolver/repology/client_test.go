package repology

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestClient_RateLimit(t *testing.T) {
	var requests atomic.Int32
	var requestTimes []time.Time
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		requestTimes = append(requestTimes, time.Now())
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintln(w, `[]`)
	}))
	defer server.Close()
	client := NewClientWithOptions(ClientOptions{
		BaseURL:      server.URL,
		CacheDir:     t.TempDir(),
		HTTPClient:   server.Client(),
		RetryBackoff: []time.Duration{},
	})

	for _, name := range []string{"project-one", "project-two"} {
		if _, _, err := client.Fetch(context.Background(), name); err != nil {
			t.Fatalf("Fetch(%q) error = %v", name, err)
		}
	}
	if got := requests.Load(); got != 2 {
		t.Fatalf("got %d requests, want 2", got)
	}
	if elapsed := requestTimes[1].Sub(requestTimes[0]); elapsed < time.Second {
		t.Errorf("requests spaced %s apart, want at least 1s", elapsed)
	}
}

func TestClient_CacheHit(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		json.NewEncoder(w).Encode([]ProjectPackage{{Repo: "debian_13", Name: "project", Version: "1"}})
	}))
	defer server.Close()
	client := NewClientWithOptions(ClientOptions{
		BaseURL:      server.URL,
		CacheDir:     t.TempDir(),
		HTTPClient:   server.Client(),
		RetryBackoff: []time.Duration{},
	})

	if _, meta, err := client.Fetch(context.Background(), "project"); err != nil || meta.CacheHit {
		t.Fatalf("first Fetch() = meta %#v, error %v", meta, err)
	}
	project, meta, err := client.Fetch(context.Background(), "project")
	if err != nil {
		t.Fatalf("second Fetch() error = %v", err)
	}
	if !meta.CacheHit || project == nil || len(project.Packages) != 1 {
		t.Errorf("second Fetch() = project %#v, meta %#v; want cached project", project, meta)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d HTTP requests, want 1", got)
	}
}

func TestClient_404Handling(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.NotFound(w, r)
	}))
	defer server.Close()
	client := NewClientWithOptions(ClientOptions{
		BaseURL:      server.URL,
		CacheDir:     t.TempDir(),
		HTTPClient:   server.Client(),
		RetryBackoff: []time.Duration{},
	})

	project, meta, err := client.Fetch(context.Background(), "missing")
	if err != nil || project != nil || !meta.NotFound {
		t.Fatalf("Fetch() = project %#v, meta %#v, error %v; want cached absence", project, meta, err)
	}
	project, meta, err = client.Fetch(context.Background(), "missing")
	if err != nil || project != nil || !meta.NotFound || !meta.CacheHit {
		t.Errorf("cached 404 Fetch() = project %#v, meta %#v, error %v", project, meta, err)
	}
	if got := requests.Load(); got != 1 {
		t.Errorf("got %d HTTP requests, want 1", got)
	}
}

func TestClient_RetryOn5xx(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			http.Error(w, "temporary", http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode([]ProjectPackage{{Repo: "debian_13", Name: "package", Version: "1"}})
	}))
	defer server.Close()
	client := NewClientWithOptions(ClientOptions{
		BaseURL:      server.URL,
		CacheDir:     t.TempDir(),
		HTTPClient:   server.Client(),
		RetryBackoff: []time.Duration{time.Millisecond},
		RateLimit:    time.Millisecond,
	})

	project, _, err := client.Fetch(context.Background(), "project")
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if project == nil || len(project.Packages) != 1 {
		t.Errorf("project = %#v, want successful retry result", project)
	}
	if got := requests.Load(); got != 2 {
		t.Errorf("got %d HTTP requests, want 2", got)
	}
}

func TestProjectPackage_DecodesRepologyBinaryNames(t *testing.T) {
	var pkg ProjectPackage
	err := json.Unmarshal([]byte(`{
		"repo":"debian_13",
		"srcname":"acl",
		"binname":"acl",
		"visiblename":"acl",
		"version":"2.3.2",
		"maintainers":["maintainer@example.test"]
	}`), &pkg)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "acl" || pkg.Maintainer != "maintainer@example.test" || pkg.Repo != "debian_13" {
		t.Errorf("decoded Repology package = %#v", pkg)
	}
}
