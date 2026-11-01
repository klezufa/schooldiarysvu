package service

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestAllowedFileType(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    string
		valid   bool
	}{
		{name: "plain text", content: "Домашнее задание", want: "text/plain", valid: true},
		{name: "HTML", content: "<!doctype html><script>alert(1)</script>", valid: false},
		{name: "PHP", content: "<?php echo 1;", valid: false},
		{name: "empty", content: "", valid: false},
		{name: "PNG", content: "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 32), want: "image/png", valid: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := allowedFileType([]byte(test.content))
			if (err == nil) != test.valid || got != test.want {
				t.Fatalf("allowedFileType() = %q, %v; want %q, valid=%v", got, err, test.want, test.valid)
			}
		})
	}
}

func TestSpreadsheetSafe(t *testing.T) {
	for _, value := range []string{"=2+2", "+SUM(A1:A2)", "-1", "@cmd", " \t=2+2"} {
		if got := spreadsheetSafe(value); !strings.HasPrefix(got, "'") {
			t.Errorf("spreadsheetSafe(%q) = %q, expected apostrophe", value, got)
		}
	}
	for _, value := range []string{"Иванов", "1+1", "безопасно"} {
		if got := spreadsheetSafe(value); got != value {
			t.Errorf("spreadsheetSafe(%q) = %q", value, got)
		}
	}
}

// Имя файла получает расширение по реальному типу содержимого: «evil.svg» с текстом внутри сохранялся под этим именем,
// и после скачивания двойной щелчок открывал его в браузере как SVG со скриптом.
func TestNameForType(t *testing.T) {
	tests := []struct {
		name, mimeType, want string
	}{
		{name: "evil.svg", mimeType: "text/plain", want: "evil.svg.txt"},
		{name: "page.html", mimeType: "text/plain", want: "page.html.txt"},
		{name: "notes.TXT", mimeType: "text/plain", want: "notes.TXT"},
		{name: "scan.pdf", mimeType: "application/pdf", want: "scan.pdf"},
		{name: "scan.exe", mimeType: "application/pdf", want: "scan.exe.pdf"},
		{name: "photo.jpeg", mimeType: "image/jpeg", want: "photo.jpeg"},
		{name: "photo", mimeType: "image/png", want: "photo.png"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := nameForType(test.name, test.mimeType); got != test.want {
				t.Fatalf("nameForType(%q, %q) = %q, want %q", test.name, test.mimeType, got, test.want)
			}
		})
	}
}

func TestSafeOriginalName_KeepsValidUTF8(t *testing.T) {
	name := safeOriginalName(strings.Repeat("ж", 300) + ".txt")
	if !utf8.ValidString(name) || len(name) > 240 || !strings.HasSuffix(name, ".txt") {
		t.Fatalf("len %d, valid %v", len(name), utf8.ValidString(name))
	}
	if got := safeOriginalName("../../etc/passwd\r\nX: 1"); strings.ContainsAny(got, "/\r\n") {
		t.Fatalf("path or control characters kept: %q", got)
	}
}
