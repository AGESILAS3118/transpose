package resolver

import (
	"fmt"
	"io"
	"strings"
)

type MarkdownReport struct {
	SourceManifest       string
	SourcePackageCount   int
	TargetRepo           string
	Date                 string
	SampleCount          int
	DatabasePackageCount int
	Resolutions          []RepologyResolution
}

func WriteMarkdownReport(writer io.Writer, report MarkdownReport) error {
	counts := map[string]int{
		"exact": 0, "equivalent": 0, "ambiguous": 0, "not_found": 0,
	}
	for _, resolution := range report.Resolutions {
		if _, ok := counts[resolution.Status]; ok {
			counts[resolution.Status]++
		}
	}

	if _, err := fmt.Fprintf(writer, "# Rapport de résolution Fedora → Debian\n\n"+
		"Source : %s (%d paquets)\n\n"+
		"Cible : %s\n\n"+
		"Date : %s\n\n"+
		"Échantillon : %d paquets\n\n"+
		"## Résumé\n\n"+
		"| Statut | Nombre | %% |\n|---|---:|---:|\n",
		report.SourceManifest, report.SourcePackageCount, report.TargetRepo, report.Date, report.SampleCount); err != nil {
		return fmt.Errorf("écriture en-tête rapport Markdown: %w", err)
	}
	for _, status := range []string{"exact", "equivalent", "ambiguous", "not_found"} {
		percentage := 0.0
		if report.SampleCount > 0 {
			percentage = float64(counts[status]) * 100 / float64(report.SampleCount)
		}
		percentageText := strings.ReplaceAll(fmt.Sprintf("%.1f", percentage), ".", ",")
		if _, err := fmt.Fprintf(writer, "| %s | %d | %s %% |\n", status, counts[status], percentageText); err != nil {
			return fmt.Errorf("écriture résumé rapport Markdown: %w", err)
		}
	}
	if _, err := fmt.Fprintf(writer, "\n⚠ Limitation : la base locale ne contient que %d paquets %s.\n"+
		"Les not_found incluent des faux négatifs (paquets réellement présents dans Debian mais absents de la base locale).\n\n"+
		"## Correspondances équivalentes (à vérifier manuellement)\n\n"+
		"| # | Source | Cible proposée | Notes |\n|---:|---|---|---|\n",
		report.DatabasePackageCount, report.TargetRepo); err != nil {
		return fmt.Errorf("écriture limitation rapport Markdown: %w", err)
	}
	for _, resolution := range report.Resolutions {
		if resolution.Status != "equivalent" {
			continue
		}
		if _, err := fmt.Fprintf(writer, "| %d | %s | %s | %s |\n",
			resolution.Index, markdownCell(resolution.SourceName), markdownCell(resolution.TargetName),
			markdownCell(resolution.Notes)); err != nil {
			return fmt.Errorf("écriture correspondances équivalentes: %w", err)
		}
	}
	if _, err := fmt.Fprint(writer, "\n## Correspondances ambiguës (choix nécessaire)\n\n"+
		"| # | Source | Candidats | Notes |\n|---:|---|---|---|\n"); err != nil {
		return fmt.Errorf("écriture en-tête ambiguïtés: %w", err)
	}
	for _, resolution := range report.Resolutions {
		if resolution.Status != "ambiguous" {
			continue
		}
		candidates := strings.Join(resolution.TargetCandidates, ", ")
		if candidates == "" && resolution.TargetName != "" {
			candidates = resolution.TargetName
		}
		if _, err := fmt.Fprintf(writer, "| %d | %s | %s | %s |\n",
			resolution.Index, markdownCell(resolution.SourceName), markdownCell(candidates),
			markdownCell(resolution.Notes)); err != nil {
			return fmt.Errorf("écriture correspondances ambiguës: %w", err)
		}
	}
	if _, err := fmt.Fprint(writer, "\n## Non trouvés (échantillon 20 premiers)\n\n"+
		"| # | Source | Raison probable |\n|---:|---|---|\n"); err != nil {
		return fmt.Errorf("écriture en-tête non trouvés: %w", err)
	}
	notFoundCount := 0
	for _, resolution := range report.Resolutions {
		if resolution.Status != "not_found" || notFoundCount >= 20 {
			continue
		}
		if _, err := fmt.Fprintf(writer, "| %d | %s | %s |\n",
			resolution.Index, markdownCell(resolution.SourceName), markdownCell(resolution.Notes)); err != nil {
			return fmt.Errorf("écriture non trouvés: %w", err)
		}
		notFoundCount++
	}
	_, err := fmt.Fprint(writer, "\n## Précision observée (vérification manuelle utilisateur)\n\n"+
		"Mesures relevées sur le CSV initial, avant reclassement du fuzzy match à candidat unique.\n\n"+
		"- 10/10 exact corrects\n"+
		"- 7/8 equivalent corrects\n"+
		"- ~6/10 not_found sont des faux négatifs (paquets présents dans Debian sous un autre nom)\n"+
		"- Précision résolveur : ~90 %+ quand il trouve\n")
	if err != nil {
		return fmt.Errorf("écriture précision rapport Markdown: %w", err)
	}
	return nil
}

func markdownCell(value string) string {
	value = strings.ReplaceAll(value, "|", "\\|")
	return strings.ReplaceAll(value, "\n", " ")
}
