// Package config keeps HassTray's settings on disk and the access token in
// the macOS Keychain.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"sync"

	"github.com/zalando/go-keyring"
)

const (
	keyringService = "HassTray"
	keyringUser    = "home-assistant-token"
)

// MaxWidgets is how many entity values fit next to the menu bar icon.
const MaxWidgets = 5

// Themes the interface knows about. The first one is the default.
var Themes = []string{"glass", "midnight", "neon", "sunset"}

// Config is everything HassTray remembers between launches, apart from the
// token.
type Config struct {
	URL      string   `json:"url"`
	Entities []string `json:"entities"`
	Graphs   []string `json:"graphs"`
	Widgets  []string `json:"widgets"`
	Theme    string   `json:"theme"`
}

// Store reads and writes the config file. It is safe to use from several
// goroutines.
type Store struct {
	mu   sync.Mutex
	path string
	cfg  Config
}

// Dir returns the folder the config file lives in. HASSTRAY_CONFIG_DIR
// overrides the default, which is handy for tests and for trying things out.
func Dir() (string, error) {
	if d := os.Getenv("HASSTRAY_CONFIG_DIR"); d != "" {
		return d, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "HassTray"), nil
}

// Open loads the config from dir, or starts with defaults when there is no
// file yet.
func Open(dir string) (*Store, error) {
	s := &Store{path: filepath.Join(dir, "config.json")}
	s.cfg.Theme = Themes[0]
	raw, err := os.ReadFile(s.path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &s.cfg); err != nil {
		return nil, err
	}
	if !slices.Contains(Themes, s.cfg.Theme) {
		s.cfg.Theme = Themes[0]
	}
	if len(s.cfg.Widgets) > MaxWidgets {
		s.cfg.Widgets = s.cfg.Widgets[:MaxWidgets]
	}
	return s, nil
}

// Get returns a copy of the current config.
func (s *Store) Get() Config {
	s.mu.Lock()
	defer s.mu.Unlock()
	c := s.cfg
	c.Entities = slices.Clone(s.cfg.Entities)
	c.Graphs = slices.Clone(s.cfg.Graphs)
	c.Widgets = slices.Clone(s.cfg.Widgets)
	return c
}

// Update changes the config through fn and writes it to disk.
func (s *Store) Update(fn func(*Config)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	fn(&s.cfg)
	return s.save()
}

func (s *Store) save() error {
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(s.cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

// SetSelected adds an entity to the tray or takes it out. Taking it out also
// drops its graph and its menu bar widget.
func (c *Config) SetSelected(id string, on bool) {
	if on {
		if !slices.Contains(c.Entities, id) {
			c.Entities = append(c.Entities, id)
		}
		return
	}
	c.Entities = remove(c.Entities, id)
	c.Graphs = remove(c.Graphs, id)
	c.Widgets = remove(c.Widgets, id)
}

// ErrTooManyWidgets is returned when the menu bar is already full.
var ErrTooManyWidgets = errors.New("the menu bar holds up to 5 widgets; remove one first")

// SetWidget shows an entity's value in the menu bar or takes it out. The
// entity is added to the tray as well, so a widget always has a row behind it.
func (c *Config) SetWidget(id string, on bool) error {
	if !on {
		c.Widgets = remove(c.Widgets, id)
		return nil
	}
	if slices.Contains(c.Widgets, id) {
		return nil
	}
	if len(c.Widgets) >= MaxWidgets {
		return ErrTooManyWidgets
	}
	c.SetSelected(id, true)
	c.Widgets = append(c.Widgets, id)
	return nil
}

// SetGraph turns the sparkline for an entity on or off.
func (c *Config) SetGraph(id string, on bool) {
	if on {
		if !slices.Contains(c.Graphs, id) {
			c.Graphs = append(c.Graphs, id)
		}
		return
	}
	c.Graphs = remove(c.Graphs, id)
}

// Move shifts an entity one place up (delta -1) or down (delta 1) in the tray.
func (c *Config) Move(id string, delta int) {
	i := slices.Index(c.Entities, id)
	j := i + delta
	if i < 0 || j < 0 || j >= len(c.Entities) {
		return
	}
	c.Entities[i], c.Entities[j] = c.Entities[j], c.Entities[i]
}

func remove(list []string, id string) []string {
	return slices.DeleteFunc(slices.Clone(list), func(v string) bool { return v == id })
}

// Token reads the access token from the Keychain. A missing token is not an
// error; it comes back empty.
func Token() (string, error) {
	t, err := keyring.Get(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", nil
	}
	return t, err
}

// SetToken stores the access token in the Keychain.
func SetToken(token string) error {
	return keyring.Set(keyringService, keyringUser, token)
}

// DeleteToken removes the access token from the Keychain.
func DeleteToken() error {
	err := keyring.Delete(keyringService, keyringUser)
	if errors.Is(err, keyring.ErrNotFound) {
		return nil
	}
	return err
}
