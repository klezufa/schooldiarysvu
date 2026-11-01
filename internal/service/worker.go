package service

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "schooldiarysvu/internal/database/sqlc"
)

func (s *Service) RunWorkerOnce(ctx context.Context, now time.Time) error {
	if err := s.queries.DeleteExpiredSessionsNow(ctx); err != nil {
		return err
	}
	settings, err := s.queries.GetSchoolSettings(ctx)
	if err != nil {
		return mapDatabaseError(err)
	}
	location, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return fmt.Errorf("load school timezone: %w", err)
	}
	localNow := now.In(location)
	recipients, err := s.queries.ListDigestRecipients(ctx)
	if err != nil {
		return err
	}
	for _, recipient := range recipients {
		if recipient.DigestFrequency == "weekly" && localNow.Weekday() != time.Monday {
			continue
		}
		if inQuietHours(localNow, recipient.QuietFrom, recipient.QuietTo) {
			continue
		}
		if err := s.sendDigest(ctx, recipient.ID, recipient.DigestFrequency, localNow); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) sendDigest(ctx context.Context, userID uuid.UUID, frequency string, now time.Time) error {
	period := 24 * time.Hour
	key := "digest:daily:" + now.Format("2006-01-02")
	if frequency == "weekly" {
		period = 7 * 24 * time.Hour
		year, week := now.ISOWeek()
		key = fmt.Sprintf("digest:weekly:%d-%02d", year, week)
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		_, err := q.GetIdempotencyResponse(ctx, db.GetIdempotencyResponseParams{ActorID: userID, Key: key, Operation: "worker.digest"})
		if err == nil {
			return nil
		}
		if !isNoRows(err) {
			return err
		}
		rows, err := q.ListDigestNotifications(ctx, db.ListDigestNotificationsParams{UserID: userID, Column2: now.Add(-period), Column3: now})
		if err != nil {
			return err
		}
		if len(rows) == 0 {
			return nil
		}
		if err := q.CreateNotification(ctx, db.CreateNotificationParams{UserID: userID, Kind: "digest", Title: "Сводка уведомлений",
			Body: fmt.Sprintf("Новых уведомлений за период: %d", len(rows)), EntityType: pgtype.Text{}, EntityID: pgtype.UUID{}}); err != nil {
			return mapDatabaseError(err)
		}
		return q.SaveIdempotencyResponse(ctx, db.SaveIdempotencyResponseParams{ActorID: userID, Key: key, Operation: "worker.digest", RequestHash: key, Response: []byte(`{"sent":true}`)})
	})
}

func inQuietHours(now time.Time, from, to pgtype.Time) bool {
	if !from.Valid || !to.Valid {
		return false
	}
	minute := now.Hour()*60 + now.Minute()
	start := int(from.Microseconds / 1_000_000 / 60)
	end := int(to.Microseconds / 1_000_000 / 60)
	if start == end {
		return true
	}
	if start < end {
		return minute >= start && minute < end
	}
	return minute >= start || minute < end
}
