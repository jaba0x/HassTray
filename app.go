package main

import (
	"context"
	"errors"
	"slices"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/jaba0x/hasstray/internal/config"
	"github.com/jaba0x/hasstray/internal/hass"
)

const (
	sparkPoints  = 48
	sparkWindow  = 24 * time.Hour
	sparkRefresh = 5 * time.Minute
)

// App is the state behind the tray: the saved settings, the live copy of
// Home Assistant's entities and the small graphs.
type App struct {
	store *config.Store

	mu        sync.RWMutex
	token     string
	client    *hass.Client
	states    map[string]hass.State
	connected bool
	lastErr   string
	info      hass.Info
	sparks    map[string][]float64
	stopLive  context.CancelFunc

	rev      atomic.Int64 // goes up on every change, so the UI can skip redraws
	onChange func()       // tells the tray to refresh its label
}

func NewApp(store *config.Store) *App {
	return &App{store: store, states: map[string]hass.State{}, sparks: map[string][]float64{}, onChange: func() {}}
}

// Start connects with the saved address and token, if there are any.
func (a *App) Start() {
	token, err := config.Token()
	if err != nil {
		a.setError("couldn't read the access token from the Keychain")
		return
	}
	cfg := a.store.Get()
	if cfg.URL == "" || token == "" {
		return
	}
	a.startLive(cfg.URL, token)
}

func (a *App) changed() {
	a.rev.Add(1)
	a.onChange()
}

func (a *App) setError(msg string) {
	a.mu.Lock()
	a.lastErr = msg
	a.mu.Unlock()
	a.changed()
}

func (a *App) startLive(url, token string) {
	ctx, cancel := context.WithCancel(context.Background())
	a.mu.Lock()
	if a.stopLive != nil {
		a.stopLive()
	}
	a.stopLive = cancel
	a.token = token
	a.client = hass.New(url, token)
	a.states = map[string]hass.State{}
	a.sparks = map[string][]float64{}
	a.connected = false
	a.lastErr = ""
	client := a.client
	a.mu.Unlock()
	a.changed()

	live := &hass.Live{
		URL:   hass.WebSocketURL(url),
		Token: token,
		OnSnapshot: func(all []hass.State) {
			m := make(map[string]hass.State, len(all))
			for _, s := range all {
				m[s.EntityID] = s
			}
			a.mu.Lock()
			a.states = m
			a.mu.Unlock()
			a.changed()
		},
		OnChange: func(id string, s *hass.State) {
			a.mu.Lock()
			if s == nil {
				delete(a.states, id)
			} else {
				a.states[id] = *s
			}
			a.mu.Unlock()
			if slices.Contains(a.store.Get().Entities, id) {
				a.changed()
			}
		},
		OnStatus: func(ok bool, err error) {
			a.mu.Lock()
			a.connected = ok
			if err != nil {
				a.lastErr = err.Error()
			} else {
				a.lastErr = ""
			}
			a.mu.Unlock()
			a.changed()
			if ok {
				go a.refreshSparks(ctx, client)
			}
		},
	}
	go live.Run(ctx)
	go func() {
		if info, err := client.Info(ctx); err == nil {
			a.mu.Lock()
			a.info = info
			a.mu.Unlock()
			a.changed()
		}
		t := time.NewTicker(sparkRefresh)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				a.refreshSparks(ctx, client)
			}
		}
	}()
}

// refreshSparks loads the last day of readings for every entity that has its
// graph switched on.
func (a *App) refreshSparks(ctx context.Context, client *hass.Client) {
	now := time.Now()
	from := now.Add(-sparkWindow)
	for _, id := range a.store.Get().Graphs {
		pts, err := client.History(ctx, id, from)
		if err != nil || ctx.Err() != nil {
			continue
		}
		vals := hass.Downsample(pts, from, now, sparkPoints)
		a.mu.Lock()
		if vals == nil {
			delete(a.sparks, id)
		} else {
			a.sparks[id] = vals
		}
		a.mu.Unlock()
	}
	a.changed()
}

func (a *App) refreshSparksSoon() {
	a.mu.RLock()
	client, stop := a.client, a.stopLive
	a.mu.RUnlock()
	if client == nil || stop == nil {
		return
	}
	go a.refreshSparks(context.Background(), client)
}

// Connect checks an address and token, and saves them when they work. An
// empty token means "keep the one already saved".
func (a *App) Connect(rawURL, token string) error {
	url, err := hass.NormalizeURL(rawURL)
	if err != nil {
		return err
	}
	token = strings.TrimSpace(token)
	if token == "" {
		a.mu.RLock()
		token = a.token
		a.mu.RUnlock()
	}
	if token == "" {
		return errors.New("paste a long-lived access token")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	info, err := hass.New(url, token).Info(ctx)
	if err != nil {
		return err
	}
	if err := config.SetToken(token); err != nil {
		return errors.New("couldn't save the token to the Keychain")
	}
	if err := a.store.Update(func(c *config.Config) { c.URL = url }); err != nil {
		return err
	}
	a.mu.Lock()
	a.info = info
	a.mu.Unlock()
	a.startLive(url, token)
	return nil
}

// Disconnect forgets the address and the token. The entity choices stay, so
// reconnecting to the same instance brings the tray back as it was.
func (a *App) Disconnect() error {
	a.mu.Lock()
	if a.stopLive != nil {
		a.stopLive()
		a.stopLive = nil
	}
	a.client, a.token = nil, ""
	a.states = map[string]hass.State{}
	a.sparks = map[string][]float64{}
	a.connected, a.lastErr = false, ""
	a.info = hass.Info{}
	a.mu.Unlock()
	err := errors.Join(
		config.DeleteToken(),
		a.store.Update(func(c *config.Config) { c.URL = "" }),
	)
	a.changed()
	return err
}

// EntityView is one entity as the popover draws it.
type EntityView struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Domain    string    `json:"domain"`
	Kind      string    `json:"kind"`
	Class     string    `json:"class"`
	On        bool      `json:"on"`
	Available bool      `json:"available"`
	Value     string    `json:"value"`
	Detail    string    `json:"detail"`
	Numeric   bool      `json:"numeric"`
	Graph     bool      `json:"graph"`
	Widget    bool      `json:"widget"`
	Spark     []float64 `json:"spark,omitempty"`
}

// Snapshot is everything the popover needs for one redraw.
type Snapshot struct {
	Rev        int64              `json:"rev"`
	Version    string             `json:"version"`
	Configured bool               `json:"configured"`
	Connected  bool               `json:"connected"`
	Error      string             `json:"error"`
	URL        string             `json:"url"`
	Name       string             `json:"name"`
	HAVersion  string             `json:"haVersion"`
	Theme      string             `json:"theme"`
	Themes     []string           `json:"themes"`
	Presets    []string           `json:"presets"`
	Fonts      []string           `json:"fonts"`
	Custom     config.CustomTheme `json:"custom"`
	Total      int                `json:"total"`
	Widgets    int                `json:"widgets"`
	MaxWidgets int                `json:"maxWidgets"`
	Entities   []EntityView       `json:"entities"`
	Update     UpdateInfo         `json:"update"`
}

func (a *App) view(s hass.State, cfg config.Config) EntityView {
	domain := s.Domain()
	unit := s.Str("unit_of_measurement")
	_, numeric := hass.Numeric(s.State)
	available := s.State != "unavailable" && s.State != "unknown" && s.State != ""
	return EntityView{
		ID:        s.EntityID,
		Name:      s.Name(),
		Domain:    domain,
		Kind:      hass.Kind(domain),
		Class:     s.Str("device_class"),
		On:        hass.IsOn(s.State),
		Available: available,
		Value:     hass.Value(s.State, unit),
		Detail:    hass.Describe(s),
		Numeric:   numeric,
		Graph:     slices.Contains(cfg.Graphs, s.EntityID),
		Widget:    slices.Contains(cfg.Widgets, s.EntityID),
	}
}

func (a *App) Snapshot() Snapshot {
	cfg := a.store.Get()
	a.mu.RLock()
	defer a.mu.RUnlock()
	snap := Snapshot{
		Rev:        a.rev.Load(),
		Version:    Version,
		Configured: cfg.URL != "" && a.token != "",
		Connected:  a.connected,
		Error:      a.lastErr,
		URL:        cfg.URL,
		Name:       a.info.LocationName,
		HAVersion:  a.info.Version,
		Theme:      cfg.Theme,
		Themes:     config.Themes,
		Presets:    config.Presets,
		Fonts:      config.Fonts,
		Custom:     cfg.Custom,
		Total:      len(a.states),
		Widgets:    len(cfg.Widgets),
		MaxWidgets: config.MaxWidgets,
		Entities:   []EntityView{},
		Update:     lastUpdate(),
	}
	for _, id := range cfg.Entities {
		s, ok := a.states[id]
		if !ok {
			// still listed, so it can be removed, but greyed out
			s = hass.State{EntityID: id, State: "unavailable"}
		}
		v := a.view(s, cfg)
		if v.Graph {
			v.Spark = a.sparks[id]
		}
		snap.Entities = append(snap.Entities, v)
	}
	return snap
}

// PickerEntity is one row in the entity picker.
type PickerEntity struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Domain   string `json:"domain"`
	Value    string `json:"value"`
	Numeric  bool   `json:"numeric"`
	Selected bool   `json:"selected"`
	Graph    bool   `json:"graph"`
	Widget   bool   `json:"widget"`
}

// AllEntities lists every entity Home Assistant has, picked ones first.
func (a *App) AllEntities() []PickerEntity {
	cfg := a.store.Get()
	a.mu.RLock()
	out := make([]PickerEntity, 0, len(a.states))
	for _, s := range a.states {
		_, numeric := hass.Numeric(s.State)
		out = append(out, PickerEntity{
			ID:       s.EntityID,
			Name:     s.Name(),
			Domain:   s.Domain(),
			Value:    hass.Value(s.State, s.Str("unit_of_measurement")),
			Numeric:  numeric,
			Selected: slices.Contains(cfg.Entities, s.EntityID),
			Graph:    slices.Contains(cfg.Graphs, s.EntityID),
			Widget:   slices.Contains(cfg.Widgets, s.EntityID),
		})
	}
	a.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Selected != out[j].Selected {
			return out[i].Selected
		}
		if out[i].Domain != out[j].Domain {
			return out[i].Domain < out[j].Domain
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out
}

// Act runs the click action of an entity: toggle a light, run a scene.
func (a *App) Act(id string) error {
	a.mu.RLock()
	client := a.client
	s, ok := a.states[id]
	a.mu.RUnlock()
	if client == nil {
		return errors.New("not connected to Home Assistant")
	}
	if !ok {
		return errors.New("Home Assistant no longer has " + id)
	}
	domain, service, ok := hass.Action(s.Domain(), s.State)
	if !ok {
		return errors.New(s.Name() + " can't be controlled from here")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return client.CallService(ctx, domain, service, map[string]any{"entity_id": id})
}

// HistoryView is the data for the large graph.
type HistoryView struct {
	ID     string     `json:"id"`
	Name   string     `json:"name"`
	Unit   string     `json:"unit"`
	Value  string     `json:"value"`
	Hours  int        `json:"hours"`
	From   int64      `json:"from"` // Unix milliseconds
	To     int64      `json:"to"`
	Values []float64  `json:"values"`
	Stats  hass.Stats `json:"stats"`
}

// History loads the readings for the large graph. hours is 1, 24 or 168.
func (a *App) History(id string, hours int) (HistoryView, error) {
	if hours != 1 && hours != 24 && hours != 168 {
		hours = 24
	}
	a.mu.RLock()
	client := a.client
	s := a.states[id]
	a.mu.RUnlock()
	if client == nil {
		return HistoryView{}, errors.New("not connected to Home Assistant")
	}
	now := time.Now()
	from := now.Add(-time.Duration(hours) * time.Hour)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	pts, err := client.History(ctx, id, from)
	if err != nil {
		return HistoryView{}, err
	}
	vals := hass.Downsample(pts, from, now, 120)
	unit := s.Str("unit_of_measurement")
	if s.EntityID == "" {
		s.EntityID = id
	}
	return HistoryView{
		ID:     id,
		Name:   s.Name(),
		Unit:   unit,
		Value:  hass.Value(s.State, unit),
		Hours:  hours,
		From:   from.UnixMilli(),
		To:     now.UnixMilli(),
		Values: vals,
		Stats:  hass.Summarize(vals),
	}, nil
}

// TrayLabel is the text next to the menu bar icon: the values of the
// widgets, in the order they were added. It is "" when there are none.
func (a *App) TrayLabel() string {
	ids := a.store.Get().Widgets
	if len(ids) == 0 {
		return ""
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	if !a.connected {
		return ""
	}
	parts := make([]string, 0, len(ids))
	for _, id := range ids {
		s, ok := a.states[id]
		if !ok || s.State == "unavailable" || s.State == "unknown" || s.State == "" {
			parts = append(parts, "–")
			continue
		}
		parts = append(parts, hass.Short(s.State, s.Str("unit_of_measurement")))
	}
	return strings.Join(parts, "  ")
}

// SetSelected shows or hides an entity in the tray.
func (a *App) SetSelected(id string, on bool) error {
	err := a.store.Update(func(c *config.Config) { c.SetSelected(id, on) })
	a.changed()
	return err
}

// SetGraph turns an entity's graph on or off.
func (a *App) SetGraph(id string, on bool) error {
	err := a.store.Update(func(c *config.Config) { c.SetGraph(id, on) })
	if !on {
		a.mu.Lock()
		delete(a.sparks, id)
		a.mu.Unlock()
	}
	a.changed()
	if on {
		a.refreshSparksSoon()
	}
	return err
}

// SetWidget shows an entity's value in the menu bar, or takes it out.
func (a *App) SetWidget(id string, on bool) error {
	var werr error
	err := a.store.Update(func(c *config.Config) { werr = c.SetWidget(id, on) })
	a.changed()
	return errors.Join(werr, err)
}

// Move shifts an entity up or down in the tray.
func (a *App) Move(id string, delta int) error {
	err := a.store.Update(func(c *config.Config) { c.Move(id, delta) })
	a.changed()
	return err
}

// SetCustom saves a theme the user mixed themselves and switches to it.
func (a *App) SetCustom(c config.CustomTheme) error {
	if err := c.Check(); err != nil {
		return err
	}
	err := a.store.Update(func(cfg *config.Config) {
		cfg.Custom = c
		cfg.Theme = "custom"
	})
	a.changed()
	return err
}

// SetTheme switches the look of the popover.
func (a *App) SetTheme(theme string) error {
	if !slices.Contains(config.Themes, theme) {
		return errors.New("unknown theme")
	}
	err := a.store.Update(func(c *config.Config) { c.Theme = theme })
	a.changed()
	return err
}
