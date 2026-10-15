package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/transpose-project/transpose/internal/adapter"
)

type mockAdapter struct {
	name   string
	found  bool
	status adapter.HealthStatus
	result *adapter.CaptureResult
	err    error
}

func (m mockAdapter) Name() string { return m.name }

func (m mockAdapter) Detect() (bool, error) { return m.found, nil }

func (m mockAdapter) Health() (adapter.HealthStatus, error) { return m.status, nil }

func (m mockAdapter) Capture() (*adapter.CaptureResult, error) {
	return m.result, m.err
}

func TestDoctorReportsInconsistency(t *testing.T) {
	mock := mockAdapter{
		name: "dnf",
		status: adapter.HealthStatus{
			Manager:    "dnf",
			Present:    true,
			Functional: false,
			ItemCount:  0,
			Message:    "⚠ INCOHÉRENCE DÉTECTÉE — repli recommandé (rpm: 3455)",
		},
	}
	root := commandRoot(newDoctorCmdWithAdapters([]adapter.Adapter{mock}))
	root.SetArgs([]string{"doctor"})
	var output bytes.Buffer
	root.SetOut(&output)
	if err := root.Execute(); err != nil {
		t.Fatalf("doctor execution error = %v", err)
	}
	for _, expected := range []string{"MANAGER", "dnf", "yes", "no", "3455", "INCOHÉRENCE DÉTECTÉE"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("doctor output %q does not contain %q", output.String(), expected)
		}
	}
}

func TestCaptureJSONIncludesInternalResult(t *testing.T) {
	mock := mockAdapter{
		name:  "dnf",
		found: true,
		result: &adapter.CaptureResult{
			Manager:     "dnf",
			Level:       adapter.Level2,
			Reliability: adapter.ReliabilityMedium,
			Packages:    []adapter.Package{{Name: "htop", Version: "3.3", Source: adapter.Level2}},
		},
	}
	root := commandRoot(newCaptureCmdWithAdapters([]adapter.Adapter{mock}))
	root.SetArgs([]string{"capture", "--only", "dnf", "--format", "json"})
	var output bytes.Buffer
	root.SetOut(&output)
	if err := root.Execute(); err != nil {
		t.Fatalf("capture execution error = %v", err)
	}
	var results []adapter.CaptureResult
	if err := json.Unmarshal(output.Bytes(), &results); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, output.String())
	}
	if len(results) != 1 || results[0].Level != adapter.Level2 || results[0].Packages[0].Name != "htop" {
		t.Errorf("JSON results = %#v, want internal dnf CaptureResult", results)
	}
}

func TestCaptureSelectedUnavailableAdapterReturnsError(t *testing.T) {
	mock := mockAdapter{name: "dnf", found: false}
	root := commandRoot(newCaptureCmdWithAdapters([]adapter.Adapter{mock}))
	root.SetArgs([]string{"capture", "--only", "dnf"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "dnf impossible") {
		t.Errorf("capture error = %v, want clear dnf unavailable error", err)
	}
}

func TestCapturePropagatesAdapterError(t *testing.T) {
	mock := mockAdapter{name: "dnf", found: true, err: errors.New("cascade failed")}
	root := commandRoot(newCaptureCmdWithAdapters([]adapter.Adapter{mock}))
	root.SetArgs([]string{"capture", "--only", "dnf"})
	if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "cascade failed") {
		t.Errorf("capture error = %v, want adapter failure", err)
	}
}

func commandRoot(command *cobra.Command) *cobra.Command {
	root := &cobra.Command{Use: "xpose", SilenceErrors: true, SilenceUsage: true}
	root.PersistentFlags().String("format", "yaml", "output format")
	root.PersistentFlags().StringSlice("only", nil, "selected managers")
	root.AddCommand(command)
	return root
}
