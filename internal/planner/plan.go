package planner

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/transpose-project/transpose/internal/resolver"
	"github.com/transpose-project/transpose/internal/sysutil"
	"github.com/transpose-project/transpose/pkg/manifest"
)

type Plan struct {
	Source      manifest.Source `json:"source"`
	Target      sysutil.Target  `json:"target"`
	GeneratedAt time.Time       `json:"generated_at"`
	Steps       []PlanStep      `json:"steps"`
	Skipped     []SkippedItem   `json:"skipped"`
	Warnings    []string        `json:"warnings"`
	Summary     PlanSummary     `json:"summary"`
}

type PlanStep struct {
	Order     int            `json:"order"`
	Manager   string         `json:"manager"`
	Action    string         `json:"action"`
	Command   string         `json:"command"`
	Items     []PlanItem     `json:"items"`
	RepoItems []RepoPlanItem `json:"repo_items,omitempty"`
}

type RepoPlanItem struct {
	Repo                 manifest.Repo `json:"repo"`
	Action               string        `json:"action"`
	TargetPath           string        `json:"target_path"`
	RequiresSudo         bool          `json:"requires_sudo"`
	RequiresConfirmation bool          `json:"requires_confirmation"`
}

type PlanItem struct {
	Source     string          `json:"source"`
	Target     string          `json:"target"`
	Status     resolver.Status `json:"status"`
	Confidence string          `json:"confidence"`
	Reason     string          `json:"reason"`
}

type SkippedItem struct {
	Source string `json:"source"`
	Reason string `json:"reason"`
}

type PlanSummary struct {
	TotalPackages int `json:"total_packages"`
	Exact         int `json:"exact"`
	Equivalent    int `json:"equivalent"`
	Alternative   int `json:"alternative"`
	Ambiguous     int `json:"ambiguous"`
	NotFound      int `json:"not_found"`
}

type PackageResolver interface {
	Resolve(pkg manifest.Package, target sysutil.Target) (resolver.Resolution, error)
}

type PlanOptions struct {
	Target  sysutil.Target
	Only    []string
	Exclude []string

	Resolver    PackageResolver
	GeneratedAt time.Time
}

var managerOrder = []string{"dnf", "flatpak", "apt", "snap"}

func GeneratePlan(m *manifest.Manifest, opts PlanOptions) (*Plan, error) {
	if m == nil {
		return nil, fmt.Errorf("manifeste requis")
	}
	sourceFamily := sysutil.FamilyForDistro(m.Source.Distro, "")
	if sourceFamily == "" {
		return nil, fmt.Errorf("famille source inconnue pour %q", m.Source.Distro)
	}
	sourceTarget := sysutil.Target{Family: sourceFamily}
	if !sysutil.SameFamily(sourceTarget, opts.Target) {
		return nil, fmt.Errorf("cross-family non supporté (%s → %s) ; requiert F8 (base Repology en CI). Voir docs/PLAN-PHASE-1.md",
			m.Source.Distro, opts.Target.Distro)
	}
	if opts.Resolver == nil {
		return nil, fmt.Errorf("résolveur de paquets requis")
	}

	packages := make([]manifest.Package, 0, len(m.Packages)+len(m.Flatpak.Apps))
	packages = append(packages, m.Packages...)
	for _, app := range m.Flatpak.Apps {
		packages = append(packages, manifest.Package{
			Name:    app.Ref,
			Manager: "flatpak",
			Extra:   map[string]any{"flatpak_ref": app.Ref},
		})
	}
	sort.SliceStable(packages, func(i, j int) bool {
		if strings.EqualFold(packages[i].Name, packages[j].Name) {
			return packages[i].Name < packages[j].Name
		}
		return strings.ToLower(packages[i].Name) < strings.ToLower(packages[j].Name)
	})

	plan := &Plan{
		Source:      m.Source,
		Target:      opts.Target,
		GeneratedAt: opts.GeneratedAt,
		Steps:       []PlanStep{},
		Skipped:     []SkippedItem{},
		Warnings:    []string{},
		Summary:     PlanSummary{TotalPackages: len(packages)},
	}
	if plan.GeneratedAt.IsZero() {
		plan.GeneratedAt = time.Now().UTC()
	} else {
		plan.GeneratedAt = plan.GeneratedAt.UTC()
	}
	if len(m.Repos) > 0 {
		repoItems := make([]RepoPlanItem, 0, len(m.Repos))
		for _, repo := range m.Repos {
			targetPath, err := repoTargetPath(repo)
			if err != nil {
				return nil, err
			}
			repoItems = append(repoItems, RepoPlanItem{
				Repo:                 repo,
				Action:               "write-file",
				TargetPath:           targetPath,
				RequiresSudo:         true,
				RequiresConfirmation: repo.ThirdParty,
			})
			if repo.ThirdParty && repo.Enabled {
				plan.Warnings = append(plan.Warnings,
					fmt.Sprintf("dépôt tiers %q activé : confirmation manuelle requise avant restauration", repo.Name))
			}
		}
		sort.SliceStable(repoItems, func(i, j int) bool {
			return repoItems[i].TargetPath < repoItems[j].TargetPath
		})
		plan.Steps = append(plan.Steps, PlanStep{
			Order:     1,
			Manager:   "repos",
			Action:    "restore-repos",
			Command:   "write repository files (no changes applied)",
			RepoItems: repoItems,
		})
	}
	excluded := stringSet(opts.Exclude)
	only := stringSet(opts.Only)
	grouped := make(map[string][]PlanItem)

	for _, pkg := range packages {
		if _, skip := excluded[strings.ToLower(strings.TrimSpace(pkg.Name))]; skip {
			plan.Skipped = append(plan.Skipped, SkippedItem{Source: pkg.Name, Reason: "exclu explicitement"})
			continue
		}
		result, err := opts.Resolver.Resolve(pkg, opts.Target)
		if err != nil {
			return nil, fmt.Errorf("résolution de %s: %w", pkg.Name, err)
		}
		switch result.Status {
		case resolver.StatusExact:
			plan.Summary.Exact++
		case resolver.StatusEquivalent:
			plan.Summary.Equivalent++
		case resolver.StatusAlternative:
			plan.Summary.Alternative++
		case resolver.StatusAmbiguous:
			plan.Summary.Ambiguous++
		case resolver.StatusNotFound:
			plan.Summary.NotFound++
		default:
			return nil, fmt.Errorf("statut de résolution inconnu pour %s: %q", pkg.Name, result.Status)
		}

		if result.Status == resolver.StatusNotFound || result.Status == resolver.StatusAmbiguous {
			reason := result.Reason
			if len(result.Alternatives) > 0 {
				reason += " : " + strings.Join(result.Alternatives, ", ")
			}
			plan.Skipped = append(plan.Skipped, SkippedItem{Source: pkg.Name, Reason: reason})
			continue
		}
		manager := strings.ToLower(strings.TrimSpace(result.Manager))
		if len(only) > 0 {
			if _, include := only[manager]; !include {
				plan.Skipped = append(plan.Skipped, SkippedItem{Source: pkg.Name, Reason: "gestionnaire " + manager + " exclu par --only"})
				continue
			}
		}
		grouped[manager] = append(grouped[manager], PlanItem{
			Source:     pkg.Name,
			Target:     result.Target,
			Status:     result.Status,
			Confidence: result.Confidence,
			Reason:     result.Reason,
		})
	}

	order := len(plan.Steps) + 1
	for _, manager := range managerOrder {
		items := grouped[manager]
		if len(items) == 0 {
			continue
		}
		sort.SliceStable(items, func(i, j int) bool {
			if strings.EqualFold(items[i].Source, items[j].Source) {
				return items[i].Source < items[j].Source
			}
			return strings.ToLower(items[i].Source) < strings.ToLower(items[j].Source)
		})
		plan.Steps = append(plan.Steps, PlanStep{
			Order:   order,
			Manager: manager,
			Action:  "install",
			Command: installCommand(manager),
			Items:   items,
		})
		order++
	}
	for manager := range grouped {
		if !contains(managerOrder, manager) {
			return nil, fmt.Errorf("gestionnaire cible non pris en charge : %s", manager)
		}
	}
	sort.SliceStable(plan.Skipped, func(i, j int) bool {
		if strings.EqualFold(plan.Skipped[i].Source, plan.Skipped[j].Source) {
			return plan.Skipped[i].Source < plan.Skipped[j].Source
		}
		return strings.ToLower(plan.Skipped[i].Source) < strings.ToLower(plan.Skipped[j].Source)
	})
	return plan, nil
}

func repoTargetPath(repo manifest.Repo) (string, error) {
	switch repo.Kind {
	case "rpm":
		name := filepath.Base(repo.FileName)
		if name == "." || name == ".." || name == string(filepath.Separator) || name == "" {
			return "", fmt.Errorf("nom de fichier .repo manquant pour %s", repo.Name)
		}
		return filepath.Join("/etc/yum.repos.d", name), nil
	case "apt":
		if repo.FileName == "" || repo.FileName == "sources.list" {
			return "/etc/apt/sources.list", nil
		}
		name := filepath.Base(repo.FileName)
		if name == "." || name == ".." || name == string(filepath.Separator) || name == "" {
			return "", fmt.Errorf("nom de fichier APT manquant pour %s", repo.Name)
		}
		return filepath.Join("/etc/apt/sources.list.d", name), nil
	default:
		return "", fmt.Errorf("type de dépôt non pris en charge : %s", repo.Kind)
	}
}

func installCommand(manager string) string {
	switch manager {
	case "dnf":
		return "dnf install -y"
	case "apt":
		return "apt install -y"
	case "flatpak":
		return "flatpak install flathub"
	case "snap":
		return "snap install"
	default:
		return manager + " install"
	}
}

func stringSet(values []string) map[string]struct{} {
	set := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "" {
			set[value] = struct{}{}
		}
	}
	return set
}

func contains(values []string, value string) bool {
	for _, candidate := range values {
		if candidate == value {
			return true
		}
	}
	return false
}
