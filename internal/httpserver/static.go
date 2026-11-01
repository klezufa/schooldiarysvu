package httpserver

import (
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"
)

// contentSecurityPolicy совпадает с политикой Nginx (deploy/nginx) и подходит для собранного React-приложения.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'self'; form-action 'self'"

// SPAHandler отдаёт собранный frontend из dir: статические файлы как есть, а любой другой путь без расширения —
// index.html (маршруты React Router). Это режим для запуска без Nginx (например, на ноутбуке для тестирования);
// production-схема с Nginx остаётся основной.
//
// Запросы к /api/ и /health/ сюда не попадают как к «фронтенду»: для них возвращается 404, чтобы опечатка в пути API
// не превращалась в HTML-страницу с кодом 200. Каталоги не листаются, выход за корень dir невозможен (os.DirFS).
func SPAHandler(dir string) (http.Handler, error) {
	root := os.DirFS(dir)
	if info, err := fs.Stat(root, "index.html"); err != nil || info.IsDir() {
		return nil, fmt.Errorf("static dir %q must contain index.html: %w", dir, err)
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		clean := path.Clean("/" + r.URL.Path)
		if clean == "/api" || strings.HasPrefix(clean, "/api/") || strings.HasPrefix(clean, "/health/") {
			http.NotFound(w, r)
			return
		}
		setStaticHeaders(w)
		name := strings.TrimPrefix(clean, "/")
		if name != "" {
			if info, err := fs.Stat(root, name); err == nil && !info.IsDir() {
				if strings.HasPrefix(clean, "/assets/") {
					// Имена файлов в /assets содержат хеш содержимого, поэтому их можно кешировать навсегда.
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				} else {
					w.Header().Set("Cache-Control", "no-cache")
				}
				http.ServeFileFS(w, r, root, name)
				return
			}
			if path.Ext(name) != "" {
				// Отсутствующий файл (например, старый хешированный бандл) не должен подменяться index.html.
				http.NotFound(w, r)
				return
			}
		}
		w.Header().Set("Cache-Control", "no-cache")
		http.ServeFileFS(w, r, root, "index.html")
	}), nil
}

func setStaticHeaders(w http.ResponseWriter) {
	header := w.Header()
	header.Set("X-Content-Type-Options", "nosniff")
	header.Set("X-Frame-Options", "DENY")
	header.Set("Referrer-Policy", "no-referrer")
	header.Set("Content-Security-Policy", contentSecurityPolicy)
}
