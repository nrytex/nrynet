package main

import (
	"embed"
	"log"
	"sync/atomic"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var appIcon []byte

type appWindow struct {
	app      *application.App
	win      *application.WebviewWindow
	quitting *atomic.Bool
}

const (
	defaultWindowWidth  = 680
	defaultWindowHeight = 720
	minWindowWidth      = 560
	minWindowHeight     = 560
)

func (w *appWindow) Show() { w.win.Show().Focus() }
func (w *appWindow) Hide() { w.win.Hide() }
func (w *appWindow) Quit() {
	w.quitting.Store(true)
	w.app.Quit()
}

func mainWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Title:            "Nrynet",
		Width:            defaultWindowWidth,
		Height:           defaultWindowHeight,
		MinWidth:         minWindowWidth,
		MinHeight:        minWindowHeight,
		URL:              "/",
		InitialPosition:  application.WindowCentered,
		BackgroundColour: application.NewRGB(238, 245, 242),
	}
}

func main() {
	logs := newMemoryLogHandler()
	store, err := newFileStore()
	if err != nil {
		log.Fatal(err)
	}
	app := application.New(application.Options{
		Name:        "Nrynet",
		Description: "Nrynet desktop client",
		Icon:        appIcon,
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(assets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Linux:   application.LinuxOptions{DisableQuitOnLastWindowClosed: true},
	})
	updaterSvc := NewUpdateService(app.Updater)
	desktopSvc, err := NewDesktopService(store, logs, updaterSvc)
	if err != nil {
		log.Fatal(err)
	}
	app.RegisterService(application.NewService(desktopSvc))
	win := app.Window.NewWithOptions(mainWindowOptions())
	var quitting atomic.Bool
	desktopSvc.setWindow(&appWindow{app: app, win: win, quitting: &quitting})
	configureCloseToTray(win, &quitting)
	configureTray(app, desktopSvc, win)
	configureMenu(app, desktopSvc)
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}

func configureCloseToTray(win *application.WebviewWindow, quitting *atomic.Bool) {
	win.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if quitting.Load() {
			return
		}
		win.Hide()
		event.Cancel()
	})
}

func configureTray(app *application.App, svc *DesktopService, win *application.WebviewWindow) {
	tray := app.SystemTray.New()
	tray.SetIcon(appIcon)
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
