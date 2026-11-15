package hass

import (
	"math"
	"strconv"
	"strings"
)

// Kinds of entity, by what a click on them does.
const (
	KindToggle = "toggle" // flips between two states
	KindAction = "action" // runs once: scenes, scripts, buttons
	KindSensor = "sensor" // read-only
)

// Kind says how the tray should treat an entity of this domain.
func Kind(domain string) string {
	switch domain {
	case "light", "switch", "input_boolean", "fan", "automation", "humidifier",
		"siren", "group", "media_player", "climate", "cover", "lock", "vacuum":
		return KindToggle
	case "scene", "script", "button", "input_button":
		return KindAction
	}
	return KindSensor
}

// IsOn reports whether a state should be drawn as active.
func IsOn(state string) bool {
	switch state {
	case "on", "open", "opening", "playing", "unlocked", "cleaning",
		"heat", "cool", "heat_cool", "auto", "dry", "fan_only", "home":
		return true
	}
	return false
}

// Action picks the service a click should call for an entity in this state.
func Action(domain, state string) (svcDomain, service string, ok bool) {
	switch domain {
	case "light", "switch", "input_boolean", "fan", "automation", "humidifier",
		"siren", "group", "media_player", "climate":
		return "homeassistant", "toggle", true
	case "cover":
		return "cover", "toggle", true
	case "lock":
		if state == "locked" {
			return "lock", "unlock", true
		}
		return "lock", "lock", true
	case "vacuum":
		if state == "cleaning" {
			return "vacuum", "return_to_base", true
		}
		return "vacuum", "start", true
	case "scene", "script":
		return domain, "turn_on", true
	case "button", "input_button":
		return domain, "press", true
	}
	return "", "", false
}

// Numeric reports whether the state is a number, and the number.
func Numeric(state string) (float64, bool) {
	v, err := strconv.ParseFloat(state, 64)
	if err != nil || math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}
	return v, true
}

// FormatNumber prints a reading without the long tail of decimals sensors
// like to send: 21.437 becomes 21.4, 1250.0 becomes 1250.
func FormatNumber(v float64) string {
	abs := math.Abs(v)
	prec := 1
	switch {
	case abs >= 1000:
		prec = 0
	case abs < 10 && abs != math.Trunc(abs):
		prec = 2
	}
	s := strconv.FormatFloat(v, 'f', prec, 64)
	if strings.Contains(s, ".") {
		s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	}
	if s == "-0" {
		s = "0"
	}
	return s
}

// Value is the state as shown on a sensor row: "12.4 °C", "48%", "Home".
func Value(state, unit string) string {
	if v, ok := Numeric(state); ok {
		n := FormatNumber(v)
		switch {
		case unit == "":
			return n
		case unit == "%":
			return n + "%"
		default:
			return n + " " + unit
		}
	}
	return Pretty(state)
}

// Short is the state as it fits in the menu bar: "12.4°", "3.2 kW".
func Short(state, unit string) string {
	if v, ok := Numeric(state); ok {
		n := FormatNumber(v)
		switch {
		case strings.HasPrefix(unit, "°"):
			return n + "°"
		case unit == "%":
			return n + "%"
		case unit == "":
			return n
		default:
			return n + " " + unit
		}
	}
	return Pretty(state)
}

// Pretty turns "heat_cool" into "Heat cool".
func Pretty(state string) string {
	if state == "" {
		return "Unknown"
	}
	s := strings.ReplaceAll(state, "_", " ")
	return strings.ToUpper(s[:1]) + s[1:]
}

// Describe is the small line under a control's name: "On · 80%",
// "Cooling · 22°", "Closed".
func Describe(s State) string {
	if s.State == "unavailable" || s.State == "unknown" {
		return Pretty(s.State)
	}
	switch s.Domain() {
	case "light":
		if b, ok := s.Num("brightness"); ok && s.State == "on" {
			return "On · " + strconv.Itoa(int(math.Round(b/255*100))) + "%"
		}
	case "cover":
		if p, ok := s.Num("current_position"); ok && p > 0 && p < 100 {
			return Pretty(s.State) + " · " + strconv.Itoa(int(math.Round(p))) + "%"
		}
	case "climate":
		label := Pretty(s.State)
		if a := s.Str("hvac_action"); a != "" && a != "off" && s.State != "off" {
			label = Pretty(a)
		}
		if t, ok := s.Num("temperature"); ok && s.State != "off" {
			return label + " · " + FormatNumber(t) + "°"
		}
		return label
	case "media_player":
		if t := s.Str("media_title"); t != "" && s.State == "playing" {
			return t
		}
	case "scene", "script", "button", "input_button":
		return "Tap to run"
	}
	return Pretty(s.State)
}
