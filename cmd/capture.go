package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/adapter"
	repoadapter "github.com/transpose-project/transpose/internal/adapter/repos"
	"github.com/transpose-project/transpose/internal/planner"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func newCaptureCmd() *cobra.Command {
	return newCaptureCmdWithAdaptersAndRepoCollectors(allAdapters(), []repoadapter.RepoCollector{
		repoadapter.NewRPMRepoCollector(""),
		repoadapter.NewAPTRepoCollector(""),
	})
}

func newCaptureCmdWithAdapters(adapters []adapter.Adapter) *cobra.Command {
	return newCaptureCmdWithAdaptersAndRepoCollectors(adapters, nil)
}

func newCaptureCmdWithAdaptersAndRepoCollectors(adapters []adapter.Adapter, repoCollectors []repoadapter.RepoCollector) *cobra.Command {
	command := &cobra.Command{
		Use:   "capture",
		Short: "Capture installed packages and write a versioned manifest",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			selected, err := cmd.Flags().GetStringSlice("only")
			if err != nil {
				return err
			}
			excluded, err := cmd.Flags().GetStringSlice("exclude")
			if err != nil {
				return err
			}
			selectedSet := stringSet(selected)
			excludedSet := stringSet(excluded)
			known := make(map[string]bool, len(adapters))
			for _, pkgAdapter := range adapters {
				known[pkgAdapter.Name()] = true
			}
			for name := range selectedSet {
				if !known[name] {
					return fmt.Errorf("gestionnaire inconnu %q (disponibles: dnf, flatpak, apt)", name)
				}
			}
			for name := range excludedSet {
				if !known[name] {
					return fmt.Errorf("gestionnaire inconnu dans --exclude : %q", name)
				}
			}

			results := make([]*adapter.CaptureResult, 0, len(adapters))
			for _, pkgAdapter := range adapters {
				name := pkgAdapter.Name()
				_, isSelected := selectedSet[name]
				if len(selectedSet) > 0 && !isSelected {
					continue
				}
				if _, skip := excludedSet[name]; skip {
					continue
				}
				if name == "apt" {
					if isSelected {
						return pkgAdapterError(name, "capture non implémentée")
					}
					continue
				}
				present, err := pkgAdapter.Detect()
				if err != nil {
					return fmt.Errorf("détection de %s impossible: %w", name, err)
				}
				if !present {
					if isSelected {
						return pkgAdapterError(name, "absent (outils requis introuvables)")
					}
					continue
				}
				result, err := pkgAdapter.Capture()
				if err != nil {
					return fmt.Errorf("capture %s échouée: %w", name, err)
				}
				results = append(results, result)
			}

			format, err := cmd.Flags().GetString("format")
			if err != nil {
				return err
			}
			if format != "yaml" && format != "json" && format != "raw" {
				return fmt.Errorf("format inconnu %q (attendu yaml, json ou raw)", format)
			}
			for _, result := range results {
				for _, warning := range result.Warnings {
					if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s\n", warning); err != nil {
						return err
					}
				}
			}
			var content bytes.Buffer
			if format == "raw" {
				encoder := json.NewEncoder(&content)
				encoder.SetIndent("", "  ")
				if err := encoder.Encode(results); err != nil {
					return err
				}
			} else {
				noRepos, err := cmd.Flags().GetBool("no-repos")
				if err != nil {
					return err
				}
				collectors := repoCollectors
				if noRepos {
					collectors = nil
				}
				result, repoWarnings, buildErr := planner.BuildManifestWithCollectors(results, collectors)
				if buildErr != nil {
					return fmt.Errorf("construction du manifeste impossible: %w", buildErr)
				}
				for _, warning := range repoWarnings {
					if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s\n", warning); err != nil {
						return err
					}
				}
				validationWarnings, err := manifest.ValidateWithWarnings(result)
				if err != nil {
					return fmt.Errorf("manifeste invalide: %w", err)
				}
				for _, warning := range validationWarnings {
					if _, err := fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s\n", warning); err != nil {
						return err
					}
				}
				if format == "json" {
					if err := manifest.WriteJSON(result, &content); err != nil {
						return err
					}
				} else if err := manifest.WriteYAML(result, &content); err != nil {
					return err
				}
			}
			output, closeOutput, err := captureOutput(cmd)
			if err != nil {
				return err
			}
			_, writeErr := io.Copy(output, &content)
			return errors.Join(writeErr, closeOutput())
		},
	}
	command.Flags().StringP("output", "o", "", "Write manifest to a file instead of stdout")
	command.Flags().Bool("no-repos", false, "Skip native repository collection")
	return command
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func captureOutput(cmd *cobra.Command) (io.Writer, func() error, error) {
	path, err := cmd.Flags().GetString("output")
	if err != nil {
		return nil, func() error { return nil }, err
	}
	if path == "" {
		return cmd.OutOrStdout(), func() error { return nil }, nil
	}
	parent := filepath.Dir(path)
	if parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return nil, func() error { return nil }, fmt.Errorf("création du dossier %s: %w", parent, err)
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return nil, func() error { return nil }, fmt.Errorf("création du manifeste %s: %w", path, err)
	}
	return file, func() error {
		if err := file.Close(); err != nil {
			return fmt.Errorf("fermeture du manifeste %s: %w", path, err)
		}
		return nil
	}, nil
}

func pkgAdapterError(name, reason string) error {
	return fmt.Errorf("capture %s impossible : %s", name, reason)
}
