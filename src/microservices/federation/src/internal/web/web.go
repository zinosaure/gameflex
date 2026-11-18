// Package web — L'INTERFACE DU SERVICE, EMBARQUÉE DANS SON BINAIRE (chantier 61) : celle que construit la chaîne d'edge
// (`npm run build:federation`, app/federation/), copiée dans dist/ par le Dockerfile avant la compilation. Le dépôt ne
// garde à sa place qu'une page qui le dit : une construction sans Node sert l'API, et le dit.
package web

import (
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var files embed.FS

// Handler — les fichiers de l'interface ; toute autre adresse (une route de l'application : /connections/3,
// /invite/<code>) rend index.html, que le routeur de la page lit.
func Handler() http.Handler {
	root, _ := fs.Sub(files, "dist")
	static := http.FileServer(http.FS(root))

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if name == "" || name == "index.html" {
			serveIndex(w, root)

			return
		}
		if info, err := fs.Stat(root, name); err == nil && !info.IsDir() {
			// LES SCRIPTS PORTENT LEUR EMPREINTE dans leur nom (bundle.<contenthash>.js) : une nouvelle version change de
			// nom, et l'ancienne peut rester en cache. Le reste se revalide : un nom fixe gardé une journée a caché une
			// nouvelle interface au porteur (2026-11-18).
			if strings.HasSuffix(name, ".js") || strings.HasPrefix(name, "fonts/") {
				w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
			} else {
				w.Header().Set("Cache-Control", "no-cache")
			}
			static.ServeHTTP(w, r)

			return
		}
		serveIndex(w, root)
	})
}

func serveIndex(w http.ResponseWriter, root fs.FS) {
	page, err := fs.ReadFile(root, "index.html")
	if err != nil {
		http.Error(w, "interface missing", http.StatusNotFound)

		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}
