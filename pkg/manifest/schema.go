package manifest

import "time"

const SchemaVersion = 1

type Manifest struct {
	Schema           int              `yaml:"schema" json:"schema"`
	Source           Source           `yaml:"source" json:"source"`
	Repos            []Repo           `yaml:"repos" json:"repos"`
	Packages         []Package        `yaml:"packages" json:"packages"`
	Flatpak          FlatpakBlock     `yaml:"flatpak" json:"flatpak"`
	Snaps            []Snap           `yaml:"snaps,omitempty" json:"snaps,omitempty"`
	LanguagePackages LanguagePackages `yaml:"language_packages,omitempty" json:"language_packages,omitempty"`
	ServicesEnabled  []string         `yaml:"services_enabled,omitempty" json:"services_enabled,omitempty"`
	Cron             []CronEntry      `yaml:"cron,omitempty" json:"cron,omitempty"`
	Configs          ConfigsBlock     `yaml:"configs,omitempty" json:"configs,omitempty"`
	Unmanaged        []UnmanagedEntry `yaml:"unmanaged,omitempty" json:"unmanaged,omitempty"`
	Extra            map[string]any   `yaml:",inline" json:"-"`
}

type Source struct {
	Distro       string         `yaml:"distro" json:"distro"`
	Version      string         `yaml:"version" json:"version"`
	Arch         string         `yaml:"arch" json:"arch"`
	Hostname     string         `yaml:"hostname" json:"hostname"`
	CapturedAt   time.Time      `yaml:"captured_at" json:"captured_at"`
	CaptureLevel map[string]int `yaml:"capture_level" json:"capture_level"`
	Extra        map[string]any `yaml:",inline" json:"-"`
}

type Repo struct {
	Kind         string         `yaml:"kind" json:"kind"`
	Name         string         `yaml:"name" json:"name"`
	URL          string         `yaml:"url,omitempty" json:"url,omitempty"`
	FileName     string         `yaml:"file_name,omitempty" json:"file_name,omitempty"`
	Content      string         `yaml:"content,omitempty" json:"content,omitempty"`
	Enabled      bool           `yaml:"enabled" json:"enabled"`
	ThirdParty   bool           `yaml:"third_party" json:"third_party"`
	GPGKeyURL    string         `yaml:"gpg_key_url,omitempty" json:"gpg_key_url,omitempty"`
	GPGKeySHA256 string         `yaml:"gpg_key_sha256,omitempty" json:"gpg_key_sha256,omitempty"`
	Extra        map[string]any `yaml:",inline" json:"-"`
}

type Package struct {
	Name     string         `yaml:"name" json:"name"`
	Manager  string         `yaml:"manager" json:"manager"`
	Upstream string         `yaml:"upstream,omitempty" json:"upstream,omitempty"`
	Version  string         `yaml:"version,omitempty" json:"version,omitempty"`
	Extra    map[string]any `yaml:",inline" json:"-"`
}

type FlatpakBlock struct {
	Remotes   []FlatpakRemote   `yaml:"remotes" json:"remotes"`
	Apps      []FlatpakApp      `yaml:"apps" json:"apps"`
	Overrides []FlatpakOverride `yaml:"overrides" json:"overrides"`
	Extra     map[string]any    `yaml:",inline" json:"-"`
}

type FlatpakRemote struct {
	Name  string         `yaml:"name" json:"name"`
	URL   string         `yaml:"url,omitempty" json:"url,omitempty"`
	Scope string         `yaml:"scope" json:"scope"`
	Extra map[string]any `yaml:",inline" json:"-"`
}

type FlatpakApp struct {
	Ref   string         `yaml:"ref" json:"ref"`
	Scope string         `yaml:"scope" json:"scope"`
	Extra map[string]any `yaml:",inline" json:"-"`
}

type FlatpakOverride struct {
	Ref   string         `yaml:"ref,omitempty" json:"ref,omitempty"`
	Scope string         `yaml:"scope" json:"scope"`
	Args  []string       `yaml:"args,omitempty" json:"args,omitempty"`
	Extra map[string]any `yaml:",inline" json:"-"`
}

type Snap struct {
	Name    string         `yaml:"name" json:"name"`
	Channel string         `yaml:"channel,omitempty" json:"channel,omitempty"`
	Scope   string         `yaml:"scope,omitempty" json:"scope,omitempty"`
	Extra   map[string]any `yaml:",inline" json:"-"`
}

type LanguagePackages struct {
	Pipx  []string       `yaml:"pipx,omitempty" json:"pipx,omitempty"`
	Cargo []string       `yaml:"cargo,omitempty" json:"cargo,omitempty"`
	Npm   []string       `yaml:"npm,omitempty" json:"npm,omitempty"`
	Extra map[string]any `yaml:",inline" json:"-"`
}

type CronEntry struct {
	Schedule string         `yaml:"schedule" json:"schedule"`
	Command  string         `yaml:"command" json:"command"`
	User     string         `yaml:"user,omitempty" json:"user,omitempty"`
	Extra    map[string]any `yaml:",inline" json:"-"`
}

type ConfigsBlock struct {
	Dotfiles   []string       `yaml:"dotfiles,omitempty" json:"dotfiles,omitempty"`
	AppConfigs []string       `yaml:"app_configs,omitempty" json:"app_configs,omitempty"`
	Extra      map[string]any `yaml:",inline" json:"-"`
}

type UnmanagedEntry struct {
	Kind  string         `yaml:"kind" json:"kind"`
	Path  string         `yaml:"path" json:"path"`
	Extra map[string]any `yaml:",inline" json:"-"`
}
