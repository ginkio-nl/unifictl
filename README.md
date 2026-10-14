# unifictl

A Go command-line tool that gives an MSP an overview of all its customers'
UniFi networks, using a single unifi.ui.com API key.

It lists every site with its device, offline, and client counts and its
internet provider, and every device with its status and firmware. It works for
all sites your ui.com account can see — including consoles you administer but
don't own.

## Install

```sh
go install github.com/ginkio-nl/unifictl/cmd/unifictl@latest
```

## Configure

Create an API key at **unifi.ui.com → Settings → API Keys**, then:

```sh
mkdir -p ~/.config/unifictl
cp .env.example ~/.config/unifictl/.env   # paste the key in
chmod 600 ~/.config/unifictl/.env
```

The key is read from `UNIFI_SITEMANAGER_KEY`: an exported variable wins, then
`./.env`, then `~/.config/unifictl/.env`.

## Usage

```sh
unifictl sites                    # all sites
unifictl sites --offline          # sites with offline devices
unifictl sites -n 'keesde*'       # sites by name
unifictl devices --offline        # offline devices
unifictl devices --updates        # devices with a firmware update
unifictl devices -s 'svrk*'       # devices of a site
unifictl sites -o json | jq .     # JSON output
```

## Limits (from ui.com, not this tool)

- **Devices on a server with many sites have no site.** ui.com only says which
  console or server a device belongs to. For a customer's own console (one
  site) that's enough; for a hosted server with many customers it isn't, so
  `SITE` shows `-` and `devices -s` can't match them.
- **No list of connected clients**, only counts per site.
- **Site names follow ui.com.** A console with a single site called "Default"
  is shown by the console's name, as unifi.ui.com does. This is observed
  behaviour, not documented; `-o json` keeps the raw name in `siteName`. See
  `siteName` in `internal/report/report.go`.

## Development

```sh
make          # vet, test, check formatting, then build ./unifictl
make install  # go install with version info from git describe
make help     # list all targets
```
