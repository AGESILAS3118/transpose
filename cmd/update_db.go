package cmd

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
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
			dumpPath, err := cmd.Flags().GetString("dump")
			if err != nil {
				return err
			}
			fromDump, err := cmd.Flags().GetString("from-dump")
			if err != nil {
				return err
			}
			if dumpPath != "" && fromDump != "" {
				return fmt.Errorf("utilisez --dump ou --from-dump, pas les deux")
			}
			if dumpPath == "" {
				dumpPath = fromDump
			}
			fromURL, err := cmd.Flags().GetString("from-url")
			if err != nil {
				return err
			}
			repo, err := cmd.Flags().GetString("repo")
			if err != nil {
				return err
			}
			manifestPath, err := cmd.Flags().GetString("from-manifest")
			if err != nil {
				return err
			}
			if (dumpPath != "" || fromURL != "") && manifestPath != "" {
				return fmt.Errorf("utilisez --dump/--from-url ou --from-manifest, pas les deux")
			}
			if dumpPath != "" && fromURL != "" {
				return fmt.Errorf("utilisez --dump ou --from-url, pas les deux")
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
			dbPath, cacheDir, err := paths()
			if err != nil {
				return err
			}
			db, err := repology.OpenDB(dbPath)
			if err != nil {
				return err
			}
			defer db.Close()
			if dumpPath != "" || fromURL != "" {
				if dumpPath == "" {
					dumpPath, err = downloadDump(cmd, fromURL, cacheDir)
					if err != nil {
						return err
					}
				}
				if filepath.Ext(dumpPath) == ".zst" || filepath.Ext(dumpPath) == ".sql" {
					return fmt.Errorf("dump Repology PostgreSQL (%s) non importable directement ; aucun dump filtré utilisable n'est publié. Utilisez --from-manifest avec le repli API", dumpPath)
				}
				file, err := os.Open(dumpPath)
				if err != nil {
					return fmt.Errorf("ouverture dump %s: %w", dumpPath, err)
				}
				started := time.Now()
				imported, importErr := repology.ImportDump(cmd.Context(), db, file, repo)
				closeErr := file.Close()
				if importErr != nil || closeErr != nil {
					return fmt.Errorf("import dump %s: %w", dumpPath, errors.Join(importErr, closeErr))
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Paquets importés: %d; dépôt: %s; durée: %s\n",
					imported, repo, time.Since(started).Round(time.Second))
				return nil
			}
			if manifestPath == "" {
				return fmt.Errorf("--from-manifest est obligatoire en mode API (ou utilisez --dump/--from-url)")
			}
			client := repology.NewClient(cacheDir, offline)
			source, err := readManifest(manifestPath)
			if err != nil {
				return err
			}
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
	command.Flags().String("dump", "", "Filtered Repology JSONL dump to import")
	command.Flags().String("from-dump", "", "Alias for --dump")
	command.Flags().String("from-url", "", "Download a filtered Repology JSONL dump with resume support")
	command.Flags().String("repo", "debian_13", "Repository to import from a dump")
	return command
}

func downloadDump(cmd *cobra.Command, sourceURL, cacheDir string) (string, error) {
	if sourceURL == "" {
		return "", fmt.Errorf("--from-url ne peut pas être vide")
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return "", fmt.Errorf("création cache dumps %s: %w", cacheDir, err)
	}
	name := filepath.Base(sourceURL)
	if name == "." || name == "/" || name == "" {
		name = "repology-filtered.jsonl"
	}
	if filepath.Ext(name) == ".zst" || filepath.Ext(name) == ".sql" {
		return "", fmt.Errorf("dump Repology PostgreSQL (%s) non importable directement ; aucun dump filtré utilisable n'est publié", name)
	}
	path := filepath.Join(cacheDir, "dumps", name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", fmt.Errorf("création dossier dump: %w", err)
	}
	command := exec.CommandContext(cmd.Context(), "curl", "-fL", "-C", "-", "-o", path, sourceURL)
	command.Stdout = cmd.ErrOrStderr()
	command.Stderr = cmd.ErrOrStderr()
	if err := command.Run(); err != nil {
		return "", fmt.Errorf("téléchargement reprenable du dump %s: %w", sourceURL, err)
	}
	return path, nil
}
