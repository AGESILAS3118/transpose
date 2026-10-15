package cmd

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/adapter"
)

func newDoctorCmd() *cobra.Command {
	return newDoctorCmdWithAdapters(allAdapters())
}

func newDoctorCmdWithAdapters(adapters []adapter.Adapter) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check package-manager availability and health",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			statuses := make([]adapter.HealthStatus, 0, len(adapters))
			for _, pkgAdapter := range adapters {
				status, err := pkgAdapter.Health()
				if err != nil {
					status = adapter.HealthStatus{
						Manager: pkgAdapter.Name(),
						Message: err.Error(),
					}
				}
				statuses = append(statuses, status)
			}
			format, err := cmd.Flags().GetString("format")
			if err != nil {
				return err
			}
			if format == "json" {
				return json.NewEncoder(cmd.OutOrStdout()).Encode(statuses)
			}
			return writeHealthTable(cmd.OutOrStdout(), statuses)
		},
	}
}

func writeHealthTable(output io.Writer, statuses []adapter.HealthStatus) error {
	writer := tabwriter.NewWriter(output, 0, 4, 2, ' ', 0)
	if _, err := fmt.Fprintln(writer, "MANAGER\tPRESENT\tFUNCTIONAL\tITEMS\tMESSAGE"); err != nil {
		return err
	}
	for _, status := range statuses {
		present, functional := "no", "no"
		if status.Present {
			present = "yes"
		}
		if status.Functional {
			functional = "yes"
		}
		message := strings.ReplaceAll(status.Message, "\t", " ")
		if _, err := fmt.Fprintf(writer, "%s\t%s\t%s\t%d\t%s\n", status.Manager, present, functional, status.ItemCount, message); err != nil {
			return err
		}
	}
	return writer.Flush()
}
