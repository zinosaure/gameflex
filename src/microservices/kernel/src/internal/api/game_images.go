package api

// LES IMAGES D'UN JEU (chantier 49, #659, premier morceau) —
// GET /api/v1/retrogaming/{consoleId}/roms/{ref}/images/{kind}?w=<largeur>.
//
// `kind` : icon (carré), background (16:9), cover (jaquette), screenshot,
// title (écran-titre), logo (transparent). Les sources, dans
// gameflex/kernel/internal/metadata : ce que le jeu porte (`sce_sys/` des PS4
// et PS5, `PS3_GAME/` d'un ISO PS3), puis libretro-thumbnails. 404 quand
// aucune ne l'a, et l'interface dessine alors son affiche générée. Un jeu d'un
// disque interne est trouvé comme pour un téléchargement (`?disk=` le
// désigne) ; ses images s'écrivent sur ce disque, avec lui
// (`<disque>/metadata/`, décision du porteur du 2026-11-13). Un jeu fédéré
// (`srv<id>:<ref>`, `?server=<id>`) a les siennes chez son serveur, relayées
// telles quelles (chantier 57, federation_metadata.go).
//
// Route gardée : la page la charge par `fetch` avec son jeton.

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"gameflex/metadata"
)

func (server *Server) handleGameImage(w http.ResponseWriter, r *http.Request) {
	// Un jeu fédéré : ses images se lisent chez son serveur (federation_metadata.go).
	if server.relayFederatedImage(w, r) {
		return
	}
	consoleID, reference, gamePath, roots, ok := server.metadataGame(w, r)
	if !ok {
		return
	}

	server.serveGameImage(w, r, roots, consoleID, gamePath, reference)
}

// serveGameImage — l'image `{kind}` de l'adresse, à la largeur `?w=`, d'un jeu trouvé : d'ici (handleGameImage), ou
// d'ici pour un pair (handleFederationGameImage).
func (server *Server) serveGameImage(w http.ResponseWriter, r *http.Request, roots metadata.Roots, consoleID, gamePath, reference string) {
	kind := chi.URLParam(r, "kind")
	width, _ := strconv.Atoi(r.URL.Query().Get("w"))

	path, err := metadata.Image(r.Context(), roots, server.metadataAccounts(), consoleID, gamePath, reference, kind, width)
	switch {
	case errors.Is(err, metadata.ErrUnknownKind):
		writeErrorCode(w, http.StatusBadRequest, "bad_request", "unknown image kind")
		return
	case errors.Is(err, os.ErrNotExist):
		writeErrorCode(w, http.StatusNotFound, "no_image", "no image")
		return
	case err != nil:
		slog.Warn("game image", "console", consoleID, "kind", kind, "error", err)
		writeErrorCode(w, http.StatusInternalServerError, "images_failed", err.Error())
		return
	}

	serveChangingImage(w, r, path)
}

// LES COMPTES DES SERVICES DE MÉTADONNÉES — saisis par l'administrateur,
// scellés dans le service d'identité, lus par sa route interne et gardés une
// minute ici : une grille de jaquettes ne le redemande pas à chaque image.
// Sans service d'identité, ou s'il ne répond pas : aucun compte, et les
// sources sans compte suffisent.
var metadataAccountsCache struct {
	sync.Mutex
	accounts  metadata.Accounts
	fetchedAt time.Time
}

func (server *Server) metadataAccounts() metadata.Accounts {
	metadataAccountsCache.Lock()
	defer metadataAccountsCache.Unlock()
	if !metadataAccountsCache.fetchedAt.IsZero() && time.Since(metadataAccountsCache.fetchedAt) < time.Minute {
		return metadataAccountsCache.accounts
	}
	if server.config.CredentialURL == "" || server.config.InternalSecret == "" {
		return nil
	}
	request, err := http.NewRequest(http.MethodGet, server.config.CredentialURL+"/internal/metadata/services", nil)
	if err != nil {
		return nil
	}
	request.Header.Set("X-Internal-Secret", server.config.InternalSecret)
	response, err := (&http.Client{Timeout: 5 * time.Second}).Do(request)
	if err != nil {
		slog.Warn("metadata accounts: identity service unreachable", "error", err)
		return metadataAccountsCache.accounts
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		slog.Warn("metadata accounts: cannot read them", "status", response.Status)
		return metadataAccountsCache.accounts
	}
	var envelope struct {
		Item metadata.Accounts `json:"item"`
	}
	if err := json.NewDecoder(response.Body).Decode(&envelope); err != nil {
		return metadataAccountsCache.accounts
	}
	metadataAccountsCache.accounts, metadataAccountsCache.fetchedAt = envelope.Item, time.Now()

	return envelope.Item
}
