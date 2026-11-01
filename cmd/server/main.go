package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"schooldiarysvu/internal/buildinfo"
	"schooldiarysvu/internal/config"
	"schooldiarysvu/internal/credentials"
	"schooldiarysvu/internal/database"
	"schooldiarysvu/internal/httpserver"
	"schooldiarysvu/internal/metrics"
	"schooldiarysvu/internal/service"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	// Пакетные вызовы slog.* (например, из HTTP-слоя) должны попадать в тот же JSON-журнал.
	slog.SetDefault(logger)
	if err := run(logger); err != nil {
		logger.Error("server stopped", "error", err)
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
	if err := database.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()
	app, err := service.New(pool, credentials.Params{
		MemoryKiB: cfg.Argon2.MemoryKiB, Iterations: cfg.Argon2.Iterations,
		Parallelism: cfg.Argon2.Parallelism, KeyLength: cfg.Argon2.KeyLength,
	}, cfg.FileStoragePath, cfg.MaxUploadBytes, cfg.SessionTTL)
	if err != nil {
		return err
	}
	collector, cpuMeter, instance := metrics.New(), metrics.NewCPUMeter(), buildinfo.Instance()
	metrics.PrimeHost()
	app.SetPasswordHashConcurrency(cfg.PasswordHashConcurrency)
	app.SetRuntimeInfo(service.RuntimeInfo{Collector: collector, CPU: cpuMeter, Instance: instance, Version: buildinfo.Current(),
		Environment: cfg.Environment, DiskPath: cfg.FileStoragePath, NetworkProfile: cfg.NetworkProfile, TLSCertFile: cfg.TLSCertFile})
	reporter := &metrics.Reporter{Store: app.MetricsStore(), Service: "server", Instance: instance, Version: buildinfo.Current(), Started: collector.Started(),
		Stats: app.ServerStats, Sample: app.ServerSample, Logger: logger}
	go reporter.Run(ctx)
	routerOptions := []httpserver.RouterOption{httpserver.WithTrustedProxy(cfg.TrustProxyHeaders)}
	if cfg.StaticDir != "" {
		static, err := httpserver.SPAHandler(cfg.StaticDir)
		if err != nil {
			return err
		}
		routerOptions = append(routerOptions, httpserver.WithStaticHandler(static))
		logger.Info("serving frontend from STATIC_DIR", "dir", cfg.StaticDir)
	}
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           httpserver.Middleware(logger, httpserver.WithMetrics(collector, httpserver.NewRouter(pool, app, cfg.SessionCookieSecure, routerOptions...))),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
	}
	errCh := make(chan error, 1)
	go func() {
		logger.Info("server listening", "addr", cfg.HTTPAddr, "profile", cfg.NetworkProfile)
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
