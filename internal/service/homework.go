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

func (s *Service) ListHomework(ctx context.Context, actor Principal, studentID *uuid.UUID) ([]api.Homework, error) {
	ids := make([]uuid.UUID, 0)
	completions := make(map[uuid.UUID]bool)
	if studentID != nil || actor.TeacherID == nil {
		id, err := readableStudentID(actor, studentID)
		if err != nil {
			return nil, err
		}
		if err := canReadStudent(ctx, s.queries, actor, id, time.Now()); err != nil {
			return nil, err
		}
		rows, err := s.queries.ListStudentHomework(ctx, id)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
			completed, _ := row.Completed.(bool)
			completions[row.ID] = completed
		}
	} else {
		if err := actor.requireTeacher(); err != nil {
			return nil, err
		}
		rows, err := s.queries.ListTeacherHomework(ctx, actor.UserID)
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
	}
	result := make([]api.Homework, 0, len(ids))
	for _, id := range ids {
		homework, err := s.homeworkByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if completed, ok := completions[id]; ok {
			homework.Completed = &completed
		}
		result = append(result, homework)
	}
	return result, nil
}

func (s *Service) CreateHomework(ctx context.Context, actor Principal, input api.CreateHomeworkJSONBody) (api.Homework, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.Homework{}, err
	}
	status := string(input.Status)
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 10000 || (status != "draft" && status != "published") {
		return api.Homework{}, apperr.Unprocessable("Проверьте текст и статус домашнего задания")
	}
	var homeworkID uuid.UUID
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := s.requireHomeworkAssignment(ctx, q, actor, input.ClassId, input.SubjectId, input.LessonId, input.DueDate.Time); err != nil {
			return err
		}
		created, err := q.CreateHomework(ctx, db.CreateHomeworkParams{ClassID: input.ClassId, SubjectID: input.SubjectId,
			LessonID: optionalUUID(input.LessonId), AuthorID: actor.UserID, Text: strings.TrimSpace(input.Text), DueDate: input.DueDate.Time, Status: status})
		if err != nil {
			return mapDatabaseError(err)
		}
		homeworkID = created.ID
		if input.StudentIds != nil {
			for _, studentID := range *input.StudentIds {
				if err := requireStudentInClass(ctx, q, studentID, input.ClassId, input.DueDate.Time); err != nil {
					return err
				}
				if err := q.AddHomeworkRecipient(ctx, db.AddHomeworkRecipientParams{HomeworkID: created.ID, StudentID: studentID}); err != nil {
					return mapDatabaseError(err)
				}
			}
		}
		newValues, _ := json.Marshal(map[string]any{"class_id": input.ClassId, "subject_id": input.SubjectId, "due_date": input.DueDate, "status": status})
		if err := audit(ctx, q, actor, "homework.created", "homework", created.ID, nil, newValues); err != nil {
			return err
		}
		if status == "published" {
			return notifyHomework(ctx, q, created.ID)
		}
		return nil
	})
	if err != nil {
		return api.Homework{}, err
	}
	return s.homeworkByID(ctx, homeworkID)
}

func (s *Service) UpdateHomework(ctx context.Context, actor Principal, homeworkID uuid.UUID, version int32, input api.UpdateHomeworkJSONBody) (api.Homework, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.Homework{}, err
	}
	if strings.TrimSpace(input.Text) == "" || len(input.Text) > 10000 {
		return api.Homework{}, apperr.Unprocessable("Проверьте текст домашнего задания")
	}
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		homework, err := q.GetHomework(ctx, homeworkID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := requireHomeworkOwner(ctx, q, actor, homework); err != nil {
			return err
		}
		if err := s.requireHomeworkAssignment(ctx, q, actor, homework.ClassID, homework.SubjectID, uuidPointer(homework.LessonID), input.DueDate.Time); err != nil {
			return err
		}
		if _, err := q.UpdateHomework(ctx, db.UpdateHomeworkParams{ID: homeworkID, Text: strings.TrimSpace(input.Text), DueDate: input.DueDate.Time, Version: version}); err != nil {
			if isNoRows(err) {
				return apperr.Conflict("Домашнее задание было изменено. Обновите данные")
			}
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(map[string]any{"text": homework.Text, "due_date": homework.DueDate, "version": homework.Version})
		newValues, _ := json.Marshal(map[string]any{"text": strings.TrimSpace(input.Text), "due_date": input.DueDate, "version": version + 1})
		return audit(ctx, q, actor, "homework.updated", "homework", homeworkID, oldValues, newValues)
	})
	if err != nil {
		return api.Homework{}, err
	}
	return s.homeworkByID(ctx, homeworkID)
}

func (s *Service) PublishHomework(ctx context.Context, actor Principal, homeworkID uuid.UUID, version int32) (api.Homework, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.Homework{}, err
	}
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		homework, err := q.GetHomework(ctx, homeworkID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := requireHomeworkOwner(ctx, q, actor, homework); err != nil {
			return err
		}
		if err := s.requireHomeworkAssignment(ctx, q, actor, homework.ClassID, homework.SubjectID, uuidPointer(homework.LessonID), homework.DueDate); err != nil {
			return err
		}
		if _, err := q.PublishHomework(ctx, db.PublishHomeworkParams{ID: homeworkID, Version: version}); err != nil {
			if isNoRows(err) {
				return apperr.Conflict("Задание уже опубликовано или было изменено")
			}
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"status": "published", "version": version + 1})
		if err := audit(ctx, q, actor, "homework.published", "homework", homeworkID, nil, newValues); err != nil {
			return err
		}
		return notifyHomework(ctx, q, homeworkID)
	})
	if err != nil {
		return api.Homework{}, err
	}
	return s.homeworkByID(ctx, homeworkID)
}

func (s *Service) SetHomeworkCompletion(ctx context.Context, actor Principal, homeworkID, studentID uuid.UUID, completed bool) (bool, error) {
	if actor.StudentID == nil || *actor.StudentID != studentID {
		return false, apperr.NotFound()
	}
	allowed, err := s.queries.StudentHasHomework(ctx, db.StudentHasHomeworkParams{ID: homeworkID, StudentID: studentID})
	if err != nil {
		return false, err
	}
	if !allowed {
		return false, apperr.NotFound()
	}
	err = s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if _, err := q.SetHomeworkCompletion(ctx, db.SetHomeworkCompletionParams{HomeworkID: homeworkID, StudentID: studentID, Column3: completed}); err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]bool{"completed": completed})
		return audit(ctx, q, actor, "homework.completion_changed", "homework", homeworkID, nil, newValues)
	})
	return completed, err
}

func (s *Service) homeworkByID(ctx context.Context, homeworkID uuid.UUID) (api.Homework, error) {
	row, err := s.queries.GetHomeworkView(ctx, homeworkID)
	if err != nil {
		return api.Homework{}, mapDatabaseError(err)
	}
	files, err := s.queries.ListHomeworkFiles(ctx, homeworkID)
	if err != nil {
		return api.Homework{}, err
	}
	mappedFiles := make([]api.File, 0, len(files))
	for _, file := range files {
		mappedFiles = append(mappedFiles, api.File{Id: file.ID, OriginalName: file.OriginalName, SizeBytes: file.SizeBytes, MimeType: file.MimeType})
	}
	className, subjectName := row.ClassName, row.SubjectName
	return api.Homework{Id: row.ID, ClassId: row.ClassID, ClassName: &className, SubjectId: row.SubjectID,
		SubjectName: &subjectName, LessonId: uuidPointer(row.LessonID), Text: row.Text, DueDate: apiDate(row.DueDate),
		Status: api.HomeworkStatus(row.Status), PublishedAt: timePointer(row.PublishedAt), Version: int(row.Version), Files: &mappedFiles}, nil
}

func (s *Service) requireHomeworkAssignment(ctx context.Context, q *db.Queries, actor Principal, classID, subjectID uuid.UUID, lessonID *uuid.UUID, dueDate time.Time) error {
	if lessonID != nil {
		lesson, err := q.GetLesson(ctx, *lessonID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if lesson.ClassID != classID || lesson.SubjectID != subjectID {
			return apperr.Unprocessable("Урок не соответствует классу и предмету задания")
		}
		if dueDate.Before(lesson.LessonDate) {
			return apperr.Unprocessable("Срок задания не может быть раньше урока")
		}
		if err := requireLessonWrite(ctx, q, actor, classID, subjectID, lesson.TeacherID, lesson.AcademicPeriodID, lesson.LessonDate); err != nil {
			return err
		}
		return requireOpenPeriod(ctx, q, lesson.AcademicPeriodID, true)
	}
	period, err := q.AcademicPeriodForDate(ctx, dueDate)
	if err != nil {
		return apperr.Unprocessable("Срок задания находится вне учебных периодов")
	}
	if err := requireTeacherAssignment(ctx, q, actor, classID, subjectID, period.ID, dueDate); err != nil {
		return err
	}
	return requireOpenPeriod(ctx, q, period.ID, true)
}

func requireHomeworkOwner(ctx context.Context, q *db.Queries, actor Principal, homework db.GetHomeworkRow) error {
	if homework.AuthorID != actor.UserID {
		return apperr.NotFound()
	}
	period, err := q.AcademicPeriodForDate(ctx, homework.DueDate)
	if err != nil {
		return mapDatabaseError(err)
	}
	if err := requireTeacherAssignment(ctx, q, actor, homework.ClassID, homework.SubjectID, period.ID, homework.DueDate); err != nil {
		return err
	}
	return requireOpenPeriod(ctx, q, period.ID, true)
}

func notifyHomework(ctx context.Context, q *db.Queries, homeworkID uuid.UUID) error {
	recipients, err := q.HomeworkNotificationRecipients(ctx, homeworkID)
	if err != nil {
		return err
	}
	for _, userID := range recipients {
		if err := q.CreateNotification(ctx, db.CreateNotificationParams{UserID: userID, Kind: "homework", Title: "Новое домашнее задание",
			Body: "Учитель опубликовал домашнее задание", EntityType: pgtype.Text{String: "homework", Valid: true}, EntityID: nullableUUID(homeworkID)}); err != nil {
			return err
		}
	}
	return nil
}
