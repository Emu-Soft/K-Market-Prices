// K-Market Prices: a tray app that keeps K-Market's auction prices current.
//
// It checks the hourly price feed every few minutes and, when a new
// snapshot is out, writes it into the KainUIForever_Prices helper addon.
// The game reads it at the next login or /reload.
package main

import (
	_ "embed"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"fyne.io/systray"
)

const Version = "0.1.0"

//go:embed icon.ico
var iconICO []byte

//go:embed icon.png
var iconPNG []byte

func main() {
	if f, err := os.OpenFile(filepath.Join(filepath.Dir(configPath()), "log.txt"),
		os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); err == nil {
		log.SetOutput(f)
	}
	systray.Run(onReady, func() {})
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

func onReady() {
	if runtime.GOOS == "windows" {
		systray.SetIcon(iconICO)
	} else {
		systray.SetIcon(iconPNG)
	}
	systray.SetTitle("")
	systray.SetTooltip("K-Market Prices")

	status := systray.AddMenuItem("Starting...", "")
	status.Disable()
	where := systray.AddMenuItem("", "")
	where.Disable()
	systray.AddSeparator()
	update := systray.AddMenuItem("Update now", "Check for new prices")
	folder := systray.AddMenuItem("Open AddOns folder", "")
	settings := systray.AddMenuItem("Open settings file", "Set addonsDir here if the folder is not found")
	autostart := systray.AddMenuItemCheckbox("Start with Windows", "", AutostartEnabled())
	if runtime.GOOS != "windows" {
		autostart.Hide()
	}
	systray.AddSeparator()
	quit := systray.AddMenuItem("Quit", "")

	cfgPath := configPath()
	cfg := LoadConfig(cfgPath)
	if cfg.AddOnsDir == "" {
		cfg.AddOnsDir = FindAddOns(wowRoots())
		if cfg.AddOnsDir != "" {
			_ = SaveConfig(cfgPath, cfg)
		}
	}
	if _, err := os.Stat(cfgPath); err != nil {
		_ = SaveConfig(cfgPath, cfg)
	}
	syncer := &Syncer{FeedURL: cfg.FeedURL, AddOnsDir: cfg.AddOnsDir,
		Client: &http.Client{Timeout: 60 * time.Second}}

	var mu sync.Mutex
	var last Result
	var lastErr error
	refresh := func() {
		mu.Lock()
		defer mu.Unlock()
		if syncer.AddOnsDir == "" {
			where.SetTitle("AddOns folder not found: see settings file")
		} else {
			where.SetTitle("Folder: " + syncer.AddOnsDir)
		}
		switch {
		case lastErr != nil && last.Meta.T == 0:
			status.SetTitle("Error: " + lastErr.Error())
		case last.Meta.T == 0:
			status.SetTitle("Checking...")
		default:
			s := fmt.Sprintf("Prices from %s (%d items)", Age(last.Meta.T, time.Now()), last.Meta.Items())
			if lastErr != nil {
				s += " - last check failed"
			}
			status.SetTitle(s)
		}
		systray.SetTooltip("K-Market Prices\n" + status.String())
	}
	check := func() {
		// Re-read the settings so a hand-set addonsDir takes effect.
		c := LoadConfig(cfgPath)
		if c.AddOnsDir != "" {
			syncer.AddOnsDir = c.AddOnsDir
		}
		res, err := syncer.Check()
		mu.Lock()
		if err == nil || res.Meta.T != 0 {
			last = res
		}
		lastErr = err
		mu.Unlock()
		if err != nil {
			log.Println("check:", err)
		} else if res.Updated {
			log.Printf("installed snapshot %d (%d items)", res.Meta.T, res.Meta.Items())
		}
		refresh()
	}

	go func() {
		check()
		tick := time.NewTicker(time.Duration(cfg.Minutes) * time.Minute)
		age := time.NewTicker(time.Minute)
		for {
			select {
			case <-tick.C:
				check()
			case <-age.C:
				refresh()
			case <-update.ClickedCh:
				status.SetTitle("Checking...")
				check()
			case <-folder.ClickedCh:
				if syncer.AddOnsDir != "" {
					openPath(syncer.AddOnsDir)
				}
			case <-settings.ClickedCh:
				openPath(cfgPath)
			case <-autostart.ClickedCh:
				on := !autostart.Checked()
				if err := SetAutostart(on); err != nil {
					log.Println("autostart:", err)
					continue
				}
				if on {
					autostart.Check()
				} else {
					autostart.Uncheck()
				}
			case <-quit.ClickedCh:
				systray.Quit()
				return
			}
		}
	}()
}
