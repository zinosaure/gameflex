package api

// LES INVITATIONS — chantier 33 (le porteur, 2026-10-26 : « Ce matin j'ai dû
// partager les tokens par WhatsApp à mon ami »).
//
// Ce qui voyage par une messagerie n'est plus le jeton, mais une INVITATION :
// un code de 128 bits, à usage unique, valable 12 heures, dans un lien
// `<ce serveur>/invite/<code>` que la page montre aussi en QR. Le serveur d'en
// face l'échange auprès du nôtre, de serveur à serveur (POST
// /api/v1/federation/redeem du kernel, puis /internal/federation/redeem ici),
// et reçoit le vrai jeton :
//
//   - federation : un jeton de catalogue, créé à l'échange avec le nom et les
//     portées choisis à l'invitation — le lien entre deux serveurs ;
//   - account : le jeton du compte qui l'a créée, pour jouer ici depuis là-bas
//     (account_tokens.go), tant qu'un administrateur le laisse actif.
//
// Le jeton durable ne passe donc jamais par une messagerie. Une invitation
// servie ou échue ne vaut plus rien ; si quelqu'un d'autre la prend le
// premier, la liste des jetons le montre et se révoque. Le système d'avant
// (montrer le jeton, le coller) reste, en repli.
//
// Seule l'empreinte du code est gardée, comme pour les jetons de catalogue.

import (
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
)

const (
	// InvitationFederation — devient un jeton de catalogue à l'échange.
	InvitationFederation = "federation"
	// InvitationAccount — rend le jeton du compte qui l'a créée.
	InvitationAccount = "account"

	// invitationLifetime — « On peut mettre la durée de 12 heure. Si après ça
	// pas d'utilisation alors il doit en redemander » (le porteur).
	invitationLifetime = 12 * time.Hour
	// invitationCodeBytes — 128 bits d'aléa : rien à deviner.
	invitationCodeBytes = 16
)

var (
	// ErrInvitationRefused — code inconnu, déjà servi ou échu.
	ErrInvitationRefused = errors.New("invitation refused")
	// ErrInvitationKind — une invitation de l'autre sorte : rien n'est
	// consommé, elle se colle ailleurs.
	ErrInvitationKind = errors.New("invitation of another kind")
	// ErrInvitationTokenOff — le jeton du compte n'est pas actif (ou échu) :
	// rien n'est consommé, l'administrateur peut le rallumer.
	ErrInvitationTokenOff = errors.New("account token disabled")
)

// ThrottledError — trop d'échecs depuis cette adresse : attendre Wait.
type ThrottledError struct{ Wait time.Duration }

func (e ThrottledError) Error() string { return "too many failed invitations" }

type invitationResponse struct {
	ID        int64  `json:"id"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
	Scopes    string `json:"scopes,omitempty"`
	ExpiresAt string `json:"expires_at"`
	CreatedAt any    `json:"created_at"`
	// Code n'est rempli qu'à la création : la base n'en a que l'empreinte.
	Code string `json:"code,omitempty"`
}

// Redeemed — ce qu'une invitation échangée rend au serveur d'en face.
type Redeemed struct {
	Kind  string `json:"kind"`
	Token string `json:"token"`
	// Identity — sous quel nom il est connu ici : le nom du jeton de
	// catalogue, ou le compte.
	Identity    string `json:"identity"`
	Scopes      string `json:"scopes,omitempty"`
	DisplayName string `json:"display_name,omitempty"`
}

// purgeInvitations — une invitation échue disparaît de la liste.
func (server *Server) purgeInvitations() {
	_, _ = server.db.Exec(`DELETE FROM invitations WHERE expires_at <= ?`, invitationClock())
}

// invitationClock — l'instant, au format de expires_at : RFC 3339 UTC à la
// seconde, une longueur fixe, donc comparable en texte.
func invitationClock() string {
	return time.Now().UTC().Format(time.RFC3339)
}

// insertInvitation — une ligne, et le code qui ne sortira qu'une fois.
func (server *Server) insertInvitation(kind, label, scopes string, remoteAdmin bool, userID any, by int64) (invitationResponse, error) {
	server.purgeInvitations()
	code := config.RandomToken(invitationCodeBytes)
	expiresAt := time.Now().Add(invitationLifetime).UTC().Format(time.RFC3339)
	result, err := server.db.Exec(
		`INSERT INTO invitations (kind, code_hash, label, scopes, remote_admin, user_id, expires_at, created_at, created_by_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		kind, hashToken(code), label, scopes, remoteAdmin, userID, expiresAt, now(), by,
	)
	if err != nil {
		return invitationResponse{}, err
	}
	id, _ := result.LastInsertId()

	return invitationResponse{ID: id, Kind: kind, Label: label, Scopes: scopes, ExpiresAt: expiresAt, Code: code}, nil
}

// handleCreateInvitation — POST /api/v1/token-invitations {kind, label, scopes}.
//
// federation : un administrateur, avec ce que demande un jeton de catalogue
// (un nom, des portées). account : le titulaire, pour son propre jeton, quand
// un administrateur l'a activé — la règle du jeton ne change pas.
func (server *Server) handleCreateInvitation(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	var body struct {
		Kind        string   `json:"kind"`
		Label       string   `json:"label"`
		Scopes      []string `json:"scopes"`
		RemoteAdmin bool     `json:"remote_admin"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	// L'ADMINISTRATION À DISTANCE (chantier 73) ne s'accorde jamais depuis une session venue d'ailleurs.
	if body.RemoteAdmin && refuseFederated(w, principal) {
		return
	}

	switch body.Kind {
	case InvitationFederation:
		if !principal.IsAdmin() {
			respondErrorCode(w, http.StatusForbidden, "admin_only", "Admin access required.")
			return
		}
		label := strings.TrimSpace(body.Label)
		if label == "" {
			respondValidationError(w, []map[string]any{{
				"loc": []string{"body", "label"}, "msg": "A name is required.", "type": "value_error.missing",
			}})
			return
		}
		scopes, err := parseScopes(body.Scopes)
		if err != nil {
			respondValidationError(w, []map[string]any{{
				"loc": []string{"body", "scopes"}, "msg": err.Error(), "type": "value_error",
			}})
			return
		}
		created, err := server.insertInvitation(InvitationFederation, label, scopes, body.RemoteAdmin, nil, principal.ID)
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		respondItem(w, http.StatusCreated, created)

	case InvitationAccount:
		if refuseFederated(w, principal) {
			return
		}
		token, err := server.accountToken(principal.ID, false)
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		if !token.Enabled {
			respondErrorCode(w, http.StatusForbidden, "token_not_enabled", "Your token is not enabled: ask an administrator.")
			return
		}
		label := principal.DisplayName
		if label == "" {
			label = principal.Username
		}
		created, err := server.insertInvitation(InvitationAccount, label, "", false, principal.ID, principal.ID)
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		respondItem(w, http.StatusCreated, created)

	default:
		respondErrorCode(w, http.StatusBadRequest, "bad_request", "kind must be federation or account")
	}
}

// handleListInvitations — GET /api/v1/token-invitations : les invitations en
// attente, des deux sortes — pour qui, jusqu'à quand. Jamais le code.
func (server *Server) handleListInvitations(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	server.purgeInvitations()
	// Ses deux sortes : les liens des comptes (member_links.go) ne sont pas des
	// invitations de fédération. Ni les codes d'appairage (pairing.go) : ils
	// vivent deux minutes, et leur fenêtre est le seul endroit où on les voit.
	rows, err := server.db.Query(
		`SELECT id, kind, label, scopes, expires_at, created_at FROM invitations WHERE kind IN (?, ?) AND pairing = 0 ORDER BY id DESC`,
		InvitationFederation, InvitationAccount)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	defer rows.Close()

	items := []invitationResponse{}
	for rows.Next() {
		var item invitationResponse
		var createdAt sql.NullString
		if err := rows.Scan(&item.ID, &item.Kind, &item.Label, &item.Scopes, &item.ExpiresAt, &createdAt); err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
			return
		}
		item.CreatedAt = isoDatetime(createdAt)
		items = append(items, item)
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

// handleRevokeInvitation — DELETE /api/v1/token-invitations/{id}.
func (server *Server) handleRevokeInvitation(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	result, err := server.db.Exec(`DELETE FROM invitations WHERE id = ? AND kind IN (?, ?) AND pairing = 0`, r.PathValue("id"), InvitationFederation, InvitationAccount)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondErrorCode(w, http.StatusNotFound, "not_found", "Invitation not found.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRedeemInvitation — POST /internal/federation/redeem {code, kind, from,
// address}. Appelée par le kernel, qui relaie le serveur d'en face ; le
// téléphone passe par RedeemInvitation directement (embed). `from` est le nom
// du serveur qui échange : la fenêtre d'un code d'appairage le montre.
func (server *Server) handleRedeemInvitation(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	var body struct {
		Code    string `json:"code"`
		Kind    string `json:"kind"`
		From    string `json:"from"`
		Address string `json:"address"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	redeemed, err := server.RedeemInvitation(body.Code, body.Kind, body.From, body.Address)
	var throttled ThrottledError
	switch {
	case errors.As(err, &throttled):
		w.Header().Set("Retry-After", strconv.Itoa(int(throttled.Wait.Seconds())+1))
		respondErrorCode(w, http.StatusTooManyRequests, "invitationThrottled", "Too many failed invitations. Try again later.")
	case errors.Is(err, ErrPairingClosed):
		respondErrorCode(w, http.StatusLocked, "pairingClosed", "Discovery mode is off.")
	case errors.Is(err, ErrInvitationRefused):
		respondErrorCode(w, http.StatusUnauthorized, "invitationRefused", "This invitation was already used, has expired or does not exist.")
	case errors.Is(err, ErrInvitationKind):
		respondErrorCode(w, http.StatusConflict, "invitationWrongKind", "This invitation is of another kind.")
	case errors.Is(err, ErrInvitationTokenOff):
		respondErrorCode(w, http.StatusForbidden, "invitationTokenOff", "The token of this account is not enabled.")
	case err != nil:
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")
	default:
		respondItem(w, http.StatusOK, redeemed)
	}
}

// RedeemInvitation — échanger un code contre son jeton, une seule fois.
//
// LA CONSOMMATION EST LE DELETE : deux échanges simultanés lisent la même
// ligne, un seul la supprime, l'autre est refusé. Une invitation d'une autre
// sorte, ou celle d'un compte dont le jeton est éteint, n'est PAS consommée.
// Les échecs sont freinés par adresse, comme la connexion (#454). Exporté pour
// le téléphone (embed).
//
// SIX CHIFFRES, C'EST UN CODE D'APPAIRAGE (chantier 60, pairing.go) : il prend
// son propre chemin, sous son mode et ses deux compteurs. Une invitation en
// lien ne voit jamais une de ses lignes.
func (server *Server) RedeemInvitation(code, kind, from, address string) (Redeemed, error) {
	// SES DEUX SORTES SEULEMENT (chantier 55) : la sorte vient du serveur d'en
	// face, et le lien d'un compte (join, reset) ne s'échange jamais contre un
	// jeton — sans cette garde, un code `join` présenté sous sa propre sorte
	// serait devenu un jeton de catalogue. Rien n'est consommé.
	if kind != InvitationFederation && kind != InvitationAccount {
		return Redeemed{}, ErrInvitationKind
	}
	code = strings.TrimSpace(code)
	if pairingCodeForm.MatchString(code) {
		return server.redeemPairingCode(code, kind, from, address)
	}
	limiterKey := "invitation\x00" + address
	if wait := server.logins.wait(limiterKey, address); wait > 0 {
		slog.Warn("invitation delayed", "from", address, "wait", wait.Round(time.Second))
		return Redeemed{}, ThrottledError{Wait: wait}
	}
	refuse := func(err error) (Redeemed, error) {
		if errors.Is(err, ErrInvitationRefused) {
			server.logins.fail(limiterKey, address)
			slog.Warn("invitation refused", "kind", kind, "from", address)
		}
		return Redeemed{}, err
	}

	if code == "" {
		return refuse(ErrInvitationRefused)
	}
	var (
		id                                   int64
		storedKind, label, scopes, expiresAt string
		userID, createdBy                    sql.NullInt64
		remoteAdmin                          bool
	)
	err := server.db.QueryRow(
		`SELECT id, kind, label, scopes, remote_admin, user_id, expires_at, created_by_id FROM invitations WHERE code_hash = ? AND pairing = 0`,
		hashToken(code),
	).Scan(&id, &storedKind, &label, &scopes, &remoteAdmin, &userID, &expiresAt, &createdBy)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse(ErrInvitationRefused)
	}
	if err != nil {
		return Redeemed{}, err
	}
	if expiresAt <= invitationClock() {
		_, _ = server.db.Exec(`DELETE FROM invitations WHERE id = ?`, id)
		return refuse(ErrInvitationRefused)
	}
	if storedKind != kind {
		return Redeemed{}, ErrInvitationKind
	}

	var (
		account      TokenAccount
		accountToken string
	)
	if kind == InvitationAccount {
		account, accountToken, err = server.validAccount(`t.user_id = ?`, userID.Int64)
		if errors.Is(err, ErrTokenRefused) {
			return Redeemed{}, ErrInvitationTokenOff
		}
		if err != nil {
			return Redeemed{}, err
		}
	}

	result, err := server.db.Exec(`DELETE FROM invitations WHERE id = ?`, id)
	if err != nil {
		return Redeemed{}, err
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		// Un autre échange l'a prise entre la lecture et ici.
		return refuse(ErrInvitationRefused)
	}
	server.logins.succeed(limiterKey, address)

	return server.deliverInvitation(kind, label, scopes, remoteAdmin, createdBy, account, accountToken, address)
}

// deliverInvitation — ce qu'une invitation consommée rend : le jeton du compte,
// ou un jeton de catalogue né maintenant, sous le nom et les portées choisis à
// sa création. Commun au lien et au code d'appairage.
func (server *Server) deliverInvitation(kind, label, scopes string, remoteAdmin bool, createdBy sql.NullInt64, account TokenAccount, accountToken, address string) (Redeemed, error) {
	if kind == InvitationAccount {
		slog.Info("invitation redeemed", "kind", kind, "account", account.Username, "from", address)

		return Redeemed{Kind: kind, Token: accountToken, Identity: account.Username, DisplayName: account.DisplayName}, nil
	}

	secret := config.RandomToken(32)
	if _, err := server.db.Exec(
		`INSERT INTO federation_tokens (label, scopes, remote_admin, token_hash, prefix, created_at, created_by_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		label, scopes, remoteAdmin, hashToken(secret), secret[:tokenPrefixLength], now(), createdBy,
	); err != nil {
		return Redeemed{}, fmt.Errorf("create the catalogue token: %w", err)
	}
	// Un accès de plus dans la copie des kernels (chantier 66) : ses consoles y sont avant sa première partie.
	server.accountsChanged()
	slog.Info("invitation redeemed", "kind", kind, "label", label, "from", address)

	return Redeemed{Kind: kind, Token: secret, Identity: label, Scopes: scopes}, nil
}
