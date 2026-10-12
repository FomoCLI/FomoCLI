# Security

- Your API key is stored in `config.json` with `0600` permissions inside your
  user config directory (`fomocli config path`). Prefer `FOMO_API_KEY` in CI.
- `fomocli` only talks to the configured API base URL (default
  `https://api.fomoapi.io`). It never sends your key anywhere else.
- `fomocli` is read-only: it does not place trades or handle wallet keys.

Please report vulnerabilities privately via GitHub Security Advisories on this
repository rather than in a public issue.
