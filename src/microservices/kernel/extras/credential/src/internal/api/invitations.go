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
func (server *Server) insertInvitation(kind, label, scopes string, userID any, by int64) (invitationResponse, error) {
	server.purgeInvitations()
	code := config.RandomToken(invitationCodeBytes)
	expiresAt := time.Now().Add(invitationLifetime).UTC().Format(time.RFC3339)
	result, err := server.db.Exec(
		`INSERT INTO invitations (kind, code_hash, label, scopes, user_id, expires_at, created_at, created_by_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		kind, hashToken(code), label, scopes, userID, expiresAt, now(), by,
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
		Kind   string   `json:"kind"`
		Label  string   `json:"label"`
		Scopes []string `json:"scopes"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	switch body.Kind {
	case InvitationFederation:
		if !principal.IsAdmin() {
			respondError(w, http.StatusForbidden, "Admin access required.")
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
		created, err := server.insertInvitation(InvitationFederation, label, scopes, nil, principal.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		respondItem(w, http.StatusCreated, created)

	case InvitationAccount:
		if refuseFederated(w, principal) {
			return
		}
		token, err := server.accountToken(principal.ID, false)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		if !token.Enabled {
			respondError(w, http.StatusForbidden, "Your token is not enabled: ask an administrator.")
			return
		}
		label := principal.DisplayName
		if label == "" {
			label = principal.Username
		}
		created, err := server.insertInvitation(InvitationAccount, label, "", principal.ID, principal.ID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		respondItem(w, http.StatusCreated, created)

	default:
		respondError(w, http.StatusBadRequest, "kind must be federation or account")
	}
}

// handleListInvitations — GET /api/v1/token-invitations : les invitations en
// attente, des deux sortes — pour qui, jusqu'à quand. Jamais le code.
func (server *Server) handleListInvitations(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	server.purgeInvitations()
	rows, err := server.db.Query(
		`SELECT id, kind, label, scopes, expires_at, created_at FROM invitations ORDER BY id DESC`)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	defer rows.Close()

	items := []invitationResponse{}
	for rows.Next() {
		var item invitationResponse
		var createdAt sql.NullString
		if err := rows.Scan(&item.ID, &item.Kind, &item.Label, &item.Scopes, &item.ExpiresAt, &createdAt); err != nil {
			respondError(w, http.StatusInternalServerError, "Internal server error.")
			return
		}
		item.CreatedAt = isoDatetime(createdAt)
		items = append(items, item)
	}
	respondItems(w, http.StatusOK, items, newPagination(len(items), 1, allItems))
}

// handleRevokeInvitation — DELETE /api/v1/token-invitations/{id}.
func (server *Server) handleRevokeInvitation(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	result, err := server.db.Exec(`DELETE FROM invitations WHERE id = ?`, r.PathValue("id"))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Internal server error.")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		respondError(w, http.StatusNotFound, "Invitation not found.")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRedeemInvitation — POST /internal/federation/redeem {code, kind,
// address}. Appelée par le kernel, qui relaie le serveur d'en face ; le
// téléphone passe par RedeemInvitation directement (embed).
func (server *Server) handleRedeemInvitation(w http.ResponseWriter, r *http.Request) {
	if !server.internalCaller(w, r) {
		return
	}
	var body struct {
		Code    string `json:"code"`
		Kind    string `json:"kind"`
		Address string `json:"address"`
	}
	if !decodeBody(w, r, &body) {
		return
	}

	redeemed, err := server.RedeemInvitation(body.Code, body.Kind, body.Address)
	var throttled ThrottledError
	switch {
	case errors.As(err, &throttled):
		w.Header().Set("Retry-After", strconv.Itoa(int(throttled.Wait.Seconds())+1))
		respondError(w, http.StatusTooManyRequests, "Too many failed invitations. Try again later.")
	case errors.Is(err, ErrInvitationRefused):
		respondError(w, http.StatusUnauthorized, "This invitation was already used, has expired or does not exist.")
	case errors.Is(err, ErrInvitationKind):
		respondError(w, http.StatusConflict, "This invitation is of another kind.")
	case errors.Is(err, ErrInvitationTokenOff):
		respondError(w, http.StatusForbidden, "The token of this account is not enabled.")
	case err != nil:
		respondError(w, http.StatusInternalServerError, "Internal server error.")
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
func (server *Server) RedeemInvitation(code, kind, address string) (Redeemed, error) {
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

	code = strings.TrimSpace(code)
	if code == "" {
		return refuse(ErrInvitationRefused)
	}
	var (
		id                                   int64
		storedKind, label, scopes, expiresAt string
		userID, createdBy                    sql.NullInt64
	)
	err := server.db.QueryRow(
		`SELECT id, kind, label, scopes, user_id, expires_at, created_by_id FROM invitations WHERE code_hash = ?`,
		hashToken(code),
	).Scan(&id, &storedKind, &label, &scopes, &userID, &expiresAt, &createdBy)
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

	if kind == InvitationAccount {
		slog.Info("invitation redeemed", "kind", kind, "account", account.Username, "from", address)

		return Redeemed{Kind: kind, Token: accountToken, Identity: account.Username, DisplayName: account.DisplayName}, nil
	}

	secret := config.RandomToken(32)
	if _, err := server.db.Exec(
		`INSERT INTO federation_tokens (label, scopes, token_hash, prefix, created_at, created_by_id)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		label, scopes, hashToken(secret), secret[:tokenPrefixLength], now(), createdBy,
	); err != nil {
		return Redeemed{}, fmt.Errorf("create the catalogue token: %w", err)
	}
	slog.Info("invitation redeemed", "kind", kind, "label", label, "from", address)

	return Redeemed{Kind: kind, Token: secret, Identity: label, Scopes: scopes}, nil
}
