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
	"strings"
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
	// Connections — qui s'en sert (#781) : les serveurs et appareils qui l'ont échangé, le plus récent d'abord.
	Connections []tokenConnection `json:"connections"`
}

// tokenConnection — un serveur ou un appareil qui a ouvert une session avec ce jeton : le nom qu'il se donne, l'adresse
// d'où il est venu, la première et la dernière fois, combien de fois.
type tokenConnection struct {
	Name string `json:"name"`
	// Member, Kind — le compte qui a collé le jeton chez l'appelant, et ce qu'est l'appelant (`server`, `phone`) :
	// déclarés par lui (#787), pour que le propriétaire reconnaisse les siens.
	Member  string `json:"member"`
	Kind    string `json:"kind"`
	Address string `json:"address"`
	// Here — l'adresse est celle d'où le propriétaire lit la page : ce que l'appelant ne peut pas inventer.
	Here        bool `json:"here"`
	FirstUsedAt any  `json:"first_used_at"`
	LastUsedAt  any  `json:"last_used_at"`
	UseCount    int  `json:"use_count"`
}

// TokenCaller — qui présente un jeton de compte : le nom qu'il déclare (`X-GameFlex-From`, vide pour un appelant qui
// ne le dit pas) et l'adresse que le proxy a vue. Pour le montrer à son propriétaire, jamais pour autoriser.
type TokenCaller struct {
	Name    string
	Member  string
	Kind    string
	Address string
	// Access — le jeton de catalogue que ce serveur tient de nous (chantier 66) : par quel accès il vient. Sans lui,
	// pas de session — les consoles d'un accès ne se contournent pas en le taisant.
	Access string
}

// trimmedTo — une valeur venue d'ailleurs, sans espaces autour, coupée à `limit` caractères.
func trimmedTo(value string, limit int) string {
	value = strings.TrimSpace(value)
	if runes := []rune(value); len(runes) > limit {
		value = string(runes[:limit])
	}

	return value
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
	response.Connections, err = server.tokenConnections(userID)

	return response, err
}

// tokenConnections — les connexions faites avec le jeton d'un compte, la plus récente d'abord.
func (server *Server) tokenConnections(userID int64) ([]tokenConnection, error) {
	rows, err := server.db.Query(`SELECT name, member, kind, address, first_used_at, last_used_at, use_count FROM account_token_connections
		WHERE user_id = ? ORDER BY last_used_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list := []tokenConnection{}
	for rows.Next() {
		var (
			row         tokenConnection
			first, last sql.NullString
		)
		if err := rows.Scan(&row.Name, &row.Member, &row.Kind, &row.Address, &first, &last, &row.UseCount); err != nil {
			return nil, err
		}
		row.FirstUsedAt, row.LastUsedAt = isoDatetime(first), isoDatetime(last)
		list = append(list, row)
	}

	return list, rows.Err()
}

// handleUpdateMyAccountToken — PATCH /api/v1/auth/me/token {enabled, duration}. Activer demande une durée (« quand on
// le enabled, on doit définir le temps que ça va expirer ») ; désactiver efface l'échéance.
//
// LE MEMBRE ET LUI SEUL (le porteur, 2026-11-20 : « Le membre l'active lui-même ») : la page d'un membre, où un
// administrateur activait son jeton, est partie avec ses routes (`/users/{id}/token`). Jamais depuis une session ouverte
// par le jeton (refuseFederated) : un jeton ne se prolonge pas lui-même.
func (server *Server) handleUpdateMyAccountToken(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if refuseFederated(w, principal) {
		return
	}
	id := principal.ID
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
			respondErrorCode(w, http.StatusBadRequest, "bad_request", "Choose how long the token stays valid: 1d, 1w, 1m, 3m, 6m, 1y or 2y.")
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
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	server.respondAccountToken(w, id)
}

// handleRegenerateMyAccountToken — POST /api/v1/auth/me/token/regenerate. L'ancien jeton cesse de valoir à l'instant, et
// les sessions qu'il avait ouvertes avec lui (leur génération ne correspond plus) ; l'état — actif, échéance — ne change
// pas, les compteurs repartent de zéro. Le membre lui-même, jamais depuis une session ouverte par le jeton.
func (server *Server) handleRegenerateMyAccountToken(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if refuseFederated(w, principal) {
		return
	}
	id := principal.ID
	result, err := server.db.Exec(
		`UPDATE account_tokens SET token = lower(hex(randomblob(32))), created_at = ?, use_count = 0, last_used_at = NULL WHERE user_id = ?`,
		now(), id,
	)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	// Les connexions parlaient de l'ancien jeton.
	_, _ = server.db.Exec(`DELETE FROM account_token_connections WHERE user_id = ?`, id)
	server.respondAccountToken(w, id)
}

// respondAccountToken — après un changement du jeton : la liste des comptes à
// jour part au kernel, puis le jeton revient à son propriétaire.
func (server *Server) respondAccountToken(w http.ResponseWriter, userID int64) {
	server.accountsChanged()
	token, err := server.accountToken(userID, true)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
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
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if token.Enabled {
		token, err = server.accountToken(principal.ID, true)
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
	}
	// « Depuis votre réseau actuel » : la connexion vient de l'adresse d'où son propriétaire lit cette page.
	viewer := claimedClientAddress(r)
	for index := range token.Connections {
		token.Connections[index].Here = viewer != "" && token.Connections[index].Address == viewer
	}
	var stored string
	if err := server.db.QueryRow(`SELECT consoles FROM users WHERE id = ?`, principal.ID).Scan(&stored); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
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
	respondErrorCode(w, http.StatusForbidden, "federated_session_play_only", "A session opened with an account token cannot manage the account.")

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

// ErrAccessRefused — le serveur qui ouvre la session ne présente pas d'accès délivré valable (chantier 66).
var ErrAccessRefused = errors.New("access refused")

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
		Token   string `json:"token"`
		From    string `json:"from"`
		Member  string `json:"member"`
		Kind    string `json:"kind"`
		Address string `json:"address"`
		Access  string `json:"access"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	session, account, err := server.FederatedSession(body.Token, TokenCaller{Name: body.From, Member: body.Member, Kind: body.Kind, Address: body.Address, Access: body.Access})
	if errors.Is(err, ErrTokenRefused) {
		respondErrorCode(w, http.StatusUnauthorized, "auth_required", "Invalid token.")
		return
	}
	if errors.Is(err, ErrAccessRefused) {
		respondErrorCode(w, http.StatusForbidden, "access_refused", "No valid access of this server was presented.")
		return
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
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
// le compteur avance (« une connexion »), et la ligne de son appelant (#781).
// ErrTokenRefused si le jeton ne vaut plus. La route interne du serveur et le
// téléphone (embed) passent par ici.
func (server *Server) FederatedSession(token string, caller TokenCaller) (string, TokenAccount, error) {
	account, err := server.AccountByToken(token)
	if err != nil {
		return "", TokenAccount{}, err
	}
	access, err := server.verifyFederationToken(caller.Access)
	if errors.Is(err, ErrCatalogTokenRefused) {
		return "", TokenAccount{}, ErrAccessRefused
	}
	if err != nil {
		return "", TokenAccount{}, err
	}
	var sessions int
	if err := server.db.QueryRow(`SELECT session_generation FROM users WHERE id = ?`, account.UserID).Scan(&sessions); err != nil {
		return "", TokenAccount{}, err
	}
	session, err := auth.CreateFederatedToken(server.config.JWTSecret, account.UserID, account.Username,
		account.Generation, sessions, access.ID, time.Until(account.ExpiresAt))
	if err != nil {
		return "", TokenAccount{}, err
	}
	at := now()
	_, _ = server.db.Exec(`UPDATE account_tokens SET use_count = use_count + 1, last_used_at = ? WHERE user_id = ?`,
		at, account.UserID)
	kind := caller.Kind
	if kind != "server" && kind != "phone" {
		kind = ""
	}
	_, _ = server.db.Exec(`INSERT INTO account_token_connections (user_id, name, member, kind, address, first_used_at, last_used_at)
		VALUES (?, ?, ?, ?, ?, ?, ?) ON CONFLICT (user_id, name, address)
		DO UPDATE SET last_used_at = excluded.last_used_at, use_count = use_count + 1, member = excluded.member, kind = excluded.kind`,
		account.UserID, trimmedTo(caller.Name, 100), trimmedTo(caller.Member, 64), kind, trimmedTo(caller.Address, 64), at, at)

	return session, account, nil
}
