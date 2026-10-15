package cmd

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/resolver"
	"github.com/transpose-project/transpose/internal/resolver/repology"
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
			manifestPath, err := cmd.Flags().GetString("from-manifest")
			if err != nil {
				return err
			}
			if manifestPath == "" {
				return fmt.Errorf("--from-manifest est obligatoire")
			}
			targetRepo, err := cmd.Flags().GetString("target")
			if err != nil {
				return err
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
			if outputPath == "" {
				return fmt.Errorf("--output est obligatoire")
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
			if parent := filepath.Dir(outputPath); parent != "." {
				if err := os.MkdirAll(parent, 0o755); err != nil {
					return fmt.Errorf("création dossier de sortie %s: %w", parent, err)
				}
			}
			file, err := os.Create(outputPath)
			if err != nil {
				return fmt.Errorf("création CSV %s: %w", outputPath, err)
			}
			writeErr := resolver.WriteCSV(file, resolutions)
			closeErr := file.Close()
			if writeErr != nil || closeErr != nil {
				return fmt.Errorf("écriture CSV %s: %w", outputPath, errors.Join(writeErr, closeErr))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%d lignes écrites dans %s\n", len(resolutions), outputPath)
			return nil
		},
	}
	command.Flags().String("from-manifest", "", "Source manifest to resolve")
	command.Flags().String("target", "debian_13", "Repology target repository")
	command.Flags().Int("sample", 0, "Limit the output to the first N source packages (0 means all)")
	command.Flags().String("output", "", "Required CSV output path")
	return command
}
