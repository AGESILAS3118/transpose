package resolver

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

type Override struct {
	SourceName    string `yaml:"source_name"`
	TargetName    string `yaml:"target_name,omitempty"`
	TargetFlatpak string `yaml:"target_flatpak,omitempty"`
	Manager       string `yaml:"manager,omitempty"`
}

type Overrides struct {
	Overrides []Override `yaml:"overrides"`
}

func LoadOverrides(path string) (*Overrides, error) {
	file, err := os.Open(path)
	if errors.Is(err, os.ErrNotExist) {
		return &Overrides{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("ouverture des overrides %s: %w", path, err)
	}
	defer file.Close()

	var overrides Overrides
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&overrides); err != nil {
		return nil, fmt.Errorf("lecture des overrides %s: %w", path, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, fmt.Errorf("les overrides %s contiennent plusieurs documents", path)
		}
		return nil, fmt.Errorf("lecture des overrides %s: %w", path, err)
	}
	for index, override := range overrides.Overrides {
		if strings.TrimSpace(override.SourceName) == "" {
			return nil, fmt.Errorf("overrides[%d].source_name ne peut pas être vide", index)
		}
		if strings.TrimSpace(override.TargetName) == "" && strings.TrimSpace(override.TargetFlatpak) == "" {
			return nil, fmt.Errorf("overrides[%d] doit définir target_name ou target_flatpak", index)
		}
	}
	return &overrides, nil
}

func (o *Overrides) Lookup(name string) (Override, bool) {
	if o == nil {
		return Override{}, false
	}
	for _, override := range o.Overrides {
		if strings.EqualFold(strings.TrimSpace(override.SourceName), strings.TrimSpace(name)) {
			return override, true
		}
	}
	return Override{}, false
}
