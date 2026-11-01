package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

type NotificationPreferences struct {
	InternalEnabled bool    `json:"internal_enabled"`
	DigestFrequency string  `json:"digest_frequency"`
	QuietFrom       *string `json:"quiet_from"`
	QuietTo         *string `json:"quiet_to"`
}

type ClassReportItem struct {
	StudentID   uuid.UUID `json:"student_id"`
	DisplayName string    `json:"display_name"`
	SubjectName string    `json:"subject_name"`
	GradeCount  int32     `json:"grade_count"`
	Average     float64   `json:"average"`
}

func (s *Service) ListAnnouncements(ctx context.Context, actor Principal, studentID *uuid.UUID) ([]api.Announcement, error) {
	if studentID != nil || actor.StudentID != nil || actor.ParentID != nil {
		id, err := readableStudentID(actor, studentID)
		if err != nil {
			return nil, err
		}
		if err := canReadStudent(ctx, s.queries, actor, id, time.Now()); err != nil {
			return nil, err
		}
		rows, err := s.queries.ListAnnouncementsForStudent(ctx, id)
		if err != nil {
			return nil, err
		}
		result := make([]api.Announcement, 0, len(rows))
		for _, row := range rows {
			result = append(result, api.Announcement{Id: row.ID, Title: row.Title, Body: row.Body, PublishedAt: row.PublishedAt})
		}
		return result, nil
	}
	if actor.TeacherID != nil && actor.HasRole("teacher") {
		rows, err := s.queries.ListTeacherAnnouncements(ctx, db.ListTeacherAnnouncementsParams{TeacherID: *actor.TeacherID, AuthorID: actor.UserID})
		if err != nil {
			return nil, err
		}
		result := make([]api.Announcement, 0, len(rows))
		for _, row := range rows {
			result = append(result, api.Announcement{Id: row.ID, Title: row.Title, Body: row.Body, PublishedAt: row.PublishedAt})
		}
		return result, nil
	}
	if actor.IsAdministrator() {
		rows, err := s.queries.ListAllAnnouncements(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]api.Announcement, 0, len(rows))
		for _, row := range rows {
			result = append(result, api.Announcement{Id: row.ID, Title: row.Title, Body: row.Body, PublishedAt: row.PublishedAt})
		}
		return result, nil
	}
	return nil, apperr.Forbidden()
}

func (s *Service) CreateAnnouncement(ctx context.Context, actor Principal, input api.CreateAnnouncementJSONBody) (api.Announcement, error) {
	if strings.TrimSpace(input.Title) == "" || strings.TrimSpace(input.Body) == "" || len(input.Title) > 300 || len(input.Body) > 20000 {
		return api.Announcement{}, apperr.Unprocessable("Проверьте заголовок и текст объявления")
	}
	audience := string(input.AudienceType)
	if err := s.requireAnnouncementAudience(ctx, actor, audience, input.ClassId, input.SubjectId); err != nil {
		return api.Announcement{}, err
	}
	var result api.Announcement
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		created, err := q.CreateAnnouncement(ctx, db.CreateAnnouncementParams{AuthorID: actor.UserID, Title: strings.TrimSpace(input.Title), Body: strings.TrimSpace(input.Body)})
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := q.AddAnnouncementAudience(ctx, db.AddAnnouncementAudienceParams{AnnouncementID: created.ID, AudienceType: audience,
			ClassID: optionalUUID(input.ClassId), SubjectID: optionalUUID(input.SubjectId)}); err != nil {
			return mapDatabaseError(err)
		}
		result = api.Announcement{Id: created.ID, Title: created.Title, Body: created.Body, PublishedAt: created.PublishedAt}
		newValues, _ := json.Marshal(map[string]any{"title": created.Title, "audience_type": audience, "class_id": input.ClassId, "subject_id": input.SubjectId})
		if err := audit(ctx, q, actor, "announcement.created", "announcement", created.ID, nil, newValues); err != nil {
			return err
		}
		recipients, err := q.AnnouncementNotificationRecipients(ctx, created.ID)
		if err != nil {
			return err
		}
		for _, userID := range recipients {
			if err := q.CreateNotification(ctx, db.CreateNotificationParams{UserID: userID, Kind: "announcement", Title: "Новое объявление",
				Body: created.Title, EntityType: pgtype.Text{String: "announcement", Valid: true}, EntityID: nullableUUID(created.ID)}); err != nil {
				return err
			}
		}
		return nil
	})
	return result, err
}

func (s *Service) ListNotifications(ctx context.Context, actor Principal, limit, offset int32) ([]api.Notification, error) {
	rows, err := s.queries.ListNotifications(ctx, db.ListNotificationsParams{UserID: actor.UserID, Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	result := make([]api.Notification, 0, len(rows))
	for _, row := range rows {
		result = append(result, api.Notification{Id: row.ID, Kind: row.Kind, Title: row.Title, Body: row.Body,
			EntityType: textPointer(row.EntityType), EntityId: uuidPointer(row.EntityID), ReadAt: timePointer(row.ReadAt), CreatedAt: row.CreatedAt})
	}
	return result, nil
}

func (s *Service) MarkNotificationRead(ctx context.Context, actor Principal, notificationID uuid.UUID) error {
	tag, err := s.queries.MarkNotificationRead(ctx, db.MarkNotificationReadParams{ID: notificationID, UserID: actor.UserID})
	if err != nil {
		return err
	}
	if tag == 0 {
		return apperr.NotFound()
	}
	return nil
}

func (s *Service) SetNotificationPreferences(ctx context.Context, actor Principal, input api.SetNotificationPreferencesJSONBody) (NotificationPreferences, error) {
	frequency := string(input.DigestFrequency)
	if frequency != "never" && frequency != "daily" && frequency != "weekly" {
		return NotificationPreferences{}, apperr.Unprocessable("Неизвестная частота сводки")
	}
	quietFrom, err := parseQuietTime(input.QuietFrom)
	if err != nil {
		return NotificationPreferences{}, err
	}
	quietTo, err := parseQuietTime(input.QuietTo)
	if err != nil {
		return NotificationPreferences{}, err
	}
	if quietFrom.Valid != quietTo.Valid {
		return NotificationPreferences{}, apperr.Unprocessable("Укажите обе границы тихих часов")
	}
	row, err := s.queries.UpsertNotificationPreferences(ctx, db.UpsertNotificationPreferencesParams{UserID: actor.UserID,
		InternalEnabled: input.InternalEnabled, DigestFrequency: frequency, QuietFrom: quietFrom, QuietTo: quietTo})
	if err != nil {
		return NotificationPreferences{}, mapDatabaseError(err)
	}
	return mapNotificationPreferences(row.InternalEnabled, row.DigestFrequency, row.QuietFrom, row.QuietTo), nil
}

func (s *Service) ClassReport(ctx context.Context, actor Principal, classID, periodID uuid.UUID) ([]ClassReportItem, error) {
	period, err := s.queries.GetAcademicPeriod(ctx, periodID)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	if err := requireClassReportAccess(ctx, s.queries, actor, classID, period.StartsOn); err != nil {
		return nil, err
	}
	rows, err := s.queries.ClassReport(ctx, db.ClassReportParams{ClassID: classID, ID: periodID})
	if err != nil {
		return nil, err
	}
	result := make([]ClassReportItem, 0, len(rows))
	for _, row := range rows {
		result = append(result, ClassReportItem{StudentID: row.StudentID, DisplayName: row.DisplayName,
			SubjectName: row.SubjectName, GradeCount: row.GradeCount, Average: row.Average})
	}
	return result, nil
}

func (s *Service) ClassReportCSV(ctx context.Context, actor Principal, classID, periodID uuid.UUID) ([]byte, error) {
	report, err := s.ClassReport(ctx, actor, classID, periodID)
	if err != nil {
		return nil, err
	}
	var buffer bytes.Buffer
	buffer.WriteString("\ufeff")
	writer := csv.NewWriter(&buffer)
	if err := writer.Write([]string{"Ученик", "Предмет", "Количество оценок", "Средний балл"}); err != nil {
		return nil, err
	}
	for _, row := range report {
		if err := writer.Write([]string{spreadsheetSafe(row.DisplayName), spreadsheetSafe(row.SubjectName),
			strconv.Itoa(int(row.GradeCount)), strconv.FormatFloat(row.Average, 'f', 2, 64)}); err != nil {
			return nil, err
		}
	}
	writer.Flush()
	if err := writer.Error(); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (s *Service) ListAudit(ctx context.Context, actor Principal, limit, offset int32, entityType, action string, entityID, actorID *uuid.UUID, from, to *time.Time) ([]api.AuditLog, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, err
	}
	entity, user := uuid.Nil, uuid.Nil
	if entityID != nil {
		entity = *entityID
	}
	if actorID != nil {
		user = *actorID
	}
	fromTime, toTime := time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(9999, 12, 31, 23, 59, 59, 0, time.UTC)
	if from != nil {
		fromTime = *from
	}
	if to != nil {
		toTime = *to
	}
	if toTime.Before(fromTime) {
		return nil, apperr.Unprocessable("Дата окончания фильтра раньше даты начала")
	}
	rows, err := s.queries.ListAuditLogs(ctx, db.ListAuditLogsParams{Column1: entityType, Column2: entity, Column3: user, Limit: limit, Offset: offset, Column6: action, Column7: fromTime, Column8: toTime})
	if err != nil {
		return nil, err
	}
	result := make([]api.AuditLog, 0, len(rows))
	for _, row := range rows {
		result = append(result, api.AuditLog{Id: row.ID, ActorId: uuidPointer(row.ActorID), Action: row.Action,
			EntityType: row.EntityType, EntityId: row.EntityID, OldValues: jsonObject(row.OldValues),
			NewValues: jsonObject(row.NewValues), RequestId: row.RequestID, CreatedAt: row.CreatedAt})
	}
	return result, nil
}

func (s *Service) requireAnnouncementAudience(ctx context.Context, actor Principal, audience string, classID, subjectID *uuid.UUID) error {
	if actor.IsAdministrator() {
		if audience == "class" && classID == nil || audience == "subject" && subjectID == nil {
			return apperr.Unprocessable("Не указана аудитория объявления")
		}
		if audience == "school" && (classID != nil || subjectID != nil) {
			return apperr.Unprocessable("Некорректная школьная аудитория")
		}
		return nil
	}
	if actor.TeacherID == nil || !actor.HasRole("teacher") {
		return apperr.Forbidden()
	}
	now := time.Now()
	switch audience {
	case "class":
		if classID == nil || subjectID != nil {
			return apperr.Unprocessable("Укажите только класс")
		}
		allowed, err := s.queries.TeacherHasClassOnDate(ctx, db.TeacherHasClassOnDateParams{UserID: actor.UserID, ClassID: *classID, Column3: now})
		if err != nil {
			return err
		}
		if !allowed {
			return apperr.NotFound()
		}
		return nil
	case "subject":
		if subjectID == nil || classID != nil {
			return apperr.Unprocessable("Укажите только предмет")
		}
		assignments, err := s.queries.ListTeachingAssignments(ctx)
		if err != nil {
			return err
		}
		for _, assignment := range assignments {
			if assignment.TeacherID == *actor.TeacherID && assignment.SubjectID == *subjectID && !assignment.StartsOn.After(now) &&
				(!assignment.EndsOn.Valid || !assignment.EndsOn.Time.Before(now)) {
				return nil
			}
		}
		return apperr.NotFound()
	default:
		return apperr.Forbidden()
	}
}

func parseQuietTime(value *string) (pgtype.Time, error) {
	if value == nil || *value == "" {
		return pgtype.Time{}, nil
	}
	parsed, err := time.Parse("15:04", *value)
	if err != nil {
		return pgtype.Time{}, apperr.Unprocessable("Время тихих часов должно быть в формате HH:MM")
	}
	return pgtype.Time{Microseconds: int64(parsed.Hour()*3600+parsed.Minute()*60) * 1_000_000, Valid: true}, nil
}

func mapNotificationPreferences(enabled bool, frequency string, from, to pgtype.Time) NotificationPreferences {
	return NotificationPreferences{InternalEnabled: enabled, DigestFrequency: frequency, QuietFrom: quietTimeString(from), QuietTo: quietTimeString(to)}
}

func quietTimeString(value pgtype.Time) *string {
	if !value.Valid {
		return nil
	}
	total := value.Microseconds / 1_000_000
	formatted := fmt.Sprintf("%02d:%02d", total/3600, total%3600/60)
	return &formatted
}

func jsonObject(value []byte) *map[string]interface{} {
	if len(value) == 0 || string(value) == "null" {
		return nil
	}
	var result map[string]interface{}
	if json.Unmarshal(value, &result) != nil {
		return nil
	}
	return &result
}

func spreadsheetSafe(value string) string {
	trimmed := strings.TrimLeft(value, " \t\r\n")
	if strings.HasPrefix(trimmed, "=") || strings.HasPrefix(trimmed, "+") || strings.HasPrefix(trimmed, "-") || strings.HasPrefix(trimmed, "@") {
		return "'" + value
	}
	return value
}
