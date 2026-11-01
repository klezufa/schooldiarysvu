package service

import (
	"regexp"
	"strings"
	"testing"
)

func TestLoginFromName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name, input, want string
	}{
		{name: "full russian name", input: "Иванов Иван Петрович", want: "ivanov.ip"},
		{name: "surname and name", input: "Щукина Юлия", want: "shchukina.y"},
		{name: "soft and hard signs are dropped", input: "Подъячев Ильич", want: "podyachev.i"},
		{name: "latin name", input: "Smith John", want: "smith.j"},
		{name: "extra spaces", input: "  Ёлкин   Пётр ", want: "elkin.p"},
		{name: "too short surname gets padded", input: "Ю", want: "yuuser"},
		{name: "empty", input: "   ", want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := loginFromName(tt.input); got != tt.want {
				t.Fatalf("loginFromName(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestGeneratedSecretsFormat(t *testing.T) {
	t.Parallel()
	password := regexp.MustCompile(`^[A-Z2-9]{4}-[A-Z2-9]{4}-[A-Z2-9]{4}$`)
	code := regexp.MustCompile(`^[A-Z2-9]{5}(-[A-Z2-9]{5}){3}$`)
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		p, err := generateTemporaryPassword()
		if err != nil || !password.MatchString(p) || strings.ContainsAny(p, "01OIL") {
			t.Fatalf("temporary password %q: %v", p, err)
		}
		c, err := generateInvitationCode()
		if err != nil || !code.MatchString(c) {
			t.Fatalf("invitation code %q: %v", c, err)
		}
		if seen[c] {
			t.Fatalf("duplicate invitation code %q", c)
		}
		seen[c] = true
	}
}

func TestNormalizeInvitationCode(t *testing.T) {
	t.Parallel()
	tests := []struct{ name, input, want string }{
		{name: "canonical", input: "ABCDE-FGHJK", want: "ABCDEFGHJK"},
		{name: "lower case with spaces", input: " abcde fghjk ", want: "ABCDEFGHJK"},
		{name: "mixed separators", input: "abcde-\tfghjk", want: "ABCDEFGHJK"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizeInvitationCode(tt.input); got != tt.want {
				t.Fatalf("normalizeInvitationCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestEscapeLike(t *testing.T) {
	t.Parallel()
	if got := escapeLike(`50%_a\b`); got != `50\%\_a\\b` {
		t.Fatalf("escapeLike = %q", got)
	}
}

func TestGuardAssignableRoles(t *testing.T) {
	t.Parallel()
	admin := Principal{Roles: []string{"administrator"}}
	system := Principal{Roles: []string{"system_admin"}}
	tests := []struct {
		name    string
		actor   Principal
		roles   []string
		wantErr bool
	}{
		{name: "admin grants teacher", actor: admin, roles: []string{"teacher", "class_teacher"}},
		{name: "admin cannot grant administrator", actor: admin, roles: []string{"administrator"}, wantErr: true},
		{name: "admin cannot grant system admin", actor: admin, roles: []string{"student", "system_admin"}, wantErr: true},
		{name: "system admin grants administrator", actor: system, roles: []string{"administrator"}},
		{name: "empty set", actor: system, roles: nil, wantErr: true},
		{name: "duplicates", actor: system, roles: []string{"teacher", "teacher"}, wantErr: true},
		{name: "unknown role", actor: system, roles: []string{"root"}, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if err := guardAssignableRoles(tt.actor, tt.roles); (err != nil) != tt.wantErr {
				t.Fatalf("guardAssignableRoles(%v) error = %v, wantErr %v", tt.roles, err, tt.wantErr)
			}
		})
	}
}
