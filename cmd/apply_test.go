package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/executor"
	"github.com/transpose-project/transpose/pkg/manifest"
)

type testConfirmer struct {
	confirmedGlobal bool
	confirmedRepo   bool
}

func (c *testConfirmer) Confirm(prompt string) (bool, error) {
	return c.confirmedGlobal, nil
}

func (c *testConfirmer) ConfirmThirdPartyRepo(repo manifest.Repo) (bool, error) {
	return c.confirmedRepo, nil
}

func samplePlanYAML() string {
	return `
source:
  distro: fedora
  version: "40"
target:
  distro: fedora
  family: rpm
steps:
  - order: 1
    manager: dnf
    action: install
    items:
      - source: htop
        target: htop
        status: exact
`
}

func sampleDestructivePlanYAML() string {
	return `
source:
  distro: fedora
target:
  distro: fedora
  family: rpm
steps:
  - order: 1
    manager: dnf
    action: remove
    items:
      - source: htop
        target: htop
`
}

func sampleRepoPlanYAML() string {
	return `
source:
  distro: fedora
target:
  distro: fedora
  family: rpm
steps:
  - order: 1
    manager: repos
    action: restore-repos
    repo_items:
      - repo:
          kind: rpm
          name: thirdparty
          file_name: thirdparty.repo
          third_party: true
          content: "[thirdparty]
name=thirdparty
"
`
}

func TestApply_DryRunByDefault(t *testing.T) {
	tmpDir := t.TempDir()
	planFile := filepath.Join(tmpDir, "plan.yaml")
	if err := os.WriteFile(planFile, []byte(samplePlanYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	var commandsRun []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		commandsRun = append(commandsRun, cmd+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	}

	applyCmd := newApplyCmdWithRunner(runner, nil)
	buf := new(bytes.Buffer)
	applyCmd.SetOut(buf)
	applyCmd.SetErr(buf)
	applyCmd.SetArgs([]string{planFile, "--journal-dir", tmpDir})

	err := applyCmd.Execute()
	if err != nil {
		t.Fatalf("expected apply dry-run to succeed, got: %v", err)
	}

	if len(commandsRun) != 0 {
		t.Fatalf("expected 0 commands run in dry-run, got: %v", commandsRun)
	}

	out := buf.String()
	if !strings.Contains(out, "Mode Dry-run") {
		t.Errorf("expected output to mention Mode Dry-run, got: %s", out)
	}
}

func TestApply_RequiresConfirmation(t *testing.T) {
	tmpDir := t.TempDir()
	planFile := filepath.Join(tmpDir, "plan.yaml")
	if err := os.WriteFile(planFile, []byte(samplePlanYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	confirmer := &testConfirmer{confirmedGlobal: false}
	applyCmd := newApplyCmdWithRunner(nil, confirmer)
	buf := new(bytes.Buffer)
	applyCmd.SetOut(buf)
	applyCmd.SetErr(buf)
	applyCmd.SetArgs([]string{planFile, "--apply", "--journal-dir", tmpDir})

	err := applyCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "annulée par l'utilisateur") {
		t.Errorf("expected cancellation message, got: %s", out)
	}
}

func TestApply_ThirdPartyRepoConfirm(t *testing.T) {
	tmpDir := t.TempDir()
	planFile := filepath.Join(tmpDir, "plan.yaml")
	if err := os.WriteFile(planFile, []byte(sampleRepoPlanYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	var commandsRun []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		commandsRun = append(commandsRun, cmd+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	}

	// Reject third-party repo
	confirmer := &testConfirmer{confirmedGlobal: true, confirmedRepo: false}
	applyCmd := newApplyCmdWithRunner(runner, confirmer)
	buf := new(bytes.Buffer)
	applyCmd.SetOut(buf)
	applyCmd.SetErr(buf)
	applyCmd.SetArgs([]string{planFile, "--apply", "--yes", "--journal-dir", tmpDir})

	err := applyCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(commandsRun) != 0 {
		t.Fatalf("expected 0 commands run when repo is rejected, got: %v", commandsRun)
	}
}

func TestApply_ResumeFlag(t *testing.T) {
	tmpDir := t.TempDir()
	planFile := filepath.Join(tmpDir, "plan.yaml")
	if err := os.WriteFile(planFile, []byte(samplePlanYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	// Pre-create journal with htop succeeded
	j, err := executor.NewJournal(tmpDir)
	if err != nil {
		t.Fatal(err)
	}
	_ = j.Append(executor.Entry{
		StepOrder: 1,
		Manager:   "dnf",
		Action:    "install",
		Items:     []string{"htop"},
		Status:    "success",
	})

	var commandsRun []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		commandsRun = append(commandsRun, cmd+" "+strings.Join(args, " "))
		return []byte("ok"), nil
	}

	applyCmd := newApplyCmdWithRunner(runner, nil)
	buf := new(bytes.Buffer)
	applyCmd.SetOut(buf)
	applyCmd.SetErr(buf)
	applyCmd.SetArgs([]string{planFile, "--apply", "--yes", "--resume", "--journal-dir", tmpDir})

	err = applyCmd.Execute()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(commandsRun) != 0 {
		t.Errorf("expected 0 commands run because htop already succeeded, got: %v", commandsRun)
	}
}

func TestApply_RejectsDestructivePlan(t *testing.T) {
	tmpDir := t.TempDir()
	planFile := filepath.Join(tmpDir, "dangerous.yaml")
	if err := os.WriteFile(planFile, []byte(sampleDestructivePlanYAML()), 0644); err != nil {
		t.Fatal(err)
	}

	applyCmd := newApplyCmdWithRunner(nil, nil)
	buf := new(bytes.Buffer)
	applyCmd.SetOut(buf)
	applyCmd.SetErr(buf)
	applyCmd.SetArgs([]string{planFile, "--journal-dir", tmpDir})

	err := applyCmd.Execute()
	if err == nil {
		t.Fatal("expected error for destructive plan, got nil")
	}

	if !strings.Contains(err.Error(), "destructive") && !strings.Contains(err.Error(), "forbidden") {
		t.Errorf("expected destructive error message, got: %v", err)
	}
}
