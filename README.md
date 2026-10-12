<div align="center">

```
███████╗ ██████╗ ███╗   ███╗ ██████╗
██╔════╝██╔═══██╗████╗ ████║██╔═══██╗
█████╗  ██║   ██║██╔████╔██║██║   ██║
██╔══╝  ██║   ██║██║╚██╔╝██║██║   ██║
██║     ╚██████╔╝██║ ╚═╝ ██║╚██████╔╝
╚═╝      ╚═════╝ ╚═╝     ╚═╝ ╚═════╝
```

**fomo.family in your terminal.**

Leaderboards · wallets · PnL · positions · token flow · theses · live trades · offline cache

[![CI](https://github.com/FomoCLI/FomoCLI/actions/workflows/ci.yml/badge.svg)](https://github.com/FomoCLI/FomoCLI/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/go-1.23+-00ADD8?logo=go)](go.mod)
[![License: MIT](https://img.shields.io/badge/license-MIT-green)](LICENSE)

[English](#install) · [Bahasa Indonesia](#-bahasa-indonesia)

</div>

---

`fomo` is a fast, single-binary CLI for [fomo.family](https://fomo.family) data, built on the
[FOMO API](https://fomoapi.io/docs). Find the traders who are up, resolve them to their real
wallets, read their positions and reasoning, and watch their trades land in realtime - without
opening a browser.

```text
▌Leaderboard · 24H  captured 2m ago

  TRADERS    COMBINED PNL    COMBINED VOLUME
  6          +$321.5K        $757.6K

   #  TRADER                         PNL                 VOLUME  TRADES  FOLLOWERS  HOLDS
  ───────────────────────────────────────────────────────────────────────────────────────
  🥇  @CryptoKaleo ✔ K A L E O  +$151.4K  ████████████  $119.2K      73        20K      4
  🥈  @frankdegods ✔ frank       +$98.2K  ███████▊      $402.3K      51     249.5K     12
  🥉  @starcatcher444 star       +$45.1K  ███▌             $88K     140       3.3K      7
   4  @theveeman vee             +$21.9K  █▋               $51K      33        812      3
   5  @laifu                        +$8K  ▋              $20.1K      19        402      2
   6  @ansem ✔                    -$3.1K  ▏                $77K      12       120K      5

  ⚡ 250 credits · 249,750 left
```

<sub>Output from `fomo --demo lb` (sample data). In a real terminal it's in color.</sub>

## Highlights

- **Everything the API offers, as commands.** Traders, positions, swaps, multi-chain portfolios,
  following/followers, spotlight, token flow, dev & insider positions, smart-money holders,
  theses, comment threads, coordinated-buy activity, search.
- **Live ticker.** `fomo live` streams the app feed (or the on-chain feed on Growth+) over a
  WebSocket with auto-reconnect. Streaming costs **0 credits**.
- **Full-screen dashboard.** `fomo dash` puts the leaderboard, token boards, the live feed and
  theses in one keyboard-driven TUI.
- **Offline-first.** Every response is cached on disk. `--offline` replays the last data you saw,
  a dropped connection falls back to the cache automatically, and repeating a command inside the
  cache TTL costs **0 credits**. Live events are recorded too, so `fomo live --offline` works.
- **Credit-aware.** Every command shows what it cost and what's left, and expensive calls are
  labeled in `--help`.
- **Scriptable.** `--json` on every command (stdout stays clean JSON; footers go to stderr),
  meaningful exit codes, shell completion, `FOMO_API_KEY`.
- **Try it with no key.** `fomo --demo ...` runs every command against built-in sample data.

## Install

**Script** (Linux / macOS):

```sh
curl -fsSL https://raw.githubusercontent.com/FomoCLI/FomoCLI/main/scripts/install.sh | sh
```

**Go** (1.23+):

```sh
go install github.com/FomoCLI/FomoCLI/cmd/fomo@latest
```

**Binaries:** download for Linux, macOS or Windows (amd64/arm64) from
[Releases](https://github.com/FomoCLI/FomoCLI/releases).

**From source:**

```sh
git clone https://github.com/FomoCLI/FomoCLI && cd FomoCLI && make build && ./fomo
```

## Quick start

```sh
fomo --demo lb                 # look around with sample data first - no key needed

fomo auth login                # paste your key (free: https://fomoapi.io/dashboard)
fomo lb                        # top traders, 24h
fomo trader CryptoKaleo        # wallets, PnL by window, stats
fomo t CryptoKaleo positions   # open & closed positions, win rate
fomo tokens                    # what FOMO users are trading right now
fomo live --chain robinhood    # realtime ticker
fomo dash                      # full-screen dashboard
```

## Commands

### Market

| Command | What it shows | Cost* |
|---|---|---|
| `fomo lb [24h\|7d\|30d\|all]` | Top traders by PnL. `--sort volume\|trades\|followers`, `--verified`, `-n` | 250 |
| `fomo tokens [trending\|most-held\|graduated]` | FOMO token boards: price, 24h, mcap, holders | 250 |
| `fomo token <addr> [stats]` | Buy/sell flow over 5m · 1h · 4h · 24h, net flow, top-10 concentration | 250 |
| `fomo token <addr> devs` | Deployer & insider positions with their own thesis (rug signal) | 250 |
| `fomo token <addr> holders` | Tracked traders holding it (smart money) | 250 |
| `fomo token <addr> theses` | What traders wrote about it. `--sort likes`, `--threshold`, `--pages` | 1,250/page |
| `fomo activity` | Several traders hitting one coin at once. `--side`, `--min-traders` | 250 |
| `fomo search <query>` | Traders and tokens by name. `--type`, `--tokens` | 250 |
| `fomo theses` | Latest theses. `--chain`, `--sort equity`, `--min-equity` | 1,250 |

### Traders & trades

`fomo trader <handle|userId> [view]` (aliases `t`, `user`):

| View | What it shows | Cost* |
|---|---|---|
| `profile` *(default)* | Wallets (Solana + EVM), PnL 24h/7d/30d/all, volume, trades, followers, avg hold, clan, account age | 2,500 |
| `positions` | Open & closed positions: entry, exit/now, cost, realized/unrealized, win rate. `--status`, `--sort`, `--all --pages N`, `--deep` | 250/page |
| `swaps` | Individual fills | 250 |
| `balances` | Multi-chain portfolio, per-chain split, allocation bars. `--chain` | 250 |
| `following` / `followers` | Their network, each annotated with PnL 24h, volume, trades. `--sort pnl` | 250 |
| `spotlight` | FOMO's own pick of their best trades and theses | 250 |
| `theses` | Everything they wrote. `--sort likes` | 1,250 |

Plus `fomo whois <userId>` (2,500) and `fomo trade <tradeId> [comments]` (250).

### Realtime

| Command | What it does | Cost* |
|---|---|---|
| `fomo live` | Streaming ticker. `--trader`, `--chain`, `--token`, `--type thesis`, `--min-usd`, `--bell`, `--onchain` | **0** |
| `fomo watch <handle>` | Follow one trader live (copy-trading view) | **0** |
| `fomo alerts` | REST snapshot of the feed. `--type whale`, `--since` | 125 |
| `fomo dash` | Full-screen dashboard (keys: `1-4` tabs, `↑↓`, `enter`, `w` window, `b` board, `r` refresh, `q` quit) | per tab |

### Setup & tools

`fomo auth login|status|logout` · `fomo config list|get|set|path` · `fomo status [--watch]` (free) ·
`fomo credits` · `fomo cache ls|clear|path` · `fomo api <path> -p key=value` (any endpoint, raw JSON) ·
`fomo completion bash|zsh|fish|powershell` · `fomo version`

<sub>* Credit costs as documented at [fomoapi.io/docs](https://fomoapi.io/docs#credits). Cached responses cost 0.</sub>

## Offline mode

`fomo` saves every API response it receives. That gives you three things:

1. **Repeat for free** - a command repeated inside the cache TTL (default `60s`) is served from disk
   for 0 credits. Change it with `fomo config set cache_ttl 5m`; skip it with `--fresh`.
2. **Automatic fallback** - if the network is down, `fomo` shows the cached copy and tells you how old it is.
3. **Explicit offline** - `--offline` (or `FOMO_OFFLINE=1`) never touches the network:

```sh
fomo lb --offline                       # last leaderboard you loaded
fomo t CryptoKaleo positions --offline  # last positions you loaded
fomo live --offline -n 50               # replay the last 50 recorded live events
fomo cache ls                           # everything available offline
```

The first load of any view needs a connection - the data comes from FOMO API. After that, it's yours offline.
`--demo` works fully offline with sample data.

## Scripting

```sh
# Top 10 by volume as CSV
fomo lb 7d --json | jq -r '.traders | sort_by(-.volumeUsd)[:10][] | [.handle, .volumeUsd] | @csv'

# Every whale buy on Robinhood Chain, one JSON object per line
fomo live --chain robinhood --min-usd 25000 --json | jq -c '{trader, token, usdValue}'

# Alert when a trader you follow buys
fomo watch frankdegods --bell
```

Exit codes: `0` ok · `1` error · `2` usage · `3` auth (401) · `4` out of credits (402) · `5` not found · `6` offline/no cache.

## Configuration

| Setting | Env var | Default |
|---|---|---|
| `api_key` | `FOMO_API_KEY` | - |
| `cache_ttl` | - | `60s` |
| `default_window` | - | `24h` |
| `default_chain` | - | all chains |
| `theme` (`auto`/`none`) | `NO_COLOR` | `auto` |
| `base_url` | `FOMO_BASE_URL` | `https://api.fomoapi.io` |
| config dir | `FOMO_CONFIG_DIR` | OS config dir + `/fomo` |
| cache dir | `FOMO_CACHE_DIR` | OS cache dir + `/fomo` |

The key is stored with `0600` permissions. Environment variables override the file; `--key` overrides both.

---

## 🇮🇩 Bahasa Indonesia

`fomo` adalah CLI untuk memakai data [fomo.family](https://fomo.family) langsung dari terminal -
tanpa buka browser. Lihat trader yang lagi cuan, wallet aslinya, posisi, portofolio, alasan (thesis)
mereka, dan pantau trade secara realtime.

### Instal

```sh
curl -fsSL https://raw.githubusercontent.com/FomoCLI/FomoCLI/main/scripts/install.sh | sh
# atau
go install github.com/FomoCLI/FomoCLI/cmd/fomo@latest
```

### Mulai

```sh
fomo --demo lb                 # coba dulu pakai data contoh, tanpa API key & tanpa internet
fomo auth login                # masukkan API key (gratis di https://fomoapi.io/dashboard)
fomo lb                        # leaderboard trader 24 jam
fomo trader CryptoKaleo        # profil, wallet, PnL
fomo t CryptoKaleo positions   # posisi open & closed + win rate
fomo t CryptoKaleo balances    # portofolio multi-chain
fomo tokens                    # token yang lagi ramai
fomo live                      # feed trade realtime (gratis, 0 kredit)
fomo watch frankdegods --bell  # pantau satu trader, bunyi saat dia trade
fomo dash                      # dashboard layar penuh
```

### Tanpa internet?

Datanya berasal dari FOMO API, jadi **pengambilan pertama butuh koneksi**. Setelah itu:

- Semua respons otomatis disimpan di cache lokal.
- `--offline` menampilkan data terakhir yang pernah kamu buka, **tanpa internet sama sekali**:
  `fomo lb --offline`, `fomo t CryptoKaleo positions --offline`, `fomo live --offline`.
- Kalau koneksi putus, `fomo` otomatis pakai cache dan memberi tahu umur datanya.
- Perintah yang diulang dalam waktu TTL cache (default 60 detik) **tidak memakai kredit**.
- `fomo --demo ...` jalan 100% offline dengan data contoh.

### Tips

- Tambahkan `--json` di perintah apa pun untuk output JSON (bisa di-pipe ke `jq`).
- Biaya kredit tiap perintah tampil di bawah output (`⚡ 250 credits · 249,750 left`).
- `fomo cache ls` untuk melihat apa saja yang tersedia offline.

---

## Development

```sh
make build    # ./fomo
make test     # go test -race ./...
make lint
./fomo --demo dash
```

The tests and `--demo` use an in-process mock of the API (`internal/mock`), so no key is needed.
See [CONTRIBUTING.md](CONTRIBUTING.md).

## Disclaimer

`fomo` is an independent, unofficial open-source project. It is not affiliated with, endorsed by,
or sponsored by fomo.family or fomoapi.io. Data is provided by the FOMO API under its
[terms](https://fomoapi.io/terms). Nothing here is financial advice. `fomo` is read-only: it never
places trades or touches wallet keys.

## License

[MIT](LICENSE)
