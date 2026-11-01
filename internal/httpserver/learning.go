package httpserver

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"strconv"

	"github.com/google/uuid"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
)

func (a *API) listHomework(w http.ResponseWriter, r *http.Request) {
	studentID, err := optionalQueryUUID(r, "student_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListHomework(r.Context(), principal(r.Context()), studentID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createHomework(w http.ResponseWriter, r *http.Request) {
	var input api.CreateHomeworkJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateHomework(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) updateHomework(w http.ResponseWriter, r *http.Request) {
	id, version, ok := versionedPath(w, r)
	if !ok {
		return
	}
	var input api.UpdateHomeworkJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.UpdateHomework(r.Context(), principal(r.Context()), id, version, input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) publishHomework(w http.ResponseWriter, r *http.Request) {
	id, version, ok := versionedPath(w, r)
	if !ok {
		return
	}
	result, err := a.service.PublishHomework(r.Context(), principal(r.Context()), id, version)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) setHomeworkCompletion(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	var input api.SetHomeworkCompletionJSONBody
	if !decode(w, r, &input) {
		return
	}
	completed, err := a.service.SetHomeworkCompletion(r.Context(), principal(r.Context()), id, input.StudentId, input.Completed)
	respond(w, r, http.StatusOK, map[string]bool{"completed": completed}, err)
}

func (a *API) uploadFile(w http.ResponseWriter, r *http.Request) {
	limit := a.service.MaxUploadBytes()
	r.Body = http.MaxBytesReader(w, r.Body, limit+(1<<20))
	if err := r.ParseMultipartForm(limit); err != nil {
		WriteError(w, r, apperr.New(http.StatusRequestEntityTooLarge, "upload_too_large", "Превышен размер загрузки"))
		return
	}
	homeworkID, err := uuid.Parse(r.FormValue("homework_id"))
	if err != nil {
		WriteError(w, r, apperr.BadRequest("Некорректный идентификатор домашнего задания"))
		return
	}
	file, header, err := r.FormFile("file")
	if err != nil {
		WriteError(w, r, apperr.BadRequest("Выберите файл для загрузки"))
		return
	}
	defer file.Close()
	if header.Size > limit {
		WriteError(w, r, apperr.New(http.StatusRequestEntityTooLarge, "file_too_large", "Файл превышает разрешённый размер"))
		return
	}
	content, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		WriteError(w, r, apperr.BadRequest("Не удалось прочитать файл"))
		return
	}
	result, err := a.service.UploadFile(r.Context(), principal(r.Context()), homeworkID, header.Filename, content)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) downloadFile(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	studentID, err := optionalQueryUUID(r, "student_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.DownloadFile(r.Context(), principal(r.Context()), id, studentID)
	if err != nil {
		WriteError(w, r, err)
		return
	}
	disposition := mime.FormatMediaType("attachment", map[string]string{"filename": result.Metadata.OriginalName})
	if disposition == "" {
		disposition = "attachment"
	}
	w.Header().Set("Content-Type", result.Metadata.MimeType)
	w.Header().Set("Content-Disposition", disposition)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Length", strconv.FormatInt(int64(len(result.Content)), 10))
	w.Header().Set("Cache-Control", "private, no-store")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(result.Content)
}

func (a *API) listAnnouncements(w http.ResponseWriter, r *http.Request) {
	studentID, err := optionalQueryUUID(r, "student_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListAnnouncements(r.Context(), principal(r.Context()), studentID)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) createAnnouncement(w http.ResponseWriter, r *http.Request) {
	var input api.CreateAnnouncementJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.CreateAnnouncement(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusCreated, result, err)
}

func (a *API) listNotifications(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	result, err := a.service.ListNotifications(r.Context(), principal(r.Context()), limit, offset)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) markNotificationRead(w http.ResponseWriter, r *http.Request) {
	id, err := pathUUID(r)
	if err == nil {
		err = a.service.MarkNotificationRead(r.Context(), principal(r.Context()), id)
	}
	respond(w, r, http.StatusNoContent, nil, err)
}

func (a *API) setNotificationPreferences(w http.ResponseWriter, r *http.Request) {
	var input api.SetNotificationPreferencesJSONBody
	if !decode(w, r, &input) {
		return
	}
	result, err := a.service.SetNotificationPreferences(r.Context(), principal(r.Context()), input)
	respond(w, r, http.StatusOK, result, err)
}

func (a *API) classReport(w http.ResponseWriter, r *http.Request) {
	classID, err := pathUUID(r)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	periodID, err := queryUUID(r, "academic_period_id", true)
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	format := r.URL.Query().Get("format")
	switch format {
	case "", "json":
		result, err := a.service.ClassReport(r.Context(), principal(r.Context()), classID, periodID)
		respond(w, r, http.StatusOK, result, err)
	case "csv":
		result, err := a.service.ClassReportCSV(r.Context(), principal(r.Context()), classID, periodID)
		if err != nil {
			WriteError(w, r, err)
			return
		}
		w.Header().Set("Content-Type", "text/csv; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=class-%s.csv", classID))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(result)
	default:
		WriteError(w, r, apperr.BadRequest("Формат отчёта должен быть json или csv"))
	}
}

func (a *API) listAudit(w http.ResponseWriter, r *http.Request) {
	limit, offset := pagination(r)
	entityID, err := optionalQueryUUID(r, "entity_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	actorID, err := optionalQueryUUID(r, "actor_id")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	from, err := queryTime(r, "from")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	to, err := queryTime(r, "to")
	if err != nil {
		respond(w, r, 0, nil, err)
		return
	}
	result, err := a.service.ListAudit(r.Context(), principal(r.Context()), limit, offset, r.URL.Query().Get("entity_type"), r.URL.Query().Get("action"), entityID, actorID, from, to)
	respond(w, r, http.StatusOK, result, err)
}
