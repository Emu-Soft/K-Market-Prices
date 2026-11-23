//go:build !windows

package main

// The window uses WebView2, which is Windows only; elsewhere the app is
// tray only.
func showMainWindow(app *App) {}
func closeMainWindow()        {}
func windowRefresh()          {}
