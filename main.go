package main

import (
	"embed"
	"io/fs"
	"log"
	"net/http"
	"os"
	"sync"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"

	"github.com/jaba0x/hasstray/internal/config"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed assets/tray.png
var trayIcon []byte

// Version is set at build time with -ldflags "-X main.Version=..."
var Version = "0.2.0"

func main() {
	dir, err := config.Dir()
	if err != nil {
		log.Fatal(err)
	}
	store, err := config.Open(dir)
	if err != nil {
		log.Fatal("couldn't read ", dir, "/config.json: ", err)
	}
	core := NewApp(store)

	dist, err := fs.Sub(assets, "frontend/dist")
	if err != nil {
		log.Fatal(err)
	}

	var window *application.WebviewWindow
	var app *application.App
	api := newAPI(core, shell{
		hide: func() { window.Hide() },
		quit: func() { app.Quit() },
	}, application.BundledAssetFileServer(dist))

	app = application.New(application.Options{
		Name:        "HassTray",
		Description: "Home Assistant in your Mac menu bar. Built by Jaba Macharashvili, clihero.com",
		Assets:      application.AssetOptions{Handler: api},
		Mac: application.MacOptions{
			// menu bar only: no Dock icon, and closing the popover doesn't quit
			ActivationPolicy: application.ActivationPolicyAccessory,
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})

	window = app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:            "popover",
		Title:           "HassTray",
		Width:           400,
		Height:          640,
		URL:             "/",
		Frameless:       true,
		AlwaysOnTop:     true,
		Hidden:          true,
		DisableResize:   true,
		HideOnEscape:    true,
		HideOnFocusLost: true,
		Mac: application.MacWindow{
			Backdrop: application.MacBackdropLiquidGlass,
			LiquidGlass: application.MacLiquidGlass{
				Style:        application.LiquidGlassStyleAutomatic,
				Material:     application.NSVisualEffectMaterialAuto,
				CornerRadius: 22,
			},
		},
	})
	window.RegisterHook(events.Common.WindowClosing, func(e *application.WindowEvent) {
		window.Hide()
		e.Cancel()
	})

	tray := app.SystemTray.New()
	tray.SetTemplateIcon(trayIcon)
	tray.SetTooltip("HassTray")
	tray.AttachWindow(window).WindowOffset(6)

	menu := app.NewMenu()
	menu.Add("Open Home Assistant").OnClick(func(*application.Context) { _ = openTarget(core, "ha") })
	menu.AddSeparator()
	menu.Add("HassTray " + Version).SetEnabled(false)
	menu.Add("Built by clihero.com").OnClick(func(*application.Context) { _ = openTarget(core, "clihero") })
	menu.AddSeparator()
	menu.Add("Quit HassTray").OnClick(func(*application.Context) { app.Quit() })
	tray.SetMenu(menu)

	// the widgets: up to five entity values next to the icon
	label := ""
	var labelMu sync.Mutex
	core.onChange = func() {
		labelMu.Lock()
		defer labelMu.Unlock()
		if next := core.TrayLabel(); next != label {
			label = next
			tray.SetLabel(next)
		}
	}

	// HASSTRAY_DEBUG_ADDR=127.0.0.1:8177 also serves the popover over plain
	// HTTP, so it can be opened in a browser with the developer tools. Off
	// unless asked for, and only meant for working on the interface.
	if addr := os.Getenv("HASSTRAY_DEBUG_ADDR"); addr != "" {
		go func() { log.Println(http.ListenAndServe(addr, api)) }()
	}

	core.Start()
	go watchUpdates(core.changed)

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
