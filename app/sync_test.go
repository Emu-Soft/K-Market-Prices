package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func feed(t *testing.T, data string, badHash bool) *httptest.Server {
	sum := sha256.Sum256([]byte(data))
	h := hex.EncodeToString(sum[:])
	if badHash {
		h = strings.Repeat("0", 64)
	}
	meta := fmt.Sprintf(`{"v":1,"region":"us","t":1795432800,"sha256":%q,"houses":{"110":{"faction":"Alliance","items":2}}}`, h)
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/meta.json":
			w.Write([]byte(meta))
		case "/Data.lua":
			w.Write([]byte(data))
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestCheckInstallsAndSkipsUnchanged(t *testing.T) {
	srv := feed(t, "KUI_PRICE_DATA = {}\n", false)
	defer srv.Close()
	dir := t.TempDir()
	s := &Syncer{FeedURL: srv.URL + "/", AddOnsDir: dir, Client: srv.Client()}

	res, err := s.Check()
	if err != nil || !res.Updated || res.Meta.Items() != 2 {
		t.Fatalf("first check: %+v %v", res, err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, HelperAddon, "Data.lua"))
	if string(got) != "KUI_PRICE_DATA = {}\n" {
		t.Fatalf("Data.lua = %q", got)
	}
	toc, _ := os.ReadFile(filepath.Join(dir, HelperAddon, HelperAddon+".toc"))
	if !strings.Contains(string(toc), "## Interface: 16001") {
		t.Fatalf("toc = %q", toc)
	}
	res, err = s.Check()
	if err != nil || res.Updated {
		t.Fatalf("second check should skip: %+v %v", res, err)
	}
}

func TestCheckRejectsHashMismatch(t *testing.T) {
	srv := feed(t, "KUI_PRICE_DATA = {}\n", true)
	defer srv.Close()
	dir := t.TempDir()
	s := &Syncer{FeedURL: srv.URL + "/", AddOnsDir: dir, Client: srv.Client()}
	if _, err := s.Check(); err == nil {
		t.Fatal("expected a hash error")
	}
	if _, err := os.Stat(filepath.Join(dir, HelperAddon, "Data.lua")); err == nil {
		t.Fatal("Data.lua must not be written on a bad hash")
	}
}

func TestCheckNeedsFolder(t *testing.T) {
	s := &Syncer{FeedURL: "http://unused/"}
	if _, err := s.Check(); err != ErrNoAddOns {
		t.Fatalf("err = %v", err)
	}
}

func TestFindAddOnsPrefersForever(t *testing.T) {
	root := t.TempDir()
	for _, client := range []string{"_retail_", "_forever_", "_classic_"} {
		os.MkdirAll(filepath.Join(root, client, "Interface", "AddOns", KUIFolder), 0o755)
	}
	os.MkdirAll(filepath.Join(root, "_beta_", "Interface", "AddOns"), 0o755)
	want := filepath.Join(root, "_forever_", "Interface", "AddOns")
	if got := FindAddOns([]string{"/nope", root}); got != want {
		t.Fatalf("got %q want %q", got, want)
	}
	if got := FindAddOns([]string{t.TempDir()}); got != "" {
		t.Fatalf("empty root gave %q", got)
	}
}

func TestAge(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	for _, c := range []struct {
		ago  time.Duration
		want string
	}{{30 * time.Second, "just now"}, {14 * time.Minute, "14 min ago"}, {3 * time.Hour, "3 h ago"}, {72 * time.Hour, "3 days ago"}} {
		if got := Age(now.Add(-c.ago).Unix(), now); got != c.want {
			t.Errorf("%v: %q want %q", c.ago, got, c.want)
		}
	}
}

func TestConfigDefaults(t *testing.T) {
	p := filepath.Join(t.TempDir(), "c.json")
	c := LoadConfig(p)
	if c.FeedURL != defaultFeed || c.Minutes != 10 {
		t.Fatalf("%+v", c)
	}
	c.AddOnsDir = `D:\WoW\_forever_\Interface\AddOns`
	if err := SaveConfig(p, c); err != nil {
		t.Fatal(err)
	}
	if LoadConfig(p).AddOnsDir != c.AddOnsDir {
		t.Fatal("addonsDir not saved")
	}
}

func TestCheckWritesIntoKUIAndRemovesHelper(t *testing.T) {
	srv := feed(t, "KUI_PRICE_DATA = {}\n", false)
	defer srv.Close()
	dir := t.TempDir()
	kui := filepath.Join(dir, KUIFolder)
	os.MkdirAll(kui, 0o755)
	os.WriteFile(filepath.Join(kui, KUIFolder+".toc"), []byte("## Interface: 16001\nKMarketAuction.lua\nKMarketPriceData.lua\nKMarketPrices.lua\n"), 0o644)
	os.WriteFile(filepath.Join(kui, PriceFile), []byte("\n"), 0o644)
	helper := filepath.Join(dir, HelperAddon)
	os.MkdirAll(helper, 0o755)
	os.WriteFile(filepath.Join(helper, HelperAddon+".toc"), []byte(helperToc), 0o644)
	os.WriteFile(filepath.Join(helper, "Data.lua"), []byte("old"), 0o644)
	s := &Syncer{FeedURL: srv.URL + "/", AddOnsDir: dir, Client: srv.Client()}

	res, err := s.Check()
	if err != nil || !res.Updated {
		t.Fatalf("check: %+v %v", res, err)
	}
	got, _ := os.ReadFile(filepath.Join(kui, PriceFile))
	if string(got) != "KUI_PRICE_DATA = {}\n" {
		t.Fatalf("%s = %q", PriceFile, got)
	}
	if _, err := os.Stat(helper); !os.IsNotExist(err) {
		t.Fatalf("old helper addon should be gone: %v", err)
	}
	// A K-UI update puts the empty file back: the next check rewrites it.
	os.WriteFile(filepath.Join(kui, PriceFile), []byte("\n"), 0o644)
	if res, err = s.Check(); err != nil || !res.Updated {
		t.Fatalf("rewrite after K-UI update: %+v %v", res, err)
	}
}

func TestHelperWithOtherFilesIsKept(t *testing.T) {
	srv := feed(t, "KUI_PRICE_DATA = {}\n", false)
	defer srv.Close()
	dir := t.TempDir()
	kui := filepath.Join(dir, KUIFolder)
	os.MkdirAll(kui, 0o755)
	os.WriteFile(filepath.Join(kui, KUIFolder+".toc"), []byte("KMarketPriceData.lua\n"), 0o644)
	helper := filepath.Join(dir, HelperAddon)
	os.MkdirAll(helper, 0o755)
	os.WriteFile(filepath.Join(helper, "Notes.txt"), []byte("mine"), 0o644)
	s := &Syncer{FeedURL: srv.URL + "/", AddOnsDir: dir, Client: srv.Client()}
	if _, err := s.Check(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(helper, "Notes.txt")); err != nil {
		t.Fatal("a helper folder holding other files must be left alone")
	}
}
