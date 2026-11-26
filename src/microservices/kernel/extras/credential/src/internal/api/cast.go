package api

// GOOGLE CAST, DANS L'ADMINISTRATION (le porteur, 2026-11-24 : « D'ailleurs ça devrait être dans l'administration
// directement et non plus dans le launch.sh »). L'identifiant de l'application réceptrice que l'administrateur a
// enregistrée chez Google (la console des développeurs de Google Cast, l'adresse de son récepteur :
// `<adresse du serveur>/cast-receiver.html`). Il était lu à la construction de la page (GAMEFLEX_GOOGLECAST_APPLICATION_ID,
// launch.sh → compose → webpack) ; il se garde maintenant ici, dans la table `settings`, à côté des comptes des
// services de métadonnées — le même service, et la même table, sur un serveur, un téléphone et un PC.
//
//	GET /api/v1/cast   {application_id} — pour tout compte : la page d'une partie le lit avant de proposer le cast
//	PUT /api/v1/cast   {application_id} — un administrateur ; vide, il est retiré
//
// Sans identifiant, la page ne propose pas le cast : rien ne se cache derrière un bouton qui échouerait.

import (
	"database/sql"
	"net/http"
	"regexp"
	"strings"

	"gameflex/credential/internal/auth"
)

const castApplicationKey = "cast_application_id"

// Un identifiant d'application Cast : huit chiffres hexadécimaux (« CC1AD845 »).
var castApplicationID = regexp.MustCompile(`^[0-9A-F]{8}$`)

type castSettings struct {
	ApplicationID string `json:"application_id"`
}

func (server *Server) readCastSettings() (castSettings, error) {
	var settings castSettings
	err := server.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, castApplicationKey).Scan(&settings.ApplicationID)
	if err == sql.ErrNoRows {
		return castSettings{}, nil
	}

	return settings, err
}

// handleGetCast — GET /api/v1/cast.
func (server *Server) handleGetCast(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	settings, err := server.readCastSettings()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	respondItem(w, http.StatusOK, settings)
}

// handlePutCast — PUT /api/v1/cast {application_id}. Le champ est exigé : un corps vide ne retire pas l'identifiant
// en silence ; une chaîne vide, si.
func (server *Server) handlePutCast(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		ApplicationID *string `json:"application_id"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if body.ApplicationID == nil {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "application_id is required.")
		return
	}
	id := strings.ToUpper(strings.TrimSpace(*body.ApplicationID))
	var err error
	if id == "" {
		_, err = server.db.Exec(`DELETE FROM settings WHERE key = ?`, castApplicationKey)
	} else if !castApplicationID.MatchString(id) {
		respondErrorCode(w, http.StatusBadRequest, "cast_application_invalid", "A Cast application ID is eight hexadecimal characters.")
		return
	} else {
		_, err = server.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, castApplicationKey, id)
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	server.handleGetCast(w, r, principal)
}
