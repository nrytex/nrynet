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
		Name:        "Nrynet",
		Description: "Nrynet desktop client",
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
		Title: "Nrynet", Width: 680, Height: 860, MinWidth: 600,
		MinHeight: 620, URL: "/", BackgroundColour: application.NewRGB(238, 245, 242),
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
	tray.SetTooltip("Nrynet")
	tray.AttachWindow(win)
	menu := application.NewMenu()
	menu.Add("显示主窗口").OnClick(func(*application.Context) { svc.ShowWindow() })
	menu.Add("连接").OnClick(func(*application.Context) { _, _ = svc.Connect() })
	menu.Add("断开连接").OnClick(func(*application.Context) { svc.Disconnect() })
	menu.AddSeparator()
	menu.Add("退出").OnClick(func(*application.Context) { svc.Quit() })
	tray.SetMenu(menu)
	tray.OnClick(func() { svc.ShowWindow() })
}

func configureMenu(app *application.App, svc *DesktopService) {
	menu := app.Menu.New()
	app.Menu.SetApplicationMenu(menu)
	appMenu := menu.AddSubmenu("Nrynet")
	appMenu.Add("显示主窗口").OnClick(func(*application.Context) { svc.ShowWindow() })
	appMenu.Add("检查更新").OnClick(func(*application.Context) {
		_, _ = svc.CheckForUpdate()
	})
	appMenu.AddSeparator()
	appMenu.Add("退出").OnClick(func(*application.Context) { svc.Quit() })
}
