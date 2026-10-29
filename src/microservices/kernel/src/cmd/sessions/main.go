// Command sessions is the live-session service: it spawns one streamex
// container per session via docker.sock and exposes the /api/v1 contract
// consumed by the GameFlex frontend (see docs/GAMEFLEX.md).
package main

import (
	"context"
	"errors"
	"gameflex/consoles"
	"gameflex/kernel/internal/api"
	"gameflex/kernel/internal/catalog"
	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/docker/profiles"
	"gameflex/kernel/internal/manager"
	"gameflex/kernel/internal/registry"
	"gameflex/logfile"
	"github.com/docker/docker/client"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

func main() {
	// LE JOURNAL SUR LE DISQUE, EN PLUS DE LA SORTIE STANDARD.
	//
	// `docker logs` meurt avec le conteneur, et c'est justement après coup
	// qu'on veut relire ce qui s'est passé. Le fichier vit dans le stockage
	// chaud, rangé par jour et par lancement (voir gameflex/logfile).
	//
	// LE JOURNAL DE LA JOURNÉE, pas celui d'un démarrage. Le kernel tourne
	// en continu : découper son fichier à chaque redémarrage éparpillerait
	// une enquête, alors que ce qui PRÉCÈDE un redémarrage est justement ce
	// qu'on cherche quand on enquête dessus.
	//
	// EN PLUS de la sortie standard, jamais à la place : elle reste ce qu'on
	// lit pendant qu'on développe, et un journal qui déplace ce qu'on avait
	// l'habitude de trouver est un journal qu'on cesse de lire.
	output := io.Writer(os.Stdout)
	if file, err := logfile.Service(config.StorageDir, "kernel"); err == nil && file != nil {
		defer file.Close()
		output = io.MultiWriter(os.Stdout, file)
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(output, &slog.HandlerOptions{Level: slog.LevelInfo})))

	cfg, err := config.Load()
	if err != nil {
		slog.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	// LE RANGEMENT DU DOSSIER (chantier 24), créé s'il manque.
	if err := api.EnsureStorageFolders(config.StorageDir); err != nil {
		slog.Error("storage folder", "error", err)
		os.Exit(1)
	}

	// The streamex core no longer embeds console buttons — write our own pad
	// profiles (host-side) so each session container can mount its own.
	if err := profiles.WriteAll(config.ProfilesDir); err != nil {
		slog.Error("pad profiles", "error", err)
		os.Exit(1)
	}

	dockerClient, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation())
	if err != nil {
		slog.Error("docker client", "error", err)
		os.Exit(1)
	}

	dockerHandler, err := docker.New(dockerClient, cfg)
	if err != nil {
		slog.Error("docker handler", "error", err)
		os.Exit(1)
	}

	// Report any session image the daemon does not have. Only a warning: the
	// other platforms still work, and the missing one is fixed by a build,
	// not a restart. Without it the first symptom is a player clicking Play.
	dockerHandler.CheckImages(context.Background())
	dockerHandler.CheckGPU()

	// LE REGISTRE DÉCIDE, PAS CETTE LISTE.
	//
	// Elle était écrite à la main — switch/ps3/xbox/ps4 — et c'était la
	// CINQUIÈME liste où une console streamex devait se déclarer, après le
	// registre partagé, la config du kernel, sessionImage et buildTargets.
	// Une console absente d'ici seulement échoue sur « unsupported console »
	// alors que tout le reste est en place : vécu en branchant la Wii U
	// (2026-09-08), et le message ne désigne aucun des cinq endroits.
	//
	// Même disposition sur le disque que le rétro : l'identifiant de la
	// console EST le nom du dossier sous roms/ (/gameflex/roms). Le registre porte déjà cette information — il n'y a
	// donc rien à répéter, seulement à le lire.
	sessionCatalogs := map[string]*catalog.Catalog{
		// keytest n'est pas une console : pas de jeux sur le disque, un
		// catalogue synthétique. Elle reste donc déclarée à la main.
		"keytest": catalog.NewKeyTest(),
	}
	for _, console := range consoles.List() {
		if console.Method != consoles.MethodStreamex {
			continue
		}
		sessionCatalogs[console.ID] = catalog.New(
			console.ID, filepath.Join(config.StorageDir, "roms", console.ID),
		)
	}

	sessionManager := manager.New(dockerHandler, sessionCatalogs, cfg)

	sessionPool := registry.NewPool()

	// LE JOURNAL D'ACCÈS — le kernel était le SEUL service à ne pas l'avoir.
	//
	// `logfile.HTTPLogger` existe depuis le 2026-09-09 (`824767fc`), où il a été
	// écrit pour un correctif de sécurité : les journaux écrivaient les jetons
	// en clair. Les trois autres services l'ont reçu ce jour-là. Pas le kernel —
	// et c'était cohérent, puisqu'il ne journalisait aucune requête, donc n'avait
	// rien à fuiter. Il est resté sans journal d'accès pour autant.
	//
	// CE QUE ÇA A COÛTÉ : le 2026-09-19, une session est restée figée 16 h et
	// l'utilisateur a appuyé sur « arrêter » sans effet. Impossible de dire si sa
	// requête était arrivée, avait été refusée (404) ou rejetée (403) —
	// `handleRetroStop` rend ces deux codes SANS rien écrire, et `stopSession` ne
	// journalise qu'en cas de succès. Trois chemins muets, aucun départageable
	// après coup. On ne diagnostique pas ce qu'on n'écrit pas.
	//
	// L'enveloppe, et non un `r.Use` : le kernel sert un `ServeMux`, pas un
	// routeur chi comme ses voisins. C'est précisément pour ça que `HTTPLogger`
	// est écrit en `net/http` pur (`redact.go:76`). Les jetons portés en query
	// par les WebSocket sont masqués au passage (`Redact`), et `Hijack`/`Flush`
	// traversent l'enrobage — sans quoi les poignées de main WebSocket et les
	// flux tomberaient.
	server := &http.Server{
		Addr:    ":" + config.HandlerPort,
		Handler: logfile.HTTPLogger(api.NewServer(sessionManager, sessionCatalogs["switch"], cfg, sessionPool).Routes()),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go func() {
		<-ctx.Done()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("shutdown", "error", err)
		}
	}()

	slog.Info("sessions service listening", "port", config.HandlerPort)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		slog.Error("server", "error", err)
		os.Exit(1)
	}
}
