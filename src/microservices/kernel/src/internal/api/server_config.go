package api

// LA CONFIGURATION DU SERVEUR (chantier 81 ; le porteur, 2026-11-25 : « On peut
// mettre les varenv dans une section "Serveur config" dans le settings pour les
// administrateurs ? ») — son nom, les manettes d'une partie, les parties en même
// temps par console, le miroir, le GPU des parties (config.Live). Ils ont quitté
// le compose : ce qui peut changer en marche se règle dans Réglages ›
// Configuration du serveur, le compose garde ce qui ne le peut pas.
//
// GARDÉE PAR LE KERNEL, comme le réglage des journaux (log_settings.go) : c'est
// lui qui s'en sert, dès son démarrage (les limites par console), avant que le
// service des comptes réponde. Dans `databases/server.json`, à côté des
// ressources. Lue au démarrage (cmd/sessions/main.go) ; changée ici, elle vaut à
// la réponse, à la partie suivante — chaque lecteur relit config.CurrentLive.
//
//	GET /api/v1/retrogaming/storage/server-config   les réglages, et ce que fixe le compose
//	PUT /api/v1/retrogaming/storage/server-config   {serverName, players, consoleLimits, gpu} — administration

import (
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gameflex/kernel/internal/auth"
	"gameflex/kernel/internal/config"
	"gameflex/storage"
)

var serverConfigMu sync.Mutex

func serverConfigPath(root string) string {
	return filepath.Join(root, "databases", "server.json")
}

// LoadServerConfig — les réglages enregistrés, sinon les défauts ; un fichier
// illisible donne les défauts aussi (le journal le dit). Une clé absente garde
// sa valeur par défaut.
func LoadServerConfig(root string) config.Live {
	raw, err := os.ReadFile(serverConfigPath(root))
	if errors.Is(err, fs.ErrNotExist) {
		return config.DefaultLive()
	}
	// Les limites lues telles quelles : fusionnées dans celles par défaut, une console réglée « sans limite » (absente
	// du fichier) reprendrait la limite par défaut.
	live := config.DefaultLive()
	live.ConsoleLimits = nil
	if err == nil {
		err = json.Unmarshal(raw, &live)
	}
	if err == nil && live.ConsoleLimits == nil {
		live.ConsoleLimits = config.DefaultLive().ConsoleLimits
	}
	if err == nil {
		live, err = live.Normalized()
	}
	if err != nil {
		slog.Warn("server config unreadable: the defaults apply", "error", err)

		return config.DefaultLive()
	}

	return live
}

func writeServerConfig(root string, live config.Live) error {
	path := serverConfigPath(root)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.MarshalIndent(live, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".part"
	if err := os.WriteFile(temporary, encoded, 0o644); err != nil {
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		return err
	}
	storage.AdoptParentOwner(path)

	return nil
}

// serverConfigView — les réglages, et à côté ce que fixe le compose, en lecture
// seule et sans secret (la section Détails de la page).
type serverConfigView struct {
	config.Live
	Fixed struct {
		PublicHost    string `json:"publicHost"`
		EdgePort      string `json:"edgePort"`
		StorageDir    string `json:"storageDir"`
		Stack         string `json:"stack"`
		Staging       bool   `json:"staging"`
		InternalDisks int    `json:"internalDisks"`
	} `json:"fixed"`
}

func (server *Server) serverConfigView() serverConfigView {
	view := serverConfigView{Live: config.CurrentLive()}
	view.Fixed.PublicHost = server.config.PublicHost
	view.Fixed.EdgePort = server.config.EdgePort
	view.Fixed.StorageDir = server.config.StorageHostDir
	view.Fixed.Stack = server.config.StackName
	view.Fixed.Staging = server.config.Staging
	if entries, err := os.ReadDir(config.InternalDisksDir); err == nil {
		for _, entry := range entries {
			if entry.IsDir() {
				view.Fixed.InternalDisks++
			}
		}
	}

	return view
}

// handleGetServerConfig — GET /api/v1/retrogaming/storage/server-config.
func (server *Server) handleGetServerConfig(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, server.serverConfigView())
}

// handlePutServerConfig — PUT /api/v1/retrogaming/storage/server-config. La
// page envoie le formulaire entier : c'est un seul bouton.
func (server *Server) handlePutServerConfig(w http.ResponseWriter, r *http.Request) {
	var live config.Live
	if err := json.NewDecoder(r.Body).Decode(&live); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "invalid request body")

		return
	}
	live, err := live.Normalized()
	if err != nil {
		// Le champ fautif nomme le code (`server_config_maxPlayers`…), que la page traduit (kernelErrors).
		writeErrorCode(w, http.StatusUnprocessableEntity, "server_config_"+strings.SplitN(err.Error(), ":", 2)[0], err.Error())

		return
	}

	serverConfigMu.Lock()
	defer serverConfigMu.Unlock()
	if err := writeServerConfig(server.storageRoot, live); err != nil {
		writeErrorCode(w, http.StatusInternalServerError, "server_config_failed", err.Error())

		return
	}
	config.ApplyLive(live)
	slog.Info("server config set", "serverName", live.ServerName, "players", live.Players,
		"consoleLimits", live.ConsoleLimits, "gpu", live.GPU, "by", auth.Username(r.Context()))
	writeJSON(w, http.StatusOK, server.serverConfigView())
}
