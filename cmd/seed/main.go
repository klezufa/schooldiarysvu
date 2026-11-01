package main

import (
	"context"
	_ "embed"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"schooldiarysvu/internal/credentials"
)

//go:embed demo.sql
var demoSQL string

// Расширенный набор для проверки графиков и дашбордов (DEMO_DATASET=rich): применяется поверх базового.
//
//go:embed demo_rich.sql
var demoRichSQL string

// Нагрузочный набор (DEMO_DATASET=load): 360 учеников, 360 родителей, 150 учителей — для сценариев k6 в load/.
//
//go:embed demo_load.sql
var demoLoadSQL string

func main() {
	if err := run(); err != nil {
		slog.Error("demo seed failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	if os.Getenv("ALLOW_DEMO_SEED") != "true" || os.Getenv("APP_ENV") == "production" {
		return fmt.Errorf("demo seed requires ALLOW_DEMO_SEED=true and a non-production APP_ENV")
	}
	password := os.Getenv("DEMO_PASSWORD")
	if len(password) < 12 {
		return fmt.Errorf("DEMO_PASSWORD must contain at least 12 characters and must be development-only")
	}
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		return fmt.Errorf("DATABASE_URL is required")
	}
	passwordHash, err := credentials.HashPassword(password)
	if err != nil {
		return err
	}
	dataset := os.Getenv("DEMO_DATASET")
	timeout := 45 * time.Second
	if dataset == "load" {
		timeout = 10 * time.Minute // сотни тысяч строк посещаемости и оценок
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	conn, err := pgx.Connect(ctx, databaseURL)
	if err != nil {
		return fmt.Errorf("connect database: %w", err)
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin seed transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, "CREATE TEMP TABLE demo_credentials (password_hash text NOT NULL) ON COMMIT DROP"); err != nil {
		return fmt.Errorf("create seed credentials table: %w", err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO demo_credentials VALUES ($1)", passwordHash); err != nil {
		return fmt.Errorf("prepare seed credentials: %w", err)
	}
	if _, err := tx.Exec(ctx, demoSQL); err != nil {
		return fmt.Errorf("apply demo seed: %w", err)
	}
	rich := dataset == "rich"
	if rich {
		if _, err := tx.Exec(ctx, demoRichSQL); err != nil {
			return fmt.Errorf("apply rich demo seed: %w", err)
		}
	}
	if dataset == "load" {
		if _, err := tx.Exec(ctx, demoLoadSQL); err != nil {
			return fmt.Errorf("apply load seed: %w", err)
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit demo seed: %w", err)
	}
	accounts := 11
	switch dataset {
	case "rich":
		accounts = 34
	case "load":
		accounts = 11 + 870
	default:
		dataset = "base"
	}
	slog.Info("development demo data applied", "accounts", accounts, "dataset", dataset, "password_source", "DEMO_PASSWORD")
	return nil
}
