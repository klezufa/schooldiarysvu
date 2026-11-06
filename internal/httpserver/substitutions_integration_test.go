package httpserver_test

import (
	"context"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	db "schooldiarysvu/internal/database/sqlc"
)

type substitutionDTO struct {
	ID                  uuid.UUID  `json:"id"`
	TeacherID           uuid.UUID  `json:"teacher_id"`
	TeacherName         string     `json:"teacher_name"`
	OriginalTeacherID   *uuid.UUID `json:"original_teacher_id"`
	OriginalTeacherName *string    `json:"original_teacher_name"`
	Reason              string     `json:"reason"`
}

// Замена на уроке: подбор свободных учителей (сначала ведущие предмет, затем знающие класс, затем остальные; занятые
// не предлагаются), назначение с уведомлениями, права заменяющего только на свой урок, повторная замена, учёт замен,
// отмена — и отказ отменить, если учитель по расписанию уже занят. Всё — только администратору.
func TestLessonSubstitutionLifecycle(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	ctx := context.Background()
	q := db.New(f.pool)
	admin := newAdmin(t, f)
	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)

	// Будущая четверть: уведомляют только о заменах уроков, которые ещё не прошли.
	day := time.Now().UTC().AddDate(0, 0, 7).Truncate(24 * time.Hour)
	period, err := q.CreateAcademicPeriod(ctx, db.CreateAcademicPeriodParams{AcademicYearID: f.yearID, Name: "Будущая",
		StartsOn: day.AddDate(0, 0, -7), EndsOn: day.AddDate(0, 0, 30)})
	mustTest(t, err)
	otherClass, err := q.CreateClass(ctx, db.CreateClassParams{AcademicYearID: f.yearID, Name: "6Б"})
	mustTest(t, err)
	thirdClass, err := q.CreateClass(ctx, db.CreateClassParams{AcademicYearID: f.yearID, Name: "7В"})
	mustTest(t, err)
	physics, err := q.CreateSubject(ctx, db.CreateSubjectParams{Name: "Физика"})
	mustTest(t, err)
	newTeacher := func(login, name string) (uuid.UUID, uuid.UUID) {
		created := createUser(t, admin, login, []string{"teacher"}, http.StatusCreated, map[string]any{"display_name": name})
		_, err := f.pool.Exec(ctx, "UPDATE users SET must_change_password = false WHERE id = $1", created.ID)
		mustTest(t, err)
		return created.ID, uuid.MustParse(*created.TeacherID)
	}
	assign := func(teacherID, classID, subjectID uuid.UUID) {
		_, err := q.CreateTeachingAssignment(ctx, db.CreateTeachingAssignmentParams{TeacherID: teacherID, ClassID: classID, SubjectID: subjectID,
			AcademicPeriodID: period.ID, StartsOn: period.StartsOn})
		mustTest(t, err)
	}
	_, mathTwo := newTeacher("math.two", "Вторая Математичка")
	_, physicist := newTeacher("physicist", "Физик Класса")
	_, busy := newTeacher("busy.teacher", "Занятой Учитель")
	var otherTeacher uuid.UUID
	mustTest(t, f.pool.QueryRow(ctx, "SELECT t.id FROM teachers t JOIN users u ON u.id = t.user_id WHERE u.login = 'other.teacher'").Scan(&otherTeacher))
	assign(f.teacherID, f.classID, f.subjectID)
	assign(f.teacherID, thirdClass.ID, f.subjectID)
	assign(mathTwo, otherClass.ID, f.subjectID)
	assign(physicist, f.classID, physics.ID)
	assign(busy, otherClass.ID, physics.ID)
	at := func(hour int) time.Time { return day.Add(time.Duration(hour) * time.Hour) }
	createLesson := func(teacherID, classID, subjectID uuid.UUID, hour int) uuid.UUID {
		var lesson struct {
			ID uuid.UUID `json:"id"`
		}
		decodeResponse(t, admin.request(http.MethodPost, "/api/v1/lessons", map[string]any{"lesson_date": day.Format(time.DateOnly),
			"starts_at": at(hour), "ends_at": at(hour).Add(45 * time.Minute), "class_id": classID, "subject_id": subjectID,
			"teacher_id": teacherID, "academic_period_id": period.ID}, http.StatusCreated), &lesson)
		return lesson.ID
	}
	lessonID := createLesson(f.teacherID, f.classID, f.subjectID, 7)
	laterLesson := createLesson(f.teacherID, f.classID, f.subjectID, 9)
	var busyLesson struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/lessons", map[string]any{"lesson_date": day.Format(time.DateOnly),
		"starts_at": at(7), "ends_at": at(7).Add(45 * time.Minute), "class_id": otherClass.ID, "subject_id": physics.ID,
		"teacher_id": busy, "academic_period_id": period.ID}, http.StatusCreated), &busyLesson)
	createLesson(physicist, f.classID, physics.ID, 11)

	teacher.request(http.MethodGet, "/api/v1/substitutions/candidates?lesson_id="+lessonID.String(), nil, http.StatusForbidden)
	var candidates []struct {
		TeacherID      uuid.UUID `json:"teacher_id"`
		DisplayName    string    `json:"display_name"`
		Reasons        []string  `json:"reasons"`
		LessonsThatDay int       `json:"lessons_that_day"`
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/substitutions/candidates?lesson_id="+lessonID.String(), nil, http.StatusOK), &candidates)
	if len(candidates) != 3 || candidates[0].TeacherID != mathTwo || strings.Join(candidates[0].Reasons, ",") != "teaches_subject" ||
		candidates[1].TeacherID != physicist || strings.Join(candidates[1].Reasons, ",") != "knows_class,at_school" || candidates[1].LessonsThatDay != 1 ||
		candidates[2].TeacherID != otherTeacher || len(candidates[2].Reasons) != 0 {
		t.Fatalf("candidates = %+v, want math teacher, class physicist, then the teacher without links; busy and own teacher excluded", candidates)
	}
	admin.request(http.MethodGet, "/api/v1/substitutions/candidates?lesson_id="+uuid.NewString(), nil, http.StatusNotFound)

	path := "/api/v1/lessons/" + lessonID.String() + "/substitution"
	teacher.request(http.MethodPut, path, map[string]any{"teacher_id": mathTwo}, http.StatusForbidden)
	admin.request(http.MethodPut, path, map[string]any{"teacher_id": f.teacherID}, http.StatusUnprocessableEntity)
	admin.request(http.MethodPut, path, map[string]any{"teacher_id": busy}, http.StatusConflict)
	admin.request(http.MethodPut, path, map[string]any{"teacher_id": uuid.New()}, http.StatusUnprocessableEntity)
	admin.request(http.MethodPut, path, map[string]any{"teacher_id": mathTwo, "reason": strings.Repeat("я", 301)}, http.StatusUnprocessableEntity)
	var substituted substitutionDTO
	decodeResponse(t, admin.request(http.MethodPut, path, map[string]any{"teacher_id": mathTwo, "reason": " Болезнь "}, http.StatusOK), &substituted)
	if substituted.TeacherID != mathTwo || substituted.OriginalTeacherID == nil || *substituted.OriginalTeacherID != f.teacherID ||
		substituted.OriginalTeacherName == nil || *substituted.OriginalTeacherName != "Учитель" || substituted.Reason != "Болезнь" {
		t.Fatalf("substitution = %+v", substituted)
	}
	scheduleNotifications := func(login string) []string {
		rows, err := f.pool.Query(ctx, `SELECT n.title FROM notifications n JOIN users u ON u.id = n.user_id
			WHERE u.login = $1 AND n.kind = 'schedule' ORDER BY n.created_at, n.title`, login)
		mustTest(t, err)
		defer rows.Close()
		var titles []string
		for rows.Next() {
			var title string
			mustTest(t, rows.Scan(&title))
			titles = append(titles, title)
		}
		return titles
	}
	if got := scheduleNotifications("math.two"); strings.Join(got, "|") != "Вам назначена замена" {
		t.Fatalf("substitute notifications = %v", got)
	}
	for _, login := range []string{"student", "parent"} {
		if got := scheduleNotifications(login); strings.Join(got, "|") != "Замена урока" {
			t.Fatalf("%s notifications = %v, want the class to learn about the substitution", login, got)
		}
	}

	// Заменяющий ведёт свой урок полностью, хотя этот предмет в классе не ведёт; чужой урок того же журнала — нет.
	substitute := newTestClient(t, f.server.URL)
	substitute.login("math.two", otherPassword)
	var lesson struct {
		Version             int     `json:"version"`
		OriginalTeacherName *string `json:"original_teacher_name"`
	}
	decodeResponse(t, substitute.request(http.MethodGet, "/api/v1/lessons/"+lessonID.String(), nil, http.StatusOK), &lesson)
	if lesson.OriginalTeacherName == nil || *lesson.OriginalTeacherName != "Учитель" {
		t.Fatalf("lesson original teacher = %v", lesson.OriginalTeacherName)
	}
	substitute.request(http.MethodPatch, "/api/v1/lessons/"+lessonID.String()+"?version="+strconv.Itoa(lesson.Version), map[string]any{"topic": "Самостоятельная работа", "status": "planned"}, http.StatusOK)
	substitute.request(http.MethodGet, "/api/v1/classes/"+f.classID.String()+"/students?date="+day.Format(time.DateOnly), nil, http.StatusOK)
	substitute.request(http.MethodPut, "/api/v1/attendance", map[string]any{"lesson_id": lessonID, "student_id": f.studentID, "status": "present"}, http.StatusOK)
	var work struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, substitute.request(http.MethodPost, "/api/v1/lesson-works", map[string]any{"lesson_id": lessonID, "work_type": "classwork",
		"title": "Самостоятельная", "weight": 1, "position": 1}, http.StatusCreated), &work)
	substitute.request(http.MethodPost, "/api/v1/grades", map[string]any{"student_id": f.studentID, "lesson_work_id": work.ID, "value": 4}, http.StatusCreated)
	substitute.request(http.MethodGet, "/api/v1/journal?class_id="+f.classID.String()+"&subject_id="+f.subjectID.String()+"&academic_period_id="+period.ID.String(), nil, http.StatusOK)
	var homework struct {
		ID      uuid.UUID `json:"id"`
		Version int       `json:"version"`
	}
	decodeResponse(t, substitute.request(http.MethodPost, "/api/v1/homework", map[string]any{"class_id": f.classID, "subject_id": f.subjectID,
		"lesson_id": lessonID, "text": "Повторить параграф", "due_date": day.AddDate(0, 0, 2).Format(time.DateOnly), "status": "draft"}, http.StatusCreated), &homework)
	substitute.request(http.MethodPatch, "/api/v1/homework/"+homework.ID.String()+"?version="+strconv.Itoa(homework.Version), map[string]any{"text": "Повторить параграф 5",
		"due_date": day.AddDate(0, 0, 2).Format(time.DateOnly)}, http.StatusOK)
	substitute.request(http.MethodPatch, "/api/v1/lessons/"+laterLesson.String()+"?version=1", map[string]any{"topic": "Чужой урок", "status": "planned"}, http.StatusNotFound)
	// Учитель по расписанию видит урок, который за него провели, но не меняет его.
	teacher.request(http.MethodGet, "/api/v1/lessons/"+lessonID.String(), nil, http.StatusOK)
	teacher.request(http.MethodPatch, "/api/v1/lessons/"+lessonID.String()+"?version=2", map[string]any{"topic": "Перехват", "status": "planned"}, http.StatusNotFound)

	// Учёт: все замены за день и уроки учителя — и свои, и те, на которых его заменили.
	from := "?from=" + day.Format(time.DateOnly) + "&to=" + day.Format(time.DateOnly)
	teacher.request(http.MethodGet, "/api/v1/substitutions"+from, nil, http.StatusForbidden)
	admin.request(http.MethodGet, "/api/v1/substitutions?from="+day.Format(time.DateOnly)+"&to="+day.AddDate(0, 3, 0).Format(time.DateOnly), nil, http.StatusUnprocessableEntity)
	var listed []substitutionDTO
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/substitutions"+from, nil, http.StatusOK), &listed)
	if len(listed) != 1 || listed[0].ID != lessonID {
		t.Fatalf("substitutions of the day = %+v", listed)
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/substitutions"+from+"&teacher_id="+f.teacherID.String(), nil, http.StatusOK), &listed)
	if len(listed) != 2 || listed[0].ID != lessonID || listed[0].TeacherID != mathTwo || listed[1].ID != laterLesson || listed[1].OriginalTeacherID != nil {
		t.Fatalf("teacher's lessons = %+v, want the substituted lesson and the own one", listed)
	}

	// Другой заменяющий: первый получает отмену, учитель по расписанию остаётся прежним.
	decodeResponse(t, admin.request(http.MethodPut, path, map[string]any{"teacher_id": physicist, "reason": "Болезнь"}, http.StatusOK), &substituted)
	if substituted.TeacherID != physicist || *substituted.OriginalTeacherID != f.teacherID {
		t.Fatalf("second substitution = %+v", substituted)
	}
	if got := scheduleNotifications("math.two"); strings.Join(got, "|") != "Вам назначена замена|Замена отменена" {
		t.Fatalf("previous substitute notifications = %v", got)
	}
	substitute.request(http.MethodGet, "/api/v1/lessons/"+lessonID.String(), nil, http.StatusNotFound)

	// Отмена: урок возвращается учителю по расписанию. Если он уже занят в это время — отказ.
	teacher.request(http.MethodDelete, path, nil, http.StatusForbidden)
	admin.request(http.MethodDelete, path, nil, http.StatusNoContent)
	var current struct {
		TeacherID           uuid.UUID `json:"teacher_id"`
		OriginalTeacherName *string   `json:"original_teacher_name"`
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/lessons/"+lessonID.String(), nil, http.StatusOK), &current)
	if current.TeacherID != f.teacherID || current.OriginalTeacherName != nil {
		t.Fatalf("lesson after removal = %+v", current)
	}
	admin.request(http.MethodDelete, path, nil, http.StatusNotFound)
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/substitutions"+from, nil, http.StatusOK), &listed)
	if len(listed) != 0 {
		t.Fatalf("substitutions after removal = %+v", listed)
	}
	admin.request(http.MethodPut, path, map[string]any{"teacher_id": mathTwo}, http.StatusOK)
	createLesson(f.teacherID, thirdClass.ID, f.subjectID, 7)
	admin.request(http.MethodDelete, path, nil, http.StatusConflict)

	var audited int
	mustTest(t, f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE entity_id = $1 AND action IN ('lesson.substituted', 'lesson.substitution_removed')", lessonID).Scan(&audited))
	if audited != 4 {
		t.Fatalf("substitution audit entries = %d, want 4", audited)
	}
}

var archiveCodePattern = regexp.MustCompile(`^[0-9A-F]{4}(-[0-9A-F]{4}){3}$`)

// Архив журнала класса за год: оценки и пропуски по урокам, темы, задания, замены, итоговые оценки и выбывшие ученики;
// выдают его администратор, руководитель и классный руководитель. По коду с распечатки руководитель проверяет, что
// архив выдан системой. После закрытия периодов архив помечен окончательным.
func TestJournalArchiveIssueAndVerify(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	ctx := context.Background()
	q := db.New(f.pool)
	admin := newAdmin(t, f)
	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)
	principal := newTestClient(t, f.server.URL)
	principal.login("principal", otherPassword)
	student := newTestClient(t, f.server.URL)
	student.login("student", studentPassword)

	first := createTestLesson(t, f, teacher, "2026-09-15")
	second := createTestLesson(t, f, teacher, "2026-09-16")
	var work struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, teacher.request(http.MethodPost, "/api/v1/lesson-works", map[string]any{"lesson_id": first, "work_type": "test",
		"title": "Контрольная", "weight": 2, "position": 1}, http.StatusCreated), &work)
	teacher.request(http.MethodPost, "/api/v1/grades", map[string]any{"student_id": f.studentID, "lesson_work_id": work.ID, "value": 5}, http.StatusCreated)
	teacher.request(http.MethodPut, "/api/v1/attendance", map[string]any{"lesson_id": second, "student_id": f.studentID, "status": "absent"}, http.StatusOK)
	teacher.request(http.MethodPost, "/api/v1/homework", map[string]any{"class_id": f.classID, "subject_id": f.subjectID, "lesson_id": first,
		"text": "Задачи 1–5", "due_date": "2026-09-16", "status": "published"}, http.StatusCreated)
	teacher.request(http.MethodPost, "/api/v1/term-grades", map[string]any{"student_id": f.studentID, "subject_id": f.subjectID,
		"academic_period_id": f.periodID, "value": 5}, http.StatusOK)
	var otherTeacher uuid.UUID
	mustTest(t, f.pool.QueryRow(ctx, "SELECT t.id FROM teachers t JOIN users u ON u.id = t.user_id WHERE u.login = 'other.teacher'").Scan(&otherTeacher))
	admin.request(http.MethodPut, "/api/v1/lessons/"+second.String()+"/substitution", map[string]any{"teacher_id": otherTeacher, "reason": "Курсы"}, http.StatusOK)
	_, err := q.CreateEnrollment(ctx, db.CreateEnrollmentParams{StudentID: f.otherStudentID, ClassID: f.classID,
		StartsOn: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)})
	mustTest(t, err)
	_, err = f.pool.Exec(ctx, "UPDATE enrollments SET ends_on = '2026-09-20' WHERE student_id = $1", f.otherStudentID)
	mustTest(t, err)
	// Перевод в следующий класс после конца года — не выбытие.
	_, err = f.pool.Exec(ctx, "UPDATE enrollments SET ends_on = '2027-09-01' WHERE student_id = $1", f.studentID)
	mustTest(t, err)

	body := map[string]any{"class_id": f.classID}
	student.request(http.MethodPost, "/api/v1/journal-archives", body, http.StatusNotFound)
	teacher.request(http.MethodPost, "/api/v1/journal-archives", body, http.StatusNotFound)
	admin.request(http.MethodPost, "/api/v1/journal-archives", map[string]any{"class_id": uuid.New()}, http.StatusNotFound)
	type archiveDTO struct {
		Code        string `json:"code"`
		Digest      string `json:"digest"`
		GeneratedBy string `json:"generated_by"`
		ClassName   string `json:"class_name"`
		YearName    string `json:"year_name"`
		Final       bool   `json:"final"`
		Periods     []struct {
			Closed bool `json:"closed"`
		} `json:"periods"`
		Students []struct {
			DisplayName string  `json:"display_name"`
			LeftOn      *string `json:"left_on"`
		} `json:"students"`
		Subjects []struct {
			SubjectName string   `json:"subject_name"`
			Teachers    []string `json:"teachers"`
			Lessons     []struct {
				ID            uuid.UUID `json:"id"`
				Topic         string    `json:"topic"`
				TeacherName   string    `json:"teacher_name"`
				SubstituteFor *string   `json:"substitute_for"`
				Homework      string    `json:"homework"`
			} `json:"lessons"`
			Marks []struct {
				LessonID   uuid.UUID `json:"lesson_id"`
				Grades     []float64 `json:"grades"`
				Attendance *string   `json:"attendance"`
			} `json:"marks"`
			TermGrades []struct {
				Value float64 `json:"value"`
			} `json:"term_grades"`
		} `json:"subjects"`
	}
	var archive archiveDTO
	decodeResponse(t, principal.request(http.MethodPost, "/api/v1/journal-archives", body, http.StatusCreated), &archive)
	if !archiveCodePattern.MatchString(archive.Code) || len(archive.Digest) != 64 || !strings.HasPrefix(strings.ToUpper(archive.Digest), strings.ReplaceAll(archive.Code, "-", "")) {
		t.Fatalf("archive code %q / digest %q", archive.Code, archive.Digest)
	}
	if archive.ClassName != "5А" || archive.YearName != "2026/2027" || archive.GeneratedBy != "Руководитель" || archive.Final || len(archive.Periods) != 1 {
		t.Fatalf("archive header = %+v", archive)
	}
	if len(archive.Students) != 2 || archive.Students[0].DisplayName != "Другой ученик" || archive.Students[0].LeftOn == nil ||
		*archive.Students[0].LeftOn != "2026-09-20" || archive.Students[1].LeftOn != nil {
		t.Fatalf("archive students = %+v, want the student who left to stay with the date", archive.Students)
	}
	if len(archive.Subjects) != 1 {
		t.Fatalf("archive subjects = %+v", archive.Subjects)
	}
	subject := archive.Subjects[0]
	if subject.SubjectName != "Математика" || strings.Join(subject.Teachers, ",") != "Учитель" || len(subject.Lessons) != 2 ||
		subject.Lessons[0].ID != first || subject.Lessons[0].Topic != "Проценты" || subject.Lessons[0].Homework != "Задачи 1–5" ||
		subject.Lessons[1].TeacherName != "Другой учитель" || subject.Lessons[1].SubstituteFor == nil || *subject.Lessons[1].SubstituteFor != "Учитель" {
		t.Fatalf("archive lessons = %+v", subject)
	}
	if len(subject.Marks) != 2 || subject.Marks[0].LessonID != first || len(subject.Marks[0].Grades) != 1 || subject.Marks[0].Grades[0] != 5 ||
		subject.Marks[1].LessonID != second || subject.Marks[1].Attendance == nil || *subject.Marks[1].Attendance != "absent" {
		t.Fatalf("archive marks = %+v", subject.Marks)
	}
	if len(subject.TermGrades) != 1 || subject.TermGrades[0].Value != 5 {
		t.Fatalf("archive term grades = %+v", subject.TermGrades)
	}

	// Классный руководитель класса тоже выдаёт архив; классный руководитель другого класса — нет.
	classTeacher := createUser(t, admin, "class.head", []string{"teacher", "class_teacher"}, http.StatusCreated, map[string]any{"display_name": "Классный Руководитель"})
	_, err = f.pool.Exec(ctx, "UPDATE users SET must_change_password = false WHERE id = $1", classTeacher.ID)
	mustTest(t, err)
	head := newTestClient(t, f.server.URL)
	head.login("class.head", otherPassword)
	head.request(http.MethodPost, "/api/v1/journal-archives", body, http.StatusNotFound)
	admin.request(http.MethodPost, "/api/v1/class-teachers", map[string]any{"teacher_id": *classTeacher.TeacherID, "class_id": f.classID, "starts_on": "2026-09-01"}, http.StatusNoContent)
	head.request(http.MethodPost, "/api/v1/journal-archives", body, http.StatusCreated)

	verify := func(client *testClient, code string, expected int) {
		client.request(http.MethodGet, "/api/v1/journal-archives/verify?code="+code, nil, expected)
	}
	verify(teacher, archive.Code, http.StatusForbidden)
	verify(principal, "0000-0000-0000-0000", http.StatusNotFound)
	verify(principal, "not-a-code", http.StatusNotFound)
	var verified struct {
		Code        string `json:"code"`
		Digest      string `json:"digest"`
		ClassName   string `json:"class_name"`
		GeneratedBy string `json:"generated_by"`
	}
	typed := strings.ToLower(strings.ReplaceAll(archive.Code, "-", ""))
	decodeResponse(t, principal.request(http.MethodGet, "/api/v1/journal-archives/verify?code="+typed, nil, http.StatusOK), &verified)
	if verified.Code != archive.Code || verified.Digest != archive.Digest || verified.ClassName != "5А" || verified.GeneratedBy != "Руководитель" {
		t.Fatalf("verification = %+v", verified)
	}

	newSystemAdmin(t, f).request(http.MethodPost, "/api/v1/academic-periods/"+f.periodID.String()+"/close", nil, http.StatusNoContent)
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/journal-archives", body, http.StatusCreated), &archive)
	if !archive.Final || !archive.Periods[0].Closed || archive.GeneratedBy != "Администратор" {
		t.Fatalf("archive after closing the period = final %v, periods %+v", archive.Final, archive.Periods)
	}
	var issued int
	mustTest(t, f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action = 'journal_archive.issued' AND entity_id = $1", f.classID).Scan(&issued))
	if issued != 3 {
		t.Fatalf("issued archives in audit = %d, want 3", issued)
	}
}
