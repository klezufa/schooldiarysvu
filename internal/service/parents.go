package service

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"schooldiarysvu/internal/api"
	db "schooldiarysvu/internal/database/sqlc"
)

const maxChildSuggestions = 10

// ListParentStudentLinks — все связи родителей и детей с именами и текущим классом ребёнка.
func (s *Service) ListParentStudentLinks(ctx context.Context, actor Principal) ([]api.ParentStudentLink, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListParentStudentLinks(ctx)
	if err != nil {
		return nil, err
	}
	links := make([]api.ParentStudentLink, 0, len(rows))
	for _, row := range rows {
		link := api.ParentStudentLink{ParentId: row.ParentID, StudentId: row.StudentID, ValidFrom: openapi_types.Date{Time: row.ValidFrom},
			ParentName: row.ParentName, StudentName: row.StudentName, ClassName: optionalString(row.ClassName), Active: row.Active}
		if row.ValidTo.Valid {
			link.ValidTo = &openapi_types.Date{Time: row.ValidTo.Time}
		}
		links = append(links, link)
	}
	return links, nil
}

// SuggestChildren предлагает вероятных детей родителя: та же фамилия (с учётом мужской и женской формы) и отчество,
// образованное от имени родителя. Совпадение по отчеству весомее: однофамильцев в школе бывает много.
func (s *Service) SuggestChildren(ctx context.Context, actor Principal, parentID uuid.UUID) ([]api.ChildSuggestion, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, err
	}
	parentName, err := s.queries.GetParentName(ctx, parentID)
	if err != nil {
		return nil, mapDatabaseError(err)
	}
	students, err := s.queries.ListStudentsForParentMatching(ctx, parentID)
	if err != nil {
		return nil, err
	}
	type scored struct {
		suggestion api.ChildSuggestion
		score      int
	}
	var found []scored
	for _, student := range students {
		reasons := childMatchReasons(parentName, student.DisplayName)
		if len(reasons) == 0 {
			continue
		}
		score := 0
		for _, reason := range reasons {
			score += map[api.ChildSuggestionReasons]int{api.Surname: 2, api.Patronymic: 3}[reason]
		}
		found = append(found, scored{api.ChildSuggestion{StudentId: student.ID, DisplayName: student.DisplayName,
			ClassName: optionalString(student.ClassName), Reasons: reasons}, score})
	}
	slices.SortStableFunc(found, func(a, b scored) int { return b.score - a.score })
	result := make([]api.ChildSuggestion, 0, min(len(found), maxChildSuggestions))
	for _, item := range found[:min(len(found), maxChildSuggestions)] {
		result = append(result, item.suggestion)
	}
	return result, nil
}

// RemoveParentStudent снимает связь сразу. Что именно сделать, решает сервер по школьной дате, а не браузер: связь,
// начатую сегодня или позже, он удаляет (завершить её вчерашним днём нельзя, а до полуночи чужой родитель видел бы
// оценки ребёнка); давнюю завершает вчерашним днём — доступ пропадает, а история, кто и когда его имел, остаётся.
func (s *Service) RemoveParentStudent(ctx context.Context, actor Principal, parentID, studentID uuid.UUID, validFrom time.Time) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		params := db.GetParentStudentLinkStartParams{ParentID: parentID, StudentID: studentID, ValidFrom: validFrom}
		link, err := q.GetParentStudentLinkStart(ctx, params)
		if err != nil {
			return mapDatabaseError(err)
		}
		oldValues, _ := json.Marshal(map[string]any{"parent_id": parentID, "student_id": studentID, "valid_from": link.ValidFrom.Format(time.DateOnly)})
		if !link.Removable {
			if err := q.EndParentStudentLinkYesterday(ctx, db.EndParentStudentLinkYesterdayParams{ParentID: parentID, StudentID: studentID, ValidFrom: validFrom}); err != nil {
				return mapDatabaseError(err)
			}
			return audit(ctx, q, actor, "parent_student.ended", "student", studentID, oldValues, nil)
		}
		if err := q.DeleteParentStudentLink(ctx, db.DeleteParentStudentLinkParams{ParentID: parentID, StudentID: studentID, ValidFrom: validFrom}); err != nil {
			return mapDatabaseError(err)
		}
		return audit(ctx, q, actor, "parent_student.removed", "student", studentID, oldValues, nil)
	})
}

func optionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

// childMatchReasons сравнивает ФИО родителя и ученика («Фамилия Имя Отчество»).
func childMatchReasons(parentName, studentName string) []api.ChildSuggestionReasons {
	parent, student := strings.Fields(normalizeName(parentName)), strings.Fields(normalizeName(studentName))
	if len(parent) == 0 || len(student) == 0 {
		return nil
	}
	var reasons []api.ChildSuggestionReasons
	if stem := surnameStem(parent[0]); len([]rune(stem)) >= 2 && stem == surnameStem(student[0]) {
		reasons = append(reasons, api.Surname)
	}
	if len(parent) >= 2 && len(student) >= 3 {
		if stem := givenNameStem(parent[1]); len([]rune(stem)) >= 2 && stem == patronymicStem(student[2]) {
			reasons = append(reasons, api.Patronymic)
		}
	}
	return reasons
}

func normalizeName(value string) string {
	return strings.ReplaceAll(strings.ToLower(value), "ё", "е")
}

// Окончания фамилий: женская и мужская форма приводятся к общей основе (Иванова → иванов, Петровская → петровск).
var surnameEndings = [][2]string{
	{"ская", "ск"}, {"цкая", "цк"}, {"ский", "ск"}, {"цкий", "цк"}, {"ова", "ов"}, {"ева", "ев"}, {"ина", "ин"}, {"ына", "ын"},
	{"ая", ""}, {"ий", ""}, {"ый", ""}, {"ой", ""},
}

func surnameStem(surname string) string {
	for _, ending := range surnameEndings {
		if base, ok := strings.CutSuffix(surname, ending[0]); ok && base != "" {
			return base + ending[1]
		}
	}
	return surname
}

// Суффиксы отчеств; длинные проверяются раньше коротких (Ильинична до -ична, -ович до -ич).
var patronymicSuffixes = []string{"инична", "ична", "ович", "овна", "евич", "евна", "ич"}

func patronymicStem(patronymic string) string {
	for _, suffix := range patronymicSuffixes {
		if base, ok := strings.CutSuffix(patronymic, suffix); ok && base != "" {
			return canonicalStem(base)
		}
	}
	return ""
}

// Имена, у которых в отчестве меняется основа: Павел → Павлович, Лев → Львович, Михаил → Михайлович, Яков → Яковлевич.
var irregularGivenNames = map[string]string{"павел": "павл", "лев": "льв", "михаил": "михайл", "яков": "яковл"}

func givenNameStem(name string) string {
	if stem, ok := irregularGivenNames[name]; ok {
		return stem
	}
	for _, ending := range []string{"й", "ь", "а", "я"} {
		if base, ok := strings.CutSuffix(name, ending); ok && base != "" {
			return canonicalStem(base)
		}
	}
	return canonicalStem(name)
}

// canonicalStem убирает конечные «и», «ь», «й»: Дмитрий → Дмитриевич, но Василий → Васильевич и Юрий → Юрьевич.
func canonicalStem(stem string) string {
	return strings.TrimRight(stem, "иьй")
}
