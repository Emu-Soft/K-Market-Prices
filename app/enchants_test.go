package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const sampleData = `-- K-Market prices. Written by the K-Market price job; do not edit.
KUI_PRICE_DATA = {
	v = 1, region = "us", ruleset = 1, t = 1795605444,
	houses = {
		[113] = { faction = "Horde", t = 1795605444,
			d = "2318,96,98,102,8548;252463,5791,5791,6060,196" },
		[110] = { faction = "Alliance", t = 1795605444,
			d = "2318,90,94,85,9834;252462,5948,5948,5188,469;252463,7555,7555,5413,130;bad,row" },
	},
}
`

func TestParsePricesKeepsOnlyPlannerItems(t *testing.T) {
	p := ParsePrices([]byte(sampleData), plannerItems())
	if p.T != 1795605444 || len(p.Houses) != 2 {
		t.Fatalf("got %+v", p)
	}
	a := p.Houses[0]
	if a.ID != 110 || a.Faction != "Alliance" || len(a.Prices) != 2 {
		t.Fatalf("Alliance = %+v", a)
	}
	if got := a.Prices["252463"]; got != (ItemPrice{Min: 7555, Market: 7555, Avg: 5413, Qty: 130}) {
		t.Fatalf("Forceful kit = %+v", got)
	}
	if _, ok := a.Prices["2318"]; ok {
		t.Fatal("Light Leather is not a planner item")
	}
}

func TestPlannerDataIsComplete(t *testing.T) {
	var d struct {
		Tiers []struct {
			Rows []struct {
				Cells []*struct{ Item int64 } `json:"cells"`
			} `json:"rows"`
		} `json:"tiers"`
		Items []struct {
			ID   int64 `json:"id"`
			Icon int64 `json:"icon"`
		} `json:"items"`
		Baseline map[string]float64               `json:"baseline"`
		Icons    struct{ Item map[string]string } `json:"icons"`
	}
	if err := json.Unmarshal([]byte(enchantsData), &d); err != nil {
		t.Fatal(err)
	}
	want := plannerItems()
	for _, tier := range d.Tiers {
		for _, r := range tier.Rows {
			for _, c := range r.Cells {
				if c != nil && !want[c.Item] {
					t.Errorf("cell item %d is not in the item list", c.Item)
				}
			}
		}
	}
	for _, it := range d.Items {
		if d.Baseline[jsonID(it.ID)] <= 0 {
			t.Errorf("item %d has no sheet price", it.ID)
		}
		if !strings.HasPrefix(d.Icons.Item[jsonID(it.Icon)], "data:image/") {
			t.Errorf("item %d has no icon", it.ID)
		}
	}
	if !strings.Contains(enchantsHTML, "{{DATA}}") {
		t.Fatal("enchants.html lost its data slot")
	}
}

func jsonID(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestEnchantPricesReadsInstalledFile(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, KUIFolder), 0o755)
	os.WriteFile(filepath.Join(dir, KUIFolder, KUIFolder+".toc"), []byte("## Interface: 16001\n"+PriceFile+"\n"), 0o644)
	os.WriteFile(filepath.Join(dir, KUIFolder, PriceFile), []byte(sampleData), 0o644)
	a := &App{cfg: Config{AddOnsDir: dir}, syncer: &Syncer{FeedURL: "http://127.0.0.1:1/"}}
	p := a.EnchantPrices()
	if p.Error != "" || len(p.Houses) != 2 || p.Houses[1].Prices["252463"].Min != 5791 {
		t.Fatalf("got %+v", p)
	}
}
