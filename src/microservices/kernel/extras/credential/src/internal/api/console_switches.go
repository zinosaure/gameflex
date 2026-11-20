package api

import (
	"net/http"
	"sort"
	"strings"

	"gameflex/consoles"
	"gameflex/credential/internal/auth"
)

// LES CONSOLES COUPÉES POUR TOUT LE SERVEUR (chantier 66, `#795`) — le porteur, 2026-11-19 : « dans settings/consoles un
// interrupteur pour couper la/es console.s directement. Ni les joueurs locaux ni les joueurs en fédération peuvent y
// accéder ». Un serveur de salon a parfois besoin de ses GPU : la PS5 se coupe d'un geste, pour tout le monde,
// administrateur compris, sans passer par chaque compte ni chaque accès.
//
//	GET /api/v1/console-switches         les consoles coupées (tout compte connecté : la page grise leur bouton Jouer)
//	PUT /api/v1/console-switches/{id}    {enabled} — l'administrateur coupe ou rallume une console
//
// Rangées ici, avec les droits des comptes et des accès : le kernel les reçoit par le même canal (accountsSnapshot) et
// refuse un lancement sur sa copie ; le téléphone les lit du même service, embarqué.

// disabledConsoles — les consoles coupées, dans un ordre stable.
func (server *Server) disabledConsoles() ([]string, error) {
	rows, err := server.db.Query(`SELECT console_id FROM disabled_consoles ORDER BY console_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		list = append(list, id)
	}

	return list, rows.Err()
}

func (server *Server) handleListConsoleSwitches(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	list, err := server.disabledConsoles()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	respondItems(w, http.StatusOK, list, newPagination(len(list), 1, allItems))
}

func (server *Server) handleSetConsoleSwitch(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		Enabled *bool `json:"enabled"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	id := strings.TrimSpace(r.PathValue("id"))
	if body.Enabled == nil || !consoles.Exists(id) {
		respondValidationError(w, []map[string]any{{
			"loc": []string{"path", "id"}, "msg": "A known console and enabled are required.", "type": "value_error",
		}})
		return
	}

	var err error
	if *body.Enabled {
		_, err = server.db.Exec(`DELETE FROM disabled_consoles WHERE console_id = ?`, id)
	} else {
		_, err = server.db.Exec(`INSERT INTO disabled_consoles (console_id, disabled_at, disabled_by_id) VALUES (?, ?, ?)
			ON CONFLICT (console_id) DO NOTHING`, id, now(), principal.ID)
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	// Les kernels décident des lancements sur leur copie : elle change tout de suite.
	server.accountsChanged()

	list, err := server.disabledConsoles()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	respondItems(w, http.StatusOK, list, newPagination(len(list), 1, allItems))
}

// accessEntry — un accès délivré, tel que le kernel le reçoit : ce qu'il laisse lancer.
type accessEntry struct {
	ID       int64    `json:"id"`
	Consoles []string `json:"consoles"`
	// RemoteAdmin — un administrateur venu par cet accès garde son rang (chantier 73).
	RemoteAdmin bool `json:"remote_admin,omitempty"`
}

// accessesSnapshot — les accès délivrés et leurs consoles, pour la copie des kernels.
func (server *Server) accessesSnapshot() ([]accessEntry, error) {
	rows, err := server.db.Query(`SELECT id, consoles, remote_admin FROM federation_tokens ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	list := []accessEntry{}
	for rows.Next() {
		var (
			entry   accessEntry
			granted string
		)
		if err := rows.Scan(&entry.ID, &granted, &entry.RemoteAdmin); err != nil {
			return nil, err
		}
		entry.Consoles = consoles.SplitGrants(granted)
		sort.Strings(entry.Consoles)
		list = append(list, entry)
	}

	return list, rows.Err()
}
