//go:build !windows

package main

const canAutostart = false

func AutostartEnabled() bool     { return false }
func SetAutostart(on bool) error { return nil }
