package resolver

import (
	"github.com/transpose-project/transpose/internal/sysutil"
	"github.com/transpose-project/transpose/pkg/manifest"
)

type Status string

const (
	StatusExact       Status = "exact"
	StatusEquivalent  Status = "equivalent"
	StatusAlternative Status = "alternative"
	StatusAmbiguous   Status = "ambiguous"
	StatusNotFound    Status = "not_found"
)

type Resolution struct {
	Source       manifest.Package `json:"source"`
	Status       Status           `json:"status"`
	Target       string           `json:"target,omitempty"`
	Manager      string           `json:"manager,omitempty"`
	Confidence   string           `json:"confidence"`
	Reason       string           `json:"reason"`
	Alternatives []string         `json:"alternatives,omitempty"`
}

type Target = sysutil.Target
