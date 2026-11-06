package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

const maxSubstitutionRangeDays = 62

// ListSubstitutions — без учителя все замены за период (для учёта и оплаты), с учителем — его уроки за период:
// и те, что он ведёт, и те, на которых его заменили.
func (s *Service) ListSubstitutions(ctx context.Context, actor Principal, from, to time.Time, teacherID *uuid.UUID) ([]api.SubstitutionLesson, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, err
	}
	if to.Before(from) || to.Sub(from) > maxSubstitutionRangeDays*24*time.Hour {
		return nil, apperr.Unprocessable(fmt.Sprintf("Проверьте период: не более %d дней", maxSubstitutionRangeDays))
	}
	params := db.ListSubstitutionLessonsParams{FromDate: from, ToDate: to}
	if teacherID != nil {
		params.TeacherID = nullableUUID(*teacherID)
	}
	rows, err := s.queries.ListSubstitutionLessons(ctx, params)
	if err != nil {
		return nil, err
	}
	result := make([]api.SubstitutionLesson, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapSubstitutionLesson(row))
	}
	return result, nil
}

// SubstitutionCandidates — учителя, свободные во время урока. Первыми идут те, кто ведёт этот предмет и знает класс,
// затем те, кто в этот день и так в школе; при равенстве — у кого меньше уроков в этот день.
func (s *Service) SubstitutionCandidates(ctx context.Context, actor Principal, lessonID uuid.UUID) ([]api.SubstitutionCandidate, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, err
	}
	if _, err := s.queries.GetLesson(ctx, lessonID); err != nil {
		return nil, mapDatabaseError(err)
	}
	rows, err := s.queries.SubstitutionCandidates(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	type scored struct {
		candidate api.SubstitutionCandidate
		score     int
	}
	found := make([]scored, 0, len(rows))
	for _, row := range rows {
		candidate := api.SubstitutionCandidate{TeacherId: row.TeacherID, DisplayName: row.DisplayName,
			Reasons: []api.SubstitutionCandidateReasons{}, LessonsThatDay: int(row.LessonsThatDay)}
		score := 0
		if row.TeachesSubject {
			candidate.Reasons = append(candidate.Reasons, api.TeachesSubject)
			score += 4
		}
		if row.KnowsClass {
			candidate.Reasons = append(candidate.Reasons, api.KnowsClass)
			score += 2
		}
		if row.LessonsThatDay > 0 {
			candidate.Reasons = append(candidate.Reasons, api.AtSchool)
			score++
		}
		found = append(found, scored{candidate, score})
	}
	slices.SortStableFunc(found, func(a, b scored) int {
		if a.score != b.score {
			return b.score - a.score
		}
		return a.candidate.LessonsThatDay - b.candidate.LessonsThatDay
	})
	result := make([]api.SubstitutionCandidate, 0, len(found))
	for _, item := range found {
		result = append(result, item.candidate)
	}
	return result, nil
}

// SetLessonSubstitution передаёт урок заменяющему учителю. Учитель по расписанию запоминается, чтобы замену можно
// было отменить и учесть; повторное назначение меняет заменяющего, но не «основного» учителя.
func (s *Service) SetLessonSubstitution(ctx context.Context, actor Principal, lessonID uuid.UUID, input api.SetLessonSubstitutionJSONBody) (api.SubstitutionLesson, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.SubstitutionLesson{}, err
	}
	reason := ""
	if input.Reason != nil {
		reason = strings.TrimSpace(*input.Reason)
	}
	if len([]rune(reason)) > 300 {
		return api.SubstitutionLesson{}, apperr.Unprocessable("Причина замены — не длиннее 300 символов")
	}
	err := s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := lockLessonSchedule(ctx, tx); err != nil {
			return err
		}
		lesson, err := q.GetLesson(ctx, lessonID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if lesson.Status == "cancelled" {
			return apperr.Conflict("Урок отменён — замена ему не нужна")
		}
		if err := requireOpenPeriod(ctx, q, lesson.AcademicPeriodID, true); err != nil {
			return err
		}
		original := lesson.TeacherID
		existing, err := q.GetLessonSubstitution(ctx, lessonID)
		switch {
		case err == nil:
			original = existing.OriginalTeacherID
		case !isNoRows(err):
			return err
		}
		if input.TeacherId == original {
			return apperr.Unprocessable("Это учитель урока по расписанию. Чтобы вернуть ему урок, отмените замену")
		}
		substitute, err := q.GetActiveTeacher(ctx, input.TeacherId)
		if err != nil {
			if isNoRows(err) {
				return apperr.Unprocessable("Заменяющий учитель не найден или его учётная запись отключена")
			}
			return err
		}
		if err := checkLessonConflict(ctx, q, lessonID, lesson.StartsAt, lesson.EndsAt, lesson.ClassID, substitute.ID, uuidPointer(lesson.RoomID)); err != nil {
			return busyTeacher(err, "У этого учителя в это время уже есть урок")
		}
		if err := q.SetLessonTeacher(ctx, db.SetLessonTeacherParams{ID: lessonID, TeacherID: substitute.ID, UpdatedBy: nullableUUID(actor.UserID)}); err != nil {
			return mapDatabaseError(err)
		}
		if err := q.UpsertLessonSubstitution(ctx, db.UpsertLessonSubstitutionParams{LessonID: lessonID, OriginalTeacherID: original,
			Reason: reason, CreatedBy: actor.UserID}); err != nil {
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(map[string]any{"teacher_id": lesson.TeacherID})
		newValues, _ := json.Marshal(map[string]any{"teacher_id": substitute.ID, "original_teacher_id": original, "reason": reason})
		if err := audit(ctx, q, actor, "lesson.substituted", "lesson", lessonID, oldValues, newValues); err != nil {
			return err
		}
		if lesson.TeacherID == substitute.ID || !lesson.EndsAt.After(time.Now()) {
			return nil // уточнили причину или записали прошедшую замену для учёта — уведомлять не о чем
		}
		view, err := q.GetLessonView(ctx, lessonID)
		if err != nil {
			return err
		}
		when := lessonMoment(ctx, q, view.StartsAt)
		if lesson.TeacherID != substitute.ID && lesson.TeacherID != original {
			if err := notifyTeacher(ctx, q, lesson.TeacherID, "Замена отменена", fmt.Sprintf("%s, %s, %s: урок проведёт другой учитель", view.SubjectName, view.ClassName, when), lessonID); err != nil {
				return err
			}
		}
		body := fmt.Sprintf("%s, %s, %s", view.SubjectName, view.ClassName, when)
		if view.RoomName.Valid {
			body += ", кабинет " + view.RoomName.String
		}
		if reason != "" {
			body += ". Причина: " + reason
		}
		if err := notifyTeacher(ctx, q, substitute.ID, "Вам назначена замена", body, lessonID); err != nil {
			return err
		}
		return notifyClass(ctx, q, lesson.ClassID, lesson.LessonDate, "Замена урока",
			fmt.Sprintf("%s %s проведёт %s", view.SubjectName, when, substitute.DisplayName), lessonID)
	})
	if err != nil {
		return api.SubstitutionLesson{}, err
	}
	return s.substitutionLesson(ctx, lessonID)
}

// RemoveLessonSubstitution возвращает урок учителю по расписанию, если тот свободен в это время.
func (s *Service) RemoveLessonSubstitution(ctx context.Context, actor Principal, lessonID uuid.UUID) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	return s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := lockLessonSchedule(ctx, tx); err != nil {
			return err
		}
		substitution, err := q.GetLessonSubstitution(ctx, lessonID)
		if err != nil {
			return mapDatabaseError(err)
		}
		lesson, err := q.GetLesson(ctx, lessonID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := requireOpenPeriod(ctx, q, lesson.AcademicPeriodID, true); err != nil {
			return err
		}
		if lesson.Status != "cancelled" {
			if err := checkLessonConflict(ctx, q, lessonID, lesson.StartsAt, lesson.EndsAt, lesson.ClassID, substitution.OriginalTeacherID, uuidPointer(lesson.RoomID)); err != nil {
				return busyTeacher(err, "У учителя по расписанию в это время уже есть другой урок")
			}
		}
		if err := q.SetLessonTeacher(ctx, db.SetLessonTeacherParams{ID: lessonID, TeacherID: substitution.OriginalTeacherID, UpdatedBy: nullableUUID(actor.UserID)}); err != nil {
			return mapDatabaseError(err)
		}
		if err := q.DeleteLessonSubstitution(ctx, lessonID); err != nil {
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(map[string]any{"teacher_id": lesson.TeacherID, "reason": substitution.Reason})
		newValues, _ := json.Marshal(map[string]any{"teacher_id": substitution.OriginalTeacherID})
		if err := audit(ctx, q, actor, "lesson.substitution_removed", "lesson", lessonID, oldValues, newValues); err != nil {
			return err
		}
		if lesson.Status == "cancelled" || !lesson.EndsAt.After(time.Now()) {
			return nil
		}
		view, err := q.GetLessonView(ctx, lessonID)
		if err != nil {
			return err
		}
		when := lessonMoment(ctx, q, view.StartsAt)
		if err := notifyTeacher(ctx, q, lesson.TeacherID, "Замена отменена", fmt.Sprintf("%s, %s, %s: урок проведёт учитель по расписанию", view.SubjectName, view.ClassName, when), lessonID); err != nil {
			return err
		}
		if err := notifyTeacher(ctx, q, substitution.OriginalTeacherID, "Урок снова ваш", fmt.Sprintf("%s, %s, %s: замена отменена", view.SubjectName, view.ClassName, when), lessonID); err != nil {
			return err
		}
		return notifyClass(ctx, q, lesson.ClassID, lesson.LessonDate, "Замена отменена",
			fmt.Sprintf("%s %s пройдёт по расписанию", view.SubjectName, when), lessonID)
	})
}

// busyTeacher уточняет общий отказ checkLessonConflict: класс и кабинет урока не меняются, занят именно учитель.
func busyTeacher(err error, message string) error {
	var appErr *apperr.Error
	if errors.As(err, &appErr) {
		return apperr.Conflict(message)
	}
	return err
}

func (s *Service) substitutionLesson(ctx context.Context, lessonID uuid.UUID) (api.SubstitutionLesson, error) {
	lesson, err := s.queries.GetLesson(ctx, lessonID)
	if err != nil {
		return api.SubstitutionLesson{}, mapDatabaseError(err)
	}
	rows, err := s.queries.ListSubstitutionLessons(ctx, db.ListSubstitutionLessonsParams{FromDate: lesson.LessonDate, ToDate: lesson.LessonDate,
		TeacherID: nullableUUID(lesson.TeacherID)})
	if err != nil {
		return api.SubstitutionLesson{}, err
	}
	for _, row := range rows {
		if row.ID == lessonID {
			return mapSubstitutionLesson(row), nil
		}
	}
	return api.SubstitutionLesson{}, apperr.NotFound()
}

func mapSubstitutionLesson(row db.ListSubstitutionLessonsRow) api.SubstitutionLesson {
	result := api.SubstitutionLesson{Id: row.ID, LessonDate: openapi_types.Date{Time: row.LessonDate}, StartsAt: row.StartsAt, EndsAt: row.EndsAt,
		Status: api.SubstitutionLessonStatus(row.Status), ClassName: row.ClassName, SubjectName: row.SubjectName, RoomName: textPointer(row.RoomName),
		TeacherId: row.TeacherID, TeacherName: row.TeacherName, OriginalTeacherName: textPointer(row.OriginalTeacherName), Reason: row.Reason}
	if row.OriginalTeacherID.Valid {
		id := uuid.UUID(row.OriginalTeacherID.Bytes)
		result.OriginalTeacherId = &id
	}
	return result
}

// lessonMoment — «3 ноября, 10:00» по часовому поясу школы: уведомление читают люди, а не программы.
func lessonMoment(ctx context.Context, q *db.Queries, startsAt time.Time) string {
	location := time.UTC
	if settings, err := q.GetSchoolSettings(ctx); err == nil {
		if loaded, err := time.LoadLocation(settings.Timezone); err == nil {
			location = loaded
		}
	}
	local := startsAt.In(location)
	return fmt.Sprintf("%d %s, %s", local.Day(), genitiveMonths[local.Month()-1], local.Format("15:04"))
}

var genitiveMonths = [...]string{"января", "февраля", "марта", "апреля", "мая", "июня", "июля", "августа", "сентября", "октября", "ноября", "декабря"}

func notifyTeacher(ctx context.Context, q *db.Queries, teacherID uuid.UUID, title, body string, lessonID uuid.UUID) error {
	teacher, err := q.GetActiveTeacher(ctx, teacherID)
	if isNoRows(err) {
		return nil // учётная запись отключена — уведомлять некого
	}
	if err != nil {
		return err
	}
	enabled, err := q.UserNotificationsEnabled(ctx, teacher.UserID)
	if err != nil || !enabled {
		return err
	}
	return q.CreateNotification(ctx, db.CreateNotificationParams{UserID: teacher.UserID, Kind: "schedule", Title: title, Body: body,
		EntityType: pgtype.Text{String: "lesson", Valid: true}, EntityID: nullableUUID(lessonID)})
}

func notifyClass(ctx context.Context, q *db.Queries, classID uuid.UUID, onDate time.Time, title, body string, lessonID uuid.UUID) error {
	recipients, err := q.ClassNotificationRecipients(ctx, db.ClassNotificationRecipientsParams{OnDate: onDate, ClassID: classID})
	if err != nil {
		return err
	}
	for _, userID := range recipients {
		if err := q.CreateNotification(ctx, db.CreateNotificationParams{UserID: userID, Kind: "schedule", Title: title, Body: body,
			EntityType: pgtype.Text{String: "lesson", Valid: true}, EntityID: nullableUUID(lessonID)}); err != nil {
			return err
		}
	}
	return nil
}
