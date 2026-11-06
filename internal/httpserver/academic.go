package httpserver

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"schooldiarysvu/internal/api"
)

func (a *API) registerAcademicRoutes(r chi.Router) {
	r.With(a.authenticate).Get("/users", a.listUsers)
	r.With(a.authenticate, a.requireCSRF).Post("/users", a.createUser)
	r.With(a.authenticate, a.requireCSRF).Patch("/users/{id}", a.updateUser)
	r.With(a.authenticate, a.requireCSRF).Delete("/users/{id}", a.deactivateUser)
	r.With(a.authenticate).Get("/academic-years", a.listAcademicYears)
	r.With(a.authenticate, a.requireCSRF).Post("/academic-years", a.createAcademicYear)
	r.With(a.authenticate, a.requireCSRF).Patch("/academic-years/{id}", a.updateAcademicYear)
	r.With(a.authenticate, a.requireCSRF).Delete("/academic-years/{id}", a.archiveAcademicYear)
	r.With(a.authenticate).Get("/academic-periods", a.listAcademicPeriods)
	r.With(a.authenticate, a.requireCSRF).Post("/academic-periods", a.createAcademicPeriod)
	r.With(a.authenticate, a.requireCSRF).Patch("/academic-periods/{id}", a.updateAcademicPeriod)
	r.With(a.authenticate, a.requireCSRF).Post("/academic-periods/{id}/close", a.closeAcademicPeriod)
	r.With(a.authenticate, a.requireCSRF).Put("/academic-periods/{id}/late-edits", a.setLateEdits)
	r.With(a.authenticate).Get("/classes", a.listClasses)
	r.With(a.authenticate, a.requireCSRF).Post("/classes", a.createClass)
	r.With(a.authenticate, a.requireCSRF).Patch("/classes/{id}", a.updateClass)
	r.With(a.authenticate, a.requireCSRF).Delete("/classes/{id}", a.archiveClass)
	r.With(a.authenticate).Get("/classes/{id}/students", a.listClassStudents)
	r.With(a.authenticate).Get("/subjects", a.listSubjects)
	r.With(a.authenticate, a.requireCSRF).Post("/subjects", a.createSubject)
	r.With(a.authenticate, a.requireCSRF).Patch("/subjects/{id}", a.updateSubject)
	r.With(a.authenticate, a.requireCSRF).Delete("/subjects/{id}", a.archiveSubject)
	r.With(a.authenticate).Get("/rooms", a.listRooms)
	r.With(a.authenticate, a.requireCSRF).Post("/rooms", a.createRoom)
	r.With(a.authenticate, a.requireCSRF).Patch("/rooms/{id}", a.updateRoom)
	r.With(a.authenticate, a.requireCSRF).Delete("/rooms/{id}", a.archiveRoom)
	r.With(a.authenticate).Get("/enrollments", a.listEnrollments)
	r.With(a.authenticate, a.requireCSRF).Post("/enrollments", a.createEnrollment)
	r.With(a.authenticate, a.requireCSRF).Patch("/enrollments/{id}", a.updateEnrollment)
	r.With(a.authenticate).Get("/teaching-assignments", a.listTeachingAssignments)
	r.With(a.authenticate, a.requireCSRF).Post("/teaching-assignments", a.createTeachingAssignment)
	r.With(a.authenticate, a.requireCSRF).Patch("/teaching-assignments/{id}", a.updateTeachingAssignment)
	r.With(a.authenticate).Get("/class-teachers", a.listClassTeachers)
	r.With(a.authenticate, a.requireCSRF).Post("/class-teachers", a.assignClassTeacher)
	r.With(a.authenticate, a.requireCSRF).Patch("/class-teachers/{id}", a.updateClassTeacher)
	r.With(a.authenticate).Get("/parent-student", a.listParentStudentLinks)
	r.With(a.authenticate, a.requireCSRF).Post("/parent-student", a.linkParentStudent)
	r.With(a.authenticate, a.requireCSRF).Put("/parent-student", a.updateParentStudent)
	r.With(a.authenticate, a.requireCSRF).Delete("/parent-student", a.removeParentStudent)
	r.With(a.authenticate).Get("/parent-student/suggestions", a.suggestChildren)
	r.With(a.authenticate).Get("/children", a.listChildren)
	r.With(a.authenticate).Get("/school/settings", a.getSchoolSettings)
	r.With(a.authenticate, a.requireCSRF).Put("/school/settings", a.updateSchoolSettings)
	r.With(a.authenticate).Get("/grade-scales", a.listGradeScales)
	r.With(a.authenticate, a.requireCSRF).Post("/grade-scales", a.createGradeScale)
	r.With(a.authenticate, a.requireCSRF).Patch("/grade-scales/{id}", a.updateGradeScale)

	r.With(a.authenticate).Get("/schedule", a.getSchedule)
	r.With(a.authenticate).Get("/schedule/current", a.getCurrentLessons)
	r.With(a.authenticate).Get("/schedule-entries", a.listScheduleEntries)
	r.With(a.authenticate, a.requireCSRF).Post("/schedule-entries", a.createScheduleEntry)
	r.With(a.authenticate, a.requireCSRF).Post("/lessons", a.createLesson)
	r.With(a.authenticate).Get("/lessons/{id}", a.getLesson)
	r.With(a.authenticate, a.requireCSRF).Patch("/lessons/{id}", a.updateLesson)
	r.With(a.authenticate).Get("/lessons/{id}/works", a.listLessonWorks)
	r.With(a.authenticate, a.requireCSRF).Post("/lesson-works", a.createLessonWork)
	r.With(a.authenticate).Get("/journal", a.getJournal)
	r.With(a.authenticate).Get("/grades", a.listGrades)
	r.With(a.authenticate, a.requireCSRF).Post("/grades", a.createGrade)
	r.With(a.authenticate, a.requireCSRF).Patch("/grades/{id}", a.updateGrade)
	r.With(a.authenticate, a.requireCSRF).Delete("/grades/{id}", a.deleteGrade)
	r.With(a.authenticate).Get("/grades/{id}/history", a.gradeHistory)
	r.With(a.authenticate, a.requireCSRF).Post("/term-grades", a.upsertTermGrade)
	r.With(a.authenticate).Get("/attendance", a.listAttendance)
	r.With(a.authenticate, a.requireCSRF).Put("/attendance", a.upsertAttendance)
	r.With(a.authenticate, a.requireCSRF).Post("/attendance/bulk", a.markAllPresent)

	r.With(a.authenticate).Get("/homework", a.listHomework)
	r.With(a.authenticate, a.requireCSRF).Post("/homework", a.createHomework)
	r.With(a.authenticate, a.requireCSRF).Patch("/homework/{id}", a.updateHomework)
	r.With(a.authenticate, a.requireCSRF).Post("/homework/{id}/publish", a.publishHomework)
	r.With(a.authenticate, a.requireCSRF).Post("/homework/{id}/copy", a.copyHomework)
	r.With(a.authenticate, a.requireCSRF).Put("/homework/{id}/completion", a.setHomeworkCompletion)
	r.With(a.authenticate, a.requireCSRF).Post("/files", a.uploadFile)
	r.With(a.authenticate).Get("/files/{id}", a.downloadFile)
	r.With(a.authenticate, a.requireCSRF).Delete("/files/{id}", a.archiveFile)
	r.With(a.authenticate).Get("/announcements", a.listAnnouncements)
	r.With(a.authenticate, a.requireCSRF).Post("/announcements", a.createAnnouncement)
	r.With(a.authenticate, a.requireCSRF).Delete("/announcements/{id}", a.archiveAnnouncement)
	r.With(a.authenticate).Get("/notifications/digest", a.getNotificationDigest)
	r.With(a.authenticate).Get("/notifications", a.listNotifications)
	r.With(a.authenticate, a.requireCSRF).Post("/notifications/{id}/read", a.markNotificationRead)
	r.With(a.authenticate, a.requireCSRF).Put("/notification-preferences", a.setNotificationPreferences)
	r.With(a.authenticate).Get("/reports/classes/{id}", a.classReport)
	r.With(a.authenticate).Get("/reports/classes/{id}/attendance", a.classAttendanceReport)
	r.With(a.authenticate).Get("/reports/students/{id}", a.studentReport)
	r.With(a.authenticate).Get("/reports/unfilled", a.unfilledLessonsReport)
	r.With(a.authenticate).Get("/reports/subjects/{id}", a.subjectReport)
	r.With(a.authenticate).Get("/audit", a.listAudit)
	a.registerAdminRoutes(r)
	a.registerSubstitutionRoutes(r)
}

func (a *API) createUser(w http.ResponseWriter, r *http.Request) {
	var input api.CreateUserJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateUser(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) deactivateUser(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		err = a.service.DeactivateUser(r.Context(), principal(r.Context()), id)
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) listAcademicYears(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.ListAcademicYears(r.Context())
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createAcademicYear(w http.ResponseWriter, r *http.Request) {
	var input api.AcademicYearInput
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateAcademicYear(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) listAcademicPeriods(w http.ResponseWriter, r *http.Request) {
	yearID, err := optionalQueryUUID(r, "academic_year_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListAcademicPeriods(r.Context(), yearID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createAcademicPeriod(w http.ResponseWriter, r *http.Request) {
	var input api.CreateAcademicPeriodJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateAcademicPeriod(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) closeAcademicPeriod(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		err = a.service.CloseAcademicPeriod(r.Context(), principal(r.Context()), id)
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) setLateEdits(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.SetLateEditsJSONBody
	if !decode(w, r, &input) {
		return
	}
	err = a.service.SetLateEdits(r.Context(), principal(r.Context()), id, input.Allowed)
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) listClasses(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.ListClasses(r.Context())
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createClass(w http.ResponseWriter, r *http.Request) {
	var input api.CreateClassJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateClass(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) listClassStudents(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	date, err := queryDate(r, "date", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListClassStudents(r.Context(), principal(r.Context()), id, date.Time)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) listSubjects(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.ListSubjects(r.Context())
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createSubject(w http.ResponseWriter, r *http.Request) {
	var input api.CreateSubjectJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateSubject(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) listRooms(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.ListRooms(r.Context())
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createRoom(w http.ResponseWriter, r *http.Request) {
	var input api.CreateRoomJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateRoom(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) listEnrollments(w http.ResponseWriter, r *http.Request) {
	classID, err := queryUUID(r, "class_id", false)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListEnrollments(r.Context(), principal(r.Context()), classID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createEnrollment(w http.ResponseWriter, r *http.Request) {
	var input api.CreateEnrollmentJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateEnrollment(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) listTeachingAssignments(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.ListTeachingAssignments(r.Context(), principal(r.Context()))
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createTeachingAssignment(w http.ResponseWriter, r *http.Request) {
	var input api.CreateTeachingAssignmentJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateTeachingAssignment(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) assignClassTeacher(w http.ResponseWriter, r *http.Request) {
	var input api.AssignClassTeacherJSONBody
	if !decode(w, r, &input) {
		return
	}
	err := a.service.AssignClassTeacher(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) linkParentStudent(w http.ResponseWriter, r *http.Request) {
	var input api.LinkParentStudentJSONBody
	if !decode(w, r, &input) {
		return
	}
	err := a.service.LinkParentStudent(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) listChildren(w http.ResponseWriter, r *http.Request) {
	result, err := a.service.ListChildren(r.Context(), principal(r.Context()))
	respond(w, r, http.StatusOK, result, err)
}
