package main

import (
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// App holds the app's state; the tray and the window both read it.
type App struct {
	mu        sync.Mutex
	cfgPath   string
	cfg       Config
	syncer    *Syncer
	last      Result
	lastErr   error
	lastCheck time.Time
	checking  bool
	changed   func() // called after every state change (tray refresh)
}

func NewApp() *App {
	a := &App{cfgPath: configPath()}
	a.cfg = LoadConfig(a.cfgPath)
	if a.cfg.AddOnsDir == "" {
		a.cfg.AddOnsDir = FindAddOns(wowRoots())
	}
	_ = SaveConfig(a.cfgPath, a.cfg)
	a.syncer = &Syncer{FeedURL: a.cfg.FeedURL, AddOnsDir: a.cfg.AddOnsDir,
		Client: &http.Client{Timeout: 60 * time.Second}}
	return a
}

func (a *App) notify() {
	if a.changed != nil {
		a.changed()
	}
}

// Check runs one feed check (skipped if one is already running).
func (a *App) Check() {
	a.mu.Lock()
	if a.checking {
		a.mu.Unlock()
		return
	}
	a.checking = true
	// Re-read the settings so a hand-edited addonsDir takes effect.
	if c := LoadConfig(a.cfgPath); c.AddOnsDir != "" {
		a.cfg.AddOnsDir = c.AddOnsDir
		a.syncer.AddOnsDir = c.AddOnsDir
	}
	a.mu.Unlock()
	a.notify()

	res, err := a.syncer.Check()

	a.mu.Lock()
	if err == nil || res.Meta.T != 0 {
		a.last = res
	}
	a.lastErr = err
	a.lastCheck = time.Now()
	a.checking = false
	a.mu.Unlock()
	if err != nil {
		log.Println("check:", err)
	} else if res.Updated {
		log.Printf("installed snapshot %d (%d items)", res.Meta.T, res.Meta.Items())
	}
	a.notify()
}

// SetAddOnsDir points the app at a folder the player chose. Accepts the
// AddOns folder itself or the client folder above it (e.g. ..._forever_).
func (a *App) SetAddOnsDir(dir string) error {
	dir = strings.Trim(strings.TrimSpace(dir), `"`)
	if dir == "" {
		return errors.New("Enter a folder.")
	}
	for _, cand := range []string{dir, filepath.Join(dir, "AddOns"), filepath.Join(dir, "Interface", "AddOns")} {
		if st, err := os.Stat(cand); err == nil && st.IsDir() && strings.EqualFold(filepath.Base(cand), "AddOns") {
			a.mu.Lock()
			a.cfg.AddOnsDir = cand
			a.syncer.AddOnsDir = cand
			err := SaveConfig(a.cfgPath, a.cfg)
			a.mu.Unlock()
			go a.Check()
			return err
		}
	}
	return errors.New("That isn't an AddOns folder. Pick ...\\_forever_\\Interface\\AddOns.")
}

// HouseStatus is one auction house in the feed, for the window.
type HouseStatus struct {
	ID      string `json:"id"`
	Faction string `json:"faction"`
	Items   int    `json:"items"`
}

// Status is everything the window shows.
type Status struct {
	Version    string        `json:"version"`
	State      string        `json:"state"` // ok | checking | error | nofolder | waiting
	Message    string        `json:"message"`
	SnapshotT  int64         `json:"snapshotT"`
	SnapshotAt string        `json:"snapshotAt"`
	Age        string        `json:"age"`
	LastCheck  string        `json:"lastCheck"`
	NextCheck  string        `json:"nextCheck"`
	Items      int           `json:"items"`
	Region     string        `json:"region"`
	Houses     []HouseStatus `json:"houses"`
	AddOnsDir  string        `json:"addonsDir"`
	KUIFound   bool          `json:"kuiFound"`
	Installed  bool          `json:"installed"`
	Autostart  bool          `json:"autostart"`
	CanAuto    bool          `json:"canAutostart"`
}

func (a *App) Status() Status {
	a.mu.Lock()
	defer a.mu.Unlock()
	s := Status{Version: Version, AddOnsDir: a.cfg.AddOnsDir, Autostart: AutostartEnabled(), CanAuto: canAutostart}
	if s.AddOnsDir != "" {
		_, err := os.Stat(filepath.Join(s.AddOnsDir, KUIFolder))
		s.KUIFound = err == nil
		_, err = os.Stat(filepath.Join(s.AddOnsDir, HelperAddon, "Data.lua"))
		s.Installed = err == nil
	}
	m := a.last.Meta
	if m.T != 0 {
		t := time.Unix(m.T, 0)
		s.SnapshotT = m.T
		s.SnapshotAt = t.Format("Mon 15:04")
		s.Age = Age(m.T, time.Now())
		s.Items = m.Items()
		s.Region = strings.ToUpper(m.Region)
		for id, h := range m.Houses {
			s.Houses = append(s.Houses, HouseStatus{ID: id, Faction: h.Faction, Items: h.Items})
		}
		sort.Slice(s.Houses, func(i, j int) bool { return s.Houses[i].ID < s.Houses[j].ID })
	}
	if !a.lastCheck.IsZero() {
		s.LastCheck = a.lastCheck.Format("15:04")
		s.NextCheck = a.lastCheck.Add(time.Duration(a.cfg.Minutes) * time.Minute).Format("15:04")
	}
	switch {
	case a.checking:
		s.State, s.Message = "checking", "Checking for new prices..."
	case a.cfg.AddOnsDir == "":
		s.State, s.Message = "nofolder", "WoW: Forever's AddOns folder wasn't found. Set it below."
	case a.lastErr != nil:
		s.State, s.Message = "error", a.lastErr.Error()
	case m.T == 0:
		s.State, s.Message = "waiting", "Waiting for the first check."
	default:
		s.State, s.Message = "ok", "Prices are up to date. The game loads them at login or /reload."
	}
	return s
}
