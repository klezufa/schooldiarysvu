package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

const maxImportRows = 200

var importableRoles = map[string]bool{"student": true, "teacher": true, "parent": true, "principal": true}

type importPlan struct {
	row      int
	name     string
	role     string
	login    string
	classID  *uuid.UUID
	class    string
	password string
	hash     string
}

// ImportUsers создаёт пользователей пакетом: логины и временные пароли генерируются, ученики сразу зачисляются в класс.
// Операция атомарна: при любой ошибке в строках ничего не создаётся. С dry_run выполняется только проверка.
func (s *Service) ImportUsers(ctx context.Context, actor Principal, input api.UserImportRequest) (api.UserImportResult, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.UserImportResult{}, err
	}
	if len(input.Rows) == 0 || len(input.Rows) > maxImportRows {
		return api.UserImportResult{}, apperr.Unprocessable("В одном импорте может быть от 1 до 200 строк")
	}
	dryRun := input.DryRun != nil && *input.DryRun
	result := api.UserImportResult{DryRun: dryRun, Created: []api.UserImportCreated{}, Errors: []api.UserImportError{}}
	addError := func(row int, message string) {
		result.Errors = append(result.Errors, api.UserImportError{Row: row, Message: message})
	}

	classByName := map[string]uuid.UUID{}
	// Классы ищутся в выбранном году (при подготовке нового года — в нём), по умолчанию в текущем.
	var yearID uuid.UUID
	var yearErr error
	if input.AcademicYearId != nil {
		yearID = *input.AcademicYearId
	} else {
		yearID, yearErr = s.queries.CurrentAcademicYearID(ctx)
	}
	yearName := ""
	if yearErr == nil {
		year, err := s.queries.GetAcademicYear(ctx, yearID)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			return result, apperr.Unprocessable("Учебный год не найден или находится в архиве")
		case err != nil:
			return result, err
		}
		yearName = year.Name
	} else if !errors.Is(yearErr, pgx.ErrNoRows) {
		return result, yearErr
	}
	taken := map[string]bool{}
	plans := make([]*importPlan, 0, len(input.Rows))
	for index, row := range input.Rows {
		number := index + 1
		name := strings.Join(strings.Fields(row.DisplayName), " ")
		role := string(row.Role)
		if name == "" || len([]rune(name)) > 160 {
			addError(number, "Укажите ФИО (до 160 символов)")
			continue
		}
		if !importableRoles[role] {
			addError(number, "Через импорт можно создать только учеников, учителей, родителей и руководителей")
			continue
		}
		plan := &importPlan{row: number, name: name, role: role}
		if row.ClassName != nil && strings.TrimSpace(*row.ClassName) != "" {
			if role != "student" {
				addError(number, "Класс указывается только для учеников")
				continue
			}
			className := strings.TrimSpace(*row.ClassName)
			if yearErr != nil {
				addError(number, "Нет текущего учебного года: класс определить нельзя")
				continue
			}
			id, known := classByName[strings.ToLower(className)]
			if !known {
				found, err := s.queries.FindClassByName(ctx, db.FindClassByNameParams{AcademicYearID: yearID, Value: className})
				if errors.Is(err, pgx.ErrNoRows) {
					addError(number, "Класс «"+className+"» не найден в учебном году «"+yearName+"»")
					continue
				}
				if err != nil {
					return result, err
				}
				id = found.ID
				classByName[strings.ToLower(className)] = id
			}
			plan.classID, plan.class = &id, className
		}
		if row.Login != nil && strings.TrimSpace(*row.Login) != "" {
			plan.login = strings.ToLower(strings.TrimSpace(*row.Login))
			if !loginPattern.MatchString(plan.login) {
				addError(number, "Логин: 3–64 символа, латиница, цифры, точка, дефис или подчёркивание")
				continue
			}
		}
		plans = append(plans, plan)
	}
	for _, plan := range plans {
		login, problem, err := s.uniqueLogin(ctx, plan, taken)
		if err != nil {
			return result, err
		}
		if problem != "" {
			addError(plan.row, problem)
			continue
		}
		plan.login = login
		taken[login] = true
	}
	if len(result.Errors) > 0 {
		return result, nil
	}
	if dryRun {
		for _, plan := range plans {
			result.Created = append(result.Created, plannedUser(plan, ""))
		}
		return result, nil
	}
	if err := s.preparePasswords(ctx, plans); err != nil {
		return result, err
	}
	startsOn := s.schoolToday(ctx)
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		for _, plan := range plans {
			id, err := createUserTx(ctx, q, newUser{Login: plan.login, DisplayName: plan.name, PasswordHash: plan.hash, MustChange: true,
				Roles: []string{plan.role}, ClassID: plan.classID, StartsOn: startsOn})
			if err != nil {
				return err
			}
			newValues, _ := json.Marshal(map[string]any{"login": plan.login, "roles": []string{plan.role}, "class_id": plan.classID, "source": "import"})
			if err := audit(ctx, q, actor, "user.created", "user", id, nil, newValues); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return result, err
	}
	for _, plan := range plans {
		result.Created = append(result.Created, plannedUser(plan, plan.password))
	}
	return result, nil
}

func plannedUser(plan *importPlan, password string) api.UserImportCreated {
	created := api.UserImportCreated{Row: plan.row, DisplayName: plan.name, Login: plan.login, Password: password, Role: plan.role}
	if plan.class != "" {
		class := plan.class
		created.ClassName = &class
	}
	return created
}

// uniqueLogin выбирает свободный логин: заданный вручную проверяется, сгенерированный получает числовой суффикс.
// problem — понятная администратору причина отказа для строки; err — сбой БД, прерывающий весь импорт.
func (s *Service) uniqueLogin(ctx context.Context, plan *importPlan, taken map[string]bool) (login, problem string, err error) {
	explicit := plan.login != ""
	base := plan.login
	if !explicit {
		base = loginFromName(plan.name)
	}
	if base == "" || !loginPattern.MatchString(base) {
		return "", "Не удалось построить логин из ФИО: укажите его вручную", nil
	}
	existing, err := s.queries.ListLoginsWithPrefix(ctx, escapeLike(base))
	if err != nil {
		return "", "", fmt.Errorf("list logins with prefix: %w", err)
	}
	used := make(map[string]bool, len(existing))
	for _, existingLogin := range existing {
		used[existingLogin] = true
	}
	if explicit {
		if used[base] || taken[base] {
			return "", "Логин «" + base + "» уже занят", nil
		}
		return base, "", nil
	}
	candidate := base
	for suffix := 2; used[candidate] || taken[candidate]; suffix++ {
		candidate = base + strconv.Itoa(suffix)
	}
	return candidate, "", nil
}

// preparePasswords генерирует пароли и считает Argon2-хеши параллельно, пока транзакция ещё не открыта; число
// одновременных хешей всё равно ограничено общими слотами (passwords.go), чтобы импорт не съел память входов.
func (s *Service) preparePasswords(ctx context.Context, plans []*importPlan) error {
	workers := runtime.NumCPU()
	if workers > 4 {
		workers = 4
	}
	if workers < 1 {
		workers = 1
	}
	jobs := make(chan *importPlan)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var firstErr error
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for plan := range jobs {
				password, err := generateTemporaryPassword()
				if err == nil {
					plan.password = password
					plan.hash, err = s.hashPassword(ctx, password)
				}
				if err != nil {
					mu.Lock()
					if firstErr == nil {
						firstErr = err
					}
					mu.Unlock()
				}
			}
		}()
	}
	for _, plan := range plans {
		jobs <- plan
	}
	close(jobs)
	wg.Wait()
	return firstErr
}

// enrollStudent переводит ученика в класс с даты startsOn: действующее зачисление закрывается накануне.
// Возвращает false, если ученик уже в этом классе (ничего не изменено).
func enrollStudent(ctx context.Context, q *db.Queries, actor Principal, studentID, classID uuid.UUID, startsOn time.Time) (bool, error) {
	current, err := q.ActiveEnrollmentClass(ctx, db.ActiveEnrollmentClassParams{StudentID: studentID, Column2: startsOn})
	if err == nil {
		if current.ClassID == classID {
			return false, nil
		}
		if !current.StartsOn.Before(startsOn) {
			return false, apperr.Conflict("Зачисление в текущий класс начинается в ту же дату или позже: выберите более позднюю дату перевода")
		}
		endsOn := startsOn.AddDate(0, 0, -1)
		if err := q.EndEnrollment(ctx, db.EndEnrollmentParams{ID: current.ID, EndsOn: pgtype.Date{Time: endsOn, Valid: true}}); err != nil {
			return false, mapDatabaseError(err)
		}
		endedValues, _ := json.Marshal(map[string]any{"ends_on": endsOn.Format("2006-01-02"), "reason": "transfer"})
		if err := audit(ctx, q, actor, "enrollment.ended", "enrollment", current.ID, nil, endedValues); err != nil {
			return false, err
		}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return false, err
	}
	overlaps, err := q.EnrollmentOverlaps(ctx, db.EnrollmentOverlapsParams{StudentID: studentID, Column2: startsOn, Column3: dateOrMax(pgtype.Date{})})
	if err != nil {
		return false, err
	}
	if overlaps {
		return false, apperr.Conflict("У ученика есть зачисление, которое начинается позже выбранной даты")
	}
	created, err := q.CreateEnrollment(ctx, db.CreateEnrollmentParams{StudentID: studentID, ClassID: classID, StartsOn: startsOn})
	if err != nil {
		return false, mapDatabaseError(err)
	}
	newValues, _ := json.Marshal(map[string]any{"student_id": studentID, "class_id": classID, "starts_on": startsOn})
	return true, audit(ctx, q, actor, "enrollment.created", "enrollment", created.ID, nil, newValues)
}

// BulkEnroll зачисляет нескольких учеников в один класс одной операцией.
func (s *Service) BulkEnroll(ctx context.Context, actor Principal, input api.BulkEnrollRequest) (api.BulkResult, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.BulkResult{}, err
	}
	return s.moveStudents(ctx, actor, input.StudentIds, input.ClassId, input.StartsOn.Time)
}

// PromoteClass переводит всех действующих учеников класса в другой класс (например, на новый учебный год).
func (s *Service) PromoteClass(ctx context.Context, actor Principal, sourceClassID uuid.UUID, input api.PromoteClassRequest) (api.BulkResult, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.BulkResult{}, err
	}
	if sourceClassID == input.TargetClassId {
		return api.BulkResult{}, apperr.Unprocessable("Выберите другой класс назначения")
	}
	if _, err := s.queries.GetAcademicYearIDForClass(ctx, sourceClassID); err != nil {
		return api.BulkResult{}, mapDatabaseError(err)
	}
	ids, err := s.queries.ListActiveClassStudentIDs(ctx, db.ListActiveClassStudentIDsParams{ClassID: sourceClassID, Column2: input.StartsOn.Time})
	if err != nil {
		return api.BulkResult{}, err
	}
	if len(ids) == 0 {
		return api.BulkResult{}, apperr.Unprocessable("В классе нет действующих учеников")
	}
	return s.moveStudents(ctx, actor, ids, input.TargetClassId, input.StartsOn.Time)
}

func (s *Service) moveStudents(ctx context.Context, actor Principal, studentIDs []uuid.UUID, classID uuid.UUID, startsOn time.Time) (api.BulkResult, error) {
	var result api.BulkResult
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		result = api.BulkResult{}
		if err := requireDateInClassYear(ctx, q, classID, startsOn); err != nil {
			return err
		}
		for _, studentID := range studentIDs {
			moved, err := enrollStudent(ctx, q, actor, studentID, classID, startsOn)
			if err != nil {
				return err
			}
			if moved {
				result.Processed++
			} else {
				result.Skipped++
			}
		}
		return nil
	})
	return result, err
}

// ---- Приглашения и саморегистрация ----

func invitationStatus(usedAt, revokedAt pgtype.Timestamptz, expiresAt time.Time) api.InvitationStatus {
	switch {
	case usedAt.Valid:
		return api.InvitationStatusUsed
	case revokedAt.Valid:
		return api.InvitationStatusRevoked
	case !expiresAt.After(time.Now()):
		return api.InvitationStatusExpired
	default:
		return api.InvitationStatusOpen
	}
}

func hashInvitationCode(code string) []byte {
	sum := sha256.Sum256([]byte(normalizeInvitationCode(code)))
	return sum[:]
}

func (s *Service) CreateInvitation(ctx context.Context, actor Principal, input api.InvitationInput) (api.InvitationCreated, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.InvitationCreated{}, err
	}
	roles := make([]string, 0, len(input.RoleCodes))
	for _, role := range input.RoleCodes {
		roles = append(roles, string(role))
	}
	if err := guardAssignableRoles(actor, roles); err != nil {
		return api.InvitationCreated{}, err
	}
	if input.ClassId != nil && !hasString(roles, "student") {
		return api.InvitationCreated{}, apperr.Unprocessable("Класс можно указать только для приглашения ученика")
	}
	if input.StudentId != nil && !hasString(roles, "parent") {
		return api.InvitationCreated{}, apperr.Unprocessable("Ребёнка можно указать только для приглашения родителя")
	}
	ttl := 168
	if input.TtlHours != nil {
		ttl = *input.TtlHours
	}
	if ttl < 1 || ttl > 720 {
		return api.InvitationCreated{}, apperr.Unprocessable("Срок действия: от 1 часа до 30 суток")
	}
	note := ""
	if input.Note != nil {
		note = strings.TrimSpace(*input.Note)
	}
	if len([]rune(note)) > 200 {
		return api.InvitationCreated{}, apperr.Unprocessable("Заметка слишком длинная")
	}
	code, err := generateInvitationCode()
	if err != nil {
		return api.InvitationCreated{}, err
	}
	expiresAt := time.Now().Add(time.Duration(ttl) * time.Hour)
	var created db.CreateInvitationRow
	err = s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if input.ClassId != nil {
			// Приглашение в класс завершённого года бесполезно: при регистрации зачислить в него уже нельзя.
			if _, err := enrollmentStartInClassYear(ctx, q, *input.ClassId, s.schoolToday(ctx)); err != nil {
				return err
			}
		}
		created, err = q.CreateInvitation(ctx, db.CreateInvitationParams{TokenHash: hashInvitationCode(code), RoleCodes: roles, Note: note,
			ClassID: optionalUUID(input.ClassId), StudentID: optionalUUID(input.StudentId), CreatedBy: actor.UserID, ExpiresAt: expiresAt})
		if err != nil {
			return mapDatabaseError(err)
		}
		newValues, _ := json.Marshal(map[string]any{"roles": roles, "class_id": input.ClassId, "student_id": input.StudentId, "expires_at": expiresAt})
		return audit(ctx, q, actor, "invitation.created", "invitation", created.ID, nil, newValues)
	})
	if err != nil {
		return api.InvitationCreated{}, err
	}
	invitation := api.Invitation{Id: created.ID, RoleCodes: toRoles(created.RoleCodes), Note: created.Note,
		ClassId: uuidPointer(created.ClassID), StudentId: uuidPointer(created.StudentID), CreatedAt: created.CreatedAt,
		ExpiresAt: created.ExpiresAt, CreatedByName: &actor.DisplayName, Status: api.InvitationStatusOpen}
	return api.InvitationCreated{Invitation: invitation, Code: code}, nil
}

func toRoles(codes []string) []api.Role {
	roles := make([]api.Role, 0, len(codes))
	for _, code := range codes {
		roles = append(roles, api.Role(code))
	}
	return roles
}

func (s *Service) ListInvitations(ctx context.Context, actor Principal, limit, offset int32) ([]api.Invitation, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListInvitations(ctx, db.ListInvitationsParams{Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	result := make([]api.Invitation, 0, len(rows))
	for _, row := range rows {
		item := api.Invitation{Id: row.ID, RoleCodes: toRoles(row.RoleCodes), Note: row.Note, ClassId: uuidPointer(row.ClassID),
			ClassName: textPointer(row.ClassName), StudentId: uuidPointer(row.StudentID), StudentName: textPointer(row.StudentName),
			CreatedAt: row.CreatedAt, ExpiresAt: row.ExpiresAt, UsedAt: timePointer(row.UsedAt), UsedByLogin: textPointer(row.UsedByLogin),
			CreatedByName: &row.CreatedByName, Status: invitationStatus(row.UsedAt, row.RevokedAt, row.ExpiresAt)}
		result = append(result, item)
	}
	return result, nil
}

func (s *Service) RevokeInvitation(ctx context.Context, actor Principal, id uuid.UUID) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		roles, err := q.GetInvitationRoles(ctx, id)
		if err != nil {
			return mapDatabaseError(err)
		}
		if !actor.IsSystemAdmin() {
			for _, role := range roles {
				if isPrivilegedRole(role) {
					return apperr.Forbidden()
				}
			}
		}
		if _, err := q.RevokeInvitation(ctx, id); err != nil {
			if isNoRows(err) {
				return apperr.Conflict("Приглашение уже использовано или отозвано")
			}
			return mapDatabaseError(err)
		}
		return audit(ctx, q, actor, "invitation.revoked", "invitation", id, nil, nil)
	})
}

// invitationRateKey ограничивает перебор кодов: общий ключ на источник запроса.
func invitationRateKey(source string) string { return "invitation\x00" + source }

func (s *Service) LookupInvitation(ctx context.Context, code, source string) (api.InvitationLookup, error) {
	key := invitationRateKey(source)
	if err := s.invites.check(key); err != nil {
		return api.InvitationLookup{}, err
	}
	row, err := s.queries.GetInvitationByHash(ctx, hashInvitationCode(code))
	if err != nil || invitationStatus(row.UsedAt, row.RevokedAt, row.ExpiresAt) != api.InvitationStatusOpen {
		if err == nil || errors.Is(err, pgx.ErrNoRows) {
			s.invites.failed(key)
			return api.InvitationLookup{}, apperr.NotFound()
		}
		return api.InvitationLookup{}, err
	}
	names := make([]string, 0, len(row.RoleCodes))
	for _, role := range row.RoleCodes {
		names = append(names, roleDisplayNames[role])
	}
	note := row.Note
	return api.InvitationLookup{RoleNames: names, ClassName: textPointer(row.ClassName), StudentName: textPointer(row.StudentName), Note: &note, ExpiresAt: row.ExpiresAt}, nil
}

// Register создаёт учётную запись по приглашению: пароль выбирает сам пользователь, смена при входе не требуется.
func (s *Service) Register(ctx context.Context, input api.RegisterRequest, source, requestID string) (api.User, error) {
	key := invitationRateKey(source)
	if err := s.invites.check(key); err != nil {
		return api.User{}, err
	}
	login := strings.ToLower(strings.TrimSpace(input.Login))
	displayName := strings.Join(strings.Fields(input.DisplayName), " ")
	if err := validateNewCredentials(login, displayName, input.Password); err != nil {
		return api.User{}, err
	}
	// Код проверяется до Argon2: без действующего приглашения нельзя заставить сервер считать дорогой хеш.
	if invitation, err := s.queries.GetInvitationByHash(ctx, hashInvitationCode(input.Code)); err != nil || invitationStatus(invitation.UsedAt, invitation.RevokedAt, invitation.ExpiresAt) != api.InvitationStatusOpen {
		if err == nil || isNoRows(err) {
			s.invites.failed(key)
			return api.User{}, apperr.NotFound()
		}
		return api.User{}, err
	}
	hash, err := s.hashPassword(ctx, input.Password)
	if err != nil {
		return api.User{}, err
	}
	startsOn := s.schoolToday(ctx)
	var userID uuid.UUID
	err = s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		row, err := q.GetInvitationByHash(ctx, hashInvitationCode(input.Code))
		if errors.Is(err, pgx.ErrNoRows) {
			s.invites.failed(key)
			return apperr.NotFound()
		}
		if err != nil {
			return err
		}
		if invitationStatus(row.UsedAt, row.RevokedAt, row.ExpiresAt) != api.InvitationStatusOpen {
			s.invites.failed(key)
			return apperr.NotFound()
		}
		userID, err = createUserTx(ctx, q, newUser{Login: login, DisplayName: displayName, PasswordHash: hash, MustChange: false,
			Roles: row.RoleCodes, ClassID: uuidPointer(row.ClassID), ChildID: uuidPointer(row.StudentID), StartsOn: startsOn})
		if err != nil {
			return err
		}
		if _, err := q.ConsumeInvitation(ctx, db.ConsumeInvitationParams{ID: row.ID, UsedBy: nullableUUID(userID)}); err != nil {
			return apperr.NotFound()
		}
		newValues, _ := json.Marshal(map[string]any{"login": login, "roles": row.RoleCodes, "invitation_id": row.ID})
		return audit(ctx, q, Principal{UserID: userID, RequestID: requestID}, "user.registered", "user", userID, nil, newValues)
	})
	if err != nil {
		return api.User{}, err
	}
	return s.userByID(ctx, userID)
}
