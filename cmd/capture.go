package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/adapter"
	"github.com/transpose-project/transpose/internal/adapter/flatpak"
)

func newCaptureCmd() *cobra.Command {
	return newCaptureCmdWithAdapters(allAdapters())
}

func newCaptureCmdWithAdapters(adapters []adapter.Adapter) *cobra.Command {
	return &cobra.Command{
		Use:   "capture",
		Short: "Capture installed packages using resilient adapters",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			selected, err := cmd.Flags().GetStringSlice("only")
			if err != nil {
				return err
			}
			selectedSet := make(map[string]struct{}, len(selected))
			for _, name := range selected {
				selectedSet[strings.TrimSpace(name)] = struct{}{}
			}
			known := make(map[string]bool, len(adapters))
			for _, pkgAdapter := range adapters {
				known[pkgAdapter.Name()] = true
			}
			for name := range selectedSet {
				if !known[name] {
					return fmt.Errorf("gestionnaire inconnu %q (disponibles: dnf, flatpak, apt)", name)
				}
			}

			results := make([]*adapter.CaptureResult, 0, len(adapters))
			for _, pkgAdapter := range adapters {
				name := pkgAdapter.Name()
				_, isSelected := selectedSet[name]
				if len(selectedSet) > 0 && !isSelected {
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
			if len(results) == 0 {
				return errors.New("aucun adaptateur de capture disponible")
			}

			format, err := cmd.Flags().GetString("format")
			if err != nil {
				return err
			}
			if format == "json" {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(results)
			}
			return writeCaptureSummary(cmd.OutOrStdout(), results)
		},
	}
}

func pkgAdapterError(name, reason string) error {
	return fmt.Errorf("capture %s impossible : %s", name, reason)
}

func writeCaptureSummary(output io.Writer, results []*adapter.CaptureResult) error {
	for _, result := range results {
		unit := "paquets"
		details := fmt.Sprintf("%d %s", len(result.Packages), unit)
		if result.Manager == "flatpak" {
			details = fmt.Sprintf("%d apps, %d remotes", len(result.Packages), countFlatpakRemotes(result))
		}
		if _, err := fmt.Fprintf(output, "%s — niveau %d (reliability: %s) — %s\n",
			result.Manager, result.Level, result.Reliability, details); err != nil {
			return err
		}
		for _, warning := range result.Warnings {
			if _, err := fmt.Fprintf(output, "  ⚠ %s\n", warning); err != nil {
				return err
			}
		}
	}
	return nil
}

func countFlatpakRemotes(result *adapter.CaptureResult) int {
	raw, ok := result.Raw["remotes"].(map[string][]flatpak.Remote)
	if !ok {
		return 0
	}
	names := make(map[string]struct{})
	for _, remotes := range raw {
		for _, remote := range remotes {
			names[remote.Name] = struct{}{}
		}
	}
	return len(names)
}
