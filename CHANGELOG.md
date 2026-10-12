# Changelog

All notable changes to this project are documented here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project uses
[Semantic Versioning](https://semver.org/).

## [Unreleased]

### Added
- `fomo leaderboard`, `tokens`, `token`, `activity`, `search`, `theses` market commands.
- `fomo trader <handle> [profile|positions|swaps|balances|following|followers|spotlight|theses]`, `whois`, `trade`.
- `fomo live` / `fomo watch` realtime ticker over the app-feed and on-chain WebSockets, with auto-reconnect.
- `fomo dash` full-screen interactive dashboard.
- Offline cache: every response is stored locally; `--offline` replays it, network failures fall back to it automatically, and repeated calls within the TTL cost 0 credits.
- Live events are recorded locally for `fomo live --offline` replay.
- `--json` on every command, scriptable exit codes, credit usage footer.
- `--demo` mode with built-in sample data.
