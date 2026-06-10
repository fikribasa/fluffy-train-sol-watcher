# solscan-watcher

Watches Meteora DLMM `open_position` transactions on Solscan, filters by SOL value > threshold, and saves to SQLite. Managed by PM2.

---

## Dependencies

- Go 1.21+
- Node.js + PM2 (`npm install -g pm2`)
- FlareSolverr (standalone binary)

---

## 1. Install FlareSolverr

```bash
# Download
wget https://github.com/FlareSolverr/FlareSolverr/releases/latest/download/flaresolverr_linux-x64.tar.gz
tar -xzf flaresolverr_linux-x64.tar.gz
sudo mv flaresolverr_linux-x64 /opt/flaresolverr

# Chromium dependencies (Ubuntu 22.04)
sudo apt-get install -y \
  chromium-browser \
  xvfb \
  libgbm1 \
  libnss3 \
  libatk-bridge2.0-0 \
  libdrm2 \
  libxkbcommon0 \
  libxcomposite1 \
  libxdamage1 \
  libxrandr2

# Test it manually first
cd /opt/flaresolverr && ./flaresolverr
# Should print: FlareSolverr is ready!
# Press Ctrl+C once confirmed
```

---

## 2. Build the watcher

```bash
git clone <repo> /opt/solscan-watcher
cd /opt/solscan-watcher

cp .env.example .env
# No edits needed for basic usage

make build
# Produces ./solscan-watcher binary
```

---

## 3. Configure PM2

```bash
# Update cwd in ecosystem.config.js if needed
# Default assumes /opt/solscan-watcher

pm2 start ecosystem.config.js

# Give flaresolverr ~10s to start before watcher polls
# (watcher will retry automatically if flaresolverr isn't ready)

pm2 save
pm2 startup  # follow the printed command to enable on reboot
```

---

## 4. Monitor

```bash
pm2 status
pm2 logs flaresolverr
pm2 logs solscan-watcher

# Inspect the database
sqlite3 watcher.db "SELECT tx_hash, datetime(block_time,'unixepoch'), wallet, sol_value, token_symbol FROM open_positions ORDER BY block_time DESC LIMIT 20;"
```

---

## 5. Telegram alerts (end phase)

Edit `.env`:
```
TELEGRAM_BOT_TOKEN=your_bot_token
TELEGRAM_CHAT_ID=your_chat_id
```

Then restart:
```bash
pm2 restart solscan-watcher
```

---

## Project structure

```
solscan-watcher/
├── main.go              # entry point, ticker loop
├── config/config.go     # env config loader
├── db/db.go             # sqlite init + upsert
├── fetcher/fetcher.go   # FlareSolverr HTTP client
├── processor/processor.go  # filter + transform logic
├── notifier/notifier.go    # telegram error alerts
├── ecosystem.config.js  # PM2 process definitions
├── Makefile
└── .env.example
```