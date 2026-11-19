package api

// LE STATUT DES JEUX (chantier 30) — jouable, plante en jeu, injouable, non
// testé, posé par un administrateur dans Réglages › Jeux, vu par tous dans la
// liste des jeux, et par les amis dans le listing qu'ils recopient.
//
// Le fichier et le calcul sont partagés (`gameflex/storage/gamestatus`) ; ici
// vivent ce que seul le kernel sait : où est le dossier, quels émulateurs une
// console a d'installés et en quelle version, qui demande, et le listing à
// tenir à jour après chaque écriture.
//
//	GET  /api/v1/retrogaming/{consoleId}/roms/{ref}/status   l'historique, versions comprises
//	POST /api/v1/retrogaming/{consoleId}/roms/{ref}/status   un signalement
//	GET  /api/v1/retrogaming/{consoleId}/status              le statut de la console
//	POST /api/v1/retrogaming/{consoleId}/status              toute la console
//
// Réservées aux administrateurs. Les jeux d'un disque interne ont leur statut
// dans le dossier de ce serveur, sous le même identifiant : un jeu présent à
// deux endroits est un seul jeu, et le kernel n'écrit jamais sur un disque.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage/gamestatus"
	"gameflex/storage/listing"
)

// statusStore — les statuts de ce serveur, EN BASE (le porteur, 2026-11-17 : « Il faut que ça soit dans la base de
// données ») : celle du service des comptes, par ses routes internes (credentialStatuses). Sans service des comptes
// (les tests), une mémoire propre à ce serveur.
func (server *Server) statusStore() gamestatus.Store {
	if server.config.CredentialURL != "" && server.config.InternalSecret != "" {
		return gamestatus.Store{Backend: credentialStatuses{server: server}}
	}
	server.statusOnce.Do(func() { server.statusMemory = gamestatus.NewMemory() })

	return gamestatus.Store{Backend: server.statusMemory}
}

// credentialStatuses — gamestatus.Backend par les routes /internal/game-status du service des comptes.
type credentialStatuses struct {
	server *Server
}

func (statuses credentialStatuses) call(method, path string, body any, into any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, statuses.server.config.CredentialURL+"/internal/game-status"+path, reader)
	if err != nil {
		return err
	}
	request.Header.Set("X-Internal-Secret", statuses.server.config.InternalSecret)
	request.Header.Set("Content-Type", "application/json")
	response, err := statusClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("game status: credential %s %s = %d", method, path, response.StatusCode)
	}
	if into == nil {
		return nil
	}
	// credential rend sa réponse sous `item` (respondItem).
	var envelope struct {
		Item json.RawMessage `json:"item"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return err
	}

	return json.Unmarshal(envelope.Item, into)
}

var statusClient = &http.Client{Timeout: 15 * time.Second}

func (statuses credentialStatuses) Console(consoleID string) (gamestatus.Console, error) {
	var console gamestatus.Console
	err := statuses.call(http.MethodGet, "/"+url.PathEscape(consoleID), nil, &console)

	return console, err
}

func (statuses credentialStatuses) AddReport(consoleID, gameID string, report gamestatus.Report) error {
	return statuses.call(http.MethodPost, "/"+url.PathEscape(consoleID)+"/games/"+url.PathEscape(gameID), report, nil)
}

func (statuses credentialStatuses) RemoveReport(consoleID, gameID string, at time.Time) error {
	return statuses.call(http.MethodDelete, "/"+url.PathEscape(consoleID)+"/games/"+url.PathEscape(gameID)+"?at="+url.QueryEscape(at.UTC().Format(time.RFC3339Nano)), nil, nil)
}

func (statuses credentialStatuses) SetConsole(consoleID, status, by string, at time.Time) error {
	return statuses.call(http.MethodPut, "/"+url.PathEscape(consoleID), map[string]any{"status": status, "by": by, "at": at}, nil)
}

func (statuses credentialStatuses) Statuses() (map[string]string, error) {
	statusMap := map[string]string{}
	err := statuses.call(http.MethodGet, "", nil, &statusMap)

	return statusMap, err
}

// consoleStatus — le fichier d'une console, ou rien : un fichier illisible se
// dit au journal, et la liste montre « non testé » plutôt que de tomber.
func (server *Server) consoleStatus(consoleID string) gamestatus.Console {
	console, err := server.statusStore().Load(consoleID)
	if err != nil {
		slog.Warn("game status unreadable", "console", consoleID, "error", err)
	}

	return console
}

// statusView — le statut d'un jeu d'ici (bibliothèque ou disque interne).
func statusView(consoleID string, console gamestatus.Console, gameID string) *gamestatus.View {
	view := gamestatus.Resolve(consoleID, console.Status, gamestatus.Public(console.Games[gameID]))

	return &view
}

// ── Les versions ───────────────────────────────────────────────────────

// emulatorVersion — la version de l'émulateur installé, prise ici au moment
// du signalement : pour une console récente, la partie du nom de fichier que
// le motif de son image laisse libre (`Eden-Linux-*-amd64…` → `de7e6e01d4`) ;
// pour un cœur libretro, qui ne dit sa version qu'une fois chargé, la date de
// son fichier. Vide quand rien n'est installé.
func (server *Server) emulatorVersion(ctx context.Context, consoleID, emulator string) string {
	if strings.HasSuffix(emulator, "(streamex)") {
		need, ok := server.requiredEmulatorFor(ctx, consoleID, emulator)
		if !ok {
			return ""
		}
		file, _, ok := server.emulatorInUse(need)
		if !ok {
			return ""
		}
		if version := wildcardPart(need.Pattern, file); version != "" {
			return version
		}

		return file
	}
	for _, path := range []string{
		filepath.Join(server.retroCoresRoot(), consoleID, retro_files.CoreFileName(emulator)),
		filepath.Join(server.retroCoresRoot(), retro_files.CoreFileName(emulator)),
	} {
		if info, err := os.Stat(path); err == nil {
			return info.ModTime().UTC().Format("2006-01-02")
		}
	}

	return ""
}

// wildcardPart — ce qu'une `*` du motif a pris dans ce nom ; vide si le nom
// ne répond pas au motif, ou si le motif n'a pas exactement une `*`.
func wildcardPart(pattern, name string) string {
	prefix, suffix, found := strings.Cut(pattern, "*")
	if !found || strings.Contains(suffix, "*") {
		return ""
	}
	if !strings.HasPrefix(name, prefix) || !strings.HasSuffix(name, suffix) || len(name) < len(prefix)+len(suffix) {
		return ""
	}

	return name[len(prefix) : len(name)-len(suffix)]
}

// installedEmulators — les émulateurs de cette console qu'on peut lancer ici,
// dans l'ordre du registre : ceux que la fiche d'un jeu propose.
func (server *Server) installedEmulators(ctx context.Context, consoleID string) []string {
	installed := []string{}
	for _, emulator := range consoles.CoresFor(consoleID) {
		if server.emulatorVersion(ctx, consoleID, emulator) != "" {
			installed = append(installed, emulator)
		}
	}

	return installed
}

// ── Les routes ─────────────────────────────────────────────────────────

type emulatorInfo struct {
	Name      string `json:"name"`
	Version   string `json:"version,omitempty"`
	Installed bool   `json:"installed"`
}

type gameStatusAnswer struct {
	View    *gamestatus.View    `json:"view"`
	Console gamestatus.Console  `json:"console"`
	Reports []gamestatus.Report `json:"reports"`
	// Emulators — ceux de la console, installés ou non, avec leur version :
	// les cases du formulaire.
	Emulators []emulatorInfo `json:"emulators"`
}

func (server *Server) gameStatusAnswer(ctx context.Context, consoleID, gameID string) gameStatusAnswer {
	console := server.consoleStatus(consoleID)
	reports := console.Games[gameID]
	if reports == nil {
		reports = []gamestatus.Report{}
	}
	emulators := []emulatorInfo{}
	for _, emulator := range consoles.CoresFor(consoleID) {
		version := server.emulatorVersion(ctx, consoleID, emulator)
		emulators = append(emulators, emulatorInfo{Name: emulator, Version: version, Installed: version != ""})
	}
	consoleOnly := console
	consoleOnly.Games = nil

	return gameStatusAnswer{
		View: statusView(consoleID, console, gameID), Console: consoleOnly, Reports: reports, Emulators: emulators,
	}
}

// handleGameStatus — GET /api/v1/retrogaming/{consoleId}/roms/{ref}/status
func (server *Server) handleGameStatus(w http.ResponseWriter, r *http.Request) {
	consoleID, gameID := chi.URLParam(r, "consoleId"), chi.URLParam(r, "ref")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusNotFound, "console_unknown", "unknown console")

		return
	}
	writeJSON(w, http.StatusOK, server.gameStatusAnswer(r.Context(), consoleID, gameID))
}

// handleReportGame — POST /api/v1/retrogaming/{consoleId}/roms/{ref}/status
// {status, emulators, note}. `emulators` vide : tous ceux de la console.
func (server *Server) handleReportGame(w http.ResponseWriter, r *http.Request) {
	consoleID, gameID := chi.URLParam(r, "consoleId"), chi.URLParam(r, "ref")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusNotFound, "console_unknown", "unknown console")

		return
	}
	var body struct {
		Status    string   `json:"status"`
		Emulators []string `json:"emulators"`
		Note      string   `json:"note"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !gamestatus.Valid(body.Status) {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "status must be untested, playable, patched, glitches, crashes or unplayable")

		return
	}
	known := consoles.CoresFor(consoleID)
	for _, emulator := range body.Emulators {
		if !slices.Contains(known, emulator) {
			writeErrorCode(w, http.StatusBadRequest, "bad_request", "unknown emulator for this console: "+emulator)

			return
		}
	}
	if !server.gameIsHere(r.Context(), consoleID, gameID) {
		writeErrorCode(w, http.StatusNotFound, "game_not_found", "no such game on this server")

		return
	}

	// LES VERSIONS, de ceux qu'il concerne : tous ceux qu'on a ici quand il
	// n'en nomme aucun.
	covered := body.Emulators
	if len(covered) == 0 {
		covered = server.installedEmulators(r.Context(), consoleID)
	}
	versions := map[string]string{}
	for _, emulator := range covered {
		if version := server.emulatorVersion(r.Context(), consoleID, emulator); version != "" {
			versions[emulator] = version
		}
	}
	if len(versions) == 0 {
		versions = nil
	}

	report := gamestatus.Report{
		Status: body.Status, Emulators: body.Emulators, Versions: versions, Note: body.Note,
		By: auth.Username(r.Context()), At: time.Now().UTC(),
	}
	if err := server.statusStore().Add(consoleID, gameID, report); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gamestatus.ErrInvalid) {
			status = http.StatusBadRequest
		}
		writeErrorCode(w, status, "status_failed", err.Error())

		return
	}
	server.refreshConsoleRoms(consoleID)
	slog.Info("game status reported", "console", consoleID, "game", gameID, "status", body.Status,
		"emulators", body.Emulators, "by", report.By)

	writeJSON(w, http.StatusOK, server.gameStatusAnswer(r.Context(), consoleID, gameID))
}

// handleRemoveReport — DELETE /api/v1/retrogaming/{consoleId}/roms/{ref}/status?at=<RFC 3339> : retire un signalement
// de l'historique (chantier 68) ; la réponse est le statut recalculé, comme après un signalement.
func (server *Server) handleRemoveReport(w http.ResponseWriter, r *http.Request) {
	consoleID, gameID := chi.URLParam(r, "consoleId"), chi.URLParam(r, "ref")
	at, err := time.Parse(time.RFC3339Nano, r.URL.Query().Get("at"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "at must be the instant of a report (RFC 3339)")

		return
	}
	if err := server.statusStore().Remove(consoleID, gameID, at); err != nil {
		status := http.StatusInternalServerError
		if errors.Is(err, gamestatus.ErrInvalid) {
			status = http.StatusBadRequest
		}
		writeErrorCode(w, status, "status_failed", err.Error())

		return
	}
	server.refreshConsoleRoms(consoleID)
	slog.Info("game status report removed", "console", consoleID, "game", gameID, "at", at, "by", auth.Username(r.Context()))

	writeJSON(w, http.StatusOK, server.gameStatusAnswer(r.Context(), consoleID, gameID))
}

// handleGetConsoleStatus — GET /api/v1/retrogaming/{consoleId}/status : le
// statut de la console, qui l'a réglé et quand, sans les signalements.
func (server *Server) handleGetConsoleStatus(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusNotFound, "console_unknown", "unknown console")

		return
	}
	writeJSON(w, http.StatusOK, server.consoleStatus(consoleID).Setting(consoleID))
}

// handleConsoleStatus — POST /api/v1/retrogaming/{consoleId}/status {status}
// : le statut de tous les jeux de la console qui n'en ont pas à eux (« on va
// update les statuts pour faire passer à jouable pour tous », le porteur).
func (server *Server) handleConsoleStatus(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusNotFound, "console_unknown", "unknown console")

		return
	}
	var body struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil || !gamestatus.Valid(body.Status) {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "status must be untested, playable, patched, glitches, crashes or unplayable")

		return
	}
	now := time.Now().UTC()
	by := auth.Username(r.Context())
	if err := server.statusStore().SetConsole(consoleID, body.Status, by, now); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "status_failed", err.Error())

		return
	}
	server.refreshConsoleStatuses(now)
	slog.Info("console status set", "console", consoleID, "status", body.Status, "by", by)

	writeJSON(w, http.StatusOK, server.consoleStatus(consoleID).Setting(consoleID))
}

// gameIsHere — ce jeu est-il dans la bibliothèque ou sur un disque interne ?
// Un signalement pour un jeu qui n'existe pas ne ferait qu'encombrer le
// fichier.
func (server *Server) gameIsHere(ctx context.Context, consoleID, gameID string) bool {
	if gameID == "" {
		return false
	}

	return server.gamesHere(ctx, consoleID)[gameID]
}

// gamesHere — les identifiants des jeux d'une console qui sont ICI : la
// bibliothèque, puis chaque disque interne. Un jeu d'ailleurs qui porte l'un
// d'eux est « déjà ici » (chantier 14 : l'identifiant écrit au listing sert à
// ça ; chantier 53, #674).
func (server *Server) gamesHere(ctx context.Context, consoleID string) map[string]bool {
	here := map[string]bool{}
	for _, item := range server.selfItems(listing.Roms, consoleID) {
		if item.ID != "" {
			here[item.ID] = true
		}
	}
	for _, disk := range server.presentDisks(ctx) {
		roms, err := retro_files.ListCatalog(disk.RomsDir(), consoleID, "")
		if err != nil {
			continue
		}
		for _, rom := range roms {
			here[rom.Reference] = true
		}
	}

	return here
}

// ── Le listing ─────────────────────────────────────────────────────────

// refreshConsoleRoms — les jeux d'UNE console dans le listing de ce serveur,
// avec leurs signalements : ce que les amis recopient. Le fichier des statuts
// est déjà écrit ; le listing se refait sous son verrou, après lui, et un
// « Rescan all » qui croise relit donc le fichier à jour.
func (server *Server) refreshConsoleRoms(consoleID string) {
	selfMu.Lock()
	defer selfMu.Unlock()
	if !listing.Scanned(listingCacheDir, listing.SelfKey) {
		if err := server.rebuildSelfLocked(); err != nil {
			slog.Warn("self listing not built", "error", err)
		}

		return
	}
	items := romItems(server.selfRoots(), consoleID)
	if err := listing.SaveConsoleSection(listingCacheDir, listing.SelfKey, listing.Roms, consoleID, items, time.Now()); err != nil {
		slog.Warn("self listing not refreshed", "console", consoleID, "error", err)
	}
	server.rememberSelfListing()
}

// refreshConsoleStatuses — le statut des consoles dans l'en-tête du listing :
// la date avance, pour qu'un ami recopie.
func (server *Server) refreshConsoleStatuses(at time.Time) {
	selfMu.Lock()
	defer selfMu.Unlock()
	statuses := server.statusStore().Statuses()
	err := listing.SaveHeader(listingCacheDir, listing.SelfKey, func(header *listing.Header) {
		header.Statuses = statuses
		if at.After(header.UpdatedAt) {
			header.UpdatedAt = at.UTC()
		}
	})
	if err != nil && !errors.Is(err, listing.ErrNoCache) {
		slog.Warn("self listing header not refreshed", "error", err)
	}
	server.rememberSelfListing()
}
