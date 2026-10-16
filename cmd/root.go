package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

var version = "0.1.0-dev"

// Execute runs the root command.
func Execute() error {
	return newRootCmd().Execute()
}

func newRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:           "xpose",
		Short:         "Capture, translate, and restore a Linux system",
		Long:          "Transpose captures a Linux system, translates it through a neutral manifest, and restores it on another distribution.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	rootCmd.SetVersionTemplate("xpose {{.Version}}\n")
	rootCmd.PersistentFlags().Bool("dry-run", false, "Show planned changes without applying them")
	rootCmd.PersistentFlags().String("format", "yaml", "Output format (yaml|json|raw)")
	rootCmd.PersistentFlags().String("lang", "en", "Interface language (fr|en)")
	rootCmd.PersistentFlags().Bool("offline", false, "Do not access the network")
	rootCmd.PersistentFlags().BoolP("yes", "y", false, "Answer yes to confirmation prompts")
	rootCmd.PersistentFlags().String("profile", "", "Configuration profile")
	rootCmd.PersistentFlags().StringSlice("only", nil, "Limit operations to these components")
	rootCmd.PersistentFlags().StringSlice("exclude", nil, "Exclude these components")

	packCmd := newPlaceholderCmd("pack <manifest>", "Create a portable archive from a manifest", cobra.ExactArgs(1))
	packCmd.Flags().StringP("output", "o", "", "Write the archive to this file")
	packCmd.Flags().Bool("encrypt", false, "Encrypt the archive with a passphrase")
	rootCmd.AddCommand(
		packCmd,
		newPlanCmd(),
		newPlaceholderCmd("apply <manifest>", "Apply a validated migration plan", cobra.ExactArgs(1)),
		newPlaceholderCmd("diff <manifest-a> <manifest-b>", "Compare two manifests", cobra.ExactArgs(2)),
	)
	rootCmd.AddCommand(newResolveCmd())
	rootCmd.AddCommand(newReposCmd())
	rootCmd.AddCommand(newUpdateDBCmd())
	rootCmd.AddCommand(newCaptureCmd())
	rootCmd.AddCommand(newDoctorCmd())

	return rootCmd
}

func newPlaceholderCmd(use, short string, args cobra.PositionalArgs) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short + " (not implemented)",
		Args:  args,
		RunE: func(cmd *cobra.Command, args []string) error {
			return fmt.Errorf("%s: not implemented", cmd.Name())
		},
	}
}
