package httpserver_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"github.com/google/uuid"
)

// createTestLesson — урок учителя фикстуры в открытой I четверти.
func createTestLesson(t *testing.T, f *fixture, teacher *testClient, date string) uuid.UUID {
	t.Helper()
	var lesson struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, teacher.request(http.MethodPost, "/api/v1/lessons", map[string]any{"lesson_date": date, "starts_at": date + "T10:00:00Z", "ends_at": date + "T10:45:00Z",
		"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID, "academic_period_id": f.periodID, "topic": "Проценты"}, http.StatusCreated), &lesson)
	return lesson.ID
}

// Черновик задания виден только учителю; правка черновика, публикация и уведомление ученику и родителю; повторная
// публикация и устаревшая версия — конфликт. Затем уведомление отмечается прочитанным, настройки уведомлений сохраняются.
func TestHomeworkDraftEditPublishAndNotifications(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)
	student := newTestClient(t, f.server.URL)
	student.login("student", studentPassword)
	parent := newTestClient(t, f.server.URL)
	parent.login("parent", parentPassword)
	otherTeacher := newTestClient(t, f.server.URL)
	otherTeacher.login("other.teacher", otherPassword)
	lessonID := createTestLesson(t, f, teacher, "2026-09-15")

	var draft struct {
		ID      uuid.UUID `json:"id"`
		Status  string    `json:"status"`
		Version int       `json:"version"`
		Text    string    `json:"text"`
	}
	decodeResponse(t, teacher.request(http.MethodPost, "/api/v1/homework", map[string]any{"class_id": f.classID, "subject_id": f.subjectID,
		"lesson_id": lessonID, "text": "Черновик", "due_date": "2026-09-22", "status": "draft"}, http.StatusCreated), &draft)
	if draft.Status != "draft" {
		t.Fatalf("created homework status = %q, want draft", draft.Status)
	}
	studentHomework := func() []map[string]any {
		var items []map[string]any
		decodeResponse(t, student.request(http.MethodGet, "/api/v1/homework", nil, http.StatusOK), &items)
		return items
	}
	for _, item := range studentHomework() {
		if item["id"] == draft.ID.String() {
			t.Fatal("student sees an unpublished draft")
		}
	}

	editPath := fmt.Sprintf("/api/v1/homework/%s?version=%d", draft.ID, draft.Version)
	otherTeacher.request(http.MethodPatch, editPath, map[string]any{"text": "Чужая правка", "due_date": "2026-09-22"}, http.StatusNotFound)
	student.request(http.MethodPatch, editPath, map[string]any{"text": "Правка ученика", "due_date": "2026-09-22"}, http.StatusForbidden)
	decodeResponse(t, teacher.request(http.MethodPatch, editPath, map[string]any{"text": "Упражнение 25", "due_date": "2026-09-23"}, http.StatusOK), &draft)
	if draft.Text != "Упражнение 25" || draft.Version != 2 {
		t.Fatalf("edited draft = %+v, want new text and version 2", draft)
	}
	teacher.request(http.MethodPatch, editPath, map[string]any{"text": "Устаревшая правка", "due_date": "2026-09-23"}, http.StatusConflict)

	publishPath := fmt.Sprintf("/api/v1/homework/%s/publish?version=%d", draft.ID, draft.Version)
	otherTeacher.request(http.MethodPost, publishPath, nil, http.StatusNotFound)
	decodeResponse(t, teacher.request(http.MethodPost, publishPath, nil, http.StatusOK), &draft)
	if draft.Status != "published" {
		t.Fatalf("published homework status = %q", draft.Status)
	}
	teacher.request(http.MethodPost, publishPath, nil, http.StatusConflict)
	published := false
	for _, item := range studentHomework() {
		published = published || item["id"] == draft.ID.String()
	}
	if !published {
		t.Fatal("student does not see the published homework")
	}

	var notifications []struct {
		ID       uuid.UUID  `json:"id"`
		Kind     string     `json:"kind"`
		EntityID *uuid.UUID `json:"entity_id"`
		ReadAt   *string    `json:"read_at"`
	}
	homeworkNotification := func(client *testClient) (uuid.UUID, *string) {
		decodeResponse(t, client.request(http.MethodGet, "/api/v1/notifications", nil, http.StatusOK), &notifications)
		for _, item := range notifications {
			if item.Kind == "homework" && item.EntityID != nil && *item.EntityID == draft.ID {
				return item.ID, item.ReadAt
			}
		}
		t.Fatalf("no homework notification in %+v", notifications)
		return uuid.Nil, nil
	}
	studentNotification, readAt := homeworkNotification(student)
	if readAt != nil {
		t.Fatal("new notification is already read")
	}
	homeworkNotification(parent)
	parent.request(http.MethodPost, "/api/v1/notifications/"+studentNotification.String()+"/read", nil, http.StatusNotFound)
	student.request(http.MethodPost, "/api/v1/notifications/"+studentNotification.String()+"/read", nil, http.StatusNoContent)
	if _, readAt = homeworkNotification(student); readAt == nil {
		t.Fatal("notification is not marked as read")
	}

	student.request(http.MethodPut, "/api/v1/notification-preferences", map[string]any{"internal_enabled": true, "digest_frequency": "weekly", "quiet_from": "22:00", "quiet_to": "07:00"}, http.StatusOK)
	student.request(http.MethodPut, "/api/v1/notification-preferences", map[string]any{"internal_enabled": true, "digest_frequency": "hourly"}, http.StatusUnprocessableEntity)
	var frequency, quietFrom string
	mustTest(t, f.pool.QueryRow(context.Background(), `SELECT np.digest_frequency, to_char(np.quiet_from, 'HH24:MI') FROM notification_preferences np
		JOIN students s ON s.user_id = np.user_id WHERE s.id = $1`, f.studentID).Scan(&frequency, &quietFrom))
	if frequency != "weekly" || quietFrom != "22:00" {
		t.Fatalf("saved preferences = %s, %s", frequency, quietFrom)
	}
}

// Итоговая оценка: ставит только учитель назначения, значение — по шкале, повторное сохранение меняет её, а не
// создаёт вторую; ученик получает уведомление. Работы урока видят учитель назначения, не чужой учитель.
func TestTermGradesAndLessonWorks(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)
	otherTeacher := newTestClient(t, f.server.URL)
	otherTeacher.login("other.teacher", otherPassword)
	student := newTestClient(t, f.server.URL)
	student.login("student", studentPassword)
	lessonID := createTestLesson(t, f, teacher, "2026-09-16")

	var lesson struct {
		ID    uuid.UUID `json:"id"`
		Topic string    `json:"topic"`
	}
	decodeResponse(t, teacher.request(http.MethodGet, "/api/v1/lessons/"+lessonID.String(), nil, http.StatusOK), &lesson)
	if lesson.Topic != "Проценты" {
		t.Fatalf("lesson = %+v", lesson)
	}
	otherTeacher.request(http.MethodGet, "/api/v1/lessons/"+lessonID.String(), nil, http.StatusNotFound)
	teacher.request(http.MethodPost, "/api/v1/lesson-works", map[string]any{"lesson_id": lessonID, "work_type": "test", "title": "Контрольная", "weight": 3, "position": 1}, http.StatusCreated)
	var works []struct {
		Title  string  `json:"title"`
		Weight float64 `json:"weight"`
	}
	decodeResponse(t, teacher.request(http.MethodGet, "/api/v1/lessons/"+lessonID.String()+"/works", nil, http.StatusOK), &works)
	if len(works) != 1 || works[0].Title != "Контрольная" || works[0].Weight != 3 {
		t.Fatalf("lesson works = %+v", works)
	}
	otherTeacher.request(http.MethodGet, "/api/v1/lessons/"+lessonID.String()+"/works", nil, http.StatusNotFound)
	var workID uuid.UUID
	mustTest(t, f.pool.QueryRow(context.Background(), "SELECT id FROM lesson_works WHERE lesson_id = $1", lessonID).Scan(&workID))
	var grade struct {
		ID      uuid.UUID `json:"id"`
		Version int       `json:"version"`
	}
	decodeResponse(t, teacher.request(http.MethodPost, "/api/v1/grades", map[string]any{"student_id": f.studentID, "lesson_work_id": workID, "value": 3}, http.StatusCreated), &grade)
	journalPath := fmt.Sprintf("/api/v1/journal?class_id=%s&subject_id=%s&academic_period_id=%s", f.classID, f.subjectID, f.periodID)
	journalGrade := func() *float64 {
		var rows []struct {
			StudentID  uuid.UUID `json:"student_id"`
			WorkTitle  string    `json:"work_title"`
			GradeValue *float64  `json:"grade_value"`
		}
		decodeResponse(t, teacher.request(http.MethodGet, journalPath, nil, http.StatusOK), &rows)
		for _, row := range rows {
			if row.StudentID == f.studentID && row.WorkTitle == "Контрольная" {
				return row.GradeValue
			}
		}
		t.Fatalf("journal has no row for the test work: %+v", rows)
		return nil
	}
	if value := journalGrade(); value == nil || *value != 3 {
		t.Fatalf("journal grade = %v, want 3", value)
	}
	otherTeacher.request(http.MethodGet, journalPath, nil, http.StatusNotFound)
	student.request(http.MethodGet, journalPath, nil, http.StatusNotFound)
	// Ошибочная оценка удаляется (архивируется) по актуальной версии и исчезает из журнала.
	gradePath := fmt.Sprintf("/api/v1/grades/%s?version=%d", grade.ID, grade.Version)
	otherTeacher.request(http.MethodDelete, gradePath, nil, http.StatusNotFound)
	teacher.request(http.MethodDelete, fmt.Sprintf("/api/v1/grades/%s?version=%d", grade.ID, grade.Version+5), nil, http.StatusConflict)
	teacher.request(http.MethodDelete, gradePath, nil, http.StatusNoContent)
	if value := journalGrade(); value != nil {
		t.Fatalf("deleted grade is still in the journal: %v", *value)
	}
	// Посещаемость урока видят учителя, но не ученики.
	teacher.request(http.MethodPut, "/api/v1/attendance", map[string]any{"lesson_id": lessonID, "student_id": f.studentID, "status": "late"}, http.StatusOK)
	var attendance []struct {
		StudentID uuid.UUID `json:"student_id"`
		Status    string    `json:"status"`
	}
	decodeResponse(t, teacher.request(http.MethodGet, "/api/v1/attendance?lesson_id="+lessonID.String(), nil, http.StatusOK), &attendance)
	if len(attendance) != 1 || attendance[0].Status != "late" {
		t.Fatalf("attendance = %+v", attendance)
	}
	student.request(http.MethodGet, "/api/v1/attendance?lesson_id="+lessonID.String(), nil, http.StatusForbidden)
	student.request(http.MethodGet, "/api/v1/schedule/current", nil, http.StatusOK)

	term := map[string]any{"student_id": f.studentID, "subject_id": f.subjectID, "academic_period_id": f.periodID, "value": 4}
	student.request(http.MethodPost, "/api/v1/term-grades", term, http.StatusForbidden)
	otherTeacher.request(http.MethodPost, "/api/v1/term-grades", term, http.StatusNotFound)
	teacher.request(http.MethodPost, "/api/v1/term-grades", map[string]any{"student_id": f.studentID, "subject_id": f.subjectID, "academic_period_id": f.periodID, "value": 9}, http.StatusUnprocessableEntity)
	teacher.request(http.MethodPost, "/api/v1/term-grades", map[string]any{"student_id": f.otherStudentID, "subject_id": f.subjectID, "academic_period_id": f.periodID, "value": 4}, http.StatusNotFound)
	var saved struct {
		ID      uuid.UUID `json:"id"`
		Value   float64   `json:"value"`
		Version int       `json:"version"`
	}
	decodeResponse(t, teacher.request(http.MethodPost, "/api/v1/term-grades", term, http.StatusOK), &saved)
	first := saved
	term["value"] = 5
	decodeResponse(t, teacher.request(http.MethodPost, "/api/v1/term-grades", term, http.StatusOK), &saved)
	if saved.ID != first.ID || saved.Value != 5 || saved.Version <= first.Version {
		t.Fatalf("term grade upsert: first %+v, then %+v", first, saved)
	}
	var count, notified int
	mustTest(t, f.pool.QueryRow(context.Background(), "SELECT count(*) FROM term_grades WHERE student_id = $1", f.studentID).Scan(&count))
	mustTest(t, f.pool.QueryRow(context.Background(), `SELECT count(*) FROM notifications n JOIN students s ON s.user_id = n.user_id
		WHERE s.id = $1 AND n.entity_type = 'term_grade'`, f.studentID).Scan(&notified))
	if count != 1 || notified != 2 {
		t.Fatalf("term grades = %d (want 1), notifications = %d (want 2)", count, notified)
	}
}
