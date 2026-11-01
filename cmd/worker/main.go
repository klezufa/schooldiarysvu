package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"schooldiarysvu/internal/buildinfo"
	"schooldiarysvu/internal/config"
	"schooldiarysvu/internal/credentials"
	"schooldiarysvu/internal/database"
	"schooldiarysvu/internal/metrics"
	"schooldiarysvu/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	// Пакетные вызовы slog.* (например, из HTTP-слоя) должны попадать в тот же JSON-журнал.
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("worker stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	app, err := service.New(pool, credentials.Params{MemoryKiB: cfg.Argon2.MemoryKiB, Iterations: cfg.Argon2.Iterations,
		Parallelism: cfg.Argon2.Parallelism, KeyLength: cfg.Argon2.KeyLength}, cfg.FileStoragePath, cfg.MaxUploadBytes, cfg.SessionTTL)
	if err != nil {
		return err
	}
	started := time.Now()
	var runs, failures atomic.Int64
	var lastRun atomic.Int64
	reporter := &metrics.Reporter{Store: app.MetricsStore(), Service: "worker", Instance: buildinfo.Instance(), Version: buildinfo.Current(), Started: started, Logger: logger,
		Stats: func() map[string]any {
			return map[string]any{"runs": runs.Load(), "failures": failures.Load(), "last_run_at": lastRun.Load(), "interval_seconds": 900}
		}}
	go reporter.Run(ctx)
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		runs.Add(1)
		lastRun.Store(time.Now().Unix())
		if err := app.RunWorkerOnce(ctx, time.Now()); err != nil && ctx.Err() == nil {
			failures.Add(1)
			logger.Error("maintenance iteration failed", "error", err)
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
