package repology

import "strings"

var projectNameSuffixes = []string{"-devel", "-libs", "-doc", "-data", "-config"}

func CandidateProjectNames(source string) []string {
	source = strings.TrimSpace(source)
	if source == "" {
		return nil
	}
	names := make([]string, 0, 8)
	add := func(name string) {
		if name == "" {
			return
		}
		for _, existing := range names {
			if existing == name {
				return
			}
		}
		names = append(names, name)
	}
	lower := strings.ToLower(source)
	add(source)
	if strings.HasPrefix(lower, "python3-") {
		base := source[len("python3-"):]
		add("python-" + base)
		add(base)
		if normalized := withoutRepologySuffix(base); normalized != base {
			add("python-" + normalized)
			add(normalized)
		}
	}
	if strings.HasPrefix(lower, "networkmanager-") {
		add(source[:len("NetworkManager")])
		add("networkmanager")
	}
	if strings.HasPrefix(lower, "lib") && len(source) > 3 {
		base := source[3:]
		add(base)
		if normalized := withoutRepologySuffix(base); normalized != base {
			add(normalized)
		}
	}
	if normalized := withoutRepologySuffix(source); normalized != source {
		add(normalized)
	}
	if index := strings.Index(lower, "-plugin-"); index >= 0 {
		add(source[:index])
	}
	add(lower)
	return names
}

func withoutRepologySuffix(name string) string {
	lower := strings.ToLower(name)
	for _, suffix := range projectNameSuffixes {
		if strings.HasSuffix(lower, suffix) && len(name) > len(suffix) {
			return name[:len(name)-len(suffix)]
		}
	}
	return name
}
