package httpserver

import (
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"schooldiarysvu/internal/apperr"
)

func pathUUID(r *http.Request) (uuid.UUID, error) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		return uuid.Nil, apperr.BadRequest("Некорректный идентификатор")
	}
	return id, nil
}

func queryUUID(r *http.Request, name string, required bool) (uuid.UUID, error) {
	value := r.URL.Query().Get(name)
	if value == "" && !required {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(value)
	if err != nil {
		return uuid.Nil, apperr.BadRequest("Некорректный идентификатор " + name)
	}
	return id, nil
}

func queryDate(r *http.Request, name string, required bool) (openapi_types.Date, error) {
	value := r.URL.Query().Get(name)
	if value == "" && !required {
		return openapi_types.Date{}, nil
	}
	parsed, err := time.Parse("2006-01-02", value)
	if err != nil {
		return openapi_types.Date{}, apperr.BadRequest("Некорректная дата " + name)
	}
	return openapi_types.Date{Time: parsed}, nil
}

// maxOffset — потолок смещения: больше записей в школьных списках не бывает, а без потолка значение больше 2³¹
// переполняло int32, становилось отрицательным, и PostgreSQL отвечал ошибкой (500).
const maxOffset = 1_000_000

func pagination(r *http.Request) (int32, int32) {
	limit := int32(50)
	offset := int32(0)
	if parsed, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && parsed >= 1 && parsed <= 100 {
		limit = int32(parsed)
	}
	if parsed, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && parsed >= 0 {
		offset = int32(min(parsed, maxOffset))
	}
	return limit, offset
}

func queryVersion(r *http.Request) (int32, error) {
	version, err := strconv.Atoi(r.URL.Query().Get("version"))
	if err != nil || version < 1 || version > math.MaxInt32 {
		return 0, apperr.BadRequest("Требуется положительная версия объекта")
	}
	return int32(version), nil
}
