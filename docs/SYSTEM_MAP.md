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
| `frontend/` | Submodule `alireza0/s-ui-frontend` (Vue 3 + Vuetify), built into `web/html` |

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

## Tests

`go test ./...`; real-binary tunnel test: `SUI_MDV_BIN=/path/to/server go test ./service -run DnsTunnel`.
