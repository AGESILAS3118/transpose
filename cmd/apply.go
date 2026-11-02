package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/adapter"
	"github.com/transpose-project/transpose/internal/archive"
	"github.com/transpose-project/transpose/internal/executor"
	"github.com/transpose-project/transpose/internal/planner"
	"github.com/transpose-project/transpose/internal/resolver"
	"github.com/transpose-project/transpose/internal/sysutil"
	"github.com/transpose-project/transpose/pkg/manifest"
	"golang.org/x/term"
	"gopkg.in/yaml.v3"
)

type interactiveConfirmer struct {
	in  io.Reader
	out io.Writer
}

func (c *interactiveConfirmer) Confirm(prompt string) (bool, error) {
	fmt.Fprintf(c.out, "%s [y/N]: ", prompt)
	reader := bufio.NewReader(c.in)
	response, err := reader.ReadString(byte(10))
	if err != nil {
		return false, err
	}
	resp := strings.TrimSpace(strings.ToLower(response))
	return resp == "y" || resp == "yes" || resp == "o" || resp == "oui", nil
}

func (c *interactiveConfirmer) ConfirmThirdPartyRepo(repo manifest.Repo) (bool, error) {
	prompt := fmt.Sprintf("Restaurer le dépôt tiers %q (%s) ?", repo.Name, repo.FileName)
	return c.Confirm(prompt)
}

func newApplyCmd() *cobra.Command {
	return newApplyCmdWithRunner(nil, nil)
}

func newApplyCmdWithRunner(runner adapter.CommandRunner, customConfirmer executor.Confirmer) *cobra.Command {
	var (
		applyFlag      bool
		yesFlag        bool
		resumeFlag     bool
		journalDirFlag string
		onlyFlag       []string
		excludeFlag    []string
		verboseFlag    bool
		formatFlag     string
		outputFlag     string
	)

	cmd := &cobra.Command{
		Use:   "apply <plan.yaml|plan.json|manifest.yaml>",
		Short: "Apply a validated migration plan",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filePath := args[0]
			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}

			// Load Plan directly, or generate Plan from Manifest
			plan, err := loadPlanOrManifest(filePath, cmd, onlyFlag, excludeFlag)
			if err != nil {
				return err
			}

			// Setup Confirmer
			confirmer := customConfirmer
			if confirmer == nil {
				confirmer = &interactiveConfirmer{in: cmd.InOrStdin(), out: cmd.OutOrStdout()}
			}

			// Detect TTY (or injected test confirmer)
			isTTY := term.IsTerminal(int(os.Stdin.Fd())) || customConfirmer != nil
			interactive := isTTY && !yesFlag

			// Setup runner if not provided (tests may provide a mock runner)
			execRunner := runner
			if execRunner == nil {
				if interactive {
					execRunner = adapter.ExecCommandRunnerInteractive
				} else {
					execRunner = adapter.ExecCommandRunner
				}
			}

			// Interactive confirmation if --apply is set without --yes
			if applyFlag && !yesFlag {
				if !isTTY {
					return errors.New("mode réel sans TTY : utilisez --yes ou attachez un TTY")
				}
				confirmed, err := confirmer.Confirm("Appliquer ce plan d'installation sur le système cible ?")
				if err != nil {
					return err
				}
				if !confirmed {
					fmt.Fprintln(cmd.OutOrStdout(), "Exécution annulée par l'utilisateur.")
					return nil
				}
			}

			opts := executor.ExecOptions{
				Apply:       applyFlag,
				Yes:         yesFlag,
				Interactive: interactive,
				JournalDir:  journalDirFlag,
				Resume:      resumeFlag,
				Verbose:     verboseFlag,
				Runner:      execRunner,
				Confirmer:   confirmer,
			}

			startTime := time.Now()
			var res *executor.Result

			if resumeFlag {
				res, err = executor.Resume(ctx, "", plan, opts)
			} else {
				res, err = executor.Execute(ctx, plan, opts)
			}

			duration := time.Since(startTime)

			// Format output
			var renderErr error
			if formatFlag == "json" {
				renderErr = renderApplyJSON(cmd.OutOrStdout(), res, duration, applyFlag, outputFlag)
			} else {
				renderErr = renderApplyTerminal(cmd.OutOrStdout(), res, duration, applyFlag, outputFlag)
			}
			if renderErr != nil {
				return renderErr
			}

			if err != nil {
				return err
			}
			return nil
		},
	}

	cmd.Flags().BoolVar(&applyFlag, "apply", false, "Actually apply changes to the system (default is dry-run)")
	cmd.Flags().BoolVarP(&yesFlag, "yes", "y", false, "Automatically confirm global application without interactive prompt")
	cmd.Flags().BoolVar(&resumeFlag, "resume", false, "Resume execution from the most recent journal")
	cmd.Flags().StringVar(&journalDirFlag, "journal-dir", "", "Override journal directory (default: ~/.local/state/xpose/runs/)")
	cmd.Flags().StringSliceVar(&onlyFlag, "only", nil, "Only execute matching components (e.g. dnf,flatpak)")
	cmd.Flags().StringSliceVar(&excludeFlag, "exclude", nil, "Exclude specific packages or components")
	cmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show verbose execution details")
	cmd.Flags().StringVar(&formatFlag, "format", "terminal", "Output format (terminal|json)")
	cmd.Flags().StringVarP(&outputFlag, "output", "o", "", "Write execution summary to a file")

	return cmd
}

func loadPlanOrManifest(filePath string, cmd *cobra.Command, only, exclude []string) (*planner.Plan, error) {
	targetFile := filePath
	lower := strings.ToLower(filePath)
	isArchive := strings.HasSuffix(lower, ".tar.zst") ||
		strings.HasSuffix(lower, ".tzst") ||
		strings.HasSuffix(lower, ".tar.zstd")

	if isArchive {
		tmpDir, err := os.MkdirTemp("", "xpose-unpack-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmpDir)

		unpackRes, err := archive.Unpack(archive.UnpackOptions{
			ArchivePath: filePath,
			TargetDir:   tmpDir,
		})
		if err != nil {
			return nil, fmt.Errorf("extraction de l'archive impossible: %w", err)
		}
		targetFile = unpackRes.ManifestPath
	}

	data, err := os.ReadFile(targetFile)
	if err != nil {
		return nil, fmt.Errorf("impossible de lire %s: %w", targetFile, err)
	}

	// Try unmarshaling as Plan first
	var p planner.Plan
	if errYaml := yaml.Unmarshal(data, &p); errYaml == nil && len(p.Steps) > 0 {
		return &p, nil
	}
	if errJSON := json.Unmarshal(data, &p); errJSON == nil && len(p.Steps) > 0 {
		return &p, nil
	}

	// Fallback: try as Manifest and generate same-family plan
	var m manifest.Manifest
	if errYaml := yaml.Unmarshal(data, &m); errYaml == nil && m.Source.Distro != "" {
		targetPtr, err := sysutil.DetectCurrent()
		var target sysutil.Target
		if err == nil && targetPtr != nil {
			target = *targetPtr
		} else {
			target = sysutil.Target{Distro: m.Source.Distro, Family: sysutil.FamilyForDistro(m.Source.Distro, "")}
		}
		if err != nil {
			target = sysutil.Target{Distro: m.Source.Distro, Family: sysutil.FamilyForDistro(m.Source.Distro, "")}
		}
		opts := planner.PlanOptions{
			Target:   target,
			Only:     only,
			Exclude:  exclude,
			Resolver: emptySameFamilyResolver{},
		}
		return planner.GeneratePlan(&m, opts)
	}

	return nil, fmt.Errorf("le fichier %s n'est ni un plan valide ni un manifeste exploitable", filePath)
}

type emptySameFamilyResolver struct{}

func (e emptySameFamilyResolver) Resolve(pkg manifest.Package, target sysutil.Target) (resolver.Resolution, error) {
	return resolver.Resolution{
		Source:     pkg,
		Status:     resolver.StatusExact,
		Target:     pkg.Name,
		Manager:    pkg.Manager,
		Confidence: "high",
	}, nil
}

func renderApplyTerminal(w io.Writer, res *executor.Result, dur time.Duration, isApply bool, outFile string) error {
	var sb strings.Builder

	mode := "Mode Dry-run (simulation)"
	if isApply {
		mode = "Mode Réel (--apply)"
	}

	runID := "n/a"
	journalPath := "n/a"
	if res != nil {
		runID = res.RunID
		journalPath = res.JournalPath
	}

	fmt.Fprintf(&sb, "Exécution %s — Run ID %s\n", time.Now().UTC().Format(time.RFC3339), runID)
	fmt.Fprintf(&sb, "  Mode     : %s\n", mode)
	if res != nil {
		fmt.Fprintf(&sb, "  Étapes   : %d\n", res.Summary.TotalSteps)
		if isApply {
			fmt.Fprintf(&sb, "  Succès   : %d items\n", res.Summary.SuccessItems)
			fmt.Fprintf(&sb, "  Échecs   : %d items\n", res.Summary.FailedItems)
			fmt.Fprintf(&sb, "  Repos    : %d restaurés\n", res.Summary.ReposRestored)
		} else {
			fmt.Fprintf(&sb, "  Dry-run  : %d items simulés\n", res.Summary.DryRunItems)
		}
	}
	fmt.Fprintf(&sb, "  Durée    : %s\n", dur.Round(time.Millisecond))
	fmt.Fprintf(&sb, "  Journal  : %s\n", journalPath)

	if res != nil && len(res.FailedItems) > 0 {
		fmt.Fprintf(&sb, "\n%d items en échec :\n", len(res.FailedItems))
		for _, f := range res.FailedItems {
			fmt.Fprintf(&sb, "  - %s\n", f)
		}
		fmt.Fprintln(&sb, "Relance `xpose apply --resume` pour réessayer.")
	}

	outStr := sb.String()
	if outFile != "" {
		if err := os.WriteFile(outFile, []byte(outStr), 0644); err != nil {
			return fmt.Errorf("impossible d'écrire le résumé dans %s: %w", outFile, err)
		}
	}

	_, err := fmt.Fprint(w, outStr)
	return err
}

func renderApplyJSON(w io.Writer, res *executor.Result, dur time.Duration, isApply bool, outFile string) error {
	type JSONSummary struct {
		Timestamp   string                    `json:"timestamp"`
		Mode        string                    `json:"mode"`
		RunID       string                    `json:"run_id"`
		JournalPath string                    `json:"journal_path"`
		DurationMs  int64                     `json:"duration_ms"`
		Summary     executor.ExecutionSummary `json:"summary"`
		FailedItems []string                  `json:"failed_items,omitempty"`
	}

	mode := "dry-run"
	if isApply {
		mode = "apply"
	}

	runID := ""
	journalPath := ""
	var summary executor.ExecutionSummary
	var failed []string

	if res != nil {
		runID = res.RunID
		journalPath = res.JournalPath
		summary = res.Summary
		failed = res.FailedItems
	}

	payload := JSONSummary{
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		Mode:        mode,
		RunID:       runID,
		JournalPath: journalPath,
		DurationMs:  dur.Milliseconds(),
		Summary:     summary,
		FailedItems: failed,
	}

	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}

	if outFile != "" {
		if err := os.WriteFile(outFile, data, 0644); err != nil {
			return fmt.Errorf("impossible d'écrire le résumé JSON dans %s: %w", outFile, err)
		}
	}

	_, err = fmt.Fprintln(w, string(data))
	return err
}
