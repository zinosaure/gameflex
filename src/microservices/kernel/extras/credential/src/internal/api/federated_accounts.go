package api

// MON COMPTE CHEZ UN SERVEUR FÉDÉRÉ — chantier 20, le porteur (2026-10-21) :
// « Au lieu de voir l'ancien formulaire login, c'est un formulaire de token. On
// met le token valide et c'est bon. »
//
// Un membre d'ici qui a un compte chez un serveur fédéré y recopie le jeton de
// ce compte (Réglages › Profil, là-bas) et le colle ici, UNE fois. Il est gardé
// avec son compte d'ici (« avec mon compte », choisi par le porteur), donc sur
// tous ses appareils. Le kernel le vérifie chez l'ami avant de le confier, puis
// l'échange contre une session à son nom quand il joue là-bas.
//
// Tout passe par le plan interne : ce jeton ouvre un compte chez quelqu'un
// d'autre, il ne ressort jamais vers un navigateur.

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
)

// ErrNoFederatedAccount — ce membre n'a pas de jeton pour ce serveur (ou l'un
// des deux n'existe pas).
var ErrNoFederatedAccount = errors.New("no federated account")

// FederatedAccountToken — le jeton que ce membre a collé pour ce serveur.
func (server *Server) FederatedAccountToken(username string, serverID int64) (string, error) {
	var token string
	err := server.db.QueryRow(
		`SELECT a.token FROM federated_accounts a JOIN users u ON u.id = a.user_id
		 WHERE u.username = ? AND a.server_id = ?`,
		username, serverID,
	).Scan(&token)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNoFederatedAccount
	}

	return token, err
}

// SetFederatedAccountToken — garder le jeton que ce membre colle pour ce
// serveur ; il remplace le précédent.
func (server *Server) SetFederatedAccountToken(username string, serverID int64, token string) error {
	result, err := server.db.Exec(
		`INSERT INTO federated_accounts (user_id, server_id, token, created_at)
		 SELECT u.id, s.id, ?, ? FROM users u, federation_servers s WHERE u.username = ? AND s.id = ?
		 ON CONFLICT (user_id, server_id) DO UPDATE SET token = excluded.token, created_at = excluded.created_at`,
		token, now(), username, serverID,
	)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNoFederatedAccount
	}

	return nil
}

// ForgetFederatedAccountToken — oublier le jeton de ce membre pour ce serveur.
func (server *Server) ForgetFederatedAccountToken(username string, serverID int64) error {
	result, err := server.db.Exec(
		`DELETE FROM federated_accounts WHERE server_id = ? AND user_id = (SELECT id FROM users WHERE username = ?)`,
		serverID, username,
	)
	if err != nil {
		return err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		return ErrNoFederatedAccount
	}

	return nil
}

// federatedAccountOf — le membre et le serveur nommés par la requête :
// `?username=…&server=…`. Répond lui-même en cas d'échec.
func federatedAccountOf(w http.ResponseWriter, username, rawServer string) (string, int64, bool) {
	username = strings.TrimSpace(username)
	serverID, err := strconv.ParseInt(strings.TrimSpace(rawServer), 10, 64)
	if username == "" || err != nil {
		respondError(w, http.StatusBadRequest, "A member and a server are required.")

		return "", 0, false
	}

	return username, serverID, true
}

// handleInternalFederatedAccount — GET /internal/federation/accounts
// ?username=…&server=… : le jeton de ce membre pour ce serveur, ou 404.
func (server *Server) handleInternalFederatedAccount(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	username, serverID, ok := federatedAccountOf(w, r.URL.Query().Get("username"), r.URL.Query().Get("server"))
	if !ok {
		return
	}
	token, err := server.FederatedAccountToken(username, serverID)
	if errors.Is(err, ErrNoFederatedAccount) {
		respondError(w, http.StatusNotFound, "No token for this server.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	respondItem(w, http.StatusOK, map[string]string{"token": token})
}

// handleInternalSetFederatedAccount — PUT /internal/federation/accounts
// {username, server, token} : le kernel l'a déjà vérifié chez l'ami.
func (server *Server) handleInternalSetFederatedAccount(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	var body struct {
		Username string `json:"username"`
		Server   int64  `json:"server"`
		Token    string `json:"token"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	username, serverID, ok := federatedAccountOf(w, body.Username, strconv.FormatInt(body.Server, 10))
	if !ok {
		return
	}
	token := strings.TrimSpace(body.Token)
	if token == "" {
		respondError(w, http.StatusBadRequest, "A token is required.")
		return
	}
	err := server.SetFederatedAccountToken(username, serverID, token)
	if errors.Is(err, ErrNoFederatedAccount) {
		respondError(w, http.StatusNotFound, "Unknown member or server.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleInternalForgetFederatedAccount — DELETE /internal/federation/accounts
// ?username=…&server=….
func (server *Server) handleInternalForgetFederatedAccount(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	username, serverID, ok := federatedAccountOf(w, r.URL.Query().Get("username"), r.URL.Query().Get("server"))
	if !ok {
		return
	}
	err := server.ForgetFederatedAccountToken(username, serverID)
	if errors.Is(err, ErrNoFederatedAccount) {
		respondError(w, http.StatusNotFound, "No token for this server.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
