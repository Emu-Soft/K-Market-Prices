"""Offline test for fetch.py: fakes the API and checks Data.lua and meta.json."""
import hashlib, json, os, re, sys, tempfile
sys.path.insert(0, os.path.join(os.path.dirname(__file__), ".."))
import fetch

HOUSE = {"auctions": [
    {"item": {"id": 2589}, "quantity": 1, "buyout": 200},
    {"item": {"id": 2589}, "quantity": 2, "buyout": 600},
    {"item": {"id": 7000}, "quantity": 1, "bid": 50},          # bid only: ignored
]}
COM = {"auctions": [{"item": {"id": 2592}, "quantity": q, "unit_price": p}
                    for p, q in [(100, 10), (110, 10), (120, 20), (5000, 60)]]}

def fake_get(tok, path):
    if path.endswith("/auctions/"):
        return {"auctions": [{"id": 110, "name": "A"}, {"id": 113, "name": "H"}, {"id": 116, "name": "B"}]}
    if "/113" in path or "/116" in path:
        return {}
    if path.endswith("/commodities"):
        return COM
    return HOUSE

fetch.token = lambda: "t"
fetch.get = fake_get
out = tempfile.mkdtemp()
fetch.main(out)
fetch.main(out)  # second run on the same day adds to history
lua = open(os.path.join(out, "Data.lua"), "rb").read()
meta = json.load(open(os.path.join(out, "meta.json")))
assert meta["sha256"] == hashlib.sha256(lua).hexdigest()
d = re.search(rb'\[110\].*?d = "([^"]*)"', lua, re.S).group(1).decode()
rows = {int(r.split(",")[0]): [int(x) for x in r.split(",")[1:]] for r in d.split(";")}
assert rows[2589] == [200, 200, 200, 3], rows[2589]       # min 200, 15% of 3 units = 1 unit
# 2592: 100 units, slice 15 -> 10 at 100 + 5 at 110 -> mean 103
assert rows[2592] == [100, 103, 103, 100], rows[2592]
assert 7000 not in rows
assert "[113]" not in lua.decode(), "houses with no listings are skipped"
day = json.load(open(os.path.join(out, "history", "110", os.listdir(os.path.join(out, "history", "110"))[0])))
assert day["2592"] == [206, 2]
print("ok")
