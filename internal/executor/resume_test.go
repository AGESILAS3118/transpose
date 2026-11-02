package executor

import (
	"context"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/planner"
	"github.com/transpose-project/transpose/internal/sysutil"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func TestResume_SkipsSuccess(t *testing.T) {
	tmpDir := t.TempDir()
	j, err := NewJournal(tmpDir)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}

	// Record pkgA as already success
	_ = j.Append(Entry{
		StepOrder: 1,
		Manager:   "dnf",
		Action:    "install",
		Items:     []string{"pkgA"},
		Status:    "success",
	})

	var commandsRun []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		commandsRun = append(commandsRun, cmd+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	}

	p := &planner.Plan{
		Source: manifest.Source{Distro: "fedora"},
		Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
		Steps: []planner.PlanStep{
			{
				Order:   1,
				Manager: "dnf",
				Action:  "install",
				Items: []planner.PlanItem{
					{Source: "pkgA", Target: "pkgA"},
					{Source: "pkgB", Target: "pkgB"},
				},
			},
		},
	}

	opts := ExecOptions{
		Apply:      true,
		JournalDir: tmpDir,
		Runner:     runner,
	}

	res, err := Resume(context.Background(), j.Path, p, opts)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	if len(commandsRun) != 1 {
		t.Fatalf("expected 1 command run, got %d: %v", len(commandsRun), commandsRun)
	}

	// The command run should only include pkgB, not pkgA
	if strings.Contains(commandsRun[0], "pkgA") {
		t.Errorf("expected pkgA to be skipped, got: %s", commandsRun[0])
	}
	if !strings.Contains(commandsRun[0], "pkgB") {
		t.Errorf("expected pkgB to be installed, got: %s", commandsRun[0])
	}

	if res.Summary.SuccessItems != 1 {
		t.Errorf("expected 1 success item, got %d", res.Summary.SuccessItems)
	}
}

func TestResume_ReplaysPending(t *testing.T) {
	tmpDir := t.TempDir()
	j, err := NewJournal(tmpDir)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}

	// Record pkgA as pending (interrupted run)
	_ = j.Append(Entry{
		StepOrder: 1,
		Manager:   "dnf",
		Action:    "install",
		Items:     []string{"pkgA"},
		Status:    "pending",
	})

	var commandsRun []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		commandsRun = append(commandsRun, cmd+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	}

	p := &planner.Plan{
		Source: manifest.Source{Distro: "fedora"},
		Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
		Steps: []planner.PlanStep{
			{
				Order:   1,
				Manager: "dnf",
				Action:  "install",
				Items: []planner.PlanItem{
					{Source: "pkgA", Target: "pkgA"},
				},
			},
		},
	}

	opts := ExecOptions{
		Apply:      true,
		JournalDir: tmpDir,
		Runner:     runner,
	}

	_, err = Resume(context.Background(), j.Path, p, opts)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	if len(commandsRun) != 1 || !strings.Contains(commandsRun[0], "pkgA") {
		t.Errorf("expected pending item pkgA to be replayed, got: %v", commandsRun)
	}
}

func TestResume_NoIncompleteActions(t *testing.T) {
	tmpDir := t.TempDir()
	j, err := NewJournal(tmpDir)
	if err != nil {
		t.Fatalf("NewJournal failed: %v", err)
	}

	_ = j.Append(Entry{
		StepOrder: 1,
		Manager:   "dnf",
		Action:    "install",
		Items:     []string{"pkgA"},
		Status:    "success",
	})

	var commandsRun []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		commandsRun = append(commandsRun, cmd+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	}

	p := &planner.Plan{
		Source: manifest.Source{Distro: "fedora"},
		Target: sysutil.Target{Distro: "fedora", Family: "rpm"},
		Steps: []planner.PlanStep{
			{
				Order:   1,
				Manager: "dnf",
				Action:  "install",
				Items: []planner.PlanItem{
					{Source: "pkgA", Target: "pkgA"},
				},
			},
		},
	}

	opts := ExecOptions{
		Apply:      true,
		JournalDir: tmpDir,
		Runner:     runner,
	}

	res, err := Resume(context.Background(), j.Path, p, opts)
	if err != nil {
		t.Fatalf("Resume failed: %v", err)
	}

	if len(commandsRun) != 0 {
		t.Errorf("expected 0 commands run when nothing left to resume, got %d: %v", len(commandsRun), commandsRun)
	}

	if len(res.Plan.Steps) != 0 {
		t.Errorf("expected empty steps in plan when all succeeded, got %d", len(res.Plan.Steps))
	}
}
