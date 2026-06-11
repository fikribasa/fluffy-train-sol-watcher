# solscan-watcher — Context & Deployment

Go app that polls Solscan API for Meteora DLMM `open_position` transactions (program `LBUZKhRxPF3XUpBCjp4YzTKgLccjZhTSDM9YuVaPwxo`), filters by SOL value > threshold, saves to SQLite. Branch: `feature/flaresolver`.

## Architecture (why it works)

The VPS IP is **Cloudflare-banned on `api-v2.solscan.io`** but NOT on the `solscan.io` front page. Two pieces defeat this:

1. **utls + http2.Transport** (`fetcher/fetcher.go`) — mimics Chrome's TLS/JA3 fingerprint. Plain `curl` gets 403 because its TLS handshake isn't Chrome; the Go utls client passes. This is mandatory — the cookie alone is NOT enough.
2. **FlareSolverr cookie refresh** (`fetcher/refresh_cookie.go`) — solves the Cloudflare challenge against `https://solscan.io/` (NOT api-v2, which the IP can't solve) to get a `cf_clearance` cookie valid for `.solscan.io` (all subdomains).

### CRITICAL: cf_clearance is bound to User-Agent + TLS fingerprint + IP
The cookie ONLY works if sent with the **exact UA that solved the challenge**. FlareSolverr returns `solution.userAgent` (e.g. Linux Chrome/148). `RefreshCookie()` captures and stores it; `Fetch()` sends `f.userAgent` dynamically. A UA mismatch (e.g. hardcoded Windows/Chrome149) → instant 403 even with a valid cookie. This was the #1 bug.

### Cookie refresh flow (simplest approach, in use)
- First run with empty cookie → `RefreshCookie()`
- On any 403 → refresh once + retry (reactive)
- NO proactive expiry timer. The cookie's reported `expiry` is ~1 year (storage expiry), but Cloudflare's real cf_clearance validity is ~15-30 min — so trusting `expiry` for proactive refresh is useless. Reactive-on-403 is the reliable path.

### Why NOT Solana RPC
`getSignaturesForAddress` returns ALL txns to the program (swaps, closes, etc.), not just open_position. Scanning 100 recent sigs found ZERO open_positions — they're rare and you'd over-fetch hundreds of full txns with rate-limit risk. Solscan API does server-side filtering via `instruction[]=...dbc0ea47bebf6650` (the open_position discriminator). RPCFetcher exists in `fetcher/rpc.go` as a fallback but is inefficient. publicnode.com also 403-bans this VPS IP under load.

## FlareSolverr — MUST use Docker

The standalone binary at `/opt/flaresolverr/` bundles Chrome 142 which crashes ("invalid session id / renderer disconnected") because system GLIBC is 2.35 and the bundled/snap Chrome needs 2.38. Do NOT try to swap the bundled Chrome — GLIBC mismatch. Use Docker instead:

```bash
docker run -d --name flaresolverr --restart unless-stopped \
  -p 127.0.0.1:8191:8191 \
  -e LOG_LEVEL=info -e HEADLESS=true -e PORT=8191 -e HOST=0.0.0.0 \
  ghcr.io/flaresolverr/flaresolverr:latest
```

Health check: `curl -s http://127.0.0.1:8191/health` → `{"status": "ok"}`

Test cookie solve (sanity):
```bash
curl -s -X POST http://127.0.0.1:8191/v1 -H 'Content-Type: application/json' \
  -d '{"cmd":"request.get","url":"https://solscan.io/","maxTimeout":120000}'
# Expect status:ok, message:"Challenge not detected!", solution.userAgent + cf_clearance cookie
# NOTE: solving api-v2.solscan.io directly FAILS ("IP is banned") — always use solscan.io
```

## Build

```bash
cd ~/agents/projects/solscan-watcher
go build -o solscan-watcher .
```
Deps utls + golang.org/x/net/http2 may need `go get github.com/refraction-networking/utls golang.org/x/net/http2 && go mod tidy` (they're main-branch deps not in feature branch go.mod originally).

## Config (.env)

- `FLARESOLVERR_URL` default `http://127.0.0.1:8191/v1`
- `SOLSCAN_COOKIE` — optional seed cookie (e.g. `cf_clearance=...`). If empty, fetcher self-refreshes on first poll.
- `SOL_THRESHOLD` default 3.0, `POLL_INTERVAL` default 2m, `DB_PATH` default ./watcher.db
- Telegram: `TELEGRAM_BOT_TOKEN`, `TELEGRAM_CHAT_ID`

Constructor wired in main.go: `fetcher.NewWithFlareSolverr(cfg.SolscanCookie, cfg.FlareSolverrURL)`.

## Run under PM2

`ecosystem.config.js` contains ONLY solscan-watcher (flaresolverr runs in Docker, not PM2). cwd = `/home/user/agents/projects/solscan-watcher`.

ALWAYS use explicit PM2_HOME (Hermes profile shell sets $HOME to profile path → spawns duplicate daemon otherwise):
```bash
PM2_HOME=/home/user/.pm2 /home/linuxbrew/.linuxbrew/bin/pm2 start ~/agents/projects/solscan-watcher/ecosystem.config.js
PM2_HOME=/home/user/.pm2 /home/linuxbrew/.linuxbrew/bin/pm2 save
PM2_HOME=/home/user/.pm2 /home/linuxbrew/.linuxbrew/bin/pm2 logs solscan-watcher
```

## Verify working

Run binary foreground first: `timeout 90 ./solscan-watcher`. Expect:
- "solscan-watcher started"
- "new open_position saved" with tx_hash, wallet, sol_value, token_symbol
- "poll completed" with candidates/inserted/total_in_db, HTTP 200 (no 403)

Inspect DB:
```bash
sqlite3 watcher.db "SELECT tx_hash, datetime(block_time,'unixepoch'), wallet, sol_value, token_symbol FROM open_positions ORDER BY block_time DESC LIMIT 20;"
```

## Pitfalls
- 403 / "Just a moment..." HTML in response → cookie expired or UA mismatch. The reactive refresh handles expiry; if persistent, FlareSolverr may be down or IP newly banned on solscan.io front page too (then need a proxy).
- "Cloudflare has blocked this request... IP is banned" from FlareSolverr → you tried solving api-v2.solscan.io. Use solscan.io front page only.
- FlareSolverr renderer crash → you're running the standalone binary, not Docker. Switch to Docker.
- Docker boot persistence: container has `--restart unless-stopped`, but verify Docker daemon starts on boot (`systemctl is-enabled docker`).
