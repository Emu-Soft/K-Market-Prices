//go:build !windows

package main

// The windows use WebView2, which is Windows only; elsewhere the app is
// tray only.
func showMainWindow(app *App)     {}
func showEnchantsWindow(app *App) {}
func closeMainWindow()            {}
func closeAllWindows()            {}
func windowRefresh()              {}
