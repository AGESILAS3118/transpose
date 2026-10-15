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

	for _, name := range []string{"pack", "plan", "apply", "resolve", "diff", "update-db"} {
		rootCmd.AddCommand(newPlaceholderCmd(name))
	}
	rootCmd.AddCommand(newCaptureCmd())
	rootCmd.AddCommand(newDoctorCmd())

	return rootCmd
}

func newPlaceholderCmd(name string) *cobra.Command {
	return &cobra.Command{
		Use:   name,
		Short: fmt.Sprintf("%s (not implemented)", name),
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s: not implemented\n", name)
		},
	}
}
