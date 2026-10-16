package repos

import (
	"context"

	"github.com/transpose-project/transpose/pkg/manifest"
)

type RepoCollector interface {
	Name() string
	Detect() (bool, error)
	Collect(ctx context.Context) ([]manifest.Repo, error)
}

type warningProvider interface {
	Warnings() []string
}

func Warnings(collector RepoCollector) []string {
	provider, ok := collector.(warningProvider)
	if !ok {
		return nil
	}
	return provider.Warnings()
}
