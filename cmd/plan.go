package cmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/planner"
	"github.com/transpose-project/transpose/internal/resolver"
	"github.com/transpose-project/transpose/internal/sysutil"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func newPlanCmd() *cobra.Command {
	return newPlanCmdWithQuerierFactory(func(target sysutil.Target) (resolver.RepoQuerier, error) {
		switch target.Family {
		case "rpm":
			return commandRepoQuerier{command: "dnf", args: []string{"repoquery", "--available", "--qf", "%{name}\n"}}, nil
		case "debian":
			return commandRepoQuerier{command: "apt-cache", args: []string{"pkgnames"}}, nil
		default:
			return nil, fmt.Errorf("requête des dépôts non implémentée pour la famille %q", target.Family)
		}
	})
}

type repoQuerierFactory func(sysutil.Target) (resolver.RepoQuerier, error)

func newPlanCmdWithQuerierFactory(factory repoQuerierFactory) *cobra.Command {
	command := &cobra.Command{
		Use:   "plan <manifest>",
		Short: "Preview the changes needed to restore a manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			source, err := readPlanManifest(args[0])
			if err != nil {
				return err
			}
			targetValue, err := cmd.Flags().GetString("target")
			if err != nil {
				return err
			}
			var target *sysutil.Target
			if targetValue == "" {
				target, err = sysutil.DetectCurrent()
			} else {
				target, err = sysutil.ParseTarget(targetValue)
			}
			if err != nil {
				return fmt.Errorf("détection de la cible impossible: %w", err)
			}
			if err := validatePlanFamily(source, *target); err != nil {
				return err
			}
			format, err := cmd.Flags().GetString("format")
			if err != nil {
				return err
			}
			if format != "terminal" && format != "markdown" && format != "json" {
				return fmt.Errorf("format inconnu %q (attendu terminal, markdown ou json)", format)
			}
			verbose, err := cmd.Flags().GetBool("verbose")
			if err != nil {
				return err
			}
			overridesPath, err := cmd.Flags().GetString("overrides")
			if err != nil {
				return err
			}
			overrides := &resolver.Overrides{}
			if overridesPath != "" {
				overrides, err = resolver.LoadOverrides(overridesPath)
				if err != nil {
					return err
				}
			}
			querier, err := factory(*target)
			if err != nil {
				return err
			}
			only, err := cmd.Flags().GetStringSlice("only")
			if err != nil {
				return err
			}
			excluded, err := cmd.Flags().GetStringSlice("exclude")
			if err != nil {
				return err
			}
			plan, err := planner.GeneratePlan(source, planner.PlanOptions{
				Target:   *target,
				Only:     only,
				Exclude:  excluded,
				Resolver: resolver.NewSameFamilyResolver(querier, overrides),
			})
			if err != nil {
				return err
			}
			render := func(writer io.Writer) error {
				switch format {
				case "terminal":
					return planner.RenderTerminal(plan, writer, verbose)
				case "markdown":
					return planner.RenderMarkdown(plan, writer)
				case "json":
					return planner.RenderJSON(plan, writer)
				}
				return fmt.Errorf("format inconnu %q", format)
			}
			outputPath, err := cmd.Flags().GetString("output")
			if err != nil {
				return err
			}
			if outputPath != "" {
				return writeResolveFile(outputPath, render)
			}
			return render(cmd.OutOrStdout())
		},
	}
	command.Flags().String("target", "", "Target distribution (for example fedora-44 or debian_13)")
	command.Flags().String("format", "terminal", "Plan format (terminal|markdown|json)")
	command.Flags().StringP("output", "o", "", "Write the plan to a file instead of stdout")
	command.Flags().Bool("verbose", false, "Show every package in terminal output")
	command.Flags().String("overrides", "", "Load package name overrides from YAML")
	return command
}

type commandRepoQuerier struct {
	command string
	args    []string
}

func (q commandRepoQuerier) ListAvailable(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, q.command, q.args...)
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s %s failed: %w", q.command, strings.Join(q.args, " "), err)
	}
	lines := strings.Split(string(output), "\n")
	names := make([]string, 0, len(lines))
	for _, line := range lines {
		if name := strings.TrimSpace(line); name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}

func readPlanManifest(path string) (*manifest.Manifest, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("ouverture du manifeste %s: %w", path, err)
	}
	defer file.Close()
	switch strings.ToLower(filepath.Ext(path)) {
	case ".yaml", ".yml":
		return manifest.ReadYAML(file)
	case ".json":
		return manifest.ReadJSON(file)
	default:
		return nil, fmt.Errorf("extension de manifeste non prise en charge %q (attendu .yaml, .yml ou .json)", filepath.Ext(path))
	}
}

func validatePlanFamily(source *manifest.Manifest, target sysutil.Target) error {
	if source == nil {
		return fmt.Errorf("manifeste requis")
	}
	sourceFamily := sysutil.FamilyForDistro(source.Source.Distro, "")
	if sourceFamily == "" {
		return fmt.Errorf("famille source inconnue pour %q", source.Source.Distro)
	}
	if sourceFamily != target.Family {
		return fmt.Errorf("cross-family non supporté (%s → %s) ; requiert F8 (base Repology en CI). Voir docs/PLAN-PHASE-1.md",
			source.Source.Distro, target.Distro)
	}
	return nil
}
