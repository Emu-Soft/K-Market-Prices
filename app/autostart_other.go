//go:build !windows

package main

func AutostartEnabled() bool    { return false }
func SetAutostart(on bool) error { return nil }
