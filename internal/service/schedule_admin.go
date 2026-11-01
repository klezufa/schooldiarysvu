package service

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

const maxGenerateDays = 120

func (p Principal) canReadSchoolSchedule() bool { return p.IsAdministrator() || p.HasRole("principal") }

// ListAdminScheduleEntries возвращает недельный шаблон расписания для экрана редактирования.
func (s *Service) ListAdminScheduleEntries(ctx context.Context, actor Principal, classID, teacherID, periodID *uuid.UUID) ([]api.ScheduleEntry, error) {
	if !actor.canReadSchoolSchedule() {
		return nil, apperr.Forbidden()
	}
	rows, err := s.queries.ListScheduleEntriesAdmin(ctx, db.ListScheduleEntriesAdminParams{
		ClassID: optionalUUID(classID), TeacherID: optionalUUID(teacherID), PeriodID: optionalUUID(periodID),
		OnDate: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		return nil, err
	}
	result := make([]api.ScheduleEntry, 0, len(rows))
	for _, row := range rows {
		entry := mapScheduleEntry(row.ID, row.ClassID, row.ClassName, row.SubjectID, row.SubjectName, row.TeacherID, row.RoomID, row.RoomName,
			row.AcademicPeriodID, row.Weekday, row.LessonNumber, row.StartsAt, row.EndsAt, row.ValidFrom, row.ValidTo, row.WeekParity)
		teacherName := row.TeacherName
		entry.TeacherName = &teacherName
		result = append(result, entry)
	}
	return result, nil
}

// UpdateScheduleEntry меняет учителя, кабинет, время и срок действия строки шаблона. Класс и предмет менять нельзя: создайте новую строку.
func (s *Service) UpdateScheduleEntry(ctx context.Context, actor Principal, id uuid.UUID, input api.ScheduleEntryUpdate) (api.ScheduleEntry, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.ScheduleEntry{}, err
	}
	startsAt, endsAt, err := scheduleTimes(input.StartsAt, input.EndsAt)
	if err != nil {
		return api.ScheduleEntry{}, err
	}
	validTo := optionalDate(input.ValidTo)
	parity := 0
	if input.WeekParity != nil {
		parity = *input.WeekParity
	}
	if input.Weekday < 1 || input.Weekday > 7 || input.LessonNumber < 1 || input.LessonNumber > 12 || parity < 0 || parity > 2 ||
		(validTo.Valid && validTo.Time.Before(input.ValidFrom.Time)) {
		return api.ScheduleEntry{}, apperr.Unprocessable("Проверьте день, номер урока, чётность и срок действия")
	}
	err = s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := lockLessonSchedule(ctx, tx); err != nil {
			return err
		}
		old, err := q.GetScheduleEntry(ctx, id)
		if err != nil {
			return mapDatabaseError(err)
		}
		period, err := q.GetAcademicPeriod(ctx, old.AcademicPeriodID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if input.ValidFrom.Before(period.StartsOn) || (validTo.Valid && validTo.Time.After(period.EndsOn)) {
			return apperr.Unprocessable("Расписание должно находиться внутри учебного периода")
		}
		assigned, err := q.TeacherAssignmentExists(ctx, db.TeacherAssignmentExistsParams{TeacherID: input.TeacherId, ClassID: old.ClassID,
			SubjectID: old.SubjectID, AcademicPeriodID: old.AcademicPeriodID, Column5: input.ValidFrom.Time})
		if err != nil {
			return err
		}
		if !assigned {
			return apperr.Unprocessable("Учитель не назначен на этот класс и предмет")
		}
		conflict, err := q.ScheduleEntryConflictExcluding(ctx, db.ScheduleEntryConflictExcludingParams{SelfID: id, PeriodID: old.AcademicPeriodID,
			Weekday: int16(input.Weekday), StartsAt: startsAt, EndsAt: endsAt, ValidTo: validTo, ValidFrom: input.ValidFrom.Time, WeekParity: int16(parity),
			ClassID: old.ClassID, TeacherID: input.TeacherId, RoomID: optionalUUID(input.RoomId)})
		if err != nil {
			return err
		}
		if conflict {
			return apperr.Conflict("Класс, учитель или кабинет уже заняты в расписании")
		}
		if _, err := q.UpdateScheduleEntry(ctx, db.UpdateScheduleEntryParams{ID: id, TeacherID: input.TeacherId, RoomID: optionalUUID(input.RoomId),
			Weekday: int16(input.Weekday), LessonNumber: int16(input.LessonNumber), StartsAt: startsAt, EndsAt: endsAt,
			ValidFrom: input.ValidFrom.Time, ValidTo: validTo, WeekParity: int16(parity)}); err != nil {
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(old)
		newValues, _ := json.Marshal(input)
		if err := audit(ctx, q, actor, "schedule_entry.updated", "schedule_entry", id, oldValues, newValues); err != nil {
			return err
		}
		return s.resyncFutureLessons(ctx, q, actor, id, old.ClassID, true)
	})
	if err != nil {
		return api.ScheduleEntry{}, err
	}
	return s.scheduleEntryByID(ctx, id)
}

func (s *Service) scheduleEntryByID(ctx context.Context, id uuid.UUID) (api.ScheduleEntry, error) {
	entry, err := s.queries.GetScheduleEntry(ctx, id)
	if err != nil {
		return api.ScheduleEntry{}, mapDatabaseError(err)
	}
	rows, err := s.queries.ListScheduleEntriesAdmin(ctx, db.ListScheduleEntriesAdminParams{ClassID: nullableUUID(entry.ClassID), OnDate: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		return api.ScheduleEntry{}, err
	}
	for _, row := range rows {
		if row.ID == id {
			result := mapScheduleEntry(row.ID, row.ClassID, row.ClassName, row.SubjectID, row.SubjectName, row.TeacherID, row.RoomID, row.RoomName,
				row.AcademicPeriodID, row.Weekday, row.LessonNumber, row.StartsAt, row.EndsAt, row.ValidFrom, row.ValidTo, row.WeekParity)
			teacherName := row.TeacherName
			result.TeacherName = &teacherName
			return result, nil
		}
	}
	return api.ScheduleEntry{}, apperr.NotFound()
}

// DeleteScheduleEntry удаляет строку шаблона и её будущие незаполненные уроки; проведённые и заполненные уроки остаются.
func (s *Service) DeleteScheduleEntry(ctx context.Context, actor Principal, id uuid.UUID) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	return s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := lockLessonSchedule(ctx, tx); err != nil {
			return err
		}
		old, err := q.GetScheduleEntry(ctx, id)
		if err != nil {
			return mapDatabaseError(err)
		}
		// До удаления строки: потом её уроки потеряют ссылку на шаблон (ON DELETE SET NULL).
		if err := s.resyncFutureLessons(ctx, q, actor, id, old.ClassID, false); err != nil {
			return err
		}
		if err := q.DeleteScheduleEntry(ctx, id); err != nil {
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(old)
		return audit(ctx, q, actor, "schedule_entry.deleted", "schedule_entry", id, oldValues, nil)
	})
}

// GetSchoolSchedule возвращает уроки класса или учителя за период для администратора, руководителя и классного руководителя класса.
func (s *Service) GetSchoolSchedule(ctx context.Context, actor Principal, from, to time.Time, classID, teacherID *uuid.UUID) ([]api.Lesson, error) {
	if from.IsZero() || to.Before(from) || to.Sub(from) > 92*24*time.Hour {
		return nil, apperr.Unprocessable("Проверьте период расписания (не более 92 дней)")
	}
	if (classID == nil) == (teacherID == nil) {
		return nil, apperr.BadRequest("Укажите класс или учителя")
	}
	var ids []uuid.UUID
	if classID != nil {
		if !actor.canReadSchoolSchedule() {
			if err := requireClassReportAccess(ctx, s.queries, actor, *classID, from); err != nil {
				return nil, err
			}
		}
		rows, err := s.queries.ListClassLessons(ctx, db.ListClassLessonsParams{ClassID: *classID, Column2: from, Column3: to})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
	} else {
		if !actor.canReadSchoolSchedule() {
			return nil, apperr.Forbidden()
		}
		rows, err := s.queries.ListTeacherLessons(ctx, db.ListTeacherLessonsParams{TeacherID: *teacherID, Column2: from, Column3: to})
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			ids = append(ids, row.ID)
		}
	}
	return s.lessonsByIDs(ctx, ids)
}

// CarryOverPeriod переносит в период назначения учителей и (по умолчанию) недельный шаблон расписания, действующие на
// последний день исходного периода. Назначения и строки шаблона привязаны к периоду, поэтому без переноса в новой
// четверти администратору пришлось бы заводить их заново. Проверки те же, что при ручном создании: назначение на класс
// и предмет не пересекается с существующим, строка шаблона требует назначения учителя и свободного слота. Всё, что не
// проходит проверку, пропускается — повторный вызов ничего не дублирует. Между учебными годами классы разные
// (5А прошлого года и 6А нового — разные записи), поэтому для такого переноса нужен класс назначения target_class_id.
func (s *Service) CarryOverPeriod(ctx context.Context, actor Principal, targetID uuid.UUID, input api.CarryOverRequest) (api.CarryOverResult, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.CarryOverResult{}, err
	}
	if input.FromPeriodId == targetID {
		return api.CarryOverResult{}, apperr.Unprocessable("Выберите другой период, из которого переносить")
	}
	if input.TargetClassId != nil && input.ClassId == nil {
		return api.CarryOverResult{}, apperr.Unprocessable("Укажите класс, из которого переносить")
	}
	includeSchedule := input.IncludeSchedule == nil || *input.IncludeSchedule
	// classFor — класс в целевом периоде для назначения или строки шаблона исходного класса.
	classFor := func(source uuid.UUID) uuid.UUID {
		if input.TargetClassId != nil {
			return *input.TargetClassId
		}
		return source
	}
	var result api.CarryOverResult
	err := s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		result = api.CarryOverResult{}
		if err := lockLessonSchedule(ctx, tx); err != nil {
			return err
		}
		source, err := q.GetAcademicPeriod(ctx, input.FromPeriodId)
		if err != nil {
			return mapDatabaseError(err)
		}
		target, err := q.GetAcademicPeriod(ctx, targetID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if target.Status == "closed" {
			return apperr.Unprocessable("Период закрыт: переносить в него назначения и расписание нельзя")
		}
		if source.AcademicYearID != target.AcademicYearID && input.TargetClassId == nil {
			return apperr.Unprocessable("Периоды из разных учебных лет: укажите, какому классу нового года перенести назначения (например, 5А → 6А)")
		}
		if input.ClassId != nil {
			if err := requireClassInYear(ctx, q, *input.ClassId, source.AcademicYearID); err != nil {
				return err
			}
		}
		if input.TargetClassId != nil {
			if err := requireClassInYear(ctx, q, *input.TargetClassId, target.AcademicYearID); err != nil {
				return err
			}
		}
		assignments, err := q.ListCarryOverAssignments(ctx, db.ListCarryOverAssignmentsParams{PeriodID: source.ID, OnDate: source.EndsOn, ClassID: optionalUUID(input.ClassId)})
		if err != nil {
			return err
		}
		createdAssignments := make([]uuid.UUID, 0, len(assignments))
		for _, item := range assignments {
			overlaps, err := q.TeachingAssignmentOverlaps(ctx, db.TeachingAssignmentOverlapsParams{ClassID: classFor(item.ClassID), SubjectID: item.SubjectID,
				AcademicPeriodID: target.ID, Column4: target.StartsOn, Column5: dateOrMax(pgtype.Date{})})
			if err != nil {
				return err
			}
			if overlaps {
				result.AssignmentsSkipped++
				continue
			}
			created, err := q.CreateTeachingAssignment(ctx, db.CreateTeachingAssignmentParams{TeacherID: item.TeacherID, ClassID: classFor(item.ClassID),
				SubjectID: item.SubjectID, AcademicPeriodID: target.ID, StartsOn: target.StartsOn})
			if err != nil {
				return mapDatabaseError(err)
			}
			createdAssignments = append(createdAssignments, created.ID)
			result.AssignmentsCreated++
		}
		createdEntries := []uuid.UUID{}
		if includeSchedule {
			entries, err := q.ListScheduleEntriesAdmin(ctx, db.ListScheduleEntriesAdminParams{ClassID: optionalUUID(input.ClassId),
				PeriodID: nullableUUID(source.ID), OnDate: source.EndsOn})
			if err != nil {
				return err
			}
			for _, entry := range entries {
				assigned, err := q.TeacherAssignmentExists(ctx, db.TeacherAssignmentExistsParams{TeacherID: entry.TeacherID, ClassID: classFor(entry.ClassID),
					SubjectID: entry.SubjectID, AcademicPeriodID: target.ID, Column5: target.StartsOn})
				if err != nil {
					return err
				}
				room := uuid.Nil
				if entry.RoomID.Valid {
					room = uuid.UUID(entry.RoomID.Bytes)
				}
				conflict := false
				if assigned {
					conflict, err = q.ScheduleEntryConflict(ctx, db.ScheduleEntryConflictParams{AcademicPeriodID: target.ID, Weekday: entry.Weekday,
						Column3: entry.StartsAt, Column4: entry.EndsAt, Column5: target.StartsOn, Column6: dateOrMax(pgtype.Date{}), Column7: int(entry.WeekParity),
						ClassID: classFor(entry.ClassID), TeacherID: entry.TeacherID, Column10: room})
					if err != nil {
						return err
					}
				}
				if !assigned || conflict {
					result.EntriesSkipped++
					continue
				}
				row, err := q.CreateScheduleEntry(ctx, db.CreateScheduleEntryParams{ClassID: classFor(entry.ClassID), SubjectID: entry.SubjectID, TeacherID: entry.TeacherID,
					RoomID: entry.RoomID, AcademicPeriodID: target.ID, Weekday: entry.Weekday, LessonNumber: entry.LessonNumber, StartsAt: entry.StartsAt,
					EndsAt: entry.EndsAt, ValidFrom: target.StartsOn, WeekParity: entry.WeekParity})
				if err != nil {
					return mapDatabaseError(err)
				}
				createdEntries = append(createdEntries, row.ID)
				result.EntriesCreated++
			}
		}
		newValues, _ := json.Marshal(map[string]any{"from_period_id": source.ID, "class_id": input.ClassId, "target_class_id": input.TargetClassId, "include_schedule": includeSchedule,
			"assignment_ids": createdAssignments, "schedule_entry_ids": createdEntries,
			"assignments_skipped": result.AssignmentsSkipped, "entries_skipped": result.EntriesSkipped})
		return audit(ctx, q, actor, "academic_period.carried_over", "academic_period", target.ID, nil, newValues)
	})
	return result, err
}

func clockToDuration(value pgtype.Time) time.Duration {
	return time.Duration(value.Microseconds) * time.Microsecond
}

// GenerateLessons создаёт уроки на период по недельному шаблону. Операция идемпотентна: для пары «строка шаблона — дата»
// урок создаётся один раз, ручные правки уже созданных уроков не затрагиваются.
func (s *Service) GenerateLessons(ctx context.Context, actor Principal, input api.GenerateLessonsRequest) (api.GenerateLessonsResult, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.GenerateLessonsResult{}, err
	}
	from, to := input.From.Time, input.To.Time
	if to.Before(from) || to.Sub(from) > maxGenerateDays*24*time.Hour {
		return api.GenerateLessonsResult{}, apperr.Unprocessable("Период генерации: от 1 до 120 дней")
	}
	skip := map[string]bool{}
	if input.SkipDates != nil {
		for _, day := range *input.SkipDates {
			skip[day.Format("2006-01-02")] = true
		}
	}
	settings, err := s.queries.GetSchoolSettings(ctx)
	if err != nil {
		return api.GenerateLessonsResult{}, mapDatabaseError(err)
	}
	location, err := time.LoadLocation(settings.Timezone)
	if err != nil {
		return api.GenerateLessonsResult{}, err
	}
	var result api.GenerateLessonsResult
	err = s.transactionReadCommitted(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := lockLessonSchedule(ctx, tx); err != nil {
			return err
		}
		entries, err := q.ListScheduleEntriesAdmin(ctx, db.ListScheduleEntriesAdminParams{ClassID: optionalUUID(input.ClassId), OnDate: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return err
		}
		result, err = generateLessonsFromEntries(ctx, q, actor, entries, from, to, skip, location)
		if err != nil {
			return err
		}
		newValues, _ := json.Marshal(map[string]any{"from": from.Format("2006-01-02"), "to": to.Format("2006-01-02"), "class_id": input.ClassId,
			"created": result.Created, "existing": result.Existing, "conflicts": result.Conflicts})
		return audit(ctx, q, actor, "lessons.generated", "lesson_batch", uuid.New(), nil, newValues)
	})
	return result, err
}

// generateLessonsFromEntries создаёт уроки по строкам шаблона на даты from..to внутри транзакции с блокировкой расписания.
func generateLessonsFromEntries(ctx context.Context, q *db.Queries, actor Principal, entries []db.ListScheduleEntriesAdminRow, from, to time.Time,
	skip map[string]bool, location *time.Location) (api.GenerateLessonsResult, error) {
	var result api.GenerateLessonsResult
	existingRows, err := q.ListGeneratedLessonDates(ctx, db.ListGeneratedLessonDatesParams{FromDate: from, ToDate: to})
	if err != nil {
		return result, err
	}
	existing := make(map[string]bool, len(existingRows))
	for _, row := range existingRows {
		existing[uuid.UUID(row.ScheduleEntryID.Bytes).String()+row.LessonDate.Format("2006-01-02")] = true
	}
	periods := map[uuid.UUID]db.GetAcademicPeriodRow{}
	for day := from; !day.After(to); day = day.AddDate(0, 0, 1) {
		if skip[day.Format("2006-01-02")] {
			continue
		}
		isoWeekday := int16(day.Weekday())
		if isoWeekday == 0 {
			isoWeekday = 7
		}
		_, isoWeek := day.ISOWeek()
		for _, entry := range entries {
			if entry.Weekday != isoWeekday || day.Before(entry.ValidFrom) || (entry.ValidTo.Valid && day.After(entry.ValidTo.Time)) {
				continue
			}
			if entry.WeekParity == 1 && isoWeek%2 == 0 || entry.WeekParity == 2 && isoWeek%2 == 1 {
				continue
			}
			period, known := periods[entry.AcademicPeriodID]
			if !known {
				period, err = q.GetAcademicPeriod(ctx, entry.AcademicPeriodID)
				if err != nil {
					return result, mapDatabaseError(err)
				}
				periods[entry.AcademicPeriodID] = period
			}
			if day.Before(period.StartsOn) || day.After(period.EndsOn) || period.Status == "closed" {
				continue
			}
			if existing[entry.ID.String()+day.Format("2006-01-02")] {
				result.Existing++
				continue
			}
			// Учитель мог смениться: урок без действующего назначения учитель не сможет заполнить.
			assigned, err := q.TeacherAssignmentExists(ctx, db.TeacherAssignmentExistsParams{TeacherID: entry.TeacherID, ClassID: entry.ClassID,
				SubjectID: entry.SubjectID, AcademicPeriodID: entry.AcademicPeriodID, Column5: day})
			if err != nil {
				return result, err
			}
			if !assigned {
				result.Conflicts++
				continue
			}
			midnight := time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, location)
			startsAt, endsAt := midnight.Add(clockToDuration(entry.StartsAt)), midnight.Add(clockToDuration(entry.EndsAt))
			room := uuidPointer(entry.RoomID)
			if err := checkLessonConflict(ctx, q, uuid.Nil, startsAt, endsAt, entry.ClassID, entry.TeacherID, room); err != nil {
				result.Conflicts++
				continue
			}
			if _, err := q.CreateLesson(ctx, db.CreateLessonParams{LessonDate: day, StartsAt: startsAt, EndsAt: endsAt, ClassID: entry.ClassID,
				SubjectID: entry.SubjectID, TeacherID: entry.TeacherID, RoomID: entry.RoomID, AcademicPeriodID: entry.AcademicPeriodID,
				ScheduleEntryID: nullableUUID(entry.ID), Topic: "", CreatedBy: nullableUUID(actor.UserID)}); err != nil {
				return result, mapDatabaseError(err)
			}
			result.Created++
		}
	}
	return result, nil
}

// resyncFutureLessons убирает будущие незаполненные уроки строки шаблона и, если строка осталась, создаёт их заново по её
// новым данным на тот же горизонт: иначе после переноса урока в расписании остались бы уроки по старому шаблону.
// Уроки с темой, работами, посещаемостью или заданиями не трогаются.
func (s *Service) resyncFutureLessons(ctx context.Context, q *db.Queries, actor Principal, entryID, classID uuid.UUID, regenerate bool) error {
	removed, err := q.DeleteUntouchedFutureLessons(ctx, nullableUUID(entryID))
	if err != nil {
		return mapDatabaseError(err)
	}
	if len(removed) == 0 {
		return nil
	}
	var created api.GenerateLessonsResult
	if regenerate {
		settings, err := q.GetSchoolSettings(ctx)
		if err != nil {
			return mapDatabaseError(err)
		}
		location, err := time.LoadLocation(settings.Timezone)
		if err != nil {
			return fmt.Errorf("load school timezone: %w", err)
		}
		horizon := removed[0]
		for _, day := range removed {
			if day.After(horizon) {
				horizon = day
			}
		}
		rows, err := q.ListScheduleEntriesAdmin(ctx, db.ListScheduleEntriesAdminParams{ClassID: nullableUUID(classID), OnDate: time.Date(1, 1, 1, 0, 0, 0, 0, time.UTC)})
		if err != nil {
			return err
		}
		entries := slices.DeleteFunc(rows, func(row db.ListScheduleEntriesAdminRow) bool { return row.ID != entryID })
		created, err = generateLessonsFromEntries(ctx, q, actor, entries, s.schoolToday(ctx), horizon, nil, location)
		if err != nil {
			return err
		}
	}
	newValues, _ := json.Marshal(map[string]any{"removed": len(removed), "created": created.Created, "conflicts": created.Conflicts})
	return audit(ctx, q, actor, "lessons.resynced", "schedule_entry", entryID, nil, newValues)
}
