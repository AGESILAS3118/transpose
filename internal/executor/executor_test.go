package executor

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/transpose-project/transpose/internal/planner"
	"github.com/transpose-project/transpose/internal/sysutil"
	"github.com/transpose-project/transpose/pkg/manifest"
)

type mockConfirmer struct {
	confirmResult bool
	repoResult    bool
}

func (m *mockConfirmer) Confirm(prompt string) (bool, error) {
	return m.confirmResult, nil
}

func (m *mockConfirmer) ConfirmThirdPartyRepo(repo manifest.Repo) (bool, error) {
	return m.repoResult, nil
}

func TestExecute_DryRunDoesNotRunCommands(t *testing.T) {
	tmpDir := t.TempDir()
	var commandsRun []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		commandsRun = append(commandsRun, cmd+" "+strings.Join(args, " "))
		return nil, nil
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
		Apply:      false, // Dry-run
		JournalDir: tmpDir,
		Runner:     runner,
	}

	res, err := Execute(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(commandsRun) != 0 {
		t.Fatalf("expected 0 commands run in dry-run, got %d: %v", len(commandsRun), commandsRun)
	}

	if res.Summary.DryRunSteps != 1 || res.Summary.DryRunItems != 1 {
		t.Errorf("unexpected summary: %+v", res.Summary)
	}
}

func TestExecute_ApplyRunsCommands(t *testing.T) {
	tmpDir := t.TempDir()
	var commandsRun []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		commandsRun = append(commandsRun, cmd+" "+strings.Join(args, " "))
		return []byte("installed successfully"), nil
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

	res, err := Execute(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(commandsRun) != 1 {
		t.Fatalf("expected 1 batch command, got %d: %v", len(commandsRun), commandsRun)
	}

	if !strings.Contains(commandsRun[0], "pkgA pkgB") {
		t.Errorf("expected batch command to include both packages, got: %s", commandsRun[0])
	}

	if res.Summary.SuccessSteps != 1 || res.Summary.SuccessItems != 2 {
		t.Errorf("unexpected summary: %+v", res.Summary)
	}
}

func TestExecute_BatchFallbackOnFailure(t *testing.T) {
	tmpDir := t.TempDir()
	var commandsRun []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		full := cmd + " " + strings.Join(args, " ")
		commandsRun = append(commandsRun, full)
		if strings.Contains(full, "pkgA pkgB") {
			return []byte("batch error: pkgB not found"), errors.New("exit status 1")
		}
		if strings.Contains(full, "pkgB") {
			return []byte("error: pkgB not found"), errors.New("exit status 1")
		}
		return []byte("installed"), nil
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

	res, err := Execute(context.Background(), p, opts)
	if err == nil {
		t.Fatal("expected error due to pkgB failure, got nil")
	}

	// 1 batch + 2 single = 3 commands run
	if len(commandsRun) != 3 {
		t.Fatalf("expected 3 commands run (1 batch + 2 single), got %d: %v", len(commandsRun), commandsRun)
	}

	if res.Summary.SuccessItems != 1 || res.Summary.FailedItems != 1 {
		t.Errorf("expected 1 success item and 1 failed item, got %+v", res.Summary)
	}
}

func TestExecute_OneItemFailsOthersSucceed(t *testing.T) {
	tmpDir := t.TempDir()
	runner := func(cmd string, args ...string) ([]byte, error) {
		full := cmd + " " + strings.Join(args, " ")
		if strings.Contains(full, "badpkg") {
			return []byte("error"), errors.New("not found")
		}
		if strings.Contains(full, "goodpkg1 goodpkg2 badpkg") {
			return []byte("error"), errors.New("batch fail")
		}
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
					{Source: "goodpkg1", Target: "goodpkg1"},
					{Source: "badpkg", Target: "badpkg"},
					{Source: "goodpkg2", Target: "goodpkg2"},
				},
			},
		},
	}

	opts := ExecOptions{
		Apply:      true,
		JournalDir: tmpDir,
		Runner:     runner,
	}

	res, err := Execute(context.Background(), p, opts)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	if res.Summary.SuccessItems != 2 || res.Summary.FailedItems != 1 {
		t.Errorf("expected 2 successes and 1 failure, got %+v", res.Summary)
	}
}

func TestExecute_RecordsJournalBeforeExecution(t *testing.T) {
	tmpDir := t.TempDir()
	var pendingLogged bool
	runner := func(cmd string, args ...string) ([]byte, error) {
		// Verify journal contains pending entry while command is running
		j, err := OpenJournal(filepathJoinTmp(tmpDir))
		if err == nil {
			entries, _ := j.ReadAll()
			for _, e := range entries {
				if e.Status == "pending" {
					pendingLogged = true
				}
			}
		}
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

	_, err := Execute(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if !pendingLogged {
		t.Errorf("expected pending status to be logged in journal before running command")
	}
}

func filepathJoinTmp(dir string) string {
	matches, _ := filepathGlob(dir + "/*.jsonl")
	if len(matches) > 0 {
		return matches[0]
	}
	return ""
}

func filepathGlob(pattern string) ([]string, error) {
	return filepath.Glob(pattern)
}

func TestExecute_ReposComeFirst(t *testing.T) {
	tmpDir := t.TempDir()
	var executionOrder []string
	runner := func(cmd string, args ...string) ([]byte, error) {
		full := cmd + " " + strings.Join(args, " ")
		executionOrder = append(executionOrder, full)
		return nil, nil
	}

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
						Repo: manifest.Repo{
							Kind:       "rpm",
							Name:       "testrepo",
							FileName:   "test.repo",
							Content:    "[test]",
							ThirdParty: true,
						},
						RequiresSudo: true,
					},
				},
			},
			{
				Order:   2,
				Manager: "dnf",
				Action:  "install",
				Items: []planner.PlanItem{
					{Source: "pkgA", Target: "pkgA"},
				},
			},
		},
	}

	confirmer := &mockConfirmer{repoResult: true}
	opts := ExecOptions{
		Apply:      true,
		JournalDir: tmpDir,
		Runner:     runner,
		Confirmer:  confirmer,
	}

	_, err := Execute(context.Background(), p, opts)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(executionOrder) != 2 {
		t.Fatalf("expected 2 commands, got %d: %v", len(executionOrder), executionOrder)
	}

	if !strings.Contains(executionOrder[0], "cp") {
		t.Errorf("expected repo cp first, got %s", executionOrder[0])
	}
	if !strings.Contains(executionOrder[1], "dnf") {
		t.Errorf("expected dnf second, got %s", executionOrder[1])
	}
}
