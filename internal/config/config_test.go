package config_test

import (
	"testing"

	"schooldiarysvu/internal/config"
)

func TestLoad_SchoolTimezoneWorksWithoutSystemTzdata(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://diary@127.0.0.1:5432/diary")
	t.Setenv("ZONEINFO", "") // имитируем окружение без системной базы часовых поясов
	t.Setenv("SCHOOL_TIMEZONE", "Europe/Moscow")
	if _, err := config.Load(); err != nil {
		t.Fatalf("Load() with Europe/Moscow: %v", err)
	}
}

func TestLoad_Validation(t *testing.T) {
	tests := []struct {
		name    string
		env     map[string]string
		wantErr bool
	}{
		{name: "defaults are valid", env: map[string]string{}},
		{name: "https profile requires secure cookies", env: map[string]string{"NETWORK_PROFILE": "https", "SESSION_COOKIE_SECURE": "false"}, wantErr: true},
		{name: "https profile with secure cookies", env: map[string]string{"NETWORK_PROFILE": "https", "SESSION_COOKIE_SECURE": "true"}},
		{name: "unknown profile", env: map[string]string{"NETWORK_PROFILE": "wan"}, wantErr: true},
		{name: "unknown timezone", env: map[string]string{"SCHOOL_TIMEZONE": "Mars/Base"}, wantErr: true},
		{name: "upload limit too large", env: map[string]string{"MAX_UPLOAD_BYTES": "999999999"}, wantErr: true},
		{name: "weak argon2 memory", env: map[string]string{"ARGON2_MEMORY_KIB": "1024"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", "postgres://diary@127.0.0.1:5432/diary")
			for key, value := range tt.env {
				t.Setenv(key, value)
			}
			_, err := config.Load()
			if (err != nil) != tt.wantErr {
				t.Fatalf("Load() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
