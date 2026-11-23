package main

import (
	"bufio"
	"net"
	"time"
)

// The running copy listens on a fixed local port. A second launch connects,
// asks it to show its window and exits, so the player never gets two coins.
const instanceAddr = "127.0.0.1:47613"

var onShowRequest func()

func claimSingleInstance() bool {
	ln, err := net.Listen("tcp", instanceAddr)
	if err != nil {
		if c, err := net.DialTimeout("tcp", instanceAddr, time.Second); err == nil {
			_, _ = c.Write([]byte("show\n"))
			_ = c.Close()
			return false
		}
		return true // port taken by something else: run anyway
	}
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			line, _ := bufio.NewReader(c).ReadString('\n')
			_ = c.Close()
			if line == "show\n" && onShowRequest != nil {
				onShowRequest()
			}
		}
	}()
	return true
}
