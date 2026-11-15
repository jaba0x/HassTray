package hass

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"homeassistant.local:8123/":       "http://homeassistant.local:8123",
		" https://ha.example.com/ ":       "https://ha.example.com",
		"http://10.0.0.5:8123/lovelace?x": "http://10.0.0.5:8123/lovelace",
	}
	for in, want := range cases {
		got, err := NormalizeURL(in)
		if err != nil || got != want {
			t.Errorf("NormalizeURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{"", "ftp://x", "http://"} {
		if _, err := NormalizeURL(bad); err == nil {
			t.Errorf("NormalizeURL(%q) should fail", bad)
		}
	}
}

func TestWebSocketURL(t *testing.T) {
	if got := WebSocketURL("https://ha.example.com"); got != "wss://ha.example.com/api/websocket" {
		t.Error(got)
	}
	if got := WebSocketURL("http://ha.local:8123"); got != "ws://ha.local:8123/api/websocket" {
		t.Error(got)
	}
}

func TestParseHistorySkipsNonNumbers(t *testing.T) {
	raw := `[[
	  {"entity_id":"sensor.t","state":"20.5","last_changed":"2026-11-15T10:00:00+00:00"},
	  {"state":"unavailable","last_changed":"2026-11-15T10:05:00+00:00"},
	  {"state":"21.5","last_changed":"2026-11-15T10:10:00.123456+00:00"}
	]]`
	pts, err := ParseHistory([]byte(raw))
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 2 || pts[0].V != 20.5 || pts[1].V != 21.5 {
		t.Fatalf("got %+v", pts)
	}
	if empty, err := ParseHistory([]byte(`[]`)); err != nil || empty != nil {
		t.Fatalf("empty history: %v %v", empty, err)
	}
}

func TestDownsample(t *testing.T) {
	t0 := time.Date(2026, 11, 15, 0, 0, 0, 0, time.UTC)
	pts := []Point{
		{t0.Add(-time.Hour), 5},                 // before the window: starting value
		{t0.Add(30 * time.Minute), 10},          // bucket 0
		{t0.Add(45 * time.Minute), 20},          // bucket 0
		{t0.Add(3*time.Hour + time.Minute), 40}, // bucket 3
	}
	got := Downsample(pts, t0, t0.Add(4*time.Hour), 4)
	want := []float64{15, 15, 15, 40}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	if Downsample(nil, t0, t0.Add(time.Hour), 4) != nil {
		t.Fatal("no readings should give nil")
	}
}

func TestSummarize(t *testing.T) {
	s := Summarize([]float64{2, 4, 9})
	if s.Min != 2 || s.Max != 9 || s.Avg != 5 {
		t.Fatalf("got %+v", s)
	}
}

func TestFormatting(t *testing.T) {
	cases := []struct{ state, unit, value, short string }{
		{"21.437", "°C", "21.4 °C", "21.4°"},
		{"48.0", "%", "48%", "48%"},
		{"1250.4", "W", "1250 W", "1250 W"},
		{"3.218", "kW", "3.22 kW", "3.22 kW"},
		{"heat_cool", "", "Heat cool", "Heat cool"},
		{"7", "", "7", "7"},
	}
	for _, c := range cases {
		if got := Value(c.state, c.unit); got != c.value {
			t.Errorf("Value(%q,%q) = %q, want %q", c.state, c.unit, got, c.value)
		}
		if got := Short(c.state, c.unit); got != c.short {
			t.Errorf("Short(%q,%q) = %q, want %q", c.state, c.unit, got, c.short)
		}
	}
}

func TestDescribe(t *testing.T) {
	cases := []struct {
		s    State
		want string
	}{
		{State{EntityID: "light.desk", State: "on", Attributes: map[string]any{"brightness": 204.0}}, "On · 80%"},
		{State{EntityID: "light.desk", State: "off"}, "Off"},
		{State{EntityID: "cover.pergola", State: "open", Attributes: map[string]any{"current_position": 40.0}}, "Open · 40%"},
		{State{EntityID: "climate.ac", State: "cool", Attributes: map[string]any{"hvac_action": "cooling", "temperature": 22.0}}, "Cooling · 22°"},
		{State{EntityID: "climate.ac", State: "off", Attributes: map[string]any{"temperature": 22.0}}, "Off"},
		{State{EntityID: "scene.movie", State: "2026-11-15T10:00:00+00:00"}, "Tap to run"},
		{State{EntityID: "switch.pump", State: "unavailable"}, "Unavailable"},
	}
	for _, c := range cases {
		if got := Describe(c.s); got != c.want {
			t.Errorf("Describe(%s=%s) = %q, want %q", c.s.EntityID, c.s.State, got, c.want)
		}
	}
}

func TestActionAndKind(t *testing.T) {
	if d, s, ok := Action("lock", "locked"); !ok || d != "lock" || s != "unlock" {
		t.Error(d, s, ok)
	}
	if d, s, ok := Action("light", "on"); !ok || d != "homeassistant" || s != "toggle" {
		t.Error(d, s, ok)
	}
	if _, _, ok := Action("sensor", "5"); ok {
		t.Error("sensors have no action")
	}
	if Kind("scene") != KindAction || Kind("binary_sensor") != KindSensor || Kind("cover") != KindToggle {
		t.Error("wrong kind")
	}
}

func TestClientCallsAndAuth(t *testing.T) {
	var gotPath, gotAuth, gotBody string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		if r.Header.Get("Authorization") != "Bearer good" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api/config":
			_, _ = w.Write([]byte(`{"location_name":"Home","version":"2026.11.1"}`))
		case strings.HasPrefix(r.URL.Path, "/api/services/"):
			var b map[string]any
			_ = json.NewDecoder(r.Body).Decode(&b)
			gotBody, _ = b["entity_id"].(string)
			_, _ = w.Write([]byte(`[]`))
		case strings.HasPrefix(r.URL.Path, "/api/history/period/"):
			if r.URL.Query().Get("filter_entity_id") != "sensor.t" {
				t.Errorf("history query: %s", r.URL.RawQuery)
			}
			_, _ = w.Write([]byte(`[[{"state":"1.5","last_changed":"2026-11-15T10:00:00+00:00"}]]`))
		}
	}))
	defer srv.Close()
	ctx := context.Background()

	if _, err := New(srv.URL, "bad").Info(ctx); err != ErrUnauthorized {
		t.Fatalf("bad token: %v", err)
	}
	c := New(srv.URL, "good")
	info, err := c.Info(ctx)
	if err != nil || info.LocationName != "Home" || info.Version != "2026.11.1" {
		t.Fatalf("info: %+v %v", info, err)
	}
	if err := c.CallService(ctx, "light", "turn_on", map[string]any{"entity_id": "light.desk"}); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/services/light/turn_on" || gotBody != "light.desk" || gotAuth != "Bearer good" {
		t.Fatalf("service call: %s %s %s", gotPath, gotBody, gotAuth)
	}
	pts, err := c.History(ctx, "sensor.t", time.Now().Add(-time.Hour))
	if err != nil || len(pts) != 1 || pts[0].V != 1.5 {
		t.Fatalf("history: %+v %v", pts, err)
	}
}

// fakeHA speaks just enough of the WebSocket API for Live.
func fakeHA(t *testing.T, token string) *httptest.Server {
	up := websocket.Upgrader{}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := up.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer c.Close()
		_ = c.WriteJSON(map[string]string{"type": "auth_required"})
		var auth map[string]string
		if c.ReadJSON(&auth) != nil {
			return
		}
		if auth["access_token"] != token {
			_ = c.WriteJSON(map[string]string{"type": "auth_invalid"})
			return
		}
		_ = c.WriteJSON(map[string]string{"type": "auth_ok"})
		for {
			var m map[string]any
			if c.ReadJSON(&m) != nil {
				return
			}
			switch m["type"] {
			case "get_states":
				_ = c.WriteJSON(map[string]any{"id": m["id"], "type": "result", "success": true,
					"result": []map[string]any{{"entity_id": "light.desk", "state": "off"}}})
			case "subscribe_events":
				_ = c.WriteJSON(map[string]any{"id": m["id"], "type": "result", "success": true})
				_ = c.WriteJSON(map[string]any{"id": m["id"], "type": "event", "event": map[string]any{
					"data": map[string]any{"entity_id": "light.desk",
						"new_state": map[string]any{"entity_id": "light.desk", "state": "on"}}}})
				_ = c.WriteJSON(map[string]any{"id": m["id"], "type": "event", "event": map[string]any{
					"data": map[string]any{"entity_id": "light.gone", "new_state": nil}}})
			}
		}
	}))
}

func TestLiveSnapshotAndChanges(t *testing.T) {
	srv := fakeHA(t, "good")
	defer srv.Close()

	var mu sync.Mutex
	var snapshot []State
	changes := map[string]string{}
	done := make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	l := &Live{
		URL:        WebSocketURL(srv.URL),
		Token:      "good",
		OnSnapshot: func(s []State) { mu.Lock(); snapshot = s; mu.Unlock() },
		OnStatus:   func(bool, error) {},
		OnChange: func(id string, s *State) {
			mu.Lock()
			defer mu.Unlock()
			if s == nil {
				changes[id] = "<removed>"
			} else {
				changes[id] = s.State
			}
			if len(changes) == 2 {
				close(done)
			}
		},
	}
	go l.Run(ctx)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("timed out waiting for state changes")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(snapshot) != 1 || snapshot[0].EntityID != "light.desk" {
		t.Fatalf("snapshot: %+v", snapshot)
	}
	if changes["light.desk"] != "on" || changes["light.gone"] != "<removed>" {
		t.Fatalf("changes: %v", changes)
	}
}

func TestLiveBadToken(t *testing.T) {
	srv := fakeHA(t, "good")
	defer srv.Close()
	got := make(chan error, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	l := &Live{
		URL:        WebSocketURL(srv.URL),
		Token:      "wrong",
		OnSnapshot: func([]State) {},
		OnChange:   func(string, *State) {},
		OnStatus: func(ok bool, err error) {
			select {
			case got <- err:
			default:
			}
		},
	}
	go l.Run(ctx)
	select {
	case err := <-got:
		if err != ErrUnauthorized {
			t.Fatalf("got %v, want ErrUnauthorized", err)
		}
	case <-ctx.Done():
		t.Fatal("timed out")
	}
}
