package httpserver

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/service"
)

func (a *API) registerAdminRoutes(r chi.Router) {
	read := r.With(a.authenticate)
	write := r.With(a.authenticate, a.requireCSRF)
	write.Post("/users/import", a.importUsers)
	write.Post("/users/{id}/reactivate", a.reactivateUser)
	write.Post("/users/{id}/password-reset", a.resetUserPassword)
	write.Post("/users/{id}/revoke-sessions", a.revokeUserSessions)
	write.Post("/enrollments/bulk", a.bulkEnroll)
	write.Post("/classes/{id}/promote", a.promoteClass)
	read.Get("/invitations", a.listInvitations)
	write.Post("/invitations", a.createInvitation)
	write.Delete("/invitations/{id}", a.revokeInvitation)
	r.Get("/invitations/lookup", a.lookupInvitation)
	r.Post("/auth/register", a.register)
	read.Get("/admin/schedule-entries", a.listAdminScheduleEntries)
	write.Patch("/schedule-entries/{id}", a.updateScheduleEntry)
	write.Delete("/schedule-entries/{id}", a.deleteScheduleEntry)
	write.Post("/schedule/generate", a.generateLessons)
	write.Post("/academic-periods/{id}/carry-over", a.carryOverAcademicPeriod)
	read.Get("/analytics/student", a.studentAnalytics)
	read.Get("/analytics/teacher", a.teacherAnalytics)
	read.Get("/analytics/school", a.schoolAnalytics)
	read.Get("/system/overview", a.systemOverview)
}

// clientIP возвращает адрес клиента для ограничения попыток. По умолчанию это адрес TCP-соединения:
// заголовки подделывает кто угодно. X-Real-IP учитывается только при включённом TRUST_PROXY_HEADERS,
// когда сервер закрыт от прямых подключений и Nginx перезаписывает заголовок значением $remote_addr.
func (a *API) clientIP(r *http.Request) string {
	if a.trustProxy {
		if forwarded := net.ParseIP(strings.TrimSpace(r.Header.Get("X-Real-IP"))); forwarded != nil {
			return forwarded.String()
		}
	}
	peer := r.RemoteAddr
	if host, _, err := net.SplitHostPort(peer); err == nil {
		peer = host
	}
	return peer
}

func noStore(w http.ResponseWriter) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
}

func (a *API) listUsers(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	query := r.URL.Query()
	classID, err := optionalQueryUUID(r, "class_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	filter := service.UserFilter{Search: query.Get("search"), Role: query.Get("role"), Status: query.Get("status"), ClassID: classID, Limit: limit, Offset: offset}
	result, total, err := a.service.ListUsers(r.Context(), principal(r.Context()), filter)
	if err == nil {
		w.Header().Set("X-Total-Count", strconv.FormatInt(total, 10))
	}
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) updateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateUserJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateUser(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) reactivateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ReactivateUser(r.Context(), principal(r.Context()), id)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) resetUserPassword(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	noStore(w)
	result, err := a.service.ResetUserPassword(r.Context(), principal(r.Context()), id)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) revokeUserSessions(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		err = a.service.RevokeUserSessions(r.Context(), principal(r.Context()), id)
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) importUsers(w http.ResponseWriter, r *http.Request) {
	var input api.UserImportRequest
	if !decode(w, r, &input) {
		return
	}
	noStore(w)
	result, err := a.service.ImportUsers(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) bulkEnroll(w http.ResponseWriter, r *http.Request) {
	var input api.BulkEnrollRequest
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.BulkEnroll(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) promoteClass(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.PromoteClassRequest
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.PromoteClass(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) listInvitations(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	result, err := a.service.ListInvitations(r.Context(), principal(r.Context()), limit, offset)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createInvitation(w http.ResponseWriter, r *http.Request) {
	var input api.InvitationInput
	if !decode(w, r, &input) {
		return
	}
	noStore(w)
	result, err := a.service.CreateInvitation(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) revokeInvitation(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		err = a.service.RevokeInvitation(r.Context(), principal(r.Context()), id)
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) lookupInvitation(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	result, err := a.service.LookupInvitation(r.Context(), r.URL.Query().Get("code"), a.clientIP(r))
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var input api.RegisterRequest
	if !decode(w, r, &input) {
		return
	}
	noStore(w)
	result, err := a.service.Register(r.Context(), input, a.clientIP(r), RequestID(r.Context()))
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) listAdminScheduleEntries(w http.ResponseWriter, r *http.Request) {
	classID, err := optionalQueryUUID(r, "class_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	teacherID, err := optionalQueryUUID(r, "teacher_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	periodID, err := optionalQueryUUID(r, "academic_period_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListAdminScheduleEntries(r.Context(), principal(r.Context()), classID, teacherID, periodID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) updateScheduleEntry(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.ScheduleEntryUpdate
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateScheduleEntry(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) deleteScheduleEntry(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		err = a.service.DeleteScheduleEntry(r.Context(), principal(r.Context()), id)
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) generateLessons(w http.ResponseWriter, r *http.Request) {
	var input api.GenerateLessonsRequest
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.GenerateLessons(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) carryOverAcademicPeriod(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.CarryOverRequest
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CarryOverPeriod(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) studentAnalytics(w http.ResponseWriter, r *http.Request) {
	studentID, err := optionalQueryUUID(r, "student_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	periodID, err := optionalQueryUUID(r, "academic_period_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.StudentAnalytics(r.Context(), principal(r.Context()), studentID, periodID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) teacherAnalytics(w http.ResponseWriter, r *http.Request) {
	periodID, err := optionalQueryUUID(r, "academic_period_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.TeacherAnalytics(r.Context(), principal(r.Context()), periodID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) schoolAnalytics(w http.ResponseWriter, r *http.Request) {
	periodID, err := optionalQueryUUID(r, "academic_period_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.SchoolAnalytics(r.Context(), principal(r.Context()), periodID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) systemOverview(w http.ResponseWriter, r *http.Request) {
	noStore(w)
	result, err := a.service.SystemOverview(r.Context(), principal(r.Context()))
	respond(w, r, http.StatusOK, result, err)
}
