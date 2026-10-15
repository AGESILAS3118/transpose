package repology

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	DefaultBaseURL     = "https://repology.org"
	DefaultRateLimit   = time.Second
	DefaultTimeout     = 60 * time.Second
	cacheTTL           = 7 * 24 * time.Hour
	cacheFormatVersion = 2
)

type ProjectPackage struct {
	Repo       string `json:"repo"`
	Name       string `json:"name"`
	Version    string `json:"version"`
	Category   string `json:"category"`
	Maintainer string `json:"maintainer"`
	Upstream   string `json:"www"`
}

func (p *ProjectPackage) UnmarshalJSON(data []byte) error {
	var raw struct {
		Repo        string   `json:"repo"`
		Name        string   `json:"name"`
		BinaryName  string   `json:"binname"`
		VisibleName string   `json:"visiblename"`
		SourceName  string   `json:"srcname"`
		Version     string   `json:"version"`
		Category    string   `json:"category"`
		Maintainer  string   `json:"maintainer"`
		Maintainers []string `json:"maintainers"`
		Upstream    string   `json:"www"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	p.Repo = raw.Repo
	p.Name = raw.BinaryName
	if p.Name == "" {
		p.Name = raw.VisibleName
	}
	if p.Name == "" {
		p.Name = raw.Name
	}
	if p.Name == "" {
		p.Name = raw.SourceName
	}
	p.Version = raw.Version
	p.Category = raw.Category
	p.Maintainer = raw.Maintainer
	if p.Maintainer == "" {
		p.Maintainer = strings.Join(raw.Maintainers, ", ")
	}
	p.Upstream = raw.Upstream
	return nil
}

type Project struct {
	Name     string           `json:"name"`
	Packages []ProjectPackage `json:"packages"`
	NotFound bool             `json:"not_found,omitempty"`
}

type FetchMeta struct {
	CacheHit bool
	NotFound bool
}

type ClientOptions struct {
	BaseURL      string
	CacheDir     string
	UserAgent    string
	HTTPClient   *http.Client
	RateLimit    time.Duration
	RetryBackoff []time.Duration
	Offline      bool
}

type Client struct {
	baseURL      string
	cacheDir     string
	userAgent    string
	httpClient   *http.Client
	rateLimit    time.Duration
	retryBackoff []time.Duration
	offline      bool

	rateMu      sync.Mutex
	nextRequest time.Time
}

type cacheEntry struct {
	FormatVersion int       `json:"format_version"`
	CachedAt      time.Time `json:"cached_at"`
	Project       *Project  `json:"project,omitempty"`
	NotFound      bool      `json:"not_found,omitempty"`
}

func NewClient(cacheDir string, offline bool) *Client {
	return NewClientWithOptions(ClientOptions{CacheDir: cacheDir, Offline: offline})
}

func NewClientWithOptions(options ClientOptions) *Client {
	if options.BaseURL == "" {
		options.BaseURL = DefaultBaseURL
	}
	if options.UserAgent == "" {
		options.UserAgent = "xpose/0.1.0-dev (+https://github.com/transpose-project/transpose)"
	}
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Timeout: DefaultTimeout}
	}
	if options.RateLimit == 0 {
		options.RateLimit = DefaultRateLimit
	}
	if options.RetryBackoff == nil {
		options.RetryBackoff = []time.Duration{2 * time.Second, 4 * time.Second, 8 * time.Second, 16 * time.Second}
	}
	return &Client{
		baseURL:      strings.TrimRight(options.BaseURL, "/"),
		cacheDir:     options.CacheDir,
		userAgent:    options.UserAgent,
		httpClient:   options.HTTPClient,
		rateLimit:    options.RateLimit,
		retryBackoff: append([]time.Duration(nil), options.RetryBackoff...),
		offline:      options.Offline,
	}
}

func (c *Client) Fetch(ctx context.Context, name string) (*Project, FetchMeta, error) {
	if strings.TrimSpace(name) == "" {
		return nil, FetchMeta{}, errors.New("nom de projet Repology vide")
	}
	if entry, err := c.readCache(name); err != nil {
		return nil, FetchMeta{}, err
	} else if entry != nil {
		if entry.NotFound {
			return nil, FetchMeta{CacheHit: true, NotFound: true}, nil
		}
		return entry.Project, FetchMeta{CacheHit: true}, nil
	}
	if c.offline {
		return nil, FetchMeta{}, fmt.Errorf("projet %q absent du cache Repology en mode hors ligne", name)
	}

	endpoint := c.baseURL + "/api/v1/project/" + url.PathEscape(name)
	var lastErr error
	for attempt := 0; attempt <= len(c.retryBackoff); attempt++ {
		if attempt > 0 {
			if err := waitContext(ctx, c.retryBackoff[attempt-1]); err != nil {
				return nil, FetchMeta{}, err
			}
		}
		if err := c.waitRateLimit(ctx); err != nil {
			return nil, FetchMeta{}, err
		}
		project, status, err := c.request(ctx, endpoint, name)
		if err == nil {
			if status == http.StatusNotFound {
				if err := c.writeCache(name, cacheEntry{CachedAt: time.Now().UTC(), NotFound: true}); err != nil {
					return nil, FetchMeta{}, err
				}
				return nil, FetchMeta{NotFound: true}, nil
			}
			if err := c.writeCache(name, cacheEntry{CachedAt: time.Now().UTC(), Project: project}); err != nil {
				return nil, FetchMeta{}, err
			}
			return project, FetchMeta{}, nil
		}
		lastErr = err
		if !isRetryable(err) || attempt == len(c.retryBackoff) {
			break
		}
	}
	return nil, FetchMeta{}, fmt.Errorf("requête Repology pour %q échouée: %w", name, lastErr)
}

func (c *Client) request(ctx context.Context, endpoint, name string) (*Project, int, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, 0, err
	}
	request.Header.Set("User-Agent", c.userAgent)
	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, 0, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusNotFound {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil, response.StatusCode, nil
	}
	if response.StatusCode >= 500 {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil, response.StatusCode, fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, response.StatusCode, fmt.Errorf("HTTP %d: %s", response.StatusCode, strings.TrimSpace(string(body)))
	}
	var packages []ProjectPackage
	if err := json.NewDecoder(response.Body).Decode(&packages); err != nil {
		return nil, response.StatusCode, fmt.Errorf("décodage réponse Repology %q: %w", name, err)
	}
	project := &Project{Name: name, Packages: packages}
	return project, response.StatusCode, nil
}

func (c *Client) waitRateLimit(ctx context.Context) error {
	c.rateMu.Lock()
	defer c.rateMu.Unlock()
	wait := time.Until(c.nextRequest)
	if wait > 0 {
		if err := waitContext(ctx, wait); err != nil {
			return err
		}
	}
	c.nextRequest = time.Now().Add(c.rateLimit + 10*time.Millisecond)
	return nil
}

func (c *Client) cachePath(name string) string {
	return filepath.Join(c.cacheDir, url.PathEscape(name)+".json")
}

func (c *Client) readCache(name string) (*cacheEntry, error) {
	if c.cacheDir == "" {
		return nil, nil
	}
	data, err := os.ReadFile(c.cachePath(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lecture cache Repology %q: %w", name, err)
	}
	var entry cacheEntry
	if err := json.Unmarshal(data, &entry); err != nil {
		return nil, fmt.Errorf("cache Repology invalide pour %q: %w", name, err)
	}
	if entry.FormatVersion != cacheFormatVersion || entry.CachedAt.IsZero() || time.Since(entry.CachedAt) > cacheTTL {
		if err := os.Remove(c.cachePath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("suppression cache expiré %q: %w", name, err)
		}
		return nil, nil
	}
	if !entry.NotFound && entry.Project == nil {
		return nil, fmt.Errorf("cache Repology incomplet pour %q", name)
	}
	if entry.Project != nil {
		for _, pkg := range entry.Project.Packages {
			if pkg.Name == "" {
				if err := os.Remove(c.cachePath(name)); err != nil && !errors.Is(err, os.ErrNotExist) {
					return nil, fmt.Errorf("suppression cache Repology incompatible %q: %w", name, err)
				}
				return nil, nil
			}
		}
	}
	return &entry, nil
}

func (c *Client) writeCache(name string, entry cacheEntry) error {
	if c.cacheDir == "" {
		return nil
	}
	if err := os.MkdirAll(c.cacheDir, 0o755); err != nil {
		return fmt.Errorf("création cache Repology %s: %w", c.cacheDir, err)
	}
	entry.FormatVersion = cacheFormatVersion
	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("encodage cache Repology %q: %w", name, err)
	}
	path := c.cachePath(name)
	tmp, err := os.CreateTemp(c.cacheDir, ".repology-*.tmp")
	if err != nil {
		return fmt.Errorf("création cache temporaire Repology: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("écriture cache Repology %q: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("fermeture cache Repology %q: %w", name, err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("installation cache Repology %q: %w", name, err)
	}
	return nil
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	var netErr net.Error
	return errors.As(err, &netErr) || strings.HasPrefix(err.Error(), "HTTP 5")
}
