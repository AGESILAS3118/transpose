package cmd

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/resolver"
	"github.com/transpose-project/transpose/internal/resolver/repology"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func newResolveCmd() *cobra.Command {
	dbPath, _, err := repologyPaths()
	command := newResolveCmdWithDBPath(dbPath)
	if err != nil {
		command.PreRunE = func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("chemin de base Repology indisponible: %w", err)
		}
	}
	return command
}

func newResolveCmdWithDBPath(dbPath string) *cobra.Command {
	command := &cobra.Command{
		Use:   "resolve",
		Short: "Generate a CSV sample of Repology candidates",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			explain, err := cmd.Flags().GetString("explain")
			if err != nil {
				return err
			}
			manifestPath, err := cmd.Flags().GetString("from-manifest")
			if err != nil {
				return err
			}
			if manifestPath == "" && explain == "" {
				return fmt.Errorf("--from-manifest est obligatoire")
			}
			targetRepo, err := cmd.Flags().GetString("target")
			if err != nil {
				return err
			}
			if explain != "" {
				if dbPath == "" {
					return fmt.Errorf("chemin de base Repology indisponible")
				}
				db, err := repology.OpenDB(dbPath)
				if err != nil {
					return err
				}
				defer db.Close()
				resolutions, err := resolver.ResolveManifest(cmd.Context(), db, &manifest.Manifest{
					Packages: []manifest.Package{{Name: explain, Manager: "dnf"}},
				}, targetRepo, 1)
				if err != nil {
					return err
				}
				if len(resolutions) != 1 {
					return fmt.Errorf("aucun diagnostic produit pour %q", explain)
				}
				result := resolutions[0]
				fmt.Fprintf(cmd.OutOrStdout(), "%s\n  → project_name candidats : [%s]\n",
					explain, strings.Join(result.ProjectNames, ", "))
				if result.TargetName != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  → candidat %s : %s (version %s)\n",
						targetRepo, result.TargetName, result.TargetVersion)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "  → 0 candidat %s\n", targetRepo)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  → status : %s, confidence : %s\n", result.Status, result.Confidence)
				if result.Notes != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "  → notes : %s\n", result.Notes)
				}
				return nil
			}
			sample, err := cmd.Flags().GetInt("sample")
			if err != nil {
				return err
			}
			if sample < 0 {
				return fmt.Errorf("--sample doit être positif ou nul")
			}
			outputPath, err := cmd.Flags().GetString("output")
			if err != nil {
				return err
			}
			reportPath, err := cmd.Flags().GetString("report")
			if err != nil {
				return err
			}
			if outputPath == "" && reportPath == "" {
				return fmt.Errorf("--output ou --report est requis")
			}
			if dbPath == "" {
				return fmt.Errorf("chemin de base Repology indisponible")
			}
			source, err := readManifest(manifestPath)
			if err != nil {
				return err
			}
			db, err := repology.OpenDB(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			resolutions, err := resolver.ResolveManifest(cmd.Context(), db, source, targetRepo, sample)
			if err != nil {
				return err
			}
			if outputPath != "" {
				if err := writeResolveFile(outputPath, func(writer io.Writer) error {
					return resolver.WriteCSV(writer, resolutions)
				}); err != nil {
					return fmt.Errorf("écriture CSV %s: %w", outputPath, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "%d lignes écrites dans %s\n", len(resolutions), outputPath)
			}
			if reportPath != "" {
				targetCount, err := repology.CountPackagesInRepo(cmd.Context(), db, targetRepo)
				if err != nil {
					return err
				}
				report := resolver.MarkdownReport{
					SourceManifest:       filepath.Base(manifestPath),
					SourcePackageCount:   len(source.Packages),
					TargetRepo:           targetRepo,
					Date:                 time.Now().Format("2006-01-02"),
					SampleCount:          len(resolutions),
					DatabasePackageCount: targetCount,
					Resolutions:          resolutions,
				}
				if err := writeResolveFile(reportPath, func(writer io.Writer) error {
					return resolver.WriteMarkdownReport(writer, report)
				}); err != nil {
					return fmt.Errorf("écriture rapport %s: %w", reportPath, err)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Rapport écrit dans %s\n", reportPath)
			}
			return nil
		},
	}
	command.Flags().String("from-manifest", "", "Source manifest to resolve")
	command.Flags().String("target", "debian_13", "Repology target repository")
	command.Flags().Int("sample", 0, "Limit the output to the first N source packages (0 means all)")
	command.Flags().String("output", "", "Required CSV output path")
	command.Flags().String("explain", "", "Explain resolution for one source package")
	command.Flags().String("report", "", "Write a human-readable Markdown report")
	return command
}

func writeResolveFile(path string, write func(io.Writer) error) error {
	if parent := filepath.Dir(path); parent != "." {
		if err := os.MkdirAll(parent, 0o755); err != nil {
			return fmt.Errorf("création dossier de sortie %s: %w", parent, err)
		}
	}
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("création fichier: %w", err)
	}
	writeErr := write(file)
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return errors.Join(writeErr, closeErr)
	}
	return nil
}
