package service

import (
	"context"
	"time"

	"schooldiarysvu/internal/apperr"
	"schooldiarysvu/internal/credentials"
)

// Хеширование паролей (Argon2id) — самая дорогая операция сервера: каждое выделяет MemoryKiB памяти (по умолчанию
// 64 МиБ). Утром класс входит почти одновременно, и без ограничения параллельных хешей память растёт до N × 64 МиБ:
// нагрузочный тест (20 входов в секунду) довёл контейнер с лимитом 512 МиБ до OOM — пять перезапусков за две минуты
// и 58 % неудачных входов, включая запросы уже вошедших пользователей. Слоты ограничивают пик памяти, остальные входы
// ждут в очереди; если очередь не продвинулась за passwordQueueWait, клиент получает 503 с Retry-After, а не падение.

const (
	defaultPasswordHashConcurrency = 2
	passwordQueueWait              = 10 * time.Second
	passwordRetryAfter             = 5 * time.Second
)

func newPasswordSlots(concurrency int) chan struct{} {
	if concurrency < 1 {
		concurrency = 1
	}
	return make(chan struct{}, concurrency)
}

// SetPasswordHashConcurrency задаёт число одновременных хешей паролей (по умолчанию 2: 2 × 64 МиБ с запасом
// укладываются в лимит памяти контейнера backend). Вызывается при запуске, до приёма запросов.
func (s *Service) SetPasswordHashConcurrency(concurrency int) {
	s.passwordSlots = newPasswordSlots(concurrency)
}

func (s *Service) withPasswordSlot(ctx context.Context, work func()) error {
	timer := time.NewTimer(passwordQueueWait)
	defer timer.Stop()
	select {
	case s.passwordSlots <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return apperr.ServiceBusy(passwordRetryAfter)
	}
	defer func() { <-s.passwordSlots }()
	work()
	return nil
}

// verifyPassword проверяет пароль в очереди хешей. Ошибка — только очередь или отмена запроса; повреждённый хеш
// считается неверным паролем, как и раньше.
func (s *Service) verifyPassword(ctx context.Context, encoded, password string) (bool, error) {
	var valid bool
	err := s.withPasswordSlot(ctx, func() {
		ok, verifyErr := credentials.VerifyPassword(encoded, password)
		valid = verifyErr == nil && ok
	})
	return valid, err
}

func (s *Service) hashPassword(ctx context.Context, password string) (string, error) {
	var hash string
	var hashErr error
	if err := s.withPasswordSlot(ctx, func() { hash, hashErr = credentials.HashPasswordWithParams(password, s.passwords) }); err != nil {
		return "", err
	}
	return hash, hashErr
}
