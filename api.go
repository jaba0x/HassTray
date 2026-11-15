package main

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/jaba0x/hasstray/internal/config"
	"strconv"
)

// The popover is plain HTML and talks to Go over these routes. They are
// served by the same in-process handler as the page itself, so nothing
// listens on a network port.

type request struct {
	ID     string `json:"id"`
	On     bool   `json:"on"`
	Delta  int    `json:"delta"`
	URL    string `json:"url"`
	Token  string `json:"token"`
	Theme  string `json:"theme"`
	Target string `json:"target"`

	Custom *config.CustomTheme `json:"custom"`
}

// shell is what the API needs from the window and the app around it.
type shell struct {
	hide func()
	quit func()
}

func newAPI(a *App, sh shell, pages http.Handler) http.Handler {
	mux := http.NewServeMux()

	get := func(path string, fn func(r *http.Request) (any, error)) {
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			out, err := fn(r)
			reply(w, out, err)
		})
	}
	post := func(path string, fn func(req request) (any, error)) {
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, r *http.Request) {
			var req request
			if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<16)).Decode(&req); err != nil {
				reply(w, nil, errors.New("bad request"))
				return
			}
			out, err := fn(req)
			reply(w, out, err)
		})
	}
	ok := func(err error) (any, error) { return map[string]bool{"ok": err == nil}, err }

	get("/api/snapshot", func(*http.Request) (any, error) { return a.Snapshot(), nil })
	get("/api/entities", func(*http.Request) (any, error) { return a.AllEntities(), nil })
	get("/api/history", func(r *http.Request) (any, error) {
		hours, _ := strconv.Atoi(r.URL.Query().Get("hours"))
		return a.History(r.URL.Query().Get("id"), hours)
	})

	post("/api/connect", func(q request) (any, error) { return ok(a.Connect(q.URL, q.Token)) })
	post("/api/disconnect", func(request) (any, error) { return ok(a.Disconnect()) })
	post("/api/select", func(q request) (any, error) { return ok(a.SetSelected(q.ID, q.On)) })
	post("/api/graph", func(q request) (any, error) { return ok(a.SetGraph(q.ID, q.On)) })
	post("/api/widget", func(q request) (any, error) { return ok(a.SetWidget(q.ID, q.On)) })
	post("/api/move", func(q request) (any, error) { return ok(a.Move(q.ID, q.Delta)) })
	post("/api/theme", func(q request) (any, error) { return ok(a.SetTheme(q.Theme)) })
	post("/api/custom", func(q request) (any, error) {
		if q.Custom == nil {
			return nil, errors.New("no theme in the request")
		}
		return ok(a.SetCustom(*q.Custom))
	})
	post("/api/act", func(q request) (any, error) { return ok(a.Act(q.ID)) })
	post("/api/open", func(q request) (any, error) { return ok(openTarget(a, q.Target)) })
	post("/api/hide", func(request) (any, error) { sh.hide(); return ok(nil) })
	post("/api/quit", func(request) (any, error) { go sh.quit(); return ok(nil) })

	mux.Handle("/", pages)
	if os.Getenv("HASSTRAY_DEBUG") == "" {
		return mux
	}
	// HASSTRAY_DEBUG=1 prints every request the popover makes
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("%s %s", r.Method, r.URL.Path)
		mux.ServeHTTP(w, r)
	})
}

func reply(w http.ResponseWriter, out any, err error) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		out = map[string]string{"error": err.Error()}
	}
	_ = json.NewEncoder(w).Encode(out)
}

// openTarget opens one of a fixed set of pages in the default browser. The
// page never passes a URL of its own.
func openTarget(a *App, target string) error {
	var url string
	switch target {
	case "ha":
		url = a.store.Get().URL
	case "clihero":
		url = "https://clihero.com"
	case "github":
		url = "https://github.com/jaba0x/HassTray"
	case "release":
		url = lastUpdate().URL
		if url == "" {
			url = releasesPage
		}
	case "token":
		if base := a.store.Get().URL; base != "" {
			url = base + "/profile/security"
		}
	}
	if url == "" {
		return errors.New("nothing to open")
	}
	return exec.Command("open", url).Start()
}
