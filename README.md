# wake-proxy

[![ci](https://github.com/Ashen00001/wake-proxy/actions/workflows/ci.yml/badge.svg)](https://github.com/Ashen00001/wake-proxy/actions/workflows/ci.yml) ![license](https://img.shields.io/badge/license-MIT-blue) ![go](https://img.shields.io/badge/Go-stdlib%20only-00ADD8)

A tiny HTTP server that wakes a sleeping machine and sends you to it. Open its link and it:

1. sends a Wake-on-LAN magic packet to the configured MAC,
2. shows a "waking up..." page that refreshes itself while it checks your target URL (e.g. a Jellyfin server),
3. redirects your browser to the target as soon as it answers `200` or `302`.

If the target is already up, it redirects straight away and sends nothing. It's one static binary, Go standard library only.

I run it on a always-on little box on the home network so I can wake the media server from my phone.

## Config

Flags win over environment variables, which win over defaults.

| Flag | Env var | Default | Notes |
|---|---|---|---|
| `-mac` | `WAKE_MAC` | (required) | MAC to wake. `AA:BB:CC:DD:EE:FF`, `aa-bb-cc-dd-ee-ff` or `aabbccddeeff` |
| `-target` | `WAKE_TARGET` | (required) | `http(s)` URL to probe and redirect to |
| `-broadcast` | `WAKE_BROADCAST` | `255.255.255.255:9` | Broadcast address; port defaults to 9. Use your subnet's broadcast (e.g. `192.0.2.255`) if the global one doesn't work |
| `-listen` | `WAKE_LISTEN` | `:8080` | Address to listen on |
| `-timeout` | `WAKE_TIMEOUT` | `3m` | How long to wait for the target before reporting failure. Accepts `90s` or plain seconds in the env var |
| `-rate-limit` | `WAKE_RATE_LIMIT` | `30s` | Minimum gap between magic packets |

## Run

```bash
go run ./cmd/wake-proxy -mac AA:BB:CC:DD:EE:FF -target http://192.0.2.10:8096/ -broadcast 192.0.2.255
```

or with env vars:

```bash
WAKE_MAC=AA:BB:CC:DD:EE:FF WAKE_TARGET=http://192.0.2.10:8096/ ./wake-proxy
```

Then open `http://<this-box>:8080/`. `GET /healthz` returns `ok`.

The machine you're waking needs WoL enabled in its BIOS/NIC settings, and this box has to be on the same LAN (broadcast packets don't cross routers).

## How it works

- Each request to `/` does one quick probe of the target (no redirect following, so Jellyfin's `302` to its web UI counts as up).
- Up: `302` to the target. Nothing is sent.
- Down: the first request starts a wake attempt and sends a packet. The waiting page refreshes every 3 seconds, so each refresh re-probes. Extra packets only go out once `-rate-limit` has passed, so lots of tabs or refreshes don't cause a packet storm. A failed send isn't counted against the limit, so it's retried on the next refresh.
- If the target is still down after `-timeout`, the page switches to a `504` "server didn't come up" page with a "try again" link (`/?retry=1`) that starts a fresh attempt. It doesn't keep sending packets on its own after giving up.

State is in memory, shared across visitors, and resets when the process restarts.

Layout:

```
cmd/wake-proxy     flags/env, wiring
internal/wol       MAC parsing, magic packet, UDP send
internal/probe     HTTP readiness check and Wait helper
internal/proxy     handler, waiting/failure pages, rate limiting
```

## Build and test

```bash
go vet ./...
go test ./...

GOOS=linux   GOARCH=amd64 go build -o dist/wake-proxy       ./cmd/wake-proxy
GOOS=windows GOARCH=amd64 go build -o dist/wake-proxy.exe   ./cmd/wake-proxy
```

Tests need no network or hardware beyond loopback (the handler tests use fakes and a fake clock; probing uses `httptest`). CI runs vet, tests and both builds.

## License

MIT, see [LICENSE](LICENSE).
