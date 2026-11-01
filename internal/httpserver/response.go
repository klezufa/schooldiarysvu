package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"schooldiarysvu/internal/apperr"
)

type ErrorResponse struct {
	Code        string            `json:"code"`
	Message     string            `json:"message"`
	RequestID   string            `json:"request_id"`
	FieldErrors map[string]string `json:"field_errors,omitempty"`
}

type Error = apperr.Error

func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func WriteError(w http.ResponseWriter, r *http.Request, err error) {
	var apiErr *Error
	if !errors.As(err, &apiErr) {
		apiErr = classifyUnhandled(r, err)
	}
	if apiErr.RetryAfter > 0 {
		seconds := int((apiErr.RetryAfter + time.Second - 1) / time.Second)
		w.Header().Set("Retry-After", strconv.Itoa(seconds))
	}
	WriteJSON(w, apiErr.Status, ErrorResponse{
		Code: apiErr.Code, Message: apiErr.Message, RequestID: RequestID(r.Context()), FieldErrors: apiErr.FieldErrors,
	})
}

// StatusClientClosedRequest — нестандартный код (как в Nginx): клиент оборвал запрос, не дождавшись ответа.
const StatusClientClosedRequest = 499

// classifyUnhandled превращает неклассифицированную ошибку в ответ. Отмена запроса клиентом (переход на другую страницу,
// закрытая вкладка) и таймаут — штатные ситуации, а не сбой сервера; всё остальное — 500 с причиной только в журнале.
func classifyUnhandled(r *http.Request, err error) *Error {
	switch {
	case errors.Is(err, context.Canceled):
		slog.DebugContext(r.Context(), "request canceled by client", "request_id", RequestID(r.Context()), "path", r.URL.Path)
		return &Error{Status: StatusClientClosedRequest, Code: "request_canceled", Message: "Запрос отменён"}
	case errors.Is(err, context.DeadlineExceeded):
		slog.WarnContext(r.Context(), "request deadline exceeded", "request_id", RequestID(r.Context()), "method", r.Method, "path", r.URL.Path)
		return &Error{Status: http.StatusGatewayTimeout, Code: "timeout", Message: "Сервер не успел ответить, повторите запрос"}
	}
	slog.ErrorContext(r.Context(), "unhandled error", "request_id", RequestID(r.Context()), "method", r.Method, "path", r.URL.Path, "error", err)
	return &Error{Status: http.StatusInternalServerError, Code: "internal_error", Message: "Внутренняя ошибка сервера"}
}

func respond(w http.ResponseWriter, r *http.Request, status int, value any, err error) {
	if err != nil {
		WriteError(w, r, err)
		return
	}
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	WriteJSON(w, status, value)
}

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	if err := DecodeJSON(w, r, dst); err != nil {
		WriteError(w, r, err)
		return false
	}
	return true
}

func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return &Error{Status: http.StatusBadRequest, Code: "invalid_json", Message: "Некорректное тело запроса"}
	}
	if decoder.More() {
		return &Error{Status: http.StatusBadRequest, Code: "invalid_json", Message: "Ожидается один JSON-объект"}
	}
	return nil
}
