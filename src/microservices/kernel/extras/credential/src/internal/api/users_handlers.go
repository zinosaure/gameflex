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

const passwordMinLength = 8

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
		respondError(w, http.StatusBadRequest, "Invalid request body.")
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
		respondError(w, http.StatusTooManyRequests,
			fmt.Sprintf("Too many failed sign-ins. Try again in %d minute(s).", minutes))
		return
	}

	var userID int64
	var username, displayName string
	var roleLevel int
	var hashedPassword string
	err := server.db.QueryRow(
		`SELECT id, username, display_name, role_level, hashed_password FROM users WHERE username = ?`,
		body.Username,
	).Scan(&userID, &username, &displayName, &roleLevel, &hashedPassword)

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
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !auth.VerifyPassword(body.Password, hashedPassword)) {
		reason := "wrong password"
		if errors.Is(err, sql.ErrNoRows) {
			reason = "no such account"
		}
		server.logins.fail(body.Username, address)
		slog.Warn("login refused",
			"username", body.Username, "reason", reason,
			"from", address, "origin", r.Header.Get("Origin"))
		respondError(w, http.StatusUnauthorized, "Invalid username or password.")
		return
	}
	if err != nil {
		slog.Error("login failed", "username", body.Username, "error", err)
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}

	token, err := auth.CreateToken(server.config.JWTSecret, userID, username, roleLevel)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
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
	if body.NewPassword != nil && *body.NewPassword != "" {
		if body.CurrentPassword == nil || !server.verifyCurrentPassword(principal.ID, *body.CurrentPassword) {
			respondError(w, http.StatusBadRequest, "Current password is incorrect.")
			return
		}
		if len(*body.NewPassword) < passwordMinLength {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Password must be at least %d characters.", passwordMinLength))
			return
		}
		hashed, err := auth.HashPassword(*body.NewPassword)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		updates["hashed_password"] = hashed
	}

	if len(updates) > 0 {
		if err := server.applyUserUpdate(principal.ID, updates); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
	}
	respondItem(w, http.StatusOK, server.fetchUserResponse(principal.ID))
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
		respondError(w, http.StatusInternalServerError, "Internal server error.")
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
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		members = append(members, entry)
	}
	if err := rows.Err(); err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
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
	where, args, err := parseSearch(r.URL.Query().Get("search"), "username,display_name,role_level")
	if err != nil {
		respondError(w, http.StatusBadRequest, "Invalid 'search' parameter. Expected a JSON object or array of objects.")
		return
	}

	base := "SELECT id, username, display_name, role_level, created_at, created_by_id, consoles FROM users"
	countQuery := "SELECT COUNT(*) FROM users"
	if where != "" {
		base += " WHERE " + where
		countQuery += " WHERE " + where
	}

	var count int
	if err := server.db.QueryRow(countQuery, args...).Scan(&count); err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}

	offset := (page - 1) * itemsPerPage
	rows, err := server.db.Query(base+" ORDER BY id LIMIT ? OFFSET ?", append(args, itemsPerPage, offset)...)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	defer rows.Close()

	users := []userResponse{}
	for rows.Next() {
		user, err := scanUser(rows)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		users = append(users, user)
	}
	rows.Close()
	for index := range users {
		if token, err := server.accountToken(users[index].ID, true); err == nil {
			users[index].Token = &token
		}
	}
	respondItems(w, http.StatusOK, users, newPagination(count, page, itemsPerPage))
}

func (server *Server) handleFetchUser(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}

	row := server.db.QueryRow(
		`SELECT id, username, display_name, role_level, created_at, created_by_id, consoles FROM users WHERE id = ?`, id,
	)
	user, err := scanUser(row)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	if token, err := server.accountToken(user.ID, true); err == nil {
		user.Token = &token
	}
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
	var body struct {
		Username    string    `json:"username"`
		Password    string    `json:"password"`
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
		respondError(w, http.StatusForbidden, "Cannot create another root account.")
		return
	}
	if !assignableRole(roleLevel) {
		respondError(w, http.StatusBadRequest, "Role must be 1 (admin) or 2 (user).")
		return
	}
	grants := consoles.GrantAll
	if body.Consoles != nil {
		parsed, err := consoles.ParseGrants(*body.Consoles)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
			return
		}
		grants = parsed
	}
	if !usernamePattern.MatchString(body.Username) {
		respondError(w, http.StatusBadRequest,
			"Username must be 3 to 32 letters, digits, dots, dashes or underscores.")
		return
	}
	if len(body.Password) < passwordMinLength {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("Password must be at least %d characters.", passwordMinLength))
		return
	}

	hashed, err := auth.HashPassword(body.Password)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}

	result, err := server.db.Exec(
		`INSERT INTO users (username, hashed_password, display_name, role_level, created_at, created_by_id, consoles)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		body.Username, hashed, body.DisplayName, roleLevel, now(), principal.ID, grants,
	)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			respondError(w, http.StatusConflict, "Username already taken.")
			return
		}
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	userID, _ := result.LastInsertId()
	server.accountsChanged()
	respondItem(w, http.StatusCreated, server.memberResponse(userID))
}

func (server *Server) handleUpdateUser(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}

	var targetRole int
	err = server.db.QueryRow(`SELECT role_level FROM users WHERE id = ?`, id).Scan(&targetRole)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	if targetRole == auth.RoleRoot {
		respondError(w, http.StatusForbidden, "Cannot modify the root account.")
		return
	}

	var body struct {
		Password    *string   `json:"password"`
		DisplayName *string   `json:"display_name"`
		RoleLevel   *int      `json:"role_level"`
		Consoles    *[]string `json:"consoles"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	if body.RoleLevel != nil && *body.RoleLevel == auth.RoleRoot {
		respondError(w, http.StatusForbidden, "Cannot grant root access.")
		return
	}
	if body.RoleLevel != nil && !assignableRole(*body.RoleLevel) {
		respondError(w, http.StatusBadRequest, "Role must be 1 (admin) or 2 (user).")
		return
	}

	updates := map[string]any{}
	if body.Consoles != nil {
		grants, err := consoles.ParseGrants(*body.Consoles)
		if err != nil {
			respondError(w, http.StatusBadRequest, err.Error())
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
	if body.Password != nil && *body.Password != "" {
		if len(*body.Password) < passwordMinLength {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("Password must be at least %d characters.", passwordMinLength))
			return
		}
		hashed, err := auth.HashPassword(*body.Password)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		updates["hashed_password"] = hashed
	}

	if err := server.applyUserUpdate(id, updates); err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	server.accountsChanged()
	respondItem(w, http.StatusOK, server.memberResponse(id))
}

func (server *Server) handleDeleteUser(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}
	if id == principal.ID {
		respondError(w, http.StatusBadRequest, "Cannot delete your own account.")
		return
	}

	var targetRole int
	err = server.db.QueryRow(`SELECT role_level FROM users WHERE id = ?`, id).Scan(&targetRole)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "User not found.")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	if targetRole == auth.RoleRoot {
		respondError(w, http.StatusForbidden, "Cannot delete the root account.")
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
	// Les dossiers de sauvegarde, eux, restent en place : supprimer un compte
	// ne doit pas détruire ses parties en silence.
	ownedByUser := []string{
		"play_history", "controller_mappings", "key_mappings",
		"key_positionings", "notification_preferences", "push_subscriptions",
		// Ses jetons chez nos amis (chantier 20) : ses comptes là-bas, à lui.
		"federated_accounts",
	}
	authoredByUser := []string{
		"users", "federation_tokens", "remote_mounts", "federation_servers",
	}

	transaction, err := server.db.Begin()
	if err != nil {
		slog.Error("delete user: begin failed", "user_id", id, "error", err)
		respondError(w, http.StatusInternalServerError, "Internal server error.")

		return
	}

	defer transaction.Rollback()

	// Les noms de tables sont des constantes écrites juste au-dessus, jamais
	// une valeur venue de la requête.
	for _, table := range ownedByUser {
		if _, err := transaction.Exec("DELETE FROM "+table+" WHERE user_id = ?", id); err != nil {
			slog.Error("delete user: owned rows failed", "user_id", id, "table", table, "error", err)
			respondError(w, http.StatusInternalServerError, "Internal server error.")

			return
		}
	}

	for _, table := range authoredByUser {
		if _, err := transaction.Exec("UPDATE "+table+" SET created_by_id = NULL WHERE created_by_id = ?", id); err != nil {
			slog.Error("delete user: authorship failed", "user_id", id, "table", table, "error", err)
			respondError(w, http.StatusInternalServerError, "Internal server error.")

			return
		}
	}

	// JOURNALISER L'ERREUR RÉELLE. Le client ne doit rien apprendre de la
	// base, mais nous si : « Internal server error. » était tout ce qui
	// existait, côté écran comme côté machine, et c'est pourquoi ce défaut
	// n'avait aucune piste à offrir.
	if _, err := transaction.Exec(`DELETE FROM users WHERE id = ?`, id); err != nil {
		slog.Error("delete user failed", "user_id", id, "error", err)
		respondError(w, http.StatusInternalServerError, "Internal server error.")

		return
	}

	if err := transaction.Commit(); err != nil {
		slog.Error("delete user: commit failed", "user_id", id, "error", err)
		respondError(w, http.StatusInternalServerError, "Internal server error.")

		return
	}

	slog.Info("user deleted", "user_id", id, "by", principal.Username)
	server.accountsChanged()
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ──

func (server *Server) fetchUserResponse(userID int64) userResponse {
	row := server.db.QueryRow(
		`SELECT id, username, display_name, role_level, created_at, created_by_id, consoles FROM users WHERE id = ?`, userID,
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
