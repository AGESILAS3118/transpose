package planner

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/transpose-project/transpose/internal/resolver"
	"github.com/transpose-project/transpose/internal/sysutil"
	"github.com/transpose-project/transpose/pkg/manifest"
)

type mockPackageResolver map[string]resolver.Resolution

func (r mockPackageResolver) Resolve(pkg manifest.Package, _ sysutil.Target) (resolver.Resolution, error) {
	if result, ok := r[pkg.Name]; ok {
		result.Source = pkg
		return result, nil
	}
	return resolver.Resolution{
		Source: pkg, Status: resolver.StatusExact, Target: pkg.Name,
		Manager: pkg.Manager, Confidence: "high", Reason: "exact",
	}, nil
}

func TestPlan_GroupsByManager(t *testing.T) {
	m := planFixture(
		manifest.Package{Name: "dnf-a", Manager: "dnf"},
		manifest.Package{Name: "dnf-b", Manager: "dnf"},
		manifest.Package{Name: "dnf-c", Manager: "dnf"},
		manifest.Package{Name: "flat-a", Manager: "flatpak"},
		manifest.Package{Name: "flat-b", Manager: "flatpak"},
	)
	results := mockPackageResolver{
		"flat-a": alternative("flat-a", "org.test.A"),
		"flat-b": alternative("flat-b", "org.test.B"),
	}
	plan, err := GeneratePlan(m, PlanOptions{Target: sysutil.Target{Distro: "fedora", Family: "rpm"}, Resolver: results})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 2 || plan.Steps[0].Manager != "dnf" || plan.Steps[1].Manager != "flatpak" {
		t.Fatalf("steps = %#v", plan.Steps)
	}
}

func TestPlan_SkipsNotFound(t *testing.T) {
	m := planFixture(manifest.Package{Name: "gone", Manager: "dnf"})
	plan, err := GeneratePlan(m, PlanOptions{
		Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
		Resolver: mockPackageResolver{"gone": {
			Status: resolver.StatusNotFound, Reason: "absent",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Source != "gone" || len(plan.Steps) != 0 {
		t.Fatalf("plan = %#v", plan)
	}
}

func TestPlan_Exclude(t *testing.T) {
	m := planFixture(manifest.Package{Name: "keep", Manager: "dnf"}, manifest.Package{Name: "drop", Manager: "dnf"})
	plan, err := GeneratePlan(m, PlanOptions{
		Target:   sysutil.Target{Distro: "fedora", Family: "rpm"},
		Exclude:  []string{"DROP"},
		Resolver: mockPackageResolver{},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || len(plan.Steps[0].Items) != 1 || plan.Steps[0].Items[0].Source != "keep" {
		t.Fatalf("steps = %#v", plan.Steps)
	}
	if len(plan.Skipped) != 1 || plan.Skipped[0].Source != "drop" {
		t.Fatalf("skipped = %#v", plan.Skipped)
	}
}

func TestPlan_Only(t *testing.T) {
	m := planFixture(manifest.Package{Name: "dnf-package", Manager: "dnf"}, manifest.Package{Name: "flatpak-app", Manager: "flatpak"})
	plan, err := GeneratePlan(m, PlanOptions{
		Target:   sysutil.Target{Distro: "fedora", Family: "rpm"},
		Only:     []string{"dnf"},
		Resolver: mockPackageResolver{"flatpak-app": alternative("flatpak-app", "org.test.App")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].Manager != "dnf" || len(plan.Steps[0].Items) != 1 {
		t.Fatalf("steps = %#v", plan.Steps)
	}
}

func TestPlan_DeterministicOrder(t *testing.T) {
	m := planFixture(manifest.Package{Name: "z", Manager: "dnf"}, manifest.Package{Name: "a", Manager: "dnf"})
	opts := PlanOptions{
		Target:      sysutil.Target{Distro: "fedora", Family: "rpm"},
		Resolver:    mockPackageResolver{},
		GeneratedAt: time.Date(2026, 10, 16, 10, 0, 0, 0, time.UTC),
	}
	first, err := GeneratePlan(m, opts)
	if err != nil {
		t.Fatal(err)
	}
	second, err := GeneratePlan(m, opts)
	if err != nil {
		t.Fatal(err)
	}
	firstJSON, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	secondJSON, err := json.Marshal(second)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(firstJSON, secondJSON) {
		t.Fatalf("JSON plans differ:\n%s\n%s", firstJSON, secondJSON)
	}
	if first.Steps[0].Items[0].Source != "a" {
		t.Fatalf("items not sorted: %#v", first.Steps[0].Items)
	}
}

func TestPlan_SummaryCounts(t *testing.T) {
	m := planFixture(
		manifest.Package{Name: "exact", Manager: "dnf"},
		manifest.Package{Name: "equiv", Manager: "dnf"},
		manifest.Package{Name: "alt", Manager: "flatpak"},
		manifest.Package{Name: "ambig", Manager: "dnf"},
		manifest.Package{Name: "missing", Manager: "dnf"},
	)
	plan, err := GeneratePlan(m, PlanOptions{
		Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
		Resolver: mockPackageResolver{
			"equiv":   {Status: resolver.StatusEquivalent, Target: "equiv-new", Manager: "dnf"},
			"alt":     alternative("alt", "org.test.A"),
			"ambig":   {Status: resolver.StatusAmbiguous, Reason: "multiple"},
			"missing": {Status: resolver.StatusNotFound, Reason: "absent"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := PlanSummary{TotalPackages: 5, Exact: 1, Equivalent: 1, Alternative: 1, Ambiguous: 1, NotFound: 1}
	if plan.Summary != want {
		t.Fatalf("summary = %#v, want %#v", plan.Summary, want)
	}
}

func planFixture(packages ...manifest.Package) *manifest.Manifest {
	return &manifest.Manifest{Source: manifest.Source{Distro: "fedora"}, Packages: packages}
}

func alternative(source, target string) resolver.Resolution {
	return resolver.Resolution{
		Source: manifest.Package{Name: source}, Status: resolver.StatusAlternative,
		Target: target, Manager: "flatpak", Confidence: "medium", Reason: "flatpak",
	}
}
