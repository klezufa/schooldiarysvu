package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"schooldiarysvu/internal/config"
	"schooldiarysvu/internal/credentials"
	"schooldiarysvu/internal/database"
)

func main() {
	if err := run(); err != nil {
		slog.Error("administrator bootstrap failed", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	login := strings.ToLower(strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_LOGIN")))
	displayName := strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_DISPLAY_NAME"))
	password := os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")
	if len(login) < 3 || displayName == "" || len([]rune(password)) < 12 {
		return fmt.Errorf("BOOTSTRAP_ADMIN_LOGIN, BOOTSTRAP_ADMIN_DISPLAY_NAME and a password of at least 12 characters are required")
	}
	hash, err := credentials.HashPasswordWithParams(password, credentials.Params{
		MemoryKiB: cfg.Argon2.MemoryKiB, Iterations: cfg.Argon2.Iterations,
		Parallelism: cfg.Argon2.Parallelism, KeyLength: cfg.Argon2.KeyLength,
	})
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := database.Migrate(ctx, cfg.DatabaseURL); err != nil {
		return err
	}
	conn, err := pgx.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer conn.Close(ctx)
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var administrators int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM user_roles ur JOIN roles r ON r.id = ur.role_id WHERE r.code = 'system_admin'").Scan(&administrators); err != nil {
		return err
	}
	if administrators != 0 {
		return fmt.Errorf("a system administrator already exists; bootstrap is disabled")
	}
	var userID string
	if err := tx.QueryRow(ctx, "INSERT INTO users (login, password_hash, display_name, must_change_password) VALUES ($1,$2,$3,true) RETURNING id", login, hash, displayName).Scan(&userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO user_roles SELECT $1, id FROM roles WHERE code = 'system_admin'", userID); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "INSERT INTO audit_logs (actor_id, action, entity_type, entity_id, new_values, request_id) VALUES ($1,'user.bootstrapped','user',$1,'{\"roles\":[\"system_admin\"]}','bootstrap')", userID); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	slog.Info("initial system administrator created; temporary password must be changed at first login", "login", login)
	return nil
}
