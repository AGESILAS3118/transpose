package executor

import (
	"testing"

	"github.com/transpose-project/transpose/internal/planner"
	"github.com/transpose-project/transpose/internal/sysutil"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func TestSafety_AllowsInstall(t *testing.T) {
	p := &planner.Plan{
		Source: manifest.Source{Distro: "fedora"},
		Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
		Steps: []planner.PlanStep{
			{
				Order:   1,
				Manager: "repos",
				Action:  "restore-repos",
				RepoItems: []planner.RepoPlanItem{
					{
						Repo:   manifest.Repo{Name: "rpmfusion"},
						Action: "create",
					},
				},
			},
			{
				Order:   2,
				Manager: "dnf",
				Action:  "install",
				Command: "dnf install -y htop vim",
				Items: []planner.PlanItem{
					{Source: "htop", Target: "htop"},
					{Source: "vim", Target: "vim"},
				},
			},
		},
	}

	if err := ValidatePlanSafety(p); err != nil {
		t.Fatalf("expected valid plan, got: %v", err)
	}

	if err := ValidateSameFamily(p); err != nil {
		t.Fatalf("expected same family valid, got: %v", err)
	}
}

func TestSafety_RejectsRemove(t *testing.T) {
	p := &planner.Plan{
		Source: manifest.Source{Distro: "fedora"},
		Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
		Steps: []planner.PlanStep{
			{
				Order:   1,
				Manager: "dnf",
				Action:  "remove",
				Command: "dnf remove foo",
			},
		},
	}

	err := ValidatePlanSafety(p)
	if err == nil {
		t.Fatal("expected error for remove action, got nil")
	}
}

func TestSafety_RejectsAutoremove(t *testing.T) {
	p := &planner.Plan{
		Source: manifest.Source{Distro: "fedora"},
		Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
		Steps: []planner.PlanStep{
			{
				Order:   1,
				Manager: "dnf",
				Action:  "install",
				Command: "dnf install --autoremove foo",
			},
		},
	}

	err := ValidatePlanSafety(p)
	if err == nil {
		t.Fatal("expected error for autoremove token in command, got nil")
	}
}

func TestSafety_RejectsDestructiveFlags(t *testing.T) {
	flags := []string{"-R", "--uninstall", "--remove", "purge", "erase"}
	for _, flag := range flags {
		p := &planner.Plan{
			Source: manifest.Source{Distro: "fedora"},
			Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
			Steps: []planner.PlanStep{
				{
					Order:   1,
					Manager: "apt",
					Action:  "install",
					Command: "apt-get install " + flag,
				},
			},
		}

		if err := ValidatePlanSafety(p); err == nil {
			t.Errorf("expected error for flag %q, got nil", flag)
		}
	}
}

func TestSafety_RejectsProhibitedManager(t *testing.T) {
	p := &planner.Plan{
		Source: manifest.Source{Distro: "fedora"},
		Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
		Steps: []planner.PlanStep{
			{
				Order:   1,
				Manager: "malicious-mgr",
				Action:  "install",
			},
		},
	}

	if err := ValidatePlanSafety(p); err == nil {
		t.Fatal("expected error for unknown manager, got nil")
	}
}

func TestSafety_RejectsCrossFamily(t *testing.T) {
	p := &planner.Plan{
		Source: manifest.Source{Distro: "fedora"},
		Target: sysutil.Target{Distro: "debian", Family: "debian"},
		Steps:  []planner.PlanStep{},
	}

	if err := ValidateSameFamily(p); err == nil {
		t.Fatal("expected cross-family rejection, got nil")
	}
}
