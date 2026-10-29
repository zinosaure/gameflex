package api

// LE JETON D'UN COMPTE — chantier 20 (le porteur, 2026-10-21).
//
// « Chaque utilisateur créé sur le webapp/mobileapp aura un token généré avec
// le compte, ce token est par défaut disabled. Mais quand on le enabled, on doit
// définir le temps que ça va expirer et sur le compte du user en question, il
// pourra copier le token pour réutiliser sur son serveur. »
//
// Un jeton par compte (account_tokens, créé par un déclencheur avec le compte).
// L'administrateur l'active pour une durée, le désactive, le régénère. Il sert
// à JOUER ICI DEPUIS UN AUTRE SERVEUR : son propriétaire le colle là-bas, à la
// place de l'identifiant et du mot de passe (« un formulaire de token […] on
// met le token valide et c'est bon »), et ce serveur-là l'échange contre une
// session à son nom (handleFederationSession). Ce qu'il ouvre ne vit pas ici
// mais dans le compte : ses consoles (`users.consoles`).
//
// Il n'est PAS le lien entre deux serveurs : celui-là reste un jeton de
// catalogue (federation_handlers.go), qui fait lister et télécharger.

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"time"

	"gameflex/consoles"
	"gameflex/credential/internal/auth"
)

// accountTokenResponse — l'état du jeton d'un compte. `token` n'y est que pour
// l'administrateur, et pour son propriétaire quand il est actif.
type accountTokenResponse struct {
	Token      string `json:"token,omitempty"`
	Enabled    bool   `json:"enabled"`
	ExpiresAt  any    `json:"expires_at"`
	CreatedAt  any    `json:"created_at"`
	UseCount   int    `json:"use_count"`
	LastUsedAt any    `json:"last_used_at"`
}

// tokenDurations — les durées qu'on peut donner à un jeton, « 2 ans maximum »
// (le porteur). Un mois est un mois du calendrier, pas trente jours.
var tokenDurations = map[string]func(time.Time) time.Time{
	"1d": func(from time.Time) time.Time { return from.AddDate(0, 0, 1) },
	"1w": func(from time.Time) time.Time { return from.AddDate(0, 0, 7) },
	"1m": func(from time.Time) time.Time { return from.AddDate(0, 1, 0) },
	"3m": func(from time.Time) time.Time { return from.AddDate(0, 3, 0) },
	"6m": func(from time.Time) time.Time { return from.AddDate(0, 6, 0) },
	"1y": func(from time.Time) time.Time { return from.AddDate(1, 0, 0) },
	"2y": func(from time.Time) time.Time { return from.AddDate(2, 0, 0) },
}

// storedTime — le format des dates de la base (celui de now()) : l'heure de la
// machine, sans fuseau — l'héritage du service Python. L'ÉCHÉANCE d'un jeton,
// elle, s'écrit en UTC au format RFC 3339 : comparée à l'instant, elle ne doit
// pas dépendre du fuseau de qui la relit (un téléphone à l'heure de Paris, un
// serveur en UTC). Le pilote SQLite relit d'ailleurs une colonne DATETIME en
// date, et la rend en RFC 3339 : ce format-là revient tel quel.
const storedTime = "2006-01-02 15:04:05.999999"

func parseStoredTime(raw sql.NullString) (time.Time, bool) {
	if !raw.Valid || raw.String == "" {
		return time.Time{}, false
	}
	if parsed, err := time.Parse(time.RFC3339Nano, raw.String); err == nil {
		return parsed, true
	}
	parsed, err := time.ParseInLocation(storedTime, raw.String, time.Local)
	if err != nil {
		return time.Time{}, false
	}

	return parsed, true
}

// effectiveGrants — ce qu'un compte peut lancer. Un administrateur a tout,
// quoi que dise sa ligne : « quand c'est un admin, toutes les consoles sont
// checked et on ne peut pas changer ».
func effectiveGrants(roleLevel int, stored string) []string {
	if roleLevel <= auth.RoleAdmin {
		return []string{consoles.GrantAll}
	}

	return consoles.SplitGrants(stored)
}

// accountToken — le jeton d'un compte, tel que la page le montre.
func (server *Server) accountToken(userID int64, withValue bool) (accountTokenResponse, error) {
	var (
		token                          string
		enabled, useCount              int
		expiresAt, createdAt, lastUsed sql.NullString
	)
	err := server.db.QueryRow(
		`SELECT token, enabled, expires_at, created_at, use_count, last_used_at FROM account_tokens WHERE user_id = ?`,
		userID,
	).Scan(&token, &enabled, &expiresAt, &createdAt, &useCount, &lastUsed)
	if err != nil {
		return accountTokenResponse{}, err
	}
	response := accountTokenResponse{
		Enabled:    enabled == 1,
		ExpiresAt:  isoDatetime(expiresAt),
		CreatedAt:  isoDatetime(createdAt),
		UseCount:   useCount,
		LastUsedAt: isoDatetime(lastUsed),
	}
	if withValue {
		response.Token = token
	}

	return response, nil
}

// handleUpdateAccountToken — PATCH /api/v1/users/{id}/token
// {enabled, duration}. Activer demande une durée (« quand on le enabled, on
// doit définir le temps que ça va expirer ») ; désactiver efface l'échéance.
func (server *Server) handleUpdateAccountToken(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}
	var body struct {
		Enabled  bool   `json:"enabled"`
		Duration string `json:"duration"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	var expiresAt any
	if body.Enabled {
		until, known := tokenDurations[body.Duration]
		if !known {
			respondError(w, http.StatusBadRequest, "Choose how long the token stays valid: 1d, 1w, 1m, 3m, 6m, 1y or 2y.")
			return
		}
		expiresAt = until(time.Now()).UTC().Format(time.RFC3339)
	}

	enabled := 0
	if body.Enabled {
		enabled = 1
	}
	result, err := server.db.Exec(`UPDATE account_tokens SET enabled = ?, expires_at = ? WHERE user_id = ?`, enabled, expiresAt, id)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}
	server.respondAccountToken(w, id)
}

// handleRegenerateAccountToken — POST /api/v1/users/{id}/token/regenerate.
// L'ancien jeton cesse de valoir à l'instant, et les sessions qu'il avait
// ouvertes avec lui (leur génération ne correspond plus) ; l'état — actif,
// échéance — ne change pas, les compteurs repartent de zéro.
func (server *Server) handleRegenerateAccountToken(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}
	result, err := server.db.Exec(
		`UPDATE account_tokens SET token = lower(hex(randomblob(32))), created_at = ?, use_count = 0, last_used_at = NULL WHERE user_id = ?`,
		now(), id,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}
	server.respondAccountToken(w, id)
}

// respondAccountToken — après un changement du jeton : la liste des comptes à
// jour part au kernel, puis le jeton revient à l'administrateur.
func (server *Server) respondAccountToken(w http.ResponseWriter, userID int64) {
	server.accountsChanged()
	token, err := server.accountToken(userID, true)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	respondItem(w, http.StatusOK, token)
}

// handleMyAccountToken — GET /api/v1/auth/me/token : ce que Réglages › Profil
// montre à son propriétaire. Le jeton lui-même seulement quand il est actif
// (« il pourra copier le token et lien si ce dernier est enabled »), et ses
// consoles. Pas dans /auth/me : celle-ci s'ouvre aux autres origines.
func (server *Server) handleMyAccountToken(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if refuseFederated(w, principal) {
		return
	}
	token, err := server.accountToken(principal.ID, false)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	if token.Enabled {
		token, err = server.accountToken(principal.ID, true)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
	}
	var stored string
	if err := server.db.QueryRow(`SELECT consoles FROM users WHERE id = ?`, principal.ID).Scan(&stored); err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	respondItem(w, http.StatusOK, map[string]any{
		"token":    token,
		"consoles": effectiveGrants(principal.RoleLevel, stored),
	})
}

// refuseFederated — une session ouverte par le jeton du compte, depuis le
// serveur de son propriétaire, JOUE, et rien d'autre : elle ne relit pas le
// jeton et ne change pas le compte (mot de passe, nom). Rend vrai si elle a
// refusé.
func refuseFederated(w http.ResponseWriter, principal auth.Principal) bool {
	if !principal.Federated {
		return false
	}
	respondError(w, http.StatusForbidden, "A session opened with an account token cannot manage the account.")

	return true
}

// TokenAccount — le compte derrière un jeton, s'il vaut encore : actif, pas
// échu, et le compte existe. Sinon ErrTokenRefused. Exporté pour le téléphone,
// dont le kernel échange ces jetons sans passer par HTTP (embed).
type TokenAccount struct {
	UserID      int64
	Username    string
	DisplayName string
	RoleLevel   int
	Grants      []string
	Generation  string
	ExpiresAt   time.Time
}

var ErrTokenRefused = errors.New("token refused")

func (server *Server) AccountByToken(token string) (TokenAccount, error) {
	if len(token) < 16 {
		return TokenAccount{}, ErrTokenRefused
	}
	account, _, err := server.validAccount(`t.token = ?`, token)

	return account, err
}

// validAccount — le compte d'UN jeton qui vaut encore, choisi par `where`
// (le jeton lui-même, ou son compte pour une invitation), et sa valeur.
func (server *Server) validAccount(where string, arg any) (TokenAccount, string, error) {
	var (
		account   TokenAccount
		token     string
		enabled   int
		expiresAt sql.NullString
		stored    string
	)
	err := server.db.QueryRow(
		`SELECT u.id, u.username, u.display_name, u.role_level, u.consoles, t.token, t.enabled, t.expires_at
		 FROM account_tokens t JOIN users u ON u.id = t.user_id WHERE `+where,
		arg,
	).Scan(&account.UserID, &account.Username, &account.DisplayName, &account.RoleLevel, &stored, &token, &enabled, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TokenAccount{}, "", ErrTokenRefused
	}
	if err != nil {
		return TokenAccount{}, "", err
	}
	until, dated := parseStoredTime(expiresAt)
	if enabled != 1 || !dated || !time.Now().Before(until) {
		return TokenAccount{}, "", ErrTokenRefused
	}
	account.Grants = effectiveGrants(account.RoleLevel, stored)
	account.Generation = token[:8]
	account.ExpiresAt = until

	return account, token, nil
}

// handleFederationSession — POST /internal/federation/session {token}.
//
// Le propriétaire du jeton veut jouer ici depuis son serveur : une session au
// nom du compte, jamais administrateur, qui ne dure pas plus que le jeton et
// porte sa génération (le kernel la coupe quand le jeton change). C'est « une
// connexion » : le compteur avance, et la dernière utilisation avec lui.
func (server *Server) handleFederationSession(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	var body struct {
		Token string `json:"token"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	session, account, err := server.FederatedSession(body.Token)
	if errors.Is(err, ErrTokenRefused) {
		respondError(w, http.StatusUnauthorized, "Invalid token.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	respondItem(w, http.StatusOK, map[string]any{
		"access_token": session,
		"user": map[string]any{
			"id":           account.UserID,
			"username":     account.Username,
			"display_name": account.DisplayName,
			"consoles":     account.Grants,
		},
	})
}

// FederatedSession — une session au nom du compte de ce jeton, jamais
// administrateur, qui ne dure pas plus que le jeton et porte sa génération ;
// le compteur avance (« une connexion »). ErrTokenRefused si le jeton ne vaut
// plus. La route interne du serveur et le téléphone (embed) passent par ici.
func (server *Server) FederatedSession(token string) (string, TokenAccount, error) {
	account, err := server.AccountByToken(token)
	if err != nil {
		return "", TokenAccount{}, err
	}
	session, err := auth.CreateFederatedToken(server.config.JWTSecret, account.UserID, account.Username,
		account.Generation, time.Until(account.ExpiresAt))
	if err != nil {
		return "", TokenAccount{}, err
	}
	_, _ = server.db.Exec(`UPDATE account_tokens SET use_count = use_count + 1, last_used_at = ? WHERE user_id = ?`,
		now(), account.UserID)

	return session, account, nil
}
