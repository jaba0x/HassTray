// Package config keeps HassTray's settings on disk and the access token in
// the macOS Keychain.
package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
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

// Presets are the themes that ship with HassTray. The first is the default.
var Presets = []string{"glass", "midnight", "neon", "sunset", "paper", "slate", "forest", "rose"}

// Themes is every value the Theme field accepts: the presets plus "custom",
// which draws its colours from the Custom field.
var Themes = append(append([]string{}, Presets...), "custom")

// Fonts the custom theme can use.
var Fonts = []string{"system", "rounded", "mono", "serif"}

// CustomTheme is a theme the user mixed themselves. Base decides the shape of
// everything the other fields don't set.
type CustomTheme struct {
	Base    string `json:"base"`
	Accent  string `json:"accent"`
	Surface string `json:"surface"` // "" keeps the base theme's background
	Corner  int    `json:"corner"`  // corner radius in px
	Font    string `json:"font"`
}

// DefaultCustom is where the theme editor starts.
func DefaultCustom() CustomTheme {
	return CustomTheme{Base: "glass", Accent: "#0A84FF", Surface: "", Corner: 16, Font: "system"}
}

var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// Check reports what is wrong with a custom theme, if anything.
func (c CustomTheme) Check() error {
	if !slices.Contains(Presets, c.Base) {
		return errors.New("pick a theme to start from")
	}
	if !hexColor.MatchString(c.Accent) {
		return errors.New("the accent colour has to look like #0A84FF")
	}
	if c.Surface != "" && !hexColor.MatchString(c.Surface) {
		return errors.New("the background colour has to look like #0A0E17")
	}
	if c.Corner < 0 || c.Corner > 26 {
		return errors.New("corners go from 0 to 26")
	}
	if !slices.Contains(Fonts, c.Font) {
		return errors.New("unknown font")
	}
	return nil
}

// Config is everything HassTray remembers between launches, apart from the
// token.
type Config struct {
	URL      string   `json:"url"`
	Entities []string `json:"entities"`
	Graphs   []string `json:"graphs"`
	Widgets  []string `json:"widgets"`
	Theme    string   `json:"theme"`

	Custom CustomTheme `json:"custom"`
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
	s.cfg.Theme = Presets[0]
	s.cfg.Custom = DefaultCustom()
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
		s.cfg.Theme = Presets[0]
	}
	// a custom theme from a newer version, or one edited by hand, falls back
	// to the defaults rather than stopping the app from starting
	if s.cfg.Custom.Check() != nil {
		s.cfg.Custom = DefaultCustom()
		if s.cfg.Theme == "custom" {
			s.cfg.Theme = Presets[0]
		}
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
