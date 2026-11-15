package config

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestOpenDefaultsAndRoundTrip(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Get().Theme; got != "glass" {
		t.Fatalf("default theme = %q, want glass", got)
	}
	err = s.Update(func(c *Config) {
		c.URL = "http://homeassistant.local:8123"
		c.SetSelected("light.desk", true)
		c.SetSelected("sensor.temp", true)
		c.SetGraph("sensor.temp", true)
		_ = c.SetWidget("sensor.temp", true)
		c.Theme = "neon"
	})
	if err != nil {
		t.Fatal(err)
	}
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(again.Get(), s.Get()) {
		t.Fatalf("reloaded config differs: %+v vs %+v", again.Get(), s.Get())
	}
}

func TestUnselectDropsGraphAndWidget(t *testing.T) {
	c := Config{}
	c.SetSelected("sensor.temp", true)
	c.SetGraph("sensor.temp", true)
	_ = c.SetWidget("sensor.temp", true)
	c.SetSelected("sensor.temp", false)
	if len(c.Entities) != 0 || len(c.Graphs) != 0 || len(c.Widgets) != 0 {
		t.Fatalf("leftovers after unselect: %+v", c)
	}
}

func TestWidgetsAreCappedAtFive(t *testing.T) {
	c := Config{}
	for _, id := range []string{"a", "b", "c", "d", "e"} {
		if err := c.SetWidget(id, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.SetWidget("a", true); err != nil {
		t.Fatalf("adding a widget twice should be a no-op, got %v", err)
	}
	if err := c.SetWidget("f", true); err != ErrTooManyWidgets {
		t.Fatalf("sixth widget: got %v, want ErrTooManyWidgets", err)
	}
	if len(c.Widgets) != 5 || len(c.Entities) != 5 {
		t.Fatalf("widgets %v, entities %v", c.Widgets, c.Entities)
	}
	_ = c.SetWidget("b", false)
	if err := c.SetWidget("f", true); err != nil {
		t.Fatal(err)
	}
	want := []string{"a", "c", "d", "e", "f"}
	if !reflect.DeepEqual(c.Widgets, want) {
		t.Fatalf("got %v, want %v", c.Widgets, want)
	}
}

func TestMove(t *testing.T) {
	c := Config{Entities: []string{"a", "b", "c"}}
	c.Move("c", -1)
	c.Move("a", -1) // already first, stays
	c.Move("x", 1)  // unknown, ignored
	want := []string{"a", "c", "b"}
	if !reflect.DeepEqual(c.Entities, want) {
		t.Fatalf("got %v, want %v", c.Entities, want)
	}
}

func TestUnknownThemeFallsBack(t *testing.T) {
	dir := t.TempDir()
	s, _ := Open(dir)
	_ = s.Update(func(c *Config) { c.Theme = "does-not-exist" })
	again, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if again.Get().Theme != "glass" {
		t.Fatalf("theme = %q, want glass", again.Get().Theme)
	}
}

func TestCustomCheck(t *testing.T) {
	ok := DefaultCustom()
	if err := ok.Check(); err != nil {
		t.Fatalf("the defaults should pass: %v", err)
	}
	bad := []CustomTheme{
		{Base: "plaid", Accent: "#0A84FF", Corner: 16, Font: "system"},
		{Base: "glass", Accent: "blue", Corner: 16, Font: "system"},
		{Base: "glass", Accent: "#0A84FF", Surface: "#fff", Corner: 16, Font: "system"},
		{Base: "glass", Accent: "#0A84FF", Corner: 40, Font: "system"},
		{Base: "glass", Accent: "#0A84FF", Corner: 16, Font: "comic"},
		{Base: "custom", Accent: "#0A84FF", Corner: 16, Font: "system"},
	}
	for _, c := range bad {
		if err := c.Check(); err == nil {
			t.Errorf("%+v should be refused", c)
		}
	}
}

func TestBrokenCustomFallsBack(t *testing.T) {
	dir := t.TempDir()
	raw := `{"url":"http://ha.local:8123","theme":"custom","custom":{"base":"glass","accent":"nonsense","corner":99,"font":"comic"}}`
	if err := os.WriteFile(filepath.Join(dir, "config.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	s, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := s.Get()
	if got.Theme != "glass" || got.Custom != DefaultCustom() {
		t.Fatalf("broken custom theme not reset: %+v", got)
	}
	if got.URL != "http://ha.local:8123" {
		t.Fatalf("the rest of the config should survive: %+v", got)
	}
}
