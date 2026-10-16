package manifest

import (
	"bytes"
	"encoding/json"
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
var repoSecretPattern = regexp.MustCompile(`(?i)(password|token|apikey|secret|private[_ -]?key|BEGIN PGP PRIVATE KEY|BEGIN (OPENSSH |RSA |EC )?PRIVATE KEY)`)

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
	if err := validateAndWarn(&m); err != nil {
		return nil, err
	}
	return &m, nil
}

// WriteJSON encodes a manifest as indented JSON, including preserved unknown fields.
func WriteJSON(m *Manifest, w io.Writer) error {
	if m == nil {
		return errors.New("manifeste nil")
	}
	yamlData, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("encodage YAML intermédiaire du manifeste: %w", err)
	}
	var value any
	if err := yaml.Unmarshal(yamlData, &value); err != nil {
		return fmt.Errorf("conversion JSON du manifeste: %w", err)
	}
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return fmt.Errorf("encodage JSON du manifeste: %w", err)
	}
	return nil
}

// ReadJSON decodes and validates a manifest while preserving unknown fields.
func ReadJSON(r io.Reader) (*Manifest, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("lecture JSON du manifeste: %w", err)
	}
	if !json.Valid(data) {
		return nil, errors.New("lecture JSON du manifeste: document JSON invalide")
	}
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	var m Manifest
	if err := decoder.Decode(&m); err != nil {
		return nil, fmt.Errorf("lecture JSON du manifeste: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return nil, errors.New("le manifeste JSON contient plusieurs documents")
		}
		return nil, fmt.Errorf("lecture JSON du manifeste: %w", err)
	}
	if err := validateAndWarn(&m); err != nil {
		return nil, err
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

func validateAndWarn(m *Manifest) error {
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
	for index, repo := range m.Repos {
		if strings.TrimSpace(repo.Name) == "" {
			return nil, fmt.Errorf("repos[%d].name ne peut pas être vide", index)
		}
		switch repo.Kind {
		case "rpm":
			if !strings.HasSuffix(strings.ToLower(repo.FileName), ".repo") &&
				!strings.HasSuffix(strings.ToLower(repo.FileName), ".repo.disabled") {
				return nil, fmt.Errorf("repos[%d].file_name doit finir par .repo", index)
			}
		case "apt":
			if repo.FileName != "" && !strings.HasSuffix(strings.ToLower(repo.FileName), ".list") &&
				!strings.HasSuffix(strings.ToLower(repo.FileName), ".sources") {
				return nil, fmt.Errorf("repos[%d].file_name doit finir par .list ou .sources", index)
			}
		}
		if repoSecretPattern.MatchString(repo.Content) || repoSecretPattern.MatchString(repo.URL) ||
			repoSecretPattern.MatchString(repo.GPGKeyURL) || containsSensitiveKey(reflect.ValueOf(repo.Extra)) {
			return nil, fmt.Errorf("repos[%d] contient un champ sensible interdit", index)
		}
	}
	knownManagers := map[string]struct{}{
		"dnf": {}, "apt": {}, "pacman": {}, "flatpak": {}, "snap": {},
		"repos_rpm": {}, "repos_apt": {},
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
	for _, repo := range m.Repos {
		if repo.ThirdParty && repo.Enabled {
			warnings = append(warnings, "dépôt tiers activé, confirmation manuelle requise avant restauration (PRD §8.5)")
		}
	}
	walkText(reflect.ValueOf(m), func(value string) {
		if secretPattern.MatchString(value) {
			warnings = append(warnings, "texte ressemblant à un secret détecté dans le manifeste")
		}
	})
	return warnings, nil
}

func containsSensitiveKey(value reflect.Value) bool {
	if !value.IsValid() {
		return false
	}
	switch value.Kind() {
	case reflect.Interface, reflect.Pointer:
		if !value.IsNil() {
			return containsSensitiveKey(value.Elem())
		}
	case reflect.Map:
		iter := value.MapRange()
		for iter.Next() {
			if iter.Key().Kind() == reflect.String && repoSecretPattern.MatchString(iter.Key().String()) {
				return true
			}
			if containsSensitiveKey(iter.Value()) {
				return true
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < value.Len(); i++ {
			if containsSensitiveKey(value.Index(i)) {
				return true
			}
		}
	case reflect.Struct:
		for i := 0; i < value.NumField(); i++ {
			if containsSensitiveKey(value.Field(i)) {
				return true
			}
		}
	case reflect.String:
		return repoSecretPattern.MatchString(value.String())
	}
	return false
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
