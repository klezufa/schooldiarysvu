package apperr

import (
	"fmt"
	"net/http"
	"time"
)

type Error struct {
	Status      int
	Code        string
	Message     string
	FieldErrors map[string]string
	// RetryAfter — через сколько можно повторить (заголовок Retry-After у 429); ноль — не сообщается.
	RetryAfter time.Duration
}

func (e *Error) Error() string { return e.Message }

func New(status int, code, message string) *Error {
	return &Error{Status: status, Code: code, Message: message}
}

func BadRequest(message string) *Error {
	return New(http.StatusBadRequest, "bad_request", message)
}

func Unauthorized() *Error {
	return New(http.StatusUnauthorized, "unauthorized", "Требуется вход в систему")
}

func Forbidden() *Error {
	return New(http.StatusForbidden, "forbidden", "Действие запрещено")
}

func NotFound() *Error {
	return New(http.StatusNotFound, "not_found", "Объект не найден или недоступен")
}

func Conflict(message string) *Error {
	return New(http.StatusConflict, "conflict", message)
}

func Unprocessable(message string) *Error {
	return New(http.StatusUnprocessableEntity, "validation_failed", message)
}

// TooManyAttempts — отказ ограничителя попыток с оставшимся временем блокировки: человек у школьного компьютера
// должен видеть, сколько ждать, а не продолжать попытки вслепую.
func TooManyAttempts(wait time.Duration) *Error {
	minutes := int((wait + time.Minute - 1) / time.Minute)
	if minutes < 1 {
		minutes = 1
	}
	return &Error{Status: http.StatusTooManyRequests, Code: "rate_limited",
		Message: fmt.Sprintf("Слишком много попыток. Повторите через %d мин.", minutes), RetryAfter: wait}
}

// ServiceBusy — сервер временно перегружен (например, очередь хеширования паролей при утреннем наплыве входов):
// повторить можно через wait, данные не изменены.
func ServiceBusy(wait time.Duration) *Error {
	return &Error{Status: http.StatusServiceUnavailable, Code: "busy",
		Message: "Сервер занят: много одновременных входов. Повторите через несколько секунд.", RetryAfter: wait}
}
