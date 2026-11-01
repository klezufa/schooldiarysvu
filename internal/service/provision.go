package service

import (
	"crypto/rand"
	"fmt"
	"math/big"
	"regexp"
	"strings"
	"unicode"
)

// Алфавит без похожих символов (0/O, 1/I/L): пароль и код приглашения диктуют голосом и переписывают с листа.
const readableAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

var loginPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{2,63}$`)

func randomString(length int, alphabet string) (string, error) {
	out := make([]byte, length)
	limit := big.NewInt(int64(len(alphabet)))
	for i := range out {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			return "", fmt.Errorf("generate random string: %w", err)
		}
		out[i] = alphabet[n.Int64()]
	}
	return string(out), nil
}

// generateTemporaryPassword выдаёт пароль вида Xk3m-9Pq2-Wt7R: 12 знаков (~60 бит), одноразовый, при входе заменяется.
func generateTemporaryPassword() (string, error) {
	raw, err := randomString(12, readableAlphabet)
	if err != nil {
		return "", err
	}
	return raw[0:4] + "-" + raw[4:8] + "-" + raw[8:12], nil
}

// generateInvitationCode выдаёт код из 20 знаков (100 бит) в виде XXXXX-XXXXX-XXXXX-XXXXX.
func generateInvitationCode() (string, error) {
	raw, err := randomString(20, readableAlphabet)
	if err != nil {
		return "", err
	}
	return raw[0:5] + "-" + raw[5:10] + "-" + raw[10:15] + "-" + raw[15:20], nil
}

// normalizeInvitationCode приводит введённый код к каноническому виду: регистр, дефисы и пробелы не важны.
func normalizeInvitationCode(code string) string {
	var out strings.Builder
	for _, r := range strings.ToUpper(code) {
		if r == '-' || unicode.IsSpace(r) {
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

var translitTable = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i", 'й': "y",
	'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t", 'у': "u", 'ф': "f",
	'х': "kh", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "shch", 'ъ': "", 'ы': "y", 'ь': "", 'э': "e", 'ю': "yu", 'я': "ya",
}

func transliterate(value string) string {
	var out strings.Builder
	for _, r := range strings.ToLower(value) {
		if latin, ok := translitTable[r]; ok {
			out.WriteString(latin)
		} else if r < unicode.MaxASCII && (unicode.IsLetter(r) || unicode.IsDigit(r)) {
			out.WriteRune(r)
		}
	}
	return out.String()
}

// loginFromName строит логин «фамилия.ио» из ФИО: «Иванов Иван Петрович» → ivanov.ip.
func loginFromName(fullName string) string {
	parts := strings.Fields(fullName)
	if len(parts) == 0 {
		return ""
	}
	base := transliterate(parts[0])
	initials := ""
	for _, part := range parts[1:] {
		if latin := transliterate(part); latin != "" {
			initials += latin[:1]
		}
	}
	if initials != "" {
		base += "." + initials
	}
	if len(base) < 3 {
		base += "user"
	}
	if len(base) > 56 {
		base = base[:56]
	}
	return base
}

// roleDisplayNames используется в сообщениях без обращения к БД.
var roleDisplayNames = map[string]string{
	"system_admin": "Системный администратор", "administrator": "Администратор", "teacher": "Учитель",
	"class_teacher": "Классный руководитель", "student": "Ученик", "parent": "Родитель", "principal": "Руководитель школы",
}

var assignableRoles = map[string]bool{
	"system_admin": true, "administrator": true, "teacher": true, "class_teacher": true, "student": true, "parent": true, "principal": true,
}
