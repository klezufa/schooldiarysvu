package database_test

import (
	"context"
	"errors"
	"net/url"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/database"
	db "schooldiarysvu/internal/database/sqlc"
)

func TestSchemaConstraintsAndQueries(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, migratedDatabase(t, ctx, databaseURL))
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	q := db.New(pool).WithTx(tx)
	start := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	finish := time.Date(2027, 5, 31, 0, 0, 0, 0, time.UTC)

	expectViolation(t, ctx, tx, func(q *db.Queries) error {
		_, err := q.CreateAcademicYear(ctx, db.CreateAcademicYearParams{
			Name: "invalid", StartsOn: finish, EndsOn: start,
		})
		return err
	})
	year, err := q.CreateAcademicYear(ctx, db.CreateAcademicYearParams{
		Name: "2026/2027", StartsOn: start, EndsOn: finish, IsCurrent: true,
	})
	must(t, err)
	expectViolation(t, ctx, tx, func(q *db.Queries) error {
		_, err := q.CreateAcademicYear(ctx, db.CreateAcademicYearParams{
			Name: "other current", StartsOn: start, EndsOn: finish, IsCurrent: true,
		})
		return err
	})
	period, err := q.CreateAcademicPeriod(ctx, db.CreateAcademicPeriodParams{
		AcademicYearID: year.ID, Name: "I четверть", StartsOn: start,
		EndsOn: time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC),
	})
	must(t, err)
	class, err := q.CreateClass(ctx, db.CreateClassParams{AcademicYearID: year.ID, Name: "5А"})
	must(t, err)
	mathematics, err := q.CreateSubject(ctx, db.CreateSubjectParams{Name: "Математика"})
	must(t, err)
	russian, err := q.CreateSubject(ctx, db.CreateSubjectParams{Name: "Русский язык"})
	must(t, err)

	teacherUser := createUser(t, ctx, q, "teacher")
	teacher, err := q.CreateTeacher(ctx, teacherUser.ID)
	must(t, err)
	studentUser := createUser(t, ctx, q, "student")
	student, err := q.CreateStudent(ctx, db.CreateStudentParams{
		UserID: studentUser.ID, StudentCode: pgtype.Text{String: "S-001", Valid: true},
	})
	must(t, err)
	expectViolation(t, ctx, tx, func(q *db.Queries) error {
		_, err := q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
			StudentID: student.ID, ClassID: class.ID, StartsOn: finish,
			EndsOn: pgtype.Date{Time: start, Valid: true},
		})
		return err
	})
	_, err = q.CreateEnrollment(ctx, db.CreateEnrollmentParams{
		StudentID: student.ID, ClassID: class.ID, StartsOn: start,
	})
	must(t, err)

	parentUser := createUser(t, ctx, q, "parent")
	parent, err := q.CreateParent(ctx, parentUser.ID)
	must(t, err)
	must(t, q.LinkParentStudent(ctx, db.LinkParentStudentParams{
		ParentID: parent.ID, StudentID: student.ID, ValidFrom: start,
	}))
	linked, err := q.ParentHasStudent(ctx, db.ParentHasStudentParams{
		UserID: parentUser.ID, StudentID: student.ID, Column3: start,
	})
	must(t, err)
	if !linked {
		t.Fatal("active parent-student link was not found")
	}
	linked, err = q.ParentHasStudent(ctx, db.ParentHasStudentParams{
		UserID: parentUser.ID, StudentID: student.ID, Column3: start.AddDate(-1, 0, 0),
	})
	must(t, err)
	if linked {
		t.Fatal("parent-student link is active before valid_from")
	}

	mathLesson := createLesson(t, ctx, q, class.ID, mathematics.ID, teacher.ID, period.ID, teacherUser.ID, start, 9)
	mathWork := createWork(t, ctx, q, mathLesson.ID)
	mathGrade := createGrade(t, ctx, q, student.ID, mathWork.ID, period.ID, teacherUser.ID, 5)
	russianLesson := createLesson(t, ctx, q, class.ID, russian.ID, teacher.ID, period.ID, teacherUser.ID, start, 11)
	russianWork := createWork(t, ctx, q, russianLesson.ID)
	createGrade(t, ctx, q, student.ID, russianWork.ID, period.ID, teacherUser.ID, 1)

	_, err = q.UpdateGrade(ctx, db.UpdateGradeParams{
		ID: mathGrade.ID, Value: numeric(t, 4), Version: mathGrade.Version + 1,
	})
	if !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("stale grade update returned %v, want pgx.ErrNoRows", err)
	}
	updated, err := q.UpdateGrade(ctx, db.UpdateGradeParams{
		ID: mathGrade.ID, Value: numeric(t, 4), Version: mathGrade.Version,
	})
	must(t, err)
	if updated.Version != mathGrade.Version+1 {
		t.Fatalf("grade version = %d, want %d", updated.Version, mathGrade.Version+1)
	}

	report, err := q.ClassReport(ctx, db.ClassReportParams{ClassID: class.ID, ID: period.ID})
	must(t, err)
	if len(report) != 2 {
		t.Fatalf("report has %d rows, want 2", len(report))
	}
	for _, row := range report {
		if row.GradeCount != 1 {
			t.Fatalf("%s grade count = %d, want 1", row.SubjectName, row.GradeCount)
		}
		want := 1.0
		if row.SubjectName == "Математика" {
			want = 4.0
		}
		if row.Average != want {
			t.Fatalf("%s average = %v, want %v", row.SubjectName, row.Average, want)
		}
	}

	present, err := q.MarkAllPresent(ctx, db.MarkAllPresentParams{LessonID: mathLesson.ID, CreatedBy: teacherUser.ID})
	must(t, err)
	if len(present) != 1 || present[0].Status != "present" {
		t.Fatalf("unexpected bulk attendance result: %#v", present)
	}
	must(t, q.CreateAuditLog(ctx, db.CreateAuditLogParams{
		Action: "test", EntityType: "student", EntityID: student.ID, RequestID: "integration-test",
	}))
	expectRawViolation(t, ctx, tx, "UPDATE audit_logs SET action = 'mutated' WHERE request_id = 'integration-test'")
}

// Связь, созданная «сегодня» по школьному времени, действует сразу, даже когда дата в часовом поясе сессии
// PostgreSQL (в контейнере — UTC) другая. UTC+14 и UTC−12 разнесены на 26 часов, поэтому в любой момент хотя бы
// в одном из них дата отличается от даты UTC — тест не зависит от времени запуска.
func TestSchoolTodayFollowsSchoolTimezone(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := database.Connect(ctx, migratedDatabase(t, ctx, databaseURL))
	must(t, err)
	defer pool.Close()

	for _, zone := range []string{"Pacific/Kiritimati", "Etc/GMT+12"} {
		t.Run(zone, func(t *testing.T) {
			location, err := time.LoadLocation(zone)
			must(t, err)
			tx, err := pool.Begin(ctx)
			must(t, err)
			defer func() { _ = tx.Rollback(ctx) }()
			_, err = tx.Exec(ctx, "SET LOCAL TimeZone = 'UTC'")
			must(t, err)
			_, err = tx.Exec(ctx, "UPDATE school_settings SET timezone = $1", zone)
			must(t, err)
			q := db.New(pool).WithTx(tx)

			local := time.Now().In(location)
			today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
			var schoolToday time.Time
			must(t, tx.QueryRow(ctx, "SELECT school_today()").Scan(&schoolToday))
			if !schoolToday.Equal(today) {
				t.Fatalf("school_today() = %s, want %s", schoolToday.Format(time.DateOnly), today.Format(time.DateOnly))
			}

			studentUser := createUser(t, ctx, q, "student-tz")
			student, err := q.CreateStudent(ctx, db.CreateStudentParams{UserID: studentUser.ID})
			must(t, err)
			parentUser := createUser(t, ctx, q, "parent-tz")
			parent, err := q.CreateParent(ctx, parentUser.ID)
			must(t, err)
			must(t, q.LinkParentStudent(ctx, db.LinkParentStudentParams{ParentID: parent.ID, StudentID: student.ID, ValidFrom: today}))
			children, err := q.ListChildren(ctx, parentUser.ID)
			must(t, err)
			if len(children) != 1 || children[0].ID != student.ID {
				t.Fatalf("children = %v, want the student linked today", children)
			}
		})
	}
}

func createUser(t *testing.T, ctx context.Context, q *db.Queries, login string) db.CreateUserRow {
	t.Helper()
	user, err := q.CreateUser(ctx, db.CreateUserParams{
		Login: login, PasswordHash: "integration-test-only", DisplayName: login,
	})
	must(t, err)
	return user
}

func createLesson(t *testing.T, ctx context.Context, q *db.Queries, classID, subjectID, teacherID, periodID, userID uuid.UUID, date time.Time, hour int) db.CreateLessonRow {
	t.Helper()
	startsAt := time.Date(date.Year(), date.Month(), date.Day(), hour, 0, 0, 0, time.UTC)
	lesson, err := q.CreateLesson(ctx, db.CreateLessonParams{
		LessonDate: date, StartsAt: startsAt, EndsAt: startsAt.Add(45 * time.Minute),
		ClassID: classID, SubjectID: subjectID, TeacherID: teacherID,
		AcademicPeriodID: periodID, CreatedBy: pgtype.UUID{Bytes: userID, Valid: true},
	})
	must(t, err)
	return lesson
}

func createWork(t *testing.T, ctx context.Context, q *db.Queries, lessonID uuid.UUID) db.CreateLessonWorkRow {
	t.Helper()
	work, err := q.CreateLessonWork(ctx, db.CreateLessonWorkParams{
		LessonID: lessonID, WorkType: "classwork", Title: "Работа", Weight: numeric(t, 1), Position: 1,
	})
	must(t, err)
	return work
}

func createGrade(t *testing.T, ctx context.Context, q *db.Queries, studentID, workID, periodID, userID uuid.UUID, value float64) db.CreateGradeRow {
	t.Helper()
	grade, err := q.CreateGrade(ctx, db.CreateGradeParams{
		StudentID: studentID, LessonWorkID: workID, AcademicPeriodID: periodID,
		Value: numeric(t, value), CreatedBy: userID,
	})
	must(t, err)
	return grade
}

func numeric(t *testing.T, value float64) pgtype.Numeric {
	t.Helper()
	var number pgtype.Numeric
	must(t, number.Scan(strconv.FormatFloat(value, 'f', -1, 64)))
	return number
}

// migratedDatabase создаёт на сервере TEST_DATABASE_URL отдельную базу с применёнными миграциями
// и удаляет её после теста: тест не зависит от состояния общей базы.
func migratedDatabase(t *testing.T, ctx context.Context, baseURL string) string {
	t.Helper()
	admin, err := database.Connect(ctx, baseURL)
	must(t, err)
	defer admin.Close()
	name := "diary_schema_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, "CREATE DATABASE "+pgx.Identifier{name}.Sanitize())
	must(t, err)
	t.Cleanup(func() {
		cleanup, err := database.Connect(context.Background(), baseURL)
		if err == nil {
			defer cleanup.Close()
			_, _ = cleanup.Exec(context.Background(), "DROP DATABASE "+pgx.Identifier{name}.Sanitize()+" WITH (FORCE)")
		}
	})
	parsed, err := url.Parse(baseURL)
	must(t, err)
	parsed.Path = "/" + name
	must(t, database.Migrate(ctx, parsed.String()))
	return parsed.String()
}

func expectViolation(t *testing.T, ctx context.Context, tx pgx.Tx, operation func(*db.Queries) error) {
	t.Helper()
	nested, err := tx.Begin(ctx)
	must(t, err)
	err = operation(db.New(nested))
	if err == nil {
		_ = nested.Rollback(ctx)
		t.Fatal("operation succeeded, want a database constraint violation")
	}
	must(t, nested.Rollback(ctx))
}

func expectRawViolation(t *testing.T, ctx context.Context, tx pgx.Tx, query string) {
	t.Helper()
	nested, err := tx.Begin(ctx)
	must(t, err)
	_, err = nested.Exec(ctx, query)
	if err == nil {
		_ = nested.Rollback(ctx)
		t.Fatal("operation succeeded, want a database constraint violation")
	}
	must(t, nested.Rollback(ctx))
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}
