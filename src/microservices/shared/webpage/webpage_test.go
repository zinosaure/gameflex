package webpage

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCacheControl(t *testing.T) {
	cases := map[string]string{
		"index.html":                         "no-cache",
		"bundle.js":                          "no-cache",
		"service-worker.js":                  "no-cache",
		"122.45943f554fd7242a675e.bundle.js": "public, max-age=31536000, immutable",
		"/fonts/bootstrap-icons.woff2":       "no-cache",
	}
	for path, want := range cases {
		if got := CacheControl(path); got != want {
			t.Errorf("CacheControl(%q) = %q, attendu %q", path, got, want)
		}
	}
}

func TestHandler(t *testing.T) {
	root := t.TempDir()
	for name, body := range map[string]string{
		"index.html":                         "<html>page</html>",
		"bundle.js":                          "bundle",
		"service-worker.js":                  "worker",
		"122.45943f554fd7242a675e.bundle.js": "chunk",
		"images/icon-192.png":                "icon",
		"fonts/bootstrap-icons.woff2":        "font",
	} {
		if err := os.MkdirAll(filepath.Dir(filepath.Join(root, name)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	handler := Handler(root)

	cases := []struct {
		path        string
		status      int
		body        string
		cache       string
		contentType string
	}{
		{path: "/bundle.js", status: 200, body: "bundle", cache: "no-cache"},
		{path: "/service-worker.js", status: 200, body: "worker", cache: "no-cache"},
		{path: "/122.45943f554fd7242a675e.bundle.js", status: 200, body: "chunk", cache: "public, max-age=31536000, immutable"},
		{path: "/retrogaming/nes", status: 200, body: "<html>page</html>", cache: "no-cache"},
		{path: "/", status: 200, body: "<html>page</html>", cache: "no-cache"},
		// Le serveur de fichiers renvoie /index.html vers / : la consigne part
		// quand même avec la redirection.
		{path: "/index.html", status: 301, cache: "no-cache"},
		{path: "/api/v1/users/12", status: 404, body: "no such route: /api/v1/users/12", contentType: "application/json"},
		{path: "/internal/sessions/x/mirror", status: 404, body: "no such route", contentType: "application/json"},
		// Le dossier des téléchargements, absent de toute copie de la page, reste
		// un dossier de fichiers : jamais de HTML sous un nom en `.apk`.
		{path: "/downloads/gameflex.apk", status: 404},
		{path: "/downloads/shadps4-aarch64.zip", status: 404},
		{path: "/images/icon-192.png", status: 200, body: "icon"},
		{path: "/images/absente.png", status: 404},
		{path: "/fonts/absente.woff2", status: 404},
		// À la racine, un fichier : un morceau d'une version d'avant, le manifeste.
		{path: "/9999.0123456789abcdef0123.bundle.js", status: 404},
		{path: "/manifest.json", status: 404},
		// LES ADRESSES DE L'APPLICATION QUI PORTENT UN POINT (#701) : le jeu d'une
		// console, le chemin d'un journal — la page, comme au serveur de dev.
		{path: "/retrogaming/nes/games/Zelda%20(USA).zip", status: 200, body: "<html>page</html>", cache: "no-cache"},
		{path: "/retrogaming/nes/games/Zelda.zip", status: 200, body: "<html>page</html>"},
		{path: "/streamex/ps5/games/CUSA12345.pkg", status: 200, body: "<html>page</html>"},
		{path: "/settings/logs/kernel/kernel.log", status: 200, body: "<html>page</html>"},
		{path: "/settings/consoles/nes", status: 200, body: "<html>page</html>"},
		// Un chemin qui remonte hors de la page est refusé avant toute lecture.
		{path: "/../../etc/passwd", status: 400},
	}
	for _, c := range cases {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "http://page"+c.path, nil))

		if recorder.Code != c.status {
			t.Errorf("%s : statut %d, attendu %d", c.path, recorder.Code, c.status)
		}
		if c.body != "" && !strings.Contains(recorder.Body.String(), c.body) {
			t.Errorf("%s : corps %q, attendu %q", c.path, recorder.Body.String(), c.body)
		}
		if c.cache != "" && recorder.Header().Get("Cache-Control") != c.cache {
			t.Errorf("%s : Cache-Control %q, attendu %q", c.path, recorder.Header().Get("Cache-Control"), c.cache)
		}
		if c.contentType != "" && recorder.Header().Get("Content-Type") != c.contentType {
			t.Errorf("%s : Content-Type %q, attendu %q", c.path, recorder.Header().Get("Content-Type"), c.contentType)
		}
	}
}
