package resolver

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/pkg/manifest"
)

type mockRepoQuerier struct {
	names []string
	err   error
	calls int
}

func (q *mockRepoQuerier) ListAvailable(context.Context) ([]string, error) {
	q.calls++
	return q.names, q.err
}

func TestSameFamily_Exact(t *testing.T) {
	result, err := NewSameFamilyResolver(&mockRepoQuerier{names: []string{"htop", "vim"}}, nil).
		Resolve(manifest.Package{Name: "htop", Manager: "dnf"}, Target{Family: "rpm"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusExact || result.Confidence != "high" || result.Target != "htop" {
		t.Fatalf("resolution = %#v", result)
	}
}

func TestSameFamily_OverrideApplied(t *testing.T) {
	result, err := NewSameFamilyResolver(&mockRepoQuerier{}, &Overrides{Overrides: []Override{
		{SourceName: "htop", TargetName: "htop-git"},
	}}).Resolve(manifest.Package{Name: "htop", Manager: "dnf"}, Target{Family: "rpm"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusEquivalent || result.Target != "htop-git" || !strings.Contains(result.Reason, "override") {
		t.Fatalf("resolution = %#v", result)
	}
}

func TestSameFamily_NotFound(t *testing.T) {
	result, err := NewSameFamilyResolver(&mockRepoQuerier{names: []string{"vim"}}, nil).
		Resolve(manifest.Package{Name: "inconnu"}, Target{Family: "rpm"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusNotFound || result.Reason != "absent des dépôts cibles" {
		t.Fatalf("resolution = %#v", result)
	}
}

func TestSameFamily_CaseInsensitive(t *testing.T) {
	result, err := NewSameFamilyResolver(&mockRepoQuerier{names: []string{"htop"}}, nil).
		Resolve(manifest.Package{Name: "HTOP"}, Target{Family: "rpm"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != StatusExact {
		t.Fatalf("status = %q, want exact", result.Status)
	}
}

func TestSameFamily_QuerierError(t *testing.T) {
	_, err := NewSameFamilyResolver(&mockRepoQuerier{err: errors.New("repository unavailable")}, nil).
		Resolve(manifest.Package{Name: "htop"}, Target{Family: "rpm"})
	if err == nil || !strings.Contains(err.Error(), "repository unavailable") {
		t.Fatalf("Resolve() error = %v, want querier failure", err)
	}
}

func TestLoadOverridesMissingFile(t *testing.T) {
	overrides, err := LoadOverrides(t.TempDir() + "/missing.yaml")
	if err != nil {
		t.Fatalf("LoadOverrides() error = %v", err)
	}
	if override, ok := overrides.Lookup("htop"); ok || override != (Override{}) {
		t.Fatalf("Lookup() = %#v, %v, want no match", override, ok)
	}
}
