package profiles

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/kadirbelkuyu/DBRTS/internal/config"
	"gopkg.in/yaml.v3"
)

const defaultDir = "configs"

var fileNameSanitizer = regexp.MustCompile(`[^a-zA-Z0-9-_]`)

// Profile represents a saved connection profile.
type Profile struct {
	Name     string
	Path     string
	Type     string
	Modified time.Time
}

// Manager discovers and persists connection profiles under a directory.
type Manager struct {
	dir string
}

// NewManager constructs a profile manager using the provided directory.
func NewManager(dir string) *Manager {
	if strings.TrimSpace(dir) == "" {
		dir = defaultDir
	}
	return &Manager{dir: dir}
}

// Directory returns the configured profile directory.
func (m *Manager) Directory() string {
	return m.dir
}

// List returns all profiles filtering by type when provided.
func (m *Manager) List(expectedType string) ([]Profile, error) {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var profiles []Profile
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(strings.ToLower(name), ".yaml") && !strings.HasSuffix(strings.ToLower(name), ".yml") {
			continue
		}
		path := filepath.Join(m.dir, name)
		cfg, err := config.LoadConfig(path)
		if err != nil {
			continue
		}
		if expectedType != "" && cfg.Database.Type != expectedType {
			continue
		}
		info, err := entry.Info()
		profiles = append(profiles, Profile{
			Name:     strings.TrimSuffix(name, filepath.Ext(name)),
			Path:     path,
			Type:     cfg.Database.Type,
			Modified: modifiedTime(info, err),
		})
	}

	return profiles, nil
}

func modifiedTime(info os.FileInfo, err error) time.Time {
	if err != nil || info == nil {
		return time.Time{}
	}
	return info.ModTime()
}

// Save persists the provided config under the given alias and returns the resulting profile.
func (m *Manager) Save(alias string, cfg *config.Config) (Profile, error) {
	if cfg == nil {
		return Profile{}, fmt.Errorf("config cannot be nil")
	}

	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return Profile{}, err
	}

	base := strings.TrimSpace(alias)
	if base == "" {
		base = fmt.Sprintf("%s-%s", cfg.Database.Type, time.Now().Format("20060102_150405"))
	}

	base = sanitizeName(base)
	if !strings.HasSuffix(strings.ToLower(base), ".yaml") && !strings.HasSuffix(strings.ToLower(base), ".yml") {
		base += ".yaml"
	}

	path := filepath.Join(m.dir, base)
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return Profile{}, err
	}

	if err := os.WriteFile(path, data, 0o644); err != nil {
		return Profile{}, err
	}

	return Profile{
		Name:     strings.TrimSuffix(base, filepath.Ext(base)),
		Path:     path,
		Type:     cfg.Database.Type,
		Modified: time.Now(),
	}, nil
}

// Load reads a profile by alias or file path.
func (m *Manager) Load(alias string) (*config.Config, error) {
	if strings.TrimSpace(alias) == "" {
		return nil, fmt.Errorf("profile alias cannot be empty")
	}

	path := alias
	if !strings.ContainsRune(alias, os.PathSeparator) {
		path = filepath.Join(m.dir, ensureYAMLExt(alias))
	}

	return config.LoadConfig(path)
}

func ensureYAMLExt(name string) string {
	ext := strings.ToLower(filepath.Ext(name))
	if ext == ".yaml" || ext == ".yml" {
		return name
	}
	return name + ".yaml"
}

func sanitizeName(input string) string {
	cleaned := fileNameSanitizer.ReplaceAllString(input, "_")
	cleaned = strings.Trim(cleaned, "_")
	if cleaned == "" {
		return "profile"
	}
	return cleaned
}

func (m *Manager) Delete(alias string) error {
	if strings.TrimSpace(alias) == "" {
		return fmt.Errorf("profile alias cannot be empty")
	}

	path := alias
	if !strings.ContainsRune(alias, os.PathSeparator) {
		path = filepath.Join(m.dir, ensureYAMLExt(alias))
	}

	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("profile not found: %s", alias)
	}

	return os.Remove(path)
}
