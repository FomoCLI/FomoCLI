# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Changed
- The command is now `fomocli` (was `fomo`); config and cache live under `fomocli/`.
- New look matching the FomoCLI logo: ice white on near-black, FOMOCLI banner.

### Added
- `fomocli leaderboard`, `tokens`, `token`, `activity`, `search`, `theses` market commands.
- `fomocli trader <handle> [profile|positions|swaps|balances|following|followers|spotlight|theses]`, `whois`, `trade`.
- `fomocli live` / `fomocli watch` realtime ticker over the app-feed and on-chain WebSockets, with auto-reconnect.
- `fomocli dash` full-screen interactive dashboard.
- Offline cache: every response is stored locally; `--offline` replays it, network failures fall back to it automatically, and repeated calls within the TTL cost 0 credits.
- Live events are recorded locally for `fomocli live --offline` replay.
- `--json` on every command, scriptable exit codes, credit usage footer.
- `--demo` mode with built-in sample data.
