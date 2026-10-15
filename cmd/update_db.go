package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/resolver/repology"
	"github.com/transpose-project/transpose/pkg/manifest"
)

func newUpdateDBCmd() *cobra.Command {
	return newUpdateDBCmdWithPaths(func() (string, string, error) { return repologyPaths() })
}

func newUpdateDBCmdWithPaths(paths func() (string, string, error)) *cobra.Command {
	command := &cobra.Command{
		Use:   "update-db",
		Short: "Bootstrap the local Repology database from a manifest",
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
			limit, err := cmd.Flags().GetInt("limit")
			if err != nil {
				return err
			}
			if limit < 0 {
				return fmt.Errorf("--limit doit être positif ou nul")
			}
			offline, err := cmd.Flags().GetBool("offline")
			if err != nil {
				return err
			}
			source, err := readManifest(manifestPath)
			if err != nil {
				return err
			}
			dbPath, cacheDir, err := paths()
			if err != nil {
				return err
			}
			db, err := repology.OpenDB(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			client := repology.NewClient(cacheDir, offline)
			limited := *source
			if limit > 0 {
				var packages []manifest.Package
				for _, pkg := range source.Packages {
					if pkg.Manager == "dnf" && len(packages) < limit {
						packages = append(packages, pkg)
					}
				}
				limited.Packages = packages
			}
			started := time.Now()
			stats, err := repology.PopulateWithObserver(cmd.Context(), db, client, &limited, targetRepo,
				func(progress repology.PopulateProgress) {
					fmt.Fprintf(cmd.ErrOrStderr(),
						"[%d/%d] %s → %d candidats %s (cache: %d, réseau: %d, 404: %d)\n",
						progress.Done, progress.Total, progress.SourceName, progress.CandidateCount,
						targetRepo, progress.CacheHits, progress.NetworkHits, progress.NotFound)
				})
			if err != nil {
				return err
			}
			for _, warning := range stats.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s\n", warning)
			}
			fmt.Fprintf(cmd.OutOrStdout(),
				"Projets traités: %d; paquets stockés: %d; 404: %d; erreurs réseau/DB: %d; durée: %s\n",
				stats.Processed, stats.StoredPackages, stats.NotFound, stats.NetworkErrors, time.Since(started).Round(time.Second))
			return nil
		},
	}
	command.Flags().String("from-manifest", "", "Source manifest to bootstrap from")
	command.Flags().String("target", "debian_13", "Repology target repository")
	command.Flags().Int("limit", 0, "Limit the number of source packages (0 means all)")
	command.Flags().Bool("offline", false, "Use only cached Repology responses")
	return command
}
