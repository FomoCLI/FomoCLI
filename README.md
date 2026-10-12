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

[English](#install) · [中文](#-中文)

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

## 🇨🇳 中文

`fomo` 是一个命令行工具，让你直接在终端里使用 [fomo.family](https://fomo.family) 的数据，无需打开浏览器。
查看正在盈利的交易员、他们的真实钱包、持仓、投资组合和交易理由（thesis），并实时跟踪交易。

### 安装

```sh
curl -fsSL https://raw.githubusercontent.com/FomoCLI/FomoCLI/main/scripts/install.sh | sh
# 或者
go install github.com/FomoCLI/FomoCLI/cmd/fomo@latest
```

### 快速开始

```sh
fomo --demo lb                 # 先用示例数据体验，无需 API key，也无需联网
fomo auth login                # 输入 API key（免费获取：https://fomoapi.io/dashboard）
fomo lb                        # 24 小时交易员排行榜
fomo trader CryptoKaleo        # 个人资料、钱包、盈亏
fomo t CryptoKaleo positions   # 未平仓和已平仓头寸 + 胜率
fomo t CryptoKaleo balances    # 多链投资组合
fomo tokens                    # 当前热门代币
fomo live                      # 实时交易流（免费，0 积分）
fomo watch frankdegods --bell  # 跟踪单个交易员，交易时响铃提醒
fomo dash                      # 全屏仪表盘
```

### 不联网也能用？

数据来自 FOMO API，所以**第一次获取需要联网**。之后：

- 所有响应都会自动保存到本地缓存。
- `--offline` 显示你最近看过的数据，**完全不需要网络**：
  `fomo lb --offline`、`fomo t CryptoKaleo positions --offline`、`fomo live --offline`。
- 网络断开时，`fomo` 会自动使用缓存，并告诉你数据的时间。
- 在缓存有效期内（默认 60 秒）重复执行命令**不消耗积分**。
- `fomo --demo ...` 使用示例数据，可 100% 离线运行。

### 小技巧

- 任何命令加上 `--json` 即可输出 JSON（可以通过管道传给 `jq`）。
- 每条命令的积分消耗会显示在输出底部（`⚡ 250 credits · 249,750 left`）。
- 用 `fomo cache ls` 查看哪些数据可以离线使用。

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
