package apt

import (
	"errors"

	"github.com/transpose-project/transpose/internal/adapter"
)

type Adapter struct {
	run adapter.CommandRunner
}

func New(run adapter.CommandRunner) *Adapter {
	if run == nil {
		run = adapter.ExecCommandRunner
	}
	return &Adapter{run: run}
}

func (a *Adapter) Name() string { return "apt" }

func (a *Adapter) Detect() (bool, error) {
	_, err := a.run("apt-get", "--version")
	return err == nil, nil
}

func (a *Adapter) Health() (adapter.HealthStatus, error) {
	status := adapter.HealthStatus{Manager: a.Name()}
	_, err := a.run("apt-get", "--version")
	status.Present = err == nil
	status.Message = "absent"
	if status.Present {
		status.Message = "détection seule (capture non implémentée)"
	}
	return status, nil
}

func (a *Adapter) Capture() (*adapter.CaptureResult, error) {
	return nil, errors.New("capture apt non implémentée")
}
