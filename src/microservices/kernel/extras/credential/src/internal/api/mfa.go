package api

// LA DOUBLE AUTHENTIFICATION DES JOUEURS (chantier 80, `#809` ; le porteur, 2026-11-24 : « Le joueur décide s'il active
// le MFA ou pas mais l'admin dans membres il peut aussi l'imposer »). Les codes, la clé et le scellement :
// internal/mfa.
//
// LA CONNEXION EN DEUX TEMPS. Le bon mot de passe (handleLogin) ou le lien d'invitation (handleJoin) n'ouvrent plus
// toujours la session : `finishSignIn` décide.
//   - le compte a la double authentification : « code demandé » (`mfa: "code"`) et un jeton d'étape, pas de session ;
//   - elle lui est imposée et il ne l'a pas encore : « à activer » (`mfa: "enroll"`), le secret neuf et son adresse
//     `otpauth://`, pour le code QR ;
//   - sinon, la session, comme avant.
// Le second temps, POST /auth/login/mfa : le jeton d'étape et un code (ou un code de secours). Les essais ratés comptent
// dans la même limite que les mots de passe (login_limiter.go), et un jeton d'étape ne survit pas à cinq codes faux.
//
// LES REFUS : 401 au second temps de la connexion (comme un mot de passe faux) ; 400 sur les routes du titulaire
// (`password_wrong`, `mfa_refused`), comme « Mon mot de passe » — un 401 y fermerait sa session (edge, api.js).
//
// QUI EST CONCERNÉ (le porteur : « fédération n'est pas concerné. C'est seulement les joueurs qui présentent plus de
// risques ») : la connexion par mot de passe et le lien d'invitation. Pas les jetons de compte, ni l'appairage (un
// appareil déjà connecté en ouvre un autre), ni les sessions venues d'un serveur fédéré, ni le propriétaire du
// téléphone, qui s'ouvre tout seul.
//
// IMPOSÉE À TOUT LE SERVEUR (`settings.mfa_required_all`), en plus de l'être à un membre (`users.mfa_required`) : sur
// un serveur seulement, jamais au téléphone (le porteur : « ça doit être que sur le serveur »).

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"gameflex/credential/internal/auth"
	"gameflex/credential/internal/config"
	"gameflex/credential/internal/mfa"
)

const (
	mfaChallengeLifetime = 5 * time.Minute
	mfaChallengeAttempts = 5
	mfaRequiredAllKey    = "mfa_required_all"
)

// mfaChallenge — le premier temps d'une connexion, en mémoire : à qui il est, ce qu'il attend, jusqu'à quand. Pour une
// activation à la connexion, le secret neuf (pas encore rangé : il ne l'est qu'avec un premier code juste).
type mfaChallenge struct {
	userID   int64
	username string
	enroll   bool
	secret   string
	expires  time.Time
	attempts int
}

type mfaChallenges struct {
	mutex   sync.Mutex
	byToken map[string]*mfaChallenge
	// key — la clé de `keys/`, lue au premier besoin (internal/mfa).
	key []byte
}

func newMfaChallenges() *mfaChallenges {
	return &mfaChallenges{byToken: map[string]*mfaChallenge{}}
}

func (challenges *mfaChallenges) open(challenge *mfaChallenge) (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	challenge.expires = time.Now().Add(mfaChallengeLifetime)
	challenges.mutex.Lock()
	defer challenges.mutex.Unlock()
	now := time.Now()
	for other, entry := range challenges.byToken {
		if now.After(entry.expires) {
			delete(challenges.byToken, other)
		}
	}
	challenges.byToken[token] = challenge

	return token, nil
}

func (challenges *mfaChallenges) find(token string) *mfaChallenge {
	challenges.mutex.Lock()
	defer challenges.mutex.Unlock()
	challenge := challenges.byToken[token]
	if challenge == nil || time.Now().After(challenge.expires) {
		delete(challenges.byToken, token)

		return nil
	}

	return challenge
}

func (challenges *mfaChallenges) close(token string) {
	challenges.mutex.Lock()
	defer challenges.mutex.Unlock()
	delete(challenges.byToken, token)
}

// failed — un code faux de plus ; au cinquième, le jeton d'étape ne vaut plus (il faut redonner le mot de passe).
func (challenges *mfaChallenges) failed(token string) {
	challenges.mutex.Lock()
	defer challenges.mutex.Unlock()
	if challenge := challenges.byToken[token]; challenge != nil {
		challenge.attempts++
		if challenge.attempts >= mfaChallengeAttempts {
			delete(challenges.byToken, token)
		}
	}
}

func (server *Server) mfaKey() ([]byte, error) {
	server.mfa.mutex.Lock()
	defer server.mfa.mutex.Unlock()
	if server.mfa.key == nil {
		key, err := mfa.LoadKey(config.KeysDir)
		if err != nil {
			return nil, err
		}
		server.mfa.key = key
	}

	return server.mfa.key, nil
}

// mfaRecord — la double authentification d'un compte, telle qu'en base.
type mfaRecord struct {
	sealed   string
	enabled  bool
	lastStep int64
	recovery []string
}

func (server *Server) loadMfa(userID int64) (*mfaRecord, error) {
	var record mfaRecord
	var enabledAt sql.NullString
	var recovery string
	err := server.db.QueryRow(`SELECT secret, enabled_at, last_step, recovery FROM user_mfa WHERE user_id = ?`, userID).
		Scan(&record.sealed, &enabledAt, &record.lastStep, &recovery)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	record.enabled = enabledAt.Valid
	_ = json.Unmarshal([]byte(recovery), &record.recovery)

	return &record, nil
}

// openSecret — le secret d'un compte. Une clé effacée et refaite ne l'ouvre plus : son activation est perdue, elle se
// refait (mfa.go, la seule issue de root sans téléphone ni codes de secours).
func (server *Server) openSecret(userID int64, record *mfaRecord) (string, bool) {
	key, err := server.mfaKey()
	if err != nil {
		slog.Error("mfa: key unavailable", "error", err)

		return "", false
	}
	secret, err := mfa.Open(key, record.sealed)
	if err != nil {
		slog.Warn("mfa: the secret no longer opens (the key changed): the account enrolls again", "user_id", userID)
		_, _ = server.db.Exec(`DELETE FROM user_mfa WHERE user_id = ?`, userID)

		return "", false
	}

	return secret, true
}

// mfaPolicyAll — imposée à tout le serveur. Jamais au téléphone.
func (server *Server) mfaPolicyAll() bool {
	if server.config.Device {
		return false
	}
	var value string
	_ = server.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, mfaRequiredAllKey).Scan(&value)

	return value == "1"
}

// mfaRequirement — d'où vient l'obligation : "server", "member", ou "" (libre).
func (server *Server) mfaRequirement(userID int64) string {
	if server.mfaPolicyAll() {
		return "server"
	}
	var required bool
	_ = server.db.QueryRow(`SELECT mfa_required FROM users WHERE id = ?`, userID).Scan(&required)
	if required {
		return "member"
	}

	return ""
}

// newRecovery — des codes de secours neufs : les codes en clair (montrés une fois) et leurs empreintes (rangées).
func newRecovery() ([]string, string, error) {
	codes, err := mfa.NewRecoveryCodes(mfa.RecoveryCount)
	if err != nil {
		return nil, "", err
	}
	hashes := make([]string, len(codes))
	for index, code := range codes {
		if hashes[index], err = auth.HashPassword(mfa.NormalizeRecovery(code)); err != nil {
			return nil, "", err
		}
	}
	encoded, _ := json.Marshal(hashes)

	return codes, string(encoded), nil
}

// checkCode — un code de l'application, ou un code de secours (alors retiré : il ne sert qu'une fois).
func (server *Server) checkCode(userID int64, record *mfaRecord, secret, code string) bool {
	if step, ok := mfa.Verify(secret, code, time.Now(), record.lastStep); ok {
		_, _ = server.db.Exec(`UPDATE user_mfa SET last_step = ? WHERE user_id = ?`, step, userID)

		return true
	}
	normalized := mfa.NormalizeRecovery(code)
	if len(normalized) != 10 {
		return false
	}
	for index, hash := range record.recovery {
		if auth.VerifyPassword(normalized, hash) {
			rest := append(append([]string{}, record.recovery[:index]...), record.recovery[index+1:]...)
			encoded, _ := json.Marshal(rest)
			_, _ = server.db.Exec(`UPDATE user_mfa SET recovery = ? WHERE user_id = ?`, string(encoded), userID)
			slog.Info("mfa: recovery code used", "user_id", userID, "left", len(rest))

			return true
		}
	}

	return false
}

// finishSignIn — le mot de passe (ou le lien) est bon : la session, ou le second temps (voir l'en-tête).
func (server *Server) finishSignIn(w http.ResponseWriter, r *http.Request, userID int64, username, how string) {
	record, err := server.loadMfa(userID)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	if record != nil && record.enabled {
		if _, ok := server.openSecret(userID, record); ok {
			token, err := server.mfa.open(&mfaChallenge{userID: userID, username: username})
			if err != nil {
				respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

				return
			}
			slog.Info("sign-in waits for its code", "username", username, "how", how, "from", claimedClientAddress(r))
			respondItem(w, http.StatusOK, map[string]any{"mfa": "code", "mfa_token": token})

			return
		}
	}
	if server.mfaRequirement(userID) != "" {
		secret, err := mfa.NewSecret()
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

			return
		}
		token, err := server.mfa.open(&mfaChallenge{userID: userID, username: username, enroll: true, secret: secret})
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

			return
		}
		slog.Info("sign-in waits for its enrolment", "username", username, "how", how, "from", claimedClientAddress(r))
		respondItem(w, http.StatusOK, map[string]any{
			"mfa": "enroll", "mfa_token": token, "secret": secret, "uri": mfa.URI(mfaIssuer(r), username, secret),
		})

		return
	}
	server.grantSession(w, r, userID, username, how, nil)
}

// grantSession — la session ouverte : le jeton, la dernière connexion, la limite remise à zéro. `recovery` : les codes
// de secours d'une activation faite à la connexion, montrés une fois.
func (server *Server) grantSession(w http.ResponseWriter, r *http.Request, userID int64, username, how string, recovery []string) {
	token, err := server.SessionToken(userID)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	address := claimedClientAddress(r)
	_, _ = server.db.Exec(`UPDATE users SET last_login = CURRENT_TIMESTAMP WHERE id = ?`, userID)
	server.logins.succeed(username, address)
	slog.Info("login granted", "username", username, "how", how, "from", address, "origin", r.Header.Get("Origin"))
	answer := map[string]any{
		"access_token": token,
		"token_type":   "bearer",
		"user":         server.fetchUserResponse(userID),
	}
	if recovery != nil {
		answer["recovery_codes"] = recovery
	}
	respondItem(w, http.StatusOK, answer)
}

// POST /api/v1/auth/login/mfa — le second temps de la connexion.
func (server *Server) handleLoginMfa(w http.ResponseWriter, r *http.Request) {
	var body struct {
		MfaToken string `json:"mfa_token"`
		Code     string `json:"code"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	challenge := server.mfa.find(body.MfaToken)
	if challenge == nil {
		respondErrorCode(w, http.StatusUnauthorized, "mfa_expired", "This sign-in has expired. Enter your password again.")

		return
	}
	address := claimedClientAddress(r)
	if wait := server.logins.wait(challenge.username, address); wait > 0 {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		respondErrorCode(w, http.StatusTooManyRequests, "too_many_attempts", "Too many failed sign-ins. Try again later.")

		return
	}
	refuse := func(reason string) {
		server.logins.fail(challenge.username, address)
		server.mfa.failed(body.MfaToken)
		slog.Warn("login refused", "username", challenge.username, "reason", reason, "from", address)
		respondErrorCode(w, http.StatusUnauthorized, "mfa_refused", "Invalid code.")
	}

	if challenge.enroll {
		step, ok := mfa.Verify(challenge.secret, body.Code, time.Now(), 0)
		if !ok {
			refuse("wrong enrolment code")

			return
		}
		codes, err := server.storeEnabled(challenge.userID, challenge.secret, step)
		if err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

			return
		}
		server.mfa.close(body.MfaToken)
		slog.Info("mfa enabled at sign-in", "username", challenge.username)
		server.grantSession(w, r, challenge.userID, challenge.username, "password+mfa enrolment", codes)

		return
	}

	record, err := server.loadMfa(challenge.userID)
	if err != nil || record == nil || !record.enabled {
		server.mfa.close(body.MfaToken)
		respondErrorCode(w, http.StatusUnauthorized, "mfa_expired", "This sign-in has expired. Enter your password again.")

		return
	}
	secret, ok := server.openSecret(challenge.userID, record)
	if !ok || !server.checkCode(challenge.userID, record, secret, body.Code) {
		refuse("wrong code")

		return
	}
	server.mfa.close(body.MfaToken)
	server.grantSession(w, r, challenge.userID, challenge.username, "password+mfa", nil)
}

// storeEnabled — la double authentification activée : le secret scellé, le premier pas employé, des codes de secours.
func (server *Server) storeEnabled(userID int64, secret string, step int64) ([]string, error) {
	key, err := server.mfaKey()
	if err != nil {
		return nil, err
	}
	sealed, err := mfa.Seal(key, secret)
	if err != nil {
		return nil, err
	}
	codes, hashes, err := newRecovery()
	if err != nil {
		return nil, err
	}
	_, err = server.db.Exec(`INSERT INTO user_mfa (user_id, secret, enabled_at, last_step, recovery) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id) DO UPDATE SET secret = excluded.secret, enabled_at = excluded.enabled_at,
		last_step = excluded.last_step, recovery = excluded.recovery`, userID, sealed, now(), step, hashes)
	if err != nil {
		return nil, err
	}

	return codes, nil
}

// mfaOwnAccount — les gestes du titulaire sur sa double authentification : pas depuis une session venue d'un serveur
// fédéré (le compte est d'ici, ses gestes de sécurité aussi).
func mfaOwnAccount(w http.ResponseWriter, principal auth.Principal) bool {
	if principal.Federated {
		respondErrorCode(w, http.StatusForbidden, "federated_session", "Manage two-step verification on the account's own server.")

		return false
	}

	return true
}

// GET /api/v1/auth/mfa — l'état de sa double authentification.
func (server *Server) handleMfaState(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	record, err := server.loadMfa(principal.ID)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	enabled := record != nil && record.enabled
	left := 0
	if enabled {
		left = len(record.recovery)
	}
	requirement := server.mfaRequirement(principal.ID)
	respondItem(w, http.StatusOK, map[string]any{
		"enabled":       enabled,
		"required":      requirement != "",
		"required_by":   requirement,
		"recovery_left": left,
		"server_policy": !server.config.Device,
		"required_all":  server.mfaPolicyAll(),
	})
}

// POST /api/v1/auth/mfa/setup — commencer l'activation : le mot de passe, puis un secret neuf, pas encore actif.
func (server *Server) handleMfaSetup(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !mfaOwnAccount(w, principal) {
		return
	}
	var body struct {
		Password string `json:"password"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	if !server.verifyCurrentPassword(principal.ID, body.Password) {
		respondErrorCode(w, http.StatusBadRequest, "password_wrong", "Current password is incorrect.")

		return
	}
	if record, err := server.loadMfa(principal.ID); err == nil && record != nil && record.enabled {
		respondErrorCode(w, http.StatusConflict, "mfa_enabled", "Two-step verification is already on.")

		return
	}
	secret, err := mfa.NewSecret()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	key, err := server.mfaKey()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	sealed, err := mfa.Seal(key, secret)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	if _, err := server.db.Exec(`INSERT INTO user_mfa (user_id, secret) VALUES (?, ?)
		ON CONFLICT(user_id) DO UPDATE SET secret = excluded.secret, enabled_at = NULL, last_step = 0, recovery = '[]'`,
		principal.ID, sealed); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	respondItem(w, http.StatusOK, map[string]any{"secret": secret, "uri": mfa.URI(mfaIssuer(r), principal.Username, secret)})
}

// mfaIssuer — le nom sous lequel l'application d'authentification range le code : celui du serveur, que la page envoie
// (`?issuer=`, sa capacité `serverName` ; Réglages › Configuration du serveur, chantier 81). Le service des comptes ne le
// garde pas : c'est le kernel qui le tient. Sans lui, « GameFlex ».
func mfaIssuer(r *http.Request) string {
	issuer := strings.TrimSpace(r.URL.Query().Get("issuer"))
	if issuer == "" || utf8.RuneCountInString(issuer) > 40 {
		return "GameFlex"
	}

	return issuer
}

// POST /api/v1/auth/mfa/enable — le premier code juste active la double authentification ; les codes de secours,
// montrés une fois.
func (server *Server) handleMfaEnable(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !mfaOwnAccount(w, principal) {
		return
	}
	var body struct {
		Code string `json:"code"`
	}
	if !decodeBody(w, r, &body) {
		return
	}
	record, err := server.loadMfa(principal.ID)
	if err != nil || record == nil || record.enabled {
		respondErrorCode(w, http.StatusConflict, "mfa_not_started", "Start the activation again.")

		return
	}
	secret, ok := server.openSecret(principal.ID, record)
	if !ok {
		respondErrorCode(w, http.StatusConflict, "mfa_not_started", "Start the activation again.")

		return
	}
	step, ok := mfa.Verify(secret, body.Code, time.Now(), 0)
	if !ok {
		respondErrorCode(w, http.StatusBadRequest, "mfa_refused", "Invalid code.")

		return
	}
	codes, err := server.storeEnabled(principal.ID, secret, step)
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	slog.Info("mfa enabled", "username", principal.Username)
	respondItem(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// mfaProof — le mot de passe et un code (ou un code de secours) : ce que demandent la désactivation et les codes neufs.
func (server *Server) mfaProof(w http.ResponseWriter, r *http.Request, principal auth.Principal) bool {
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decodeBody(w, r, &body) {
		return false
	}
	if !server.verifyCurrentPassword(principal.ID, body.Password) {
		respondErrorCode(w, http.StatusBadRequest, "password_wrong", "Current password is incorrect.")

		return false
	}
	record, err := server.loadMfa(principal.ID)
	if err != nil || record == nil || !record.enabled {
		respondErrorCode(w, http.StatusConflict, "mfa_disabled", "Two-step verification is off.")

		return false
	}
	secret, ok := server.openSecret(principal.ID, record)
	if !ok || !server.checkCode(principal.ID, record, secret, body.Code) {
		respondErrorCode(w, http.StatusBadRequest, "mfa_refused", "Invalid code.")

		return false
	}

	return true
}

// POST /api/v1/auth/mfa/disable — la désactiver ; impossible quand elle est imposée.
func (server *Server) handleMfaDisable(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !mfaOwnAccount(w, principal) {
		return
	}
	if server.mfaRequirement(principal.ID) != "" {
		respondErrorCode(w, http.StatusForbidden, "mfa_required", "Two-step verification is required on this account.")

		return
	}
	if !server.mfaProof(w, r, principal) {
		return
	}
	if _, err := server.db.Exec(`DELETE FROM user_mfa WHERE user_id = ?`, principal.ID); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	slog.Info("mfa disabled", "username", principal.Username)
	respondItem(w, http.StatusOK, map[string]any{"enabled": false})
}

// POST /api/v1/auth/mfa/recovery — des codes de secours neufs ; les anciens ne valent plus rien.
func (server *Server) handleMfaRecovery(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if !mfaOwnAccount(w, principal) {
		return
	}
	if !server.mfaProof(w, r, principal) {
		return
	}
	codes, hashes, err := newRecovery()
	if err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	if _, err := server.db.Exec(`UPDATE user_mfa SET recovery = ? WHERE user_id = ?`, hashes, principal.ID); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	respondItem(w, http.StatusOK, map[string]any{"recovery_codes": codes})
}

// POST /api/v1/users/{id}/mfa/reset — l'administrateur efface la double authentification d'un membre (téléphone et codes
// perdus) ; imposée, elle se réactive à sa prochaine connexion.
func (server *Server) handleMfaReset(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")

		return
	}
	var roleLevel int
	err = server.db.QueryRow(`SELECT role_level FROM users WHERE id = ?`, id).Scan(&roleLevel)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		respondErrorCode(w, http.StatusNotFound, "not_found", "User not found.")

		return
	case err != nil:
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	case roleLevel == auth.RoleRoot:
		respondErrorCode(w, http.StatusForbidden, "root_locked", "Cannot modify the root account.")

		return
	case id == principal.ID:
		respondErrorCode(w, http.StatusForbidden, "own_account", "Turn your own two-step verification off from your account.")

		return
	}
	if _, err := server.db.Exec(`DELETE FROM user_mfa WHERE user_id = ?`, id); err != nil {
		respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

		return
	}
	slog.Info("mfa reset by an administrator", "user_id", id, "by", principal.Username)
	respondItem(w, http.StatusOK, server.memberResponse(id))
}

// GET et PUT /api/v1/auth/mfa/policy — imposée à tout le serveur. Un serveur seulement.
func (server *Server) handleMfaPolicy(w http.ResponseWriter, r *http.Request, principal auth.Principal) {
	if server.config.Device {
		respondErrorCode(w, http.StatusNotFound, "server_only", "Only a server has this setting.")

		return
	}
	if r.Method == http.MethodPut {
		var body struct {
			RequiredAll bool `json:"required_all"`
		}
		if !decodeBody(w, r, &body) {
			return
		}
		value := "0"
		if body.RequiredAll {
			value = "1"
		}
		if _, err := server.db.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
			mfaRequiredAllKey, value); err != nil {
			respondErrorCode(w, http.StatusInternalServerError, "server_failed", "Internal server error.")

			return
		}
		slog.Info("mfa policy changed", "required_all", body.RequiredAll, "by", principal.Username)
	}
	respondItem(w, http.StatusOK, map[string]any{"required_all": server.mfaPolicyAll()})
}
