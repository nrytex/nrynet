package main

import (
	"embed"
	"log"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

type appWindow struct {
	app *application.App
	win *application.WebviewWindow
}

func (w *appWindow) Show() { w.win.Show().Focus() }
func (w *appWindow) Hide() { w.win.Hide() }
func (w *appWindow) Quit() { w.app.Quit() }

func main() {
	logs := newMemoryLogHandler()
	store, err := newFileStore()
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(application.Options{
		Name:        "NAT-Link",
		Description: "NAT-Link desktop client",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
	})
	updaterSvc := NewUpdateService(app.Updater)
	desktopSvc, err := NewDesktopService(store, logs, updaterSvc)
	if err != nil {
		log.Fatal(err)
	}
	app.RegisterService(application.NewService(desktopSvc))
	win := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title: "NAT-Link", Width: 1120, Height: 760, MinWidth: 920,
		MinHeight: 620, URL: "/", BackgroundColour: application.NewRGB(246, 248, 251),
	})
	desktopSvc.setWindow(&appWindow{app: app, win: win})
	configureTray(app, desktopSvc, win)
	configureMenu(app, desktopSvc)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func configureTray(app *application.App, svc *DesktopService, win *application.WebviewWindow) {
	tray := app.SystemTray.New()
	tray.SetTooltip("NAT-Link")
	tray.AttachWindow(win)
	menu := application.NewMenu()
	menu.Add("Show").OnClick(func(*application.Context) { svc.ShowWindow() })
	menu.Add("Connect").OnClick(func(*application.Context) { _, _ = svc.Connect() })
	menu.Add("Disconnect").OnClick(func(*application.Context) { svc.Disconnect() })
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) { svc.Quit() })
	tray.SetMenu(menu)
	tray.OnClick(func() { svc.ShowWindow() })
}

func configureMenu(app *application.App, svc *DesktopService) {
	menu := app.Menu.New()
	app.Menu.SetApplicationMenu(menu)
	appMenu := menu.AddSubmenu("NAT-Link")
	appMenu.Add("Show").OnClick(func(*application.Context) { svc.ShowWindow() })
	appMenu.Add("Check for Updates").OnClick(func(*application.Context) {
		_, _ = svc.CheckForUpdate()
	})
	appMenu.AddSeparator()
	appMenu.Add("Quit").OnClick(func(*application.Context) { svc.Quit() })
}
