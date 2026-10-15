package adapter

import (
	"fmt"
	"os/exec"
)

type CaptureLevel int

const (
	Level1 CaptureLevel = 1
	Level2 CaptureLevel = 2
	Level3 CaptureLevel = 3
)

type Package struct {
	Name    string       `json:"name"`
	Version string       `json:"version"`
	Source  CaptureLevel `json:"source"`
}

type Reliability string

const (
	ReliabilityHigh   Reliability = "high"
	ReliabilityMedium Reliability = "medium"
	ReliabilityLow    Reliability = "low"
)

type CaptureResult struct {
	Manager     string         `json:"manager"`
	Level       CaptureLevel   `json:"level"`
	Reliability Reliability    `json:"reliability"`
	Packages    []Package      `json:"packages"`
	Warnings    []string       `json:"warnings"`
	Raw         map[string]any `json:"raw"`
}

type HealthStatus struct {
	Manager    string `json:"manager"`
	Present    bool   `json:"present"`
	Functional bool   `json:"functional"`
	ItemCount  int    `json:"item_count"`
	Message    string `json:"message"`
}

type Adapter interface {
	Name() string
	Detect() (bool, error)
	Health() (HealthStatus, error)
	Capture() (*CaptureResult, error)
}

type CommandRunner func(name string, args ...string) (stdout []byte, err error)

func ExecCommandRunner(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	stdout, err := cmd.Output()
	if err == nil {
		return stdout, nil
	}
	if exitErr, ok := err.(*exec.ExitError); ok && len(exitErr.Stderr) > 0 {
		return stdout, fmt.Errorf("%s: %w: %s", name, err, exitErr.Stderr)
	}
	return stdout, fmt.Errorf("%s: %w", name, err)
}
