package main

import (
	_ "embed"
	"encoding/json"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The Enchants window: the level 40 Enchant Stack Planner, priced from the
// same snapshot the app installs for K-Market.

//go:embed enchants.html
var enchantsHTML string

// enchantsData holds the planner's tiers, specs, items, sheet prices and icons.
//
//go:embed enchants.json
var enchantsData string

// ItemPrice is one item's line in a house, in copper.
type ItemPrice struct {
	Min    int64 `json:"min"`    // lowest buyout
	Market int64 `json:"market"` // market value (cheapest slice, outliers dropped)
	Avg    int64 `json:"avg"`    // 7-day average of the market value
	Qty    int64 `json:"qty"`    // how many are listed
}

// HousePrices is one auction house's prices for the planner's items.
type HousePrices struct {
	ID      int                  `json:"id"`
	Faction string               `json:"faction"`
	Prices  map[string]ItemPrice `json:"prices"`
}

// PlannerPrices is what the Enchants window asks for.
type PlannerPrices struct {
	T      int64         `json:"t"`
	Houses []HousePrices `json:"houses"`
	Error  string        `json:"error,omitempty"`
}

var houseLine = regexp.MustCompile(`\[(\d+)\] = \{ faction = "([^"]*)", t = (\d+),\s*d = "([^"]*)"`)

// plannerItems is the set of item IDs the planner prices: the items sold as
// they are (armor kits) and the mats every enchant is made from, since
// enchants themselves are never listed.
func plannerItems() map[int64]bool {
	var d struct {
		Items []struct {
			ID   int64   `json:"id"`
			Mats [][]any `json:"mats"` // [itemID, name, quantity]
		} `json:"items"`
	}
	_ = json.Unmarshal([]byte(enchantsData), &d)
	want := map[int64]bool{}
	for _, it := range d.Items {
		want[it.ID] = true
		for _, m := range it.Mats {
			if id, ok := m[0].(float64); ok {
				want[int64(id)] = true
			}
		}
	}
	return want
}

// ParsePrices reads the wanted items out of a Data.lua snapshot. Each house
// packs its items as "id,min,market,avg,qty;..." (see job/fetch.py).
func ParsePrices(body []byte, want map[int64]bool) PlannerPrices {
	var out PlannerPrices
	for _, m := range houseLine.FindAllStringSubmatch(string(body), -1) {
		id, _ := strconv.Atoi(m[1])
		t, _ := strconv.ParseInt(m[3], 10, 64)
		if t > out.T {
			out.T = t
		}
		h := HousePrices{ID: id, Faction: m[2], Prices: map[string]ItemPrice{}}
		for _, row := range strings.Split(m[4], ";") {
			f := strings.Split(row, ",")
			if len(f) != 5 {
				continue
			}
			var n [5]int64
			for i, s := range f {
				n[i], _ = strconv.ParseInt(s, 10, 64)
			}
			if want[n[0]] {
				h.Prices[f[0]] = ItemPrice{Min: n[1], Market: n[2], Avg: n[3], Qty: n[4]}
			}
		}
		out.Houses = append(out.Houses, h)
	}
	sort.Slice(out.Houses, func(i, j int) bool { return out.Houses[i].ID < out.Houses[j].ID })
	return out
}

// EnchantPrices prices the planner from the installed snapshot, or straight
// from the feed when nothing is installed yet.
func (a *App) EnchantPrices() PlannerPrices {
	a.mu.Lock()
	dir := a.cfg.AddOnsDir
	a.mu.Unlock()
	want := plannerItems()
	if dir != "" {
		if body, err := os.ReadFile(PricePath(dir)); err == nil {
			if p := ParsePrices(body, want); len(p.Houses) > 0 {
				return p
			}
		}
	}
	body, err := a.syncer.get("Data.lua", 64<<20)
	if err != nil {
		return PlannerPrices{Error: "No prices yet: " + err.Error()}
	}
	return ParsePrices(body, want)
}
