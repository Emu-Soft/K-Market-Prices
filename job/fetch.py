#!/usr/bin/env python3
"""Hourly price job for K-Market.

Pulls the WoW: Forever auction houses from Blizzard's Battle.net Game Data
API, turns every listing into per-item prices and writes:

  out/Data.lua     the file the desktop app copies into the helper addon
  out/meta.json    snapshot time, item counts and the SHA-256 of Data.lua
  out/houses.json  the house index as the API returned it (for checking ids)
  out/history/     daily sums of market values, kept 8 days, for the 7-day average

Only the Python standard library is used. Credentials come from the
environment (BNET_CLIENT_ID, BNET_CLIENT_SECRET); nothing secret is written.

Price fields, all copper per unit:
  min     cheapest buyout
  market  mean of the cheapest 15% of units, after dropping units priced
          over 1.5x the median of that slice (at least one unit)
  avg7d   mean of the market values seen over the last 7 days
  qty     units listed
"""

import base64
import datetime as dt
import gzip
import hashlib
import json
import os
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
CONFIG = json.load(open(os.path.join(HERE, "config.json"), encoding="utf-8"))
REGION = os.environ.get("KM_REGION", CONFIG["region"])
RULESET = int(os.environ.get("KM_RULESET", CONFIG["ruleset"]))
NAMESPACE = "dynamic-forever-" + REGION
API = "https://%s.api.blizzard.com" % REGION
OAUTH = "https://oauth.battle.net/token"
MARKET_SLICE = 0.15
OUTLIER = 1.5
HISTORY_DAYS = 7
FORMAT_VERSION = 1
# Linen Cloth, Wool Cloth, Copper Ore, Light Leather, Peacebloom
PROBE_ITEMS = (2589, 2592, 2770, 2318, 2447)


def log(*args):
    print(*args, file=sys.stderr, flush=True)


def http(req, attempts=4):
    for i in range(attempts):
        try:
            with urllib.request.urlopen(req, timeout=120) as resp:
                body = resp.read()
                if resp.headers.get("Content-Encoding") == "gzip":
                    body = gzip.decompress(body)
                return body
        except urllib.error.HTTPError as e:
            if e.code == 404 or i == attempts - 1:
                raise
            wait = 2 ** i * 5
            log("HTTP %d on %s, retry in %ds" % (e.code, req.full_url, wait))
            time.sleep(wait)
        except urllib.error.URLError as e:
            if i == attempts - 1:
                raise
            wait = 2 ** i * 5
            log("network error %s, retry in %ds" % (e.reason, wait))
            time.sleep(wait)


def token():
    cid = os.environ.get("BNET_CLIENT_ID")
    secret = os.environ.get("BNET_CLIENT_SECRET")
    if not cid or not secret:
        sys.exit("BNET_CLIENT_ID and BNET_CLIENT_SECRET must be set")
    auth = base64.b64encode(("%s:%s" % (cid, secret)).encode()).decode()
    req = urllib.request.Request(
        OAUTH,
        data=b"grant_type=client_credentials",
        headers={"Authorization": "Basic " + auth,
                 "Content-Type": "application/x-www-form-urlencoded"})
    return json.loads(http(req))["access_token"]


def get(tok, path):
    q = urllib.parse.urlencode({"namespace": NAMESPACE, "locale": "en_US"})
    req = urllib.request.Request(
        "%s%s?%s" % (API, path, q),
        headers={"Authorization": "Bearer " + tok, "Accept-Encoding": "gzip"})
    return json.loads(http(req))


def unit_prices(listings):
    """{itemId: [(unitPrice, quantity), ...]} from item + commodity listings."""
    out = {}
    for a in listings:
        item = a.get("item") or {}
        iid = item.get("id")
        qty = int(a.get("quantity") or 0)
        if not iid or qty <= 0:
            continue
        if a.get("unit_price"):
            unit = int(a["unit_price"])
        elif a.get("buyout"):
            unit = int(a["buyout"]) // qty
        else:
            continue  # bid-only auction
        if unit <= 0:
            continue
        out.setdefault(iid, []).append((unit, qty))
    return out


def market_value(rows):
    rows.sort()
    total = sum(q for _, q in rows)
    want = max(1, int(total * MARKET_SLICE + 0.5))
    units = []
    for price, qty in rows:
        take = min(qty, want - len(units))
        units.extend([price] * take)
        if len(units) >= want:
            break
    median = units[len(units) // 2]
    kept = [p for p in units if p <= median * OUTLIER] or units[:1]
    return sum(kept) // len(kept), rows[0][0], total


def load_history(path):
    try:
        return json.load(open(path, encoding="utf-8"))
    except (OSError, ValueError):
        return {}


def main(out_dir):
    os.makedirs(out_dir, exist_ok=True)
    tok = token()
    index = get(tok, "/data/wow/ruleset/%d/auctions/" % RULESET)
    json.dump(index, open(os.path.join(out_dir, "houses.json"), "w"), indent=1)
    log("house index:", [(h.get("id"), h.get("name")) for h in index.get("auctions", [])])

    now = int(time.time())
    today = dt.datetime.utcfromtimestamp(now).strftime("%Y-%m-%d")
    hist_root = os.path.join(out_dir, "history")
    houses_out = {}
    meta_houses = {}

    for faction, house_id in CONFIG["houses"].items():
        listings = []
        try:
            house = get(tok, "/data/wow/ruleset/%d/auctions/%d" % (RULESET, house_id))
            listings.extend(house.get("auctions") or [])
        except urllib.error.HTTPError as e:
            log("house %d items: HTTP %d" % (house_id, e.code))
        try:
            com = get(tok, "/data/wow/ruleset/%d/auctions/%d/commodities" % (RULESET, house_id))
            listings.extend(com.get("auctions") or [])
        except urllib.error.HTTPError as e:
            log("house %d commodities: HTTP %d" % (house_id, e.code))
        if not listings:
            log("house %d (%s): no listings, skipped" % (house_id, faction))
            continue

        prices = {}
        for iid, rows in unit_prices(listings).items():
            prices[iid] = market_value(rows)

        # History: one file per house per UTC day, {itemId: [sum, count]}.
        hdir = os.path.join(hist_root, str(house_id))
        os.makedirs(hdir, exist_ok=True)
        day_path = os.path.join(hdir, today + ".json")
        day = load_history(day_path)
        for iid, (market, _, _) in prices.items():
            s = day.setdefault(str(iid), [0, 0])
            s[0] += market
            s[1] += 1
        json.dump(day, open(day_path, "w"), separators=(",", ":"))

        keep = sorted(f for f in os.listdir(hdir) if f.endswith(".json"))
        for old in keep[:-(HISTORY_DAYS + 1)]:
            os.remove(os.path.join(hdir, old))
        days = [load_history(os.path.join(hdir, f)) for f in keep[-HISTORY_DAYS:]]

        parts = []
        for iid in sorted(prices):
            market, low, qty = prices[iid]
            tot = cnt = 0
            for d in days:
                s = d.get(str(iid))
                if s:
                    tot += s[0]
                    cnt += s[1]
            avg = tot // cnt if cnt else market
            parts.append("%d,%d,%d,%d,%d" % (iid, low, market, avg, qty))
        houses_out[house_id] = (faction, ";".join(parts))
        meta_houses[str(house_id)] = {"faction": faction, "items": len(parts),
                                      "listings": len(listings)}
        log("house %d (%s): %d listings, %d items" % (house_id, faction, len(listings), len(parts)))

    if not houses_out:
        sys.exit("no house returned listings; nothing written")

    lines = [
        "-- K-Market prices. Written by the K-Market price job; do not edit.",
        "KUI_PRICE_DATA = {",
        "\tv = %d, region = %s, ruleset = %d, t = %d," % (FORMAT_VERSION, json.dumps(REGION), RULESET, now),
        "\thouses = {",
    ]
    for house_id, (faction, data) in sorted(houses_out.items()):
        lines.append("\t\t[%d] = { faction = %s, t = %d," % (house_id, json.dumps(faction), now))
        lines.append("\t\t\td = \"%s\" }," % data)
    lines += ["\t},", "}", ""]
    body = "\n".join(lines).encode("utf-8")
    with open(os.path.join(out_dir, "Data.lua"), "wb") as f:
        f.write(body)

    meta = {"v": FORMAT_VERSION, "region": REGION, "ruleset": RULESET, "t": now,
            "sha256": hashlib.sha256(body).hexdigest(), "bytes": len(body),
            "houses": meta_houses}
    json.dump(meta, open(os.path.join(out_dir, "meta.json"), "w"), indent=1)
    log("wrote Data.lua: %d bytes" % len(body))


def probe():
    """Prints listing counts for every house in the index (to find which
    houses belong to which server). Writes nothing."""
    tok = token()
    index = get(tok, "/data/wow/ruleset/%d/auctions/" % RULESET)
    for h in index.get("auctions", []):
        hid = h.get("id")
        counts, listings = [], []
        for suffix in ("", "/commodities"):
            try:
                body = get(tok, "/data/wow/ruleset/%d/auctions/%d%s" % (RULESET, hid, suffix))
                counts.append(len(body.get("auctions") or []))
                listings.extend(body.get("auctions") or [])
            except urllib.error.HTTPError as e:
                counts.append("HTTP %d" % e.code)
        prices = unit_prices(listings)
        # Cheapest buyout of a few common goods, to match a house to what a
        # player sees in game.
        sample = ", ".join("%d: %s" % (iid, min(p for p, _ in prices[iid]) if iid in prices else "-")
                           for iid in PROBE_ITEMS)
        print("- %s %s: %s item auctions, %s commodity auctions; cheapest %s"
              % (hid, h.get("name"), counts[0], counts[1], sample))


if __name__ == "__main__":
    if sys.argv[1:2] == ["--probe"]:
        probe()
    else:
        main(sys.argv[1] if len(sys.argv) > 1 else "out")
