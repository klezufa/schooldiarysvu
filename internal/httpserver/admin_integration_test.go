package httpserver_test

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"schooldiarysvu/internal/service"
)

type userDTO struct {
	ID                 uuid.UUID `json:"id"`
	Login              string    `json:"login"`
	DisplayName        string    `json:"display_name"`
	IsActive           bool      `json:"is_active"`
	MustChangePassword bool      `json:"must_change_password"`
	Roles              []string  `json:"roles"`
	StudentID          *string   `json:"student_id"`
	TeacherID          *string   `json:"teacher_id"`
	ClassName          *string   `json:"class_name"`
}

func newAdmin(t *testing.T, f *fixture) *testClient {
	t.Helper()
	client := newTestClient(t, f.server.URL)
	client.login("admin", adminInitialPassword)
	client.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": adminInitialPassword, "new_password": adminNewPassword}, http.StatusNoContent)
	client.refreshCSRF()
	return client
}

func createUser(t *testing.T, c *testClient, login string, roles []string, expected int, extra map[string]any) userDTO {
	t.Helper()
	body := map[string]any{"login": login, "password": otherPassword, "display_name": "Пользователь " + login, "roles": roles}
	for key, value := range extra {
		body[key] = value
	}
	raw := c.request(http.MethodPost, "/api/v1/users", body, expected)
	var user userDTO
	if expected == http.StatusCreated {
		decodeResponse(t, raw, &user)
	}
	return user
}

func TestUserManagementPrivilegesAndLifecycle(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin, sysadmin := newAdmin(t, f), newSystemAdmin(t, f)

	// Обычный администратор не может выдавать привилегированные роли ни при создании, ни при изменении.
	createUser(t, admin, "evil.admin", []string{"administrator"}, http.StatusForbidden, nil)
	createUser(t, admin, "evil.sys", []string{"system_admin"}, http.StatusForbidden, nil)
	second := createUser(t, sysadmin, "second.admin", []string{"administrator"}, http.StatusCreated, nil)
	teacher := createUser(t, admin, "new.teacher", []string{"teacher"}, http.StatusCreated, nil)
	if teacher.TeacherID == nil {
		t.Fatal("teacher profile was not created")
	}
	admin.request(http.MethodPatch, "/api/v1/users/"+teacher.ID.String(), map[string]any{"roles": []string{"teacher", "administrator"}}, http.StatusForbidden)
	// Над привилегированными пользователями обычный администратор не властен.
	admin.request(http.MethodPatch, "/api/v1/users/"+second.ID.String(), map[string]any{"display_name": "Взлом"}, http.StatusForbidden)
	admin.request(http.MethodDelete, "/api/v1/users/"+second.ID.String(), nil, http.StatusForbidden)
	admin.request(http.MethodPost, "/api/v1/users/"+second.ID.String()+"/password-reset", nil, http.StatusForbidden)
	admin.request(http.MethodPost, "/api/v1/users/"+second.ID.String()+"/revoke-sessions", nil, http.StatusForbidden)
	// Редактирование ролей обычного пользователя: добавить классного руководителя, затем снять учителя.
	updated := userDTO{}
	decodeResponse(t, admin.request(http.MethodPatch, "/api/v1/users/"+teacher.ID.String(), map[string]any{"roles": []string{"teacher", "class_teacher"}, "display_name": "Иванова И. И."}, http.StatusOK), &updated)
	if len(updated.Roles) != 2 || updated.DisplayName != "Иванова И. И." {
		t.Fatalf("roles were not updated: %+v", updated)
	}
	admin.request(http.MethodPatch, "/api/v1/users/"+teacher.ID.String(), map[string]any{}, http.StatusUnprocessableEntity)
	admin.request(http.MethodPatch, "/api/v1/users/"+teacher.ID.String(), map[string]any{"roles": []string{"nonsense"}}, http.StatusUnprocessableEntity)
	// Свои административные роли менять нельзя; последнего системного администратора защитить.
	var me userDTO
	decodeResponse(t, sysadmin.request(http.MethodGet, "/api/v1/me", nil, http.StatusOK), &me)
	sysadmin.request(http.MethodPatch, "/api/v1/users/"+me.ID.String(), map[string]any{"roles": []string{"administrator"}}, http.StatusConflict)
	sysadmin.request(http.MethodDelete, "/api/v1/users/"+me.ID.String(), nil, http.StatusConflict)
	otherSys := createUser(t, sysadmin, "other.sys", []string{"system_admin"}, http.StatusCreated, nil)
	sysadmin.request(http.MethodPatch, "/api/v1/users/"+otherSys.ID.String(), map[string]any{"roles": []string{"administrator"}}, http.StatusOK)
	// Деактивация, фильтры, повторная активация.
	admin.request(http.MethodDelete, "/api/v1/users/"+teacher.ID.String(), nil, http.StatusNoContent)
	var active, inactive, all []userDTO
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/users?search=new.teach", nil, http.StatusOK), &active)
	if len(active) != 0 {
		t.Fatalf("deactivated user is listed as active: %+v", active)
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/users?status=inactive", nil, http.StatusOK), &inactive)
	if len(inactive) != 1 || inactive[0].ID != teacher.ID {
		t.Fatalf("inactive filter = %+v", inactive)
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/users?status=all&role=teacher", nil, http.StatusOK), &all)
	if len(all) < 2 {
		t.Fatalf("role filter with status=all returned %d users", len(all))
	}
	admin.request(http.MethodGet, "/api/v1/users?status=bogus", nil, http.StatusBadRequest)
	admin.request(http.MethodGet, "/api/v1/users?search=%25", nil, http.StatusOK) // подстановочные символы экранируются
	admin.request(http.MethodPost, "/api/v1/users/"+teacher.ID.String()+"/password-reset", nil, http.StatusConflict)
	admin.request(http.MethodPost, "/api/v1/users/"+teacher.ID.String()+"/reactivate", nil, http.StatusOK)
	// Сброс пароля: старый пароль перестаёт работать, новый одноразовый требует смены.
	var reset struct {
		Password string `json:"password"`
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/users/"+teacher.ID.String()+"/password-reset", nil, http.StatusOK), &reset)
	if len(reset.Password) < 12 {
		t.Fatalf("temporary password is too short: %q", reset.Password)
	}
	victim := newTestClient(t, f.server.URL)
	victim.request(http.MethodPost, "/api/v1/auth/login", map[string]string{"login": "new.teacher", "password": otherPassword}, http.StatusUnauthorized)
	victim.login("new.teacher", reset.Password)
	victim.request(http.MethodGet, "/api/v1/analytics/teacher", nil, http.StatusForbidden) // пока не сменён временный пароль
	admin.request(http.MethodPost, "/api/v1/users/"+teacher.ID.String()+"/revoke-sessions", nil, http.StatusNoContent)
	victim.request(http.MethodGet, "/api/v1/me", nil, http.StatusUnauthorized)
	admin.request(http.MethodPost, "/api/v1/users/"+me.ID.String()+"/password-reset", nil, http.StatusForbidden)
	var audit int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_logs WHERE action IN ('user.password_reset','user.sessions_revoked','user.reactivated') AND new_values::text ILIKE '%'||$1||'%'", reset.Password).Scan(&audit); err != nil {
		t.Fatal(err)
	}
	if audit != 0 {
		t.Fatal("temporary password leaked into the audit log")
	}
	// Снятая роль перестаёт давать доступ, хотя профиль остаётся.
	student := createUser(t, admin, "pupil", []string{"student"}, http.StatusCreated, map[string]any{"class_id": f.classID})
	if student.StudentID == nil {
		t.Fatal("student profile missing")
	}
	var listed []userDTO
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/users?search=pupil", nil, http.StatusOK), &listed)
	if len(listed) != 1 || listed[0].ClassName == nil || *listed[0].ClassName != "5А" {
		t.Fatalf("student was not auto-enrolled into the class: %+v", listed)
	}
	createUser(t, admin, "bad.class", []string{"teacher"}, http.StatusUnprocessableEntity, map[string]any{"class_id": f.classID})
	pupil := newTestClient(t, f.server.URL)
	pupil.login("pupil", otherPassword)
	pupil.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": otherPassword, "new_password": studentPassword}, http.StatusNoContent)
	pupil.refreshCSRF()
	pupil.request(http.MethodGet, "/api/v1/analytics/student", nil, http.StatusOK)
	admin.request(http.MethodPatch, "/api/v1/users/"+student.ID.String(), map[string]any{"roles": []string{"parent"}}, http.StatusOK)
	pupil.request(http.MethodGet, "/api/v1/analytics/student", nil, http.StatusBadRequest)
	pupil.request(http.MethodGet, "/api/v1/analytics/student?student_id="+*student.StudentID, nil, http.StatusNotFound)
}

func TestImportEnrollmentAndPromotion(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin := newAdmin(t, f)
	rows := []map[string]any{
		{"display_name": "Иванов Иван Петрович", "role": "student", "class_name": "5а"},
		{"display_name": "Иванов Игорь Павлович", "role": "student", "class_name": "5А"},
		{"display_name": "Петрова Анна", "role": "teacher"},
	}
	var dry struct {
		DryRun  bool `json:"dry_run"`
		Created []struct {
			Login    string `json:"login"`
			Password string `json:"password"`
		} `json:"created"`
		Errors []struct {
			Row     int    `json:"row"`
			Message string `json:"message"`
		} `json:"errors"`
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/users/import", map[string]any{"dry_run": true, "rows": rows}, http.StatusOK), &dry)
	if len(dry.Errors) != 0 || len(dry.Created) != 3 || dry.Created[0].Password != "" {
		t.Fatalf("dry run: %+v", dry)
	}
	if dry.Created[0].Login != "ivanov.ip" || dry.Created[1].Login != "ivanov.ip2" || dry.Created[2].Login != "petrova.a" {
		t.Fatalf("generated logins = %+v", dry.Created)
	}
	var count int
	_ = f.pool.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE login LIKE 'ivanov%'").Scan(&count)
	if count != 0 {
		t.Fatal("dry run created users")
	}
	bad := append([]map[string]any{{"display_name": "Сидоров Сидор", "role": "student", "class_name": "9Я"}, {"display_name": "Админов", "role": "administrator"}}, rows...)
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/users/import", map[string]any{"rows": bad}, http.StatusOK), &dry)
	if len(dry.Errors) != 2 || len(dry.Created) != 0 {
		t.Fatalf("invalid rows must block the whole import: %+v", dry)
	}
	_ = f.pool.QueryRow(context.Background(), "SELECT count(*) FROM users WHERE login LIKE 'ivanov%'").Scan(&count)
	if count != 0 {
		t.Fatal("import with errors was partially applied")
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/users/import", map[string]any{"rows": rows}, http.StatusOK), &dry)
	if len(dry.Created) != 3 || dry.Created[0].Password == "" {
		t.Fatalf("import result: %+v", dry)
	}
	imported := newTestClient(t, f.server.URL)
	imported.login(dry.Created[0].Login, dry.Created[0].Password)
	var listed []userDTO
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/users?role=student&class_id="+f.classID.String(), nil, http.StatusOK), &listed)
	if len(listed) != 3 { // ученик из фикстуры и двое импортированных
		t.Fatalf("students of the class = %d, want 3", len(listed))
	}
	// Второй импорт с теми же ФИО не должен падать на занятых логинах.
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/users/import", map[string]any{"rows": rows[:1]}, http.StatusOK), &dry)
	if len(dry.Created) != 1 || dry.Created[0].Login != "ivanov.ip3" {
		t.Fatalf("repeated import login = %+v", dry.Created)
	}
	admin.request(http.MethodPost, "/api/v1/users/import", map[string]any{"rows": []map[string]any{}}, http.StatusUnprocessableEntity)

	// Перевод класса целиком.
	// Импорт зачислил учеников «сегодня» по школьному времени; перевод — на следующий день.
	moscow, err := time.LoadLocation("Europe/Moscow")
	mustTest(t, err)
	promoteOn := time.Now().In(moscow).AddDate(0, 0, 1).Format(time.DateOnly)
	var target struct {
		ID uuid.UUID `json:"id"`
	}
	// Классами управляет обычный администратор; учебными годами и периодами — только системный.
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/classes", map[string]any{"academic_year_id": f.yearID, "name": "6А"}, http.StatusCreated), &target)
	// Перевод в ту же дату, когда начинается текущее зачисление, отклоняется понятной ошибкой, а не нарушением CHECK.
	admin.request(http.MethodPost, "/api/v1/classes/"+f.classID.String()+"/promote", map[string]any{"target_class_id": target.ID, "starts_on": "2026-09-01"}, http.StatusConflict)
	var moved struct {
		Processed int `json:"processed"`
		Skipped   int `json:"skipped"`
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/classes/"+f.classID.String()+"/promote", map[string]any{"target_class_id": target.ID, "starts_on": promoteOn}, http.StatusOK), &moved)
	if moved.Processed != 4 || moved.Skipped != 0 {
		t.Fatalf("promotion result: %+v", moved)
	}
	admin.request(http.MethodPost, "/api/v1/classes/"+f.classID.String()+"/promote", map[string]any{"target_class_id": target.ID, "starts_on": promoteOn}, http.StatusUnprocessableEntity) // в классе уже никого нет
	admin.request(http.MethodPost, "/api/v1/classes/"+f.classID.String()+"/promote", map[string]any{"target_class_id": f.classID, "starts_on": promoteOn}, http.StatusUnprocessableEntity)
	var inTarget int
	_ = f.pool.QueryRow(context.Background(), "SELECT count(*) FROM enrollments WHERE class_id = $1 AND ends_on IS NULL", target.ID).Scan(&inTarget)
	if inTarget != 4 {
		t.Fatalf("students in the target class = %d, want 4", inTarget)
	}
	var oldOpen int
	_ = f.pool.QueryRow(context.Background(), "SELECT count(*) FROM enrollments WHERE class_id = $1 AND ends_on IS NULL", f.classID).Scan(&oldOpen)
	if oldOpen != 0 {
		t.Fatalf("source enrollments remained open: %d", oldOpen)
	}
	// Массовое зачисление пропускает уже зачисленных.
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/enrollments/bulk", map[string]any{"student_ids": []uuid.UUID{f.studentID, f.otherStudentID}, "class_id": target.ID, "starts_on": promoteOn}, http.StatusOK), &moved)
	if moved.Processed != 1 || moved.Skipped != 1 { // otherStudent был без класса, studentID уже переведён
		t.Fatalf("bulk enroll: %+v", moved)
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/enrollments/bulk", map[string]any{"student_ids": []uuid.UUID{f.studentID}, "class_id": target.ID, "starts_on": promoteOn}, http.StatusOK), &moved)
	if moved.Processed != 0 || moved.Skipped != 1 {
		t.Fatalf("repeated bulk enroll: %+v", moved)
	}
}

func TestInvitationsAndRegistration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin, sysadmin := newAdmin(t, f), newSystemAdmin(t, f)
	anonymous := newTestClient(t, f.server.URL)
	type created struct {
		Code       string `json:"code"`
		Invitation struct {
			ID     uuid.UUID `json:"id"`
			Status string    `json:"status"`
		} `json:"invitation"`
	}
	invite := func(c *testClient, body map[string]any, expected int) created {
		var result created
		raw := c.request(http.MethodPost, "/api/v1/invitations", body, expected)
		if expected == http.StatusCreated {
			decodeResponse(t, raw, &result)
		}
		return result
	}
	invite(admin, map[string]any{"role_codes": []string{"administrator"}}, http.StatusForbidden)
	invite(admin, map[string]any{"role_codes": []string{"teacher"}, "class_id": f.classID}, http.StatusUnprocessableEntity)
	invite(admin, map[string]any{"role_codes": []string{"student"}, "student_id": f.studentID}, http.StatusUnprocessableEntity)
	invite(admin, map[string]any{"role_codes": []string{"student"}, "ttl_hours": 5000}, http.StatusUnprocessableEntity)
	adminInvite := invite(sysadmin, map[string]any{"role_codes": []string{"administrator"}, "note": "Завуч"}, http.StatusCreated)
	admin.request(http.MethodDelete, "/api/v1/invitations/"+adminInvite.Invitation.ID.String(), nil, http.StatusForbidden)

	studentInvite := invite(admin, map[string]any{"role_codes": []string{"student"}, "class_id": f.classID, "note": "Новенький"}, http.StatusCreated)
	if len(studentInvite.Code) != 23 || strings.Count(studentInvite.Code, "-") != 3 {
		t.Fatalf("unexpected invitation code format: %q", studentInvite.Code)
	}
	var stored int
	_ = f.pool.QueryRow(context.Background(), "SELECT count(*) FROM invitations WHERE encode(token_hash, 'escape') LIKE '%'||$1||'%'", studentInvite.Code).Scan(&stored)
	if stored != 0 {
		t.Fatal("invitation code stored in clear text")
	}
	var lookup struct {
		RoleNames []string `json:"role_names"`
		ClassName *string  `json:"class_name"`
		Note      string   `json:"note"`
	}
	decodeResponse(t, anonymous.request(http.MethodGet, "/api/v1/invitations/lookup?code="+url.QueryEscape(strings.ToLower(strings.ReplaceAll(studentInvite.Code, "-", " "))), nil, http.StatusOK), &lookup)
	if len(lookup.RoleNames) != 1 || lookup.RoleNames[0] != "Ученик" || lookup.ClassName == nil || *lookup.ClassName != "5А" {
		t.Fatalf("lookup: %+v", lookup)
	}
	register := map[string]any{"code": studentInvite.Code, "login": "newbie", "display_name": "Новенький Н.", "password": "Newbie-password-123"}
	anonymous.request(http.MethodPost, "/api/v1/auth/register", map[string]any{"code": studentInvite.Code, "login": "x", "display_name": "Н", "password": "short"}, http.StatusUnprocessableEntity)
	anonymous.request(http.MethodPost, "/api/v1/auth/register", map[string]any{"code": "AAAAA-AAAAA-AAAAA-AAAAA", "login": "newbie", "display_name": "Н", "password": "Newbie-password-123"}, http.StatusNotFound)
	var registered userDTO
	decodeResponse(t, anonymous.request(http.MethodPost, "/api/v1/auth/register", register, http.StatusCreated), &registered)
	if registered.MustChangePassword || len(registered.Roles) != 1 || registered.Roles[0] != "student" || registered.StudentID == nil {
		t.Fatalf("registered user: %+v", registered)
	}
	newbie := newTestClient(t, f.server.URL)
	newbie.login("newbie", "Newbie-password-123")
	newbie.request(http.MethodGet, "/api/v1/analytics/student", nil, http.StatusOK)
	// Код одноразовый; занятый логин не сжигает код.
	anonymous.request(http.MethodPost, "/api/v1/auth/register", register, http.StatusNotFound)
	anonymous.request(http.MethodGet, "/api/v1/invitations/lookup?code="+studentInvite.Code, nil, http.StatusNotFound)
	second := invite(admin, map[string]any{"role_codes": []string{"parent"}, "student_id": registered.StudentID}, http.StatusCreated)
	anonymous.request(http.MethodPost, "/api/v1/auth/register", map[string]any{"code": second.Code, "login": "newbie", "display_name": "Родитель", "password": "Parent-password-456"}, http.StatusConflict)
	anonymous.request(http.MethodPost, "/api/v1/auth/register", map[string]any{"code": second.Code, "login": "newbie.parent", "display_name": "Родитель", "password": "Parent-password-456"}, http.StatusCreated)
	parent := newTestClient(t, f.server.URL)
	parent.login("newbie.parent", "Parent-password-456")
	var children []map[string]any
	decodeResponse(t, parent.request(http.MethodGet, "/api/v1/children", nil, http.StatusOK), &children)
	if len(children) != 1 {
		t.Fatalf("parent invited with a student must see the child, got %v", children)
	}
	// Отзыв приглашения и список.
	third := invite(admin, map[string]any{"role_codes": []string{"teacher"}}, http.StatusCreated)
	admin.request(http.MethodDelete, "/api/v1/invitations/"+third.Invitation.ID.String(), nil, http.StatusNoContent)
	anonymous.request(http.MethodGet, "/api/v1/invitations/lookup?code="+third.Code, nil, http.StatusNotFound)
	var list []struct {
		Status string `json:"status"`
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/invitations", nil, http.StatusOK), &list)
	statuses := map[string]int{}
	for _, item := range list {
		statuses[item.Status]++
	}
	if statuses["used"] != 2 || statuses["revoked"] != 1 || statuses["open"] != 1 {
		t.Fatalf("invitation statuses: %v", statuses)
	}
	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)
	teacher.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": teacherPassword, "new_password": "Teacher-new-password-1"}, http.StatusNoContent)
	teacher.refreshCSRF()
	teacher.request(http.MethodGet, "/api/v1/invitations", nil, http.StatusForbidden)
	teacher.request(http.MethodPost, "/api/v1/invitations", map[string]any{"role_codes": []string{"student"}}, http.StatusForbidden)
	// Перебор кодов ограничен: после серии неудач блокируются и верные коды с того же адреса.
	blocked := false
	for i := 0; i < 8 && !blocked; i++ {
		response, _ := anonymous.raw(http.MethodGet, "/api/v1/invitations/lookup?code=WRONGCODE"+strings.Repeat("A", i+1), nil)
		blocked = response == http.StatusTooManyRequests
	}
	if !blocked {
		t.Fatal("invitation lookups were never rate limited")
	}
	anonymous.request(http.MethodGet, "/api/v1/invitations/lookup?code="+second.Code, nil, http.StatusTooManyRequests)
}

// Назначения и шаблон привязаны к периоду: в новой четверти без переноса редактору шаблона нечего предложить.
func TestCarryOverAcademicPeriod(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin, sysadmin := newAdmin(t, f), newSystemAdmin(t, f)
	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)
	teacher.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": teacherPassword, "new_password": "Teacher-new-password-1"}, http.StatusNoContent)
	teacher.refreshCSRF()

	var second struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, sysadmin.request(http.MethodPost, "/api/v1/academic-periods", map[string]any{"academic_year_id": f.yearID, "name": "II",
		"starts_on": "2026-11-01", "ends_on": "2026-12-31"}, http.StatusCreated), &second)
	admin.request(http.MethodPost, "/api/v1/schedule-entries", map[string]any{"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID,
		"room_id": f.roomID, "academic_period_id": f.periodID, "weekday": 2, "lesson_number": 1, "starts_at": "09:00", "ends_at": "09:45", "valid_from": "2026-09-01"},
		http.StatusCreated)

	path := "/api/v1/academic-periods/" + second.ID.String() + "/carry-over"
	request := map[string]any{"from_period_id": f.periodID, "class_id": f.classID}
	teacher.request(http.MethodPost, path, request, http.StatusForbidden)
	admin.request(http.MethodPost, "/api/v1/academic-periods/"+f.periodID.String()+"/carry-over", request, http.StatusUnprocessableEntity)
	admin.request(http.MethodPost, path, map[string]any{"from_period_id": uuid.New()}, http.StatusNotFound)

	var result struct {
		AssignmentsCreated int `json:"assignments_created"`
		AssignmentsSkipped int `json:"assignments_skipped"`
		EntriesCreated     int `json:"entries_created"`
		EntriesSkipped     int `json:"entries_skipped"`
	}
	decodeResponse(t, admin.request(http.MethodPost, path, request, http.StatusOK), &result)
	if result.AssignmentsCreated != 1 || result.EntriesCreated != 1 || result.AssignmentsSkipped != 0 || result.EntriesSkipped != 0 {
		t.Fatalf("first carry-over: %+v", result)
	}
	decodeResponse(t, admin.request(http.MethodPost, path, request, http.StatusOK), &result)
	if result.AssignmentsCreated != 0 || result.EntriesCreated != 0 || result.AssignmentsSkipped != 1 || result.EntriesSkipped != 1 {
		t.Fatalf("repeated carry-over must not duplicate: %+v", result)
	}

	var entries []map[string]any
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/admin/schedule-entries?class_id="+f.classID.String()+"&academic_period_id="+second.ID.String(), nil, http.StatusOK), &entries)
	if len(entries) != 1 || entries[0]["valid_from"] != "2026-11-01" || entries[0]["teacher_name"] != "Учитель" {
		t.Fatalf("carried template: %v", entries)
	}
	var assignments []map[string]any
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/teaching-assignments", nil, http.StatusOK), &assignments)
	carried := 0
	for _, item := range assignments {
		// Имена нужны экрану назначений и выбору учителя в шаблоне — без них там показывались идентификаторы.
		if item["academic_period_id"] == second.ID.String() && item["starts_on"] == "2026-11-01" && item["ends_on"] == nil &&
			item["teacher_name"] == "Учитель" && item["class_name"] == "5А" && item["subject_name"] == "Математика" {
			carried++
		}
	}
	if carried != 1 {
		t.Fatalf("carried assignments: %v", assignments)
	}

	var generated struct {
		Created, Existing, Conflicts int
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/schedule/generate", map[string]any{"from": "2026-11-01", "to": "2026-11-30", "class_id": f.classID},
		http.StatusOK), &generated)
	if generated.Created != 4 || generated.Conflicts != 0 { // вторники ноября: 3, 10, 17, 24
		t.Fatalf("lessons of the new period: %+v", generated)
	}

	sysadmin.request(http.MethodPost, "/api/v1/academic-periods/"+second.ID.String()+"/close", nil, http.StatusNoContent)
	admin.request(http.MethodPost, path, request, http.StatusUnprocessableEntity)
}

// Справочники и связи, которыми администратор пользуется каждый день: предметы, кабинеты, переименование класса и
// периода, состав класса на дату, связь родителя с ребёнком и её завершение, завершение назначения учителя.
func TestDirectoryAndRelationsAdministration(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin := newAdmin(t, f)
	sysadmin := newSystemAdmin(t, f)
	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)
	parent := newTestClient(t, f.server.URL)
	parent.login("parent", parentPassword)
	ctx := context.Background()
	type named struct {
		ID   uuid.UUID `json:"id"`
		Name string    `json:"name"`
	}
	listNames := func(client *testClient, path string) []string {
		var items []named
		decodeResponse(t, client.request(http.MethodGet, path, nil, http.StatusOK), &items)
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.Name)
		}
		return names
	}

	// Предметы: создание, повтор названия, переименование, архив. Учитель справочники не меняет.
	var physics named
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/subjects", map[string]any{"name": "Физика"}, http.StatusCreated), &physics)
	admin.request(http.MethodPost, "/api/v1/subjects", map[string]any{"name": "Физика"}, http.StatusConflict)
	teacher.request(http.MethodPost, "/api/v1/subjects", map[string]any{"name": "Астрономия"}, http.StatusForbidden)
	admin.request(http.MethodPatch, "/api/v1/subjects/"+physics.ID.String(), map[string]any{"name": "Физика и астрономия"}, http.StatusOK)
	if names := strings.Join(listNames(admin, "/api/v1/subjects"), ","); !strings.Contains(names, "Физика и астрономия") {
		t.Fatalf("subjects after rename = %s", names)
	}
	admin.request(http.MethodDelete, "/api/v1/subjects/"+physics.ID.String(), nil, http.StatusNoContent)
	if names := strings.Join(listNames(admin, "/api/v1/subjects"), ","); strings.Contains(names, "Физика") {
		t.Fatalf("archived subject is still listed: %s", names)
	}

	// Кабинеты.
	admin.request(http.MethodPost, "/api/v1/rooms", map[string]any{"name": "204"}, http.StatusCreated)
	teacher.request(http.MethodPost, "/api/v1/rooms", map[string]any{"name": "205"}, http.StatusForbidden)
	if names := strings.Join(listNames(admin, "/api/v1/rooms"), ","); names != "101,204" {
		t.Fatalf("rooms = %s, want 101,204", names)
	}

	// Класс переименовывается, его состав читается на дату.
	admin.request(http.MethodPatch, "/api/v1/classes/"+f.classID.String(), map[string]any{"name": "5 «А»"}, http.StatusOK)
	if names := strings.Join(listNames(admin, "/api/v1/classes"), ","); names != "5 «А»" {
		t.Fatalf("classes after rename = %s", names)
	}
	var roster []struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/classes/"+f.classID.String()+"/students?date=2026-09-10", nil, http.StatusOK), &roster)
	if len(roster) != 1 || roster[0].ID != f.studentID {
		t.Fatalf("class roster = %+v, want only the enrolled student", roster)
	}
	// Учителю класса состав нужен для журнала; учителю без назначения в этом классе — нет.
	teacher.request(http.MethodGet, "/api/v1/classes/"+f.classID.String()+"/students?date=2026-09-10", nil, http.StatusOK)
	otherTeacher := newTestClient(t, f.server.URL)
	otherTeacher.login("other.teacher", otherPassword)
	if status, _ := otherTeacher.raw(http.MethodGet, "/api/v1/classes/"+f.classID.String()+"/students?date=2026-09-10", nil); status != http.StatusForbidden && status != http.StatusNotFound {
		t.Fatalf("teacher without an assignment read the class roster: %d", status)
	}

	// Учебный год и период меняет только системный администратор.
	yearInput := map[string]any{"name": "2026/2027 учебный год", "starts_on": "2026-09-01", "ends_on": "2027-09-01", "is_current": true}
	admin.request(http.MethodPatch, "/api/v1/academic-years/"+f.yearID.String(), yearInput, http.StatusForbidden)
	sysadmin.request(http.MethodPatch, "/api/v1/academic-years/"+f.yearID.String(), yearInput, http.StatusOK)
	if names := strings.Join(listNames(admin, "/api/v1/academic-years"), ","); names != "2026/2027 учебный год" {
		t.Fatalf("academic years = %s", names)
	}
	periodInput := map[string]any{"name": "I четверть", "starts_on": "2026-09-01", "ends_on": "2026-10-31"}
	admin.request(http.MethodPatch, "/api/v1/academic-periods/"+f.periodID.String(), periodInput, http.StatusForbidden)
	sysadmin.request(http.MethodPatch, "/api/v1/academic-periods/"+f.periodID.String(), periodInput, http.StatusOK)
	sysadmin.request(http.MethodPatch, "/api/v1/academic-periods/"+f.periodID.String(), map[string]any{"name": "I", "starts_on": "2026-10-31", "ends_on": "2026-09-01"}, http.StatusUnprocessableEntity)
	var settings struct {
		Timezone string `json:"timezone"`
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/school/settings", nil, http.StatusOK), &settings)
	if settings.Timezone == "" {
		t.Fatal("school settings without a time zone")
	}

	// Второй ребёнок родителя: связь действует сразу, а после завершения ребёнок пропадает из списка родителя.
	var parentID uuid.UUID
	mustTest(t, f.pool.QueryRow(ctx, "SELECT p.id FROM parents p JOIN users u ON u.id = p.user_id WHERE u.login = 'parent'").Scan(&parentID))
	children := func() int {
		var items []struct {
			ID uuid.UUID `json:"id"`
		}
		decodeResponse(t, parent.request(http.MethodGet, "/api/v1/children", nil, http.StatusOK), &items)
		return len(items)
	}
	link := map[string]any{"parent_id": parentID, "student_id": f.otherStudentID, "valid_from": "2026-09-01"}
	teacher.request(http.MethodPost, "/api/v1/parent-student", link, http.StatusForbidden)
	admin.request(http.MethodPost, "/api/v1/parent-student", link, http.StatusNoContent)
	if got := children(); got != 2 {
		t.Fatalf("children after linking = %d, want 2", got)
	}
	link["valid_to"] = "2026-09-02"
	admin.request(http.MethodPut, "/api/v1/parent-student", link, http.StatusNoContent)
	if got := children(); got != 1 {
		t.Fatalf("children after ending the link = %d, want 1", got)
	}

	// Учитель снят с предмета: после даты завершения он не может создать урок этого класса.
	var assignments []struct {
		ID        uuid.UUID `json:"id"`
		TeacherID uuid.UUID `json:"teacher_id"`
		EndsOn    *string   `json:"ends_on"`
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/teaching-assignments", nil, http.StatusOK), &assignments)
	if len(assignments) != 1 || assignments[0].TeacherID != f.teacherID {
		t.Fatalf("assignments = %+v", assignments)
	}
	teacher.request(http.MethodPatch, "/api/v1/teaching-assignments/"+assignments[0].ID.String(), map[string]any{"ends_on": "2026-09-30"}, http.StatusForbidden)
	admin.request(http.MethodPatch, "/api/v1/teaching-assignments/"+assignments[0].ID.String(), map[string]any{"ends_on": "2026-09-30"}, http.StatusOK)
	lesson := map[string]any{"starts_at": "2026-10-05T09:00:00Z", "ends_at": "2026-10-05T09:45:00Z", "lesson_date": "2026-10-05",
		"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID, "academic_period_id": f.periodID}
	if status, body := teacher.raw(http.MethodPost, "/api/v1/lessons", lesson); status == http.StatusCreated {
		t.Fatalf("teacher created a lesson after the assignment ended: %s", body)
	}
	lesson["lesson_date"], lesson["starts_at"], lesson["ends_at"] = "2026-09-29", "2026-09-29T09:00:00Z", "2026-09-29T09:45:00Z"
	teacher.request(http.MethodPost, "/api/v1/lessons", lesson, http.StatusCreated)
}

// Изменение строки шаблона пересоздаёт её будущие незаполненные уроки по новым данным, удаление — убирает их.
// Уроки с темой (и тем более с оценками) остаются. Свой учебный год вокруг текущей даты: уроки должны быть в будущем.
func TestScheduleEntryChangesResyncFutureLessons(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin := newAdmin(t, f)
	sysadmin := newSystemAdmin(t, f)
	ctx := context.Background()
	moscow, err := time.LoadLocation("Europe/Moscow")
	mustTest(t, err)
	now := time.Now().In(moscow)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	day := func(offset int) string { return today.AddDate(0, 0, offset).Format(time.DateOnly) }
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, sysadmin.request(http.MethodPost, "/api/v1/academic-years", map[string]any{"name": "Синхронизация", "starts_on": day(-30), "ends_on": day(120), "is_current": false}, http.StatusCreated), &created)
	yearID := created.ID
	decodeResponse(t, sysadmin.request(http.MethodPost, "/api/v1/academic-periods", map[string]any{"academic_year_id": yearID, "name": "Четверть", "starts_on": day(-30), "ends_on": day(120)}, http.StatusCreated), &created)
	periodID := created.ID
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/classes", map[string]any{"academic_year_id": yearID, "name": "7С"}, http.StatusCreated), &created)
	classID := created.ID
	admin.request(http.MethodPost, "/api/v1/teaching-assignments", map[string]any{"teacher_id": f.teacherID, "class_id": classID, "subject_id": f.subjectID,
		"academic_period_id": periodID, "starts_on": day(-30)}, http.StatusCreated)
	weekday := func(date time.Time) int { return (int(date.Weekday())+6)%7 + 1 }
	entry := map[string]any{"class_id": classID, "subject_id": f.subjectID, "teacher_id": f.teacherID, "academic_period_id": periodID,
		"weekday": weekday(today.AddDate(0, 0, 1)), "lesson_number": 7, "starts_at": "14:00", "ends_at": "14:45", "valid_from": day(-30)}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/schedule-entries", entry, http.StatusCreated), &created)
	entryID := created.ID
	admin.request(http.MethodPost, "/api/v1/schedule/generate", map[string]any{"from": day(1), "to": day(28), "class_id": classID}, http.StatusOK)

	futureDates := func() []string {
		rows, err := f.pool.Query(ctx, "SELECT lesson_date::text FROM lessons WHERE class_id = $1 ORDER BY lesson_date", classID)
		mustTest(t, err)
		var dates []string
		for rows.Next() {
			var date string
			mustTest(t, rows.Scan(&date))
			dates = append(dates, date)
		}
		mustTest(t, rows.Err())
		return dates
	}
	before := futureDates()
	if len(before) != 4 || before[0] != day(1) {
		t.Fatalf("generated lessons = %v, want 4 weekly lessons from %s", before, day(1))
	}
	// Учитель заранее записал тему третьего урока: такой урок не удаляется.
	_, err = f.pool.Exec(ctx, "UPDATE lessons SET topic = 'Подготовленная тема' WHERE class_id = $1 AND lesson_date = $2::date", classID, before[2])
	mustTest(t, err)

	// Урок переносится на следующий день недели.
	entry["weekday"] = weekday(today.AddDate(0, 0, 2))
	delete(entry, "class_id")
	delete(entry, "subject_id")
	delete(entry, "academic_period_id")
	admin.request(http.MethodPatch, "/api/v1/schedule-entries/"+entryID.String(), entry, http.StatusOK)
	after := futureDates()
	want := []string{day(2), day(9), before[2], day(16)}
	if strings.Join(after, ",") != strings.Join(want, ",") {
		t.Fatalf("lessons after the move = %v, want %v (new weekday up to the old horizon, the lesson with a topic kept)", after, want)
	}

	admin.request(http.MethodDelete, "/api/v1/schedule-entries/"+entryID.String(), nil, http.StatusNoContent)
	if left := futureDates(); len(left) != 1 || left[0] != before[2] {
		t.Fatalf("lessons after deleting the entry = %v, want only the one with a topic", left)
	}
	var resynced int
	mustTest(t, f.pool.QueryRow(ctx, "SELECT count(*) FROM audit_logs WHERE action = 'lessons.resynced' AND entity_id = $1", entryID).Scan(&resynced))
	if resynced != 2 {
		t.Fatalf("lessons.resynced audit entries = %d, want 2", resynced)
	}
}

// Переход на новый учебный год: классы относятся к году, поэтому 5А прошлого года и 6А нового — разные классы.
// Назначения, шаблон и зачисления не должны смешивать годы, а перенос переносит 5А → 6А явно.
func TestAcademicYearTransition(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin, sysadmin := newAdmin(t, f), newSystemAdmin(t, f)
	var ref struct {
		ID uuid.UUID `json:"id"`
	}
	create := func(c *testClient, path string, body map[string]any) uuid.UUID {
		t.Helper()
		decodeResponse(t, c.request(http.MethodPost, path, body, http.StatusCreated), &ref)
		return ref.ID
	}
	nextYear := create(sysadmin, "/api/v1/academic-years", map[string]any{"name": "2027/2028", "starts_on": "2027-09-02", "ends_on": "2028-05-31"})
	nextPeriod := create(sysadmin, "/api/v1/academic-periods", map[string]any{"academic_year_id": nextYear, "name": "I четверть", "starts_on": "2027-09-02", "ends_on": "2027-10-31"})
	nextClass := create(admin, "/api/v1/classes", map[string]any{"academic_year_id": nextYear, "name": "6А"})

	var periods []map[string]any
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/academic-periods", nil, http.StatusOK), &periods)
	if len(periods) != 2 || periods[1]["id"] != nextPeriod.String() {
		t.Fatalf("periods of all years: %v", periods)
	}

	// Класс прошлого года нельзя назначить на период нового.
	admin.request(http.MethodPost, "/api/v1/teaching-assignments", map[string]any{"teacher_id": f.teacherID, "class_id": f.classID, "subject_id": f.subjectID,
		"academic_period_id": nextPeriod, "starts_on": "2027-09-02"}, http.StatusUnprocessableEntity)
	admin.request(http.MethodPost, "/api/v1/schedule-entries", map[string]any{"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID,
		"academic_period_id": nextPeriod, "weekday": 2, "lesson_number": 1, "starts_at": "09:00", "ends_at": "09:45", "valid_from": "2027-09-02"}, http.StatusUnprocessableEntity)

	// Перенос между годами: без класса нового года — отказ; класс назначения обязан быть из года целевого периода.
	admin.request(http.MethodPost, "/api/v1/schedule-entries", map[string]any{"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID,
		"room_id": f.roomID, "academic_period_id": f.periodID, "weekday": 2, "lesson_number": 1, "starts_at": "09:00", "ends_at": "09:45", "valid_from": "2026-09-01"}, http.StatusCreated)
	path := "/api/v1/academic-periods/" + nextPeriod.String() + "/carry-over"
	admin.request(http.MethodPost, path, map[string]any{"from_period_id": f.periodID}, http.StatusUnprocessableEntity)
	admin.request(http.MethodPost, path, map[string]any{"from_period_id": f.periodID, "class_id": f.classID, "target_class_id": f.classID}, http.StatusUnprocessableEntity)
	var carried struct {
		AssignmentsCreated int `json:"assignments_created"`
		EntriesCreated     int `json:"entries_created"`
	}
	decodeResponse(t, admin.request(http.MethodPost, path, map[string]any{"from_period_id": f.periodID, "class_id": f.classID, "target_class_id": nextClass},
		http.StatusOK), &carried)
	if carried.AssignmentsCreated != 1 || carried.EntriesCreated != 1 {
		t.Fatalf("carry-over 5А → 6А: %+v", carried)
	}
	var entries []map[string]any
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/admin/schedule-entries?academic_period_id="+nextPeriod.String(), nil, http.StatusOK), &entries)
	if len(entries) != 1 || entries[0]["class_id"] != nextClass.String() || entries[0]["valid_from"] != "2027-09-02" {
		t.Fatalf("template of the new year: %v", entries)
	}

	// Перевод класса и зачисление — только датой внутри учебного года класса назначения.
	admin.request(http.MethodPost, "/api/v1/classes/"+f.classID.String()+"/promote", map[string]any{"target_class_id": nextClass, "starts_on": "2027-08-31"}, http.StatusUnprocessableEntity)
	admin.request(http.MethodPost, "/api/v1/enrollments/bulk", map[string]any{"student_ids": []uuid.UUID{f.otherStudentID}, "class_id": nextClass, "starts_on": "2026-11-01"}, http.StatusUnprocessableEntity)
	admin.request(http.MethodPost, "/api/v1/enrollments", map[string]any{"student_id": f.otherStudentID, "class_id": nextClass, "starts_on": "2026-11-01"}, http.StatusUnprocessableEntity)
	var promoted struct{ Processed, Skipped int }
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/classes/"+f.classID.String()+"/promote", map[string]any{"target_class_id": nextClass, "starts_on": "2027-09-02"},
		http.StatusOK), &promoted)
	if promoted.Processed != 1 {
		t.Fatalf("promotion 5А → 6А: %+v", promoted)
	}

	// Ученик, созданный сейчас в класс следующего года, зачисляется с первого дня того года.
	newcomer := createUser(t, admin, "newcomer.6a", []string{"student"}, http.StatusCreated, map[string]any{"class_id": nextClass})
	var enrollments []map[string]any
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/enrollments?class_id="+nextClass.String(), nil, http.StatusOK), &enrollments)
	starts := map[string]string{}
	for _, item := range enrollments {
		starts[valueString(item["student_name"])] = valueString(item["starts_on"])
	}
	if starts[newcomer.DisplayName] != "2027-09-02" || starts["Ученик"] != "2027-09-02" {
		t.Fatalf("enrollments of 6А: %v", enrollments)
	}

	// Импорт ищет классы в выбранном году; без года — в текущем, где 6А нет.
	rows := []map[string]any{{"display_name": "Новикова Анна", "role": "student", "class_name": "6А"}}
	var imported struct {
		Errors []struct{ Message string } `json:"errors"`
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/users/import", map[string]any{"dry_run": true, "rows": rows}, http.StatusOK), &imported)
	if len(imported.Errors) != 1 || !strings.Contains(imported.Errors[0].Message, "2026/2027") {
		t.Fatalf("import into the current year: %+v", imported)
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/users/import", map[string]any{"dry_run": true, "academic_year_id": nextYear, "rows": rows}, http.StatusOK), &imported)
	if len(imported.Errors) != 0 {
		t.Fatalf("import into the next year: %+v", imported)
	}

	// Приглашение в класс завершённого года бесполезно: при регистрации зачислить в него уже нельзя.
	pastYear := create(sysadmin, "/api/v1/academic-years", map[string]any{"name": "2024/2025", "starts_on": "2024-09-01", "ends_on": "2025-05-31"})
	pastClass := create(admin, "/api/v1/classes", map[string]any{"academic_year_id": pastYear, "name": "9Б"})
	admin.request(http.MethodPost, "/api/v1/invitations", map[string]any{"role_codes": []string{"student"}, "class_id": pastClass}, http.StatusUnprocessableEntity)
	admin.request(http.MethodPost, "/api/v1/invitations", map[string]any{"role_codes": []string{"student"}, "class_id": nextClass}, http.StatusCreated)
}

func valueString(value any) string {
	text, _ := value.(string)
	return text
}

func TestScheduleManagementGenerationAndAnalytics(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	admin, sysadmin := newAdmin(t, f), newSystemAdmin(t, f)
	teacher := newTestClient(t, f.server.URL)
	teacher.login("teacher", teacherPassword)
	teacher.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": teacherPassword, "new_password": "Teacher-new-password-1"}, http.StatusNoContent)
	teacher.refreshCSRF()
	entry := map[string]any{"class_id": f.classID, "subject_id": f.subjectID, "teacher_id": f.teacherID, "room_id": f.roomID, "academic_period_id": f.periodID,
		"weekday": 2, "lesson_number": 1, "starts_at": "09:00", "ends_at": "09:45", "valid_from": "2026-09-01"}
	var created struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/schedule-entries", entry, http.StatusCreated), &created)
	var entries []map[string]any
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/admin/schedule-entries?class_id="+f.classID.String(), nil, http.StatusOK), &entries)
	if len(entries) != 1 || entries[0]["teacher_name"] != "Учитель" {
		t.Fatalf("admin schedule entries: %v", entries)
	}
	teacher.request(http.MethodGet, "/api/v1/admin/schedule-entries", nil, http.StatusForbidden)
	admin.request(http.MethodPatch, "/api/v1/schedule-entries/"+created.ID.String(), map[string]any{"teacher_id": f.teacherID, "room_id": f.roomID, "weekday": 2, "lesson_number": 1,
		"starts_at": "09:00", "ends_at": "09:40", "valid_from": "2026-09-01"}, http.StatusOK)
	admin.request(http.MethodPatch, "/api/v1/schedule-entries/"+created.ID.String(), map[string]any{"teacher_id": f.teacherID, "weekday": 9, "lesson_number": 1,
		"starts_at": "09:00", "ends_at": "09:40", "valid_from": "2026-09-01"}, http.StatusUnprocessableEntity)

	var generated struct {
		Created, Existing, Conflicts int
	}
	request := map[string]any{"from": "2026-09-01", "to": "2026-09-30", "skip_dates": []string{"2026-09-15"}}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/schedule/generate", request, http.StatusOK), &generated)
	if generated.Created != 4 { // вторники: 1, 8, 22, 29 (15-е пропущено)
		t.Fatalf("generated lessons: %+v", generated)
	}
	decodeResponse(t, admin.request(http.MethodPost, "/api/v1/schedule/generate", request, http.StatusOK), &generated)
	if generated.Created != 0 || generated.Existing != 4 {
		t.Fatalf("generation must be idempotent: %+v", generated)
	}
	admin.request(http.MethodPost, "/api/v1/schedule/generate", map[string]any{"from": "2026-09-30", "to": "2026-09-01"}, http.StatusUnprocessableEntity)
	teacher.request(http.MethodPost, "/api/v1/schedule/generate", request, http.StatusForbidden)

	var lessons []map[string]any
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/schedule?from=2026-09-01&to=2026-09-30&class_id="+f.classID.String(), nil, http.StatusOK), &lessons)
	if len(lessons) != 4 {
		t.Fatalf("school schedule for class: %d lessons", len(lessons))
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/schedule?from=2026-09-01&to=2026-09-30&teacher_id="+f.teacherID.String(), nil, http.StatusOK), &lessons)
	if len(lessons) != 4 {
		t.Fatalf("school schedule for teacher: %d lessons", len(lessons))
	}
	admin.request(http.MethodGet, "/api/v1/schedule?from=2026-09-01&to=2026-09-30", nil, http.StatusForbidden)
	teacher.request(http.MethodGet, "/api/v1/schedule?from=2026-09-01&to=2026-09-30&class_id="+f.classID.String(), nil, http.StatusNotFound)
	admin.request(http.MethodGet, "/api/v1/schedule?from=2026-09-01&to=2026-09-30&class_id="+f.classID.String()+"&teacher_id="+f.teacherID.String(), nil, http.StatusBadRequest)
	admin.request(http.MethodDelete, "/api/v1/schedule-entries/"+created.ID.String(), nil, http.StatusNoContent)
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/schedule?from=2026-09-01&to=2026-09-30&class_id="+f.classID.String(), nil, http.StatusOK), &lessons)
	if len(lessons) != 4 {
		t.Fatalf("deleting the template must not delete lessons: %d", len(lessons))
	}

	// Аналитика: отметки через API, затем проверка форм ответов и прав.
	var firstLesson struct {
		ID uuid.UUID `json:"id"`
	}
	firstLesson.ID = uuid.MustParse(lessons[0]["id"].(string))
	var work struct {
		ID uuid.UUID `json:"id"`
	}
	decodeResponse(t, teacher.request(http.MethodPost, "/api/v1/lesson-works", map[string]any{"lesson_id": firstLesson.ID, "work_type": "classwork", "title": "Работа", "weight": 1, "position": 1}, http.StatusCreated), &work)
	admin.request(http.MethodPost, "/api/v1/enrollments", map[string]any{"student_id": f.otherStudentID, "class_id": f.classID, "starts_on": "2026-09-01"}, http.StatusCreated)
	teacher.request(http.MethodPost, "/api/v1/grades", map[string]any{"student_id": f.studentID, "lesson_work_id": work.ID, "value": 5}, http.StatusCreated)
	teacher.request(http.MethodPost, "/api/v1/grades", map[string]any{"student_id": f.otherStudentID, "lesson_work_id": work.ID, "value": 3}, http.StatusCreated)
	var school struct {
		Counts struct {
			Students int `json:"students"`
			Classes  int `json:"classes"`
		} `json:"counts"`
		GradeByClass []struct {
			Name    string  `json:"name"`
			Average float64 `json:"average"`
		} `json:"grade_by_class"`
		Distribution []struct {
			Value float64 `json:"value"`
			Count int     `json:"count"`
		} `json:"distribution"`
		Alerts []struct {
			Kind string `json:"kind"`
		} `json:"alerts"`
	}
	decodeResponse(t, admin.request(http.MethodGet, "/api/v1/analytics/school", nil, http.StatusOK), &school)
	if school.Counts.Students != 2 || school.Counts.Classes != 1 || len(school.GradeByClass) != 1 || school.GradeByClass[0].Average != 4 || len(school.Distribution) != 2 {
		t.Fatalf("school analytics: %+v", school)
	}
	teacher.request(http.MethodGet, "/api/v1/analytics/school", nil, http.StatusForbidden)
	var teacherStats struct {
		Classes []struct {
			Average *float64 `json:"average"`
		} `json:"classes"`
		Distribution []map[string]any `json:"distribution"`
	}
	decodeResponse(t, teacher.request(http.MethodGet, "/api/v1/analytics/teacher", nil, http.StatusOK), &teacherStats)
	if len(teacherStats.Classes) != 1 || teacherStats.Classes[0].Average == nil || *teacherStats.Classes[0].Average != 4 || len(teacherStats.Distribution) != 2 {
		t.Fatalf("teacher analytics: %+v", teacherStats)
	}
	admin.request(http.MethodGet, "/api/v1/analytics/teacher", nil, http.StatusForbidden)
	student := newTestClient(t, f.server.URL)
	student.login("student", studentPassword)
	student.request(http.MethodPost, "/api/v1/auth/password", map[string]string{"old_password": studentPassword, "new_password": "Student-new-password-1"}, http.StatusNoContent)
	student.refreshCSRF()
	var own struct {
		Average  *float64 `json:"average"`
		Subjects []struct {
			SubjectName string   `json:"subject_name"`
			Average     *float64 `json:"average"`
			TermValue   *float64 `json:"term_value"`
		} `json:"subjects"`
	}
	decodeResponse(t, student.request(http.MethodGet, "/api/v1/analytics/student", nil, http.StatusOK), &own)
	if own.Average == nil || *own.Average != 5 || len(own.Subjects) != 1 || own.Subjects[0].TermValue != nil {
		t.Fatalf("student analytics: %+v", own)
	}
	student.request(http.MethodGet, "/api/v1/analytics/student?student_id="+f.otherStudentID.String(), nil, http.StatusNotFound)
	admin.request(http.MethodGet, "/api/v1/analytics/student?student_id="+f.studentID.String(), nil, http.StatusNotFound)

	// Системная панель: только системному администратору.
	admin.request(http.MethodGet, "/api/v1/system/overview", nil, http.StatusForbidden)
	teacher.request(http.MethodGet, "/api/v1/system/overview", nil, http.StatusForbidden)
	var overview struct {
		Database struct {
			Version     string `json:"version"`
			SizeBytes   int64  `json:"size_bytes"`
			Connections int    `json:"connections"`
			Tables      []struct {
				Name string `json:"name"`
			} `json:"tables"`
		} `json:"database"`
		Host struct {
			NumCPU int `json:"num_cpu"`
		} `json:"host"`
		Counts struct {
			UsersActive int `json:"users_active"`
		} `json:"counts"`
	}
	decodeResponse(t, sysadmin.request(http.MethodGet, "/api/v1/system/overview", nil, http.StatusOK), &overview)
	if !strings.Contains(overview.Database.Version, "PostgreSQL") || overview.Database.SizeBytes <= 0 || overview.Database.Connections < 1 || len(overview.Database.Tables) == 0 || overview.Host.NumCPU < 1 || overview.Counts.UsersActive < 4 {
		t.Fatalf("system overview: %+v", overview)
	}
}

func TestSystemOverviewOperations(t *testing.T) {
	if os.Getenv("TEST_DATABASE_URL") == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	f := newFixture(t)
	sysadmin := newSystemAdmin(t, f)
	certPath := writeTestCertificate(t, []string{"dnevnik.svu"}, time.Now().Add(20*24*time.Hour))
	f.service.SetRuntimeInfo(service.RuntimeInfo{NetworkProfile: "https", TLSCertFile: certPath})
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO maintenance_events (kind, status, details, created_at) VALUES
		('backup', 'success', '{"size_bytes": 1048576}', now() - interval '2 hours'),
		('backup', 'success', '{"size_bytes": 2048}', now() - interval '3 days'),
		('backup', 'failure', '{"reason": "pg_dump завершился с ошибкой"}', now() - interval '1 day'),
		('restore', 'success', '{}', now() - interval '5 days')`); err != nil {
		t.Fatal(err)
	}
	var overview struct {
		Operations struct {
			NetworkProfile          string     `json:"network_profile"`
			LastBackupAt            *time.Time `json:"last_backup_at"`
			LastBackupSizeBytes     *int64     `json:"last_backup_size_bytes"`
			LastBackupFailureAt     *time.Time `json:"last_backup_failure_at"`
			LastBackupFailureReason *string    `json:"last_backup_failure_reason"`
			LastRestoreAt           *time.Time `json:"last_restore_at"`
			TLSDaysLeft             *int       `json:"tls_days_left"`
			TLSNames                []string   `json:"tls_names"`
			TLSError                *string    `json:"tls_error"`
		} `json:"operations"`
	}
	decodeResponse(t, sysadmin.request(http.MethodGet, "/api/v1/system/overview", nil, http.StatusOK), &overview)
	ops := overview.Operations
	if ops.NetworkProfile != "https" || ops.LastBackupAt == nil || time.Since(*ops.LastBackupAt) > 3*time.Hour || ops.LastBackupSizeBytes == nil || *ops.LastBackupSizeBytes != 1048576 {
		t.Fatalf("latest backup: %+v", ops)
	}
	if ops.LastBackupFailureReason == nil || *ops.LastBackupFailureReason != "pg_dump завершился с ошибкой" || ops.LastRestoreAt == nil {
		t.Fatalf("failure/restore: %+v", ops)
	}
	if ops.TLSDaysLeft == nil || *ops.TLSDaysLeft != 19 || len(ops.TLSNames) != 1 || ops.TLSNames[0] != "dnevnik.svu" || ops.TLSError != nil {
		t.Fatalf("certificate: %+v", ops)
	}

	f.service.SetRuntimeInfo(service.RuntimeInfo{NetworkProfile: "local-http", TLSCertFile: filepath.Join(t.TempDir(), "missing.pem")})
	decodeResponse(t, sysadmin.request(http.MethodGet, "/api/v1/system/overview", nil, http.StatusOK), &overview)
	if overview.Operations.NetworkProfile != "local-http" || overview.Operations.TLSError == nil {
		t.Fatalf("missing certificate must be reported, not fail the page: %+v", overview.Operations)
	}
}

func writeTestCertificate(t *testing.T, names []string, notAfter time.Time) string {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "fullchain.pem")
	if err := os.WriteFile(path, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
