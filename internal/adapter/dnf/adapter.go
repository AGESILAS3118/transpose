package dnf

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/transpose-project/transpose/internal/adapter"
)

const defaultHistoryPath = "/var/lib/dnf/history.sqlite"

type Adapter struct {
	run         adapter.CommandRunner
	historyPath string
}

func New(run adapter.CommandRunner) *Adapter {
	return NewWithHistoryPath(run, defaultHistoryPath)
}

func NewWithHistoryPath(run adapter.CommandRunner, historyPath string) *Adapter {
	if run == nil {
		run = adapter.ExecCommandRunner
	}
	return &Adapter{run: run, historyPath: historyPath}
}

func (a *Adapter) Name() string { return "dnf" }

func (a *Adapter) Detect() (bool, error) {
	_, dnfErr := a.run("dnf", "--version")
	if dnfErr == nil {
		return true, nil
	}
	_, dnf5Err := a.run("dnf5", "--version")
	if dnf5Err == nil {
		return true, nil
	}
	_, rpmErr := a.run("rpm", "--version")
	if rpmErr == nil {
		return true, nil
	}
	return false, nil
}

func (a *Adapter) Health() (adapter.HealthStatus, error) {
	status := adapter.HealthStatus{Manager: a.Name()}
	dnfName, dnfPresent := a.dnfCommand()
	_, rpmVersionErr := a.run("rpm", "--version")
	rpmPresent := rpmVersionErr == nil
	status.Present = dnfPresent || rpmPresent
	if !status.Present {
		status.Message = "absent"
		return status, nil
	}

	rpmOutput, rpmErr := a.run("rpm", "-qa")
	rpmCount := lineCount(rpmOutput)
	dnfItems, dnfErr := []adapter.Package(nil), errors.New("dnf unavailable")
	if dnfPresent {
		dnfItems, dnfErr = a.queryDNF(dnfName)
	}
	status.ItemCount = len(dnfItems)
	if dnfPresent && dnfErr == nil && len(dnfItems) == 0 && rpmErr == nil && rpmCount > 100 {
		status.Functional = false
		status.Message = fmt.Sprintf("⚠ INCOHÉRENCE DÉTECTÉE — repli recommandé (rpm: %d)", rpmCount)
		return status, nil
	}
	if dnfPresent && dnfErr == nil && len(dnfItems) > 0 {
		status.Functional = true
		status.Message = "ok"
		return status, nil
	}
	if rpmErr == nil && rpmCount > 0 {
		status.Functional = true
		status.ItemCount = rpmCount
		status.Message = "dnf indisponible, repli rpm disponible"
		return status, nil
	}
	if dnfErr != nil {
		status.Message = "dnf indisponible: " + dnfErr.Error()
	} else {
		status.Message = "aucun paquet détecté"
	}
	if rpmErr != nil {
		status.Message += "; rpm indisponible: " + rpmErr.Error()
	}
	return status, nil
}

func (a *Adapter) Capture() (*adapter.CaptureResult, error) {
	result := &adapter.CaptureResult{
		Manager: a.Name(),
		Raw:     make(map[string]any),
	}

	dnfName, dnfPresent := a.dnfCommand()
	var dnfItems []adapter.Package
	var dnfErr error
	if dnfPresent {
		dnfItems, dnfErr = a.queryDNF(dnfName)
	} else {
		dnfErr = errors.New("dnf et dnf5 introuvables")
	}

	rpmOutput, rpmCountErr := a.run("rpm", "-qa")
	rpmCount := lineCount(rpmOutput)
	if dnfErr == nil && len(dnfItems) > 0 && rpmCountErr == nil && !isInconsistent(len(dnfItems), rpmCount) {
		result.Level = adapter.Level1
		result.Reliability = adapter.ReliabilityHigh
		result.Packages = dnfItems
		result.Raw["rpm_item_count"] = rpmCount
		return result, nil
	}

	if dnfErr != nil {
		result.Warnings = append(result.Warnings, "niveau 1 indisponible : "+dnfErr.Error()+" — bascule au niveau 2")
	} else if rpmCountErr == nil && rpmCount > 100 && isInconsistent(len(dnfItems), rpmCount) {
		result.Warnings = append(result.Warnings, fmt.Sprintf(
			"niveau 1 incohérent : %d renvoyés par dnf vs %d installés via rpm — bascule au niveau 2",
			len(dnfItems), rpmCount,
		))
	} else {
		result.Warnings = append(result.Warnings, "niveau 1 vide ou invérifiable — bascule au niveau 2")
	}

	level2Packages, level2Err := a.captureRPM()
	if level2Err == nil && len(level2Packages) > 0 {
		result.Level = adapter.Level2
		result.Reliability = adapter.ReliabilityMedium
		result.Packages = level2Packages
		return result, nil
	}
	if level2Err == nil {
		level2Err = errors.New("rpm n'a renvoyé aucun paquet de premier niveau")
	}
	result.Warnings = append(result.Warnings, "niveau 2 indisponible : "+level2Err.Error()+" — bascule au niveau 3")

	level3Packages, level3Err := a.captureHistory()
	if level3Err != nil {
		return nil, errors.Join(
			fmt.Errorf("capture dnf impossible (niveau 1: %v; niveau 2: %v)", dnfErr, level2Err),
			fmt.Errorf("niveau 3: %w", level3Err),
		)
	}
	result.Level = adapter.Level3
	result.Reliability = adapter.ReliabilityLow
	result.Packages = level3Packages
	return result, nil
}

func (a *Adapter) dnfCommand() (string, bool) {
	if _, err := a.run("dnf", "--version"); err == nil {
		return "dnf", true
	}
	if _, err := a.run("dnf5", "--version"); err == nil {
		return "dnf5", true
	}
	return "", false
}

func (a *Adapter) queryDNF(command string) ([]adapter.Package, error) {
	output, err := a.run(command, "repoquery", "--userinstalled", "--qf", "%{name}\t%{version}\n")
	if err != nil {
		return nil, err
	}
	return parsePackages(output, adapter.Level1), nil
}

func (a *Adapter) captureRPM() ([]adapter.Package, error) {
	output, err := a.run("rpm", "-qa", "--qf", "%{name}\t%{version}\n")
	if err != nil {
		return nil, err
	}
	installed := parsePackages(output, adapter.Level2)
	if len(installed) == 0 {
		return nil, errors.New("rpm -qa n'a renvoyé aucun paquet")
	}
	requirements, err := a.run("rpm", "-qa", "--qf", "[%{REQUIRENAME}\n]")
	if err != nil {
		return nil, err
	}
	required := make(map[string]struct{})
	for _, line := range strings.Split(string(requirements), "\n") {
		name := strings.TrimSpace(line)
		if name != "" {
			required[name] = struct{}{}
		}
	}
	topLevel := make([]adapter.Package, 0, len(installed))
	for _, pkg := range installed {
		if _, isRequired := required[pkg.Name]; !isRequired {
			topLevel = append(topLevel, pkg)
		}
	}
	return topLevel, nil
}

func (a *Adapter) captureHistory() ([]adapter.Package, error) {
	path := filepath.Clean(a.historyPath)
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("impossible d'ouvrir l'historique en lecture seule %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return nil, fmt.Errorf("impossible de fermer l'historique %s: %w", path, err)
	}
	uri := (&url.URL{Scheme: "file", Path: path, RawQuery: "mode=ro"}).String()
	query := "SELECT DISTINCT name || char(9) || version FROM rpm ORDER BY name;"
	output, err := a.run("sqlite3", uri, query)
	if err != nil {
		return nil, fmt.Errorf("lecture en lecture seule de l'historique %s impossible: %w", path, err)
	}
	packages := parsePackages(output, adapter.Level3)
	if len(packages) == 0 {
		return nil, fmt.Errorf("aucun paquet exploitable dans l'historique %s", path)
	}
	return packages, nil
}

func parsePackages(output []byte, level adapter.CaptureLevel) []adapter.Package {
	var packages []adapter.Package
	seen := make(map[string]struct{})
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		fields := strings.SplitN(strings.TrimSpace(line), "\t", 2)
		if len(fields) == 0 || fields[0] == "" {
			continue
		}
		key := fields[0]
		version := ""
		if len(fields) == 2 {
			version = fields[1]
			key += "\x00" + version
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		packages = append(packages, adapter.Package{Name: fields[0], Version: version, Source: level})
	}
	return packages
}

func lineCount(output []byte) int {
	count := 0
	for _, line := range strings.Split(string(output), "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}

func isInconsistent(dnfCount, rpmCount int) bool {
	return rpmCount > 100 && (dnfCount == 0 || dnfCount*20 < rpmCount)
}
