package httpserver

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"

	"schooldiarysvu/internal/apperr"
	"schooldiarysvu/internal/service"
)

type principalKey struct{}

type userResponse struct {
	ID                 uuid.UUID  `json:"id"`
	Login              string     `json:"login"`
	DisplayName        string     `json:"display_name"`
	IsActive           bool       `json:"is_active"`
	MustChangePassword bool       `json:"must_change_password"`
	Roles              []string   `json:"roles"`
	StudentID          *uuid.UUID `json:"student_id,omitempty"`
	TeacherID          *uuid.UUID `json:"teacher_id,omitempty"`
	ParentID           *uuid.UUID `json:"parent_id,omitempty"`
}

func mapPrincipal(actor service.Principal) userResponse {
	return userResponse{ID: actor.UserID, Login: actor.Login, DisplayName: actor.DisplayName,
		IsActive: true, MustChangePassword: actor.MustChangePassword, Roles: actor.Roles,
		StudentID: actor.StudentID, TeacherID: actor.TeacherID, ParentID: actor.ParentID}
}

func principal(ctx context.Context) service.Principal {
	actor, _ := ctx.Value(principalKey{}).(service.Principal)
	return actor
}

func (a *API) authenticate(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sessionCookie, err := r.Cookie(service.SessionCookie)
		if err != nil {
			WriteError(w, r, apperr.Unauthorized())
			return
		}
		csrfToken := ""
		if cookie, err := r.Cookie(service.CSRFCookie); err == nil {
			csrfToken = cookie.Value
		}
		actor, err := a.service.Authenticate(r.Context(), sessionCookie.Value, csrfToken)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		actor.RequestID = RequestID(r.Context())
		if actor.MustChangePassword && r.URL.Path != "/api/v1/me" && r.URL.Path != "/api/v1/auth/password" && r.URL.Path != "/api/v1/auth/logout" {
			WriteError(w, r, apperr.New(http.StatusForbidden, "password_change_required", "Сначала смените временный пароль"))
			return
		}
		ctx := context.WithValue(r.Context(), principalKey{}, actor)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func (a *API) requireCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := a.service.ValidateCSRF(principal(r.Context()), r.Header.Get("X-CSRF-Token")); err != nil {
			WriteError(w, r, err)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Login    string `json:"login"`
		Password string `json:"password"`
	}
	if err := DecodeJSON(w, r, &input); err != nil {
		WriteError(w, r, err)
		return
	}
	session, err := a.service.Login(r.Context(), input.Login, input.Password, a.clientIP(r))
	if err != nil {
		WriteError(w, r, err)
		return
	}
	a.setSessionCookies(w, session)
	WriteJSON(w, http.StatusOK, struct {
		userResponse
		CSRFToken string `json:"csrf_token"`
	}{userResponse: mapPrincipal(session.Principal), CSRFToken: session.CSRFToken})
}

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	WriteJSON(w, http.StatusOK, mapPrincipal(principal(r.Context())))
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	if err := a.service.Logout(r.Context(), principal(r.Context())); err != nil {
		WriteError(w, r, err)
		return
	}
	a.clearSessionCookies(w)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) changePassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		OldPassword string `json:"old_password"`
		NewPassword string `json:"new_password"`
	}
	if err := DecodeJSON(w, r, &input); err != nil {
		WriteError(w, r, err)
		return
	}
	session, err := a.service.ChangePassword(r.Context(), principal(r.Context()), input.OldPassword, input.NewPassword)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	a.setSessionCookies(w, session)
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) setSessionCookies(w http.ResponseWriter, session service.Session) {
	maxAge := int(time.Until(session.ExpiresAt).Seconds())
	http.SetCookie(w, &http.Cookie{Name: service.SessionCookie, Value: session.Token, Path: "/",
		HttpOnly: true, Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge, Expires: session.ExpiresAt})
	http.SetCookie(w, &http.Cookie{Name: service.CSRFCookie, Value: session.CSRFToken, Path: "/",
		Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: maxAge, Expires: session.ExpiresAt})
}

func (a *API) clearSessionCookies(w http.ResponseWriter) {
	for _, name := range []string{service.SessionCookie, service.CSRFCookie} {
		http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: "/", HttpOnly: name == service.SessionCookie,
			Secure: a.cookieSecure, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(1, 0)})
	}
}
