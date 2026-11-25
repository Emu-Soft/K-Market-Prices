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
	user32         = windows.NewLazySystemDLL("user32.dll")
	setForeground  = user32.NewProc("SetForegroundWindow")
	showWindowProc = user32.NewProc("ShowWindow")
	postMessageW   = user32.NewProc("PostMessageW")
	swRestore      = uintptr(9)
	wmClose        = uintptr(0x0010)
)

// page is one app window. Each open window runs its own message loop on a
// locked thread.
type page struct {
	mu            sync.Mutex
	w             webview2.WebView
	title         string
	width, height int
	hint          webview2.Hint
	html          func() string
	bind          func(webview2.WebView)
}

var (
	mainPage     = &page{title: "K-Market Prices", width: 460, height: 760, hint: webview2.HintFixed}
	enchantsPage = &page{title: "K-Market Prices: Enchants", width: 1180, height: 860, hint: webview2.HintNone}
)

// show opens the window, or brings it to the front when it is already open.
func (p *page) show() {
	p.mu.Lock()
	if p.w != nil {
		h := uintptr(p.w.Window())
		p.mu.Unlock()
		_, _, _ = showWindowProc.Call(h, swRestore)
		_, _, _ = setForeground.Call(h)
		return
	}
	p.mu.Unlock()
	ready := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		w := webview2.NewWithOptions(webview2.WebViewOptions{
			AutoFocus: true,
			DataPath:  filepath.Join(filepath.Dir(configPath()), "webview"),
			WindowOptions: webview2.WindowOptions{
				Title: p.title, Width: uint(p.width), Height: uint(p.height), IconId: 1, Center: true,
			},
		})
		if w == nil {
			// No WebView2 runtime: point the player at it once.
			close(ready)
			openPath("https://developer.microsoft.com/microsoft-edge/webview2/")
			return
		}
		w.SetSize(p.width, p.height, p.hint)
		p.bind(w)
		w.SetHtml(p.html())
		p.mu.Lock()
		p.w = w
		p.mu.Unlock()
		close(ready)
		w.Run()
		p.mu.Lock()
		p.w = nil
		p.mu.Unlock()
		w.Destroy()
	}()
	<-ready
}

func (p *page) close() {
	p.mu.Lock()
	w := p.w
	p.mu.Unlock()
	if w != nil {
		_, _, _ = postMessageW.Call(uintptr(w.Window()), wmClose, 0, 0)
	}
}

// refresh asks the page to pull its data again.
func (p *page) refresh() {
	p.mu.Lock()
	w := p.w
	p.mu.Unlock()
	if w != nil {
		w.Dispatch(func() { w.Eval("window.kmRefresh && window.kmRefresh()") })
	}
}

func showMainWindow(app *App) {
	mainPage.html = func() string {
		return strings.Replace(uiHTML, "{{ICON}}", base64.StdEncoding.EncodeToString(iconPNG), 1)
	}
	mainPage.bind = func(w webview2.WebView) { bindMain(w, app) }
	mainPage.show()
}

func showEnchantsWindow(app *App) {
	enchantsPage.html = func() string { return strings.Replace(enchantsHTML, "{{DATA}}", enchantsData, 1) }
	enchantsPage.bind = func(w webview2.WebView) {
		_ = w.Bind("kmEnchantPrices", func() string {
			b, _ := json.Marshal(app.EnchantPrices())
			return string(b)
		})
	}
	enchantsPage.show()
}

func closeMainWindow() { mainPage.close() }

func closeAllWindows() {
	mainPage.close()
	enchantsPage.close()
}

// windowRefresh tells every open window the state changed.
func windowRefresh() {
	mainPage.refresh()
	enchantsPage.refresh()
}

func bindMain(w webview2.WebView, app *App) {
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
	_ = w.Bind("kmEnchants", func() { go showEnchantsWindow(app) })
	_ = w.Bind("kmHide", func() { closeMainWindow() })
}
