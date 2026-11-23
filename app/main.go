// K-Market Prices: keeps K-Market's auction prices current.
//
// It checks the hourly price feed every few minutes and, when a new
// snapshot is out, writes it into the KainUIForever_Prices helper addon.
// The game reads it at the next login or /reload. A small window shows
// what it's doing; closing the window leaves it running in the tray.
package main

import (
	_ "embed"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"fyne.io/systray"
)

const Version = "0.2.0"

//go:embed icon.ico
var iconICO []byte

//go:embed icon.png
var iconPNG []byte

func main() {
	trayOnly := len(os.Args) > 1 && os.Args[1] == "--tray"
	// A second launch asks the running copy to show its window, then exits.
	if !claimSingleInstance() {
		return
	}
	if f, err := os.OpenFile(filepath.Join(filepath.Dir(configPath()), "log.txt"),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(f)
	}
	app := NewApp()
	systray.Run(func() { onReady(app, !trayOnly) }, func() {})
}

func openPath(p string) {
	switch runtime.GOOS {
	case "windows":
		_ = exec.Command("explorer", p).Start()
	case "darwin":
		_ = exec.Command("open", p).Start()
	default:
		_ = exec.Command("xdg-open", p).Start()
	}
}

func onReady(app *App, showWindow bool) {
	if runtime.GOOS == "windows" {
		systray.SetIcon(iconICO)
	} else {
		systray.SetIcon(iconPNG)
	}
	systray.SetTooltip("K-Market Prices")

	open := systray.AddMenuItem("Open K-Market Prices", "")
	systray.AddSeparator()
	status := systray.AddMenuItem("Starting...", "")
	status.Disable()
	update := systray.AddMenuItem("Update now", "Check for new prices")
	folder := systray.AddMenuItem("Open AddOns folder", "")
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "")

	app.changed = func() {
		s := app.Status()
		title := s.Message
		if s.State == "ok" {
			title = fmt.Sprintf("Prices from %s (%d items)", s.Age, s.Items)
		}
		status.SetTitle(title)
		systray.SetTooltip("K-Market Prices\n" + title)
		windowRefresh()
	}
	systray.SetOnTapped(func() { showMainWindow(app) })
	onShowRequest = func() { showMainWindow(app) }
	if showWindow {
		showMainWindow(app)
	}

	go func() {
		app.Check()
		tick := time.NewTicker(time.Duration(app.cfg.Minutes) * time.Minute)
		age := time.NewTicker(time.Minute)
		for {
			select {
			case <-tick.C:
				app.Check()
			case <-age.C:
				app.notify()
			case <-update.ClickedCh:
				go app.Check()
			case <-open.ClickedCh:
				showMainWindow(app)
			case <-folder.ClickedCh:
				if d := app.Status().AddOnsDir; d != "" {
					openPath(d)
				}
			case <-quit.ClickedCh:
				closeMainWindow()
				systray.Quit()
				return
			}
		}
	}()
}
