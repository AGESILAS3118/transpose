package apt

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/transpose-project/transpose/internal/adapter"
)

const defaultExtendedStatesPath = "/var/lib/apt/extended_states"

// Adapter implements the package manager adapter for Debian/Ubuntu (APT & dpkg).
type Adapter struct {
	run       adapter.CommandRunner
	statePath string
}

// New creates a new APT adapter using default system paths.
func New(run adapter.CommandRunner) *Adapter {
	return NewWithStatePath(run, defaultExtendedStatesPath)
}

// NewWithStatePath creates a new APT adapter with a custom extended_states path (useful for testing).
func NewWithStatePath(run adapter.CommandRunner, statePath string) *Adapter {
	if run == nil {
		run = adapter.ExecCommandRunner
	}
	if statePath == "" {
		statePath = defaultExtendedStatesPath
	}
	return &Adapter{
		run:       run,
		statePath: statePath,
	}
}

// NewWithPaths provides constructor compatibility if needed.
func NewWithPaths(run adapter.CommandRunner, statePath, _ string) *Adapter {
	return NewWithStatePath(run, statePath)
}

// Name returns the identifier of the adapter.
func (a *Adapter) Name() string { return "apt" }

// Detect checks if apt-get is available or if dpkg has installed packages.
func (a *Adapter) Detect() (bool, error) {
	if _, err := a.run("apt-get", "--version"); err == nil {
		return true, nil
	}
	if _, err := a.run("dpkg", "--version"); err == nil {
		out, err := a.run("dpkg-query", "-W", "-f", "${Package}\n")
		return err == nil && lineCount(out) > 0, nil
	}
	return false, nil
}

// Health checks the availability and consistency of APT and dpkg.
func (a *Adapter) Health() (adapter.HealthStatus, error) {
	status := adapter.HealthStatus{Manager: a.Name()}

	_, aptErr := a.run("apt-get", "--version")
	aptPresent := aptErr == nil
	_, dpkgErr := a.run("dpkg", "--version")
	dpkgInstalled := dpkgErr == nil

	// Count Level 3 (total installed packages via dpkg-query)
	var totalDpkg int
	if dpkgInstalled {
		dpkgOut, err := a.run("dpkg-query", "-W", "-f", "${Package}\t${Version}\t${db:Status-Abbrev}\n")
		if err == nil {
			totalDpkg = lineCount(dpkgOut)
		}
	}

	dpkgPresent := dpkgInstalled && totalDpkg > 0
	status.Present = aptPresent || dpkgPresent
	if !status.Present {
		status.Message = "absent"
		return status, nil
	}

	// Count Level 1 (manual packages via apt-mark showmanual)
	var manualCount int
	var manualErr error
	if aptPresent {
		var manualOut []byte
		manualOut, manualErr = a.run("apt-mark", "showmanual")
		if manualErr == nil {
			manualCount = lineCount(manualOut)
		}
	}

	status.ItemCount = manualCount

	// Inconsistency check: Level 1 == 0 while total dpkg > 100
	if aptPresent && manualErr == nil && manualCount == 0 && totalDpkg > 100 {
		status.Functional = false
		status.Message = fmt.Sprintf("⚠ INCOHÉRENCE DÉTECTÉE — repli recommandé (dpkg: %d)", totalDpkg)
		return status, nil
	}

	if aptPresent && manualErr == nil && manualCount > 0 {
		status.Functional = true
		status.Message = "ok"
		return status, nil
	}

	if dpkgPresent && totalDpkg > 0 {
		status.Functional = true
		status.ItemCount = totalDpkg
		status.Message = "apt indisponible, repli dpkg disponible"
		return status, nil
	}

	if aptErr != nil {
		status.Message = "apt indisponible: " + aptErr.Error()
	} else {
		status.Message = "aucun paquet détecté"
	}
	if dpkgErr != nil {
		status.Message += "; dpkg indisponible: " + dpkgErr.Error()
	}

	return status, nil
}

// Capture captures manually installed packages using a 3-level cascade fallback:
// Level 1 (high): apt-mark showmanual (cross-checked with dpkg-query)
// Level 2 (medium): dpkg-query -W filtered with /var/lib/apt/extended_states (Auto-Installed: 0)
// Level 3 (low): dpkg-query -W (all installed packages, degraded fallback)
func (a *Adapter) Capture() (*adapter.CaptureResult, error) {
	result := &adapter.CaptureResult{
		Manager: a.Name(),
		Raw:     make(map[string]any),
	}

	// 1. Check total installed packages via dpkg-query for consistency validation
	dpkgPkgs, dpkgErr := a.queryDpkgInstalled()
	totalInstalled := len(dpkgPkgs)

	// Attempt Level 1: apt-mark showmanual
	level1Pkgs, err1 := a.queryAptMarkShowmanual(dpkgPkgs)
	if err1 == nil {
		// Check for inconsistency: Level 1 returned 0 but total dpkg > 100
		if len(level1Pkgs) == 0 && totalInstalled > 100 {
			result.Warnings = append(result.Warnings,
				fmt.Sprintf("incohérence détectée : apt-mark showmanual renvoie 0 paquet alors que dpkg en compte %d ; bascule au niveau 2", totalInstalled))
		} else if len(level1Pkgs) > 0 {
			result.Level = adapter.Level1
			result.Reliability = adapter.ReliabilityHigh
			result.Packages = level1Pkgs
			return result, nil
		}
	} else {
		result.Warnings = append(result.Warnings, fmt.Sprintf("apt-mark showmanual échoué (%v) ; bascule au niveau 2", err1))
	}

	// Attempt Level 2: dpkg-query crossed with extended_states
	level2Pkgs, err2 := a.queryExtendedStates(dpkgPkgs)
	if err2 == nil && len(level2Pkgs) > 0 {
		result.Level = adapter.Level2
		result.Reliability = adapter.ReliabilityMedium
		result.Packages = level2Pkgs
		return result, nil
	}
	if err2 != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("croisement extended_states échoué (%v) ; bascule au niveau 3", err2))
	}

	// Attempt Level 3: All installed packages from dpkg-query
	if dpkgErr == nil && len(dpkgPkgs) > 0 {
		result.Warnings = append(result.Warnings, "repli dégradé sur l'ensemble des paquets installés (dpkg-query)")
		result.Level = adapter.Level3
		result.Reliability = adapter.ReliabilityLow
		pkgs := make([]adapter.Package, 0, len(dpkgPkgs))
		for _, p := range dpkgPkgs {
			p.Source = adapter.Level3
			pkgs = append(pkgs, p)
		}
		result.Packages = pkgs
		return result, nil
	}

	// If all levels fail, return a combined error without panic
	return nil, fmt.Errorf("échec de tous les niveaux de capture apt (niveau 1: %v, niveau 2: %v, niveau 3: %v)", err1, err2, dpkgErr)
}

// queryAptMarkShowmanual queries manual packages via apt-mark and cross-references with dpkg info.
func (a *Adapter) queryAptMarkShowmanual(dpkgMap map[string]adapter.Package) ([]adapter.Package, error) {
	out, err := a.run("apt-mark", "showmanual")
	if err != nil {
		return nil, err
	}

	lines := splitNonEmptyLines(out)
	pkgs := make([]adapter.Package, 0, len(lines))
	for _, name := range lines {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		// Strip architecture suffix if present (e.g., "htop:amd64" -> "htop")
		baseName := name
		if idx := strings.IndexByte(name, ':'); idx != -1 {
			baseName = name[:idx]
		}

		ver := ""
		if dp, ok := dpkgMap[name]; ok {
			ver = dp.Version
		} else if dp, ok := dpkgMap[baseName]; ok {
			ver = dp.Version
		}

		pkgs = append(pkgs, adapter.Package{
			Name:    baseName,
			Version: ver,
			Source:  adapter.Level1,
		})
	}

	return pkgs, nil
}

// queryDpkgInstalled queries all installed packages with status "ii".
func (a *Adapter) queryDpkgInstalled() (map[string]adapter.Package, error) {
	out, err := a.run("dpkg-query", "-W", "-f", "${Package}\t${Version}\t${db:Status-Abbrev}\n")
	if err != nil {
		return nil, err
	}

	pkgs := make(map[string]adapter.Package)
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		parts := strings.Split(line, "\t")
		if len(parts) < 3 {
			continue
		}
		pkgName := strings.TrimSpace(parts[0])
		version := strings.TrimSpace(parts[1])
		status := strings.TrimSpace(parts[2])

		// Only retain packages with status starting with "ii" (installed)
		if strings.HasPrefix(status, "ii") {
			pkgs[pkgName] = adapter.Package{
				Name:    pkgName,
				Version: version,
			}
		}
	}

	return pkgs, scanner.Err()
}

// queryExtendedStates reads extended_states and filters dpkg packages where Auto-Installed == 0.
func (a *Adapter) queryExtendedStates(dpkgMap map[string]adapter.Package) ([]adapter.Package, error) {
	if a.statePath == "" {
		return nil, errors.New("statePath not set")
	}

	autoMap, err := ParseExtendedStatesFile(a.statePath)
	if err != nil {
		return nil, err
	}

	var pkgs []adapter.Package
	for name, dpkgPkg := range dpkgMap {
		baseName := name
		if idx := strings.IndexByte(name, ':'); idx != -1 {
			baseName = name[:idx]
		}

		isAuto, found := autoMap[name]
		if !found {
			isAuto, found = autoMap[baseName]
		}

		// In Debian/APT, packages recorded with Auto-Installed: 0 are explicitly installed.
		if found && !isAuto {
			pkgs = append(pkgs, adapter.Package{
				Name:    baseName,
				Version: dpkgPkg.Version,
				Source:  adapter.Level2,
			})
		}
	}

	return pkgs, nil
}

// ParseExtendedStatesFile reads and parses an APT extended_states file.
// Returns a map where key is the package name and value is true if Auto-Installed: 1, false if 0.
func ParseExtendedStatesFile(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseExtendedStates(data), nil
}

// ParseExtendedStates parses the byte content of an extended_states file.
func ParseExtendedStates(data []byte) map[string]bool {
	result := make(map[string]bool)
	scanner := bufio.NewScanner(bytes.NewReader(data))

	var currentPkg string
	var currentAuto bool
	var hasAuto bool

	flush := func() {
		if currentPkg != "" && hasAuto {
			result[currentPkg] = currentAuto
		}
		currentPkg = ""
		currentAuto = false
		hasAuto = false
	}

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])

		switch strings.ToLower(key) {
		case "package":
			currentPkg = val
		case "auto-installed":
			hasAuto = true
			currentAuto = (val == "1")
		}
	}
	flush()

	return result
}

func lineCount(data []byte) int {
	return len(splitNonEmptyLines(data))
}

func splitNonEmptyLines(data []byte) []string {
	var lines []string
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		text := strings.TrimSpace(scanner.Text())
		if text != "" {
			lines = append(lines, text)
		}
	}
	return lines
}
