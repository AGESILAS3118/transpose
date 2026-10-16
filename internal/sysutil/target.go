package sysutil

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

type Target struct {
	Distro   string   `json:"distro"`
	Version  string   `json:"version,omitempty"`
	Arch     string   `json:"arch,omitempty"`
	Family   string   `json:"family"`
	Managers []string `json:"managers,omitempty"`
}

func DetectCurrent() (*Target, error) {
	file, err := os.Open("/etc/os-release")
	if err != nil {
		return nil, fmt.Errorf("lecture /etc/os-release: %w", err)
	}
	defer file.Close()

	values := make(map[string]string)
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		key, value, ok := strings.Cut(strings.TrimSpace(scanner.Text()), "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		if unquoted, err := strconv.Unquote(value); err == nil {
			value = unquoted
		} else if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '\'' && value[len(value)-1] == '\'')) {
			value = value[1 : len(value)-1]
		}
		values[strings.TrimSpace(key)] = value
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("lecture /etc/os-release: %w", err)
	}

	target := &Target{
		Distro:  values["ID"],
		Version: values["VERSION_ID"],
		Arch:    currentArchitecture(),
		Family:  FamilyForDistro(values["ID"], values["ID_LIKE"]),
	}
	for _, manager := range []string{"dnf", "rpm", "apt", "dpkg", "pacman", "zypper"} {
		if _, err := exec.LookPath(manager); err == nil {
			target.Managers = append(target.Managers, manager)
		}
	}
	if target.Family == "" {
		switch {
		case hasManager(target.Managers, "dnf", "rpm"):
			target.Family = "rpm"
		case hasManager(target.Managers, "apt", "dpkg"):
			target.Family = "debian"
		case hasManager(target.Managers, "pacman"):
			target.Family = "arch"
		case hasManager(target.Managers, "zypper"):
			target.Family = "suse"
		}
	}
	if target.Distro == "" {
		return nil, errors.New("ID absent de /etc/os-release")
	}
	if target.Family == "" {
		return nil, fmt.Errorf("famille de distribution inconnue pour %q", target.Distro)
	}
	return target, nil
}

func ParseTarget(value string) (*Target, error) {
	value = strings.TrimSpace(strings.ToLower(value))
	if value == "" {
		return nil, errors.New("cible vide")
	}
	distro, version := splitTarget(value)
	family := FamilyForDistro(distro, "")
	if family == "" {
		return nil, fmt.Errorf("cible inconnue %q", value)
	}
	if version == "" && (strings.HasSuffix(value, "-") || strings.HasSuffix(value, "_")) {
		return nil, fmt.Errorf("version de cible manquante dans %q", value)
	}
	return &Target{
		Distro:   distro,
		Version:  version,
		Arch:     currentArchitecture(),
		Family:   family,
		Managers: managersForFamily(family),
	}, nil
}

func SameFamily(a, b Target) bool {
	return strings.EqualFold(strings.TrimSpace(a.Family), strings.TrimSpace(b.Family)) &&
		strings.TrimSpace(a.Family) != ""
}

func FamilyForDistro(id, idLike string) string {
	for _, value := range append([]string{id}, strings.Fields(idLike)...) {
		value = strings.ToLower(strings.TrimSpace(value))
		switch {
		case value == "fedora", value == "rhel", value == "rocky", value == "rockylinux", value == "almalinux", value == "alma", value == "centos", value == "ol", value == "scientific":
			return "rpm"
		case value == "debian", value == "ubuntu", value == "linuxmint", value == "mint", value == "pop", value == "pop_os", value == "elementary", value == "zorin":
			return "debian"
		case value == "arch", value == "archlinux", value == "manjaro", value == "endeavouros", value == "endeavour":
			return "arch"
		case strings.HasPrefix(value, "opensuse"), value == "suse", value == "sles", value == "sled":
			return "suse"
		}
	}
	return ""
}

func splitTarget(value string) (string, string) {
	distributions := []string{
		"opensuse-tumbleweed", "opensuse-leap", "endeavouros", "rockylinux",
		"almalinux", "archlinux", "linuxmint", "elementary", "scientific",
		"opensuse", "endeavour", "manjaro", "fedora", "debian", "ubuntu",
		"rocky", "rhel", "alma", "centos", "pop_os", "pop", "mint",
		"zorin", "arch", "suse", "sles", "sled", "ol",
	}
	for _, distro := range distributions {
		if value == distro {
			return distro, ""
		}
		for _, separator := range []byte{'-', '_'} {
			prefix := distro + string(separator)
			if strings.HasPrefix(value, prefix) {
				return distro, strings.TrimPrefix(value, prefix)
			}
		}
	}
	return value, ""
}

func managersForFamily(family string) []string {
	switch family {
	case "rpm":
		return []string{"dnf"}
	case "debian":
		return []string{"apt"}
	case "arch":
		return []string{"pacman"}
	case "suse":
		return []string{"zypper"}
	default:
		return nil
	}
}

func currentArchitecture() string {
	if value, err := exec.LookPath("uname"); err == nil {
		output, runErr := exec.Command(value, "-m").Output()
		if runErr == nil {
			return strings.TrimSpace(string(output))
		}
	}
	return ""
}

func hasManager(managers []string, candidates ...string) bool {
	for _, manager := range managers {
		for _, candidate := range candidates {
			if manager == candidate {
				return true
			}
		}
	}
	return false
}
