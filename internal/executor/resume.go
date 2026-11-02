package executor

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/transpose-project/transpose/internal/adapter"
	"github.com/transpose-project/transpose/internal/planner"
)

// FindLatestJournal looks for the most recent journal file in the given directory or standard path.
func FindLatestJournal(dir string) (string, error) {
	if dir == "" {
		var err error
		dir, err = DefaultJournalDir()
		if err != nil {
			return "", err
		}
	}

	matches, err := filepath.Glob(filepath.Join(dir, "*.jsonl"))
	if err != nil {
		return "", fmt.Errorf("failed to search journal directory: %w", err)
	}

	if len(matches) == 0 {
		return "", errors.New("no journal files found")
	}

	sort.Strings(matches)
	return matches[len(matches)-1], nil
}

// Resume reads a journal file, skips items that already succeeded, and re-executes
// any items that were pending, running, or failed.
func Resume(ctx context.Context, journalPath string, p *planner.Plan, opts ExecOptions) (*Result, error) {
	if journalPath == "" {
		latest, err := FindLatestJournal(opts.JournalDir)
		if err != nil {
			return nil, fmt.Errorf("cannot find journal to resume: %w", err)
		}
		journalPath = latest
	}

	if _, err := os.Stat(journalPath); err != nil {
		return nil, fmt.Errorf("journal file not found: %w", err)
	}

	j, err := OpenJournal(journalPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open journal: %w", err)
	}

	entries, err := j.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to read journal: %w", err)
	}

	if len(entries) == 0 {
		return nil, errors.New("journal is empty, nothing to resume")
	}

	// Set journal RunID from first entry
	j.RunID = entries[0].RunID

	// Map of successful items (manager:target -> true)
	successItems := make(map[string]bool)
	successRepos := make(map[string]bool)

	for _, e := range entries {
		if e.Status == "success" {
			if strings.ToLower(e.Manager) == "repos" {
				for _, it := range e.Items {
					successRepos[it] = true
				}
			} else {
				for _, it := range e.Items {
					key := fmt.Sprintf("%s:%s", strings.ToLower(e.Manager), it)
					successItems[key] = true
				}
			}
		}
	}

	// Filter plan to retain only items that have NOT yet succeeded
	resumedPlan := filterPlanForResume(p, successItems, successRepos)

	// Check if there is anything left to execute
	var remainingCount int
	for _, step := range resumedPlan.Steps {
		remainingCount += len(step.Items) + len(step.RepoItems)
	}

	res := &Result{
		Plan:        resumedPlan,
		JournalPath: j.Path,
		RunID:       j.RunID,
	}

	if remainingCount == 0 {
		return res, nil
	}

	runner := opts.Runner
	if runner == nil {
		runner = adapter.ExecCommandRunner
	}

	nowFunc := opts.Now
	if nowFunc == nil {
		nowFunc = time.Now
	}

	res.Summary.TotalSteps = len(resumedPlan.Steps)

	for _, step := range resumedPlan.Steps {
		select {
		case <-ctx.Done():
			return res, ctx.Err()
		default:
		}

		mgr := strings.ToLower(strings.TrimSpace(step.Manager))
		if !opts.Apply {
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
			itemCount := len(step.Items) + len(step.RepoItems)
			res.Summary.DryRunItems += itemCount
			res.Summary.TotalItems += itemCount
			continue
		}

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
		return res, fmt.Errorf("resume completed with %d failed item(s)", res.Summary.FailedItems)
	}

	return res, nil
}

func filterPlanForResume(p *planner.Plan, successItems map[string]bool, successRepos map[string]bool) *planner.Plan {
	if p == nil {
		return nil
	}

	filtered := &planner.Plan{
		Source:      p.Source,
		Target:      p.Target,
		GeneratedAt: p.GeneratedAt,
		Skipped:     p.Skipped,
		Warnings:    p.Warnings,
	}

	for _, step := range p.Steps {
		stepCopy := step
		mgr := strings.ToLower(step.Manager)

		if mgr == "repos" {
			var remainingRepos []planner.RepoPlanItem
			for _, r := range step.RepoItems {
				if !successRepos[r.Repo.Name] {
					remainingRepos = append(remainingRepos, r)
				}
			}
			stepCopy.RepoItems = remainingRepos
			if len(remainingRepos) > 0 {
				filtered.Steps = append(filtered.Steps, stepCopy)
			}
		} else {
			var remainingItems []planner.PlanItem
			for _, item := range step.Items {
				key := fmt.Sprintf("%s:%s", mgr, item.Target)
				if !successItems[key] {
					remainingItems = append(remainingItems, item)
				}
			}
			stepCopy.Items = remainingItems
			if len(remainingItems) > 0 {
				filtered.Steps = append(filtered.Steps, stepCopy)
			}
		}
	}

	return filtered
}
