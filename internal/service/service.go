package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"schooldiarysvu/internal/apperr"
	"schooldiarysvu/internal/credentials"
	db "schooldiarysvu/internal/database/sqlc"
)

type Service struct {
	pool       *pgxpool.Pool
	queries    *db.Queries
	passwords  credentials.Params
	logins     *loginLimiter
	invites    *loginLimiter   // отдельный счётчик для перебора кодов приглашений
	addresses  *addressLimiter // неудачные входы с адреса по всем логинам (password spraying)
	fileRoot   string
	maxUpload  int64
	sessionTTL time.Duration
	dummyHash  string
	runtime    RuntimeInfo
	// passwordSlots ограничивает одновременные хеши паролей (passwords.go).
	passwordSlots chan struct{}
}

func New(pool *pgxpool.Pool, passwordParams credentials.Params, fileRoot string, maxUpload int64, sessionTTL time.Duration) (*Service, error) {
	dummyHash, err := credentials.HashPasswordWithParams("not-a-user-password", passwordParams)
	if err != nil {
		return nil, err
	}
	return &Service{
		pool: pool, queries: db.New(pool), passwords: passwordParams,
		logins: newLoginLimiter(), invites: newLoginLimiter(), addresses: newAddressLimiter(), fileRoot: fileRoot, maxUpload: maxUpload,
		sessionTTL: sessionTTL, dummyHash: dummyHash, passwordSlots: newPasswordSlots(defaultPasswordHashConcurrency),
	}, nil
}

func (s *Service) queriesFor(tx pgx.Tx) *db.Queries {
	if tx == nil {
		return s.queries
	}
	return s.queries.WithTx(tx)
}

func (s *Service) transaction(ctx context.Context, operation func(pgx.Tx, *db.Queries) error) error {
	return s.transactionAt(ctx, pgx.Serializable, operation)
}

func (s *Service) transactionReadCommitted(ctx context.Context, operation func(pgx.Tx, *db.Queries) error) error {
	return s.transactionAt(ctx, pgx.ReadCommitted, operation)
}

func (s *Service) transactionAt(ctx context.Context, isolation pgx.TxIsoLevel, operation func(pgx.Tx, *db.Queries) error) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: isolation})
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := operation(tx, s.queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return mapDatabaseError(err)
	}
	return nil
}

func mapDatabaseError(err error) error {
	if err == nil {
		return nil
	}
	var apiErr *apperr.Error
	if errors.As(err, &apiErr) {
		return err
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.NotFound()
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23505":
			return apperr.Conflict("Такая запись уже существует")
		case "23503", "23514", "22P02":
			return apperr.Unprocessable("Некорректные или несовместимые данные")
		case "40001", "40P01":
			return apperr.Conflict("Данные изменились, повторите операцию")
		}
	}
	return err
}

func audit(ctx context.Context, q *db.Queries, actor Principal, action, entityType string, entityID uuid.UUID, oldValues, newValues []byte) error {
	if err := q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		ActorID: nullableUUID(actor.UserID), Action: action, EntityType: entityType,
		EntityID: entityID, OldValues: oldValues, NewValues: newValues, RequestID: actor.RequestID,
	}); err != nil {
		return fmt.Errorf("write audit: %w", err)
	}
	return nil
}
