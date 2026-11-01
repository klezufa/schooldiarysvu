package httpserver

import (
	"net/http"
	"time"

	"github.com/google/uuid"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
)

func (a *API) updateAcademicYear(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.AcademicYearInput
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateAcademicYear(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) archiveAcademicYear(w http.ResponseWriter, r *http.Request) {
	a.archive(w, r, "academic_year")
}

func (a *API) updateAcademicPeriod(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateAcademicPeriodJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateAcademicPeriod(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) updateClass(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateClassJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateClass(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) archiveClass(w http.ResponseWriter, r *http.Request) {
	a.archive(w, r, "class")
}

func (a *API) updateSubject(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateSubjectJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateSubject(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) archiveSubject(w http.ResponseWriter, r *http.Request) {
	a.archive(w, r, "subject")
}

func (a *API) updateRoom(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateRoomJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateRoom(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) archiveRoom(w http.ResponseWriter, r *http.Request) {
	a.archive(w, r, "room")
}

func (a *API) archive(w http.ResponseWriter, r *http.Request, kind string) {
	id, err := pathUUID(r)
	if err == nil {
		if kind == "academic_year" {
			err = a.service.ArchiveAcademicYear(r.Context(), principal(r.Context()), id)
		} else {
			err = a.service.ArchiveEntity(r.Context(), principal(r.Context()), kind, id)
		}
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) updateEnrollment(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateEnrollmentJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateEnrollment(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) updateTeachingAssignment(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateTeachingAssignmentJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateTeachingAssignment(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) updateClassTeacher(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateClassTeacherJSONBody
	if !decode(w, r, &input) {
		return
	}
	err = a.service.UpdateClassTeacher(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) listClassTeachers(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.ListClassTeachers(r.Context(), principal(r.Context()))
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) listParentStudentLinks(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.ListParentStudentLinks(r.Context(), principal(r.Context()))
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) updateParentStudent(w http.ResponseWriter, r *http.Request) {
	var input api.LinkParentStudentJSONBody
	if !decode(w, r, &input) {
		return
	}
	err := a.service.UpdateParentStudent(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) getSchoolSettings(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.GetSchoolSettings(r.Context())
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) updateSchoolSettings(w http.ResponseWriter, r *http.Request) {
	var input api.SchoolSettings
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateSchoolSettings(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) listGradeScales(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.ListGradeScales(r.Context())
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createGradeScale(w http.ResponseWriter, r *http.Request) {
	var input api.CreateGradeScaleJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateGradeScale(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) updateGradeScale(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.UpdateGradeScaleJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateGradeScale(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createScheduleEntry(w http.ResponseWriter, r *http.Request) {
	var input api.ScheduleEntryInput
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateScheduleEntry(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) listScheduleEntries(w http.ResponseWriter, r *http.Request) {
	weekStart, err := queryDate(r, "week_start", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	studentID, err := optionalQueryUUID(r, "student_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListScheduleEntries(r.Context(), principal(r.Context()), weekStart.Time, studentID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) getCurrentLessons(w http.ResponseWriter, r *http.Request) {
	studentID, err := optionalQueryUUID(r, "student_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.CurrentLessons(r.Context(), principal(r.Context()), studentID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) copyHomework(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.CopyHomeworkJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CopyHomework(r.Context(), principal(r.Context()), id, input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) archiveFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		err = a.service.ArchiveFile(r.Context(), principal(r.Context()), id)
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) archiveAnnouncement(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		err = a.service.ArchiveAnnouncement(r.Context(), principal(r.Context()), id)
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) getNotificationDigest(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.NotificationDigest(r.Context(), principal(r.Context()), r.URL.Query().Get("frequency"))
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) studentReport(w http.ResponseWriter, r *http.Request) {
	studentID, periodID, ok := reportIDs(w, r)
	if !ok {
		return
	}
	result, err := a.service.StudentReport(r.Context(), principal(r.Context()), studentID, periodID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) classAttendanceReport(w http.ResponseWriter, r *http.Request) {
	classID, periodID, ok := reportIDs(w, r)
	if !ok {
		return
	}
	result, err := a.service.ClassAttendanceReport(r.Context(), principal(r.Context()), classID, periodID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) unfilledLessonsReport(w http.ResponseWriter, r *http.Request) {
	periodID, err := queryUUID(r, "academic_period_id", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.UnfilledLessonsReport(r.Context(), principal(r.Context()), periodID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) subjectReport(w http.ResponseWriter, r *http.Request) {
	subjectID, periodID, ok := reportIDs(w, r)
	if !ok {
		return
	}
	classID, err := queryUUID(r, "class_id", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.SubjectReport(r.Context(), principal(r.Context()), subjectID, classID, periodID)
	respond(w, r, http.StatusOK, result, err)
}

func reportIDs(w http.ResponseWriter, r *http.Request) (uuid.UUID, uuid.UUID, bool) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return uuid.Nil, uuid.Nil, false
	}
	periodID, err := queryUUID(r, "academic_period_id", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return uuid.Nil, uuid.Nil, false
	}
	return id, periodID, true
}

func queryTime(r *http.Request, name string) (*time.Time, error) {
	value := r.URL.Query().Get(name)
	if value == "" {
		return nil, nil
	}
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, apperr.BadRequest("Некорректная дата и время " + name)
	}
	return &parsed, nil
}
