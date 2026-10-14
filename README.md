# unifictl

A Go command-line tool that lets an MSP query all its customers' UniFi
networks from one place.

With a single UniFi Site Manager API key, it finds every console on your
account and reaches each one through Ubiquiti's cloud connector, so you don't
need a VPN or LAN access. It can also query individual consoles directly with
their own keys.

It queries all consoles and sites at the same time and lists consoles, sites,
devices and connected clients as a table or JSON. You can narrow the results
to certain customers or sites, or filter for things like offline devices. If
one customer's console is down, you get a warning for it and still see
everyone else's results.

> **Status:** early. Tested against a fake API server; not yet validated
> against a real UniFi account.

## Sources

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
go install github.com/ginkio-nl/unifictl/cmd/unifictl@latest
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
