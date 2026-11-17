package api

// LE DÉFAUT D'UNE CONSOLE (chantier 53, P4, #668 et #669) — l'émulateur en
// service, et le BIOS par défaut d'une console rétro, enregistrés ici et lus au
// lancement. Avant, rien ne l'était :
//
//   - en rétro, le choix de la fiche ne vivait que dans la mémoire de la page,
//     et sans lui le premier cœur du registre partait ;
//   - en Streamex, le fichier déposé en dernier gagnait (emulatorInUse,
//     unpack-emulator.sh), et le seul moyen d'en prendre un autre était d'y
//     toucher sur le disque.
//
// Le porteur, 2026-11-13 : « pour les consoles rétro "utiliser" devient
// l'émulateur par défaut si la console utilise plusieurs émulateurs. Mais par
// contre dans la fiche de jeu avant de lancer le jeu on peut toujours choisir
// quel émulateur on veut. Pour streamex c'est différent en effet. Le "in use"
// permet de choisir la version qu'on veut. Et quand il y a plusieurs
// émulateurs, le in use devient celui par défaut. » D'où :
//
//   - Core : l'émulateur par défaut (le nom du registre, celui que la fiche
//     choisit) ;
//   - EmulatorFile : en Streamex, LA version en service — un fichier de
//     cores/<console>/ ; elle fait de son émulateur celui par défaut ;
//   - Bios : le BIOS par défaut d'une console rétro (vide : le runtime dépose
//     tout le dossier et le cœur choisit, comme avant).
//
// La fiche choisit toujours pour UN lancement : un choix de la requête passe
// devant, ce qui est enregistré ne sert que quand elle ne dit rien.
//
// UN DÉFAUT QUI N'EST PLUS LÀ NE BLOQUE RIEN : un cœur, un fichier ou un BIOS
// supprimé depuis est ignoré au lancement (le journal le dit), et la console
// reprend la règle d'avant — le premier cœur du registre, le fichier déposé en
// dernier, tout le dossier des BIOS.
//
//	GET /api/v1/retrogaming/{consoleId}/defaults   ce qui est enregistré, et ce qui partirait
//	PUT /api/v1/retrogaming/{consoleId}/defaults   {core?, emulatorFile?, bios?} — administration

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"

	"github.com/go-chi/chi/v5"

	"gameflex/consoles"
	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/docker"
	"gameflex/kernel/internal/retro_files"
	"gameflex/storage"
	"gameflex/storage/listing"
)

// consoleDefaults — le fichier partagé avec le téléphone (gameflex/storage,
// chantier 54, P1) : sa forme et son écriture vivent là-bas.
type consoleDefaults = storage.ConsoleDefaults

// Un écrivain à la fois : deux administrateurs qui règlent la même console ne
// mêlent pas leurs fichiers.
var consoleDefaultsMu sync.Mutex

// consoleDefaultsOf — ce qui est enregistré pour une console ; rien quand le
// fichier manque, ou qu'il est illisible (le journal le dit).
func (server *Server) consoleDefaultsOf(consoleID string) consoleDefaults {
	defaults, err := storage.ReadConsoleDefaults(server.storageRoot, consoleID)
	if err != nil {
		slog.Warn("console defaults unreadable", "console", consoleID, "error", err)

		return consoleDefaults{}
	}

	return defaults
}

func (server *Server) writeConsoleDefaults(consoleID string, defaults consoleDefaults) error {
	return storage.WriteConsoleDefaults(server.storageRoot, consoleID, defaults)
}

// retroCoreFor — le cœur d'une partie rétro : celui de la requête, sinon le
// défaut de la console s'il est encore installé, sinon vide (le premier du
// registre, la règle du runtime).
func (server *Server) retroCoreFor(consoleID, requested string) string {
	if requested = strings.TrimSpace(requested); requested != "" {
		return requested
	}
	core := server.consoleDefaultsOf(consoleID).Core
	if core == "" {
		return ""
	}
	if err := requireCore(server.retroCoresRoot(), consoleID, core); err != nil {
		slog.Warn("console default core ignored", "console", consoleID, "core", core, "error", err)

		return ""
	}

	return core
}

// retroBiosFor — le BIOS d'une partie rétro : celui de la requête, sinon le
// défaut de la console s'il est encore là, sinon vide (tout le dossier).
func (server *Server) retroBiosFor(consoleID, requested string) string {
	if requested = strings.TrimSpace(requested); requested != "" {
		return requested
	}
	bios := server.consoleDefaultsOf(consoleID).Bios
	if bios == "" || server.biosInstalled(consoleID, bios) {
		return bios
	}
	slog.Warn("console default bios ignored", "console", consoleID, "bios", bios)

	return ""
}

// streamexCoreFor — l'émulateur d'une partie Streamex : celui de la requête,
// sinon le défaut enregistré (choisir une version en service l'y met), sinon
// vide (le premier de la console).
func (server *Server) streamexCoreFor(consoleID, requested string) string {
	if requested = strings.TrimSpace(requested); requested != "" {
		return requested
	}
	core := server.consoleDefaultsOf(consoleID).Core
	if core != "" && slices.Contains(docker.StreamexCores(consoleID), core) {
		return core
	}

	return ""
}

func (server *Server) biosInstalled(consoleID, name string) bool {
	for _, item := range server.selfItems(listing.Bios, consoleID) {
		if strings.EqualFold(item.Name, name) {
			return true
		}
	}

	return false
}

// handleGetConsoleDefaults — ce qui est enregistré, et ce qu'une partie lancée
// sans choix prendrait (`inUse`) : l'écran de la console met celui-là en avant,
// la fiche part de lui.
func (server *Server) handleGetConsoleDefaults(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusNotFound, "console_unknown", "unknown console")

		return
	}
	writeJSON(w, http.StatusOK, server.consoleDefaultsView(r, consoleID))
}

func (server *Server) consoleDefaultsView(r *http.Request, consoleID string) map[string]any {
	stored := server.consoleDefaultsOf(consoleID)
	inUse := consoleDefaults{}
	if cores := docker.StreamexCores(consoleID); len(cores) > 0 {
		inUse.Core = server.streamexCoreFor(consoleID, "")
		if inUse.Core == "" {
			inUse.Core = cores[0]
		}
		if need, ok := server.requiredEmulatorFor(r.Context(), consoleID, inUse.Core); ok {
			inUse.EmulatorFile, _, _ = server.emulatorInUse(need)
		}
	} else {
		inUse.Core = server.retroCoreFor(consoleID, "")
		if inUse.Core == "" {
			if declared := consoles.CoresFor(consoleID); len(declared) > 0 {
				inUse.Core = declared[0]
			}
		}
		inUse.Bios = server.retroBiosFor(consoleID, "")
	}

	return map[string]any{"consoleId": consoleID, "stored": stored, "inUse": inUse}
}

// handlePutConsoleDefaults — {core?, emulatorFile?, bios?} : un champ absent
// garde sa valeur, une chaîne vide l'efface. Chaque valeur doit désigner ce qui
// est installé ICI pour cette console : un nom venu de la requête ne choisit
// jamais un fichier pris ailleurs.
func (server *Server) handlePutConsoleDefaults(w http.ResponseWriter, r *http.Request) {
	consoleID := chi.URLParam(r, "consoleId")
	if !consoles.Exists(consoleID) {
		writeErrorCode(w, http.StatusNotFound, "console_unknown", "unknown console")

		return
	}
	var body struct {
		Core         *string `json:"core"`
		EmulatorFile *string `json:"emulatorFile"`
		Bios         *string `json:"bios"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid request body")

		return
	}

	consoleDefaultsMu.Lock()
	defer consoleDefaultsMu.Unlock()
	defaults := server.consoleDefaultsOf(consoleID)
	streamex := len(docker.StreamexCores(consoleID)) > 0

	if body.Core != nil {
		core := strings.TrimSpace(*body.Core)
		if err := server.checkDefaultCore(r, consoleID, core, streamex); err != nil {
			writeErrorCode(w, http.StatusUnprocessableEntity, "defaults_failed", err.Error())

			return
		}
		defaults.Core = core
		// Un autre émulateur par défaut : la version en service, qui était
		// celle d'un autre, ne vaut plus.
		if streamex && defaults.EmulatorFile != "" && !server.fileOfCore(r, consoleID, core, defaults.EmulatorFile) {
			defaults.EmulatorFile = ""
		}
	}
	if body.EmulatorFile != nil {
		file := strings.TrimSpace(*body.EmulatorFile)
		if file != "" {
			core, err := server.checkEmulatorFile(r, consoleID, file)
			if err != nil {
				writeErrorCode(w, http.StatusUnprocessableEntity, "defaults_failed", err.Error())

				return
			}
			// « Le in use devient celui par défaut » (le porteur).
			defaults.Core = core
		}
		defaults.EmulatorFile = file
	}
	if body.Bios != nil {
		bios := strings.TrimSpace(*body.Bios)
		if bios != "" && (streamex || !server.biosInstalled(consoleID, bios)) {
			writeErrorCode(w, http.StatusUnprocessableEntity, "bios_unknown", fmt.Sprintf("« %s » n'est pas un BIOS installé de cette console", bios))

			return
		}
		defaults.Bios = bios
	}

	if err := server.writeConsoleDefaults(consoleID, defaults); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "defaults_failed", err.Error())

		return
	}
	slog.Info("console defaults set", "console", consoleID, "core", defaults.Core, "emulatorFile", defaults.EmulatorFile,
		"bios", defaults.Bios, "by", auth.Username(r.Context()))
	writeJSON(w, http.StatusOK, server.consoleDefaultsView(r, consoleID))
}

func (server *Server) checkDefaultCore(r *http.Request, consoleID, core string, streamex bool) error {
	if core == "" {
		return nil
	}
	if streamex {
		if !slices.Contains(docker.StreamexCores(consoleID), core) {
			return fmt.Errorf("« %s » n'est pas un émulateur de cette console", core)
		}
		if err := server.requireEmulator(r.Context(), consoleID, core); err != nil {
			return err
		}

		return nil
	}

	return requireCore(server.retroCoresRoot(), consoleID, core)
}

// checkEmulatorFile — un fichier de cores/<console>/ que l'image d'un des
// émulateurs de la console déballe (son motif) ; rend cet émulateur.
func (server *Server) checkEmulatorFile(r *http.Request, consoleID, file string) (string, error) {
	if file != filepath.Base(file) || !retro_files.IsEmulatorFile(consoleID, file) {
		return "", fmt.Errorf("« %s » n'est pas un fichier d'émulateur", file)
	}
	if _, err := os.Stat(filepath.Join(server.retroCoresRoot(), consoleID, file)); err != nil {
		return "", fmt.Errorf("« %s » n'est pas dans cores/%s/", file, consoleID)
	}
	for _, need := range server.requiredEmulators(r.Context()) {
		if need.Console != consoleID {
			continue
		}
		if matched, _ := filepath.Match(need.Pattern, file); matched {
			return need.Core, nil
		}
	}

	return "", fmt.Errorf("aucune image de %s ne déballe « %s »", consoleID, file)
}

func (server *Server) fileOfCore(r *http.Request, consoleID, core, file string) bool {
	need, ok := server.requiredEmulatorFor(r.Context(), consoleID, core)
	if !ok {
		return false
	}
	matched, _ := filepath.Match(need.Pattern, file)

	return matched
}
