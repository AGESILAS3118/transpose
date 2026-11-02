package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/archive"
)

func newPackCmd() *cobra.Command {
	var (
		outputFlag       string
		includeReposFlag bool
		verboseFlag      bool
	)

	cmd := &cobra.Command{
		Use:   "pack <manifest.yaml>",
		Short: "Create a portable .tar.zst archive from a manifest",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manifestPath := args[0]

			opts := archive.PackOptions{
				ManifestPath: manifestPath,
				OutputPath:   outputFlag,
				IncludeRepos: includeReposFlag,
			}

			res, err := archive.Pack(opts)
			if err != nil {
				return err
			}

			for _, w := range res.Warnings {
				fmt.Fprintf(cmd.ErrOrStderr(), "⚠ %s\n", w)
			}

			sizeStr := formatFileSize(res.Size)
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "Archive créée : %s (%s)\n", res.ArchivePath, sizeStr)
			fmt.Fprintf(out, "  Manifeste    : %d paquets\n", res.PackageCount)
			fmt.Fprintf(out, "  Dépôts       : %d fichiers\n", res.RepoCount)
			fmt.Fprintf(out, "  Source       : %s\n", res.SourceDistro)

			if verboseFlag {
				fmt.Fprintf(out, "  Format       : tar.zst (version %d)\n", archive.CurrentArchiveVersion)
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&outputFlag, "output", "o", "", "Write the archive to this file (default: <manifest-name>.tar.zst)")
	cmd.Flags().BoolVar(&includeReposFlag, "include-repos", true, "Include repository definitions in the archive")
	cmd.Flags().BoolVarP(&verboseFlag, "verbose", "v", false, "Show detailed archive information")

	return cmd
}

func formatFileSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
