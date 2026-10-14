# unifictl

Query many UniFi consoles and sites at once from the command line.

Consoles come from **sources**:

- **`sitemanager`** — a UniFi Site Manager account (`api.ui.com`). One API key
  discovers every console the account can see and reaches each one through the
  cloud connector. Ideal for MSPs.
- **`local`** — a single console reached directly by URL with its own key.

Queries fan out concurrently across all consoles and sites. Failures on one
console (offline, firmware too old, bad key) are reported as warnings on
stderr; the other results are still printed and the exit code is non-zero.

## Install

```sh
go install github.com/ginkio/unifictl/cmd/unifictl@latest
```

## Configure

```sh
mkdir -p ~/.config/unifictl
cp config.example.yaml ~/.config/unifictl/config.yaml
cp .env.example ~/.config/unifictl/.env   # then fill in your keys
```

API keys referenced by `api_key_env` are read from the environment, with
`.env` files loaded automatically (no `source`/`export` needed):

1. `./.env` in the current directory (may also set `UNIFICTL_CONFIG`)
2. `.env` next to the config file, e.g. `~/.config/unifictl/.env`

Variables already exported in your shell take precedence over `.env` values,
and earlier files take precedence over later ones. Keep `.env` out of git
(`chmod 600` it).

## Usage

```sh
unifictl consoles                   # all consoles from all sources
unifictl consoles --check           # also contact each Network app
unifictl sites
unifictl devices --state OFFLINE    # every offline device, everywhere
unifictl clients --type WIRELESS -C 'acme*'
unifictl devices -S msp -s warehouse -o json | jq .
```

Global filters (all accept multiple values / comma-separated):

| Flag | Filters |
| --- | --- |
| `-S, --source` | source names from the config |
| `-C, --console` | console name or ID (glob, case-insensitive) |
| `-s, --site` | site name, internal reference, or ID (glob, case-insensitive) |

## Notes

- Site Manager is rate limited per API key. Each source has its own request
  limiter (`max_concurrency`, default 4 for `sitemanager`, 8 for `local`) and
  `429` responses are retried honoring `Retry-After`.
- The cloud connector requires reasonably recent console firmware.
