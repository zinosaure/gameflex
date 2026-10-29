// Package manager is the live-session engine — the raison d'être of the
// sessions service. One game session per player, TTL watchdog, Docker labels
// as the source of truth, and WS push with snapshot semantics (subscribers
// re-fetch a fresh snapshot instead of receiving queued payloads).
package manager

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"time"

	"gameflex/consoles"
	"gameflex/kernel/internal/catalog"
	"gameflex/kernel/internal/config"
	"gameflex/kernel/internal/docker"
	"gameflex/sessionstatus"
	"gameflex/storage"
)

var (
	ErrInvalidUsername     = errors.New("invalid username")
	ErrUnsupportedConsole  = errors.New("unsupported console")
	ErrGameNotFound        = errors.New("game not found")
	ErrConsoleLimitReached = errors.New("console session limit reached")
)

// unplayableError — le jeu est là, mais rien dedans ne se lance (#431). Un
// ErrGameNotFound pour le code (errors.Is), une phrase claire pour la page,
// qui l'affiche telle quelle.
type unplayableError string

func (e unplayableError) Error() string { return string(e) }

func (e unplayableError) Unwrap() error { return ErrGameNotFound }

const watchdogInterval = 20 * time.Second

var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{1,32}$`)

// Session is one spawned streamex session container.
type Session struct {
	BaseSession
	Game    string `json:"game"`
	GameID  string `json:"game_id"`
	Console string `json:"console"`
}

func (s *Session) base() *BaseSession { return &s.BaseSession }

// RetroSession is one spawned retrogaming session container — the
// one-container-per-session model, mirroring the external consoles. The
// container runs the retrogaming binary in standalone child mode (see
// docker.RetroSpawnSpec); the frontend reaches it through this service's
// own WS relay, never through play/watch URLs.
type RetroSession struct {
	BaseSession
	ConsoleID string `json:"console_id"`
	Reference string `json:"reference"`
	Title     string `json:"title"`
	Core      string `json:"core"`
	Bios      string `json:"bios"`
	// MaxPlayers is this console's real controller-port count (shared
	// consoles registry), fixed at launch.
	MaxPlayers int `json:"-"`
}

func (s *RetroSession) base() *BaseSession { return &s.BaseSession }

type Manager struct {
	config        config.Config
	dockerHandler *docker.Client
	catalogs      map[string]*catalog.Catalog

	// diskCatalogs — les catalogues streamex des disques internes (chantier
	// 23), un par disque et par console, créés à la première demande :
	// « <disque>/<console> » → catalogue.
	diskCatalogsMu sync.Mutex
	diskCatalogs   map[string]*catalog.Catalog

	sessions    *sessionStore[Session, *Session]
	retro       *sessionStore[RetroSession, *RetroSession]
	remoteplay  *sessionStore[RemoteplaySession, *RemoteplaySession]
	subMu       sync.Mutex
	subscribers map[chan struct{}]bool

	// baselines — la première mesure de chaque partie, pour les écarts du
	// panneau de performances (sessionstatus.Baselines).
	baselines sessionstatus.Baselines
}

func New(dockerHandler *docker.Client, catalogs map[string]*catalog.Catalog, cfg config.Config) *Manager {
	manager := &Manager{
		config:        cfg,
		dockerHandler: dockerHandler,
		catalogs:      catalogs,
		diskCatalogs:  map[string]*catalog.Catalog{},
		sessions:      newSessionStore[Session, *Session](),
		retro:         newSessionStore[RetroSession, *RetroSession](),
		remoteplay:    newSessionStore[RemoteplaySession, *RemoteplaySession](),
		subscribers:   map[chan struct{}]bool{},
	}

	manager.adoptExisting()
	go manager.watchdog()
	go manager.watchdogRemoteplay()

	return manager
}

// adoptExisting rebuilds every platform's records from live Docker
// containers at startup — a kernel restart (air hot-reload) must not strand
// sessions the daemon still has running. Each platform's adoption is
// independent: an error on one (e.g. AdoptExistingRetro) must not skip the
// others — a bug found while writing this generic version (the old code
// had a bare `return` on error that silently skipped remoteplay adoption
// whenever retro's adoption failed).
func (manager *Manager) adoptExisting() {
	adopted, err := manager.dockerHandler.AdoptExisting(context.Background())
	if err != nil {
		slog.Warn("adopting existing sessions failed", "error", err)
	}
	for _, adoptedSession := range adopted {
		session := manager.sessionFromAdopted(adoptedSession)
		manager.sessions.register(session)
		slog.Info("adopted session", "session_id", session.SessionID, "container", session.Container)
	}

	// Retro containers too — a service restart must not strand live
	// emulation (or worse, let the streamex TTL reap it as a foreign
	// record; the docker-side adoption now separates retro from streamex).
	adoptedRetro, err := manager.dockerHandler.AdoptExistingRetro(context.Background())
	if err != nil {
		slog.Warn("adopting existing retro sessions failed", "error", err)
	}
	for _, adoptedSession := range adoptedRetro {
		record := &RetroSession{
			BaseSession: BaseSession{
				SessionID: adoptedSession.SessionID,
				Container: adoptedSession.Container,
				Owner:     adoptedSession.Owner,
				Port:      adoptedSession.Port,
				StartedAt: startedAtOf(adoptedSession.StartedAt),
				State:     "running",
				// PlayerCount refreshed by the watchdog's next poll.
			},
			ConsoleID:  adoptedSession.ConsoleID,
			Reference:  adoptedSession.Reference,
			Title:      adoptedSession.Title,
			Core:       adoptedSession.Core,
			Bios:       adoptedSession.Bios,
			MaxPlayers: consoles.MaxPlayers(adoptedSession.ConsoleID),
		}
		manager.retro.register(record)
		slog.Info("adopted retro session", "session_id", record.SessionID, "container", record.Container)
	}

	// Remoteplay containers too — a kernel restart (air hot-reload) must
	// not strand a live PS5 session: an unadopted container is invisible
	// to the manager, so the next Connect would spawn a SECOND Takion for
	// the same console (handshake 403, or a degraded stream whose feedback
	// keys never arrive — frozen video and dead input, seen live).
	adoptedRemoteplay, err := manager.dockerHandler.AdoptExistingRemoteplay(context.Background())
	if err != nil {
		slog.Warn("adopting existing remoteplay sessions failed", "error", err)
	}
	for _, adoptedSession := range adoptedRemoteplay {
		record := &RemoteplaySession{
			BaseSession: BaseSession{
				SessionID: adoptedSession.SessionID,
				Container: adoptedSession.Container,
				Owner:     adoptedSession.Owner,
				Port:      adoptedSession.Port,
				StartedAt: startedAtOf(adoptedSession.StartedAt),
				State:     "running",
			},
			HostID:   adoptedSession.HostID,
			HostName: adoptedSession.HostName,
			HostType: adoptedSession.HostType,
		}
		manager.remoteplay.register(record)
		slog.Info("adopted remoteplay session", "session_id", record.SessionID, "container", record.Container, "port", record.Port)
	}
}

// startedAtOf — l'heure de lancement d'une partie retrouvée : la création de
// son conteneur, ou maintenant si Docker ne l'a pas dite.
func startedAtOf(created time.Time) time.Time {
	if created.IsZero() || created.Unix() <= 0 {
		return time.Now()
	}

	return created
}

func (manager *Manager) sessionFromAdopted(adopted docker.AdoptedSession) *Session {
	return &Session{
		BaseSession: BaseSession{
			SessionID: adopted.SessionID,
			Container: adopted.Container,
			Owner:     adopted.Owner,
			Port:      adopted.Port,
			StartedAt: startedAtOf(adopted.StartedAt),
			State:     "running",
		},
		Game:    adopted.Game,
		Console: adopted.Console,
	}
}

// LaunchSettings are the optional per-launch stream quality overrides from
// the frontend's Stream Settings form (GameDetailPanel, external consoles).
// Zero values mean "keep the console's default"; normalization below
// revalidates everything the form can send.
type LaunchSettings struct {
	Resolution string // "540p" | "720p" | "1080p", anything else ignored
	FPS        int    // 15-60, otherwise ignored
	Bitrate    int    // kbps, 500-20000, otherwise ignored
	Language   string // the page's language, "fr" | "en" (chantier 21), otherwise ignored
	// Core — l'émulateur choisi sur la fiche, au nom du registre ; vide, le
	// premier de la console (#438 : Eden, puis Citron, pour la Switch).
	Core string
}

// normalizeLaunchSettings revalidates what the form sent, with the table and
// the bounds the phone uses too (`consoles.StreamSize`, `StreamFPS`,
// `StreamBitrate`): the resolution becomes the container's RESOLUTION env
// format (streamex contract, docs/GAMEFLEX.md), anything unknown — « 480p »,
// retired on 2026-09-20 — empties it and the container keeps its console's
// own geometry.
func normalizeLaunchSettings(settings LaunchSettings) LaunchSettings {
	if width, height, ok := consoles.StreamSize(settings.Resolution); ok {
		settings.Resolution = fmt.Sprintf("%dx%d", width, height)
	} else {
		settings.Resolution = ""
	}
	settings.FPS = consoles.StreamFPS(settings.FPS)
	settings.Bitrate = consoles.StreamBitrate(settings.Bitrate)
	settings.Language = consoles.Language(settings.Language)
	return settings
}

func (manager *Manager) Launch(
	ctx context.Context, console, username, gameIdentifier string,
	settings LaunchSettings, progress func(step, detail string),
) (*Session, error) {
	// LA MÊME COUTURE QUE `LaunchRetro`, et volontairement à l'identique : un
	// conteneur qu'on crée puis qu'on attend, c'est la même chose ici et là, et
	// c'est la plus longue attente du parcours. Le manager ignore qu'un canal
	// existe — il annonce, quelqu'un écoute ou personne.
	announce := func(step, detail string) {
		if progress != nil {
			progress(step, detail)
		}
	}

	if !usernamePattern.MatchString(username) {
		return nil, ErrInvalidUsername
	}

	// Revalidate everything the form can send — an empty/invalid value
	// falls back to the console's default (0/"") downstream.
	settings = normalizeLaunchSettings(settings)

	gameCatalog := manager.catalogs[console]
	if gameCatalog == nil {
		return nil, ErrUnsupportedConsole
	}

	// LA BIBLIOTHÈQUE PRINCIPALE D'ABORD, PUIS LES DISQUES INTERNES dans
	// l'ordre de leur nom (chantier 23) : un jeu a la même référence partout,
	// et se joue depuis le premier endroit qui l'a.
	gamesHostDir := ""
	game, err := gameCatalog.Resolve(gameIdentifier)
	gamePath := gameCatalog.Path(game)
	if err != nil {
		game, gamesHostDir, gamePath, err = manager.resolveOnDisks(ctx, console, gameIdentifier)
		if err != nil {
			// LE JEU EST LÀ, mais rien dedans ne se lance : le dire, au lieu
			// d'un « game not found » qui ment (#431, un paquet PS5).
			if manager.holdsFolder(ctx, console, gameCatalog, gameIdentifier) {
				return nil, unplayableError(fmt.Sprintf(
					"« %s » est bien dans la bibliothèque, mais aucun de ses fichiers ne se lance — il faut un jeu extrait ou un fichier %s",
					gameIdentifier, catalog.Formats(console)))
			}
			return nil, ErrGameNotFound
		}
	}

	if console != "keytest" {
		// Stop the player's previous session BEFORE spawning the new one so
		// the GPU and the port are free and the "1 session per player" rule
		// holds. Keytest previews (SeeLayout) are exempt: a test screen must
		// never kill a real game in progress.
		manager.stopOwnedSessions(ctx, username)
	} else {
		// UN SEUL KEYTEST PAR COMPTE (#601) : la page réutilise le sien, mais
		// rien ne l'imposait ici — un compte ouvrait autant d'écrans de test
		// que streamex avait de places, et plus personne ne jouait.
		manager.stopOwnedKeytests(ctx, username)
	}

	// Per-console container budget (GAMEFLEX_CONSOLE_LIMITS) — the emulators'
	// GPU/CPU footprint differs wildly, so each console gets its own cap.
	// The user's OWN previous session was just stopped above, so a relaunch
	// never trips its own console's limit.
	if maximum, limited := manager.config.ConsoleLimits[console]; limited && manager.runningCount(console) >= maximum {
		return nil, ErrConsoleLimitReached
	}

	if console != "keytest" {
		// Keytest sessions have no saves (display-only console).
		if err := manager.dockerHandler.EnsureSaveDir(username, console); err != nil {
			return nil, fmt.Errorf("create save dir: %w", err)
		}
	}

	// Container names read "streamex-<console>-<nnn>" — `docker ps` alone
	// tells which emulator is running and how many of it. The initiator is
	// traced through the gameflex.owner label (spec.Username), never the
	// name: an owner change must not require a rename.
	spec := docker.SpawnSpec{
		SessionID:      newSessionID(),
		GameRelPath:    game.RelPath,
		GameTitle:      game.Title,
		Console:        console,
		Username:       username,
		Seats:          manager.Seats(console),
		ContainerBase:  "streamex-" + console,
		ContainerIndex: manager.runningCount(console) + 1,
		Resolution:     settings.Resolution,
		FPS:            settings.FPS,
		Bitrate:        settings.Bitrate,
		Language:       settings.Language,
		Core:           settings.Core,
		GamesHostDir:   gamesHostDir,
		Engine:         gameEngine(console, gamePath, game.RelPath),
	}

	announce("container.creating", spec.ContainerBase)

	port, containerName, err := manager.dockerHandler.CreateAndStart(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("spawn container: %w", err)
	}

	announce("container.created", containerName)

	// QUATRE-VINGT-DIX SECONDES POSSIBLES, et aucun nom à l'écran jusqu'ici :
	// sans cette annonce, l'attente la plus longue passait pour un trou.
	announce("container.waiting", containerName)

	// LE DÉBALLAGE D'UN JEU PS4 SE MONTRE (2026-10-22) : le pourcentage et
	// ce qui est déjà écrit, sur une seule ligne de la page. Le pourcentage
	// compare l'écrit à la taille du .pkg, qui est compressé (FF7 Remake :
	// 85 Go de .pkg, 145 Go déballés) : il plafonne à 99, et seule la taille
	// avance encore ensuite (chantier 22). Une étape par dizaine de
	// pourcents ou par gigaoctet écrit.
	lastTenth, lastGiB := int64(-1), int64(-1)
	onUnpacking := func(done, total int64) {
		if total <= 0 {
			return
		}
		percent := min(99, done*100/total)
		if percent/10 == lastTenth && done>>30 == lastGiB {
			return
		}
		lastTenth, lastGiB = percent/10, done>>30
		announce("game.unpacking", fmt.Sprintf("%d %% · %s", percent, writtenSize(done)))
	}
	if err := manager.dockerHandler.WaitReadyHostExternal(ctx, containerName, port, 90*time.Second, onUnpacking); err != nil {
		_ = manager.dockerHandler.Stop(context.Background(), containerName)
		return nil, fmt.Errorf("spawn container: %w", err)
	}

	announce("container.ready", containerName)

	session := &Session{
		BaseSession: BaseSession{
			SessionID: spec.SessionID,
			Container: containerName,
			Owner:     username,
			Port:      port,
			StartedAt: time.Now(),
			State:     "running",
		},
		Game:    game.Title,
		GameID:  game.ID,
		Console: console,
	}

	manager.sessions.register(session)
	manager.notify()

	slog.Info("session launched", "session_id", session.SessionID, "container", session.Container, "game", session.Game, "owner", username, "port", port)
	return session, nil
}

// runningCount counts live streamex sessions of one console — the source of
// both the spawn numbering and the per-console limit check.
// RequiredEmulators — les fichiers d'émulateur qu'attendent les images de
// console installées ici (chantier 29).
func (manager *Manager) RequiredEmulators(ctx context.Context) []docker.RequiredEmulator {
	return manager.dockerHandler.RequiredEmulators(ctx)
}

// InternalDisks — les disques internes de cette machine (chantier 23).
func (manager *Manager) InternalDisks(ctx context.Context) []docker.InternalDisk {
	return manager.dockerHandler.InternalDisks(ctx)
}

// resolveOnDisks — un jeu streamex cherché sur les disques internes présents,
// dans l'ordre de leur nom. Rend le jeu et le dossier de la console sur
// l'hôte, à monter sur /opt/game.
func (manager *Manager) resolveOnDisks(ctx context.Context, console, identifier string) (catalog.Game, string, string, error) {
	for _, disk := range manager.dockerHandler.InternalDisks(ctx) {
		if !disk.Present {
			continue
		}
		key := disk.Name + "/" + console
		manager.diskCatalogsMu.Lock()
		diskCatalog, ok := manager.diskCatalogs[key]
		if !ok {
			diskCatalog = catalog.New(console, filepath.Join(disk.RomsDir(), console))
			manager.diskCatalogs[key] = diskCatalog
		}
		manager.diskCatalogsMu.Unlock()

		if game, err := diskCatalog.Resolve(identifier); err == nil {
			return game, filepath.Join(disk.RomsHostDir(), console), diskCatalog.Path(game), nil
		}
	}

	return catalog.Game{}, "", "", ErrGameNotFound
}

// gameEngine — LE MOTEUR DU JEU, reconnu au lancement à ses fichiers, par la
// règle du scan (storage.GameEngine, chantier 36) : ses réglages passent
// par-dessus les paramètres de l'émulateur. `path` est le jeu tel que le kernel
// le voit. Un jeu rangé à plat dans le dossier de la console (un `.pkg` posé
// là) n'a pas de dossier à lui : pas de moteur — comme un `.pkg` dans son
// dossier, qui n'a pas encore de fichiers à montrer.
func gameEngine(console, path, relPath string) string {
	dir := path
	if info, err := os.Stat(path); err != nil {
		return ""
	} else if !info.IsDir() {
		if filepath.Dir(relPath) == "." {
			return ""
		}
		dir = filepath.Dir(path)
	}

	return storage.GameEngineAt(console, dir)
}

// holdsFolder — un dossier de ce nom est-il rangé pour cette console, ici ou
// sur un disque interne présent ?
func (manager *Manager) holdsFolder(ctx context.Context, console string, local *catalog.Catalog, identifier string) bool {
	if local.HoldsFolder(identifier) {
		return true
	}
	for _, disk := range manager.dockerHandler.InternalDisks(ctx) {
		if disk.Present && catalog.New(console, filepath.Join(disk.RomsDir(), console)).HoldsFolder(identifier) {
			return true
		}
	}

	return false
}

func (manager *Manager) runningCount(console string) int {
	return manager.sessions.runningCount(func(s Session) bool { return s.Console == console })
}

// Seats — how many people can hold a pad in a game of this console: the
// shared registry's count, or the container's ceiling for what it does not
// know (the KeyTest bench, which wants every pad it can get). ONE definition:
// the queue the child runs and the pool the dashboard reads both take it from
// here.
func (manager *Manager) Seats(console string) int {
	return consoles.SeatsOr(console, manager.config.MaxPlayers)
}

func (manager *Manager) stopOwnedSessions(ctx context.Context, username string) {
	for _, sessionID := range manager.sessions.ownedRunning(username) {
		manager.stop(ctx, sessionID, "relaunch")
	}
}

// stopOwnedKeytests — l'écran de test des touches précédent de ce compte, et
// lui seul : sa partie en cours continue.
func (manager *Manager) stopOwnedKeytests(ctx context.Context, username string) {
	for _, sessionID := range manager.sessions.ownedRunning(username) {
		if session, found := manager.sessions.lookup(sessionID); found && session.Console == "keytest" {
			manager.stop(ctx, sessionID, "relaunch")
		}
	}
}

// LaunchRetro spawns one retrogaming session container. Same rules as
// Launch: one PLAYING session per user (a relaunch auto-stops the previous
// one) and the optional per-console budget from GAMEFLEX_CONSOLE_LIMITS.
// `settings` — la résolution choisie avant le lancement (#150). Normalisée
// ICI, par la même fonction que streamex et remote-play : le vocabulaire
// (« 480p ») et sa traduction en pixels ne vivent qu'à un seul endroit.
// `progress` — les deux étapes que SEUL ce niveau connaît (#191) : le conteneur
// créé, puis le conteneur qui répond. Elles durent, et elles étaient invisibles
// — l'attente peut atteindre quatre-vingt-dix secondes derrière un unique
// « En attente de la vidéo… ».
//
// Un paramètre à part et NON un champ de LaunchSettings : celui-ci est un sac
// de RÉGLAGES (résolution, images/seconde, débit) partagé par les trois
// plateformes, et un rappel n'est pas un réglage. Peut être nil — un appelant
// qui ne veut rien annoncer n'a rien à fournir.
func (manager *Manager) LaunchRetro(
	ctx context.Context, consoleID, reference, title, core, bios, username string,
	disk *docker.InternalDisk, settings LaunchSettings, progress func(step, detail string),
) (*RetroSession, error) {
	announce := func(step, detail string) {
		if progress != nil {
			progress(step, detail)
		}
	}

	if !usernamePattern.MatchString(username) {
		return nil, ErrInvalidUsername
	}
	if !consoleStreams(consoleID) {
		return nil, ErrUnsupportedConsole
	}

	manager.stopOwnedRetroSessions(ctx, username)

	if maximum, limited := manager.config.ConsoleLimits[consoleID]; limited && manager.runningRetroCount(consoleID) >= maximum {
		return nil, ErrConsoleLimitReached
	}

	settings = normalizeLaunchSettings(settings)

	spec := docker.RetroSpawnSpec{
		SessionID:      newSessionID(),
		ConsoleID:      consoleID,
		Reference:      reference,
		Core:           core,
		Bios:           bios,
		Username:       username,
		GameTitle:      title,
		Resolution:     settings.Resolution,
		FPS:            settings.FPS,
		Bitrate:        settings.Bitrate,
		Language:       settings.Language,
		ContainerBase:  "retro-" + consoleID,
		ContainerIndex: manager.runningRetroCount(consoleID) + 1,
		Disk:           disk,
	}

	announce("container.creating", spec.ContainerBase)

	port, containerName, err := manager.dockerHandler.CreateAndStartRetro(ctx, spec)
	if err != nil {
		return nil, fmt.Errorf("spawn container: %w", err)
	}

	// CRÉÉ N'EST PAS PRÊT, et la distinction vaut la seconde étape : docker rend
	// la main dès que le conteneur démarre, l'émulateur met encore du temps à
	// répondre — jusqu'à quatre-vingt-dix secondes, sondées chaque seconde
	// ci-dessous. C'est la plus longue attente du parcours, et elle n'avait
	// aucun nom à l'écran.
	announce("container.created", containerName)

	announce("container.waiting", containerName)

	if err := manager.dockerHandler.WaitReadyHostRetro(ctx, containerName, port, 90*time.Second); err != nil {
		_ = manager.dockerHandler.Stop(context.Background(), containerName)
		return nil, fmt.Errorf("spawn container: %w", err)
	}

	// LA PLUS LONGUE ATTENTE DU PARCOURS VIENT DE FINIR, et elle mérite son
	// propre nom : la laisser se terminer en silence, suivie seulement de
	// « session prête », ferait passer quatre-vingt-dix secondes possibles pour
	// un trou. Une étape qui dure est précisément celle qu'il faut annoncer.
	announce("container.ready", containerName)

	record := &RetroSession{
		BaseSession: BaseSession{
			SessionID:   spec.SessionID,
			Container:   containerName,
			Owner:       username,
			Port:        port,
			StartedAt:   time.Now(),
			State:       "running",
			PlayerCount: 1, // owner — the watchdog refreshes from /status
		},
		ConsoleID:  consoleID,
		Reference:  reference,
		Title:      title,
		Core:       core,
		Bios:       bios,
		MaxPlayers: consoles.MaxPlayers(consoleID),
	}

	manager.retro.register(record)
	manager.notify()

	slog.Info("retro session launched", "session_id", record.SessionID, "container", record.Container, "console", consoleID, "game", title, "owner", username, "port", port)
	return record, nil
}

// consoleStreams reports whether the shared registry knows a RETROGAMING
// console — launch validation (the child re-validates the core at start).
// Streamex consoles (switch/ps3/xbox) are deliberately NOT streamed by the
// retrogaming image and go through Launch instead.
func consoleStreams(consoleID string) bool {
	for _, entry := range consoles.List() {
		if entry.ID == consoleID && entry.Method == consoles.MethodRetroGaming {
			return true
		}
	}
	return false
}

// StopRetro removes the session container and the record. Reports whether
// the session existed.
func (manager *Manager) StopRetro(ctx context.Context, sessionID string) bool {
	ok := stopSession(manager.retro, manager.dockerHandler, ctx, sessionID, "retro")
	if ok {
		manager.baselines.Forget(sessionID)
		manager.notify()
	}
	return ok
}

// SweepRetro re-runs the watchdog's checks for ONE session — called by the
// idle watchdog's container sweep (api/idle.go, sweepContainer), so the
// record and the façade entry drop as soon as the child exits instead of
// waiting for the next 20 s tick. Idempotent: an unknown session is a no-op.
func (manager *Manager) SweepRetro(ctx context.Context, sessionID string) {
	record, ok := manager.retro.lookup(sessionID)
	if !ok {
		return
	}

	running, err := manager.dockerHandler.IsRunning(ctx, record.Container)
	if err != nil || !running {
		manager.StopRetro(ctx, sessionID)
		return
	}

	status, err := manager.dockerHandler.StatusRetro(ctx, record.Container, record.Port)
	if err != nil {
		return
	}
	if manager.retro.patchPlayerCount(sessionID, status.Players) {
		manager.notify()
	}
}

// RetroStatus returns the session container's live CPU/RAM telemetry
// (self-reported by the child, the old /proc sampling the supervisor did).
func (manager *Manager) RetroStatus(sessionID string) (sessionstatus.Status, bool) {
	record, ok := manager.retro.lookup(sessionID)
	if !ok {
		return sessionstatus.Status{}, false
	}

	status, err := manager.dockerHandler.StatusRetro(context.Background(), record.Container, record.Port)
	if err != nil {
		return sessionstatus.Status{}, false
	}
	// UNE SEULE FORME, POUR LES TROIS PLATEFORMES ET LE TÉLÉPHONE.
	//
	// Cette fonction rendait une carte écrite à la main, en snake_case, avec
	// le GPU imbriqué — et le panneau, lui, lisait `cpuPercent`/`rssBytes`/
	// `vramUsedBytes`. Aucune des clés ne se rencontrait : toutes ces lignes
	// affichaient un tiret alors que la mesure arrivait bel et bien (vu le
	// 2026-09-08, 13 Mo de RSS envoyés, « — » à l'écran). Voir le paquet
	// sessionstatus, qui porte désormais la forme une seule fois.
	//
	// `fps` et le GPU restent facultatifs pour la même raison qu'avant : la
	// cadence n'existe qu'après le premier échantillon de cinq secondes, le
	// GPU seulement si l'hôte expose un sysfs lisible. Absent veut dire « pas
	// mesurable », ce qui n'est pas zéro.
	host := sessionstatus.ServerResources(manager.config.GPU)
	out := sessionstatus.Status{
		StartedAt:    record.StartedAt,
		Emulator:     status.Emulator,
		Host:         &host,
		Players:      status.Players,
		Spectators:   status.Spectators,
		EmulationFPS: status.FPS,
		CPUPercent:   sessionstatus.Percent(status.CPUPercent),
		RSSBytes:     status.RSSBytes,
	}
	if status.GPU != nil {
		out.GPUBusyPercent = sessionstatus.Percent(float64(status.GPU.BusyPercent))
		out.VRAMUsedBytes = int64(status.GPU.VRAMUsedBytes)
		out.VRAMTotalBytes = int64(status.GPU.VRAMTotalBytes)
	}
	// Ce que Docker compte pour le conteneur ENTIER, en plus du processus —
	// voir ContainerMemory. Best-effort : une session qui vient de mourir
	// rend zéro plutôt que de faire échouer tout le statut.
	out.ContainerRSSBytes, out.ContainerLimitBytes = manager.dockerHandler.ContainerMemory(
		context.Background(), record.Container,
	)
	sessionstatus.ServerPower(&out)
	manager.baselines.Stamp(sessionID, &out)

	return out, true
}

// LookupRetro returns a copy of one retro session record.
func (manager *Manager) LookupRetro(sessionID string) (RetroSession, bool) {
	return manager.retro.lookup(sessionID)
}

// LookupStreamex returns a copy of one streamex session record — StopStreamex's
// own sibling, same reasoning as LookupRetro (handleStreamexStop needs the Owner
// before it can decide whether the caller is allowed to stop it). Named
// explicitly (not the bare "Lookup" it used to be) to match LookupRetro/
// LookupRemoteplay — the unprefixed name read as "works for any platform"
// when it only ever covered streamex, confusing next to its siblings.
func (manager *Manager) LookupStreamex(sessionID string) (Session, bool) {
	return manager.sessions.lookup(sessionID)
}

// ListRetro returns retro sessions sorted by start time (oldest first) —
// same deterministic order as List.
func (manager *Manager) ListRetro() []RetroSession {
	return manager.retro.list()
}

func (manager *Manager) runningRetroCount(consoleID string) int {
	return manager.retro.runningCount(func(r RetroSession) bool { return r.ConsoleID == consoleID })
}

func (manager *Manager) stopOwnedRetroSessions(ctx context.Context, username string) {
	for _, sessionID := range manager.retro.ownedRunning(username) {
		manager.StopRetro(ctx, sessionID)
	}
}

// StopStreamex removes the streamex session container and the record.
// Reports whether the session existed. Named explicitly (not the bare
// "Stop" it used to be) to match StopRetro/StopRemoteplay.
func (manager *Manager) StopStreamex(ctx context.Context, sessionID string) bool {
	return manager.stop(ctx, sessionID, "stop")
}

func (manager *Manager) stop(ctx context.Context, sessionID, reason string) bool {
	ok := stopSession(manager.sessions, manager.dockerHandler, ctx, sessionID, "streamex")
	if ok {
		manager.baselines.Forget(sessionID)
		manager.notify()
		slog.Debug("streamex session stop reason", "session_id", sessionID, "reason", reason)
	}
	return ok
}

// List returns sessions sorted by start time (oldest first) — deterministic
// order so WS consumers never see the list reshuffle on every push.
func (manager *Manager) List() []Session {
	return manager.sessions.list()
}

func (manager *Manager) Subscribe() chan struct{} {
	channel := make(chan struct{}, 1)
	manager.subMu.Lock()
	manager.subscribers[channel] = true
	manager.subMu.Unlock()
	return channel
}

func (manager *Manager) notify() {
	manager.subMu.Lock()
	defer manager.subMu.Unlock()

	for channel := range manager.subscribers {
		select {
		case channel <- struct{}{}:
		default:
		}
	}
}

// pruneExitedContainers removes stopped session containers that no
// record owns — they exited while this service was down or mid-restart
// (adoption only ever sees RUNNING containers), so nothing else would ever
// remove them. Called on every watchdog tick, cheap (one daemon list).
func (manager *Manager) pruneExitedContainers() {
	ctx := context.Background()

	containerToID := make(map[string]string)
	for _, session := range manager.sessions.list() {
		containerToID[session.Container] = session.SessionID
	}
	for _, record := range manager.retro.list() {
		containerToID[record.Container] = record.SessionID
	}

	exited, err := manager.dockerHandler.ListExitedSessionContainers(ctx)
	if err != nil {
		slog.Warn("listing exited session containers failed", "error", err)
		return
	}
	for _, containerName := range exited {
		if _, knownSession := containerToID[containerName]; knownSession {
			continue
		}
		if err := manager.dockerHandler.Stop(ctx, containerName); err != nil {
			slog.Warn("pruning exited session container failed", "container", containerName, "error", err)
			continue
		}
		slog.Info("pruned exited session container", "container", containerName)
	}
}

// watchdog keeps the manager records in sync with the daemon reality:
// exited containers drop their records, and the live player count is
// mirrored into the records (notify → bridge → pool). The IDLE shutdown
// itself lives in the kernel's unified pool watchdog (api/idle.go) — the
// per-platform TTL/grace rules this loop used to run (streamex
// SessionTTLSeconds, retro retroPlayersAt) were removed in étape 3.
func (manager *Manager) watchdog() {
	ticker := time.NewTicker(watchdogInterval)
	defer ticker.Stop()

	for range ticker.C {
		manager.pruneExitedContainers()
		ctx := context.Background()

		for _, session := range manager.List() {
			running, err := manager.dockerHandler.IsRunning(ctx, session.Container)
			if err != nil || !running {
				manager.stop(ctx, session.SessionID, "container-gone")
				continue
			}

			status, err := manager.dockerHandler.StatusExternal(ctx, session.Container, session.Port)
			if err != nil {
				continue
			}

			// Keep the live player count in the record (and the façade,
			// via notify → bridge) so join buttons see real slot occupancy.
			if manager.sessions.patchPlayerCount(session.SessionID, status.Players) {
				manager.notify()
			}
		}

		// Retro sessions: the child is PID 1, so the container stops itself
		// when the session ends (its own 5-min grace covers orphans and
		// improper leaves) — this sweep only cleans the record up and
		// mirrors the live player count (same notify → bridge flow as above).
		for _, session := range manager.ListRetro() {
			running, err := manager.dockerHandler.IsRunning(ctx, session.Container)
			if err != nil || !running {
				manager.StopRetro(ctx, session.SessionID)
				continue
			}

			status, err := manager.dockerHandler.StatusRetro(ctx, session.Container, session.Port)
			if err != nil {
				continue
			}

			if manager.retro.patchPlayerCount(session.SessionID, status.Players) {
				manager.notify()
			}
		}
	}
}

// newSessionID — 128 BITS, PARCE QUE C'EST UN SECRET (#458).
//
// L'identifiant d'une partie suffit à la regarder : la passerelle vidéo
// l'accepte sans jeton, c'est ce qui sert les liens de partage. Il faisait
// quatre octets — quatre milliards de valeurs, sans aucune limite aux essais
// (relevé le 2026-10-24). Seize octets ne se devinent pas.
func newSessionID() string {
	return randomHex(16)
}

func randomHex(byteCount int) string {
	buffer := make([]byte, byteCount)
	if _, err := rand.Read(buffer); err != nil {
		// crypto/rand only fails catastrophically; keep sessions working.
		slog.Error("crypto/rand failed", "error", err)
		fallback := fmt.Sprintf("%016x", time.Now().UnixNano())
		for len(fallback) < byteCount*2 {
			fallback += fallback
		}

		return fallback[:byteCount*2]
	}

	return hex.EncodeToString(buffer)
}

// StreamexStatus — le pendant de RetroStatus pour les consoles externes, dans
// la MÊME forme (sessionstatus.Status). Les deux champs que l'enfant ne peut
// pas connaître — depuis quand la partie tourne, et ce que Docker compte pour
// son conteneur — sont ajoutés ici, où ils sont su.
func (manager *Manager) StreamexStatus(sessionID string) (sessionstatus.Status, bool) {
	record, ok := manager.sessions.lookup(sessionID)
	if !ok {
		return sessionstatus.Status{}, false
	}

	status, err := manager.dockerHandler.StatusStreamex(manager.config.PublicHost, record.Port)
	if err != nil {
		return sessionstatus.Status{}, false
	}
	status.StartedAt = record.StartedAt
	host := sessionstatus.ServerResources(manager.config.GPU)
	status.Host = &host
	status.ContainerRSSBytes, status.ContainerLimitBytes = manager.dockerHandler.ContainerMemory(
		context.Background(), record.Container,
	)
	sessionstatus.ServerPower(&status)
	manager.baselines.Stamp(sessionID, &status)

	return status, true
}

// IsRunning — is this session container still up? For the kernel's stop path
// (api/child_end.go), which lets a child that agreed to end its game exit on
// its own before tidying up behind it. A nil manager or Docker handler (tests)
// answers false: nothing to wait for.
func (manager *Manager) IsRunning(ctx context.Context, container string) (bool, error) {
	if manager == nil || manager.dockerHandler == nil {
		return false, nil
	}

	return manager.dockerHandler.IsRunning(ctx, container)
}

// writtenSize — des octets écrits, lisibles d'un coup d'œil : en Mo sous le
// gigaoctet, en Go au-delà, avec les unités que la page emploie déjà (« GB »).
func writtenSize(bytes int64) string {
	if bytes < 1<<30 {
		return fmt.Sprintf("%d MB", bytes>>20)
	}

	return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
}
