package api

// LES SERVEURS FÉDÉRÉS DONT UN COMPTE NE VEUT PAS VOIR LES JEUX (le porteur, 2026-11-16 : « Je demandais à ce qu'on voit
// la liste des serveurs fédérés en format switch pour ensuite choisir quel serveur on veut voir ou pas les jeux. […]
// Ça fait que chaque utilisateur choisit quel serveur fédéré il a envie de voir les jeux. ») — Réglages › Application,
// un interrupteur par serveur : éteint, ses jeux quittent les bibliothèques de ce compte et leurs nombres ; les Réglages
// des consoles, eux, montrent toujours toutes les sources. Le choix vit ICI, avec le compte, comme les favoris
// (favorites.go) : il suit le joueur d'un appareil à l'autre, et le téléphone le sert par le MÊME code, embarqué, sous
// le même chemin (le domaine `auth`).
//
// ALLUMÉ PAR DÉFAUT : la table ne garde que les serveurs ÉTEINTS. Un serveur ajouté plus tard se montre tant que le
// compte ne l'a pas éteint ; retiré du registre, il emporte ses lignes (la clé étrangère, comme les favoris).
//
//	GET    /api/v1/auth/me/hidden-servers        les serveurs éteints du compte connecté : [{server_id}]
//	PUT    /api/v1/auth/me/hidden-servers/{id}   en éteindre un (déjà éteint : 204 aussi) ; 404 s'il n'est pas au registre
//	DELETE /api/v1/auth/me/hidden-servers/{id}   le rallumer (déjà allumé : 204 aussi)

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"

	"gameflex/credential/internal/auth"
)

type hiddenServerResponse struct {
	ServerID int64 `json:"server_id"`
}

// handleListHiddenServers — GET /api/v1/auth/me/hidden-servers.
func (server *Server) handleListHiddenServers(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	// Les serveurs relayés sous leur identifiant de la page, -(relais × 10⁶ + id).
	rows, err := server.db.Query(`SELECT server_id FROM hidden_servers WHERE user_id = ?
		UNION ALL SELECT -(via_id * 1000000 + relayed_id) FROM hidden_relayed_servers WHERE user_id = ?
		ORDER BY 1`, principal.ID, principal.ID)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	defer rows.Close()

	items := []hiddenServerResponse{}
	for rows.Next() {
		var item hiddenServerResponse
		if err := rows.Scan(&item.ServerID); err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		items = append(items, item)
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

// hiddenServerID — l'identifiant du chemin, un entier non nul (négatif : un serveur relayé) ; sinon 400, et false.
func hiddenServerID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	// UN SERVEUR RELAYÉ (chantier 83) a un identifiant négatif, -(relais × 10⁶ + son id chez le relais) : il se cache
	// comme les autres (le porteur, 2026-11-26 : « pour qu'on puisse les afficher si on veut ou pas »).
	if err != nil || id == 0 {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "Invalid server id.")
		return 0, false
	}

	return id, true
}

// handleHideServer — PUT /api/v1/auth/me/hidden-servers/{id}. Le serveur doit être au registre : ce que la page
// vérifie se revérifie ici (CODE_CONVENTIONS.md).
func (server *Server) handleHideServer(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, ok := hiddenServerID(w, r)
	if !ok {
		return
	}
	// Au registre : lui, ou — relayé — le serveur qui le relaie.
	registered, relayed := splitRelayed(id)
	var known int64
	if err := server.db.QueryRow(`SELECT id FROM federation_servers WHERE id = ?`, registered).Scan(&known); errors.Is(err, sql.ErrNoRows) {
		respondErrorCode(w, http.StatusNotFound, "not_found", "Server not found.")
		return
	} else if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	query, args := `INSERT INTO hidden_servers (user_id, server_id, created_at) VALUES (?, ?, ?) ON CONFLICT (user_id, server_id) DO NOTHING`,
		[]any{principal.ID, id, now()}
	if relayed > 0 {
		query, args = `INSERT INTO hidden_relayed_servers (user_id, via_id, relayed_id, created_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id, via_id, relayed_id) DO NOTHING`, []any{principal.ID, registered, relayed, now()}
	}
	if _, err := server.db.Exec(query, args...); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleShowServer — DELETE /api/v1/auth/me/hidden-servers/{id}. Un serveur déjà montré n'est pas une erreur : le geste
// voulait qu'il le soit.
func (server *Server) handleShowServer(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, ok := hiddenServerID(w, r)
	if !ok {
		return
	}
	query, args := `DELETE FROM hidden_servers WHERE user_id = ? AND server_id = ?`, []any{principal.ID, id}
	if via, relayed := splitRelayed(id); relayed > 0 {
		query, args = `DELETE FROM hidden_relayed_servers WHERE user_id = ? AND via_id = ? AND relayed_id = ?`, []any{principal.ID, via, relayed}
	}
	if _, err := server.db.Exec(query, args...); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// splitRelayed — l'identifiant d'un serveur relayé, -(relais × 10⁶ + id), en ses deux parts (remote.RelayID) ; un
// serveur du registre rend (lui-même, 0).
func splitRelayed(id int64) (registered, relayed int64) {
	if id > 0 {
		return id, 0
	}

	return -id / 1_000_000, -id % 1_000_000
}
