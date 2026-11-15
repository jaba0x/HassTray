package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/jaba0x/hasstray/internal/config"
)

func testAPI(t *testing.T) (*App, http.Handler) {
	t.Helper()
	store, err := config.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := NewApp(store)
	pages := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("page")) })
	return a, newAPI(a, shell{hide: func() {}, quit: func() {}}, pages)
}

func call(t *testing.T, h http.Handler, method, path, body string) (int, map[string]any) {
	t.Helper()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(method, path, strings.NewReader(body)))
	var out map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func TestSnapshotBeforeSetup(t *testing.T) {
	_, h := testAPI(t)
	code, snap := call(t, h, "GET", "/api/snapshot", "")
	if code != 200 || snap["configured"] != false || snap["theme"] != "glass" || snap["maxWidgets"] != 5.0 {
		t.Fatalf("%d %v", code, snap)
	}
	if list, ok := snap["entities"].([]any); !ok || len(list) != 0 {
		t.Fatalf("entities should be an empty list, got %v", snap["entities"])
	}
}

func TestThemeRoute(t *testing.T) {
	a, h := testAPI(t)
	if code, _ := call(t, h, "POST", "/api/theme", `{"theme":"neon"}`); code != 200 {
		t.Fatal(code)
	}
	if a.store.Get().Theme != "neon" {
		t.Fatal("theme not saved")
	}
	code, out := call(t, h, "POST", "/api/theme", `{"theme":"plaid"}`)
	if code != 400 || out["error"] == nil {
		t.Fatalf("unknown theme: %d %v", code, out)
	}
}

func TestWidgetRouteStopsAtFive(t *testing.T) {
	a, h := testAPI(t)
	for _, id := range []string{"sensor.a", "sensor.b", "sensor.c", "sensor.d", "sensor.e"} {
		if code, out := call(t, h, "POST", "/api/widget", `{"id":"`+id+`","on":true}`); code != 200 {
			t.Fatalf("%s: %d %v", id, code, out)
		}
	}
	code, out := call(t, h, "POST", "/api/widget", `{"id":"sensor.f","on":true}`)
	if code != 400 || !strings.Contains(out["error"].(string), "up to 5") {
		t.Fatalf("sixth widget: %d %v", code, out)
	}
	if got := len(a.store.Get().Widgets); got != 5 {
		t.Fatalf("widgets = %d", got)
	}
	// the widgets were added to the tray too, and the snapshot shows them greyed out until HA reports them
	_, snap := call(t, h, "GET", "/api/snapshot", "")
	if len(snap["entities"].([]any)) != 5 || snap["widgets"] != 5.0 {
		t.Fatalf("snapshot: %v", snap)
	}
}

func TestSelectAndMove(t *testing.T) {
	a, h := testAPI(t)
	call(t, h, "POST", "/api/select", `{"id":"light.a","on":true}`)
	call(t, h, "POST", "/api/select", `{"id":"light.b","on":true}`)
	call(t, h, "POST", "/api/move", `{"id":"light.b","delta":-1}`)
	got := a.store.Get().Entities
	if len(got) != 2 || got[0] != "light.b" {
		t.Fatalf("order: %v", got)
	}
	call(t, h, "POST", "/api/select", `{"id":"light.b","on":false}`)
	if got := a.store.Get().Entities; len(got) != 1 || got[0] != "light.a" {
		t.Fatalf("after remove: %v", got)
	}
}

func TestBadRequestsAndPages(t *testing.T) {
	_, h := testAPI(t)
	if code, _ := call(t, h, "POST", "/api/theme", `not json`); code != 400 {
		t.Fatalf("bad json: %d", code)
	}
	if code, out := call(t, h, "POST", "/api/act", `{"id":"light.a"}`); code != 400 || out["error"] != "not connected to Home Assistant" {
		t.Fatalf("act while offline: %d %v", code, out)
	}
	if code, out := call(t, h, "POST", "/api/open", `{"target":"https://evil.example"}`); code != 400 {
		t.Fatalf("open with a raw URL should be refused: %d %v", code, out)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest("GET", "/index.html", nil))
	if rec.Body.String() != "page" {
		t.Fatalf("pages handler not reached: %q", rec.Body.String())
	}
}
