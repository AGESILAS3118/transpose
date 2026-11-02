package executor

import (
	"errors"
	"fmt"
	"strings"

	"github.com/transpose-project/transpose/internal/planner"
	"github.com/transpose-project/transpose/internal/sysutil"
)

var forbiddenSubcommands = []string{
	"remove", "erase", "autoremove", "purge", "uninstall",
	"-R", "-Rs", "--uninstall", "--remove", "--orphans",
	"clean", "cleanup",
}

var allowedManagers = map[string]bool{
	"repos":   true,
	"dnf":     true,
	"apt":     true,
	"flatpak": true,
	"snap":    true,
}

// isForbiddenToken checks if a token or option matches any forbidden string or prefix/suffix.
func isForbiddenToken(tok string) bool {
	tok = strings.TrimSpace(tok)
	lower := strings.ToLower(tok)

	// Exact check
	for _, forbidden := range forbiddenSubcommands {
		if lower == strings.ToLower(forbidden) {
			return true
		}
	}

	// Check if flag matches e.g. --autoremove, --remove, -R
	trimmed := strings.TrimLeft(lower, "-")
	for _, forbidden := range forbiddenSubcommands {
		forbiddenTrimmed := strings.TrimLeft(strings.ToLower(forbidden), "-")
		if trimmed == forbiddenTrimmed {
			return true
		}
	}

	return false
}

// ValidatePlanSafety verifies that the plan does not contain destructive actions,
// unsafe arguments, or prohibited commands.
func ValidatePlanSafety(p *planner.Plan) error {
	if p == nil {
		return errors.New("plan is nil")
	}

	for _, step := range p.Steps {
		// Verify manager is recognized
		mgr := strings.TrimSpace(strings.ToLower(step.Manager))
		if !allowedManagers[mgr] {
			return fmt.Errorf("unrecognized or prohibited manager %q in step %d", step.Manager, step.Order)
		}

		// Check step action
		if isForbiddenToken(step.Action) {
			return fmt.Errorf("destructive action %q forbidden in step %d", step.Action, step.Order)
		}

		// Check command line tokens
		if step.Command != "" {
			tokens := strings.Fields(step.Command)
			for _, tok := range tokens {
				if isForbiddenToken(tok) {
					return fmt.Errorf("destructive token %q forbidden in command %q (step %d)", tok, step.Command, step.Order)
				}
			}
		}

		// Check plan items target names
		for _, item := range step.Items {
			if isForbiddenToken(item.Target) || isForbiddenToken(item.Source) {
				return fmt.Errorf("destructive token found in item %q -> %q (step %d)", item.Source, item.Target, step.Order)
			}
		}

		// Check repo plan items
		for _, repoItem := range step.RepoItems {
			if isForbiddenToken(repoItem.Action) {
				return fmt.Errorf("destructive action %q in repo item %s (step %d)", repoItem.Action, repoItem.Repo.Name, step.Order)
			}
		}
	}

	return nil
}

// ValidateSameFamily verifies that target.Family matches source distro family.
func ValidateSameFamily(p *planner.Plan) error {
	if p == nil {
		return errors.New("plan is nil")
	}

	sourceFamily := sysutil.FamilyForDistro(p.Source.Distro, "")
	if sourceFamily == "" {
		return fmt.Errorf("unknown source family for distro %q", p.Source.Distro)
	}

	sourceTarget := sysutil.Target{Family: sourceFamily, Distro: p.Source.Distro}
	if !sysutil.SameFamily(sourceTarget, p.Target) {
		return fmt.Errorf("cross-family plan not supported (%s -> %s): MVP v0.1 only supports same-family migrations", p.Source.Distro, p.Target.Distro)
	}

	return nil
}
