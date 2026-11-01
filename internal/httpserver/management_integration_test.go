package httpserver_test

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestManagementAndReportsEndToEnd(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin := newTestClient(t, f.server.URL)
	admin.login("admin", adminInitialPassword)
	admin.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": adminInitialPassword, "new_password": adminNewPassword}, http.StatusNoContent)
	admin.refreshCSRF()
	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)
	student := newTestClient(t, f.server.URL)
	student.login("student", studentPassword)

	admin.request(http.MethodPatch, "/api/v1/rooms/"+f.roomID.String(), map[string]string{"name": "102"}, http.StatusOK)
	sysadmin := newSystemAdmin(t, f)
	admin.request(http.MethodPut, "/api/v1/school/settings", map[string]string{"school_name": "Школа № 1", "timezone": "Europe/Moscow"}, http.StatusForbidden)
	admin.request(http.MethodPost, "/api/v1/grade-scales", map[string]any{"name": "Чужая шкала", "allowed_values": []int{2, 3}}, http.StatusForbidden)
	sysadmin.request(http.MethodPut, "/api/v1/school/settings", map[string]string{"school_name": "Школа № 1", "timezone": "Europe/Moscow"}, http.StatusOK)
	sysadmin.request(http.MethodPut, "/api/v1/school/settings", map[string]string{"school_name": "Школа", "timezone": "Invalid/Zone"}, http.StatusUnprocessableEntity)
	scaleBody := sysadmin.request(http.MethodPost, "/api/v1/grade-scales", map[string]any{"name": "Тестовая шкала", "allowed_values": []int{2, 3, 4, 5}}, http.StatusCreated)
	var scale struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, scaleBody, &scale)
	sysadmin.request(http.MethodPatch, "/api/v1/grade-scales/"+scale.ID.String(), map[string]any{"name": "Тестовая шкала", "allowed_values": []int{2, 3, 4, 5}, "is_active": true}, http.StatusOK)
	admin.request(http.MethodGet, "/api/v1/grade-scales", nil, http.StatusOK)

	schedule := map[string]any{"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID, "room_id": f.roomID,
		"academic_period_id": f.periodID, "weekday": 2, "lesson_number": 1, "starts_at": "08:00", "ends_at": "08:45",
		"valid_from": "2026-09-01", "valid_to": "2026-10-31", "week_parity": 0}
	admin.request(http.MethodPost, "/api/v1/schedule-entries", schedule, http.StatusCreated)
	admin.request(http.MethodPost, "/api/v1/schedule-entries", schedule, http.StatusConflict)
	teacher.request(http.MethodGet, "/api/v1/schedule-entries?week_start=2026-09-01", nil, http.StatusOK)
	student.request(http.MethodGet, "/api/v1/schedule-entries?week_start=2026-09-01", nil, http.StatusOK)

	lessonBody := teacher.request(http.MethodPost, "/api/v1/lessons", map[string]any{"lesson_date": "2026-09-01", "starts_at": "2026-09-01T09:00:00Z", "ends_at": "2026-09-01T09:45:00Z",
		"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID, "room_id": f.roomID, "academic_period_id": f.periodID, "topic": "Исходная тема"}, http.StatusCreated)
	var lesson struct {
		ID      uuid.UUID `json:"id"`
		Version int       `json:"version"`
	}
	decodeResponse(t, lessonBody, &lesson)
	teacher.request(http.MethodPatch, fmt.Sprintf("/api/v1/lessons/%s?version=%d", lesson.ID, lesson.Version), map[string]any{
		"lesson_date": "2026-09-02", "starts_at": "2026-09-02T10:00:00Z", "ends_at": "2026-09-02T10:45:00Z",
		"teacher_id": f.teacherID, "room_id": f.roomID, "topic": "Перенесённый урок", "status": "planned"}, http.StatusOK)

	homeworkBody := teacher.request(http.MethodPost, "/api/v1/homework", map[string]any{"class_id": f.classID, "subject_id": f.subjectID,
		"lesson_id": lesson.ID, "text": "Упражнение", "due_date": "2026-09-08", "status": "published"}, http.StatusCreated)
	var homework struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, homeworkBody, &homework)
	fileID := uploadTextFile(t, teacher, homework.ID, "task.txt", "Текст задания")
	copyBody := teacher.request(http.MethodPost, "/api/v1/homework/"+homework.ID.String()+"/copy", map[string]any{"due_date": "2026-09-09", "status": "published", "student_ids": []string{f.studentID.String()}}, http.StatusCreated)
	var copied struct {
		ID    uuid.UUID `json:"id"`
		Files []struct {
			ID uuid.UUID `json:"id"`
		} `json:"files"`
	}
	decodeResponse(t, copyBody, &copied)
	if len(copied.Files) != 1 || copied.Files[0].ID == fileID {
		t.Fatalf("copied homework files = %#v", copied.Files)
	}
	teacher.request(http.MethodDelete, "/api/v1/files/"+fileID.String(), nil, http.StatusNoContent)
	student.request(http.MethodGet, "/api/v1/files/"+fileID.String(), nil, http.StatusNotFound)
	student.request(http.MethodGet, "/api/v1/files/"+copied.Files[0].ID.String(), nil, http.StatusOK)

	announcementBody := teacher.request(http.MethodPost, "/api/v1/announcements", map[string]any{"title": "Важно", "body": "Объявление классу", "audience_type": "class", "class_id": f.classID}, http.StatusCreated)
	var announcement struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, announcementBody, &announcement)
	teacher.request(http.MethodDelete, "/api/v1/announcements/"+announcement.ID.String(), nil, http.StatusNoContent)
	teacher.request(http.MethodGet, "/api/v1/notifications/digest?frequency=daily", nil, http.StatusOK)
	teacher.request(http.MethodGet, "/api/v1/notifications/digest?frequency=hourly", nil, http.StatusUnprocessableEntity)

	admin.request(http.MethodPost, "/api/v1/class-teachers", map[string]any{"teacher_id": f.teacherID, "class_id": f.classID, "starts_on": "2026-09-01"}, http.StatusNoContent)
	assignmentsBody := admin.request(http.MethodGet, "/api/v1/class-teachers", nil, http.StatusOK)
	var assignments []struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, assignmentsBody, &assignments)
	if len(assignments) != 1 {
		t.Fatalf("class teacher assignments = %d", len(assignments))
	}
	admin.request(http.MethodPatch, "/api/v1/class-teachers/"+assignments[0].ID.String(), map[string]string{"ends_on": "2026-10-31"}, http.StatusNoContent)
	admin.request(http.MethodGet, "/api/v1/parent-student", nil, http.StatusOK)

	var enrollmentID uuid.UUID
	if err := f.pool.QueryRow(context.Background(), "SELECT id FROM enrollments WHERE student_id = $1", f.studentID).Scan(&enrollmentID); err != nil {
		t.Fatal(err)
	}
	admin.request(http.MethodPatch, "/api/v1/enrollments/"+enrollmentID.String(), map[string]string{"ends_on": "2026-10-31"}, http.StatusOK)
	// Список зачислений с именами: только администратор, с фильтром по классу.
	var enrollments []struct {
		ID          string  `json:"id"`
		StudentName *string `json:"student_name"`
		ClassName   *string `json:"class_name"`
		EndsOn      *string `json:"ends_on"`
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/enrollments?class_id="+f.classID.String(), nil, http.StatusOK), &enrollments)
	var found bool
	for _, enrollment := range enrollments {
		if enrollment.ID == enrollmentID.String() {
			found = true
			if enrollment.StudentName == nil || *enrollment.StudentName == "" || enrollment.ClassName == nil || enrollment.EndsOn == nil {
				t.Fatalf("enrollment without names or end date: %+v", enrollment)
			}
		}
	}
	if !found {
		t.Fatalf("enrollment %s is missing from the class list (%d rows)", enrollmentID, len(enrollments))
	}
	teacher.request(http.MethodGet, "/api/v1/enrollments", nil, http.StatusForbidden)

	// Деактивированный пользователь исчезает из списка, но список остаётся рабочим для остальных (регрессия: раньше 404 на весь список).
	tempBody := admin.request(http.MethodPost, "/api/v1/users", map[string]any{"login": "temp.deactivated", "password": "Temp-user-password-123", "display_name": "Временный Пользователь", "roles": []string{"teacher"}}, http.StatusCreated)
	var tempUser struct {
		ID string `json:"id"`
	}
	decodeResponse(t, tempBody, &tempUser)
	admin.request(http.MethodDelete, "/api/v1/users/"+tempUser.ID, nil, http.StatusNoContent)
	var activeUsers []struct {
		ID    string `json:"id"`
		Login string `json:"login"`
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/users?limit=100", nil, http.StatusOK), &activeUsers)
	if len(activeUsers) == 0 {
		t.Fatal("users list is empty after deactivating one user")
	}
	for _, user := range activeUsers {
		if user.ID == tempUser.ID {
			t.Fatalf("deactivated user %s is still listed", user.Login)
		}
	}
	newTestClient(t, f.server.URL).request(http.MethodPost, "/api/v1/auth/login", map[string]string{"login": "temp.deactivated", "password": "Temp-user-password-123"}, http.StatusUnauthorized)
	admin.request(http.MethodGet, "/api/v1/enrollments?class_id=not-a-uuid", nil, http.StatusBadRequest)
	student.request(http.MethodGet, fmt.Sprintf("/api/v1/reports/students/%s?academic_period_id=%s", f.studentID, f.periodID), nil, http.StatusOK)

	principal := newTestClient(t, f.server.URL)
	principal.login("principal", otherPassword)
	principal.request(http.MethodGet, fmt.Sprintf("/api/v1/reports/classes/%s/attendance?academic_period_id=%s", f.classID, f.periodID), nil, http.StatusOK)
	principal.request(http.MethodGet, fmt.Sprintf("/api/v1/reports/subjects/%s?class_id=%s&academic_period_id=%s", f.subjectID, f.classID, f.periodID), nil, http.StatusOK)
	principal.request(http.MethodGet, "/api/v1/reports/unfilled?academic_period_id="+f.periodID.String(), nil, http.StatusOK)
	// Учитель видит только собственные незаполненные уроки; ученик отчёт не получает.
	teacher.request(http.MethodGet, "/api/v1/reports/unfilled?academic_period_id="+f.periodID.String(), nil, http.StatusOK)
	student.request(http.MethodGet, "/api/v1/reports/unfilled?academic_period_id="+f.periodID.String(), nil, http.StatusForbidden)

	auditBody := admin.request(http.MethodGet, "/api/v1/audit?action=homework.copied&from=2020-01-01T00:00:00Z&to=2099-01-01T00:00:00Z", nil, http.StatusOK)
	var audit []any
	decodeResponse(t, auditBody, &audit)
	if len(audit) != 1 {
		t.Fatalf("filtered audit entries = %d, want 1", len(audit))
	}
	admin.request(http.MethodDelete, "/api/v1/rooms/"+f.roomID.String(), nil, http.StatusNoContent)
	sysadmin.request(http.MethodDelete, "/api/v1/academic-years/"+f.yearID.String(), nil, http.StatusConflict)
	admin.request(http.MethodDelete, "/api/v1/classes/"+f.classID.String(), nil, http.StatusNoContent)
	admin.request(http.MethodDelete, "/api/v1/academic-years/"+f.yearID.String(), nil, http.StatusForbidden)
	sysadmin.request(http.MethodDelete, "/api/v1/academic-years/"+f.yearID.String(), nil, http.StatusNoContent)
}

func TestWorkerDigestAndQuietHours(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	ctx := context.Background()
	var userID uuid.UUID
	if err := f.pool.QueryRow(ctx, "SELECT user_id FROM students WHERE id = $1", f.studentID).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "INSERT INTO notification_preferences (user_id, digest_frequency) VALUES ($1, 'daily')", userID); err != nil {
		t.Fatal(err)
	}
	// Полдень по школьному времени: «now + 1 час» остаётся в тех же сутках при любом часе запуска.
	now := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	if _, err := f.pool.Exec(ctx, "INSERT INTO notifications (user_id, kind, title, body, created_at) VALUES ($1, 'homework', 'Задание', 'Новое задание', $2)", userID, now.Add(-30*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RunWorkerOnce(ctx, now); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RunWorkerOnce(ctx, now.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	var digests int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = 'digest'", userID).Scan(&digests); err != nil {
		t.Fatal(err)
	}
	if digests != 1 {
		t.Fatalf("digest count = %d, want 1", digests)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE notification_preferences SET quiet_from = '00:00', quiet_to = '23:59' WHERE user_id = $1", userID); err != nil {
		t.Fatal(err)
	}
	if err := f.service.RunWorkerOnce(ctx, now.AddDate(0, 0, 1)); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM notifications WHERE user_id = $1 AND kind = 'digest'", userID).Scan(&digests); err != nil {
		t.Fatal(err)
	}
	if digests != 1 {
		t.Fatalf("digest was sent during quiet hours: %d", digests)
	}
}
