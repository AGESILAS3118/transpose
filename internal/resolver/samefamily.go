package resolver

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/transpose-project/transpose/pkg/manifest"
)

type RepoQuerier interface {
	ListAvailable(ctx context.Context) ([]string, error)
}

type SameFamilyResolver struct {
	querier       RepoQuerier
	overrides     *Overrides
	availableOnce sync.Once
	available     []string
	availableErr  error
}

func NewSameFamilyResolver(querier RepoQuerier, overrides *Overrides) *SameFamilyResolver {
	return &SameFamilyResolver{querier: querier, overrides: overrides}
}

func (r *SameFamilyResolver) Resolve(pkg manifest.Package, target Target) (Resolution, error) {
	if r == nil {
		return Resolution{}, fmt.Errorf("résolveur même-famille nil")
	}
	result := Resolution{Source: pkg, Confidence: "low"}
	if override, ok := r.overrides.Lookup(pkg.Name); ok {
		result.Status = StatusEquivalent
		result.Manager = override.Manager
		if override.TargetFlatpak != "" {
			result.Target = override.TargetFlatpak
			if result.Manager == "" {
				result.Manager = "flatpak"
			}
		} else {
			result.Target = override.TargetName
		}
		if result.Manager == "" {
			result.Manager = managerForTarget(target, pkg.Manager)
		}
		result.Confidence = "high"
		result.Reason = "override utilisateur"
		return result, nil
	}

	if r.querier == nil {
		return Resolution{}, fmt.Errorf("querier de dépôts cible nil")
	}
	r.availableOnce.Do(func() {
		r.available, r.availableErr = r.querier.ListAvailable(context.Background())
	})
	if r.availableErr != nil {
		return Resolution{}, fmt.Errorf("lecture des paquets disponibles: %w", r.availableErr)
	}
	for _, name := range r.available {
		if strings.EqualFold(strings.TrimSpace(name), strings.TrimSpace(pkg.Name)) {
			result.Status = StatusExact
			result.Target = name
			result.Manager = managerForTarget(target, pkg.Manager)
			result.Confidence = "high"
			result.Reason = "nom identique disponible dans les dépôts cibles"
			return result, nil
		}
	}

	if ref, ok := pkg.Extra["flatpak_ref"].(string); ok && strings.TrimSpace(ref) != "" {
		result.Status = StatusAlternative
		result.Target = ref
		result.Manager = "flatpak"
		result.Confidence = "medium"
		result.Reason = "app graphique, Flatpak recommandé"
		return result, nil
	}
	result.Status = StatusNotFound
	result.Reason = "absent des dépôts cibles"
	return result, nil
}

func managerForTarget(target Target, sourceManager string) string {
	switch target.Family {
	case "rpm":
		return "dnf"
	case "debian":
		return "apt"
	default:
		if sourceManager != "" {
			return sourceManager
		}
		return ""
	}
}
