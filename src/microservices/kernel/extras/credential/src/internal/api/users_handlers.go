package api

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gameflex/consoles"
	"gameflex/credential/internal/auth"
)

type credentialsBody struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type meUpdateBody struct {
	DisplayName     *string `json:"display_name"`
	CurrentPassword *string `json:"current_password"`
	NewPassword     *string `json:"new_password"`
}

func decodeBody(w http.ResponseWriter, r *http.Request, target any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "Invalid request body.")
		return false
	}
	return true
}

func (server *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var body credentialsBody
	if !decodeBody(w, r, &body) {
		return
	}

	// TROP D'ÉCHECS : ATTENDRE (#454, login_limiter.go). Avant de regarder le
	// mot de passe, pour qu'un essai de plus ne dise rien, juste ou faux.
	address := claimedClientAddress(r)
	if wait := server.logins.wait(body.Username, address); wait > 0 {
		minutes := int((wait + time.Minute - 1) / time.Minute)
		slog.Warn("login delayed", "username", body.Username, "from", address, "wait", wait.Round(time.Second))
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		respondErrorCode(w, http.StatusTooManyRequests, "too_many_attempts", fmt.Sprintf("Too many failed sign-ins. Try again in %d minute(s).", minutes))
		return
	}

	var userID int64
	var username, displayName string
	var roleLevel, generation int
	var hashedPassword string
	err := server.db.QueryRow(
		`SELECT id, username, display_name, role_level, hashed_password, session_generation FROM users WHERE username = ?`,
		body.Username,
	).Scan(&userID, &username, &displayName, &roleLevel, &hashedPassword, &generation)

	// UN REFUS LAISSE UNE TRACE, ET C'EST TOUT CE QU'IL LAISSE.
	//
	// Ce service authentifiait sans rien noter de qui essayait : une tentative
	// ratée ne s'écrivait NULLE PART, et la seule mémoire d'une réussite était
	// `last_login`, écrasée à chaque fois. On ne pouvait donc ni voir
	// quelqu'un s'acharner sur un mot de passe, ni confirmer à quelqu'un qu'on
	// l'avait bien vu arriver (constaté le 2026-09-07, en essayant de faire
	// exactement ça).
	//
	// Ça pèse plus lourd depuis ce matin : `/api/v1/auth/login` est désormais
	// joignable depuis une AUTRE origine (chantier 9, C9-P3), donc depuis le
	// GameFlex de quelqu'un d'autre. La surface s'est élargie ; sa mémoire
	// était restée vide.
	//
	// La cause exacte du refus est dite ICI et seulement ici. La réponse HTTP,
	// elle, reste la même dans les deux cas — « invalid username or password »
	// — sinon elle dirait à un inconnu quels comptes existent.
	//
	// UN COMPTE QUI ATTEND SON LIEN (chantier 55) n'a pas de mot de passe : il est
	// refusé avant bcrypt, avec la même réponse et le même compte d'échecs qu'un
	// mot de passe faux — rien ne dit à un inconnu qu'un compte attend.
	awaiting := err == nil && hashedPassword == ""
	if errors.Is(err, sql.ErrNoRows) || awaiting || (err == nil && !auth.VerifyPassword(body.Password, hashedPassword)) {
		reason := "wrong password"
		if errors.Is(err, sql.ErrNoRows) {
			reason = "no such account"
		} else if awaiting {
			reason = "awaiting its invitation link"
		}
		server.logins.fail(body.Username, address)
		slog.Warn("login refused",
			"username", body.Username, "reason", reason,
			"from", address, "origin", r.Header.Get("Origin"))
		respondErrorCode(w, http.StatusUnauthorized, "login_refused", "Invalid username or password.")
		return
	}
	if err != nil {
		slog.Error("login failed", "username", body.Username, "error", err)
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	token, err := auth.CreateToken(server.config.JWTSecret, userID, username, roleLevel, generation)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	// Invite panel ordering source — most recent connection first. Elle est
	// ÉCRASÉE à chaque connexion : elle dit « la dernière fois », jamais « les
	// fois ». C'est pour ça que le journal ci-dessous existe à côté.
	_, _ = server.db.Exec(
		`UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = ?`,
		userID,
	)

	server.logins.succeed(body.Username, address)
	slog.Info("login granted",
		"username", username, "role_level", roleLevel,
		"from", address, "origin", r.Header.Get("Origin"))

	respondItem(w, http.StatusOK, map[string]any{
		"access_token": token,
		"token_type":   "bearer",
		"user":         server.fetchUserResponse(userID),
	})
}

func (server *Server) handleMe(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	respondItem(w, http.StatusOK, server.fetchUserResponse(principal.ID))
}

func (server *Server) handleUpdateMe(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if refuseFederated(w, principal) {
		return
	}
	var body meUpdateBody
	if !decodeBody(w, r, &body) {
		return
	}

	updates := map[string]any{}
	if body.DisplayName != nil {
		updates["display_name"] = *body.DisplayName
	}
	// UN NOUVEAU MOT DE PASSE (chantier 55, le porteur : « ça reste, et pareil
	// comme pour l'initialisation du mot de passe, il faut que ça soit sur les
	// mêmes recommandations ») : la règle du lien, puis la génération des
	// sessions avance — les autres appareils tombent, celui-ci reçoit un jeton
	// neuf (`access_token`, à côté du compte).
	changingPassword := body.NewPassword != nil && *body.NewPassword != ""
	if changingPassword {
		if body.CurrentPassword == nil || !server.verifyCurrentPassword(principal.ID, *body.CurrentPassword) {
			respondErrorCode(w, http.StatusBadRequest, "password_wrong", "Current password is incorrect.")
			return
		}
		if refusals := auth.PasswordRefusals(*body.NewPassword); len(refusals) > 0 {
			respondPasswordRefused(w, refusals)
			return
		}
		hashed, err := auth.HashPassword(*body.NewPassword)
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		updates["hashed_password"] = hashed
		updates["password_set_at"] = now()
	}

	if len(updates) > 0 {
		if err := server.applyUserUpdate(principal.ID, updates); err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
	}
	if !changingPassword {
		respondItem(w, http.StatusOK, server.fetchUserResponse(principal.ID))
		return
	}

	if err := server.AdvanceSessions(principal.ID); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	token, err := server.SessionToken(principal.ID)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	slog.Info("password changed", "username", principal.Username)
	respondItem(w, http.StatusOK, struct {
		userResponse
		AccessToken string `json:"access_token"`
	}{server.fetchUserResponse(principal.ID), token})
}

func (server *Server) handleLogout(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	// Stateless JWT — nothing to invalidate server-side, same as Python.
	respondItem(w, http.StatusOK, map[string]any{"logged_out": true})
}

func (server *Server) handleResolveUsers(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		Usernames []string `json:"usernames"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	resolved := map[string]string{}
	for _, username := range body.Usernames {
		var displayName string
		err := server.db.QueryRow(`SELECT display_name FROM users WHERE username = ?`, username).Scan(&displayName)
		if err == nil {
			resolved[username] = displayName
		}
	}
	respondItem(w, http.StatusOK, resolved)
}

// ── Admin member management ──

// handleListMembers — invite panel source: every member, most recent
// connection first (offline users included — the frontend overlays live
// presence badges on top). Not admin-gated: any member can invite another.
func (server *Server) handleListMembers(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	rows, err := server.db.Query(
		`SELECT id, username, display_name, last_login
		 FROM users
		 ORDER BY last_login IS NULL, last_login DESC, username ASC`,
	)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	defer rows.Close()

	type member struct {
		ID          int64   `json:"id"`
		Username    string  `json:"username"`
		DisplayName string  `json:"display_name"`
		LastLogin   *string `json:"last_login"`
	}

	members := make([]member, 0, 16)
	for rows.Next() {
		var entry member
		if err := rows.Scan(&entry.ID, &entry.Username, &entry.DisplayName, &entry.LastLogin); err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		members = append(members, entry)
	}
	if err := rows.Err(); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	respondItem(w, http.StatusOK, map[string]any{"members": members})
}

func (server *Server) handleListUsers(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	page := queryInt(r, "page", 1)
	if page < 1 {
		page = 1
	}
	itemsPerPage := queryInt(r, "items_per_page", 20)
	if itemsPerPage < 1 || itemsPerPage > 100 {
		itemsPerPage = 20
	}

	// role_level est filtrable depuis le 2026-09-01 : l'écran des membres
	// propose un filtre par rôle, et la liste est paginée PAR LE SERVEUR —
	// trier côté page n'aurait filtré que la page affichée, en laissant le
	// compteur annoncer le total non filtré.
	//
	// password_set_at depuis le chantier 55 : le filtre « invitation validée »
	// (`not_null` : ceux qui ont choisi leur mot de passe ; `null` : ceux qui
	// attendent leur lien).
	where, args, err := parseSearch(r.URL.Query().Get("search"), "username,display_name,role_level,password_set_at")
	if err != nil {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "Invalid 'search' parameter. Expected a JSON object or array of objects.")
		return
	}

	// token_enabled=1 depuis le 2026-11-14 (chantier 53, P11) : le filtre des
	// membres du launcher propose « jeton activé ». Le jeton vit dans sa table
	// (account_tokens.go) : il se joint ici, au serveur, pour la même raison que
	// le rôle — la liste est paginée, et le compteur doit dire le total filtré.
	// Les groupes de `search` se combinent en OU : ils restent entre parenthèses.
	if r.URL.Query().Get("token_enabled") == "1" {
		tokenClause := "id IN (SELECT user_id FROM account_tokens WHERE enabled = 1)"
		if where != "" {
			where = "(" + where + ") AND " + tokenClause
		} else {
			where = tokenClause
		}
	}

	base := "SELECT " + userColumns + " FROM users"
	countQuery := "SELECT COUNT(*) FROM users"
	if where != "" {
		base += " WHERE " + where
		countQuery += " WHERE " + where
	}

	var count int
	if err := server.db.QueryRow(countQuery, args...).Scan(&count); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	offset := (page - 1) * itemsPerPage
	rows, err := server.db.Query(base+" ORDER BY id LIMIT ? OFFSET ?", append(args, itemsPerPage, offset)...)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	defer rows.Close()

	users := []userResponse{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		users = append(users, user)
	}
	rows.Close()
	for index := range users {
		if token, err := server.accountToken(users[index].ID, true); err == nil {
			users[index].Token = &token
		}
		users[index].Invitation = server.memberLink(users[index].ID)
	}
	respondItems(w, http.StatusOK, users, newPagination(count, page, itemsPerPage))
}

func (server *Server) handleFetchUser(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}

	row := server.db.QueryRow(
		"SELECT "+userColumns+" FROM users WHERE id = ?", id,
	)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if token, err := server.accountToken(user.ID, true); err == nil {
		user.Token = &token
	}
	user.Invitation = server.memberLink(user.ID)
	respondItem(w, http.StatusOK, user)
}

// usernamePattern — un nom de compte : celui que le kernel accepte pour lancer
// une partie (manager.usernamePattern), trois caractères au moins. Le nom
// devient un dossier (saves/<nom>) : ni `/`, ni `..` (revue de sécurité du
// kernel, #601). Le kernel les refusait déjà ; un compte au nom refusé ne
// pouvait que se connecter.
var usernamePattern = regexp.MustCompile(`^[A-Za-z0-9._-]{3,32}$`)

// assignableRole — les rangs qu'un administrateur donne : administrateur ou
// joueur. Ni root, ni un rang inventé : un rang négatif comptait comme
// administrateur, ici comme dans le kernel (« <= 1 »).
func assignableRole(level int) bool {
	return level == auth.RoleAdmin || level == auth.RoleUser
}

func (server *Server) handleCreateUser(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	// SANS MOT DE PASSE (chantier 55) : le membre le choisit par son lien.
	// `password` n'est plus un champ : envoyé, il est refusé (400,
	// DisallowUnknownFields).
	var body struct {
		Username    string    `json:"username"`
		DisplayName string    `json:"display_name"`
		RoleLevel   *int      `json:"role_level"`
		Consoles    *[]string `json:"consoles"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	roleLevel := auth.RoleUser
	if body.RoleLevel != nil {
		roleLevel = *body.RoleLevel
	}

	if roleLevel == auth.RoleRoot {
		respondErrorCode(w, http.StatusForbidden, "root_locked", "Cannot create another root account.")
		return
	}
	if !assignableRole(roleLevel) {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "Role must be 1 (admin) or 2 (user).")
		return
	}
	grants := consoles.GrantAll
	if body.Consoles != nil {
		parsed, err := consoles.ParseGrants(*body.Consoles)
		if err != nil {
			respondErrorCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		grants = parsed
	}
	if !usernamePattern.MatchString(body.Username) {
		respondErrorCode(w, http.StatusBadRequest, "username_invalid", "Username must be 3 to 32 letters, digits, dots, dashes or underscores.")
		return
	}
	// Le compte et son lien, dans la même transaction : un compte n'existe
	// jamais sans le moyen d'y entrer.
	label := body.DisplayName
	if label == "" {
		label = body.Username
	}
	var userID int64
	link, err := server.inTransaction(func(transaction *sql.Tx) (memberLinkResponse, error) {
		result, err := transaction.Exec(
			`INSERT INTO users (username, hashed_password, display_name, role_level, created_at, created_by_id, consoles)
			 VALUES (?, '', ?, ?, ?, ?, ?)`,
			body.Username, body.DisplayName, roleLevel, now(), principal.ID, grants,
		)
		if err != nil {
			return memberLinkResponse{}, err
		}
		userID, _ = result.LastInsertId()

		return replaceMemberLink(transaction, InvitationJoin, userID, label, principal)
	})
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			respondErrorCode(w, http.StatusConflict, "username_taken", "Username already taken.")
			return
		}
		slog.Error("create member failed", "username", body.Username, "error", err)
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	server.accountsChanged()
	slog.Info("member invited", "username", body.Username, "by", principal.Username, "until", link.ExpiresAt)
	respondItem(w, http.StatusCreated, server.memberWithLink(userID, link))
}

func (server *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}

	var targetRole int
	err = server.db.QueryRow(`SELECT role_level FROM users WHERE id = ?`, id).Scan(&targetRole)
	if errors.Is(err, sql.ErrNoRows) {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if targetRole == auth.RoleRoot {
		respondErrorCode(w, http.StatusForbidden, "root_locked", "Cannot modify the root account.")
		return
	}

	// `password` n'est plus un champ (chantier 55) : un administrateur
	// réinitialise (member_links.go), il ne choisit plus.
	var body struct {
		DisplayName *string   `json:"display_name"`
		RoleLevel   *int      `json:"role_level"`
		Consoles    *[]string `json:"consoles"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	if body.RoleLevel != nil && *body.RoleLevel == auth.RoleRoot {
		respondErrorCode(w, http.StatusForbidden, "root_locked", "Cannot grant root access.")
		return
	}
	if body.RoleLevel != nil && !assignableRole(*body.RoleLevel) {
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "Role must be 1 (admin) or 2 (user).")
		return
	}

	updates := map[string]any{}
	if body.Consoles != nil {
		grants, err := consoles.ParseGrants(*body.Consoles)
		if err != nil {
			respondErrorCode(w, http.StatusBadRequest, "bad_request", err.Error())
			return
		}
		updates["consoles"] = grants
	}
	if body.DisplayName != nil {
		updates["display_name"] = *body.DisplayName
	}
	if body.RoleLevel != nil {
		updates["role_level"] = *body.RoleLevel
	}
	if err := server.applyUserUpdate(id, updates); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	server.accountsChanged()
	respondItem(w, http.StatusOK, server.memberResponse(id))
}

func (server *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	if id == principal.ID {
		respondErrorCode(w, http.StatusBadRequest, "own_account_delete", "Cannot delete your own account.")
		return
	}

	var (
		targetName string
		targetRole int
	)
	err = server.db.QueryRow(`SELECT username, role_level FROM users WHERE id = ?`, id).Scan(&targetName, &targetRole)
	if errors.Is(err, sql.ErrNoRows) {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if targetRole == auth.RoleRoot {
		respondErrorCode(w, http.StatusForbidden, "root_locked", "Cannot delete the root account.")
		return
	}

	// SUPPRIMER UN COMPTE, C'EST AUSSI SUPPRIMER CE QUI LUI APPARTIENT.
	//
	// Toutes les clés étrangères vers users(id) sont en NO ACTION, et
	// `foreign_keys=ON` (voir database.go) : un DELETE nu échouait donc en 500
	// dès que le compte avait un favori, une ligne d'historique ou un mapping
	// de manette. Relevé par l'utilisateur le 2026-09-18 — sur un compte dont
	// il venait justement d'effacer les sauvegardes, ce qui n'y était pour
	// rien : les sauvegardes sont des FICHIERS, la base n'en sait rien. Les
	// deux faits paraissaient liés, ils ne l'étaient pas.
	//
	// Deux traitements, et la distinction EST le correctif :
	//   - ce qui APPARTIENT à la personne part avec elle ;
	//   - ce qui note seulement QUI A CRÉÉ une ressource partagée (un autre
	//     compte, un jeton de catalogue, un montage distant, un serveur
	//     fédéré) perd son auteur, et rien d'autre. Supprimer un
	//     administrateur ne doit ni révoquer les accès qu'il a distribués, ni
	//     effacer les comptes qu'il a créés.
	//
	// SES SAUVEGARDES PARTENT AUSSI (le porteur, 2026-11-14 : « Supprimer ») :
	// ce sont des fichiers, au kernel, qui les efface quand la liste des comptes
	// le nomme supprimé (server.deleted). Pas en silence : la fenêtre qui
	// confirme la suppression le dit.
	ownedByUser := []string{
		"play_history", "controller_mappings", "key_mappings",
		"key_positionings", "notification_preferences", "push_subscriptions",
		// Ses jetons chez nos amis (chantier 20) : ses comptes là-bas, à lui.
		"federated_accounts",
		// Ses favoris (favorites.go).
		"favorites",
		// Les serveurs fédérés qu'il a éteints (hidden_servers.go).
		"hidden_servers",
	}
	// Chaque table et sa colonne d'auteur. LE FOURNISSEUR DE MÉTADONNÉES (chantier 49) note QUI a enregistré ses
	// clés (`metadata_services.updated_by_id`) : il perd son auteur, jamais son réglage — il manquait ici, et
	// supprimer un administrateur qui avait configuré un fournisseur échouait sur la clé étrangère (un 500).
	authoredByUser := []struct{ table, column string }{
		{"users", "created_by_id"}, {"federation_tokens", "created_by_id"}, {"remote_mounts", "created_by_id"},
		{"federation_servers", "created_by_id"}, {"metadata_services", "updated_by_id"},
	}

	transaction, err := server.db.Begin()
	if err != nil {
		slog.Error("delete user: begin failed", "user_id", id, "error", err)
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}

	defer transaction.Rollback()

	// Les noms de tables et de colonnes sont des constantes écrites juste
	// au-dessus, jamais une valeur venue de la requête.
	for _, table := range ownedByUser {
		if _, err := transaction.Exec("DELETE FROM "+table+" WHERE user_id = ?", id); err != nil {
			slog.Error("delete user: owned rows failed", "user_id", id, "table", table, "error", err)
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

			return
		}
	}

	for _, authored := range authoredByUser {
		if _, err := transaction.Exec("UPDATE "+authored.table+" SET "+authored.column+" = NULL WHERE "+authored.column+" = ?", id); err != nil {
			slog.Error("delete user: authorship failed", "user_id", id, "table", authored.table, "error", err)
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

			return
		}
	}

	// JOURNALISER L'ERREUR RÉELLE. Le client ne doit rien apprendre de la
	// base, mais nous si : « Internal server error. » était tout ce qui
	// existait, côté écran comme côté machine, et c'est pourquoi ce défaut
	// n'avait aucune piste à offrir.
	if _, err := transaction.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		slog.Error("delete user failed", "user_id", id, "error", err)
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}

	if err := transaction.Commit(); err != nil {
		slog.Error("delete user: commit failed", "user_id", id, "error", err)
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}

	slog.Info("user deleted", "user_id", id, "username", targetName, "by", principal.Username)
	server.deleted.add(targetName, time.Now())
	server.accountsChanged()
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ──

func (server *Server) fetchUserResponse(userID int64) userResponse {
	row := server.db.QueryRow(
		"SELECT "+userColumns+" FROM users WHERE id = ?", userID,
	)
	user, err := scanUser(row)
	if err != nil {
		slog.Error("fetch user failed", "user_id", userID, "error", err)
	}
	return user
}

// memberResponse — un compte tel que l'écran « Membres » le montre, jeton
// compris (chantier 20) : la réponse d'une création ou d'une modification
// remplace sa ligne sur place, et ne doit pas lui faire perdre son jeton.
func (server *Server) memberResponse(userID int64) userResponse {
	user := server.fetchUserResponse(userID)
	if token, err := server.accountToken(userID, true); err == nil {
		user.Token = &token
	}
	user.Invitation = server.memberLink(userID)

	return user
}

func (server *Server) verifyCurrentPassword(userID int64, password string) bool {
	var hashed string
	err := server.db.QueryRow(`SELECT hashed_password FROM users WHERE id = ?`, userID).Scan(&hashed)
	return err == nil && auth.VerifyPassword(password, hashed)
}

func (server *Server) applyUserUpdate(userID int64, updates map[string]any) error {
	if len(updates) == 0 {
		return nil
	}
	columns := []string{}
	args := []any{}
	for column, value := range updates {
		columns = append(columns, column+" = ?")
		args = append(args, value)
	}
	args = append(args, userID)
	_, err := server.db.Exec(
		fmt.Sprintf("UPDATE users SET %s WHERE id = ?", strings.Join(columns, ", ")),
		args...,
	)
	return err
}

func queryInt(r *http.Request, name string, fallback int) int {
	raw := r.URL.Query().Get(name)
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}
