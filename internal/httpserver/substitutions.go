package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"schooldiarysvu/internal/api"
)

func (a *API) registerSubstitutionRoutes(r chi.Router) {
	r.With(a.authenticate).Get("/substitutions", a.listSubstitutions)
	r.With(a.authenticate).Get("/substitutions/candidates", a.listSubstitutionCandidates)
	r.With(a.authenticate, a.requireCSRF).Put("/lessons/{id}/substitution", a.setLessonSubstitution)
	r.With(a.authenticate, a.requireCSRF).Delete("/lessons/{id}/substitution", a.removeLessonSubstitution)
	r.With(a.authenticate, a.requireCSRF).Post("/journal-archives", a.issueJournalArchive)
	r.With(a.authenticate).Get("/journal-archives/verify", a.verifyJournalArchive)
}

func (a *API) listSubstitutions(w http.ResponseWriter, r *http.Request) {
	from, err := queryDate(r, "from", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	to, err := queryDate(r, "to", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	teacherID, err := optionalQueryUUID(r, "teacher_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListSubstitutions(r.Context(), principal(r.Context()), from.Time, to.Time, teacherID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) listSubstitutionCandidates(w http.ResponseWriter, r *http.Request) {
	lessonID, err := queryUUID(r, "lesson_id", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.SubstitutionCandidates(r.Context(), principal(r.Context()), lessonID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) setLessonSubstitution(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.SetLessonSubstitutionJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.SetLessonSubstitution(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) removeLessonSubstitution(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		err = a.service.RemoveLessonSubstitution(r.Context(), principal(r.Context()), id)
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) issueJournalArchive(w http.ResponseWriter, r *http.Request) {
	var input api.IssueJournalArchiveJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.IssueJournalArchive(r.Context(), principal(r.Context()), input.ClassId)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) verifyJournalArchive(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.VerifyJournalArchive(r.Context(), principal(r.Context()), r.URL.Query().Get("code"))
	respond(w, r, http.StatusOK, result, err)
}
