package manifest

import (
	"errors"
	"fmt"
	"io"
	"log"
	"reflect"
	"regexp"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

var secretPattern = regexp.MustCompile(`(?i)(password|token|BEGIN PRIVATE KEY|BEGIN OPENSSH PRIVATE KEY)`)

func WriteYAML(m *Manifest, w io.Writer) error {
	if m == nil {
		return errors.New("manifeste nil")
	}
	encoder := yaml.NewEncoder(w)
	encoder.SetIndent(2)
	if err := encoder.Encode(m); err != nil {
		return fmt.Errorf("encodage YAML du manifeste: %w", err)
	}
	if err := encoder.Close(); err != nil {
		return fmt.Errorf("finalisation YAML du manifeste: %w", err)
	}
	return nil
}

func ReadYAML(r io.Reader) (*Manifest, error) {
	decoder := yaml.NewDecoder(r)
	var m Manifest
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("lecture YAML du manifeste: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("le manifeste YAML contient plusieurs documents")
		}
		return nil, fmt.Errorf("lecture YAML du manifeste: %w", err)
	}
	warnings, err := ValidateWithWarnings(&m)
	if err != nil {
		return nil, err
	}
	for _, warning := range warnings {
		log.Printf("avertissement manifeste: %s", warning)
	}
	return &m, nil
}

func Validate(m *Manifest) error {
	warnings, err := ValidateWithWarnings(m)
	if err != nil {
		return err
	}
	for _, warning := range warnings {
		log.Printf("avertissement manifeste: %s", warning)
	}
	return nil
}

func ValidateWithWarnings(m *Manifest) ([]string, error) {
	if m == nil {
		return nil, errors.New("manifeste nil")
	}
	if m.Schema != SchemaVersion {
		return nil, fmt.Errorf("schéma inconnu : %d, attendu %d", m.Schema, SchemaVersion)
	}
	if strings.TrimSpace(m.Source.Distro) == "" {
		return nil, errors.New("source.distro ne peut pas être vide")
	}
	for index, pkg := range m.Packages {
		if strings.TrimSpace(pkg.Name) == "" {
			return nil, fmt.Errorf("packages[%d].name ne peut pas être vide", index)
		}
		if strings.TrimSpace(pkg.Manager) == "" {
			return nil, fmt.Errorf("packages[%d].manager ne peut pas être vide", index)
		}
	}
	knownManagers := map[string]struct{}{
		"dnf": {}, "apt": {}, "pacman": {}, "flatpak": {}, "snap": {},
	}
	for manager, level := range m.Source.CaptureLevel {
		if _, ok := knownManagers[manager]; !ok {
			return nil, fmt.Errorf("gestionnaire inconnu dans capture_level : %q", manager)
		}
		if level < 1 || level > 3 {
			return nil, fmt.Errorf("niveau capture_level invalide pour %q : %d (attendu 1, 2 ou 3)", manager, level)
		}
	}
	var warnings []string
	walkText(reflect.ValueOf(m), func(value string) {
		if secretPattern.MatchString(value) {
			warnings = append(warnings, "texte ressemblant à un secret détecté dans le manifeste")
		}
	})
	return warnings, nil
}

func walkText(value reflect.Value, visit func(string)) {
	if !value.IsValid() {
		return
	}
	if value.Type() == reflect.TypeOf(time.Time{}) {
		return
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if !value.IsNil() {
			walkText(value.Elem(), visit)
		}
	case reflect.String:
		visit(value.String())
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			walkText(value.Field(i), visit)
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			walkText(iter.Key(), visit)
			walkText(iter.Value(), visit)
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			walkText(value.Index(i), visit)
		}
	}
}
