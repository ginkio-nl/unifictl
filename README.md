# unifictl

A Go command-line tool that gives an MSP an overview of all its customers'
UniFi networks, using a single unifi.ui.com API key.

It lists every site with its device, offline, and client counts and its
internet provider; every device with its status and firmware; and every
network per site with its VLAN, subnet, gateway, and DHCP range.

unifictl only ever reads: it sends GET requests and nothing else.

## Install

```sh
go install github.com/ginkio-nl/unifictl/cmd/unifictl@latest
```

## Configure

Create an API key at **unifi.ui.com → Settings → API Keys**. For `networks`
the key must be created by the **owner** of the consoles, with **UniFi
Applications → Network** ticked for the sites you want. ui.com has no read-only
keys, so treat it as a key that can change settings. Then:

```sh
mkdir -p ~/.config/unifictl
cp .env.example ~/.config/unifictl/.env   # paste the key in
chmod 600 ~/.config/unifictl/.env
```

The key is read from `UNIFI_API_KEY`: an exported variable wins, then
`./.env`, then `~/.config/unifictl/.env`. unifictl refuses to run if a `.env`
file is readable by anyone but you (it must be `600` or `400`).

### Ignoring sites (optional)

To hide sites everywhere, list them in `~/.config/unifictl/config.yaml`
(see `config.example.yaml`):

```yaml
ignore:
  - Default
  - "Dream Router"
  - "keesde*"      # wildcards work
```

Names are matched case-insensitively against the site name unifictl shows.
`--no-ignore` shows everything for one run; `-c` points at another file.

## Usage

```sh
unifictl sites                    # all sites
unifictl sites --offline          # sites with offline devices
unifictl sites -n 'keesde*'       # sites by name
unifictl devices --offline        # offline devices
unifictl devices --updates        # devices with a firmware update
unifictl devices -s 'svrk*'       # devices of a site
unifictl networks                 # networks per site: VLAN, subnet, DHCP
unifictl networks -n 'earforce*'  # networks of one site
unifictl sites -o json | jq .     # JSON output
```

## Limits (from ui.com, not this tool)

- **Devices on a server with many sites have no site.** ui.com only says which
  console or server a device belongs to. For a customer's own console (one
  site) that's enough; for a hosted server with many customers it isn't, so
  `SITE` shows `-` and `devices -s` can't match them.
- **No list of connected clients**, only counts per site.
- **`networks` needs the owner's key.** ui.com only lets a key reach a
  console's Network application if the key's account owns the console and the
  key includes it. Other consoles are skipped with a warning.
- **`networks` is slow** (about a minute for ~25 sites): ui.com rate-limits
  requests, and each network's subnet needs its own request.
- **Site names follow ui.com.** A console with a single site called "Default"
  is shown by the console's name, as unifi.ui.com does. This is observed
  behaviour, not documented; `-o json` keeps the raw name in `siteName`. See
  `DisplayName` in `internal/report/report.go`.

## Development

```sh
make          # vet, test, check formatting, then build ./unifictl
make install  # go install with version info from git describe
make help     # list all targets
```
