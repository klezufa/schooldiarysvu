package httpserver

import (
	"net/http"

	"github.com/google/uuid"

	"schooldiarysvu/internal/api"
)

func (a *API) getSchedule(w http.ResponseWriter, r *http.Request) {
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
	studentID, err := optionalQueryUUID(r, "student_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
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
	if classID != nil || teacherID != nil {
		result, err := a.service.GetSchoolSchedule(r.Context(), principal(r.Context()), from.Time, to.Time, classID, teacherID)
		respond(w, r, http.StatusOK, result, err)
		return
	}
	result, err := a.service.GetSchedule(r.Context(), principal(r.Context()), from.Time, to.Time, studentID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createLesson(w http.ResponseWriter, r *http.Request) {
	var input api.CreateLessonJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateLesson(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) getLesson(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		var result api.Lesson
		result, err = a.service.GetLesson(r.Context(), principal(r.Context()), id)
		respond(w, r, http.StatusOK, result, err)
		return
	}
	respond(w, r, 0, nil, err)
}

func (a *API) updateLesson(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	version, err := queryVersion(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateLessonJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateLesson(r.Context(), principal(r.Context()), id, version, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) listLessonWorks(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListLessonWorks(r.Context(), principal(r.Context()), id)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createLessonWork(w http.ResponseWriter, r *http.Request) {
	var input api.CreateLessonWorkJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateLessonWork(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) getJournal(w http.ResponseWriter, r *http.Request) {
	classID, err := queryUUID(r, "class_id", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	subjectID, err := queryUUID(r, "subject_id", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	periodID, err := queryUUID(r, "academic_period_id", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.GetJournal(r.Context(), principal(r.Context()), classID, subjectID, periodID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createGrade(w http.ResponseWriter, r *http.Request) {
	var input api.CreateGradeJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateGrade(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) listGrades(w http.ResponseWriter, r *http.Request) {
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
	result, err := a.service.ListGrades(r.Context(), principal(r.Context()), studentID, periodID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) updateGrade(w http.ResponseWriter, r *http.Request) {
	id, version, ok := versionedPath(w, r)
	if !ok {
		return
	}
	var input api.UpdateGradeJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateGrade(r.Context(), principal(r.Context()), id, version, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) deleteGrade(w http.ResponseWriter, r *http.Request) {
	id, version, ok := versionedPath(w, r)
	if !ok {
		return
	}
	err := a.service.DeleteGrade(r.Context(), principal(r.Context()), id, version)
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) gradeHistory(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.GradeHistory(r.Context(), principal(r.Context()), id)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) upsertTermGrade(w http.ResponseWriter, r *http.Request) {
	var input api.UpsertTermGradeJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpsertTermGrade(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) listAttendance(w http.ResponseWriter, r *http.Request) {
	lessonID, err := queryUUID(r, "lesson_id", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListAttendance(r.Context(), principal(r.Context()), lessonID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) upsertAttendance(w http.ResponseWriter, r *http.Request) {
	var input api.UpsertAttendanceJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpsertAttendance(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) markAllPresent(w http.ResponseWriter, r *http.Request) {
	var input api.MarkAllPresentJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.MarkAllPresent(r.Context(), principal(r.Context()), input.LessonId, r.Header.Get("Idempotency-Key"))
	respond(w, r, http.StatusOK, result, err)
}

func versionedPath(w http.ResponseWriter, r *http.Request) (uuid.UUID, int32, bool) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return uuid.Nil, 0, false
	}
	version, err := queryVersion(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return uuid.Nil, 0, false
	}
	return id, version, true
}

func optionalQueryUUID(r *http.Request, name string) (*uuid.UUID, error) {
	if r.URL.Query().Get(name) == "" {
		return nil, nil
	}
	id, err := queryUUID(r, name, true)
	if err != nil {
		return nil, err
	}
	return &id, nil
}
