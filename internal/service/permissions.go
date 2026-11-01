package service

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

type Principal struct {
	UserID             uuid.UUID
	SessionID          uuid.UUID
	Login              string
	DisplayName        string
	Roles              []string
	StudentID          *uuid.UUID
	TeacherID          *uuid.UUID
	ParentID           *uuid.UUID
	CSRFHash           []byte
	CSRFToken          string
	RequestID          string
	MustChangePassword bool
}

func (p Principal) HasRole(role string) bool {
	for _, candidate := range p.Roles {
		if candidate == role {
			return true
		}
	}
	return false
}

// Коды ролей, которые используются в проверках прав.
const (
	roleSystemAdmin   = "system_admin"
	roleAdministrator = "administrator"
)

// IsSystemAdmin сообщает, что у пользователя есть полный доступ к системе.
func (p Principal) IsSystemAdmin() bool { return p.HasRole(roleSystemAdmin) }

// IsAdministrator включает и системного администратора: он может всё, что может администратор.
func (p Principal) IsAdministrator() bool {
	return p.HasRole(roleAdministrator) || p.HasRole(roleSystemAdmin)
}

func (p Principal) requireAdministrator() error {
	if !p.IsAdministrator() {
		return apperr.Forbidden()
	}
	return nil
}

func (p Principal) requireSystemAdmin() error {
	if !p.IsSystemAdmin() {
		return apperr.Forbidden()
	}
	return nil
}

// isPrivilegedRole — роли, которые обычный администратор не вправе выдавать, отзывать или менять у других.
func isPrivilegedRole(code string) bool { return code == roleSystemAdmin || code == roleAdministrator }

func (p Principal) requireTeacher() error {
	if p.TeacherID == nil || !p.HasRole("teacher") {
		return apperr.Forbidden()
	}
	return nil
}

func canReadStudent(ctx context.Context, q *db.Queries, actor Principal, studentID uuid.UUID, onDate time.Time) error {
	if actor.StudentID != nil && *actor.StudentID == studentID {
		return nil
	}
	if actor.ParentID != nil {
		allowed, err := q.ParentHasStudent(ctx, db.ParentHasStudentParams{UserID: actor.UserID, StudentID: studentID, Column3: onDate})
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}
	}
	return apperr.NotFound()
}

func requireClassReportAccess(ctx context.Context, q *db.Queries, actor Principal, classID uuid.UUID, onDate time.Time) error {
	if actor.HasRole("principal") {
		return nil
	}
	if actor.HasRole("class_teacher") {
		allowed, err := q.ClassTeacherHasClass(ctx, db.ClassTeacherHasClassParams{UserID: actor.UserID, ClassID: classID, Column3: onDate})
		if err != nil {
			return err
		}
		if allowed {
			return nil
		}
	}
	return apperr.NotFound()
}

func requireTeacherAssignment(ctx context.Context, q *db.Queries, actor Principal, classID, subjectID, periodID uuid.UUID, onDate time.Time) error {
	if err := actor.requireTeacher(); err != nil {
		return err
	}
	allowed, err := q.TeacherHasAssignment(ctx, db.TeacherHasAssignmentParams{
		UserID: actor.UserID, ClassID: classID, SubjectID: subjectID, AcademicPeriodID: periodID, Column5: onDate,
	})
	if err != nil {
		return err
	}
	if !allowed {
		return apperr.NotFound()
	}
	return nil
}

func requireOpenPeriod(ctx context.Context, q *db.Queries, periodID uuid.UUID, allowLate bool) error {
	period, err := q.GetAcademicPeriod(ctx, periodID)
	if err != nil {
		return mapDatabaseError(err)
	}
	if period.Status == "closed" && !(allowLate && period.AllowLateEdits) {
		return apperr.Conflict("Учебный период закрыт для изменений")
	}
	return nil
}

func nullableUUID(id uuid.UUID) pgtype.UUID {
	return pgtype.UUID{Bytes: id, Valid: true}
}
