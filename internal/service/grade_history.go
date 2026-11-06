package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
)

// gradeAuditValues — подмножество полей audit_logs.old_values/new_values, которое безопасно показывать учителю.
type gradeAuditValues struct {
	Value   *float32 `json:"value"`
	Comment *string  `json:"comment"`
}

// GradeHistory возвращает историю изменений оценки только для чтения.
// Учитель получает её лишь для оценок своего назначения (иначе 404, чтобы не раскрывать существование оценки),
// руководитель и администратор — для любой. Остальные роли получают 403 до обращения к БД, поэтому по ответу
// нельзя определить, существует ли оценка. История не даёт права изменять оценку.
func (s *Service) GradeHistory(ctx context.Context, actor Principal, gradeID uuid.UUID) ([]api.GradeHistoryEntry, error) {
	privileged := actor.HasRole("principal") || actor.IsAdministrator()
	if !privileged {
		if err := actor.requireTeacher(); err != nil {
			return nil, err
		}
	}
	grade, work, err := gradeAndWork(ctx, s.queries, gradeID)
	if err != nil {
		return nil, err
	}
	if !privileged {
		if *actor.TeacherID != work.TeacherID {
			return nil, apperr.NotFound()
		}
		if err := requireLessonTeacher(ctx, s.queries, actor, work.LessonID, work.ClassID, work.SubjectID, work.AcademicPeriodID, work.LessonDate); err != nil {
			return nil, err
		}
	}
	rows, err := s.queries.ListGradeHistory(ctx, grade.ID)
	if err != nil {
		return nil, fmt.Errorf("list grade history: %w", mapDatabaseError(err))
	}
	result := make([]api.GradeHistoryEntry, 0, len(rows))
	for _, row := range rows {
		entry := api.GradeHistoryEntry{Id: row.ID, Action: row.Action, ActorName: textPointer(row.ActorName), CreatedAt: row.CreatedAt}
		oldValues, newValues := decodeGradeAudit(ctx, row.ID, row.OldValues), decodeGradeAudit(ctx, row.ID, row.NewValues)
		entry.OldValue, entry.OldComment = oldValues.Value, oldValues.Comment
		entry.NewValue, entry.NewComment = newValues.Value, newValues.Comment
		result = append(result, entry)
	}
	return result, nil
}

// decodeGradeAudit разбирает значения аудита. Запись без значений (создание не имеет old_values) — штатный случай;
// повреждённый JSON не должен скрывать остальную историю, поэтому он логируется и даёт пустые поля.
func decodeGradeAudit(ctx context.Context, auditID uuid.UUID, raw []byte) gradeAuditValues {
	var values gradeAuditValues
	if len(raw) == 0 {
		return values
	}
	if err := json.Unmarshal(raw, &values); err != nil {
		slog.WarnContext(ctx, "grade audit values are not valid JSON", "audit_id", auditID, "error", err)
		return gradeAuditValues{}
	}
	return values
}
