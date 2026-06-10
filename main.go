package main

import (
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"solscan-watcher/config"
	"solscan-watcher/db"
	"solscan-watcher/fetcher"
	"solscan-watcher/notifier"
	"solscan-watcher/processor"
)

func main() {
	// Structured JSON logging — plays nicely with pm2 log viewers
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

	fetch := fetcher.New(cfg.FlareSolverrURL, cfg.FlareSolverrTimeout)
	notify := notifier.New(cfg.TelegramToken, cfg.TelegramChatID, cfg.TelegramThreadId, cfg.ErrorCooldown)

	slog.Info("solscan-watcher started",
		"poll_interval", cfg.PollInterval.String(),
		"sol_threshold", cfg.SolThreshold,
		"db_path", cfg.DBPath,
		"flaresolverr_url", cfg.FlareSolverrURL,
	)

	// Run immediately on startup, then on every tick
	run(fetch, database, notify, cfg.SolThreshold)

	ticker := time.NewTicker(cfg.PollInterval)
	defer ticker.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	for {
		select {
		case <-ticker.C:
			run(fetch, database, notify, cfg.SolThreshold)
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
) {
	slog.Debug("poll started")
	start := time.Now()

	resp, err := fetch.Fetch()
	if err != nil {
		notify.NotifyError("fetch", err)
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
