package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

// UserFilter — параметры списка пользователей.
type UserFilter struct {
	Search  string
	Role    string
	Status  string // active | inactive | all
	ClassID *uuid.UUID
	Limit   int32
	Offset  int32
}

func escapeLike(value string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(value)
}

// ListUsers возвращает страницу пользователей и общее число подходящих записей.
func (s *Service) ListUsers(ctx context.Context, actor Principal, filter UserFilter) ([]api.User, int64, error) {
	if err := actor.requireAdministrator(); err != nil {
		return nil, 0, err
	}
	if filter.Status == "" {
		filter.Status = "active"
	}
	if filter.Status != "active" && filter.Status != "inactive" && filter.Status != "all" {
		return nil, 0, apperr.BadRequest("Некорректный статус")
	}
	if filter.Role != "" && !assignableRoles[filter.Role] {
		return nil, 0, apperr.BadRequest("Неизвестная роль")
	}
	rows, err := s.queries.SearchUsers(ctx, db.SearchUsersParams{
		Search: escapeLike(strings.ToLower(strings.TrimSpace(filter.Search))), Role: filter.Role, Status: filter.Status,
		ClassID: optionalUUID(filter.ClassID), RowLimit: filter.Limit, RowOffset: filter.Offset,
	})
	if err != nil {
		return nil, 0, err
	}
	result := make([]api.User, 0, len(rows))
	var total int64
	for _, row := range rows {
		total = row.Total
		roles := make([]api.Role, 0, len(row.Roles))
		for _, role := range row.Roles {
			roles = append(roles, api.Role(role))
		}
		user := api.User{Id: row.ID, Login: row.Login, DisplayName: row.DisplayName, IsActive: row.IsActive,
			MustChangePassword: row.MustChangePassword, Roles: roles,
			StudentId: uuidPointer(row.StudentID), TeacherId: uuidPointer(row.TeacherID), ParentId: uuidPointer(row.ParentID)}
		user.LastLoginAt = timePointer(row.LastLoginAt)
		if row.ClassName != "" {
			name := row.ClassName
			user.ClassName = &name
		}
		result = append(result, user)
	}
	return result, total, nil
}

// newUser описывает учётную запись, которую нужно создать целиком: пользователь, роли, профили, зачисление, связь с ребёнком.
type newUser struct {
	Login        string
	DisplayName  string
	PasswordHash string
	MustChange   bool
	Roles        []string
	StudentCode  *string
	ClassID      *uuid.UUID
	ChildID      *uuid.UUID // для родителя: ученик, с которым создаётся связь
	StartsOn     time.Time
}

// guardAssignableRoles проверяет набор ролей и запрещает обычному администратору выдавать привилегированные.
func guardAssignableRoles(actor Principal, roles []string) error {
	if len(roles) == 0 {
		return apperr.Unprocessable("Выберите хотя бы одну роль")
	}
	seen := map[string]bool{}
	for _, role := range roles {
		if !assignableRoles[role] || seen[role] {
			return apperr.Unprocessable("Неизвестная или повторяющаяся роль")
		}
		seen[role] = true
		if isPrivilegedRole(role) && !actor.IsSystemAdmin() {
			return apperr.Forbidden()
		}
	}
	return nil
}

func hasString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

// ensureProfiles создаёт профили, которые нужны ролям (ученик, учитель, родитель); повторный вызов безопасен.
func ensureProfiles(ctx context.Context, q *db.Queries, userID uuid.UUID, roles []string, studentCode *string) error {
	if hasString(roles, "student") {
		if err := q.EnsureStudent(ctx, db.EnsureStudentParams{UserID: userID, StudentCode: nullableText(studentCode)}); err != nil {
			return mapDatabaseError(err)
		}
	}
	if hasString(roles, "teacher") || hasString(roles, "class_teacher") {
		if err := q.EnsureTeacher(ctx, userID); err != nil {
			return mapDatabaseError(err)
		}
	}
	if hasString(roles, "parent") {
		if err := q.EnsureParent(ctx, userID); err != nil {
			return mapDatabaseError(err)
		}
	}
	return nil
}

func loginTakenError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_login_key" {
		return apperr.Conflict("Этот логин уже занят")
	}
	return mapDatabaseError(err)
}

// createUserTx выполняет всю цепочку создания в переданной транзакции и возвращает идентификатор пользователя.
func createUserTx(ctx context.Context, q *db.Queries, spec newUser) (uuid.UUID, error) {
	created, err := q.CreateUser(ctx, db.CreateUserParams{Login: spec.Login, PasswordHash: spec.PasswordHash, DisplayName: spec.DisplayName, MustChangePassword: spec.MustChange})
	if err != nil {
		return uuid.Nil, loginTakenError(err)
	}
	for _, role := range spec.Roles {
		if err := q.AssignRole(ctx, db.AssignRoleParams{UserID: created.ID, Code: role}); err != nil {
			return uuid.Nil, mapDatabaseError(err)
		}
	}
	if err := ensureProfiles(ctx, q, created.ID, spec.Roles, spec.StudentCode); err != nil {
		return uuid.Nil, err
	}
	if spec.ClassID != nil && hasString(spec.Roles, "student") {
		student, err := q.GetStudentByUserID(ctx, created.ID)
		if err != nil {
			return uuid.Nil, mapDatabaseError(err)
		}
		startsOn, err := enrollmentStartInClassYear(ctx, q, *spec.ClassID, spec.StartsOn)
		if err != nil {
			return uuid.Nil, err
		}
		if _, err := q.CreateEnrollment(ctx, db.CreateEnrollmentParams{StudentID: student.ID, ClassID: *spec.ClassID, StartsOn: startsOn}); err != nil {
			return uuid.Nil, mapDatabaseError(err)
		}
	}
	if spec.ChildID != nil && hasString(spec.Roles, "parent") {
		parent, err := q.GetParentByUserID(ctx, created.ID)
		if err != nil {
			return uuid.Nil, mapDatabaseError(err)
		}
		if err := q.LinkParentStudent(ctx, db.LinkParentStudentParams{ParentID: parent.ID, StudentID: *spec.ChildID, ValidFrom: spec.StartsOn}); err != nil {
			return uuid.Nil, mapDatabaseError(err)
		}
	}
	return created.ID, nil
}

// schoolToday возвращает сегодняшнюю дату по часовому поясу школы (дата без времени, UTC-полночь).
func (s *Service) schoolToday(ctx context.Context) time.Time {
	location := time.UTC
	if settings, err := s.queries.GetSchoolSettings(ctx); err == nil {
		if loaded, err := time.LoadLocation(settings.Timezone); err == nil {
			location = loaded
		}
	}
	now := time.Now().In(location)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

func validateNewCredentials(login, displayName, password string) error {
	if !loginPattern.MatchString(login) {
		return apperr.Unprocessable("Логин: 3–64 символа, латиница, цифры, точка, дефис или подчёркивание")
	}
	if displayName == "" || len([]rune(displayName)) > 160 {
		return apperr.Unprocessable("Укажите ФИО (до 160 символов)")
	}
	if len([]rune(password)) < 12 || len(password) > 128 {
		return apperr.Unprocessable("Пароль должен содержать от 12 до 128 символов")
	}
	return nil
}

func (s *Service) CreateUser(ctx context.Context, actor Principal, input api.CreateUserJSONBody) (api.User, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.User{}, err
	}
	login := strings.ToLower(strings.TrimSpace(input.Login))
	displayName := strings.TrimSpace(input.DisplayName)
	if err := validateNewCredentials(login, displayName, input.Password); err != nil {
		return api.User{}, err
	}
	roles := make([]string, 0, len(input.Roles))
	for _, role := range input.Roles {
		roles = append(roles, string(role))
	}
	if err := guardAssignableRoles(actor, roles); err != nil {
		return api.User{}, err
	}
	if input.ClassId != nil && !hasString(roles, "student") {
		return api.User{}, apperr.Unprocessable("Класс можно указать только для ученика")
	}
	hash, err := s.hashPassword(ctx, input.Password)
	if err != nil {
		return api.User{}, err
	}
	startsOn := s.schoolToday(ctx)
	var userID uuid.UUID
	err = s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		userID, err = createUserTx(ctx, q, newUser{Login: login, DisplayName: displayName, PasswordHash: hash, MustChange: true,
			Roles: roles, StudentCode: input.StudentCode, ClassID: input.ClassId, StartsOn: startsOn})
		if err != nil {
			return err
		}
		newValues, _ := json.Marshal(map[string]any{"login": login, "roles": roles, "class_id": input.ClassId})
		return audit(ctx, q, actor, "user.created", "user", userID, nil, newValues)
	})
	if err != nil {
		return api.User{}, err
	}
	return s.userByID(ctx, userID)
}

// loadManagedUser читает цель операции и проверяет, что actor вправе ею управлять: привилегированных
// пользователей (администратор, системный администратор) может менять только системный администратор.
func loadManagedUser(ctx context.Context, q *db.Queries, actor Principal, userID uuid.UUID) (db.GetUserAnyByIDRow, []string, error) {
	user, err := q.GetUserAnyByID(ctx, userID)
	if err != nil {
		return db.GetUserAnyByIDRow{}, nil, mapDatabaseError(err)
	}
	roles, err := q.GetUserRoles(ctx, userID)
	if err != nil {
		return db.GetUserAnyByIDRow{}, nil, err
	}
	if !actor.IsSystemAdmin() {
		for _, role := range roles {
			if isPrivilegedRole(role) {
				return db.GetUserAnyByIDRow{}, nil, apperr.Forbidden()
			}
		}
	}
	return user, roles, nil
}

// ensureNotLastSystemAdmin не даёт оставить систему без активного системного администратора.
func ensureNotLastSystemAdmin(ctx context.Context, q *db.Queries, targetRoles []string, targetActive bool) error {
	if !targetActive || !hasString(targetRoles, roleSystemAdmin) {
		return nil
	}
	active, err := q.CountActiveUsersWithRole(ctx, roleSystemAdmin)
	if err != nil {
		return err
	}
	if active <= 1 {
		return apperr.Conflict("Нельзя лишить систему последнего системного администратора")
	}
	return nil
}

// UpdateUser меняет имя и/или набор ролей. Обычный администратор не может выдавать, снимать или менять привилегированные роли.
func (s *Service) UpdateUser(ctx context.Context, actor Principal, userID uuid.UUID, input api.UpdateUserJSONBody) (api.User, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.User{}, err
	}
	var displayName *string
	if input.DisplayName != nil {
		trimmed := strings.TrimSpace(*input.DisplayName)
		if trimmed == "" || len([]rune(trimmed)) > 160 {
			return api.User{}, apperr.Unprocessable("Проверьте отображаемое имя")
		}
		displayName = &trimmed
	}
	var wanted []string
	if input.Roles != nil {
		for _, role := range *input.Roles {
			wanted = append(wanted, string(role))
		}
		if err := guardAssignableRoles(actor, wanted); err != nil {
			return api.User{}, err
		}
	}
	if displayName == nil && input.Roles == nil {
		return api.User{}, apperr.Unprocessable("Нечего изменять")
	}
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		user, current, err := loadManagedUser(ctx, q, actor, userID)
		if err != nil {
			return err
		}
		oldValues, _ := json.Marshal(map[string]any{"display_name": user.DisplayName, "roles": current})
		if displayName != nil {
			if _, err := q.UpdateUserProfile(ctx, db.UpdateUserProfileParams{ID: userID, DisplayName: *displayName}); err != nil {
				return mapDatabaseError(err)
			}
		}
		if input.Roles != nil {
			if userID == actor.UserID && (hasString(current, roleSystemAdmin) != hasString(wanted, roleSystemAdmin) || hasString(current, roleAdministrator) != hasString(wanted, roleAdministrator)) {
				return apperr.Conflict("Нельзя изменить собственные административные роли")
			}
			if hasString(current, roleSystemAdmin) && !hasString(wanted, roleSystemAdmin) {
				if err := ensureNotLastSystemAdmin(ctx, q, current, user.IsActive); err != nil {
					return err
				}
			}
			for _, role := range current {
				if !hasString(wanted, role) {
					if err := q.RemoveRole(ctx, db.RemoveRoleParams{UserID: userID, Code: role}); err != nil {
						return mapDatabaseError(err)
					}
				}
			}
			for _, role := range wanted {
				if !hasString(current, role) {
					if err := q.AssignRole(ctx, db.AssignRoleParams{UserID: userID, Code: role}); err != nil {
						return mapDatabaseError(err)
					}
				}
			}
			if err := ensureProfiles(ctx, q, userID, wanted, nil); err != nil {
				return err
			}
		}
		finalName := user.DisplayName
		if displayName != nil {
			finalName = *displayName
		}
		finalRoles := current
		if input.Roles != nil {
			finalRoles = wanted
		}
		newValues, _ := json.Marshal(map[string]any{"display_name": finalName, "roles": finalRoles})
		return audit(ctx, q, actor, "user.updated", "user", userID, oldValues, newValues)
	})
	if err != nil {
		return api.User{}, err
	}
	return s.userByID(ctx, userID)
}

func (s *Service) DeactivateUser(ctx context.Context, actor Principal, userID uuid.UUID) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	if actor.UserID == userID {
		return apperr.Conflict("Нельзя деактивировать текущую учётную запись")
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		user, roles, err := loadManagedUser(ctx, q, actor, userID)
		if err != nil {
			return err
		}
		if !user.IsActive {
			return nil
		}
		if err := ensureNotLastSystemAdmin(ctx, q, roles, user.IsActive); err != nil {
			return err
		}
		if err := q.DeactivateUser(ctx, userID); err != nil {
			return err
		}
		if err := q.RevokeUserSessions(ctx, userID); err != nil {
			return err
		}
		return audit(ctx, q, actor, "user.deactivated", "user", userID, []byte(`{"is_active":true}`), []byte(`{"is_active":false}`))
	})
}

func (s *Service) ReactivateUser(ctx context.Context, actor Principal, userID uuid.UUID) (api.User, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.User{}, err
	}
	err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if _, _, err := loadManagedUser(ctx, q, actor, userID); err != nil {
			return err
		}
		if err := q.ReactivateUser(ctx, userID); err != nil {
			return err
		}
		return audit(ctx, q, actor, "user.reactivated", "user", userID, []byte(`{"is_active":false}`), []byte(`{"is_active":true}`))
	})
	if err != nil {
		return api.User{}, err
	}
	return s.userByID(ctx, userID)
}

// ResetUserPassword выдаёт новый временный пароль и завершает все сеансы пользователя. Пароль не пишется ни в аудит, ни в журнал.
func (s *Service) ResetUserPassword(ctx context.Context, actor Principal, userID uuid.UUID) (api.PasswordReset, error) {
	if err := actor.requireAdministrator(); err != nil {
		return api.PasswordReset{}, err
	}
	if actor.UserID == userID {
		return api.PasswordReset{}, apperr.Conflict("Свой пароль меняйте в настройках профиля")
	}
	password, err := generateTemporaryPassword()
	if err != nil {
		return api.PasswordReset{}, err
	}
	hash, err := s.hashPassword(ctx, password)
	if err != nil {
		return api.PasswordReset{}, err
	}
	var login string
	err = s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		user, _, err := loadManagedUser(ctx, q, actor, userID)
		if err != nil {
			return err
		}
		if !user.IsActive {
			return apperr.Conflict("Пользователь деактивирован: сначала верните его в работу")
		}
		login = user.Login
		if err := q.ResetUserPassword(ctx, db.ResetUserPasswordParams{ID: userID, PasswordHash: hash}); err != nil {
			return err
		}
		if err := q.RevokeUserSessions(ctx, userID); err != nil {
			return err
		}
		return audit(ctx, q, actor, "user.password_reset", "user", userID, nil, nil)
	})
	if err != nil {
		return api.PasswordReset{}, err
	}
	s.logins.clearLogin(login)
	return api.PasswordReset{Password: password}, nil
}

func (s *Service) RevokeUserSessions(ctx context.Context, actor Principal, userID uuid.UUID) error {
	if err := actor.requireAdministrator(); err != nil {
		return err
	}
	return s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if _, _, err := loadManagedUser(ctx, q, actor, userID); err != nil {
			return err
		}
		if err := q.RevokeUserSessions(ctx, userID); err != nil {
			return err
		}
		return audit(ctx, q, actor, "user.sessions_revoked", "user", userID, nil, nil)
	})
}

func (s *Service) userByID(ctx context.Context, userID uuid.UUID) (api.User, error) {
	user, err := s.queries.GetUserAnyByID(ctx, userID)
	if err != nil {
		return api.User{}, mapDatabaseError(err)
	}
	roles, err := s.queries.GetUserRoles(ctx, userID)
	if err != nil {
		return api.User{}, err
	}
	result := api.User{Id: user.ID, Login: user.Login, DisplayName: user.DisplayName,
		IsActive: user.IsActive, MustChangePassword: user.MustChangePassword, Roles: make([]api.Role, 0, len(roles))}
	for _, role := range roles {
		result.Roles = append(result.Roles, api.Role(role))
	}
	if student, err := s.queries.GetStudentByUserID(ctx, userID); err == nil {
		result.StudentId = &student.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return api.User{}, err
	}
	if teacher, err := s.queries.GetTeacherByUserID(ctx, userID); err == nil {
		result.TeacherId = &teacher.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return api.User{}, err
	}
	if parent, err := s.queries.GetParentByUserID(ctx, userID); err == nil {
		result.ParentId = &parent.ID
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return api.User{}, err
	}
	return result, nil
}
