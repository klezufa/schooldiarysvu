package service

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"schooldiarysvu/internal/api"
	"schooldiarysvu/internal/apperr"
	db "schooldiarysvu/internal/database/sqlc"
)

type DownloadedFile struct {
	Metadata api.File
	Content  []byte
}

func (s *Service) MaxUploadBytes() int64 {
	return s.maxUpload
}

func (s *Service) UploadFile(ctx context.Context, actor Principal, homeworkID uuid.UUID, originalName string, content []byte) (api.File, error) {
	if err := actor.requireTeacher(); err != nil {
		return api.File{}, err
	}
	if len(content) == 0 {
		return api.File{}, apperr.Unprocessable("Пустой файл не поддерживается")
	}
	if int64(len(content)) > s.maxUpload {
		return api.File{}, apperr.New(http.StatusRequestEntityTooLarge, "file_too_large", "Файл превышает разрешённый размер")
	}
	mimeType, err := allowedFileType(content)
	if err != nil {
		return api.File{}, err
	}
	originalName = nameForType(safeOriginalName(originalName), mimeType)
	storageKey := uuid.NewString() + ".bin"
	if err := os.MkdirAll(s.fileRoot, 0o700); err != nil {
		return api.File{}, err
	}
	path := filepath.Join(s.fileRoot, storageKey)
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return api.File{}, err
	}
	keep := false
	defer func() {
		_ = file.Close()
		if !keep {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(content); err != nil {
		return api.File{}, err
	}
	if err := file.Sync(); err != nil {
		return api.File{}, err
	}
	if err := file.Close(); err != nil {
		return api.File{}, err
	}
	var result api.File
	err = s.transaction(ctx, func(tx pgx.Tx, q *db.Queries) error {
		homework, err := q.GetHomework(ctx, homeworkID)
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := requireHomeworkOwner(ctx, q, actor, homework); err != nil {
			return err
		}
		created, err := q.CreateFile(ctx, db.CreateFileParams{StorageKey: storageKey, OriginalName: originalName,
			SizeBytes: int64(len(content)), MimeType: mimeType, OwnerID: actor.UserID})
		if err != nil {
			return mapDatabaseError(err)
		}
		if err := q.AttachHomeworkFile(ctx, db.AttachHomeworkFileParams{HomeworkID: homeworkID, FileID: created.ID}); err != nil {
			return mapDatabaseError(err)
		}
		result = api.File{Id: created.ID, OriginalName: created.OriginalName, SizeBytes: created.SizeBytes, MimeType: created.MimeType, CreatedAt: &created.CreatedAt}
		newValues, _ := json.Marshal(map[string]any{"homework_id": homeworkID, "original_name": originalName, "size_bytes": len(content), "mime_type": mimeType})
		return audit(ctx, q, actor, "file.uploaded", "file", created.ID, nil, newValues)
	})
	if err != nil {
		return api.File{}, err
	}
	keep = true
	return result, nil
}

func (s *Service) DownloadFile(ctx context.Context, actor Principal, fileID uuid.UUID, studentID *uuid.UUID) (DownloadedFile, error) {
	metadata, err := s.queries.GetFile(ctx, fileID)
	if err != nil {
		return DownloadedFile{}, mapDatabaseError(err)
	}
	allowed := false
	if actor.TeacherID != nil && actor.HasRole("teacher") {
		allowed, err = s.queries.FileVisibleToTeacher(ctx, db.FileVisibleToTeacherParams{ID: fileID, UserID: actor.UserID})
		if err != nil {
			return DownloadedFile{}, err
		}
	}
	if !allowed {
		id, err := readableStudentID(actor, studentID)
		if err != nil {
			return DownloadedFile{}, apperr.NotFound()
		}
		if canReadStudent(ctx, s.queries, actor, id, time.Now()) != nil {
			return DownloadedFile{}, apperr.NotFound()
		}
		allowed, err = s.queries.FileVisibleToStudent(ctx, db.FileVisibleToStudentParams{ID: fileID, StudentID: id})
		if err != nil {
			return DownloadedFile{}, err
		}
	}
	if !allowed {
		return DownloadedFile{}, apperr.NotFound()
	}
	if !isSafeStorageKey(metadata.StorageKey) {
		return DownloadedFile{}, apperr.NotFound()
	}
	content, err := os.ReadFile(filepath.Join(s.fileRoot, metadata.StorageKey))
	if err != nil {
		if os.IsNotExist(err) {
			return DownloadedFile{}, apperr.NotFound()
		}
		return DownloadedFile{}, err
	}
	return DownloadedFile{Metadata: api.File{Id: metadata.ID, OriginalName: metadata.OriginalName, SizeBytes: metadata.SizeBytes,
		MimeType: metadata.MimeType, CreatedAt: &metadata.CreatedAt}, Content: content}, nil
}

func allowedFileType(content []byte) (string, error) {
	if len(content) == 0 {
		return "", apperr.Unprocessable("Пустой файл не поддерживается")
	}
	mimeType := http.DetectContentType(content)
	switch mimeType {
	case "image/jpeg", "image/png", "image/gif", "application/pdf":
		return mimeType, nil
	}
	if mimeType == "text/plain; charset=utf-8" && utf8.Valid(content) && !bytes.ContainsRune(content, '\x00') {
		lower := strings.ToLower(strings.TrimSpace(string(content)))
		if strings.HasPrefix(lower, "<!doctype html") || strings.HasPrefix(lower, "<html") || strings.HasPrefix(lower, "<script") ||
			strings.HasPrefix(lower, "<?php") || strings.HasPrefix(lower, "<%") {
			return "", apperr.Unprocessable("Активное содержимое файла запрещено")
		}
		return "text/plain", nil
	}
	return "", apperr.Unprocessable("Разрешены только JPEG, PNG, GIF, PDF и текстовые файлы")
}

func safeOriginalName(value string) string {
	value = filepath.Base(strings.ReplaceAll(value, "\\", "/"))
	value = strings.Map(func(r rune) rune {
		if r < 32 || r == 127 {
			return -1
		}
		return r
	}, value)
	value = strings.TrimSpace(value)
	if value == "" || value == "." {
		return "file"
	}
	// Обрезка по символам, а не байтам: разрезанная кириллическая буква — невалидный UTF-8, его не примет PostgreSQL.
	// Запас под расширение, которое может добавить nameForType.
	for len(value) > 240 {
		_, size := utf8.DecodeRuneInString(value)
		value = value[size:]
	}
	return value
}

func isSafeStorageKey(value string) bool {
	id, err := uuid.Parse(strings.TrimSuffix(value, ".bin"))
	return err == nil && id.String()+".bin" == value
}

// fileExtensions — допустимые расширения для каждого разрешённого типа; первое добавляется, если имя не подходит.
var fileExtensions = map[string][]string{
	"image/jpeg":      {".jpg", ".jpeg"},
	"image/png":       {".png"},
	"image/gif":       {".gif"},
	"application/pdf": {".pdf"},
	"text/plain":      {".txt"},
}

// nameForType приводит расширение имени к реальному типу содержимого. Скачанный файл открывается по расширению:
// текст под именем «evil.svg» или «page.html» браузер исполнил бы как SVG или HTML со скриптом, хотя сервер отдаёт его
// как text/plain с attachment. Поэтому к неподходящему имени добавляется правильное расширение.
func nameForType(name, mimeType string) string {
	allowed := fileExtensions[mimeType]
	lower := strings.ToLower(name)
	for _, extension := range allowed {
		if strings.HasSuffix(lower, extension) {
			return name
		}
	}
	if len(allowed) == 0 {
		return name
	}
	return name + allowed[0]
}
