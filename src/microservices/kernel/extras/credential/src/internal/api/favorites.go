package api

// LES FAVORIS D'UN COMPTE (le porteur, 2026-11-16 : « dans les fiches game info, remplace le bouton download this rom
// par add to my favorite » ; le Catalogue des Réglages les filtre et les pose). Ils vivent ICI, avec le compte, comme
// l'historique et les mappages : ils suivent le joueur d'un appareil à l'autre, et le téléphone les sert par le MÊME
// code, embarqué (embed), sous le même chemin.
//
// La notion était partie le 2026-10-29 (« on n'aura plus cette notion de favoris », journal 344 : ses routes, sa table,
// effacée à chaque démarrage) ; le porteur la redemande. Elle revient sous le compte connecté, comme « Mon jeton »
// (/auth/me/token) — le domaine `auth` est déjà celui de ce service, au serveur comme au téléphone (identityPrefixes) :
//
//	GET    /api/v1/auth/me/favorites                                              les miens, du plus ancien au plus récent
//	POST   /api/v1/auth/me/favorites {console_id, reference, server_id?, title?}  en poser un (le reposer rend le même)
//	DELETE /api/v1/auth/me/favorites?console_id=&reference=                       en retirer un (absent : 204 aussi)
//
// UN FAVORI DÉSIGNE UN JEU PAR SA CONSOLE ET SA RÉFÉRENCE, celle que la liste des jeux donne : l'UUID d'un jeu d'ici (le
// même sur la bibliothèque et sur un disque, storage.RomID), `srv<id>:<référence>` pour un jeu d'un serveur fédéré
// (remoteReference, au kernel). UN JEU FÉDÉRÉ GARDE AUSSI SON SERVEUR (`server_id`) : il doit dire le même que le
// préfixe de la référence, et désigner un serveur du registre ; retiré du registre, le serveur emporte ses favoris (la
// clé étrangère, comme federated_accounts). Ce que la page vérifie se revérifie ici (CODE_CONVENTIONS.md).

import (
	"database/sql"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"gameflex/consoles"
	"gameflex/credential/internal/auth"
)

// favoriteReferenceMax — la longueur de la colonne `reference` (VARCHAR(255)) ; `title` a la même.
const favoriteReferenceMax = 255

// federatedReference — `srv<id>:<référence chez lui>`, la référence d'un jeu d'un serveur fédéré (remoteReference, au
// kernel ; le même préfixe au téléphone).
var federatedReference = regexp.MustCompile(`^srv([0-9]+):(.+)$`)

type favoriteResponse struct {
	ID        int64  `json:"id"`
	ConsoleID string `json:"console_id"`
	Reference string `json:"reference"`
	ServerID  *int64 `json:"server_id"`
	Title     any    `json:"title"`
	CreatedAt any    `json:"created_at"`
}

const favoriteColumns = `id, console_id, reference, server_id, title, created_at`

func scanFavorite(row interface{ Scan(...any) error }) (favoriteResponse, error) {
	var favorite favoriteResponse
	var serverID sql.NullInt64
	var title, createdAt sql.NullString
	if err := row.Scan(&favorite.ID, &favorite.ConsoleID, &favorite.Reference, &serverID, &title, &createdAt); err != nil {
		return favorite, err
	}
	if serverID.Valid {
		favorite.ServerID = &serverID.Int64
	}
	favorite.Title, favorite.CreatedAt = text(title), isoDatetime(createdAt)

	return favorite, nil
}

// handleListFavorites — GET /api/v1/auth/me/favorites.
func (server *Server) handleListFavorites(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	rows, err := server.db.Query(`SELECT `+favoriteColumns+` FROM favorites WHERE user_id = ? ORDER BY id`, principal.ID)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	defer rows.Close()

	items := []favoriteResponse{}
	for rows.Next() {
		favorite, err := scanFavorite(rows)
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		items = append(items, favorite)
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

// handleAddFavorite — POST /api/v1/auth/me/favorites. Reposer un favori déjà là rend le même, sans en écrire un second
// (la clé unique : le compte, la console, la référence).
func (server *Server) handleAddFavorite(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		ConsoleID string  `json:"console_id"`
		Reference string  `json:"reference"`
		ServerID  *int64  `json:"server_id"`
		Title     *string `json:"title"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	body.ConsoleID, body.Reference = strings.TrimSpace(body.ConsoleID), strings.TrimSpace(body.Reference)
	if !consoles.Exists(body.ConsoleID) {
		respondErrorCode(w, http.StatusBadRequest, "console_unknown", "Unknown console.")
		return
	}
	if body.Reference == "" || len(body.Reference) > favoriteReferenceMax {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "A game reference is required, 255 characters at most.")
		return
	}
	var title any
	if body.Title != nil {
		trimmed := strings.TrimSpace(*body.Title)
		if utf8.RuneCountInString(trimmed) > favoriteReferenceMax {
			respondErrorCode(w, http.StatusBadRequest, "bad_request", "The title is 255 characters at most.")
			return
		}
		if trimmed != "" {
			title = trimmed
		}
	}

	// LE SERVEUR D'UN JEU FÉDÉRÉ : celui que sa référence nomme, et seulement lui ; un jeu d'ici n'en a pas.
	match := federatedReference.FindStringSubmatch(body.Reference)
	switch {
	case match == nil && body.ServerID != nil:
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "A game of this server has no server_id.")
		return
	case match != nil && (body.ServerID == nil || strconv.FormatInt(*body.ServerID, 10) != match[1]):
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "A federated game is srv<id>:<reference>, with the same server_id.")
		return
	case match != nil:
		var known int64
		if err := server.db.QueryRow(`SELECT id FROM federation_servers WHERE id = ?`, *body.ServerID).Scan(&known); errors.Is(err, sql.ErrNoRows) {
			respondErrorCode(w, http.StatusNotFound, "not_found", "Server not found.")
			return
		} else if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
	}

	if _, err := server.db.Exec(
		`INSERT INTO favorites (user_id, console_id, reference, server_id, title, created_at) VALUES (?, ?, ?, ?, ?, ?)
		 ON CONFLICT (user_id, console_id, reference) DO NOTHING`,
		principal.ID, body.ConsoleID, body.Reference, body.ServerID, title, now(),
	); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	favorite, err := scanFavorite(server.db.QueryRow(`SELECT `+favoriteColumns+` FROM favorites WHERE user_id = ? AND console_id = ? AND reference = ?`,
		principal.ID, body.ConsoleID, body.Reference))
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	respondItem(w, http.StatusCreated, favorite)
}

// handleRemoveFavorite — DELETE /api/v1/auth/me/favorites?console_id=&reference=. Un favori absent n'est pas une
// erreur : le geste voulait qu'il n'y soit plus.
func (server *Server) handleRemoveFavorite(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	consoleID, reference := r.URL.Query().Get("console_id"), r.URL.Query().Get("reference")
	if consoleID == "" || reference == "" {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "Missing console_id or reference.")
		return
	}
	if _, err := server.db.Exec(`DELETE FROM favorites WHERE user_id = ? AND console_id = ? AND reference = ?`, principal.ID, consoleID, reference); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
