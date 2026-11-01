package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

type GradeAverages struct {
	Average         float64 `json:"average"`
	WeightedAverage float64 `json:"weighted_average"`
	GradeCount      int32   `json:"grade_count"`
}

type BulkAttendanceResult struct {
	CreatedOrUpdated int              `json:"created_or_updated"`
	Items            []api.Attendance `json:"items"`
}

func (s *Service) GetJournal(ctx context.Context, actor Principal, classID, subjectID, periodID uuid.UUID) ([]api.JournalEntry, error) {
	period, err := s.queries.GetAcademicPeriod(ctx, periodID)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	allowed := false
	if actor.HasRole("class_teacher") {
		allowed, err = s.queries.ClassTeacherHasClass(ctx, db.ClassTeacherHasClassParams{UserID: actor.UserID, ClassID: classID, Column3: period.StartsOn})
		if err != nil {
			return nil, err
		}
	}
	if !allowed && actor.TeacherID != nil && actor.HasRole("teacher") {
		if err := requireTeacherAssignment(ctx, s.queries, actor, classID, subjectID, periodID, period.StartsOn); err == nil {
			allowed = true
		} else {
			var apiErr *apperr.Error
			if !errors.As(err, &apiErr) {
				return nil, err
			}
		}
	}
	if !allowed {
		return nil, apperr.NotFound()
	}
	rows, err := s.queries.ListJournalGrades(ctx, db.ListJournalGradesParams{ClassID: classID, SubjectID: subjectID, AcademicPeriodID: periodID})
	if err != nil {
		return nil, err
	}
	result := make([]api.JournalEntry, 0, len(rows))
	for _, row := range rows {
		entry := api.JournalEntry{StudentId: row.StudentID, StudentName: row.StudentName, LessonId: row.LessonID,
			LessonDate: apiDate(row.LessonDate), LessonWorkId: row.LessonWorkID, WorkType: row.WorkType,
			WorkTitle: row.WorkTitle, Weight: numericValue(row.Weight), GradeId: uuidPointer(row.GradeID), GradeComment: textPointer(row.GradeComment)}
		if row.GradeValue.Valid {
			value := numericValue(row.GradeValue)
			entry.GradeValue = &value
		}
		if row.GradeVersion.Valid {
			version := int(row.GradeVersion.Int32)
			entry.GradeVersion = &version
		}
		result = append(result, entry)
	}
	return result, nil
}

func (s *Service) ListLessonWorks(ctx context.Context, actor Principal, lessonID uuid.UUID) ([]api.LessonWork, error) {
	lesson, err := s.queries.GetLessonView(ctx, lessonID)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	if err := requireLessonRead(ctx, s.queries, actor, lesson); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListLessonWorks(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	result := make([]api.LessonWork, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapLessonWork(row.ID, row.LessonID, row.WorkType, row.Title, row.Weight, row.IsImportant, row.Position))
	}
	return result, nil
}

func (s *Service) CreateLessonWork(ctx context.Context, actor Principal, input api.CreateLessonWorkJSONBody) (api.LessonWork, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.LessonWork{}, err
	}
	if strings.TrimSpace(input.Title) == "" || input.Weight <= 0 || input.Weight > 100 || input.Position < 1 || input.Position > 100 || !validWorkType(string(input.WorkType)) {
		return api.LessonWork{}, apperr.Unprocessable("Проверьте тип, название, вес и позицию работы")
	}
	var result api.LessonWork
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		lesson, err := q.GetLesson(ctx, input.LessonId)
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := requireLessonWrite(ctx, q, actor, lesson.ClassID, lesson.SubjectID, lesson.TeacherID, lesson.AcademicPeriodID, lesson.LessonDate); err != nil {
			return err
		}
		if err := requireOpenPeriod(ctx, q, lesson.AcademicPeriodID, true); err != nil {
			return err
		}
		important := input.IsImportant != nil && *input.IsImportant
		created, err := q.CreateLessonWork(ctx, db.CreateLessonWorkParams{LessonID: input.LessonId, WorkType: string(input.WorkType),
			Title: strings.TrimSpace(input.Title), Weight: number(input.Weight), IsImportant: important, Position: int32(input.Position)})
		if err != nil {
			return mapDatabaseError(err)
		}
		result = mapLessonWork(created.ID, created.LessonID, created.WorkType, created.Title, created.Weight, created.IsImportant, created.Position)
		newValues, _ := json.Marshal(map[string]any{"lesson_id": created.LessonID, "work_type": created.WorkType, "title": created.Title, "weight": numericValue(created.Weight)})
		return audit(ctx, q, actor, "lesson_work.created", "lesson_work", created.ID, nil, newValues)
	})
	return result, err
}

func (s *Service) CreateGrade(ctx context.Context, actor Principal, input api.CreateGradeJSONBody) (api.Grade, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.Grade{}, err
	}
	var result api.Grade
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		work, err := q.GetLessonWork(ctx, input.LessonWorkId)
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := requireGradeChange(ctx, q, actor, work, input.StudentId, input.Value); err != nil {
			return err
		}
		created, err := q.CreateGrade(ctx, db.CreateGradeParams{StudentID: input.StudentId, LessonWorkID: input.LessonWorkId,
			AcademicPeriodID: work.AcademicPeriodID, Value: number(input.Value), Comment: nullableText(input.Comment), CreatedBy: actor.UserID})
		if err != nil {
			return mapDatabaseError(err)
		}
		result = api.Grade{Id: created.ID, StudentId: created.StudentID, LessonWorkId: created.LessonWorkID,
			AcademicPeriodId: created.AcademicPeriodID, Value: numericValue(created.Value), Comment: textPointer(created.Comment), Version: int(created.Version)}
		newValues, _ := json.Marshal(map[string]any{"value": input.Value, "comment": input.Comment, "version": created.Version})
		if err := audit(ctx, q, actor, "grade.created", "grade", created.ID, nil, newValues); err != nil {
			return err
		}
		return notifyStudent(ctx, q, input.StudentId, work.LessonDate, "grade", "Новая оценка", "Учитель выставил оценку", "grade", created.ID)
	})
	return result, err
}

func (s *Service) UpdateGrade(ctx context.Context, actor Principal, gradeID uuid.UUID, version int32, input api.UpdateGradeJSONBody) (api.Grade, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.Grade{}, err
	}
	var result api.Grade
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		grade, work, err := gradeAndWork(ctx, q, gradeID)
		if err != nil {
			return err
		}
		if err := requireGradeChange(ctx, q, actor, work, grade.StudentID, input.Value); err != nil {
			return err
		}
		updated, err := q.UpdateGrade(ctx, db.UpdateGradeParams{ID: gradeID, Value: number(input.Value), Comment: nullableText(input.Comment), Version: version})
		if err != nil {
			if isNoRows(err) {
				return apperr.Conflict("Оценка была изменена. Обновите данные и повторите операцию")
			}
			return mapDatabaseError(err)
		}
		result = api.Grade{Id: grade.ID, StudentId: grade.StudentID, LessonWorkId: grade.LessonWorkID,
			AcademicPeriodId: grade.AcademicPeriodID, Value: numericValue(updated.Value), Comment: textPointer(updated.Comment), Version: int(updated.Version)}
		oldValues, _ := json.Marshal(map[string]any{"value": numericValue(grade.Value), "comment": textPointer(grade.Comment), "version": grade.Version})
		newValues, _ := json.Marshal(map[string]any{"value": input.Value, "comment": input.Comment, "version": updated.Version})
		if err := audit(ctx, q, actor, "grade.updated", "grade", gradeID, oldValues, newValues); err != nil {
			return err
		}
		return notifyStudent(ctx, q, grade.StudentID, work.LessonDate, "grade", "Оценка изменена", "Учитель изменил оценку", "grade", gradeID)
	})
	return result, err
}

func (s *Service) DeleteGrade(ctx context.Context, actor Principal, gradeID uuid.UUID, version int32) error {
	if err := actor.requireTeacher(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		grade, work, err := gradeAndWork(ctx, q, gradeID)
		if err != nil {
			return err
		}
		if err := requireGradeChange(ctx, q, actor, work, grade.StudentID, numericValue(grade.Value)); err != nil {
			return err
		}
		if _, err := q.SoftDeleteGrade(ctx, db.SoftDeleteGradeParams{ID: gradeID, Version: version}); err != nil {
			if isNoRows(err) {
				return apperr.Conflict("Оценка была изменена. Обновите данные и повторите операцию")
			}
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(map[string]any{"value": numericValue(grade.Value), "version": grade.Version})
		newValues, _ := json.Marshal(map[string]any{"deleted": true, "version": version + 1})
		return audit(ctx, q, actor, "grade.deleted", "grade", gradeID, oldValues, newValues)
	})
}

func (s *Service) ListGrades(ctx context.Context, actor Principal, studentID *uuid.UUID, periodID *uuid.UUID) ([]api.StudentGrade, error) {
	id, err := readableStudentID(actor, studentID)
	if err != nil {
		return nil, err
	}
	if err := canReadStudent(ctx, s.queries, actor, id, time.Now()); err != nil {
		return nil, err
	}
	filter := uuid.Nil
	if periodID != nil {
		filter = *periodID
	}
	rows, err := s.queries.ListStudentGrades(ctx, db.ListStudentGradesParams{StudentID: id, Column2: filter})
	if err != nil {
		return nil, err
	}
	result := make([]api.StudentGrade, 0, len(rows))
	for _, row := range rows {
		result = append(result, api.StudentGrade{Id: row.ID, SubjectId: row.SubjectID, SubjectName: row.SubjectName,
			LessonDate: apiDate(row.LessonDate), WorkType: row.WorkType, Value: numericValue(row.Value),
			Comment: textPointer(row.Comment), Version: int(row.Version), Weight: numericValue(row.Weight)})
	}
	return result, nil
}

func (s *Service) GetGradeAverages(ctx context.Context, actor Principal, studentID, subjectID, periodID uuid.UUID) (GradeAverages, error) {
	if err := canReadStudent(ctx, s.queries, actor, studentID, time.Now()); err != nil {
		return GradeAverages{}, err
	}
	row, err := s.queries.GradeAverages(ctx, db.GradeAveragesParams{StudentID: studentID, SubjectID: subjectID, AcademicPeriodID: periodID})
	if err != nil {
		return GradeAverages{}, err
	}
	return GradeAverages{Average: row.Average, WeightedAverage: row.WeightedAverage, GradeCount: row.GradeCount}, nil
}

func (s *Service) UpsertTermGrade(ctx context.Context, actor Principal, input api.UpsertTermGradeJSONBody) (api.TermGrade, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.TermGrade{}, err
	}
	var result api.TermGrade
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		allowed, err := q.TeacherCanGradeTerm(ctx, db.TeacherCanGradeTermParams{UserID: actor.UserID, StudentID: input.StudentId,
			SubjectID: input.SubjectId, AcademicPeriodID: input.AcademicPeriodId})
		if err != nil {
			return err
		}
		if !allowed {
			return apperr.NotFound()
		}
		if err := requireOpenPeriod(ctx, q, input.AcademicPeriodId, true); err != nil {
			return err
		}
		if err := validateGradeValue(ctx, q, input.Value); err != nil {
			return err
		}
		created, err := q.UpsertTermGrade(ctx, db.UpsertTermGradeParams{StudentID: input.StudentId, SubjectID: input.SubjectId,
			AcademicPeriodID: input.AcademicPeriodId, Value: number(input.Value), CreatedBy: actor.UserID})
		if err != nil {
			return mapDatabaseError(err)
		}
		result = api.TermGrade{Id: created.ID, StudentId: created.StudentID, SubjectId: created.SubjectID,
			AcademicPeriodId: created.AcademicPeriodID, Value: numericValue(created.Value), Version: int(created.Version)}
		newValues, _ := json.Marshal(map[string]any{"student_id": input.StudentId, "subject_id": input.SubjectId, "value": input.Value, "version": created.Version})
		if err := audit(ctx, q, actor, "term_grade.upserted", "term_grade", created.ID, nil, newValues); err != nil {
			return err
		}
		return notifyStudent(ctx, q, input.StudentId, time.Now(), "grade", "Итоговая оценка", "Учитель сохранил итоговую оценку", "term_grade", created.ID)
	})
	return result, err
}

func (s *Service) ListAttendance(ctx context.Context, actor Principal, lessonID uuid.UUID) ([]api.Attendance, error) {
	lesson, err := s.queries.GetLessonView(ctx, lessonID)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	if err := requireLessonRead(ctx, s.queries, actor, lesson); err != nil {
		return nil, err
	}
	if actor.StudentID != nil || actor.ParentID != nil {
		return nil, apperr.Forbidden()
	}
	rows, err := s.queries.ListAttendance(ctx, lessonID)
	if err != nil {
		return nil, err
	}
	return mapAttendanceRows(rows, lessonID), nil
}

func (s *Service) UpsertAttendance(ctx context.Context, actor Principal, input api.UpsertAttendanceJSONBody) (api.Attendance, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.Attendance{}, err
	}
	if !validAttendanceStatus(string(input.Status)) {
		return api.Attendance{}, apperr.Unprocessable("Неизвестный статус посещаемости")
	}
	var result api.Attendance
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		lesson, err := q.GetLesson(ctx, input.LessonId)
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := requireLessonWrite(ctx, q, actor, lesson.ClassID, lesson.SubjectID, lesson.TeacherID, lesson.AcademicPeriodID, lesson.LessonDate); err != nil {
			return err
		}
		if err := requireOpenPeriod(ctx, q, lesson.AcademicPeriodID, true); err != nil {
			return err
		}
		if err := requireStudentInClass(ctx, q, input.StudentId, lesson.ClassID, lesson.LessonDate); err != nil {
			return err
		}
		created, err := q.UpsertAttendance(ctx, db.UpsertAttendanceParams{LessonID: input.LessonId, StudentID: input.StudentId,
			Status: string(input.Status), Reason: nullableText(input.Reason), Comment: nullableText(input.Comment), CreatedBy: actor.UserID})
		if err != nil {
			return mapDatabaseError(err)
		}
		id := created.ID
		result = api.Attendance{Id: &id, LessonId: created.LessonID, StudentId: created.StudentID, Status: api.AttendanceStatus(created.Status),
			Reason: textPointer(created.Reason), Comment: textPointer(created.Comment), Version: int(created.Version)}
		newValues, _ := json.Marshal(map[string]any{"lesson_id": input.LessonId, "student_id": input.StudentId, "status": input.Status, "version": created.Version})
		if err := audit(ctx, q, actor, "attendance.upserted", "attendance", created.ID, nil, newValues); err != nil {
			return err
		}
		if created.Status == "absent" {
			return notifyStudent(ctx, q, input.StudentId, lesson.LessonDate, "attendance", "Отметка об отсутствии", "Учитель отметил отсутствие на уроке", "attendance", created.ID)
		}
		return nil
	})
	return result, err
}

func (s *Service) MarkAllPresent(ctx context.Context, actor Principal, lessonID uuid.UUID, idempotencyKey string) (BulkAttendanceResult, error) {
	if err := actor.requireTeacher(); err != nil {
		return BulkAttendanceResult{}, err
	}
	if len(idempotencyKey) < 8 || len(idempotencyKey) > 128 {
		return BulkAttendanceResult{}, apperr.BadRequest("Idempotency-Key должен содержать от 8 до 128 символов")
	}
	hash := sha256.Sum256([]byte(lessonID.String()))
	requestHash := hex.EncodeToString(hash[:])
	var result BulkAttendanceResult
	err := s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if _, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtext($1))", actor.UserID.String()+"|"+idempotencyKey+"|attendance.bulk"); err != nil {
			return err
		}
		stored, err := q.GetIdempotencyResponse(ctx, db.GetIdempotencyResponseParams{ActorID: actor.UserID, Key: idempotencyKey, Operation: "attendance.bulk"})
		if err == nil {
			if stored.RequestHash != requestHash {
				return apperr.Conflict("Этот Idempotency-Key уже использован для другого запроса")
			}
			if err := json.Unmarshal(stored.Response, &result); err != nil {
				return err
			}
			return nil
		}
		if !isNoRows(err) {
			return err
		}
		lesson, err := q.GetLesson(ctx, lessonID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := requireLessonWrite(ctx, q, actor, lesson.ClassID, lesson.SubjectID, lesson.TeacherID, lesson.AcademicPeriodID, lesson.LessonDate); err != nil {
			return err
		}
		if err := requireOpenPeriod(ctx, q, lesson.AcademicPeriodID, true); err != nil {
			return err
		}
		marked, err := q.MarkAllPresent(ctx, db.MarkAllPresentParams{LessonID: lessonID, CreatedBy: actor.UserID})
		if err != nil {
			return mapDatabaseError(err)
		}
		rows, err := q.ListAttendance(ctx, lessonID)
		if err != nil {
			return err
		}
		result = BulkAttendanceResult{CreatedOrUpdated: len(marked), Items: mapAttendanceRows(rows, lessonID)}
		response, _ := json.Marshal(result)
		if err := q.SaveIdempotencyResponse(ctx, db.SaveIdempotencyResponseParams{ActorID: actor.UserID, Key: idempotencyKey,
			Operation: "attendance.bulk", RequestHash: requestHash, Response: response}); err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"lesson_id": lessonID, "created_or_updated": len(marked)})
		return audit(ctx, q, actor, "attendance.marked_all_present", "lesson", lessonID, nil, newValues)
	})
	return result, err
}

func (s *Service) CleanupSessions(ctx context.Context) error {
	return s.queries.DeleteExpiredSessions(ctx)
}

func requireGradeChange(ctx context.Context, q *db.Queries, actor Principal, work db.GetLessonWorkRow, studentID uuid.UUID, value float32) error {
	if actor.TeacherID == nil || *actor.TeacherID != work.TeacherID {
		return apperr.NotFound()
	}
	if err := requireTeacherAssignment(ctx, q, actor, work.ClassID, work.SubjectID, work.AcademicPeriodID, work.LessonDate); err != nil {
		return err
	}
	if err := requireOpenPeriod(ctx, q, work.AcademicPeriodID, true); err != nil {
		return err
	}
	if err := requireStudentInClass(ctx, q, studentID, work.ClassID, work.LessonDate); err != nil {
		return err
	}
	return validateGradeValue(ctx, q, value)
}

func requireStudentInClass(ctx context.Context, q *db.Queries, studentID, classID uuid.UUID, onDate time.Time) error {
	allowed, err := q.StudentInClassOnDate(ctx, db.StudentInClassOnDateParams{StudentID: studentID, ClassID: classID, Column3: onDate})
	if err != nil {
		return err
	}
	if !allowed {
		return apperr.Unprocessable("Ученик не состоит в этом классе на дату урока")
	}
	return nil
}

func validateGradeValue(ctx context.Context, q *db.Queries, value float32) error {
	if math.IsNaN(float64(value)) || math.IsInf(float64(value), 0) {
		return apperr.Unprocessable("Некорректное значение оценки")
	}
	scale, err := q.GetGradeScale(ctx)
	if err != nil {
		return mapDatabaseError(err)
	}
	var allowed []float32
	if err := json.Unmarshal(scale.AllowedValues, &allowed); err != nil {
		return err
	}
	for _, candidate := range allowed {
		if math.Abs(float64(candidate-value)) < 0.0001 {
			return nil
		}
	}
	return apperr.Unprocessable("Оценка не соответствует активной шкале")
}

func gradeAndWork(ctx context.Context, q *db.Queries, gradeID uuid.UUID) (db.GetGradeRow, db.GetLessonWorkRow, error) {
	grade, err := q.GetGrade(ctx, gradeID)
	if err != nil {
		return db.GetGradeRow{}, db.GetLessonWorkRow{}, mapDatabaseError(err)
	}
	if grade.DeletedAt.Valid {
		return db.GetGradeRow{}, db.GetLessonWorkRow{}, apperr.NotFound()
	}
	work, err := q.GetLessonWork(ctx, grade.LessonWorkID)
	if err != nil {
		return db.GetGradeRow{}, db.GetLessonWorkRow{}, mapDatabaseError(err)
	}
	return grade, work, nil
}

func mapLessonWork(id, lessonID uuid.UUID, workType, title string, weight pgtype.Numeric, important bool, position int32) api.LessonWork {
	return api.LessonWork{Id: id, LessonId: lessonID, WorkType: api.LessonWorkWorkType(workType), Title: title,
		Weight: numericValue(weight), IsImportant: important, Position: int(position)}
}

func mapAttendanceRows(rows []db.ListAttendanceRow, lessonID uuid.UUID) []api.Attendance {
	result := make([]api.Attendance, 0, len(rows))
	for _, row := range rows {
		id, name := row.ID, row.StudentName
		result = append(result, api.Attendance{Id: &id, LessonId: lessonID, StudentId: row.StudentID, StudentName: &name,
			Status: api.AttendanceStatus(row.Status), Reason: textPointer(row.Reason), Comment: textPointer(row.Comment), Version: int(row.Version)})
	}
	return result
}

func readableStudentID(actor Principal, requested *uuid.UUID) (uuid.UUID, error) {
	if requested != nil {
		return *requested, nil
	}
	if actor.StudentID != nil {
		return *actor.StudentID, nil
	}
	return uuid.Nil, apperr.BadRequest("Укажите student_id")
}

func validWorkType(value string) bool {
	switch value {
	case "classwork", "homework", "test", "quiz", "project", "oral":
		return true
	}
	return false
}

func validAttendanceStatus(value string) bool {
	switch value {
	case "present", "absent", "late", "excused", "exempt":
		return true
	}
	return false
}

func notifyStudent(ctx context.Context, q *db.Queries, studentID uuid.UUID, onDate time.Time, kind, title, body, entityType string, entityID uuid.UUID) error {
	recipients, err := q.StudentNotificationRecipients(ctx, db.StudentNotificationRecipientsParams{ID: studentID, Column2: onDate})
	if err != nil {
		return err
	}
	for _, userID := range recipients {
		if err := q.CreateNotification(ctx, db.CreateNotificationParams{UserID: userID, Kind: kind, Title: title, Body: body,
			EntityType: pgtype.Text{String: entityType, Valid: true}, EntityID: nullableUUID(entityID)}); err != nil {
			return err
		}
	}
	return nil
}
