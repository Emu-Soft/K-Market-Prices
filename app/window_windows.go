//go:build windows

package main

import (
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	webview2 "github.com/jchv/go-webview2"
	"golang.org/x/sys/windows"
)

//go:embed ui.html
var uiHTML string

var (
	winMu sync.Mutex
	win   webview2.WebView

	user32         = windows.NewLazySystemDLL("user32.dll")
	setForeground  = user32.NewProc("SetForegroundWindow")
	showWindowProc = user32.NewProc("ShowWindow")
	postMessageW   = user32.NewProc("PostMessageW")
	swRestore      = uintptr(9)
	wmClose        = uintptr(0x0010)
)

// showMainWindow opens the window, or brings it to the front when it is
// already open. Each window runs its own message loop on a locked thread.
func showMainWindow(app *App) {
	winMu.Lock()
	if win != nil {
		h := uintptr(win.Window())
		winMu.Unlock()
		_, _, _ = showWindowProc.Call(h, swRestore)
		_, _, _ = setForeground.Call(h)
		return
	}
	winMu.Unlock()
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			AutoFocus: true,
			DataPath:  filepath.Join(filepath.Dir(configPath()), "webview"),
			WindowOptions: webview2.WindowOptions{
				Title: "K-Market Prices", Width: 460, Height: 760, IconId: 1, Center: true,
			},
		})
		if w == nil {
			// No WebView2 runtime: point the player at it once.
			close(ready)
			openPath("https://developer.microsoft.com/microsoft-edge/webview2/")
			return
		}
		w.SetSize(460, 760, webview2.HintFixed)
		bind(w, app)
		w.SetHtml(strings.Replace(uiHTML, "{{ICON}}", base64.StdEncoding.EncodeToString(iconPNG), 1))
		winMu.Lock()
		win = w
		winMu.Unlock()
		close(ready)
		w.Run()
		winMu.Lock()
		win = nil
		winMu.Unlock()
		w.Destroy()
	}()
	<-ready
}

func closeMainWindow() {
	winMu.Lock()
	w := win
	winMu.Unlock()
	if w != nil {
		_, _, _ = postMessageW.Call(uintptr(w.Window()), wmClose, 0, 0)
	}
}

// windowRefresh asks the page to pull the status again.
func windowRefresh() {
	winMu.Lock()
	w := win
	winMu.Unlock()
	if w != nil {
		w.Dispatch(func() { w.Eval("window.kmRefresh && window.kmRefresh()") })
	}
}

func bind(w webview2.WebView, app *App) {
	_ = w.Bind("kmStatus", func() string {
		b, _ := json.Marshal(app.Status())
		return string(b)
	})
	_ = w.Bind("kmCheck", func() { go app.Check() })
	_ = w.Bind("kmOpenFolder", func() {
		if d := app.Status().AddOnsDir; d != "" {
			openPath(d)
		}
	})
	_ = w.Bind("kmSetFolder", func(dir string) string {
		if err := app.SetAddOnsDir(dir); err != nil {
			return err.Error()
		}
		return ""
	})
	_ = w.Bind("kmAutostart", func(on bool) string {
		if err := SetAutostart(on); err != nil {
			return err.Error()
		}
		return ""
	})
	_ = w.Bind("kmOpenLink", func(url string) {
		if len(url) > 8 && url[:8] == "https://" {
			openPath(url)
		}
	})
	_ = w.Bind("kmHide", func() { closeMainWindow() })
}
