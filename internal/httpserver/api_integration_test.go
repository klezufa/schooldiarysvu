package httpserver_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"os"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"schooldiarysvu/internal/credentials"
	"schooldiarysvu/internal/database"
	db "schooldiarysvu/internal/database/sqlc"
	"schooldiarysvu/internal/httpserver"
	"schooldiarysvu/internal/service"
)

const (
	adminInitialPassword = "Admin-initial-password-123"
	adminNewPassword     = "Admin-new-password-456"
	teacherPassword      = "Teacher-password-123"
	studentPassword      = "Student-password-123"
	parentPassword       = "Parent-password-123"
	otherPassword        = "Other-user-password-123"
)

type fixture struct {
	pool                                               *pgxpool.Pool
	service                                            *service.Service
	server                                             *httptest.Server
	yearID, periodID, classID, subjectID, roomID       uuid.UUID
	teacherID, studentID, otherStudentID, lessonWorkID uuid.UUID
}

type testClient struct {
	t      *testing.T
	base   *url.URL
	http   *http.Client
	csrf   string
	method string
}

func TestAPIEndToEnd(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin := newTestClient(t, f.server.URL)
	login := admin.request(http.MethodPost, "/api/v1/auth/login", map[string]string{"login": "ADMIN", "password": adminInitialPassword}, http.StatusOK)
	var auth struct {
		CSRFToken string `json:"csrf_token"`
	}
	decodeResponse(t, login, &auth)
	admin.csrf = auth.CSRFToken
	admin.request(http.MethodGet, "/api/v1/classes", nil, http.StatusForbidden)
	oldSession := cookieValue(t, admin, "diary_session")
	var storedHash []byte
	if err := f.pool.QueryRow(context.Background(), "SELECT token_hash FROM sessions WHERE user_id = (SELECT id FROM users WHERE login = 'admin') ORDER BY created_at DESC LIMIT 1").Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(oldSession))
	if len(storedHash) != 32 || !bytes.Equal(storedHash, hash[:]) || bytes.Contains(storedHash, []byte(oldSession)) {
		t.Fatal("session token is not stored exclusively as a SHA-256 hash")
	}

	withoutCSRF := *admin
	withoutCSRF.csrf = ""
	withoutCSRF.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": adminInitialPassword, "new_password": adminNewPassword}, http.StatusForbidden)
	admin.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": adminInitialPassword, "new_password": adminNewPassword}, http.StatusNoContent)
	admin.refreshCSRF()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/me", nil)
	request.AddCookie(&http.Cookie{Name: "diary_session", Value: oldSession})
	response := httptest.NewRecorder()
	httpserver.NewRouter(f.pool, f.service, false).ServeHTTP(response, request)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked session returned %d, want 401", response.Code)
	}
	admin.request(http.MethodPost, "/api/v1/users", map[string]any{"login": "class.teacher", "password": otherPassword,
		"display_name": "Классный руководитель", "roles": []string{"teacher", "class_teacher"}}, http.StatusCreated)

	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)
	createLesson := map[string]any{"lesson_date": "2026-09-01", "starts_at": "2026-09-01T09:00:00Z", "ends_at": "2026-09-01T09:45:00Z",
		"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID, "room_id": f.roomID,
		"academic_period_id": f.periodID, "topic": "Дроби"}
	withoutCSRF = *teacher
	withoutCSRF.csrf = ""
	withoutCSRF.request(http.MethodPost, "/api/v1/lessons", createLesson, http.StatusForbidden)
	lessonBody := teacher.request(http.MethodPost, "/api/v1/lessons", createLesson, http.StatusCreated)
	lesson := struct {
		ID      uuid.UUID `json:"id"`
		Version int       `json:"version"`
	}{}
	decodeResponse(t, lessonBody, &lesson)
	teacher.request(http.MethodPost, "/api/v1/lessons", createLesson, http.StatusConflict)
	concurrentLesson := map[string]any{"lesson_date": "2026-09-01", "starts_at": "2026-09-01T12:00:00Z", "ends_at": "2026-09-01T12:45:00Z",
		"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID, "academic_period_id": f.periodID}
	statuses := make(chan int, 2)
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			status, _ := teacher.raw(http.MethodPost, "/api/v1/lessons", concurrentLesson)
			statuses <- status
		}()
	}
	wait.Wait()
	close(statuses)
	var gotStatuses []int
	for status := range statuses {
		gotStatuses = append(gotStatuses, status)
	}
	sort.Ints(gotStatuses)
	if fmt.Sprint(gotStatuses) != "[201 409]" {
		t.Fatalf("concurrent schedule statuses = %v, want [201 409]", gotStatuses)
	}

	workBody := teacher.request(http.MethodPost, "/api/v1/lesson-works", map[string]any{"lesson_id": lesson.ID,
		"work_type": "classwork", "title": "Самостоятельная", "weight": 2, "position": 1}, http.StatusCreated)
	var work struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, workBody, &work)
	f.lessonWorkID = work.ID
	gradeInput := map[string]any{"student_id": f.studentID, "lesson_work_id": work.ID, "value": 5, "comment": "Хорошо"}
	gradeBody := teacher.request(http.MethodPost, "/api/v1/grades", gradeInput, http.StatusCreated)
	var grade struct {
		ID      uuid.UUID `json:"id"`
		Version int       `json:"version"`
	}
	decodeResponse(t, gradeBody, &grade)
	teacher.request(http.MethodPatch, fmt.Sprintf("/api/v1/grades/%s?version=99", grade.ID), map[string]any{"value": 4}, http.StatusConflict)
	teacher.request(http.MethodPatch, fmt.Sprintf("/api/v1/grades/%s?version=%d", grade.ID, grade.Version), map[string]any{"value": 4, "comment": "Исправлено"}, http.StatusOK)
	teacher.request(http.MethodPost, "/api/v1/grades", map[string]any{"student_id": f.otherStudentID, "lesson_work_id": work.ID, "value": 5}, http.StatusUnprocessableEntity)

	bulk := map[string]any{"lesson_id": lesson.ID}
	bulkBody := teacher.requestWithHeaders(http.MethodPost, "/api/v1/attendance/bulk", bulk, http.StatusOK, map[string]string{"Idempotency-Key": "bulk-lesson-001"})
	var firstBulk service.BulkAttendanceResult
	decodeResponse(t, bulkBody, &firstBulk)
	repeatedBody := teacher.requestWithHeaders(http.MethodPost, "/api/v1/attendance/bulk", bulk, http.StatusOK, map[string]string{"Idempotency-Key": "bulk-lesson-001"})
	var repeated service.BulkAttendanceResult
	decodeResponse(t, repeatedBody, &repeated)
	if firstBulk.CreatedOrUpdated != 1 || repeated.CreatedOrUpdated != 1 || repeated.Items[0].Version != firstBulk.Items[0].Version {
		t.Fatalf("bulk attendance is not idempotent: first=%#v repeated=%#v", firstBulk, repeated)
	}
	teacher.requestWithHeaders(http.MethodPost, "/api/v1/attendance/bulk", map[string]any{"lesson_id": uuid.New()}, http.StatusConflict, map[string]string{"Idempotency-Key": "bulk-lesson-001"})
	teacher.request(http.MethodPut, "/api/v1/attendance", map[string]any{"lesson_id": lesson.ID, "student_id": f.studentID, "status": "absent", "reason": "Проверка"}, http.StatusOK)

	homeworkBody := teacher.request(http.MethodPost, "/api/v1/homework", map[string]any{"class_id": f.classID, "subject_id": f.subjectID,
		"lesson_id": lesson.ID, "text": "Упражнение 10", "due_date": "2026-09-08", "status": "published"}, http.StatusCreated)
	var homework struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, homeworkBody, &homework)
	fileID := uploadTextFile(t, teacher, homework.ID, "notes.txt", "Обычный текст")
	student := newTestClient(t, f.server.URL)
	student.login("student", studentPassword)
	student.request(http.MethodPut, fmt.Sprintf("/api/v1/homework/%s/completion", homework.ID), map[string]any{"student_id": f.studentID, "completed": true}, http.StatusOK)
	student.request(http.MethodGet, "/api/v1/files/"+fileID.String(), nil, http.StatusOK)
	student.request(http.MethodGet, "/api/v1/grades", nil, http.StatusOK)
	otherStudent := newTestClient(t, f.server.URL)
	otherStudent.login("other.student", otherPassword)
	otherStudent.request(http.MethodGet, "/api/v1/files/"+fileID.String(), nil, http.StatusNotFound)
	otherStudent.request(http.MethodGet, "/api/v1/grades?student_id="+f.studentID.String(), nil, http.StatusNotFound)
	otherTeacher := newTestClient(t, f.server.URL)
	otherTeacher.login("other.teacher", otherPassword)
	otherTeacher.request(http.MethodPost, "/api/v1/grades", gradeInput, http.StatusNotFound)
	parent := newTestClient(t, f.server.URL)
	parent.login("parent", parentPassword)
	parent.request(http.MethodGet, "/api/v1/grades?student_id="+f.studentID.String(), nil, http.StatusOK)
	parent.request(http.MethodGet, "/api/v1/grades?student_id="+f.otherStudentID.String(), nil, http.StatusNotFound)
	parent.request(http.MethodGet, "/api/v1/notifications", nil, http.StatusOK)
	// История оценки: учитель назначения видит создание и изменение; остальные роли не узнают, существует ли оценка.
	historyPath := fmt.Sprintf("/api/v1/grades/%s/history", grade.ID)
	var history []struct {
		Action    string   `json:"action"`
		ActorName *string  `json:"actor_name"`
		OldValue  *float32 `json:"old_value"`
		NewValue  *float32 `json:"new_value"`
	}
	decodeResponse(t, teacher.request(http.MethodGet, historyPath, nil, http.StatusOK), &history)
	if len(history) != 2 || history[0].Action != "grade.updated" || history[1].Action != "grade.created" {
		t.Fatalf("grade history = %+v, want updated then created", history)
	}
	if history[0].OldValue == nil || *history[0].OldValue != 5 || history[0].NewValue == nil || *history[0].NewValue != 4 || history[0].ActorName == nil {
		t.Fatalf("grade history update entry = %+v", history[0])
	}
	otherTeacher.request(http.MethodGet, historyPath, nil, http.StatusNotFound)
	student.request(http.MethodGet, historyPath, nil, http.StatusForbidden)
	parent.request(http.MethodGet, historyPath, nil, http.StatusForbidden)
	student.request(http.MethodGet, "/api/v1/grades/"+uuid.NewString()+"/history", nil, http.StatusForbidden)

	var rollbackWorkID uuid.UUID
	if err := f.pool.QueryRow(context.Background(), "INSERT INTO lesson_works (lesson_id, work_type, title, position) VALUES ($1, 'test', 'Проверка отката', 2) RETURNING id", lesson.ID).Scan(&rollbackWorkID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), `CREATE FUNCTION reject_grade_audit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN IF NEW.action = 'grade.created' THEN RAISE EXCEPTION 'forced audit failure'; END IF; RETURN NEW; END $$;
CREATE TRIGGER reject_grade_audit BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION reject_grade_audit()`); err != nil {
		t.Fatal(err)
	}
	teacher.request(http.MethodPost, "/api/v1/grades", map[string]any{"student_id": f.studentID, "lesson_work_id": rollbackWorkID, "value": 5}, http.StatusInternalServerError)
	if _, err := f.pool.Exec(context.Background(), "DROP TRIGGER reject_grade_audit ON audit_logs; DROP FUNCTION reject_grade_audit()"); err != nil {
		t.Fatal(err)
	}
	var gradeCount int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM grades WHERE lesson_work_id = $1", rollbackWorkID).Scan(&gradeCount); err != nil {
		t.Fatal(err)
	}
	if gradeCount != 0 {
		t.Fatalf("grade was committed despite audit rollback: count=%d", gradeCount)
	}

	principal := newTestClient(t, f.server.URL)
	principal.login("principal", otherPassword)
	if _, err := f.pool.Exec(context.Background(), "UPDATE users SET display_name = '=2+2' WHERE login = 'student'"); err != nil {
		t.Fatal(err)
	}
	csvBody := principal.request(http.MethodGet, fmt.Sprintf("/api/v1/reports/classes/%s?academic_period_id=%s&format=csv", f.classID, f.periodID), nil, http.StatusOK)
	if !strings.Contains(string(csvBody), "'=2+2") {
		t.Fatalf("CSV formula was not neutralized: %q", csvBody)
	}
	var closedWorkID uuid.UUID
	if err := f.pool.QueryRow(context.Background(), "INSERT INTO lesson_works (lesson_id, work_type, title, position) VALUES ($1, 'test', 'Закрытый период', 3) RETURNING id", lesson.ID).Scan(&closedWorkID); err != nil {
		t.Fatal(err)
	}
	sysadmin := newSystemAdmin(t, f)
	admin.request(http.MethodPost, "/api/v1/academic-periods/"+f.periodID.String()+"/close", nil, http.StatusForbidden)
	sysadmin.request(http.MethodPost, "/api/v1/academic-periods/"+f.periodID.String()+"/close", nil, http.StatusNoContent)
	teacher.request(http.MethodPost, "/api/v1/grades", map[string]any{"student_id": f.studentID, "lesson_work_id": closedWorkID, "value": 5}, http.StatusConflict)
	sysadmin.request(http.MethodPut, "/api/v1/academic-periods/"+f.periodID.String()+"/late-edits", map[string]bool{"allowed": true}, http.StatusNoContent)
	teacher.request(http.MethodPost, "/api/v1/grades", map[string]any{"student_id": f.studentID, "lesson_work_id": closedWorkID, "value": 5}, http.StatusCreated)
	admin.request(http.MethodGet, "/api/v1/audit?entity_type=grade", nil, http.StatusOK)
	teacher.request(http.MethodPost, "/api/v1/auth/logout", nil, http.StatusNoContent)
	teacher.request(http.MethodGet, "/api/v1/me", nil, http.StatusUnauthorized)
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	baseURL := os.Getenv("TEST_DATABASE_URL")
	admin, err := database.Connect(ctx, baseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	name := "diary_api_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize()); err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Path = "/" + name
	testURL := parsed.String()
	pool, err := database.Connect(ctx, testURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		cleanup, err := database.Connect(context.Background(), baseURL)
		if err == nil {
			defer cleanup.Close()
			_, _ = cleanup.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		}
	})
	if err := database.Migrate(ctx, testURL); err != nil {
		t.Fatal(err)
	}
	params := credentials.Params{MemoryKiB: 19 * 1024, Iterations: 1, Parallelism: 1, KeyLength: 32}
	app, err := service.New(pool, params, t.TempDir(), 1<<20, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(httpserver.Middleware(slog.New(slog.NewTextHandler(os.Stderr, nil)), httpserver.NewRouter(pool, app, false)))
	t.Cleanup(server.Close)
	q := db.New(pool)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	year, err := q.CreateAcademicYear(ctx, db.CreateAcademicYearParams{Name: "2026/2027", StartsOn: start, EndsOn: start.AddDate(1, 0, 0), IsCurrent: true})
	mustTest(t, err)
	period, err := q.CreateAcademicPeriod(ctx, db.CreateAcademicPeriodParams{AcademicYearID: year.ID, Name: "I", StartsOn: start, EndsOn: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)})
	mustTest(t, err)
	class, err := q.CreateClass(ctx, db.CreateClassParams{AcademicYearID: year.ID, Name: "5А"})
	mustTest(t, err)
	subject, err := q.CreateSubject(ctx, db.CreateSubjectParams{Name: "Математика"})
	mustTest(t, err)
	room, err := q.CreateRoom(ctx, "101")
	mustTest(t, err)
	adminUser := createFixtureUser(t, ctx, q, params, "admin", "Администратор", adminInitialPassword, "administrator")
	mustTest(t, adminUser.Err())
	systemUser := createFixtureUser(t, ctx, q, params, "sysadmin", "Системный администратор", adminInitialPassword, "system_admin")
	mustTest(t, systemUser.Err())
	teacherUser := createFixtureUser(t, ctx, q, params, "teacher", "Учитель", teacherPassword, "teacher")
	teacher, err := q.CreateTeacher(ctx, teacherUser.ID)
	mustTest(t, err)
	studentUser := createFixtureUser(t, ctx, q, params, "student", "Ученик", studentPassword, "student")
	student, err := q.CreateStudent(ctx, db.CreateStudentParams{UserID: studentUser.ID})
	mustTest(t, err)
	parentUser := createFixtureUser(t, ctx, q, params, "parent", "Родитель", parentPassword, "parent")
	parent, err := q.CreateParent(ctx, parentUser.ID)
	mustTest(t, err)
	otherStudentUser := createFixtureUser(t, ctx, q, params, "other.student", "Другой ученик", otherPassword, "student")
	otherStudent, err := q.CreateStudent(ctx, db.CreateStudentParams{UserID: otherStudentUser.ID})
	mustTest(t, err)
	otherTeacherUser := createFixtureUser(t, ctx, q, params, "other.teacher", "Другой учитель", otherPassword, "teacher")
	_, err = q.CreateTeacher(ctx, otherTeacherUser.ID)
	mustTest(t, err)
	createFixtureUser(t, ctx, q, params, "principal", "Руководитель", otherPassword, "principal")
	_, err = q.CreateEnrollment(ctx, db.CreateEnrollmentParams{StudentID: student.ID, ClassID: class.ID, StartsOn: start})
	mustTest(t, err)
	mustTest(t, q.LinkParentStudent(ctx, db.LinkParentStudentParams{ParentID: parent.ID, StudentID: student.ID, ValidFrom: time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)}))
	_, err = q.CreateTeachingAssignment(ctx, db.CreateTeachingAssignmentParams{TeacherID: teacher.ID, ClassID: class.ID, SubjectID: subject.ID, AcademicPeriodID: period.ID, StartsOn: start})
	mustTest(t, err)
	_, err = pool.Exec(ctx, "UPDATE users SET must_change_password = false WHERE login <> 'admin'")
	mustTest(t, err)
	return &fixture{pool: pool, service: app, server: server, yearID: year.ID, periodID: period.ID, classID: class.ID,
		subjectID: subject.ID, roomID: room.ID, teacherID: teacher.ID, studentID: student.ID, otherStudentID: otherStudent.ID}
}

type fixtureUser struct {
	db.CreateUserRow
	err error
}

func (u fixtureUser) Err() error { return u.err }

func createFixtureUser(t *testing.T, ctx context.Context, q *db.Queries, params credentials.Params, login, name, password, role string) fixtureUser {
	t.Helper()
	hash, err := credentials.HashPasswordWithParams(password, params)
	if err != nil {
		t.Fatal(err)
	}
	user, err := q.CreateUser(ctx, db.CreateUserParams{Login: login, PasswordHash: hash, DisplayName: name, MustChangePassword: true})
	if err == nil {
		err = q.AssignRole(ctx, db.AssignRoleParams{UserID: user.ID, Code: role})
	}
	return fixtureUser{CreateUserRow: user, err: err}
}

// newSystemAdmin входит под системным администратором и меняет временный пароль.
func newSystemAdmin(t *testing.T, f *fixture) *testClient {
	t.Helper()
	client := newTestClient(t, f.server.URL)
	client.login("sysadmin", adminInitialPassword)
	client.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": adminInitialPassword, "new_password": adminNewPassword}, http.StatusNoContent)
	client.refreshCSRF()
	return client
}

func newTestClient(t *testing.T, base string) *testClient {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatal(err)
	}
	return &testClient{t: t, base: parsed, http: &http.Client{Jar: jar, Timeout: 20 * time.Second}}
}

func (c *testClient) login(login, password string) {
	body := c.request(http.MethodPost, "/api/v1/auth/login", map[string]string{"login": login, "password": password}, http.StatusOK)
	var response struct {
		CSRFToken string `json:"csrf_token"`
	}
	decodeResponse(c.t, body, &response)
	c.csrf = response.CSRFToken
}

func (c *testClient) request(method, path string, body any, expected int) []byte {
	return c.requestWithHeaders(method, path, body, expected, nil)
}

func (c *testClient) requestWithHeaders(method, path string, body any, expected int, headers map[string]string) []byte {
	status, content := c.rawWithHeaders(method, path, body, headers)
	if status != expected {
		c.t.Fatalf("%s %s returned %d, want %d: %s", method, path, status, expected, content)
	}
	return content
}

func (c *testClient) raw(method, path string, body any) (int, []byte) {
	return c.rawWithHeaders(method, path, body, nil)
}

func (c *testClient) rawWithHeaders(method, path string, body any, headers map[string]string) (int, []byte) {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			c.t.Fatal(err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequest(method, c.base.String()+path, reader)
	if err != nil {
		c.t.Fatal(err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.csrf != "" {
		req.Header.Set("X-CSRF-Token", c.csrf)
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	resp, err := c.http.Do(req)
	if err != nil {
		c.t.Error(err)
		return 0, nil
	}
	defer resp.Body.Close()
	content, err := io.ReadAll(resp.Body)
	if err != nil {
		c.t.Fatal(err)
	}
	return resp.StatusCode, content
}

func (c *testClient) refreshCSRF() {
	c.csrf = cookieValue(c.t, c, "diary_csrf")
}

func cookieValue(t *testing.T, client *testClient, name string) string {
	t.Helper()
	for _, cookie := range client.http.Jar.Cookies(client.base) {
		if cookie.Name == name {
			return cookie.Value
		}
	}
	t.Fatalf("cookie %s not found", name)
	return ""
}

func uploadTextFile(t *testing.T, client *testClient, homeworkID uuid.UUID, name, content string) uuid.UUID {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("homework_id", homeworkID.String()); err != nil {
		t.Fatal(err)
	}
	part, err := writer.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(part, content)
	_ = writer.Close()
	req, err := http.NewRequest(http.MethodPost, client.base.String()+"/api/v1/files", &body)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("X-CSRF-Token", client.csrf)
	resp, err := client.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	responseBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("upload returned %d: %s", resp.StatusCode, responseBody)
	}
	var file struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, responseBody, &file)
	return file.ID
}

func decodeResponse(t *testing.T, body []byte, target any) {
	t.Helper()
	if err := json.Unmarshal(body, target); err != nil {
		t.Fatalf("decode %q: %v", body, err)
	}
}

func mustTest(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
