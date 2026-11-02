package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/transpose-project/transpose/internal/adapter"
	"github.com/transpose-project/transpose/internal/planner"
	"github.com/transpose-project/transpose/pkg/manifest"
)

type Confirmer interface {
	Confirm(prompt string) (bool, error)
	ConfirmThirdPartyRepo(repo manifest.Repo) (bool, error)
}

type ExecOptions struct {
	Apply       bool
	Yes         bool
	Interactive bool
	JournalDir  string
	Resume      bool
	Offline     bool
	Verbose     bool
	Runner      adapter.CommandRunner
	Confirmer   Confirmer
	Now         func() time.Time
}

type ExecutionSummary struct {
	TotalSteps    int
	SuccessSteps  int
	FailedSteps   int
	SkippedSteps  int
	DryRunSteps   int
	TotalItems    int
	SuccessItems  int
	FailedItems   int
	SkippedItems  int
	DryRunItems   int
	ReposRestored int
}

type Result struct {
	Plan        *planner.Plan
	JournalPath string
	RunID       string
	Summary     ExecutionSummary
	Incomplete  []Entry
	FailedItems []string
}

// Execute processes a plan according to ExecOptions, recording actions in a JSONL journal.
func Execute(ctx context.Context, p *planner.Plan, opts ExecOptions) (*Result, error) {
	if p == nil {
		return nil, errors.New("plan is nil")
	}

	// 1. Safety validation
	if err := ValidatePlanSafety(p); err != nil {
		return nil, fmt.Errorf("safety validation failed: %w", err)
	}
	if err := ValidateSameFamily(p); err != nil {
		return nil, fmt.Errorf("family validation failed: %w", err)
	}
	if opts.Apply && !opts.Yes && !opts.Interactive {
		return nil, errors.New("mode réel sans TTY : utilisez --yes ou attachez un TTY")
	}

	runner := opts.Runner
	if runner == nil {
		runner = adapter.ExecCommandRunner
	}

	nowFunc := opts.Now
	if nowFunc == nil {
		nowFunc = time.Now
	}

	// 2. Open or create Journal
	j, err := NewJournal(opts.JournalDir)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize journal: %w", err)
	}

	res := &Result{
		Plan:        p,
		JournalPath: j.Path,
		RunID:       j.RunID,
	}

	res.Summary.TotalSteps = len(p.Steps)

	// Filter and sort steps: 1: repos, 2: dnf/apt, 3: flatpak
	// Any step beyond flatpak is skipped with warning.
	for _, step := range p.Steps {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}

		mgr := strings.ToLower(strings.TrimSpace(step.Manager))
		if mgr != "repos" && mgr != "dnf" && mgr != "apt" && mgr != "flatpak" {
			// Skip unsupported step
			entry := Entry{
				Timestamp: nowFunc().UTC(),
				StepOrder: step.Order,
				Manager:   step.Manager,
				Action:    step.Action,
				Command:   step.Command,
				Status:    "skipped",
				Error:     "step manager not supported in this version",
			}
			_ = j.Append(entry)
			res.Summary.SkippedSteps++
			continue
		}

		if !opts.Apply {
			// Dry-run mode
			entry := Entry{
				Timestamp: nowFunc().UTC(),
				StepOrder: step.Order,
				Manager:   step.Manager,
				Action:    step.Action,
				Command:   step.Command,
				Items:     extractItemTargets(step),
				Status:    "dry-run",
			}
			_ = j.Append(entry)
			res.Summary.DryRunSteps++
			itemCount := len(step.Items)
			if itemCount == 0 && len(step.RepoItems) > 0 {
				itemCount = len(step.RepoItems)
			}
			res.Summary.DryRunItems += itemCount
			res.Summary.TotalItems += itemCount
			continue
		}

		// Mode Apply: Execute step
		switch mgr {
		case "repos":
			err := executeRepoStep(ctx, j, step, opts, runner, nowFunc, res)
			if err != nil {
				res.Summary.FailedSteps++
			} else {
				res.Summary.SuccessSteps++
			}
		case "dnf", "apt", "flatpak":
			err := executePackageStep(ctx, j, step, opts, runner, nowFunc, res)
			if err != nil {
				res.Summary.FailedSteps++
			} else {
				res.Summary.SuccessSteps++
			}
		}
	}

	incomplete, _ := j.FindIncomplete()
	res.Incomplete = incomplete

	if res.Summary.FailedSteps > 0 || res.Summary.FailedItems > 0 {
		return res, fmt.Errorf("execution completed with %d failed item(s)", res.Summary.FailedItems)
	}

	return res, nil
}

func extractItemTargets(step planner.PlanStep) []string {
	if len(step.RepoItems) > 0 {
		var names []string
		for _, r := range step.RepoItems {
			names = append(names, r.Repo.Name)
		}
		return names
	}
	var targets []string
	for _, it := range step.Items {
		targets = append(targets, it.Target)
	}
	return targets
}

func executeRepoStep(ctx context.Context, j *Journal, step planner.PlanStep, opts ExecOptions, runner adapter.CommandRunner, nowFunc func() time.Time, res *Result) error {
	stagingDir := filepath.Join(os.TempDir(), fmt.Sprintf("xpose-staging-%s", j.RunID))
	_ = os.MkdirAll(stagingDir, 0700)
	defer os.RemoveAll(stagingDir)

	var hasError bool

	for _, repoItem := range step.RepoItems {
		res.Summary.TotalItems++
		repo := repoItem.Repo

		// Check third-party confirmation
		if repo.ThirdParty && opts.Confirmer != nil {
			confirmed, err := opts.Confirmer.ConfirmThirdPartyRepo(repo)
			if err != nil || !confirmed {
				entry := Entry{
					Timestamp: nowFunc().UTC(),
					StepOrder: step.Order,
					Manager:   "repos",
					Action:    repoItem.Action,
					Command:   fmt.Sprintf("restore repo %s", repo.Name),
					Items:     []string{repo.Name},
					Status:    "skipped",
					Error:     "third-party repo confirmation rejected",
				}
				_ = j.Append(entry)
				res.Summary.SkippedItems++
				continue
			}
		}

		// Stage file content
		destPath := repoItem.TargetPath
		if destPath == "" {
			if repo.Kind == "rpm" {
				destPath = filepath.Join("/etc/yum.repos.d", repo.FileName)
			} else {
				destPath = filepath.Join("/etc/apt/sources.list.d", repo.FileName)
			}
		}

		stagingFile := filepath.Join(stagingDir, filepath.Base(destPath))
		if err := os.WriteFile(stagingFile, []byte(repo.Content), 0644); err != nil {
			entry := Entry{
				Timestamp: nowFunc().UTC(),
				StepOrder: step.Order,
				Manager:   "repos",
				Action:    repoItem.Action,
				Command:   fmt.Sprintf("write staging %s", stagingFile),
				Items:     []string{repo.Name},
				Status:    "failed",
				Error:     err.Error(),
			}
			_ = j.Append(entry)
			res.Summary.FailedItems++
			res.FailedItems = append(res.FailedItems, fmt.Sprintf("%s (repos): %v", repo.Name, err))
			hasError = true
			continue
		}

		cmdArgs := []string{"cp", stagingFile, destPath}
		displayCmd := fmt.Sprintf("sudo cp %s %s", stagingFile, destPath)

		// 1. Log pending
		entry := Entry{
			Timestamp: nowFunc().UTC(),
			StepOrder: step.Order,
			Manager:   "repos",
			Action:    repoItem.Action,
			Command:   displayCmd,
			Items:     []string{repo.Name},
			Status:    "pending",
		}
		_ = j.Append(entry)

		start := nowFunc()
		var out []byte
		var runErr error
		if repoItem.RequiresSudo {
			out, runErr = runner("sudo", cmdArgs...)
		} else {
			out, runErr = runner(cmdArgs[0], cmdArgs[1:]...)
		}
		duration := nowFunc().Sub(start).Milliseconds()

		status := "success"
		errStr := ""
		exitCode := 0
		if runErr != nil {
			status = "failed"
			errStr = runErr.Error()
			exitCode = 1
			hasError = true
			res.Summary.FailedItems++
			res.FailedItems = append(res.FailedItems, fmt.Sprintf("%s (repos): %v", repo.Name, runErr))
		} else {
			res.Summary.SuccessItems++
			res.Summary.ReposRestored++
		}

		// 2. Log result
		resultEntry := Entry{
			Timestamp:  nowFunc().UTC(),
			StepOrder:  step.Order,
			Manager:    "repos",
			Action:     repoItem.Action,
			Command:    displayCmd,
			Items:      []string{repo.Name},
			Status:     status,
			DurationMs: duration,
			ExitCode:   exitCode,
			Stdout:     string(out),
			Error:      errStr,
		}
		_ = j.Append(resultEntry)
	}

	if hasError {
		return errors.New("one or more repo restorations failed")
	}
	return nil
}

func executePackageStep(ctx context.Context, j *Journal, step planner.PlanStep, opts ExecOptions, runner adapter.CommandRunner, nowFunc func() time.Time, res *Result) error {
	var targets []string
	for _, it := range step.Items {
		targets = append(targets, it.Target)
	}

	if len(targets) == 0 {
		return nil
	}

	res.Summary.TotalItems += len(targets)

	// Attempt batch first
	batchCmd, batchArgs := buildInstallCommand(step.Manager, targets, opts.Yes)
	displayCmd := formatCommandLine(batchCmd, batchArgs)

	// Log batch pending
	batchPending := Entry{
		Timestamp: nowFunc().UTC(),
		StepOrder: step.Order,
		Manager:   step.Manager,
		Action:    step.Action,
		Command:   displayCmd,
		Items:     targets,
		Status:    "pending",
	}
	_ = j.Append(batchPending)

	start := nowFunc()
	out, err := runner(batchCmd, batchArgs...)
	duration := nowFunc().Sub(start).Milliseconds()

	if err == nil {
		// Batch succeeded
		batchSuccess := Entry{
			Timestamp:  nowFunc().UTC(),
			StepOrder:  step.Order,
			Manager:    step.Manager,
			Action:     step.Action,
			Command:    displayCmd,
			Items:      targets,
			Status:     "success",
			DurationMs: duration,
			ExitCode:   0,
			Stdout:     string(out),
		}
		_ = j.Append(batchSuccess)
		res.Summary.SuccessItems += len(targets)
		return nil
	}

	// Batch failed: record batch failure and fallback to single items
	batchFailed := Entry{
		Timestamp:  nowFunc().UTC(),
		StepOrder:  step.Order,
		Manager:    step.Manager,
		Action:     step.Action,
		Command:    displayCmd,
		Items:      targets,
		Status:     "failed",
		DurationMs: duration,
		ExitCode:   1,
		Stderr:     string(out),
		Error:      fmt.Sprintf("batch failed: %v; falling back to individual items", err),
	}
	_ = j.Append(batchFailed)

	var hasItemError bool
	for _, target := range targets {
		singleCmd, singleArgs := buildInstallCommand(step.Manager, []string{target}, opts.Yes)
		singleDisplay := formatCommandLine(singleCmd, singleArgs)

		// Log single pending
		singlePending := Entry{
			Timestamp: nowFunc().UTC(),
			StepOrder: step.Order,
			Manager:   step.Manager,
			Action:    step.Action,
			Command:   singleDisplay,
			Items:     []string{target},
			Status:    "pending",
		}
		_ = j.Append(singlePending)

		itemStart := nowFunc()
		itemOut, itemErr := runner(singleCmd, singleArgs...)
		itemDur := nowFunc().Sub(itemStart).Milliseconds()

		if itemErr == nil {
			itemSuccess := Entry{
				Timestamp:  nowFunc().UTC(),
				StepOrder:  step.Order,
				Manager:    step.Manager,
				Action:     step.Action,
				Command:    singleDisplay,
				Items:      []string{target},
				Status:     "success",
				DurationMs: itemDur,
				ExitCode:   0,
				Stdout:     string(itemOut),
			}
			_ = j.Append(itemSuccess)
			res.Summary.SuccessItems++
		} else {
			hasItemError = true
			res.Summary.FailedItems++
			res.FailedItems = append(res.FailedItems, fmt.Sprintf("%s (%s): %v", target, step.Manager, itemErr))
			itemFail := Entry{
				Timestamp:  nowFunc().UTC(),
				StepOrder:  step.Order,
				Manager:    step.Manager,
				Action:     step.Action,
				Command:    singleDisplay,
				Items:      []string{target},
				Status:     "failed",
				DurationMs: itemDur,
				ExitCode:   1,
				Stderr:     string(itemOut),
				Error:      itemErr.Error(),
			}
			_ = j.Append(itemFail)
		}
	}

	if hasItemError {
		return errors.New("one or more individual packages failed")
	}
	return nil
}

func buildInstallCommand(manager string, targets []string, assumeYes bool) (string, []string) {
	switch strings.ToLower(manager) {
	case "dnf":
		args := []string{"dnf", "install"}
		if assumeYes {
			args = append(args, "-y")
		}
		args = append(args, targets...)
		return "sudo", args
	case "apt":
		args := []string{"apt-get", "install"}
		if assumeYes {
			args = append(args, "-y")
		}
		args = append(args, targets...)
		return "sudo", args
	case "flatpak":
		args := []string{"install"}
		if assumeYes {
			args = append(args, "--noninteractive")
		}
		args = append(args, targets...)
		return "flatpak", args
	default:
		return manager, targets
	}
}

func formatCommandLine(cmd string, args []string) string {
	parts := append([]string{cmd}, args...)
	return strings.Join(parts, " ")
}
