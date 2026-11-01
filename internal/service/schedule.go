package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

func (s *Service) GetSchedule(ctx context.Context, actor Principal, from, to time.Time, studentID *uuid.UUID) ([]api.Lesson, error) {
	if from.IsZero() || to.Before(from) || to.Sub(from) > 92*24*time.Hour {
		return nil, apperr.Unprocessable("Проверьте период расписания (не более 92 дней)")
	}
	ids := make([]uuid.UUID, 0)
	if studentID != nil {
		if err := canReadStudent(ctx, s.queries, actor, *studentID, from); err != nil {
			return nil, err
		}
		rows, err := s.queries.ListStudentLessons(ctx, db.ListStudentLessonsParams{StudentID: *studentID, Column2: from, Column3: to})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
	} else if actor.TeacherID != nil && actor.HasRole("teacher") {
		rows, err := s.queries.ListTeacherLessons(ctx, db.ListTeacherLessonsParams{TeacherID: *actor.TeacherID, Column2: from, Column3: to})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
	} else if actor.StudentID != nil {
		rows, err := s.queries.ListStudentLessons(ctx, db.ListStudentLessonsParams{StudentID: *actor.StudentID, Column2: from, Column3: to})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
	} else {
		return nil, apperr.Forbidden()
	}
	return s.lessonsByIDs(ctx, ids)
}

// lessonsByIDs загружает уроки одним запросом; порядок — по времени начала.
func (s *Service) lessonsByIDs(ctx context.Context, ids []uuid.UUID) ([]api.Lesson, error) {
	result := make([]api.Lesson, 0, len(ids))
	if len(ids) == 0 {
		return result, nil
	}
	rows, err := s.queries.ListLessonViews(ctx, ids)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		result = append(result, mapLesson(db.GetLessonViewRow(row)))
	}
	return result, nil
}

func (s *Service) GetLesson(ctx context.Context, actor Principal, lessonID uuid.UUID) (api.Lesson, error) {
	lesson, err := s.queries.GetLessonView(ctx, lessonID)
	if err != nil {
		return api.Lesson{}, mapDatabaseError(err)
	}
	if err := requireLessonRead(ctx, s.queries, actor, lesson); err != nil {
		return api.Lesson{}, err
	}
	return mapLesson(lesson), nil
}

func (s *Service) CreateLesson(ctx context.Context, actor Principal, input api.CreateLessonJSONBody) (api.Lesson, error) {
	if !actor.IsAdministrator() {
		if err := actor.requireTeacher(); err != nil {
			return api.Lesson{}, err
		}
		if *actor.TeacherID != input.TeacherId {
			return api.Lesson{}, apperr.NotFound()
		}
	}
	if err := validateLessonTimes(input.LessonDate.Time, input.StartsAt, input.EndsAt); err != nil {
		return api.Lesson{}, err
	}
	topic := ""
	if input.Topic != nil {
		topic = strings.TrimSpace(*input.Topic)
	}
	if len(topic) > 1000 {
		return api.Lesson{}, apperr.Unprocessable("Тема урока слишком длинная")
	}
	var lessonID uuid.UUID
	err := s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := lockLessonSchedule(ctx, tx); err != nil {
			return err
		}
		period, err := q.GetAcademicPeriod(ctx, input.AcademicPeriodId)
		if err != nil {
			return mapDatabaseError(err)
		}
		if input.LessonDate.Before(period.StartsOn) || input.LessonDate.After(period.EndsOn) {
			return apperr.Unprocessable("Дата урока находится вне учебного периода")
		}
		allowed, err := q.TeacherAssignmentExists(ctx, db.TeacherAssignmentExistsParams{TeacherID: input.TeacherId,
			ClassID: input.ClassId, SubjectID: input.SubjectId, AcademicPeriodID: input.AcademicPeriodId, Column5: input.LessonDate.Time})
		if err != nil {
			return err
		}
		if !allowed {
			return apperr.Unprocessable("Учитель не назначен на этот класс, предмет и дату")
		}
		if err := checkLessonConflict(ctx, q, uuid.Nil, input.StartsAt, input.EndsAt, input.ClassId, input.TeacherId, input.RoomId); err != nil {
			return err
		}
		created, err := q.CreateLesson(ctx, db.CreateLessonParams{LessonDate: input.LessonDate.Time, StartsAt: input.StartsAt, EndsAt: input.EndsAt,
			ClassID: input.ClassId, SubjectID: input.SubjectId, TeacherID: input.TeacherId, RoomID: optionalUUID(input.RoomId),
			AcademicPeriodID: input.AcademicPeriodId, Topic: topic, CreatedBy: nullableUUID(actor.UserID)})
		if err != nil {
			return mapDatabaseError(err)
		}
		lessonID = created.ID
		newValues, _ := json.Marshal(map[string]any{"lesson_date": created.LessonDate, "class_id": created.ClassID, "subject_id": created.SubjectID, "teacher_id": created.TeacherID})
		return audit(ctx, q, actor, "lesson.created", "lesson", created.ID, nil, newValues)
	})
	if err != nil {
		return api.Lesson{}, err
	}
	return s.lessonByID(ctx, s.queries, lessonID)
}

func (s *Service) UpdateLesson(ctx context.Context, actor Principal, lessonID uuid.UUID, version int32, input api.UpdateLessonJSONBody) (api.Lesson, error) {
	if !actor.IsAdministrator() && actor.TeacherID == nil {
		return api.Lesson{}, apperr.Forbidden()
	}
	status := string(input.Status)
	if status != "planned" && status != "completed" && status != "cancelled" {
		return api.Lesson{}, apperr.Unprocessable("Неизвестный статус урока")
	}
	if status == "cancelled" && (input.CancellationReason == nil || strings.TrimSpace(*input.CancellationReason) == "") {
		return api.Lesson{}, apperr.Unprocessable("Укажите причину отмены урока")
	}
	if len(input.Topic) > 1000 {
		return api.Lesson{}, apperr.Unprocessable("Тема урока слишком длинная")
	}
	err := s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := lockLessonSchedule(ctx, tx); err != nil {
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
		lessonDate, startsAt, endsAt, teacherID := lesson.LessonDate, lesson.StartsAt, lesson.EndsAt, lesson.TeacherID
		if input.LessonDate != nil {
			lessonDate = input.LessonDate.Time
		}
		if input.StartsAt != nil {
			startsAt = *input.StartsAt
		}
		if input.EndsAt != nil {
			endsAt = *input.EndsAt
		}
		if input.TeacherId != nil {
			teacherID = *input.TeacherId
		}
		if err := validateLessonTimes(lessonDate, startsAt, endsAt); err != nil {
			return err
		}
		period, err := q.GetAcademicPeriod(ctx, lesson.AcademicPeriodID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if lessonDate.Before(period.StartsOn) || lessonDate.After(period.EndsOn) {
			return apperr.Unprocessable("Новая дата урока находится вне учебного периода")
		}
		assigned, err := q.TeacherAssignmentExists(ctx, db.TeacherAssignmentExistsParams{TeacherID: teacherID, ClassID: lesson.ClassID, SubjectID: lesson.SubjectID, AcademicPeriodID: lesson.AcademicPeriodID, Column5: lessonDate})
		if err != nil {
			return err
		}
		if !assigned {
			return apperr.Unprocessable("Новый учитель не назначен на класс, предмет и дату урока")
		}
		if status != "cancelled" {
			if err := checkLessonConflict(ctx, q, lessonID, startsAt, endsAt, lesson.ClassID, teacherID, input.RoomId); err != nil {
				return err
			}
		}
		reason := nullableText(input.CancellationReason)
		if status != "cancelled" {
			reason = pgtype.Text{}
		}
		if _, err := q.RescheduleLesson(ctx, db.RescheduleLessonParams{ID: lessonID, LessonDate: lessonDate, StartsAt: startsAt, EndsAt: endsAt, TeacherID: teacherID, Topic: strings.TrimSpace(input.Topic), RoomID: optionalUUID(input.RoomId),
			Status: status, CancellationReason: reason, UpdatedBy: nullableUUID(actor.UserID), Version: version}); err != nil {
			if isNoRows(err) {
				return apperr.Conflict("Урок был изменён. Обновите данные и повторите операцию")
			}
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(map[string]any{"topic": lesson.Topic, "status": lesson.Status, "lesson_date": lesson.LessonDate, "starts_at": lesson.StartsAt, "ends_at": lesson.EndsAt, "teacher_id": lesson.TeacherID, "version": lesson.Version})
		newValues, _ := json.Marshal(map[string]any{"topic": strings.TrimSpace(input.Topic), "status": status, "lesson_date": lessonDate, "starts_at": startsAt, "ends_at": endsAt, "teacher_id": teacherID, "version": version + 1})
		return audit(ctx, q, actor, "lesson.updated", "lesson", lessonID, oldValues, newValues)
	})
	if err != nil {
		return api.Lesson{}, err
	}
	return s.lessonByID(ctx, s.queries, lessonID)
}

func (s *Service) lessonByID(ctx context.Context, q *db.Queries, lessonID uuid.UUID) (api.Lesson, error) {
	lesson, err := q.GetLessonView(ctx, lessonID)
	if err != nil {
		return api.Lesson{}, mapDatabaseError(err)
	}
	return mapLesson(lesson), nil
}

func mapLesson(row db.GetLessonViewRow) api.Lesson {
	className, subjectName := row.ClassName, row.SubjectName
	return api.Lesson{Id: row.ID, LessonDate: apiDate(row.LessonDate), StartsAt: row.StartsAt, EndsAt: row.EndsAt,
		ClassId: row.ClassID, ClassName: &className, SubjectId: row.SubjectID, SubjectName: &subjectName,
		TeacherId: row.TeacherID, RoomId: uuidPointer(row.RoomID), RoomName: textPointer(row.RoomName),
		AcademicPeriodId: row.AcademicPeriodID, Topic: row.Topic, Status: api.LessonStatus(row.Status),
		CancellationReason: textPointer(row.CancellationReason), Version: int(row.Version)}
}

func validateLessonTimes(date time.Time, startsAt, endsAt time.Time) error {
	if !endsAt.After(startsAt) || startsAt.Format("2006-01-02") != date.Format("2006-01-02") || endsAt.Format("2006-01-02") != date.Format("2006-01-02") {
		return apperr.Unprocessable("Время начала и окончания должно относиться к дате урока")
	}
	if endsAt.Sub(startsAt) > 4*time.Hour {
		return apperr.Unprocessable("Продолжительность урока не может превышать 4 часа")
	}
	return nil
}

func lockLessonSchedule(ctx context.Context, tx pgx.Tx) error {
	_, err := tx.Exec(ctx, "SELECT pg_advisory_xact_lock(731905240)")
	return err
}

func checkLessonConflict(ctx context.Context, q *db.Queries, lessonID uuid.UUID, startsAt, endsAt time.Time, classID, teacherID uuid.UUID, roomID *uuid.UUID) error {
	room := uuid.Nil
	if roomID != nil {
		room = *roomID
	}
	conflict, err := q.LessonTimeConflict(ctx, db.LessonTimeConflictParams{ID: lessonID, Column2: startsAt, Column3: endsAt,
		ClassID: classID, TeacherID: teacherID, Column6: room})
	if err != nil {
		return err
	}
	if conflict {
		return apperr.Conflict("Класс, учитель или кабинет уже заняты в это время")
	}
	return nil
}

func requireLessonWrite(ctx context.Context, q *db.Queries, actor Principal, classID, subjectID, teacherID, periodID uuid.UUID, onDate time.Time) error {
	if actor.IsAdministrator() {
		return nil
	}
	if actor.TeacherID == nil || *actor.TeacherID != teacherID {
		return apperr.NotFound()
	}
	return requireTeacherAssignment(ctx, q, actor, classID, subjectID, periodID, onDate)
}

func requireLessonRead(ctx context.Context, q *db.Queries, actor Principal, lesson db.GetLessonViewRow) error {
	if actor.IsAdministrator() {
		return nil
	}
	if actor.TeacherID != nil && *actor.TeacherID == lesson.TeacherID {
		return requireTeacherAssignment(ctx, q, actor, lesson.ClassID, lesson.SubjectID, lesson.AcademicPeriodID, lesson.LessonDate)
	}
	if actor.HasRole("class_teacher") {
		allowed, err := q.ClassTeacherHasClass(ctx, db.ClassTeacherHasClassParams{UserID: actor.UserID, ClassID: lesson.ClassID, Column3: lesson.LessonDate})
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}
	}
	if actor.StudentID != nil {
		allowed, err := q.StudentInClassOnDate(ctx, db.StudentInClassOnDateParams{StudentID: *actor.StudentID, ClassID: lesson.ClassID, Column3: lesson.LessonDate})
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}
	}
	if actor.ParentID != nil {
		rows, err := q.ListClassStudentsOnDate(ctx, db.ListClassStudentsOnDateParams{ClassID: lesson.ClassID, Column2: lesson.LessonDate})
		if err != nil {
			return err
		}
		for _, student := range rows {
			if canReadStudent(ctx, q, actor, student.ID, lesson.LessonDate) == nil {
				return nil
			}
		}
	}
	return apperr.NotFound()
}
