// Package auth handles JWT issuing/validation and the bearer-token
// middleware. Tokens carry the claims the Python service issued ({sub,
// username, role_level, exp}).
//
// SIGNÉS EN Ed25519 DEPUIS LE 2026-10-28 (chantier 34, P10) : ce service est
// le seul à tenir la clé privée, tirée du secret de l'installation par
// gameflex/auth.KeyPair ; le kernel et les parties ne vérifient qu'avec la
// publique. En HS256, chaque conteneur de partie recevait le secret qui
// signe — de quoi se fabriquer un jeton root.
package auth

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	sharedauth "gameflex/auth"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

const (
	RoleRoot  = 0
	RoleAdmin = 1
	RoleUser  = 2
)

// TokenLifetime — la durée par défaut, celle du serveur.
//
// VARIABLE et non constante depuis le 2026-09-04 : l'hôte embarqué du
// téléphone la rallonge (voir embed.Options.TokenLifetime). Un serveur a des
// gens qui se reconnectent ; un téléphone sur un réseau local a des invités
// qui rouvrent une page trois jours plus tard et n'ont aucune envie de
// retaper quoi que ce soit — et le refuser ne protège personne de plus,
// puisque le jeton ne quitte pas le réseau où il a été émis.
//
// SEPT JOURS SUR UN SERVEUR (le porteur, 2026-10-21) : « le 24h peut pas être
// 7 jours, il n'y a rien de donnée sensible sur le serveur pour pouvoir mettre
// autant de sécurité ». Un jour obligeait à se reconnecter chaque jour à un
// serveur fédéré depuis le téléphone (#411).
var TokenLifetime = 7 * 24 * time.Hour

type Claims struct {
	Username  string `json:"username"`
	RoleLevel int    `json:"role_level"`
	// Fed — la session a été ouverte par le jeton du compte, depuis un autre
	// serveur (chantier 20) : la génération de ce jeton (ses 8 premiers
	// caractères). Le kernel coupe la session quand le jeton est désactivé,
	// expiré ou régénéré. Vide : une connexion ordinaire.
	Fed string `json:"fed,omitempty"`
	jwt.RegisteredClaims
}

func CreateToken(secret string, userID int64, username string, roleLevel int) (string, error) {
	claims := Claims{
		Username:  username,
		RoleLevel: roleLevel,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(TokenLifetime)),
		},
	}
	return sign(secret, claims)
}

// sign — la clé privée de l'installation, tirée de son secret.
func sign(secret string, claims Claims) (string, error) {
	private, _, err := sharedauth.KeyPair(secret)
	if err != nil {
		return "", err
	}

	return jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims).SignedString(private)
}

// CreateFederatedToken — la session qu'un autre serveur ouvre ici avec le jeton
// d'un compte (chantier 20). Elle n'est JAMAIS administrateur, même si le compte
// l'est : le jeton se prête au serveur de son propriétaire, et tous les membres
// de ce serveur passent par lui. Ses consoles, elles, restent celles du compte.
// Elle ne dure pas plus que le jeton.
func CreateFederatedToken(secret string, userID int64, username, generation string, lifetime time.Duration) (string, error) {
	if lifetime > TokenLifetime {
		lifetime = TokenLifetime
	}
	claims := Claims{
		Username:  username,
		RoleLevel: RoleUser,
		Fed:       generation,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   fmt.Sprintf("%d", userID),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(lifetime)),
		},
	}
	return sign(secret, claims)
}

// ParseToken validates the token and returns its claims. The user row is
// NOT fetched here — role authority is the database, so callers re-fetch
// the user on every request (same contract as the Python service).
func ParseToken(secret, token string) (*Claims, error) {
	_, public, err := sharedauth.KeyPair(secret)
	if err != nil {
		return nil, err
	}
	claims := &Claims{}
	_, err = jwt.ParseWithClaims(token, claims, func(t *jwt.Token) (any, error) {
		if t.Method != jwt.SigningMethodEdDSA {
			return nil, errors.New("unexpected signing method")
		}
		return public, nil
	}, jwt.WithExpirationRequired())
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

func VerifyPassword(password, hashed string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hashed), []byte(password)) == nil
}

// BearerToken extracts the raw token from an Authorization header.
func BearerToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if !strings.HasPrefix(header, "Bearer ") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
}

// Principal is the authenticated user loaded from the database on every
// request — role_level comes from the row, never from the token's cached
// copy (a role change takes effect on the next request).
type Principal struct {
	ID          int64
	Username    string
	DisplayName string
	RoleLevel   int
	// Federated — la session a été ouverte par le jeton du compte, depuis un
	// autre serveur (chantier 20). Elle joue ; elle ne gère pas le compte.
	Federated bool
}

func (p Principal) IsAdmin() bool { return p.RoleLevel <= RoleAdmin }
func (p Principal) IsRoot() bool  { return p.RoleLevel == RoleRoot }

// LoadUser resolves a bearer token to the database row. Returns
// ErrMissingToken, ErrInvalidToken or ErrUserGone so handlers can map each
// to the exact 401 detail message the frontend expects.
var (
	ErrMissingToken = errors.New("missing bearer token")
	ErrInvalidToken = errors.New("invalid token")
	ErrUserGone     = errors.New("user no longer exists")
)

func LoadUser(db *sql.DB, secret string, r *http.Request) (Principal, error) {
	return LoadToken(db, secret, BearerToken(r))
}

// LoadToken — LoadUser, pour un jeton déjà en main : le téléphone juge ainsi
// ceux que son kernel reçoit (embed.RoleOf), avec les mêmes règles.
func LoadToken(db *sql.DB, secret string, token string) (Principal, error) {
	if token == "" {
		return Principal{}, ErrMissingToken
	}

	claims, err := ParseToken(secret, token)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}

	var principal Principal
	principal.ID, err = parseInt64(claims.Subject)
	if err != nil {
		return Principal{}, ErrInvalidToken
	}

	err = db.QueryRow(
		`SELECT id, username, display_name, role_level FROM users WHERE id = ?`,
		principal.ID,
	).Scan(&principal.ID, &principal.Username, &principal.DisplayName, &principal.RoleLevel)
	if errors.Is(err, sql.ErrNoRows) {
		return Principal{}, ErrUserGone
	}
	if err != nil {
		return Principal{}, fmt.Errorf("load user: %w", err)
	}
	// UNE SESSION OUVERTE PAR JETON SUIT SON JETON (chantier 20) : désactivé,
	// régénéré ou échu, il ne la porte plus — et elle n'est jamais
	// administrateur ici non plus. La page du serveur qui l'a ouverte apprend
	// ainsi (401) qu'il faut la redemander.
	if claims.Fed != "" {
		var (
			token     string
			enabled   int
			expiresAt sql.NullString
		)
		err := db.QueryRow(`SELECT token, enabled, expires_at FROM account_tokens WHERE user_id = ?`, principal.ID).
			Scan(&token, &enabled, &expiresAt)
		if err != nil || enabled != 1 || len(token) < 8 || token[:8] != claims.Fed || !stillValid(expiresAt) {
			return Principal{}, ErrInvalidToken
		}
		principal.RoleLevel = RoleUser
		principal.Federated = true
	}
	return principal, nil
}

// stillValid — l'échéance d'un jeton (UTC, RFC 3339) est-elle encore devant ?
func stillValid(expiresAt sql.NullString) bool {
	if !expiresAt.Valid {
		return false
	}
	until, err := time.Parse(time.RFC3339Nano, expiresAt.String)

	return err == nil && time.Now().Before(until)
}

func parseInt64(raw string) (int64, error) {
	var value int64
	_, err := fmt.Sscanf(raw, "%d", &value)
	return value, err
}
