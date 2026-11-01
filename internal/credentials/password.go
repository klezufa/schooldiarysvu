package credentials

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strings"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024
	argonThreads = 2
	argonKeyLen  = 32
	argonSaltLen = 16
)

type Params struct {
	MemoryKiB   uint32
	Iterations  uint32
	Parallelism uint8
	KeyLength   uint32
}

func DefaultParams() Params {
	return Params{MemoryKiB: argonMemory, Iterations: argonTime, Parallelism: argonThreads, KeyLength: argonKeyLen}
}

func HashPassword(password string) (string, error) {
	return HashPasswordWithParams(password, DefaultParams())
}

func HashPasswordWithParams(password string, params Params) (string, error) {
	if params.MemoryKiB < 1024 || params.MemoryKiB > 1024*1024 || params.Iterations < 1 ||
		params.Iterations > 10 || params.Parallelism < 1 || params.Parallelism > 16 ||
		params.KeyLength < 16 || params.KeyLength > 64 {
		return "", fmt.Errorf("unsafe Argon2 parameters")
	}
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey([]byte(password), salt, params.Iterations, params.MemoryKiB, params.Parallelism, params.KeyLength)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		params.MemoryKiB, params.Iterations, params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key)), nil
}

func VerifyPassword(encoded, password string) (bool, error) {
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false, fmt.Errorf("unsupported password hash")
	}
	var memory, time, threads uint32
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &time, &threads); err != nil {
		return false, fmt.Errorf("parse password hash parameters: %w", err)
	}
	if memory < 1024 || memory > 1024*1024 || time < 1 || time > 10 || threads < 1 || threads > 16 {
		return false, fmt.Errorf("unsafe password hash parameters")
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, fmt.Errorf("decode password salt: %w", err)
	}
	expected, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, fmt.Errorf("decode password hash: %w", err)
	}
	if len(salt) < 8 || len(expected) < 16 || len(expected) > 64 {
		return false, fmt.Errorf("invalid password hash length")
	}
	actual := argon2.IDKey([]byte(password), salt, time, memory, uint8(threads), uint32(len(expected)))
	return subtle.ConstantTimeCompare(actual, expected) == 1, nil
}
