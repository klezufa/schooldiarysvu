package service

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

func (s *Service) UpdateAcademicYear(ctx context.Context, actor Principal, id uuid.UUID, input api.AcademicYearInput) (api.AcademicYear, error) {
	if err := actor.requireSystemAdmin(); err != nil {
		return api.AcademicYear{}, err
	}
	if strings.TrimSpace(input.Name) == "" || !input.EndsOn.After(input.StartsOn.Time) {
		return api.AcademicYear{}, apperr.Unprocessable("Проверьте название и даты учебного года")
	}
	current := input.IsCurrent != nil && *input.IsCurrent
	var row db.UpdateAcademicYearRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		old, err := q.GetAcademicYear(ctx, id)
		if err != nil {
			return mapDatabaseError(err)
		}
		if current {
			if err := q.ClearCurrentAcademicYear(ctx, id); err != nil {
				return err
			}
		}
		row, err = q.UpdateAcademicYear(ctx, db.UpdateAcademicYearParams{ID: id, Name: strings.TrimSpace(input.Name), StartsOn: input.StartsOn.Time, EndsOn: input.EndsOn.Time, IsCurrent: current})
		if err != nil {
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(old)
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "academic_year.updated", "academic_year", id, oldValues, newValues)
	})
	if err != nil {
		return api.AcademicYear{}, err
	}
	return api.AcademicYear{Id: row.ID, Name: row.Name, StartsOn: apiDate(row.StartsOn), EndsOn: apiDate(row.EndsOn), IsCurrent: &row.IsCurrent}, nil
}

func (s *Service) ArchiveAcademicYear(ctx context.Context, actor Principal, id uuid.UUID) error {
	if err := actor.requireSystemAdmin(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		affected, err := q.ArchiveAcademicYear(ctx, id)
		if err != nil {
			return err
		}
		if affected == 0 {
			if _, err := q.GetAcademicYear(ctx, id); err != nil {
				return mapDatabaseError(err)
			}
			return apperr.Conflict("Сначала архивируйте классы учебного года")
		}
		return audit(ctx, q, actor, "academic_year.archived", "academic_year", id, nil, []byte(`{"archived":true}`))
	})
}

func (s *Service) UpdateAcademicPeriod(ctx context.Context, actor Principal, id uuid.UUID, input api.UpdateAcademicPeriodJSONBody) (api.AcademicPeriod, error) {
	if err := actor.requireSystemAdmin(); err != nil {
		return api.AcademicPeriod{}, err
	}
	if strings.TrimSpace(input.Name) == "" || !input.EndsOn.After(input.StartsOn.Time) {
		return api.AcademicPeriod{}, apperr.Unprocessable("Проверьте название и даты периода")
	}
	var row db.UpdateAcademicPeriodRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		old, err := q.GetAcademicPeriod(ctx, id)
		if err != nil {
			return mapDatabaseError(err)
		}
		year, err := q.GetAcademicYear(ctx, old.AcademicYearID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if input.StartsOn.Before(year.StartsOn) || input.EndsOn.After(year.EndsOn) {
			return apperr.Unprocessable("Период должен находиться внутри учебного года")
		}
		row, err = q.UpdateAcademicPeriod(ctx, db.UpdateAcademicPeriodParams{ID: id, Name: strings.TrimSpace(input.Name), StartsOn: input.StartsOn.Time, EndsOn: input.EndsOn.Time})
		if err != nil {
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(old)
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "academic_period.updated", "academic_period", id, oldValues, newValues)
	})
	if err != nil {
		return api.AcademicPeriod{}, err
	}
	return mapAcademicPeriod(row.ID, row.AcademicYearID, row.Name, row.StartsOn, row.EndsOn, row.Status, row.AllowLateEdits), nil
}

func mapAcademicPeriod(id, yearID uuid.UUID, name string, startsOn, endsOn time.Time, status string, late bool) api.AcademicPeriod {
	return api.AcademicPeriod{Id: id, AcademicYearId: yearID, Name: name, StartsOn: apiDate(startsOn), EndsOn: apiDate(endsOn), Status: api.AcademicPeriodStatus(status), AllowLateEdits: late}
}

func (s *Service) UpdateClass(ctx context.Context, actor Principal, id uuid.UUID, input api.UpdateClassJSONBody) (api.NamedEntity, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.NamedEntity{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return api.NamedEntity{}, apperr.Unprocessable("Введите название класса")
	}
	var row db.UpdateClassRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var err error
		row, err = q.UpdateClass(ctx, db.UpdateClassParams{ID: id, Name: strings.TrimSpace(input.Name)})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "class.updated", "class", id, nil, newValues)
	})
	return api.NamedEntity{Id: row.ID, Name: row.Name, AcademicYearId: &row.AcademicYearID}, err
}

func (s *Service) UpdateSubject(ctx context.Context, actor Principal, id uuid.UUID, input api.UpdateSubjectJSONBody) (api.NamedEntity, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.NamedEntity{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return api.NamedEntity{}, apperr.Unprocessable("Введите название предмета")
	}
	var row db.UpdateSubjectRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var err error
		row, err = q.UpdateSubject(ctx, db.UpdateSubjectParams{ID: id, Name: strings.TrimSpace(input.Name), ShortName: nullableText(input.ShortName)})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "subject.updated", "subject", id, nil, newValues)
	})
	return api.NamedEntity{Id: row.ID, Name: row.Name, ShortName: textPointer(row.ShortName)}, err
}

func (s *Service) UpdateRoom(ctx context.Context, actor Principal, id uuid.UUID, input api.UpdateRoomJSONBody) (api.NamedEntity, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.NamedEntity{}, err
	}
	if strings.TrimSpace(input.Name) == "" {
		return api.NamedEntity{}, apperr.Unprocessable("Введите название кабинета")
	}
	var row db.UpdateRoomRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var err error
		row, err = q.UpdateRoom(ctx, db.UpdateRoomParams{ID: id, Name: strings.TrimSpace(input.Name)})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "room.updated", "room", id, nil, newValues)
	})
	return api.NamedEntity{Id: row.ID, Name: row.Name}, err
}

func (s *Service) ArchiveEntity(ctx context.Context, actor Principal, kind string, id uuid.UUID) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var affected int64
		var err error
		switch kind {
		case "class":
			affected, err = q.ArchiveClass(ctx, id)
		case "subject":
			affected, err = q.ArchiveSubject(ctx, id)
		case "room":
			affected, err = q.ArchiveRoom(ctx, id)
		default:
			return apperr.BadRequest("Неизвестный тип объекта")
		}
		if err != nil {
			return err
		}
		if affected == 0 {
			return apperr.NotFound()
		}
		return audit(ctx, q, actor, kind+".archived", kind, id, nil, []byte(`{"archived":true}`))
	})
}

func (s *Service) UpdateEnrollment(ctx context.Context, actor Principal, id uuid.UUID, input api.UpdateEnrollmentJSONBody) (api.Enrollment, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.Enrollment{}, err
	}
	var row db.UpdateEnrollmentEndRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		old, err := q.GetEnrollment(ctx, id)
		if err != nil {
			return mapDatabaseError(err)
		}
		endsOn := optionalDate(input.EndsOn)
		if endsOn.Valid && endsOn.Time.Before(old.StartsOn) {
			return apperr.Unprocessable("Дата окончания раньше даты начала")
		}
		overlaps, err := q.EnrollmentOverlapsExcept(ctx, db.EnrollmentOverlapsExceptParams{ID: id, StudentID: old.StudentID, Column3: old.StartsOn, Column4: dateOrMax(endsOn)})
		if err != nil {
			return err
		}
		if overlaps {
			return apperr.Conflict("Зачисление пересекается с существующим интервалом")
		}
		row, err = q.UpdateEnrollmentEnd(ctx, db.UpdateEnrollmentEndParams{ID: id, EndsOn: endsOn})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "enrollment.updated", "enrollment", id, nil, newValues)
	})
	if err != nil {
		return api.Enrollment{}, err
	}
	return api.Enrollment{Id: row.ID, StudentId: row.StudentID, ClassId: row.ClassID, StartsOn: apiDate(row.StartsOn), EndsOn: apiNullableDate(row.EndsOn)}, nil
}

func (s *Service) UpdateTeachingAssignment(ctx context.Context, actor Principal, id uuid.UUID, input api.UpdateTeachingAssignmentJSONBody) (api.TeachingAssignment, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.TeachingAssignment{}, err
	}
	var row db.UpdateTeachingAssignmentEndRow
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		old, err := q.GetTeachingAssignment(ctx, id)
		if err != nil {
			return mapDatabaseError(err)
		}
		endsOn := optionalDate(input.EndsOn)
		if endsOn.Valid && endsOn.Time.Before(old.StartsOn) {
			return apperr.Unprocessable("Дата окончания раньше даты начала")
		}
		overlaps, err := q.TeachingAssignmentOverlapsExcept(ctx, db.TeachingAssignmentOverlapsExceptParams{ID: id, ClassID: old.ClassID, SubjectID: old.SubjectID, AcademicPeriodID: old.AcademicPeriodID, Column5: old.StartsOn, Column6: dateOrMax(endsOn)})
		if err != nil {
			return err
		}
		if overlaps {
			return apperr.Conflict("Назначение пересекается с существующим интервалом")
		}
		row, err = q.UpdateTeachingAssignmentEnd(ctx, db.UpdateTeachingAssignmentEndParams{ID: id, EndsOn: endsOn})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "teaching_assignment.updated", "teaching_assignment", id, nil, newValues)
	})
	if err != nil {
		return api.TeachingAssignment{}, err
	}
	return api.TeachingAssignment{Id: row.ID, TeacherId: row.TeacherID, ClassId: row.ClassID, SubjectId: row.SubjectID, AcademicPeriodId: row.AcademicPeriodID, StartsOn: apiDate(row.StartsOn), EndsOn: apiNullableDate(row.EndsOn)}, nil
}

func (s *Service) UpdateClassTeacher(ctx context.Context, actor Principal, id uuid.UUID, input api.UpdateClassTeacherJSONBody) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		old, err := q.GetClassTeacher(ctx, id)
		if err != nil {
			return mapDatabaseError(err)
		}
		endsOn := optionalDate(input.EndsOn)
		if endsOn.Valid && endsOn.Time.Before(old.StartsOn) {
			return apperr.Unprocessable("Дата окончания раньше даты начала")
		}
		overlaps, err := q.ClassTeacherOverlapsExcept(ctx, db.ClassTeacherOverlapsExceptParams{ID: id, ClassID: old.ClassID, Column3: old.StartsOn, Column4: dateOrMax(endsOn)})
		if err != nil {
			return err
		}
		if overlaps {
			return apperr.Conflict("Назначение классного руководителя пересекается с существующим интервалом")
		}
		if _, err := q.UpdateClassTeacherEnd(ctx, db.UpdateClassTeacherEndParams{ID: id, EndsOn: endsOn}); err != nil {
			return err
		}
		newValues, _ := json.Marshal(map[string]any{"ends_on": input.EndsOn})
		return audit(ctx, q, actor, "class_teacher.updated", "class", old.ClassID, nil, newValues)
	})
}

func (s *Service) ListClassTeachers(ctx context.Context, actor Principal) ([]db.ClassTeacher, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, err
	}
	return s.queries.ListClassTeachers(ctx)
}

func (s *Service) UpdateParentStudent(ctx context.Context, actor Principal, input api.LinkParentStudentJSONBody) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	validTo := optionalDate(input.ValidTo)
	if validTo.Valid && validTo.Time.Before(input.ValidFrom.Time) {
		return apperr.Unprocessable("Дата окончания связи раньше даты начала")
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		affected, err := q.UpdateParentStudentEnd(ctx, db.UpdateParentStudentEndParams{ParentID: input.ParentId, StudentID: input.StudentId, Column3: input.ValidFrom.Time, ValidTo: validTo})
		if err != nil {
			return err
		}
		if affected == 0 {
			return apperr.NotFound()
		}
		newValues, _ := json.Marshal(map[string]any{"parent_id": input.ParentId, "student_id": input.StudentId, "valid_from": input.ValidFrom, "valid_to": input.ValidTo})
		return audit(ctx, q, actor, "parent_student.updated", "student", input.StudentId, nil, newValues)
	})
}

func (s *Service) GetSchoolSettings(ctx context.Context) (api.SchoolSettings, error) {
	row, err := s.queries.GetSchoolSettings(ctx)
	if err != nil {
		return api.SchoolSettings{}, mapDatabaseError(err)
	}
	return api.SchoolSettings{SchoolName: row.SchoolName, Timezone: row.Timezone}, nil
}

func (s *Service) UpdateSchoolSettings(ctx context.Context, actor Principal, input api.SchoolSettings) (api.SchoolSettings, error) {
	if err := actor.requireSystemAdmin(); err != nil {
		return api.SchoolSettings{}, err
	}
	input.SchoolName = strings.TrimSpace(input.SchoolName)
	if input.SchoolName == "" || len(input.SchoolName) > 200 {
		return api.SchoolSettings{}, apperr.Unprocessable("Проверьте название школы")
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return api.SchoolSettings{}, apperr.Unprocessable("Неизвестный часовой пояс")
	}
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := q.UpdateSchoolSettings(ctx, db.UpdateSchoolSettingsParams{SchoolName: input.SchoolName, Timezone: input.Timezone}); err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(input)
		return audit(ctx, q, actor, "school_settings.updated", "school_settings", uuid.Nil, nil, newValues)
	})
	return input, err
}

func (s *Service) ListGradeScales(ctx context.Context) ([]api.GradeScale, error) {
	rows, err := s.queries.ListGradeScales(ctx)
	if err != nil {
		return nil, err
	}
	result := make([]api.GradeScale, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapGradeScale(row.ID, row.Name, row.AllowedValues, row.IsActive))
	}
	return result, nil
}

func (s *Service) CreateGradeScale(ctx context.Context, actor Principal, input api.CreateGradeScaleJSONBody) (api.GradeScale, error) {
	if err := actor.requireSystemAdmin(); err != nil {
		return api.GradeScale{}, err
	}
	values, err := validGradeScale(input.Name, input.AllowedValues)
	if err != nil {
		return api.GradeScale{}, err
	}
	var row db.CreateGradeScaleRow
	err = s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var err error
		row, err = q.CreateGradeScale(ctx, db.CreateGradeScaleParams{Name: strings.TrimSpace(input.Name), AllowedValues: values, IsActive: false})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "grade_scale.created", "grade_scale", row.ID, nil, newValues)
	})
	return mapGradeScale(row.ID, row.Name, row.AllowedValues, row.IsActive), err
}

func (s *Service) UpdateGradeScale(ctx context.Context, actor Principal, id uuid.UUID, input api.UpdateGradeScaleJSONBody) (api.GradeScale, error) {
	if err := actor.requireSystemAdmin(); err != nil {
		return api.GradeScale{}, err
	}
	values, err := validGradeScale(input.Name, input.AllowedValues)
	if err != nil {
		return api.GradeScale{}, err
	}
	var row db.UpdateGradeScaleRow
	err = s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		var err error
		row, err = q.UpdateGradeScale(ctx, db.UpdateGradeScaleParams{ID: id, Name: strings.TrimSpace(input.Name), AllowedValues: values, IsActive: input.IsActive})
		if err != nil {
			return mapDatabaseError(err)
		}
		if input.IsActive {
			if err := q.ClearActiveGradeScales(ctx, id); err != nil {
				return err
			}
		}
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "grade_scale.updated", "grade_scale", id, nil, newValues)
	})
	return mapGradeScale(row.ID, row.Name, row.AllowedValues, row.IsActive), err
}

func validGradeScale(name string, values []float32) ([]byte, error) {
	if strings.TrimSpace(name) == "" || len(values) == 0 {
		return nil, apperr.Unprocessable("Введите название и хотя бы одно значение шкалы")
	}
	seen := map[float32]bool{}
	for _, value := range values {
		if value < 0 || value > 100 || seen[value] {
			return nil, apperr.Unprocessable("Значения шкалы должны быть уникальными числами от 0 до 100")
		}
		seen[value] = true
	}
	return json.Marshal(values)
}

func mapGradeScale(id uuid.UUID, name string, raw []byte, active bool) api.GradeScale {
	values := []float32{}
	_ = json.Unmarshal(raw, &values)
	return api.GradeScale{Id: id, Name: name, AllowedValues: values, IsActive: active}
}

func (s *Service) CreateScheduleEntry(ctx context.Context, actor Principal, input api.ScheduleEntryInput) (api.ScheduleEntry, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.ScheduleEntry{}, err
	}
	startsAt, endsAt, err := scheduleTimes(input.StartsAt, input.EndsAt)
	if err != nil {
		return api.ScheduleEntry{}, err
	}
	validTo := optionalDate(input.ValidTo)
	if input.Weekday < 1 || input.Weekday > 7 || input.LessonNumber < 1 || input.LessonNumber > 12 || (validTo.Valid && validTo.Time.Before(input.ValidFrom.Time)) {
		return api.ScheduleEntry{}, apperr.Unprocessable("Проверьте день, номер урока и период действия")
	}
	parity := 0
	if input.WeekParity != nil {
		parity = *input.WeekParity
	}
	if parity < 0 || parity > 2 {
		return api.ScheduleEntry{}, apperr.Unprocessable("Чётность недели должна быть 0, 1 или 2")
	}
	var row db.CreateScheduleEntryRow
	err = s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := lockLessonSchedule(ctx, tx); err != nil {
			return err
		}
		period, err := q.GetAcademicPeriod(ctx, input.AcademicPeriodId)
		if err != nil {
			return mapDatabaseError(err)
		}
		if input.ValidFrom.Before(period.StartsOn) || (validTo.Valid && validTo.Time.After(period.EndsOn)) {
			return apperr.Unprocessable("Расписание должно находиться внутри учебного периода")
		}
		if err := requireClassInYear(ctx, q, input.ClassId, period.AcademicYearID); err != nil {
			return err
		}
		allowed, err := q.TeacherAssignmentExists(ctx, db.TeacherAssignmentExistsParams{TeacherID: input.TeacherId, ClassID: input.ClassId, SubjectID: input.SubjectId, AcademicPeriodID: input.AcademicPeriodId, Column5: input.ValidFrom.Time})
		if err != nil {
			return err
		}
		if !allowed {
			return apperr.Unprocessable("Учитель не назначен на этот класс и предмет")
		}
		room := uuid.Nil
		if input.RoomId != nil {
			room = *input.RoomId
		}
		conflict, err := q.ScheduleEntryConflict(ctx, db.ScheduleEntryConflictParams{AcademicPeriodID: input.AcademicPeriodId, Weekday: int16(input.Weekday), Column3: startsAt, Column4: endsAt, Column5: input.ValidFrom.Time, Column6: dateOrMax(validTo), Column7: parity, ClassID: input.ClassId, TeacherID: input.TeacherId, Column10: room})
		if err != nil {
			return err
		}
		if conflict {
			return apperr.Conflict("Класс, учитель или кабинет уже заняты в расписании")
		}
		row, err = q.CreateScheduleEntry(ctx, db.CreateScheduleEntryParams{ClassID: input.ClassId, SubjectID: input.SubjectId, TeacherID: input.TeacherId, RoomID: optionalUUID(input.RoomId), AcademicPeriodID: input.AcademicPeriodId, Weekday: int16(input.Weekday), LessonNumber: int16(input.LessonNumber), StartsAt: startsAt, EndsAt: endsAt, ValidFrom: input.ValidFrom.Time, ValidTo: validTo, WeekParity: int16(parity)})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(row)
		return audit(ctx, q, actor, "schedule_entry.created", "schedule_entry", row.ID, nil, newValues)
	})
	if err != nil {
		return api.ScheduleEntry{}, err
	}
	return mapScheduleEntry(row.ID, row.ClassID, "", row.SubjectID, "", row.TeacherID, row.RoomID, pgtype.Text{}, row.AcademicPeriodID, row.Weekday, row.LessonNumber, row.StartsAt, row.EndsAt, row.ValidFrom, row.ValidTo, row.WeekParity), nil
}

func (s *Service) ListScheduleEntries(ctx context.Context, actor Principal, weekStart time.Time, studentID *uuid.UUID) ([]api.ScheduleEntry, error) {
	if studentID != nil || actor.StudentID != nil || actor.ParentID != nil {
		id, err := readableStudentID(actor, studentID)
		if err != nil {
			return nil, err
		}
		if err := canReadStudent(ctx, s.queries, actor, id, weekStart); err != nil {
			return nil, err
		}
		rows, err := s.queries.ListStudentScheduleEntries(ctx, db.ListStudentScheduleEntriesParams{StudentID: id, Column2: weekStart})
		if err != nil {
			return nil, err
		}
		result := make([]api.ScheduleEntry, 0, len(rows))
		for _, row := range rows {
			result = append(result, mapScheduleEntry(row.ID, row.ClassID, row.ClassName, row.SubjectID, row.SubjectName, row.TeacherID, row.RoomID, row.RoomName, row.AcademicPeriodID, row.Weekday, row.LessonNumber, row.StartsAt, row.EndsAt, row.ValidFrom, row.ValidTo, row.WeekParity))
		}
		return result, nil
	}
	if err := actor.requireTeacher(); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListTeacherScheduleEntries(ctx, db.ListTeacherScheduleEntriesParams{TeacherID: *actor.TeacherID, Column2: weekStart})
	if err != nil {
		return nil, err
	}
	result := make([]api.ScheduleEntry, 0, len(rows))
	for _, row := range rows {
		result = append(result, mapScheduleEntry(row.ID, row.ClassID, row.ClassName, row.SubjectID, row.SubjectName, row.TeacherID, row.RoomID, row.RoomName, row.AcademicPeriodID, row.Weekday, row.LessonNumber, row.StartsAt, row.EndsAt, row.ValidFrom, row.ValidTo, row.WeekParity))
	}
	return result, nil
}

func mapScheduleEntry(id, classID uuid.UUID, className string, subjectID uuid.UUID, subjectName string, teacherID uuid.UUID, roomID pgtype.UUID, roomName pgtype.Text, periodID uuid.UUID, weekday, number int16, startsAt, endsAt pgtype.Time, validFrom time.Time, validTo pgtype.Date, parity int16) api.ScheduleEntry {
	parityValue := int(parity)
	return api.ScheduleEntry{Id: id, ClassId: classID, ClassName: &className, SubjectId: subjectID, SubjectName: &subjectName, TeacherId: teacherID, RoomId: uuidPointer(roomID), RoomName: textPointer(roomName), AcademicPeriodId: periodID, Weekday: int(weekday), LessonNumber: int(number), StartsAt: formatScheduleTime(startsAt), EndsAt: formatScheduleTime(endsAt), ValidFrom: apiDate(validFrom), ValidTo: apiNullableDate(validTo), WeekParity: &parityValue}
}

func scheduleTimes(start, end string) (pgtype.Time, pgtype.Time, error) {
	startsAt, err := parseQuietTime(&start)
	if err != nil {
		return pgtype.Time{}, pgtype.Time{}, apperr.Unprocessable("Время должно быть в формате HH:MM")
	}
	endsAt, err := parseQuietTime(&end)
	if err != nil || endsAt.Microseconds <= startsAt.Microseconds {
		return pgtype.Time{}, pgtype.Time{}, apperr.Unprocessable("Проверьте время начала и окончания урока")
	}
	return startsAt, endsAt, nil
}

func formatScheduleTime(value pgtype.Time) string {
	seconds := value.Microseconds / 1_000_000
	return fmt.Sprintf("%02d:%02d", seconds/3600, seconds%3600/60)
}

func (s *Service) CurrentLessons(ctx context.Context, actor Principal, studentID *uuid.UUID) (api.CurrentLessons, error) {
	now := time.Now()
	from := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	lessons, err := s.GetSchedule(ctx, actor, from.AddDate(0, 0, -1), from.AddDate(0, 0, 8), studentID)
	if err != nil {
		return api.CurrentLessons{}, err
	}
	var result api.CurrentLessons
	for _, lesson := range lessons {
		if lesson.Status == api.LessonStatusCancelled {
			continue
		}
		if !now.Before(lesson.StartsAt) && now.Before(lesson.EndsAt) {
			lessonCopy := lesson
			result.Current = &lessonCopy
		}
		if lesson.StartsAt.After(now) && result.Next == nil {
			lessonCopy := lesson
			result.Next = &lessonCopy
		}
	}
	return result, nil
}

func (s *Service) CopyHomework(ctx context.Context, actor Principal, homeworkID uuid.UUID, input api.CopyHomeworkJSONBody) (api.Homework, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.Homework{}, err
	}
	status := string(input.Status)
	if status != "draft" && status != "published" {
		return api.Homework{}, apperr.Unprocessable("Неизвестный статус задания")
	}
	var copyID uuid.UUID
	var copiedPaths []string
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		original, err := q.GetHomework(ctx, homeworkID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := requireHomeworkOwner(ctx, q, actor, original); err != nil {
			return err
		}
		if err := s.requireHomeworkAssignment(ctx, q, actor, original.ClassID, original.SubjectID, uuidPointer(original.LessonID), input.DueDate.Time); err != nil {
			return err
		}
		created, err := q.CreateHomework(ctx, db.CreateHomeworkParams{ClassID: original.ClassID, SubjectID: original.SubjectID, LessonID: pgtype.UUID{}, AuthorID: actor.UserID, Text: original.Text, DueDate: input.DueDate.Time, Status: status})
		if err != nil {
			return mapDatabaseError(err)
		}
		copyID = created.ID
		if input.StudentIds != nil {
			for _, studentID := range *input.StudentIds {
				if err := requireStudentInClass(ctx, q, studentID, original.ClassID, input.DueDate.Time); err != nil {
					return err
				}
				if err := q.AddHomeworkRecipient(ctx, db.AddHomeworkRecipientParams{HomeworkID: copyID, StudentID: studentID}); err != nil {
					return mapDatabaseError(err)
				}
			}
		}
		files, err := q.ListHomeworkFiles(ctx, homeworkID)
		if err != nil {
			return err
		}
		for _, file := range files {
			metadata, err := q.GetFile(ctx, file.ID)
			if err != nil {
				return mapDatabaseError(err)
			}
			if !isSafeStorageKey(metadata.StorageKey) {
				return apperr.NotFound()
			}
			content, err := os.ReadFile(filepath.Join(s.fileRoot, metadata.StorageKey))
			if err != nil {
				return err
			}
			storageKey := uuid.NewString() + ".bin"
			path := filepath.Join(s.fileRoot, storageKey)
			if err := os.WriteFile(path, content, 0o600); err != nil {
				return err
			}
			copiedPaths = append(copiedPaths, path)
			cloned, err := q.CreateFile(ctx, db.CreateFileParams{StorageKey: storageKey, OriginalName: metadata.OriginalName, SizeBytes: metadata.SizeBytes, MimeType: metadata.MimeType, OwnerID: actor.UserID})
			if err != nil {
				return mapDatabaseError(err)
			}
			if err := q.AttachHomeworkFile(ctx, db.AttachHomeworkFileParams{HomeworkID: copyID, FileID: cloned.ID}); err != nil {
				return mapDatabaseError(err)
			}
		}
		newValues, _ := json.Marshal(map[string]any{"source_id": homeworkID, "due_date": input.DueDate, "status": status})
		if err := audit(ctx, q, actor, "homework.copied", "homework", copyID, nil, newValues); err != nil {
			return err
		}
		if status == "published" {
			return notifyHomework(ctx, q, copyID)
		}
		return nil
	})
	if err != nil {
		for _, path := range copiedPaths {
			_ = os.Remove(path)
		}
		return api.Homework{}, err
	}
	return s.homeworkByID(ctx, copyID)
}

func (s *Service) ArchiveFile(ctx context.Context, actor Principal, id uuid.UUID) error {
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if _, err := q.ArchiveFile(ctx, db.ArchiveFileParams{ID: id, OwnerID: actor.UserID}); err != nil {
			return mapDatabaseError(err)
		}
		return audit(ctx, q, actor, "file.archived", "file", id, nil, []byte(`{"archived":true}`))
	})
}

func (s *Service) ArchiveAnnouncement(ctx context.Context, actor Principal, id uuid.UUID) error {
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if _, err := q.ArchiveAnnouncement(ctx, db.ArchiveAnnouncementParams{ID: id, AuthorID: actor.UserID}); err != nil {
			return mapDatabaseError(err)
		}
		return audit(ctx, q, actor, "announcement.archived", "announcement", id, nil, []byte(`{"archived":true}`))
	})
}

func (s *Service) NotificationDigest(ctx context.Context, actor Principal, frequency string) (map[string]any, error) {
	if frequency != "daily" && frequency != "weekly" {
		return nil, apperr.Unprocessable("Частота сводки должна быть daily или weekly")
	}
	now := time.Now()
	from := now.Add(-24 * time.Hour)
	if frequency == "weekly" {
		from = now.Add(-7 * 24 * time.Hour)
	}
	rows, err := s.queries.ListDigestNotifications(ctx, db.ListDigestNotificationsParams{UserID: actor.UserID, Column2: from, Column3: now})
	if err != nil {
		return nil, err
	}
	items := make([]api.Notification, 0, len(rows))
	for _, row := range rows {
		items = append(items, api.Notification{Id: row.ID, Kind: row.Kind, Title: row.Title, Body: row.Body, EntityType: textPointer(row.EntityType), EntityId: uuidPointer(row.EntityID), ReadAt: timePointer(row.ReadAt), CreatedAt: row.CreatedAt})
	}
	return map[string]any{"frequency": frequency, "from": from, "to": now, "count": len(items), "notifications": items}, nil
}

func (s *Service) StudentReport(ctx context.Context, actor Principal, studentID, periodID uuid.UUID) (map[string]any, error) {
	grades, err := s.ListGrades(ctx, actor, &studentID, &periodID)
	if err != nil {
		return nil, err
	}
	attendance, err := s.queries.StudentAttendanceReport(ctx, db.StudentAttendanceReportParams{StudentID: studentID, AcademicPeriodID: periodID})
	if err != nil {
		return nil, err
	}
	return map[string]any{"student_id": studentID, "academic_period_id": periodID, "grades": grades, "attendance": attendance}, nil
}

func (s *Service) ClassAttendanceReport(ctx context.Context, actor Principal, classID, periodID uuid.UUID) ([]db.ClassAttendanceReportRow, error) {
	period, err := s.queries.GetAcademicPeriod(ctx, periodID)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	if err := requireClassReportAccess(ctx, s.queries, actor, classID, period.StartsOn); err != nil {
		return nil, err
	}
	return s.queries.ClassAttendanceReport(ctx, db.ClassAttendanceReportParams{ClassID: classID, AcademicPeriodID: periodID})
}

// UnfilledLessonsReport показывает школу целиком руководителю и администратору, а учителю — только его собственные уроки.
func (s *Service) UnfilledLessonsReport(ctx context.Context, actor Principal, periodID uuid.UUID) ([]db.UnfilledLessonsReportRow, error) {
	teacherID := uuid.Nil
	if !actor.HasRole("principal") && !actor.IsAdministrator() {
		if err := actor.requireTeacher(); err != nil {
			return nil, err
		}
		teacherID = *actor.TeacherID
	}
	return s.queries.UnfilledLessonsReport(ctx, db.UnfilledLessonsReportParams{AcademicPeriodID: periodID, Column2: teacherID})
}

func (s *Service) SubjectReport(ctx context.Context, actor Principal, subjectID, classID, periodID uuid.UUID) ([]db.SubjectReportRow, error) {
	period, err := s.queries.GetAcademicPeriod(ctx, periodID)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	if err := requireClassReportAccess(ctx, s.queries, actor, classID, period.StartsOn); err != nil {
		return nil, err
	}
	return s.queries.SubjectReport(ctx, db.SubjectReportParams{ClassID: classID, SubjectID: subjectID, ID: periodID})
}
