package service

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

type resolvedPeriod struct {
	ID   uuid.UUID
	Name string
}

// resolvePeriod выбирает учебный период для дашборда: указанный явно или тот, что идёт сегодня; если сегодня каникулы —
// ближайший прошедший период. Если периодов нет совсем, возвращает nil — дашборд при этом остаётся рабочим, но пустым.
func (s *Service) resolvePeriod(ctx context.Context, requested *uuid.UUID, today time.Time) (*resolvedPeriod, error) {
	if requested != nil {
		period, err := s.queries.GetAcademicPeriod(ctx, *requested)
		if err != nil {
			return nil, mapDatabaseError(err)
		}
		return &resolvedPeriod{ID: period.ID, Name: period.Name}, nil
	}
	period, err := s.queries.AcademicPeriodForDate(ctx, today)
	if err == nil {
		return &resolvedPeriod{ID: period.ID, Name: period.Name}, nil
	}
	if !isNoRows(err) {
		return nil, err
	}
	years, err := s.queries.ListAcademicYears(ctx)
	if err != nil {
		return nil, err
	}
	for _, year := range years {
		if !year.IsCurrent {
			continue
		}
		periods, err := s.queries.ListAcademicPeriods(ctx, year.ID)
		if err != nil {
			return nil, err
		}
		var best *db.ListAcademicPeriodsRow
		for index := range periods {
			if !periods[index].StartsOn.After(today) && (best == nil || periods[index].StartsOn.After(best.StartsOn)) {
				best = &periods[index]
			}
		}
		if best == nil && len(periods) > 0 {
			best = &periods[0]
		}
		if best != nil {
			return &resolvedPeriod{ID: best.ID, Name: best.Name}, nil
		}
	}
	return nil, nil
}

func periodFields(period *resolvedPeriod) (*openapi_types.UUID, *string) {
	if period == nil {
		return nil, nil
	}
	id, name := period.ID, period.Name
	return &id, &name
}

func round(value float64, places int) float64 {
	factor := math.Pow(10, float64(places))
	return math.Round(value*factor) / factor
}

func ratio(part, whole float64) *float64 {
	if whole <= 0 {
		return nil
	}
	value := round(part/whole, 4)
	return &value
}

func roundedPointer(value float64, valid bool) *float64 {
	if !valid {
		return nil
	}
	rounded := round(value, 2)
	return &rounded
}

func valueCounts[T any](rows []T, pick func(T) (float64, int)) []api.ValueCount {
	result := make([]api.ValueCount, 0, len(rows))
	for _, row := range rows {
		value, count := pick(row)
		result = append(result, api.ValueCount{Value: float32(value), Count: count})
	}
	return result
}

// StudentAnalytics собирает показатели ученика. Доступно самому ученику и его родителю.
func (s *Service) StudentAnalytics(ctx context.Context, actor Principal, requested, periodID *uuid.UUID) (api.StudentAnalytics, error) {
	studentID, err := readableStudentID(actor, requested)
	if err != nil {
		return api.StudentAnalytics{}, err
	}
	today := s.schoolToday(ctx)
	if err := canReadStudent(ctx, s.queries, actor, studentID, today); err != nil {
		return api.StudentAnalytics{}, err
	}
	period, err := s.resolvePeriod(ctx, periodID, today)
	if err != nil {
		return api.StudentAnalytics{}, err
	}
	result := api.StudentAnalytics{StudentId: studentID, Subjects: []api.SubjectAverage{}, Dynamics: []api.WeekAverage{}, Distribution: []api.ValueCount{},
		Recent: []api.RecentGrade{}, UpcomingHomework: []api.UpcomingHomework{}}
	result.PeriodId, result.PeriodName = periodFields(period)
	homework, err := s.queries.StudentHomeworkStats(ctx, db.StudentHomeworkStatsParams{StudentID: studentID, Today: today})
	if err != nil {
		return result, err
	}
	result.Homework = api.HomeworkStats{Done: int(homework.Done), Open: int(homework.Open), Overdue: int(homework.Overdue)}
	upcoming, err := s.queries.StudentUpcomingHomework(ctx, db.StudentUpcomingHomeworkParams{StudentID: studentID, Today: today, RowLimit: 6})
	if err != nil {
		return result, err
	}
	for _, item := range upcoming {
		result.UpcomingHomework = append(result.UpcomingHomework, api.UpcomingHomework{Id: item.ID, SubjectName: item.SubjectName, Text: item.Text, DueDate: apiDate(item.DueDate), Completed: item.Completed})
	}
	recent, err := s.queries.StudentRecentGrades(ctx, db.StudentRecentGradesParams{StudentID: studentID, Limit: 8})
	if err != nil {
		return result, err
	}
	for _, item := range recent {
		result.Recent = append(result.Recent, api.RecentGrade{LessonDate: apiDate(item.LessonDate), SubjectName: item.SubjectName, Title: item.Title, WorkType: item.WorkType, Value: float32(item.Value)})
	}
	if period == nil {
		return result, nil
	}
	subjects, err := s.queries.StudentSubjectAverages(ctx, db.StudentSubjectAveragesParams{StudentID: studentID, AcademicPeriodID: period.ID})
	if err != nil {
		return result, err
	}
	var weighted, weight float64
	for _, row := range subjects {
		item := api.SubjectAverage{SubjectId: row.SubjectID, SubjectName: row.SubjectName, GradesCount: int(row.GradesCount)}
		if row.Average >= 0 {
			item.Average = roundedPtr32(row.Average)
			weighted += row.Average * float64(row.GradesCount)
			weight += float64(row.GradesCount)
		}
		if row.TermValue >= 0 {
			item.TermValue = roundedPtr32(row.TermValue)
		}
		result.GradesCount += int(row.GradesCount)
		result.Subjects = append(result.Subjects, item)
	}
	if weight > 0 {
		result.Average = roundedPtr32(weighted / weight)
	}
	dynamics, err := s.queries.StudentWeeklyDynamics(ctx, db.StudentWeeklyDynamicsParams{StudentID: studentID, AcademicPeriodID: period.ID})
	if err != nil {
		return result, err
	}
	for _, row := range dynamics {
		if row.Average >= 0 {
			result.Dynamics = append(result.Dynamics, api.WeekAverage{WeekStart: apiDate(row.WeekStart), Average: float32(round(row.Average, 2)), GradesCount: int(row.GradesCount)})
		}
	}
	distribution, err := s.queries.StudentGradeDistribution(ctx, db.StudentGradeDistributionParams{StudentID: studentID, AcademicPeriodID: period.ID})
	if err != nil {
		return result, err
	}
	result.Distribution = valueCounts(distribution, func(row db.StudentGradeDistributionRow) (float64, int) { return row.Value, int(row.GradesCount) })
	attendance, err := s.queries.StudentAttendanceCounts(ctx, db.StudentAttendanceCountsParams{StudentID: studentID, AcademicPeriodID: period.ID})
	if err != nil {
		return result, err
	}
	var attended, counted int
	for _, row := range attendance {
		count := int(row.LessonsCount)
		switch row.Status {
		case "present":
			attended += count
			counted += count
		case "late":
			attended += count
			counted += count
			result.LateCount += count
		case "absent", "excused":
			counted += count
			result.Absences += count
		}
	}
	result.LessonsMarked = counted
	if rate := ratio(float64(attended), float64(counted)); rate != nil {
		value := float32(*rate)
		result.AttendanceRate = &value
	}
	return result, nil
}

func roundedPtr32(value float64) *float32 {
	rounded := float32(round(value, 2))
	return &rounded
}

// TeacherAnalytics собирает показатели по классам и предметам учителя.
func (s *Service) TeacherAnalytics(ctx context.Context, actor Principal, periodID *uuid.UUID) (api.TeacherAnalytics, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.TeacherAnalytics{}, err
	}
	teacherID := *actor.TeacherID
	today := s.schoolToday(ctx)
	period, err := s.resolvePeriod(ctx, periodID, today)
	if err != nil {
		return api.TeacherAnalytics{}, err
	}
	result := api.TeacherAnalytics{Classes: []api.TeacherClassStat{}, Distribution: []api.ValueCount{}, Dynamics: []api.WeekAverage{}, LowPerformers: []api.LowPerformer{}}
	result.PeriodId, result.PeriodName = periodFields(period)
	homework, err := s.queries.TeacherHomeworkCounts(ctx, db.TeacherHomeworkCountsParams{Today: today, UserID: actor.UserID})
	if err != nil {
		return result, err
	}
	result.DraftHomework, result.ActiveHomework = int(homework.Drafts), int(homework.Active)
	if period == nil {
		return result, nil
	}
	counts, err := s.queries.TeacherLessonCounts(ctx, db.TeacherLessonCountsParams{Today: today, TeacherID: teacherID, PeriodID: period.ID})
	if err != nil {
		return result, err
	}
	result.TodayLessons, result.UnmarkedPastLessons = int(counts.TodayLessons), int(counts.UnmarkedPastLessons)
	unfilled, err := s.queries.UnfilledLessonsReport(ctx, db.UnfilledLessonsReportParams{AcademicPeriodID: period.ID, Column2: teacherID})
	if err != nil {
		return result, err
	}
	result.UnfilledLessons = len(unfilled)
	classes, err := s.queries.TeacherClassStats(ctx, db.TeacherClassStatsParams{TeacherID: teacherID, AcademicPeriodID: period.ID})
	if err != nil {
		return result, err
	}
	for _, row := range classes {
		item := api.TeacherClassStat{ClassId: row.ClassID, ClassName: row.ClassName, SubjectId: row.SubjectID, SubjectName: row.SubjectName, Students: int(row.Students)}
		item.Average = float32PointerFrom(row.Average)
		item.AttendanceRate = float32PointerFrom(row.AttendanceRate)
		result.Classes = append(result.Classes, item)
	}
	distribution, err := s.queries.TeacherGradeDistribution(ctx, db.TeacherGradeDistributionParams{TeacherID: teacherID, AcademicPeriodID: period.ID})
	if err != nil {
		return result, err
	}
	result.Distribution = valueCounts(distribution, func(row db.TeacherGradeDistributionRow) (float64, int) { return row.Value, int(row.GradesCount) })
	dynamics, err := s.queries.TeacherWeeklyDynamics(ctx, db.TeacherWeeklyDynamicsParams{TeacherID: teacherID, AcademicPeriodID: period.ID})
	if err != nil {
		return result, err
	}
	for _, row := range dynamics {
		if row.Average >= 0 {
			result.Dynamics = append(result.Dynamics, api.WeekAverage{WeekStart: apiDate(row.WeekStart), Average: float32(round(row.Average, 2)), GradesCount: int(row.GradesCount)})
		}
	}
	low, err := s.queries.TeacherLowPerformers(ctx, db.TeacherLowPerformersParams{TeacherID: teacherID, AcademicPeriodID: period.ID, Limit: 5})
	if err != nil {
		return result, err
	}
	for _, row := range low {
		if row.Average >= 0 {
			result.LowPerformers = append(result.LowPerformers, api.LowPerformer{StudentId: row.StudentID, DisplayName: row.DisplayName, ClassName: row.ClassName,
				Average: float32(round(row.Average, 2)), GradesCount: int(row.GradesCount)})
		}
	}
	return result, nil
}

// float32PointerFrom переводит значение «-1 = нет данных» в nil.
func float32PointerFrom(value float64) *float32 {
	if value < 0 {
		return nil
	}
	rounded := float32(round(value, 4))
	return &rounded
}

// SchoolAnalytics собирает сводку по школе для администратора, системного администратора и руководителя.
func (s *Service) SchoolAnalytics(ctx context.Context, actor Principal, periodID *uuid.UUID) (api.SchoolAnalytics, error) {
	if !actor.canReadSchoolSchedule() {
		return api.SchoolAnalytics{}, apperr.Forbidden()
	}
	today := s.schoolToday(ctx)
	period, err := s.resolvePeriod(ctx, periodID, today)
	if err != nil {
		return api.SchoolAnalytics{}, err
	}
	result := api.SchoolAnalytics{UsersByRole: []api.RoleCount{}, AttendanceToday: []api.ClassAttendance{}, AttendanceTrend: []api.DayRate{},
		GradeByClass: []api.NamedAverage{}, GradeBySubject: []api.NamedAverage{}, Distribution: []api.ValueCount{}, Alerts: []api.SchoolAlert{}, RecentActivity: []api.ActivityItem{}}
	result.PeriodId, result.PeriodName = periodFields(period)
	counts, err := s.schoolCounts(ctx)
	if err != nil {
		return result, err
	}
	result.Counts = counts
	roles, err := s.queries.UsersByRole(ctx)
	if err != nil {
		return result, err
	}
	for _, row := range roles {
		result.UsersByRole = append(result.UsersByRole, api.RoleCount{Role: api.Role(row.Role), Count: int(row.UsersCount)})
	}
	attendance, err := s.queries.SchoolAttendanceByClass(ctx, today)
	if err != nil {
		return result, err
	}
	for _, row := range attendance {
		item := api.ClassAttendance{ClassId: row.ClassID, ClassName: row.ClassName, Students: int(row.Students), Present: int(row.Present), Absent: int(row.Absent)}
		if rate := ratio(float64(row.Present), float64(row.Present+row.Absent)); rate != nil {
			value := float32(*rate)
			item.Rate = &value
		}
		result.AttendanceToday = append(result.AttendanceToday, item)
	}
	trend, err := s.queries.SchoolAttendanceTrend(ctx, db.SchoolAttendanceTrendParams{FromDate: today.AddDate(0, 0, -13), ToDate: today})
	if err != nil {
		return result, err
	}
	for _, row := range trend {
		if row.Rate >= 0 {
			result.AttendanceTrend = append(result.AttendanceTrend, api.DayRate{Day: apiDate(row.Day), Rate: float32(round(row.Rate, 4))})
		}
	}
	if period != nil {
		byClass, err := s.queries.SchoolGradeAverageByClass(ctx, period.ID)
		if err != nil {
			return result, err
		}
		for _, row := range byClass {
			if row.Average >= 0 {
				result.GradeByClass = append(result.GradeByClass, api.NamedAverage{Id: row.ClassID, Name: row.ClassName, Average: float32(round(row.Average, 2)), GradesCount: int(row.GradesCount)})
			}
		}
		bySubject, err := s.queries.SchoolGradeAverageBySubject(ctx, period.ID)
		if err != nil {
			return result, err
		}
		for _, row := range bySubject {
			if row.Average >= 0 {
				result.GradeBySubject = append(result.GradeBySubject, api.NamedAverage{Id: row.SubjectID, Name: row.SubjectName, Average: float32(round(row.Average, 2)), GradesCount: int(row.GradesCount)})
			}
		}
		distribution, err := s.queries.SchoolGradeDistribution(ctx, period.ID)
		if err != nil {
			return result, err
		}
		result.Distribution = valueCounts(distribution, func(row db.SchoolGradeDistributionRow) (float64, int) { return row.Value, int(row.GradesCount) })
	}
	alerts, err := s.schoolAlerts(ctx, period != nil, counts)
	if err != nil {
		return result, err
	}
	result.Alerts = alerts
	activity, err := s.queries.RecentAuditWithActor(ctx, 8)
	if err != nil {
		return result, err
	}
	for _, row := range activity {
		item := api.ActivityItem{Id: row.ID, Action: row.Action, EntityType: row.EntityType, CreatedAt: row.CreatedAt}
		if row.ActorName.Valid {
			name := row.ActorName.String
			item.ActorName = &name
		}
		result.RecentActivity = append(result.RecentActivity, item)
	}
	return result, nil
}

func (s *Service) schoolCounts(ctx context.Context) (api.SchoolCounts, error) {
	row, err := s.queries.SchoolCounts(ctx)
	if err != nil {
		return api.SchoolCounts{}, err
	}
	return api.SchoolCounts{Students: int(row.Students), Teachers: int(row.Teachers), Parents: int(row.Parents), Classes: int(row.Classes), Subjects: int(row.Subjects),
		UsersActive: int(row.UsersActive), UsersInactive: int(row.UsersInactive), PendingPasswordChange: int(row.PendingPasswordChange),
		ActiveSessions: int(row.ActiveSessions), OpenInvitations: int(row.OpenInvitations)}, nil
}

// schoolAlerts превращает счётчики в понятные администратору задачи со ссылкой на нужный раздел.
func (s *Service) schoolAlerts(ctx context.Context, hasPeriod bool, counts api.SchoolCounts) ([]api.SchoolAlert, error) {
	row, err := s.queries.SchoolAlertCounts(ctx)
	if err != nil {
		return nil, err
	}
	alerts := []api.SchoolAlert{}
	add := func(kind, severity, title, text string, count int, link string) {
		if count <= 0 {
			return
		}
		alert := api.SchoolAlert{Kind: kind, Severity: api.SchoolAlertSeverity(severity), Title: title, Text: text, Count: count}
		if link != "" {
			alert.Link = &link
		}
		alerts = append(alerts, alert)
	}
	if !hasPeriod {
		alerts = append(alerts, api.SchoolAlert{Kind: "no_period", Severity: api.Danger, Title: "Не настроен учебный период",
			Text: "Создайте учебный год и периоды: без них журнал и расписание не работают."})
	}
	add("unenrolled_students", "warning", fmt.Sprintf("Учеников без класса: %d", row.UnenrolledStudents), "Зачислите их в классы, иначе они не увидят расписание и оценки.", int(row.UnenrolledStudents), "/admin?tab=people")
	add("classes_without_class_teacher", "warning", fmt.Sprintf("Классов без классного руководителя: %d", row.ClassesWithoutClassTeacher), "Назначьте классных руководителей в разделе «Назначения».", int(row.ClassesWithoutClassTeacher), "/admin?tab=assignments")
	add("teachers_without_assignments", "info", fmt.Sprintf("Учителей без нагрузки: %d", row.TeachersWithoutAssignments), "Назначьте им классы и предметы, чтобы они увидели журнал.", int(row.TeachersWithoutAssignments), "/admin?tab=assignments")
	add("parents_without_children", "info", fmt.Sprintf("Родителей без привязанных детей: %d", row.ParentsWithoutChildren), "Свяжите их с учениками.", int(row.ParentsWithoutChildren), "/admin?tab=assignments")
	add("never_signed_in", "info", fmt.Sprintf("Не входили в систему более недели: %d", row.NeverSignedIn), "Проверьте, что им передан временный пароль или приглашение.", int(row.NeverSignedIn), "/admin?tab=people")
	add("unmarked_past_lessons", "warning", fmt.Sprintf("Проведённых уроков без отметки: %d", row.UnmarkedPastLessons), "Учителя не отметили уроки как проведённые.", int(row.UnmarkedPastLessons), "/reports")
	add("pending_password_change", "info", fmt.Sprintf("Пользователей с временным паролем: %d", counts.PendingPasswordChange), "Они ещё не сменили пароль, выданный администратором.", counts.PendingPasswordChange, "/admin?tab=people")
	add("open_invitations", "info", fmt.Sprintf("Действующих приглашений: %d", counts.OpenInvitations), "Приглашения ещё не использованы.", counts.OpenInvitations, "/admin?tab=invitations")
	return alerts, nil
}
