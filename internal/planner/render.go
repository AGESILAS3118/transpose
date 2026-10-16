package planner

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

func RenderTerminal(p *Plan, w io.Writer, verbose bool) error {
	if p == nil {
		return fmt.Errorf("plan nil")
	}
	if _, err := fmt.Fprintln(w, "Plan de migration"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  Source : %s\n  Cible  : %s\n  Généré : %s\n\n",
		formatDistro(p.Source.Distro, p.Source.Version, p.Source.Arch),
		formatDistro(p.Target.Distro, p.Target.Version, p.Target.Arch),
		p.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z")); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "Résumé"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  %d paquets source\n  %d exacts\n  %d équivalents\n  %d alternatives Flatpak\n  %d ambigus\n  %d non trouvés\n  %d ignorés\n",
		p.Summary.TotalPackages, p.Summary.Exact, p.Summary.Equivalent, p.Summary.Alternative,
		p.Summary.Ambiguous, p.Summary.NotFound, len(p.Skipped)); err != nil {
		return err
	}
	for _, step := range p.Steps {
		if step.Manager == "repos" {
			if _, err := fmt.Fprintf(w, "\nÉtape %d — dépôts (%d)\n", step.Order, len(step.RepoItems)); err != nil {
				return err
			}
			for _, item := range step.RepoItems {
				marker := "[officiel]"
				if item.RequiresConfirmation {
					marker = "[tiers, confirmation requise]"
				}
				if _, err := fmt.Fprintf(w, "  + %-48s %s\n", item.TargetPath, marker); err != nil {
					return err
				}
			}
			continue
		}
		unit := "paquets"
		if step.Manager == "flatpak" {
			unit = "apps"
		}
		if _, err := fmt.Fprintf(w, "\nÉtape %d — %s (%d %s)\n", step.Order, step.Manager, len(step.Items), unit); err != nil {
			return err
		}
		limit := len(step.Items)
		if !verbose && limit > 5 {
			limit = 5
		}
		names := make([]string, 0, limit)
		for _, item := range step.Items[:limit] {
			names = append(names, shellArg(item.Target))
		}
		line := step.Command
		if len(names) > 0 {
			line += " " + strings.Join(names, " ")
		}
		if !verbose && len(step.Items) > limit {
			line += fmt.Sprintf(" … (%d au total)", len(step.Items))
		}
		if _, err := fmt.Fprintf(w, "  %s\n", line); err != nil {
			return err
		}
	}
	if len(p.Skipped) > 0 {
		if _, err := fmt.Fprintf(w, "\nIgnorés (%d)\n", len(p.Skipped)); err != nil {
			return err
		}
		for _, item := range p.Skipped {
			if _, err := fmt.Fprintf(w, "  - %s (%s)\n", item.Source, item.Reason); err != nil {
				return err
			}
		}
	}
	if len(p.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "\nAvertissements"); err != nil {
			return err
		}
		for _, warning := range p.Warnings {
			if _, err := fmt.Fprintf(w, "  - %s\n", warning); err != nil {
				return err
			}
		}
	}
	return nil
}

func RenderMarkdown(p *Plan, w io.Writer) error {
	if p == nil {
		return fmt.Errorf("plan nil")
	}
	if _, err := fmt.Fprintf(w, "# Plan de migration\n\n- **Source :** %s\n- **Cible :** %s\n- **Généré :** %s\n\n",
		formatDistro(p.Source.Distro, p.Source.Version, p.Source.Arch),
		formatDistro(p.Target.Distro, p.Target.Version, p.Target.Arch),
		p.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z")); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "## Résumé\n\n| Mesure | Nombre |\n|---|---:|\n| Paquets source | %d |\n| Exacts | %d |\n| Équivalents | %d |\n| Alternatives | %d |\n| Ambigus | %d |\n| Non trouvés | %d |\n| Ignorés | %d |\n",
		p.Summary.TotalPackages, p.Summary.Exact, p.Summary.Equivalent, p.Summary.Alternative,
		p.Summary.Ambiguous, p.Summary.NotFound, len(p.Skipped)); err != nil {
		return err
	}
	for _, step := range p.Steps {
		if step.Manager == "repos" {
			if _, err := fmt.Fprintf(w, "\n## Étape %d — dépôts (%d)\n\n| Fichier cible | Dépôt | État |\n|---|---|---|\n",
				step.Order, len(step.RepoItems)); err != nil {
				return err
			}
			for _, item := range step.RepoItems {
				name := markdownCell(item.Repo.Name)
				state := "officiel"
				if item.RequiresConfirmation {
					name = "**" + name + "** ⚠"
					state = "tiers — confirmation requise"
				}
				if _, err := fmt.Fprintf(w, "| %s | %s | %s |\n",
					markdownCell(item.TargetPath), name, state); err != nil {
					return err
				}
			}
			continue
		}
		command := markdownCommand(step)
		if _, err := fmt.Fprintf(w, "\n## Étape %d — %s (%d items)\n\n```sh\n%s\n```\n\n| Source | Cible | Statut | Confiance | Raison |\n|---|---|---|---|---|\n",
			step.Order, step.Manager, len(step.Items), command); err != nil {
			return err
		}
		for _, item := range step.Items {
			if _, err := fmt.Fprintf(w, "| %s | %s | %s | %s | %s |\n",
				markdownCell(item.Source), markdownCell(item.Target), item.Status,
				markdownCell(item.Confidence), markdownCell(item.Reason)); err != nil {
				return err
			}
		}
	}
	if len(p.Skipped) > 0 {
		if _, err := fmt.Fprintln(w, "\n## Ignorés\n\n| Source | Raison |\n|---|---|"); err != nil {
			return err
		}
		for _, item := range p.Skipped {
			if _, err := fmt.Fprintf(w, "| %s | %s |\n", markdownCell(item.Source), markdownCell(item.Reason)); err != nil {
				return err
			}
		}
	}
	if len(p.Warnings) > 0 {
		if _, err := fmt.Fprintln(w, "\n## Avertissements"); err != nil {
			return err
		}
		for _, warning := range p.Warnings {
			if _, err := fmt.Fprintf(w, "- %s\n", warning); err != nil {
				return err
			}
		}
	}
	return nil
}

func RenderJSON(p *Plan, w io.Writer) error {
	if p == nil {
		return fmt.Errorf("plan nil")
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(p); err != nil {
		return fmt.Errorf("encodage JSON du plan: %w", err)
	}
	return nil
}

func formatDistro(distro, version, arch string) string {
	result := distro
	if version != "" {
		result += " " + version
	}
	if arch != "" {
		result += " (" + formatArch(arch) + ")"
	}
	return result
}

func formatArch(arch string) string {
	switch arch {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return arch
	}
}

func markdownCommand(step PlanStep) string {
	base := strings.Fields(step.Command)
	if len(step.Items) == 0 {
		return strings.Join(base, " ")
	}
	targets := make([]string, 0, len(step.Items))
	for _, item := range step.Items {
		targets = append(targets, shellArg(item.Target))
	}
	var lines []string
	lines = append(lines, strings.Join(base, " ")+" \\")
	for start := 0; start < len(targets); start += 8 {
		end := start + 8
		if end > len(targets) {
			end = len(targets)
		}
		line := "  " + strings.Join(targets[start:end], " ")
		if end < len(targets) {
			line += " \\"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func shellArg(value string) string {
	if value != "" && strings.IndexFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' ||
			strings.ContainsRune("_+-./:@", r))
	}) < 0 {
		return value
	}
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func markdownCell(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "|", "\\|"), "\n", " ")
}
