package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func (s *Service) ListAcademicYears(ctx context.Context) ([]api.AcademicYear, error) {
	rows, err := s.queries.ListAcademicYears(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]api.AcademicYear, 0, len(rows))
	for _, row := range rows {
		current := row.IsCurrent
		result = append(result, api.AcademicYear{Id: row.ID, Name: row.Name, StartsOn: apiDate(row.StartsOn), EndsOn: apiDate(row.EndsOn), IsCurrent: &current})
	}
	return result, nil
}

func (s *Service) CreateAcademicYear(ctx context.Context, actor Principal, input api.AcademicYearInput) (api.AcademicYear, error) {
	if err := actor.requireSystemAdmin(); err != nil {
		return api.AcademicYear{}, err
	}
	if !input.EndsOn.After(input.StartsOn.Time) || strings.TrimSpace(input.Name) == "" {
		return api.AcademicYear{}, apperr.Unprocessable("Проверьте название и даты учебного года")
	}
	current := input.IsCurrent != nil && *input.IsCurrent
	var created db.CreateAcademicYearRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if current {
			if _, err := tx.Exec(ctx, "UPDATE academic_years SET is_current = false, updated_at = now() WHERE is_current"); err != nil {
				return err
			}
		}
		var err error
		created, err = q.CreateAcademicYear(ctx, db.CreateAcademicYearParams{Name: strings.TrimSpace(input.Name), StartsOn: input.StartsOn.Time, EndsOn: input.EndsOn.Time, IsCurrent: current})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"name": created.Name, "starts_on": created.StartsOn, "ends_on": created.EndsOn, "is_current": created.IsCurrent})
		return audit(ctx, q, actor, "academic_year.created", "academic_year", created.ID, nil, newValues)
	})
	if err != nil {
		return api.AcademicYear{}, err
	}
	return api.AcademicYear{Id: created.ID, Name: created.Name, StartsOn: apiDate(created.StartsOn), EndsOn: apiDate(created.EndsOn), IsCurrent: &created.IsCurrent}, nil
}

// ListAcademicPeriods возвращает периоды года или, без yearID, всех неархивных лет.
func (s *Service) ListAcademicPeriods(ctx context.Context, yearID *uuid.UUID) ([]api.AcademicPeriod, error) {
	var rows []db.ListAcademicPeriodsRow
	if yearID != nil {
		var err error
		if rows, err = s.queries.ListAcademicPeriods(ctx, *yearID); err != nil {
			return nil, mapDatabaseError(err)
		}
	} else {
		all, err := s.queries.ListAllAcademicPeriods(ctx)
		if err != nil {
			return nil, err
		}
		for _, row := range all {
			rows = append(rows, db.ListAcademicPeriodsRow(row))
		}
	}
	result := make([]api.AcademicPeriod, 0, len(rows))
	for _, row := range rows {
		result = append(result, api.AcademicPeriod{Id: row.ID, AcademicYearId: row.AcademicYearID, Name: row.Name,
			StartsOn: apiDate(row.StartsOn), EndsOn: apiDate(row.EndsOn), Status: api.AcademicPeriodStatus(row.Status), AllowLateEdits: row.AllowLateEdits})
	}
	return result, nil
}

func (s *Service) CreateAcademicPeriod(ctx context.Context, actor Principal, input api.CreateAcademicPeriodJSONBody) (api.AcademicPeriod, error) {
	if err := actor.requireSystemAdmin(); err != nil {
		return api.AcademicPeriod{}, err
	}
	if !input.EndsOn.After(input.StartsOn.Time) || strings.TrimSpace(input.Name) == "" {
		return api.AcademicPeriod{}, apperr.Unprocessable("Проверьте название и даты периода")
	}
	var created db.CreateAcademicPeriodRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		year, err := q.GetAcademicYear(ctx, input.AcademicYearId)
		if err != nil {
			return mapDatabaseError(err)
		}
		if input.StartsOn.Before(year.StartsOn) || input.EndsOn.After(year.EndsOn) {
			return apperr.Unprocessable("Период должен находиться внутри учебного года")
		}
		overlaps, err := q.AcademicPeriodOverlaps(ctx, db.AcademicPeriodOverlapsParams{AcademicYearID: input.AcademicYearId, Column2: input.StartsOn.Time, Column3: input.EndsOn.Time})
		if err != nil {
			return err
		}
		if overlaps {
			return apperr.Conflict("Учебные периоды не должны пересекаться")
		}
		created, err = q.CreateAcademicPeriod(ctx, db.CreateAcademicPeriodParams{AcademicYearID: input.AcademicYearId,
			Name: strings.TrimSpace(input.Name), StartsOn: input.StartsOn.Time, EndsOn: input.EndsOn.Time})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"name": created.Name, "academic_year_id": created.AcademicYearID})
		return audit(ctx, q, actor, "academic_period.created", "academic_period", created.ID, nil, newValues)
	})
	if err != nil {
		return api.AcademicPeriod{}, err
	}
	return api.AcademicPeriod{Id: created.ID, AcademicYearId: created.AcademicYearID, Name: created.Name,
		StartsOn: apiDate(created.StartsOn), EndsOn: apiDate(created.EndsOn), Status: api.AcademicPeriodStatus(created.Status), AllowLateEdits: created.AllowLateEdits}, nil
}

func (s *Service) CloseAcademicPeriod(ctx context.Context, actor Principal, periodID uuid.UUID) error {
	if err := actor.requireSystemAdmin(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		period, err := q.GetAcademicPeriod(ctx, periodID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if period.Status != "open" {
			return apperr.Conflict("Период уже закрыт")
		}
		if err := q.CloseAcademicPeriod(ctx, db.CloseAcademicPeriodParams{ID: periodID, ClosedBy: nullableUUID(actor.UserID)}); err != nil {
			return err
		}
		return audit(ctx, q, actor, "academic_period.closed", "academic_period", periodID, []byte(`{"status":"open"}`), []byte(`{"status":"closed"}`))
	})
}

func (s *Service) SetLateEdits(ctx context.Context, actor Principal, periodID uuid.UUID, allowed bool) error {
	if err := actor.requireSystemAdmin(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if _, err := q.GetAcademicPeriod(ctx, periodID); err != nil {
			return mapDatabaseError(err)
		}
		if err := q.SetLateEdits(ctx, db.SetLateEditsParams{ID: periodID, AllowLateEdits: allowed}); err != nil {
			return err
		}
		newValues, _ := json.Marshal(map[string]bool{"allow_late_edits": allowed})
		return audit(ctx, q, actor, "academic_period.late_edits", "academic_period", periodID, nil, newValues)
	})
}

func (s *Service) ListClasses(ctx context.Context) ([]api.NamedEntity, error) {
	rows, err := s.queries.ListClasses(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]api.NamedEntity, 0, len(rows))
	for _, row := range rows {
		yearID := row.AcademicYearID
		result = append(result, api.NamedEntity{Id: row.ID, Name: row.Name, AcademicYearId: &yearID})
	}
	return result, nil
}

func (s *Service) CreateClass(ctx context.Context, actor Principal, input api.CreateClassJSONBody) (api.NamedEntity, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.NamedEntity{}, err
	}
	var created db.CreateClassRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var err error
		created, err = q.CreateClass(ctx, db.CreateClassParams{AcademicYearID: input.AcademicYearId, Name: strings.TrimSpace(input.Name)})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"name": created.Name, "academic_year_id": created.AcademicYearID})
		return audit(ctx, q, actor, "class.created", "class", created.ID, nil, newValues)
	})
	if err != nil {
		return api.NamedEntity{}, err
	}
	return api.NamedEntity{Id: created.ID, Name: created.Name, AcademicYearId: &created.AcademicYearID}, nil
}

func (s *Service) ListSubjects(ctx context.Context) ([]api.NamedEntity, error) {
	rows, err := s.queries.ListSubjects(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]api.NamedEntity, 0, len(rows))
	for _, row := range rows {
		result = append(result, api.NamedEntity{Id: row.ID, Name: row.Name, ShortName: textPointer(row.ShortName)})
	}
	return result, nil
}

func (s *Service) CreateSubject(ctx context.Context, actor Principal, input api.CreateSubjectJSONBody) (api.NamedEntity, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.NamedEntity{}, err
	}
	var created db.CreateSubjectRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var err error
		created, err = q.CreateSubject(ctx, db.CreateSubjectParams{Name: strings.TrimSpace(input.Name), ShortName: nullableText(input.ShortName)})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]string{"name": created.Name})
		return audit(ctx, q, actor, "subject.created", "subject", created.ID, nil, newValues)
	})
	if err != nil {
		return api.NamedEntity{}, err
	}
	return api.NamedEntity{Id: created.ID, Name: created.Name, ShortName: textPointer(created.ShortName)}, nil
}

func (s *Service) ListRooms(ctx context.Context) ([]api.NamedEntity, error) {
	rows, err := s.queries.ListRooms(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]api.NamedEntity, 0, len(rows))
	for _, row := range rows {
		result = append(result, api.NamedEntity{Id: row.ID, Name: row.Name})
	}
	return result, nil
}

func (s *Service) CreateRoom(ctx context.Context, actor Principal, input api.CreateRoomJSONBody) (api.NamedEntity, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.NamedEntity{}, err
	}
	var created db.CreateRoomRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var err error
		created, err = q.CreateRoom(ctx, strings.TrimSpace(input.Name))
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]string{"name": created.Name})
		return audit(ctx, q, actor, "room.created", "room", created.ID, nil, newValues)
	})
	if err != nil {
		return api.NamedEntity{}, err
	}
	return api.NamedEntity{Id: created.ID, Name: created.Name}, nil
}

func (s *Service) CreateEnrollment(ctx context.Context, actor Principal, input api.CreateEnrollmentJSONBody) (api.Enrollment, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.Enrollment{}, err
	}
	endsOn := optionalDate(input.EndsOn)
	if endsOn.Valid && endsOn.Time.Before(input.StartsOn.Time) {
		return api.Enrollment{}, apperr.Unprocessable("Дата окончания раньше даты начала")
	}
	var created db.CreateEnrollmentRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := requireDateInClassYear(ctx, q, input.ClassId, input.StartsOn.Time); err != nil {
			return err
		}
		overlaps, err := q.EnrollmentOverlaps(ctx, db.EnrollmentOverlapsParams{StudentID: input.StudentId, Column2: input.StartsOn.Time, Column3: dateOrMax(endsOn)})
		if err != nil {
			return err
		}
		if overlaps {
			return apperr.Conflict("Зачисление пересекается с существующим интервалом")
		}
		created, err = q.CreateEnrollment(ctx, db.CreateEnrollmentParams{StudentID: input.StudentId, ClassID: input.ClassId, StartsOn: input.StartsOn.Time, EndsOn: endsOn})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"student_id": created.StudentID, "class_id": created.ClassID, "starts_on": created.StartsOn})
		return audit(ctx, q, actor, "enrollment.created", "enrollment", created.ID, nil, newValues)
	})
	if err != nil {
		return api.Enrollment{}, err
	}
	return api.Enrollment{Id: created.ID, StudentId: created.StudentID, ClassId: created.ClassID, StartsOn: apiDate(created.StartsOn), EndsOn: apiNullableDate(created.EndsOn)}, nil
}

// ListEnrollments возвращает зачисления (все или одного класса) с именами для экрана администратора.
func (s *Service) ListEnrollments(ctx context.Context, actor Principal, classID uuid.UUID) ([]api.Enrollment, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListEnrollments(ctx, classID)
	if err != nil {
		return nil, fmt.Errorf("list enrollments: %w", mapDatabaseError(err))
	}
	result := make([]api.Enrollment, 0, len(rows))
	for _, row := range rows {
		studentName, className := row.StudentName, row.ClassName
		result = append(result, api.Enrollment{Id: row.ID, StudentId: row.StudentID, StudentName: &studentName,
			ClassId: row.ClassID, ClassName: &className, StartsOn: apiDate(row.StartsOn), EndsOn: apiNullableDate(row.EndsOn)})
	}
	return result, nil
}

func (s *Service) ListClassStudents(ctx context.Context, actor Principal, classID uuid.UUID, onDate time.Time) ([]api.Student, error) {
	allowed := actor.IsAdministrator() || actor.HasRole("principal")
	if !allowed && actor.TeacherID != nil {
		var err error
		allowed, err = s.queries.TeacherHasClassOnDate(ctx, db.TeacherHasClassOnDateParams{UserID: actor.UserID, ClassID: classID, Column3: onDate})
		if err != nil {
			return nil, err
		}
		if !allowed {
			// Заменяющий отмечает посещаемость и ставит оценки классу, в котором обычно не работает.
			allowed, err = s.queries.TeacherSubstitutesInClass(ctx, db.TeacherSubstitutesInClassParams{TeacherID: *actor.TeacherID, ClassID: classID})
			if err != nil {
				return nil, err
			}
		}
	}
	if !allowed {
		return nil, apperr.NotFound()
	}
	rows, err := s.queries.ListClassStudentsOnDate(ctx, db.ListClassStudentsOnDateParams{ClassID: classID, Column2: onDate})
	if err != nil {
		return nil, err
	}
	result := make([]api.Student, 0, len(rows))
	for _, row := range rows {
		result = append(result, api.Student{Id: row.ID, DisplayName: row.DisplayName, StudentCode: textPointer(row.StudentCode)})
	}
	return result, nil
}

func (s *Service) CreateTeachingAssignment(ctx context.Context, actor Principal, input api.CreateTeachingAssignmentJSONBody) (api.TeachingAssignment, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.TeachingAssignment{}, err
	}
	endsOn := optionalDate(input.EndsOn)
	if endsOn.Valid && endsOn.Time.Before(input.StartsOn.Time) {
		return api.TeachingAssignment{}, apperr.Unprocessable("Некорректный интервал назначения")
	}
	var created db.CreateTeachingAssignmentRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		period, err := q.GetAcademicPeriod(ctx, input.AcademicPeriodId)
		if err != nil {
			return mapDatabaseError(err)
		}
		if input.StartsOn.Before(period.StartsOn) || (endsOn.Valid && endsOn.Time.After(period.EndsOn)) {
			return apperr.Unprocessable("Назначение должно находиться внутри учебного периода")
		}
		if err := requireClassInYear(ctx, q, input.ClassId, period.AcademicYearID); err != nil {
			return err
		}
		overlaps, err := q.TeachingAssignmentOverlaps(ctx, db.TeachingAssignmentOverlapsParams{
			ClassID: input.ClassId, SubjectID: input.SubjectId, AcademicPeriodID: input.AcademicPeriodId, Column4: input.StartsOn.Time, Column5: dateOrMax(endsOn)})
		if err != nil {
			return err
		}
		if overlaps {
			return apperr.Conflict("Назначение на этот класс и предмет уже существует")
		}
		created, err = q.CreateTeachingAssignment(ctx, db.CreateTeachingAssignmentParams{TeacherID: input.TeacherId,
			ClassID: input.ClassId, SubjectID: input.SubjectId, AcademicPeriodID: input.AcademicPeriodId, StartsOn: input.StartsOn.Time, EndsOn: endsOn})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"teacher_id": created.TeacherID, "class_id": created.ClassID, "subject_id": created.SubjectID})
		return audit(ctx, q, actor, "teaching_assignment.created", "teaching_assignment", created.ID, nil, newValues)
	})
	if err != nil {
		return api.TeachingAssignment{}, err
	}
	return api.TeachingAssignment{Id: created.ID, TeacherId: created.TeacherID, ClassId: created.ClassID, SubjectId: created.SubjectID,
		AcademicPeriodId: created.AcademicPeriodID, StartsOn: apiDate(created.StartsOn), EndsOn: apiNullableDate(created.EndsOn)}, nil
}

func (s *Service) ListTeachingAssignments(ctx context.Context, actor Principal) ([]api.TeachingAssignment, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListTeachingAssignments(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]api.TeachingAssignment, 0, len(rows))
	for _, row := range rows {
		teacherName, className, subjectName := row.TeacherName, row.ClassName, row.SubjectName
		result = append(result, api.TeachingAssignment{Id: row.ID, TeacherId: row.TeacherID, TeacherName: &teacherName, ClassId: row.ClassID, ClassName: &className,
			SubjectId: row.SubjectID, SubjectName: &subjectName, AcademicPeriodId: row.AcademicPeriodID, StartsOn: apiDate(row.StartsOn), EndsOn: apiNullableDate(row.EndsOn)})
	}
	return result, nil
}

func (s *Service) AssignClassTeacher(ctx context.Context, actor Principal, input api.AssignClassTeacherJSONBody) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	endsOn := optionalDate(input.EndsOn)
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		overlaps, err := q.ClassTeacherOverlaps(ctx, db.ClassTeacherOverlapsParams{ClassID: input.ClassId, Column2: input.StartsOn.Time, Column3: dateOrMax(endsOn)})
		if err != nil {
			return err
		}
		if overlaps {
			return apperr.Conflict("На этот период уже назначен классный руководитель")
		}
		if err := q.AssignClassTeacher(ctx, db.AssignClassTeacherParams{TeacherID: input.TeacherId, ClassID: input.ClassId, StartsOn: input.StartsOn.Time, EndsOn: endsOn}); err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"teacher_id": input.TeacherId, "class_id": input.ClassId})
		return audit(ctx, q, actor, "class_teacher.assigned", "class", input.ClassId, nil, newValues)
	})
}

func (s *Service) LinkParentStudent(ctx context.Context, actor Principal, input api.LinkParentStudentJSONBody) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := q.LinkParentStudent(ctx, db.LinkParentStudentParams{ParentID: input.ParentId, StudentID: input.StudentId,
			ValidFrom: input.ValidFrom.Time, ValidTo: optionalDate(input.ValidTo)}); err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"parent_id": input.ParentId, "student_id": input.StudentId})
		return audit(ctx, q, actor, "parent_student.linked", "student", input.StudentId, nil, newValues)
	})
}

func (s *Service) ListChildren(ctx context.Context, actor Principal) ([]api.Student, error) {
	if actor.ParentID == nil {
		return nil, apperr.Forbidden()
	}
	rows, err := s.queries.ListChildren(ctx, actor.UserID)
	if err != nil {
		return nil, err
	}
	result := make([]api.Student, 0, len(rows))
	for _, row := range rows {
		result = append(result, api.Student{Id: row.ID, DisplayName: row.DisplayName, StudentCode: textPointer(row.StudentCode)})
	}
	return result, nil
}

func optionalDate(value *openapi_types.Date) pgtype.Date {
	if value == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: value.Time, Valid: true}
}

func textPointer(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func isNoRows(err error) bool {
	return errors.Is(err, pgx.ErrNoRows)
}

// Классы относятся к учебному году (в новом году «6А» — это новый класс), а назначения, шаблон расписания и зачисления
// живут внутри года. Без этих проверок класс прошлого года можно было назначить на период нового или зачислить в него
// ученика с датой вне года — данные выглядели бы корректно, но не попадали бы ни в журнал, ни в расписание.

func classYear(ctx context.Context, q *db.Queries, classID uuid.UUID) (db.GetClassAcademicYearRow, error) {
	year, err := q.GetClassAcademicYear(ctx, classID)
	if errors.Is(err, pgx.ErrNoRows) {
		return year, apperr.Unprocessable("Класс не найден или находится в архиве")
	}
	return year, err
}

// requireClassInYear проверяет, что класс действует и относится к учебному году периода.
func requireClassInYear(ctx context.Context, q *db.Queries, classID, yearID uuid.UUID) error {
	year, err := classYear(ctx, q, classID)
	if err != nil {
		return err
	}
	if year.AcademicYearID != yearID {
		return apperr.Unprocessable("Класс относится к другому учебному году («" + year.YearName + "»), чем выбранный период")
	}
	return nil
}

// requireDateInClassYear — для дат, которые администратор выбрал сам (зачисление, перевод класса).
func requireDateInClassYear(ctx context.Context, q *db.Queries, classID uuid.UUID, day time.Time) error {
	year, err := classYear(ctx, q, classID)
	if err != nil {
		return err
	}
	if day.Before(year.StartsOn) || day.After(year.EndsOn) {
		return apperr.Unprocessable(fmt.Sprintf("Дата должна быть внутри учебного года класса «%s»: с %s по %s",
			year.YearName, year.StartsOn.Format("02.01.2006"), year.EndsOn.Format("02.01.2006")))
	}
	return nil
}

// enrollmentStartInClassYear — для дат по умолчанию (создание ученика, импорт, регистрация по приглашению): в класс
// следующего года ученика зачисляют с первого дня этого года, в класс завершённого года — нельзя.
func enrollmentStartInClassYear(ctx context.Context, q *db.Queries, classID uuid.UUID, day time.Time) (time.Time, error) {
	year, err := classYear(ctx, q, classID)
	if err != nil {
		return day, err
	}
	if day.After(year.EndsOn) {
		return day, apperr.Unprocessable("Учебный год «" + year.YearName + "» уже закончился: выберите класс текущего года")
	}
	if day.Before(year.StartsOn) {
		return year.StartsOn, nil
	}
	return day, nil
}
