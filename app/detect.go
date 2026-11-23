package main

import (
	"os"
	"path/filepath"
	"strings"
)

// KUIFolder is K-UI's own addon folder; finding it tells us which WoW
// install (and which client folder inside it) the player uses.
const KUIFolder = "Kain-UI-Forever"

// wowRoots lists likely "World of Warcraft" folders on Windows drives.
func wowRoots() []string {
	var roots []string
	for _, d := range "CDEFGH" {
		drive := string(d) + `:\`
		for _, sub := range []string{
			`Program Files (x86)\World of Warcraft`,
			`Program Files\World of Warcraft`,
			`World of Warcraft`,
			`Games\World of Warcraft`,
			`Battle.net\World of Warcraft`,
		} {
			roots = append(roots, filepath.Join(drive, sub))
		}
	}
	return roots
}

// FindAddOns returns the Interface/AddOns folder of the client that has K-UI
// installed, preferring a client folder whose name mentions "forever".
// Returns "" when none is found.
func FindAddOns(roots []string) string {
	best := ""
	for _, root := range roots {
		entries, err := os.ReadDir(root)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() || !strings.HasPrefix(e.Name(), "_") {
				continue
			}
			addons := filepath.Join(root, e.Name(), "Interface", "AddOns")
			if _, err := os.Stat(filepath.Join(addons, KUIFolder)); err != nil {
				continue
			}
			if strings.Contains(strings.ToLower(e.Name()), "forever") {
				return addons
			}
			if best == "" {
				best = addons
			}
		}
	}
	return best
}
