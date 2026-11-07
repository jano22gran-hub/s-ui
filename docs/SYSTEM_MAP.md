# S-UI System Map

Short reference for working on this fork without re-scanning the tree. Update
it with every structural change.

## Layout

| Path | Role |
|---|---|
| `main.go`, `cmd/` | CLI entry (`sui` + admin sub-commands) |
| `app/app.go` | Boot/stop order: DB → core → cron → web → sub → core start → DNS tunnels |
| `config/` | Env paths (`SUI_DB_FOLDER`, `SUI_DEBUG`, `SUI_LOG_LEVEL`), name/version |
| `database/` | GORM + SQLite. `schema.go` = every table (migrate + backup). `model/` = rows |
| `core/` | Embedded sing-box 1.14 (`main.go` Core, `box.go`), custom protocol inbounds in `core/protocol/*` with in-place user updates, `usersession/` session mute/kick registry, `tracker_session.go` traffic counters |
| `service/` | Business logic. `ConfigService` = save dispatcher + core lifecycle (`lifecycleMu`), `ClientService` (deplete/reset), `InboundService` (users → core), `StatsService`, `SettingService`, `DnsTunnelService` |
| `dnstunnel/` | MasterDnsVPN child processes: `manager.go` (Apply/StopAll/Statuses, restart w/ backoff), `install.go` (release download → `bin/masterdnsvpn-server`, or `$SUI_MDV_BIN`) |
| `cronjob/` | stats 10s, deplete 1m, global reset 1m poll, watchdog 5s, WAL 10m, old-stats daily |
| `api/` | `/api/:action` (session) and `/apiv2/:action` (token). `apiService.go` handlers |
| `sub/` | Subscription server: links, sing-box JSON, Clash/Mihomo YAML |
| `util/` | Link generation (`genLink.go`), link → outbound (`linkToJson.go`), external subs |
| `frontend/` | Submodule `jano22gran-hub/s-ui-frontend` (branch `main`) (Vue 3 + Vuetify), built into `web/html` |

## Data flow

- Save: `POST /api/save obj=<clients|inbounds|outbounds|services|endpoints|tls|config|settings|dnstunnels> act=<new|edit|del>` → `ConfigService.Save` (one tx) → in-place core update → after commit `DnsTunnelService.Sync` for `dnstunnels|inbounds|clients`.
- Load: `GET /api/load` returns all objects incl. `dnstunnels`; partial: `GET /api/<obj>`.
- Users in core: built from `clients` where `enable = true`, `json_extract(config, '$.<type>')`.
- Disable (quota/expiry): `DepleteJob` → `UpdateInboundsUsers` (in place, cuts sessions).
- Global reset: `ResetTrafficJob` → `ConfigService.ResetAllTraffic` → in-place re-add, restart only on failure. `clientStateMu` serialises with deplete.

## DNS tunnel (MasterDnsVPN)

- Table `dns_tunnels`: tag, enable, domain (NS-delegated zone), listen, port (53), method 0–5, key (hex, auto), inbound (socks|mixed tag), client (name), options (extra upper-case MDV keys).
- Process config: `PROTOCOL_TYPE=SOCKS5`, `USE_EXTERNAL_SOCKS5=true` → forwards into the sing-box inbound at 127.0.0.1:<listen_port> with the client's socks/mixed credentials, so traffic is counted and limited per client.
- Files: `<db>/dnstunnel/<tag>/{server_config.json,encrypt_key.txt}`.
- API: `GET dnstunnels` (+ `mdvVersion`, per-tunnel `status`), `POST dnstunnelInstall [version]`, `GET dnstunnelClient?tag=` → client TOML.
- Note: MDV refuses loopback/private *targets*; the forward hop itself may be loopback.
- DNS setup: `A ns.example.com → server IP`, `NS t.example.com → ns.example.com`; port 53/udp must be free (disable systemd-resolved stub).

## Logging

- Panel level: setting `logLevel` (`""|debug|info|warn|error`, Settings → general). Applied at boot (`app.Init` → `SettingService.ApplyLogLevel`) and after every `settings` save. `SUI_DEBUG` / `SUI_LOG_LEVEL` env override it.
- `logger/`: `SetLevel`, `ParseLevel`; the in-memory buffer (`GetLogs`, `/api/logs`) only keeps lines at or above the active level.
- sing-box core level = `config.log.level` (Basics page, stored in setting `config`); core lines also pass through the panel level.
- Frontend `Logs.vue` remembers filter/count in `localStorage` key `sui.logs.prefs`.

## Tests

`go test ./...`; real-binary tunnel test: `SUI_MDV_BIN=/path/to/server go test ./service -run DnsTunnel`.

## Frontend (s-ui-frontend fork, branch `main`)

- Nav groups: `src/layouts/default/Drawer.vue` (Overview / Users / Routing / Services / System).
- DNS tunnels: `src/views/DnsTunnels.vue` (polls `api/dnstunnels` every 5s), `src/layouts/modals/DnsTunnel.vue`, `src/types/dnsTunnels.ts`, store field `dnsTunnels` (`api/load` key `dnstunnels`).
- Clients volume column sorts by `up + down` (`src/views/Clients.vue` headers).
- Strings: `src/locales/{en,fa}.ts` keys `pages.dnstunnels`, `objects.dnstunnel`, `dnstunnel.*`, `menu.*`; other locales fall back to en.

## Subscription defaults

- `ensureCacheFile`: adds `experimental.cache_file.enabled` unless the JSON extension sets `cache_file` (clients keep the node picked in "proxy" across restarts; tests `sub/json_cache_file_test.go`).
- `sub/jsonService.go` `defaultJson`: tun `stack: gvisor` (system stack left Android TCP stuck after pre-match on cellular; test `sub/json_tun_stack_test.go`), mtu 9000, auto_route, mixed 127.0.0.1:2080.

## sing-box dependency

- Stays on upstream release `sing-box v1.14.4`; managed users live in `core/protocol/*` (+ `core/usersession`, `core/quicgrace`).
- Fork `jano22gran-hub/Singbox` has the same managed-user code upstreamed, but is **not** wired in: it is private (public builds could not fetch it) and based on upstream `testing`. Wire it (`replace` in `go.mod`, drop `core/protocol/*`) only once it is public and rebased on a release tag.
- Security: govulncheck clean (brotli 1.2.7, klauspost/compress 1.20.2). Re-run: `GOTOOLCHAIN=go1.27.3 go run golang.org/x/vuln/cmd/govulncheck@latest -tags "$(tags_for test)" ./...`.

## Upstream sync
- Synced with `alireza0/s-ui` v1.6.7 (fork version `1.6.7-dns.4`).
- In-panel update (`service/update.go`, `service/upgrade.go`) and `s-ui.sh` point to `jano22gran-hub/s-ui` releases; tags `vX.Y.Z-dns.N` accepted, `-dns.N` compared as a 4th component.
- Global traffic reset: `ConfigService.ResetAllTraffic` (in-place user update, core restart fallback).
