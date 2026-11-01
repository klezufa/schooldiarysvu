package service

import (
	"math"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

func apiDate(value time.Time) openapi_types.Date {
	return openapi_types.Date{Time: value}
}

func apiNullableDate(value pgtype.Date) *openapi_types.Date {
	if !value.Valid {
		return nil
	}
	result := apiDate(value.Time)
	return &result
}

func dateOrMax(value pgtype.Date) time.Time {
	if value.Valid {
		return value.Time
	}
	return time.Date(9999, 12, 31, 0, 0, 0, 0, time.UTC)
}

func nullableText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func nullableTime(value *time.Time) pgtype.Timestamptz {
	if value == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *value, Valid: true}
}

func optionalUUID(value *uuid.UUID) pgtype.UUID {
	if value == nil {
		return pgtype.UUID{}
	}
	return nullableUUID(*value)
}

func uuidPointer(value pgtype.UUID) *uuid.UUID {
	if !value.Valid {
		return nil
	}
	result := uuid.UUID(value.Bytes)
	return &result
}

func timePointer(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

func number(value float32) pgtype.Numeric {
	var result pgtype.Numeric
	_ = result.Scan(strconv.FormatFloat(float64(value), 'f', -1, 32))
	return result
}

func numericValue(value pgtype.Numeric) float32 {
	converted, _ := value.Float64Value()
	if !converted.Valid || math.IsNaN(converted.Float64) || math.IsInf(converted.Float64, 0) {
		return 0
	}
	return float32(converted.Float64)
}
