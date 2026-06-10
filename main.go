package main

import (
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"solscan-watcher/config"
	"solscan-watcher/db"
	"solscan-watcher/fetcher"
	"solscan-watcher/notifier"
	"solscan-watcher/processor"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelDebug,
	}))
	slog.SetDefault(logger)

	cfg, err := config.Load()
	if err != nil {
		slog.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	database, err := db.New(cfg.DBPath)
	if err != nil {
		slog.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer database.Close()

	fetch := fetcher.New(cfg.SolscanCookie)
	notify := notifier.New(cfg.TelegramToken, cfg.TelegramChatID, cfg.TelegramThreadId, cfg.ErrorCooldown)

	slog.Info("solscan-watcher started",
		"poll_interval", cfg.PollInterval.String(),
		"sol_threshold", cfg.SolThreshold,
		"db_path", cfg.DBPath,
	)

	quit := make(chan os.Signal, 2)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	// Run immediately on startup
	run(fetch, database, notify, cfg.SolThreshold, quit)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			run(fetch, database, notify, cfg.SolThreshold, quit)
		case sig := <-quit:
			slog.Info("shutting down", "signal", sig.String())
			return
		}
	}
}

func run(
	fetch *fetcher.Fetcher,
	database *db.DB,
	notify *notifier.Notifier,
	threshold float64,
	quit chan os.Signal,
) {
	slog.Debug("poll started")
	start := time.Now()

	resp, err := fetch.Fetch()
	if err != nil {
		notify.NotifyError("fetch", err)
		// Cookie expired (403 from Cloudflare) — notify and stop permanently
		if strings.Contains(err.Error(), "403") {
			slog.Error("cookie expired — shutting down until a fresh cf_clearance cookie is provided")
			quit <- syscall.SIGTERM
		}
		return
	}

	positions := processor.Process(resp, threshold)

	inserted := 0
	skipped := 0

	for _, pos := range positions {
		ok, err := database.Upsert(pos)
		if err != nil {
			notify.NotifyError("db upsert", err)
			continue
		}
		if ok {
			inserted++
			slog.Info("new open_position saved",
				"tx_hash", pos.TxHash,
				"wallet", pos.Wallet,
				"sol_value", pos.SolValue,
				"token_address", pos.TokenAddress,
				"token_symbol", pos.TokenSymbol,
				"token_price_usdt", pos.TokenPrice,
			)
		} else {
			skipped++
		}
	}

	total, _ := database.Count()

	slog.Info("poll completed",
		"duration_ms", time.Since(start).Milliseconds(),
		"candidates", len(positions),
		"inserted", inserted,
		"skipped_duplicates", skipped,
		"total_in_db", total,
	)
}