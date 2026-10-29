// edge — le frontal d'une installation publique.
//
// En développement, le serveur de webpack sert la page et relaie l'API. Une
// installation publique n'a pas de serveur de développement : ce binaire sert
// la page construite (`dist/`) et relaie /api/v1/<domaine> — WebSocket compris —
// vers les services, d'après la même table que webpack (`api-domains.json`).
//
// Ni nginx ni Caddy : la table des domaines et les consignes de cache de la
// page n'existent ainsi qu'une fois, et le kernel du téléphone sert la page
// avec le même code (gameflex/webpage). Décidé le 2026-09-15.
//
// Variables : EDGE_PORT (8080), EDGE_WEB_ROOT (/app/web), EDGE_API_DOMAINS
// (/app/api-domains.json), et l'adresse de chaque service sous le nom que la
// table lui donne. `edge healthcheck` interroge le frontal lui-même, pour le
// HEALTHCHECK de l'image.
package main

import (
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"gameflex/edge/internal/domains"
	"gameflex/edge/internal/edge"
	"gameflex/webpage"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	port := envOr("EDGE_PORT", "8080")
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(healthcheck(port))
	}

	webRoot := envOr("EDGE_WEB_ROOT", "/app/web")
	domainsFile := envOr("EDGE_API_DOMAINS", "/app/api-domains.json")

	if _, err := os.Stat(filepath.Join(webRoot, "index.html")); err != nil {
		fatal("the built page is missing", "web_root", webRoot, "error", err)
	}

	services, err := domains.Load(domainsFile)
	if err != nil {
		fatal("unreadable API domain table", "file", domainsFile, "error", err)
	}

	handler, err := edge.New(services, os.Getenv, webpage.Handler(webRoot))
	if err != nil {
		fatal("cannot route the API", "error", err)
	}

	server := &http.Server{
		Addr:              ":" + port,
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
	}

	slog.Info("edge listening", "port", port, "services", len(services), "version", os.Getenv("GAMEFLEX_VERSION"))
	if err := server.ListenAndServe(); err != nil {
		fatal("edge stopped", "error", err)
	}
}

// healthcheck — 0 quand la page répond, 1 sinon.
func healthcheck(port string) int {
	client := http.Client{Timeout: 3 * time.Second}

	response, err := client.Get("http://127.0.0.1:" + port + "/")
	if err != nil {
		return 1
	}
	defer response.Body.Close()

	if response.StatusCode != http.StatusOK {
		return 1
	}

	return 0
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}

	return fallback
}

func fatal(message string, args ...any) {
	slog.Error(message, args...)
	os.Exit(1)
}
