package api

// LE LIEN D'UN COMPTE (chantier 55, `#693`, `#696`) — le porteur, 2026-11-14 :
// « les nouveaux membres vont rejoindre avec un lien d'invitation où ils vont
// devoir mettre un mot de passe strong pour la première fois » ; « le reset
// password se fait par un administrateur et donc on lui donne un nouveau lien
// pour rejoindre et entrer un nouveau mot de passe. Quand on reset un compte,
// son ancienne session devient invalide et se déconnecte automatiquement. »
//
// L'administrateur ne choisit plus le mot de passe d'un membre, et ne le connaît
// plus. Il crée le compte SANS mot de passe, et reçoit un lien
// `<launcher>/join/<code>` : la personne l'ouvre dans son navigateur, choisit
// son mot de passe (la règle, auth.PasswordRefusals), et se trouve connectée.
// Réinitialiser vide le mot de passe, coupe toutes les sessions du compte (la
// génération, sessions.go) et donne un nouveau lien.
//
// Les règles du chantier 33 (invitations.go), reprises telles quelles : un code
// de 128 bits, à usage unique, seule son empreinte en base, la consommation par
// le DELETE (un seul gagnant), les échecs freinés par adresse. Ce qui change :
//
//   - 24 heures, et non 12 (« Max 24 heures et une seule utilisation. Une fois
//     que le mot de passe est défini le lien expire. ») ;
//   - un seul lien vivant par membre : en créer un retire le précédent, dans la
//     même transaction ;
//   - la personne l'échange elle-même, sans compte, par deux routes du domaine
//     `auth` — le code dans le CORPS d'un POST, jamais dans une adresse ;
//   - un lien mort répond 410, et non 401 : le launcher ferme sa session sur un
//     401 du service des comptes, et un lien périmé ouvert par quelqu'un de
//     connecté ne doit pas le déconnecter ;
//   - la fédération ne le voit pas : sa liste, sa révocation et son échange ne
//     lisent que ses deux sortes (invitations.go).

import (
	"database/sql"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
)

const (
	// InvitationJoin — un nouveau membre choisit son premier mot de passe.
	InvitationJoin = "join"
	// InvitationReset — un administrateur a réinitialisé le mot de passe.
	InvitationReset = "reset"

	// memberLinkLifetime — « Max 24 heures » (le porteur, 2026-11-14).
	memberLinkLifetime = 24 * time.Hour
)

// memberLinkKinds — les sortes d'un lien de compte, pour une requête SQL.
const memberLinkKinds = "('" + InvitationJoin + "', '" + InvitationReset + "')"

// memberLinkResponse — le lien vivant d'un membre, tel que l'administrateur le
// voit : sa sorte, jusqu'à quand, qui l'a créé et quand. Code n'est rempli qu'à
// la création : la base n'en a que l'empreinte.
type memberLinkResponse struct {
	Kind        string `json:"kind"`
	ExpiresAt   string `json:"expires_at"`
	CreatedAt   any    `json:"created_at"`
	CreatedByID any    `json:"created_by_id"`
	CreatedBy   any    `json:"created_by"`
	Code        string `json:"code,omitempty"`
}

// replaceMemberLink — le lien de ce membre, à la place de tout autre : dans la
// transaction de celui qui le demande (une création, une réinitialisation).
func replaceMemberLink(transaction *sql.Tx, kind string, userID int64, label string, by auth.Principal) (memberLinkResponse, error) {
	if _, err := transaction.Exec(`DELETE FROM invitations WHERE user_id = ? AND kind IN `+memberLinkKinds, userID); err != nil {
		return memberLinkResponse{}, err
	}
	code := config.RandomToken(invitationCodeBytes)
	createdAt := now()
	expiresAt := time.Now().Add(memberLinkLifetime).UTC().Format(time.RFC3339)
	if _, err := transaction.Exec(
		`INSERT INTO invitations (kind, code_hash, label, scopes, user_id, expires_at, created_at, created_by_id)
		 VALUES (?, ?, ?, '', ?, ?, ?, ?)`,
		kind, hashToken(code), label, userID, expiresAt, createdAt, by.ID,
	); err != nil {
		return memberLinkResponse{}, err
	}

	return memberLinkResponse{
		Kind: kind, ExpiresAt: expiresAt, CreatedAt: isoDatetime(sql.NullString{String: createdAt, Valid: true}),
		CreatedByID: by.ID, CreatedBy: by.Username, Code: code,
	}, nil
}

// memberLink — le lien vivant de ce membre, ou nil.
func (server *Server) memberLink(userID int64) *memberLinkResponse {
	server.purgeInvitations()
	var (
		link      memberLinkResponse
		createdAt sql.NullString
		createdBy sql.NullInt64
		author    sql.NullString
	)
	err := server.db.QueryRow(
		`SELECT i.kind, i.expires_at, i.created_at, i.created_by_id, u.username
		 FROM invitations i LEFT JOIN users u ON u.id = i.created_by_id
		 WHERE i.user_id = ? AND i.kind IN `+memberLinkKinds+` ORDER BY i.id DESC LIMIT 1`, userID,
	).Scan(&link.Kind, &link.ExpiresAt, &createdAt, &createdBy, &author)
	if err != nil {
		return nil
	}
	link.CreatedAt = isoDatetime(createdAt)
	if createdBy.Valid {
		link.CreatedByID = createdBy.Int64
	}
	link.CreatedBy = text(author)

	return &link
}

// awaitingLink — ce membre attend-il son lien (aucun mot de passe) ? Rend aussi
// son rang et son nom affiché ; sql.ErrNoRows s'il n'existe pas.
func (server *Server) awaitingLink(userID int64) (awaiting bool, roleLevel int, label string, err error) {
	var hashed, username, displayName string
	err = server.db.QueryRow(`SELECT hashed_password, role_level, username, display_name FROM users WHERE id = ?`, userID).
		Scan(&hashed, &roleLevel, &username, &displayName)
	label = displayName
	if label == "" {
		label = username
	}

	return hashed == "", roleLevel, label, err
}

// memberWithLink — la réponse d'une route qui crée un lien : le membre, son lien
// et, cette fois-là seulement, son code.
func (server *Server) memberWithLink(userID int64, link memberLinkResponse) userResponse {
	member := server.memberResponse(userID)
	member.Invitation = &link

	return member
}

// handleNewMemberLink — POST /api/v1/users/{id}/invitation : un nouveau lien pour
// un membre qui n'a pas encore rejoint ; l'ancien ne vaut plus. Un membre qui a
// déjà choisi son mot de passe se réinitialise (409).
func (server *Server) handleNewMemberLink(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	awaiting, _, label, err := server.awaitingLink(id)
	if errors.Is(err, sql.ErrNoRows) {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if !awaiting {
		respondErrorCode(w, http.StatusConflict, "member_has_password", "This member has already chosen a password: reset it instead.")
		return
	}

	link, err := server.inTransaction(func(transaction *sql.Tx) (memberLinkResponse, error) {
		return replaceMemberLink(transaction, InvitationJoin, id, label, principal)
	})
	if err != nil {
		slog.Error("invitation renewal failed", "user_id", id, "error", err)
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	slog.Info("invitation renewed", "user_id", id, "by", principal.Username, "until", link.ExpiresAt)
	respondItem(w, http.StatusCreated, server.memberWithLink(id, link))
}

// handleResetMember — POST /api/v1/users/{id}/reset : réinitialiser le mot de
// passe d'un membre qui a rejoint. Dans une transaction : son mot de passe vidé,
// sa date effacée, la génération de ses sessions avancée — et celle de ses
// parties —, et le lien `reset` à la place de tout autre. Ensuite, toutes ses
// sessions tombent (sessionsEnded) — et ses parties en cours, au kernel, qui le
// lit dans la liste des comptes (« Oui, toutes les sessions », le porteur). Le
// jeton du compte ne change pas.
// Ni root, ni son propre compte (403) ; un membre en attente reçoit un nouveau
// lien, pas une réinitialisation (409).
func (server *Server) handleResetMember(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	awaiting, roleLevel, label, err := server.awaitingLink(id)
	if errors.Is(err, sql.ErrNoRows) {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")
		return
	}
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	switch {
	case roleLevel == auth.RoleRoot:
		respondErrorCode(w, http.StatusForbidden, "root_locked", "Cannot reset the root account.")
		return
	case id == principal.ID:
		respondErrorCode(w, http.StatusForbidden, "own_account", "Cannot reset your own account: change your password instead.")
		return
	case awaiting:
		respondErrorCode(w, http.StatusConflict, "member_no_password", "This member has not chosen a password yet: send a new invitation instead.")
		return
	}

	link, err := server.inTransaction(func(transaction *sql.Tx) (memberLinkResponse, error) {
		if _, err := transaction.Exec(
			`UPDATE users SET hashed_password = '', password_set_at = NULL, session_generation = session_generation + 1, games_generation = games_generation + 1 WHERE id = ?`, id,
		); err != nil {
			return memberLinkResponse{}, err
		}

		return replaceMemberLink(transaction, InvitationReset, id, label, principal)
	})
	if err != nil {
		slog.Error("password reset failed", "user_id", id, "error", err)
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	server.sessionsEnded(id)
	slog.Info("password reset", "user_id", id, "by", principal.Username, "until", link.ExpiresAt)
	respondItem(w, http.StatusOK, server.memberWithLink(id, link))
}

// inTransaction — `work` dans une transaction, validée s'il réussit.
func (server *Server) inTransaction(work func(*sql.Tx) (memberLinkResponse, error)) (memberLinkResponse, error) {
	transaction, err := server.db.Begin()
	if err != nil {
		return memberLinkResponse{}, err
	}
	defer transaction.Rollback()

	link, err := work(transaction)
	if err != nil {
		return memberLinkResponse{}, err
	}

	return link, transaction.Commit()
}

// ErrLinkRefused — un lien de compte inconnu, servi ou échu : 410.
var ErrLinkRefused = errors.New("member link refused")

// openedLink — un lien de compte, tel que sa lecture le trouve.
type openedLink struct {
	ID          int64
	Kind        string
	UserID      int64
	Username    string
	DisplayName string
	ExpiresAt   string
}

// openLink — le lien de ce code, freiné par adresse comme la connexion : un code
// inconnu, servi ou échu compte comme un échec (ErrLinkRefused), trop d'échecs
// font attendre (ThrottledError). Rien n'est consommé ici.
func (server *Server) openLink(code, address string) (openedLink, error) {
	limiterKey := "join\x00" + address
	if wait := server.logins.wait(limiterKey, address); wait > 0 {
		slog.Warn("join delayed", "from", address, "wait", wait.Round(time.Second))
		return openedLink{}, ThrottledError{Wait: wait}
	}
	refuse := func(cause string) (openedLink, error) {
		server.logins.fail(limiterKey, address)
		slog.Warn("join refused", "from", address, "reason", cause)

		return openedLink{}, ErrLinkRefused
	}

	code = strings.TrimSpace(code)
	if code == "" {
		return refuse("no code")
	}
	var link openedLink
	err := server.db.QueryRow(
		`SELECT i.id, i.kind, i.user_id, i.expires_at, u.username, u.display_name
		 FROM invitations i JOIN users u ON u.id = i.user_id
		 WHERE i.code_hash = ? AND i.kind IN `+memberLinkKinds, hashToken(code),
	).Scan(&link.ID, &link.Kind, &link.UserID, &link.ExpiresAt, &link.Username, &link.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse("unknown or used")
	}
	if err != nil {
		return openedLink{}, err
	}
	if link.ExpiresAt <= invitationClock() {
		_, _ = server.db.Exec(`DELETE FROM invitations WHERE id = ?`, link.ID)
		return refuse("expired")
	}

	return link, nil
}

// respondLinkError — les refus d'un lien, dits à la page.
func respondLinkError(w http.ResponseWriter, err error) {
	var throttled ThrottledError
	switch {
	case errors.As(err, &throttled):
		minutes := int((throttled.Wait + time.Minute - 1) / time.Minute)
		w.Header().Set("Retry-After", strconv.Itoa(int(throttled.Wait.Seconds())+1))
		respondErrorCode(w, http.StatusTooManyRequests, "too_many_attempts", "Too many failed attempts. Try again in "+strconv.Itoa(minutes)+" minute(s).")
	case errors.Is(err, ErrLinkRefused):
		respondErrorCode(w, http.StatusGone, "link_dead", "This link was already used or has expired. Ask an administrator for a new one.")
	default:
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
	}
}

// handleCheckJoin — POST /api/v1/auth/join/check {code}, sans jeton : à qui est
// ce lien, de quelle sorte, jusqu'à quand. Ne consomme rien.
func (server *Server) handleCheckJoin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code string `json:"code"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	link, err := server.openLink(body.Code, claimedClientAddress(r))
	if err != nil {
		respondLinkError(w, err)
		return
	}
	respondItem(w, http.StatusOK, map[string]any{
		"username": link.Username, "display_name": link.DisplayName, "kind": link.Kind, "expires_at": link.ExpiresAt,
	})
}

// handleJoin — POST /api/v1/auth/join {code, password}, sans jeton : le mot de
// passe choisi (la règle, 422), posé avec sa date ; le lien consommé ; la
// personne connectée — la réponse est celle de la connexion.
//
// LA CONSOMMATION EST LE DELETE, comme au chantier 33 : huit envois simultanés
// lisent le même lien, un seul le supprime et pose son mot de passe, les autres
// reçoivent 410. bcrypt se calcule AVANT la transaction : il dure, et la base
// n'a qu'une connexion.
func (server *Server) handleJoin(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Code     string `json:"code"`
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	address := claimedClientAddress(r)
	link, err := server.openLink(body.Code, address)
	if err != nil {
		respondLinkError(w, err)
		return
	}
	if refusals := auth.PasswordRefusals(body.Password); len(refusals) > 0 {
		slog.Info("join refused", "username", link.Username, "from", address, "reason", "weak password", "codes", refusals)
		respondPasswordRefused(w, refusals)
		return
	}
	hashed, err := auth.HashPassword(body.Password)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}

	transaction, err := server.db.Begin()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	defer transaction.Rollback()
	result, err := transaction.Exec(`DELETE FROM invitations WHERE id = ?`, link.ID)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		// Un autre envoi l'a pris entre la lecture et ici, ou un nouveau lien
		// l'a remplacé.
		slog.Warn("join refused", "username", link.Username, "from", address, "reason", "used meanwhile")
		respondLinkError(w, ErrLinkRefused)
		return
	}
	if _, err := transaction.Exec(`UPDATE users SET hashed_password = ?, password_set_at = ? WHERE id = ?`, hashed, now(), link.UserID); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if err := transaction.Commit(); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	server.logins.succeed("join\x00"+address, address)

	// Son compte attendait : la liste des kernels le dit rejoint.
	server.accountsChanged()
	slog.Info("invitation joined", "username", link.Username, "kind", link.Kind, "from", address)
	// LA SESSION, OU LA DOUBLE AUTHENTIFICATION D'ABORD (chantier 80) : imposée, il l'active avant d'entrer.
	server.finishSignIn(w, r, link.UserID, link.Username, "invitation")
}
