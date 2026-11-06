package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

const journalArchiveIssued = "journal_archive.issued"

// IssueJournalArchive собирает журнал класса за учебный год для печати и архива: оценки и пропуски по урокам, темы,
// задания, замены и итоговые оценки. Каждая выдача получает код подлинности — SHA-256 содержимого — и попадает в
// журнал аудита: по коду можно проверить, что распечатка выдана системой, кем и когда.
func (s *Service) IssueJournalArchive(ctx context.Context, actor Principal, classID uuid.UUID) (api.JournalArchive, error) {
	class, err := s.queries.GetArchiveClass(ctx, classID)
	if err != nil {
		return api.JournalArchive{}, mapDatabaseError(err)
	}
	if !actor.IsAdministrator() {
		onDate := s.schoolToday(ctx)
		if onDate.After(class.EndsOn) {
			onDate = class.EndsOn
		}
		if err := requireClassReportAccess(ctx, s.queries, actor, classID, onDate); err != nil {
			return api.JournalArchive{}, err
		}
	}
	archive, err := s.buildJournalArchive(ctx, class)
	if err != nil {
		return api.JournalArchive{}, err
	}
	archive.GeneratedAt = time.Now().UTC().Truncate(time.Second)
	archive.GeneratedBy = actor.DisplayName
	content, err := json.Marshal(archive)
	if err != nil {
		return api.JournalArchive{}, fmt.Errorf("encode journal archive: %w", err)
	}
	sum := sha256.Sum256(content)
	archive.Digest = hex.EncodeToString(sum[:])
	archive.Code = archiveCode(archive.Digest)
	err = s.transaction(ctx, func(_ pgx.Tx, q *db.Queries) error {
		values, _ := json.Marshal(map[string]any{"code": archive.Code, "digest": archive.Digest, "class_name": archive.ClassName,
			"year_name": archive.YearName, "generated_at": archive.GeneratedAt, "final": archive.Final})
		return audit(ctx, q, actor, journalArchiveIssued, "class", classID, nil, values)
	})
	if err != nil {
		return api.JournalArchive{}, err
	}
	return archive, nil
}

// VerifyJournalArchive находит выдачу архива по коду с распечатки.
func (s *Service) VerifyJournalArchive(ctx context.Context, actor Principal, code string) (api.JournalArchiveVerification, error) {
	if !actor.IsAdministrator() && !actor.HasRole("principal") {
		return api.JournalArchiveVerification{}, apperr.Forbidden()
	}
	normalized := normalizeArchiveCode(code)
	if normalized == "" {
		return api.JournalArchiveVerification{}, apperr.NotFound()
	}
	row, err := s.queries.FindJournalArchiveIssue(ctx, normalized)
	if err != nil {
		return api.JournalArchiveVerification{}, mapDatabaseError(err)
	}
	var issued struct {
		Digest    string `json:"digest"`
		ClassName string `json:"class_name"`
		YearName  string `json:"year_name"`
	}
	if err := json.Unmarshal(row.NewValues, &issued); err != nil {
		return api.JournalArchiveVerification{}, fmt.Errorf("decode journal archive audit: %w", err)
	}
	return api.JournalArchiveVerification{Code: normalized, Digest: issued.Digest, ClassName: issued.ClassName, YearName: issued.YearName,
		GeneratedAt: row.CreatedAt, GeneratedBy: row.IssuedBy}, nil
}

func (s *Service) buildJournalArchive(ctx context.Context, class db.GetArchiveClassRow) (api.JournalArchive, error) {
	archive := api.JournalArchive{SchoolName: class.SchoolName, ClassId: class.ID, ClassName: class.ClassName, YearName: class.YearName,
		Periods: []api.JournalArchivePeriod{}, Students: []api.JournalArchiveStudent{}, Subjects: []api.JournalArchiveSubject{}}
	periods, err := s.queries.ListArchivePeriods(ctx, class.AcademicYearID)
	if err != nil {
		return archive, err
	}
	archive.Final = len(periods) > 0
	for _, period := range periods {
		closed := period.Status == "closed"
		archive.Final = archive.Final && closed
		archive.Periods = append(archive.Periods, api.JournalArchivePeriod{Id: period.ID, Name: period.Name,
			StartsOn: openapi_types.Date{Time: period.StartsOn}, EndsOn: openapi_types.Date{Time: period.EndsOn}, Closed: closed})
	}
	students, err := s.queries.ListArchiveStudents(ctx, class.ID)
	if err != nil {
		return archive, err
	}
	for _, student := range students {
		item := api.JournalArchiveStudent{Id: student.ID, DisplayName: student.DisplayName}
		// Зачисление, закрытое в конце года или позже, — переход в следующий класс, а не выбытие.
		if !student.Active && student.LeftOn.Before(class.EndsOn) {
			item.LeftOn = &openapi_types.Date{Time: student.LeftOn}
		}
		archive.Students = append(archive.Students, item)
	}
	lessons, err := s.queries.ListArchiveLessons(ctx, class.ID)
	if err != nil {
		return archive, err
	}
	grades, err := s.queries.ListArchiveGrades(ctx, class.ID)
	if err != nil {
		return archive, err
	}
	attendance, err := s.queries.ListArchiveAttendance(ctx, class.ID)
	if err != nil {
		return archive, err
	}
	terms, err := s.queries.ListArchiveTermGrades(ctx, db.ListArchiveTermGradesParams{AcademicYearID: class.AcademicYearID, ClassID: class.ID})
	if err != nil {
		return archive, err
	}

	type cellKey struct{ lesson, student uuid.UUID }
	marks := make(map[cellKey]*api.JournalArchiveMark)
	cell := func(lessonID, studentID uuid.UUID) *api.JournalArchiveMark {
		key := cellKey{lessonID, studentID}
		if marks[key] == nil {
			marks[key] = &api.JournalArchiveMark{LessonId: lessonID, StudentId: studentID, Grades: []float32{}}
		}
		return marks[key]
	}
	for _, grade := range grades {
		mark := cell(grade.LessonID, grade.StudentID)
		mark.Grades = append(mark.Grades, numericValue(grade.Value))
	}
	for _, row := range attendance {
		status := row.Status
		cell(row.LessonID, row.StudentID).Attendance = &status
	}

	subjects := make(map[uuid.UUID]*api.JournalArchiveSubject)
	order := make([]uuid.UUID, 0)
	subjectFor := func(id uuid.UUID, name string) *api.JournalArchiveSubject {
		if subjects[id] == nil {
			subjects[id] = &api.JournalArchiveSubject{SubjectId: id, SubjectName: name, Teachers: []string{},
				Lessons: []api.JournalArchiveLesson{}, Marks: []api.JournalArchiveMark{}, TermGrades: []api.JournalArchiveTermGrade{}}
			order = append(order, id)
		}
		return subjects[id]
	}
	for _, lesson := range lessons {
		subject := subjectFor(lesson.SubjectID, lesson.SubjectName)
		item := api.JournalArchiveLesson{Id: lesson.ID, LessonDate: openapi_types.Date{Time: lesson.LessonDate}, PeriodId: lesson.AcademicPeriodID,
			Topic: lesson.Topic, Status: api.JournalArchiveLessonStatus(lesson.Status), TeacherName: lesson.TeacherName, Homework: lesson.Homework,
			CancellationReason: optionalString(lesson.CancellationReason), SubstituteFor: optionalString(lesson.OriginalTeacherName)}
		subject.Lessons = append(subject.Lessons, item)
		// Учитель предмета — тот, кто ведёт его по расписанию; заменяющие видны в строке урока.
		teacher := lesson.TeacherName
		if lesson.OriginalTeacherName != "" {
			teacher = lesson.OriginalTeacherName
		}
		if !slices.Contains(subject.Teachers, teacher) {
			subject.Teachers = append(subject.Teachers, teacher)
		}
		for _, student := range archive.Students {
			if mark := marks[cellKey{lesson.ID, student.Id}]; mark != nil {
				subject.Marks = append(subject.Marks, *mark)
			}
		}
	}
	for _, term := range terms {
		subject := subjectFor(term.SubjectID, term.SubjectName)
		subject.TermGrades = append(subject.TermGrades, api.JournalArchiveTermGrade{StudentId: term.StudentID, PeriodId: term.AcademicPeriodID,
			Value: numericValue(term.Value)})
	}
	for _, id := range order {
		archive.Subjects = append(archive.Subjects, *subjects[id])
	}
	return archive, nil
}

// archiveCode — первые 16 знаков SHA-256 группами по четыре: их удобно переписать с бумаги.
func archiveCode(digest string) string {
	short := strings.ToUpper(digest[:16])
	return short[0:4] + "-" + short[4:8] + "-" + short[8:12] + "-" + short[12:16]
}

// normalizeArchiveCode принимает код в любом регистре, с пробелами и дефисами или без них.
func normalizeArchiveCode(code string) string {
	var hexDigits strings.Builder
	for _, r := range strings.ToUpper(code) {
		switch {
		case r >= '0' && r <= '9', r >= 'A' && r <= 'F':
			hexDigits.WriteRune(r)
		case r == '-' || r == ' ':
		default:
			return ""
		}
	}
	if hexDigits.Len() != 16 {
		return ""
	}
	return archiveCode(hexDigits.String())
}
