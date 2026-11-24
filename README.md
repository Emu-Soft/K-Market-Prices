# K-Market Prices

Auction house prices for K-Market, the market window in [Kain-UI Forever](https://github.com/Emu-Soft/Kain-UI-Forever), on WoW: Forever.

Two parts:

- **Price job** (`job/`): runs every hour on GitHub Actions (`loop.yml`: one run publishes hourly for 5 hours, then starts the next, because GitHub's cron didn't fire for this repo). It reads the PvP auction houses from Blizzard's Battle.net API and publishes `Data.lua` and `meta.json` on the [`data`](../../tree/data) branch.
- **Desktop app** (`app/`): a small Windows tray app. Every 10 minutes it checks the `data` branch and, when a new snapshot is out, writes it into the `KainUIForever_Prices` addon folder. The game picks it up at the next login or `/reload`.

Players don't need a Battle.net developer account. Guildmates without the app get prices shared in game by K-UI.

## Prices

For each item, in copper per unit:

| Field | Meaning |
| --- | --- |
| min | Cheapest buyout |
| market | Mean of the cheapest 15% of units, ignoring units over 1.5x that slice's median |
| avg7d | Mean of the market values seen over the last 7 days |
| qty | Units listed |

## Using the app

1. Download `K-Market-Prices.exe` from the [latest release](https://github.com/Emu-Soft/K-Market-Prices/releases/latest) and run it. Its window shows when prices were last updated, how many items each auction house has, and the AddOns folder it writes to. A gold K coin stays in the tray.
2. It finds the WoW: Forever folder with K-UI in it. If it doesn't, paste your `..._forever_\Interface\AddOns` folder into the window and press **Use this folder**.
3. Turn on **Start with Windows** to keep prices current without thinking about it. Closing the window leaves it running in the tray; click the coin to open it again.

The window uses Microsoft Edge WebView2, which Windows 10 and 11 already have.

## Data format

`Data.lua` sets one global, `KUI_PRICE_DATA`:

```lua
KUI_PRICE_DATA = {
  v = 1, region = "us", ruleset = 1, t = 1795432800,
  houses = {
    [110] = { faction = "Alliance", t = 1795432800,
      d = "2589,180,215,230,4410;2592,950,1100,1080,812;..." },  -- itemId,min,market,avg7d,qty
  },
}
```

## Running the job

The workflow needs two repository secrets, `BNET_CLIENT_ID` and `BNET_CLIENT_SECRET`, from a client made at [develop.battle.net](https://develop.battle.net/access/clients). Houses and region are set in `job/config.json`. Each run's summary lists the house index the API returned.

The exe icon comes from `app/rsrc_windows_amd64.syso`, made with `rsrc -ico icon.ico` (github.com/akavel/rsrc) after `python3 tools/icon.py`.

Local run: `BNET_CLIENT_ID=... BNET_CLIENT_SECRET=... python3 job/fetch.py out`. Tests: `python3 job/test/test_fetch.py` and `cd app && go test ./...`.
