package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

const SessionCookie = "diary_session"
const CSRFCookie = "diary_csrf"

type Session struct {
	Token     string
	CSRFToken string
	ExpiresAt time.Time
	Principal Principal
}

type loginAttempt struct {
	failures     int
	blockedUntil time.Time
}

type loginLimiter struct {
	mu       sync.Mutex
	attempts map[string]loginAttempt
}

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{attempts: make(map[string]loginAttempt)}
}

// check возвращает nil, если попытка разрешена, иначе 429 с оставшимся временем блокировки.
func (l *loginLimiter) check(key string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt := l.attempts[key]
	if wait := time.Until(attempt.blockedUntil); wait > 0 {
		return apperr.TooManyAttempts(wait)
	}
	if !attempt.blockedUntil.IsZero() {
		delete(l.attempts, key)
	}
	return nil
}

func (l *loginLimiter) failed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	attempt := l.attempts[key]
	attempt.failures++
	if attempt.failures >= 5 {
		attempt.blockedUntil = time.Now().Add(15 * time.Minute)
		attempt.failures = 0
	}
	l.attempts[key] = attempt
}

// clearLogin снимает блокировку всех ключей, относящихся к логину (после сброса пароля администратором).
func (l *loginLimiter) clearLogin(login string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	for key := range l.attempts {
		if strings.HasSuffix(key, "\x00"+login) {
			delete(l.attempts, key)
		}
	}
}

func (l *loginLimiter) succeeded(key string) {
	l.mu.Lock()
	delete(l.attempts, key)
	l.mu.Unlock()
}

func (s *Service) Login(ctx context.Context, login, password, rateLimitKey string) (Session, error) {
	login = strings.ToLower(strings.TrimSpace(login))
	if err := s.addresses.check(rateLimitKey); err != nil {
		return Session{}, err
	}
	if err := s.logins.check(rateLimitKey + "\x00" + login); err != nil {
		return Session{}, err
	}
	user, err := s.queries.GetUserByLogin(ctx, login)
	if errors.Is(err, pgx.ErrNoRows) {
		// Тот же хеш для неизвестного логина: время ответа не выдаёт, существует ли пользователь.
		if _, err := s.verifyPassword(ctx, s.dummyHash, password); err != nil {
			return Session{}, err
		}
		s.logins.failed(rateLimitKey + "\x00" + login)
		s.addresses.failed(rateLimitKey)
		return Session{}, apperr.Unauthorized()
	}
	if err != nil {
		return Session{}, err
	}
	valid, err := s.verifyPassword(ctx, user.PasswordHash, password)
	if err != nil {
		return Session{}, err
	}
	if !valid {
		s.logins.failed(rateLimitKey + "\x00" + login)
		s.addresses.failed(rateLimitKey)
		return Session{}, apperr.Unauthorized()
	}
	s.logins.succeeded(rateLimitKey + "\x00" + login)
	if err := s.queries.TouchLastLogin(ctx, user.ID); err != nil {
		return Session{}, err
	}
	return s.createSession(ctx, user.ID, user.Login, user.DisplayName, user.MustChangePassword)
}

func (s *Service) createSession(ctx context.Context, userID uuid.UUID, login, displayName string, mustChange bool) (Session, error) {
	token, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	csrfToken, err := randomToken()
	if err != nil {
		return Session{}, err
	}
	expiresAt := time.Now().Add(s.sessionTTL)
	created, err := s.queries.CreateSession(ctx, db.CreateSessionParams{
		UserID: userID, TokenHash: hashToken(token), CsrfHash: hashToken(csrfToken), ExpiresAt: expiresAt,
	})
	if err != nil {
		return Session{}, err
	}
	principal, err := s.loadPrincipal(ctx, db.GetSessionByTokenHashRow{
		SessionID: created.ID, UserID: userID, CsrfHash: hashToken(csrfToken), ExpiresAt: expiresAt,
		Login: login, DisplayName: displayName, MustChangePassword: mustChange,
	})
	if err != nil {
		return Session{}, err
	}
	principal.CSRFToken = csrfToken
	return Session{Token: token, CSRFToken: csrfToken, ExpiresAt: expiresAt, Principal: principal}, nil
}

func (s *Service) Authenticate(ctx context.Context, token, csrfToken string) (Principal, error) {
	if token == "" {
		return Principal{}, apperr.Unauthorized()
	}
	row, err := s.queries.GetSessionByTokenHash(ctx, hashToken(token))
	if errors.Is(err, pgx.ErrNoRows) {
		return Principal{}, apperr.Unauthorized()
	}
	if err != nil {
		return Principal{}, err
	}
	principal, err := s.loadPrincipal(ctx, row)
	if err != nil {
		return Principal{}, err
	}
	if csrfToken != "" && subtle.ConstantTimeCompare(hashToken(csrfToken), row.CsrfHash) == 1 {
		principal.CSRFToken = csrfToken
	}
	return principal, nil
}

func (s *Service) loadPrincipal(ctx context.Context, row db.GetSessionByTokenHashRow) (Principal, error) {
	roles, err := s.queries.GetUserRoles(ctx, row.UserID)
	if err != nil {
		return Principal{}, err
	}
	principal := Principal{UserID: row.UserID, SessionID: row.SessionID, Login: row.Login,
		DisplayName: row.DisplayName, Roles: roles, CSRFHash: row.CsrfHash, MustChangePassword: row.MustChangePassword}
	// Профиль без соответствующей роли (роль отозвана) доступа не даёт: проверяем и профиль, и роль.
	if principal.HasRole("student") {
		if student, err := s.queries.GetStudentByUserID(ctx, row.UserID); err == nil {
			principal.StudentID = &student.ID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, err
		}
	}
	if principal.HasRole("teacher") || principal.HasRole("class_teacher") {
		if teacher, err := s.queries.GetTeacherByUserID(ctx, row.UserID); err == nil {
			principal.TeacherID = &teacher.ID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, err
		}
	}
	if principal.HasRole("parent") {
		if parent, err := s.queries.GetParentByUserID(ctx, row.UserID); err == nil {
			principal.ParentID = &parent.ID
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return Principal{}, err
		}
	}
	return principal, nil
}

func (s *Service) Logout(ctx context.Context, actor Principal) error {
	return s.queries.RevokeSession(ctx, actor.SessionID)
}

func (s *Service) ChangePassword(ctx context.Context, actor Principal, oldPassword, newPassword string) (Session, error) {
	user, err := s.queries.GetUserByID(ctx, actor.UserID)
	if err != nil {
		return Session{}, mapDatabaseError(err)
	}
	valid, err := s.verifyPassword(ctx, user.PasswordHash, oldPassword)
	if err != nil {
		return Session{}, err
	}
	if !valid {
		return Session{}, apperr.Unprocessable("Текущий пароль указан неверно")
	}
	if len([]rune(newPassword)) < 12 || len(newPassword) > 128 {
		return Session{}, apperr.Unprocessable("Новый пароль должен содержать от 12 до 128 символов")
	}
	if oldPassword == newPassword {
		return Session{}, apperr.Unprocessable("Новый пароль должен отличаться от текущего")
	}
	hash, err := s.hashPassword(ctx, newPassword)
	if err != nil {
		return Session{}, err
	}
	if err := s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		if err := q.UpdatePassword(ctx, db.UpdatePasswordParams{ID: actor.UserID, PasswordHash: hash}); err != nil {
			return err
		}
		if err := q.RevokeUserSessions(ctx, actor.UserID); err != nil {
			return err
		}
		return audit(ctx, q, actor, "password.changed", "user", actor.UserID, nil, nil)
	}); err != nil {
		return Session{}, err
	}
	return s.createSession(ctx, actor.UserID, actor.Login, actor.DisplayName, false)
}

func (s *Service) ValidateCSRF(actor Principal, token string) error {
	if token == "" || actor.CSRFToken == "" || subtle.ConstantTimeCompare([]byte(token), []byte(actor.CSRFToken)) != 1 ||
		subtle.ConstantTimeCompare(hashToken(token), actor.CSRFHash) != 1 {
		return apperr.Forbidden()
	}
	return nil
}

func randomToken() (string, error) {
	value := make([]byte, 32)
	if _, err := rand.Read(value); err != nil {
		return "", fmt.Errorf("generate session token: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashToken(token string) []byte {
	hash := sha256.Sum256([]byte(token))
	return hash[:]
}

func dateOnly(value time.Time) pgtype.Date {
	return pgtype.Date{Time: value, Valid: true}
}
