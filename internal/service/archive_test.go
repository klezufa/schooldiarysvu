package service

import "testing"

func TestNormalizeArchiveCode(t *testing.T) {
	tests := []struct {
		name, input, want string
	}{
		{"as printed", "A1B2-C3D4-E5F6-0789", "A1B2-C3D4-E5F6-0789"},
		{"lower case without dashes", "a1b2c3d4e5f60789", "A1B2-C3D4-E5F6-0789"},
		{"spaces", " a1b2 c3d4 e5f6 0789 ", "A1B2-C3D4-E5F6-0789"},
		{"too short", "A1B2-C3D4-E5F6", ""},
		{"too long", "A1B2-C3D4-E5F6-0789-0", ""},
		{"not hexadecimal", "G1B2-C3D4-E5F6-0789", ""},
		{"cyrillic look-alike", "А1В2-C3D4-E5F6-0789", ""},
		{"empty", "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeArchiveCode(tt.input); got != tt.want {
				t.Errorf("normalizeArchiveCode(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
