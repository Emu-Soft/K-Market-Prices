package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// HelperAddon is the folder the app owns inside Interface/AddOns. K-UI lists
// it as an optional dependency and reads KUI_PRICE_DATA from it at login.
const HelperAddon = "KainUIForever_Prices"

const helperToc = `## Interface: 16001
## Title: K-UI Forever Prices
## Notes: Auction house prices for K-Market. Written by the K-Market Prices app; updates after /reload.
## Author: Emu-Soft
Data.lua
`

// Meta mirrors meta.json on the data branch.
type Meta struct {
	V      int    `json:"v"`
	Region string `json:"region"`
	T      int64  `json:"t"`
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
	Houses map[string]struct {
		Faction string `json:"faction"`
		Items   int    `json:"items"`
	} `json:"houses"`
}

// Items is the item count summed over all houses.
func (m Meta) Items() int {
	n := 0
	for _, h := range m.Houses {
		n += h.Items
	}
	return n
}

// Syncer downloads the feed and keeps the helper addon's Data.lua current.
type Syncer struct {
	FeedURL   string // folder URL holding meta.json and Data.lua, ending in "/"
	AddOnsDir string
	Client    *http.Client
}

// Result says what one Check did.
type Result struct {
	Meta    Meta
	Updated bool
}

var ErrNoAddOns = errors.New("WoW: Forever AddOns folder not found")

func (s *Syncer) get(name string, limit int64) ([]byte, error) {
	req, err := http.NewRequest("GET", s.FeedURL+name, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cache-Control", "no-cache")
	req.Header.Set("User-Agent", "K-Market-Prices/"+Version)
	resp, err := s.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", name, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, limit))
}

func fileSHA(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// writeAtomic writes to a temp file next to path and renames it over path, so
// the game never loads half a file.
func writeAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Check fetches meta.json and, when the snapshot differs from what is on
// disk, downloads Data.lua, verifies its hash and installs it.
func (s *Syncer) Check() (Result, error) {
	var res Result
	if s.AddOnsDir == "" {
		return res, ErrNoAddOns
	}
	raw, err := s.get("meta.json", 1<<20)
	if err != nil {
		return res, err
	}
	if err := json.Unmarshal(raw, &res.Meta); err != nil {
		return res, fmt.Errorf("meta.json: %w", err)
	}
	if res.Meta.V != 1 {
		return res, fmt.Errorf("feed format %d is newer than this app; please update it", res.Meta.V)
	}
	dir := filepath.Join(s.AddOnsDir, HelperAddon)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return res, err
	}
	toc := filepath.Join(dir, HelperAddon+".toc")
	if old, _ := os.ReadFile(toc); string(old) != helperToc {
		if err := writeAtomic(toc, []byte(helperToc)); err != nil {
			return res, err
		}
	}
	data := filepath.Join(dir, "Data.lua")
	if fileSHA(data) == res.Meta.SHA256 {
		return res, nil
	}
	body, err := s.get("Data.lua", 64<<20)
	if err != nil {
		return res, err
	}
	sum := sha256.Sum256(body)
	if hex.EncodeToString(sum[:]) != res.Meta.SHA256 {
		// The data branch moved between the two downloads; next check gets it.
		return res, errors.New("Data.lua did not match meta.json; will retry")
	}
	if err := writeAtomic(data, body); err != nil {
		return res, err
	}
	res.Updated = true
	return res, nil
}

// Age formats how old the snapshot is, for the tray menu.
func Age(t int64, now time.Time) string {
	d := now.Sub(time.Unix(t, 0))
	switch {
	case d < 2*time.Minute:
		return "just now"
	case d < time.Hour:
		return fmt.Sprintf("%d min ago", int(d.Minutes()))
	case d < 48*time.Hour:
		return fmt.Sprintf("%d h ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%d days ago", int(d.Hours()/24))
	}
}
