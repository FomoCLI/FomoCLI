# Security

- Your API key is stored in `config.json` with `0600` permissions inside your
  user config directory (`fomocli config path`). Prefer `FOMO_API_KEY` in CI.
- `fomocli` only talks to the configured API base URL (default
  `https://api.fomoapi.io`). It never sends your key anywhere else.
- Every command is read-only except `fomocli launch`, which signs one Pons v2 launch
  transaction. Its private key is read from `FOMO_PRIVATE_KEY` or a hidden prompt, used in
  memory to sign and never written anywhere. Nothing is sent before the transaction has been
  simulated and you have typed the ticker to confirm.

Please report vulnerabilities privately via GitHub Security Advisories on this
repository rather than in a public issue.
