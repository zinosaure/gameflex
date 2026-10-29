// Package auth validates the user JWTs issued by the auth backend (same
// claims as python/src/app/users/routes.py). Validation is LOCAL and
// stateless: an auth service outage must never block an already logged-in
// player from launching a session.
//
// EdDSA DEPUIS LE 2026-10-28 (chantier 34, P10) : le kernel vérifie avec la
// clé PUBLIQUE de l'installation, qu'il tire du même secret que le service
// d'identité (gameflex/auth.KeyPair). Le paramètre `secret` des fonctions
// ci-dessous reste ce secret : le kernel le détient, ses parties non — elles
// reçoivent la clé publique (internal/docker).
package auth

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"net/http"
	"strings"
	"sync"

	sharedauth "gameflex/auth"

	"github.com/golang-jwt/jwt/v5"
)

type Claims struct {
	Username string `json:"username"`
	// RoleLevel — le rang, tel que le service d'identité l'a signé. Plus
	// petit = plus de droits (0 root, 1 admin), la même échelle que
	// credential/internal/auth.
	//
	// Lu ici pour les réglages de la MACHINE — la taille du cache disque —
	// que tout le monde ne doit pas pouvoir changer. Pour tout ce qui touche
	// à un COMPTE, c'est le service d'identité qui tranche, sur la ligne en
	// base et non sur une claim : un jeton reste valide après une
	// rétrogradation.
	RoleLevel int `json:"role_level"`
	// Fed — la session a été ouverte par le jeton du compte, depuis un autre
	// serveur (chantier 20) : la génération de ce jeton. Vide, une connexion
	// ordinaire.
	Fed string `json:"fed,omitempty"`
	jwt.RegisteredClaims
}

// LA PORTE DES COMPTES (chantier 20). Un jeton valide ne suffit plus une fois
// que le kernel connaît les comptes (internal/accounts) : le compte doit
// exister encore, son rang est le sien et non celui du jeton, et une session
// ouverte par jeton suit son jeton. Sans porte posée — ou avant que la liste
// des comptes n'arrive — tout jeton valide passe, comme avant : la promesse
// en tête de ce paquet tient.
//
// `subject` est l'identifiant du compte que le jeton nomme (`sub`) : un compte
// supprimé puis recréé sous le même nom n'hérite pas des jetons de l'ancien
// (revue de sécurité du kernel, #601).
var gate func(username, subject, fed string, claimedRole int) (int, bool)

// SetGate — la porte, posée au démarrage par le serveur.
func SetGate(admit func(username, subject, fed string, claimedRole int) (int, bool)) {
	gate = admit
}

// Admit — ce jeton vérifié passe-t-il la porte ? Rend ses droits, rang à jour.
func Admit(claims Claims) (Claims, bool) {
	if gate == nil {
		return claims, true
	}
	role, ok := gate(claims.Username, claims.Subject, claims.Fed, claims.RoleLevel)
	if !ok {
		return Claims{}, false
	}
	claims.RoleLevel = role

	return claims, true
}

type usernameKey struct{}
type roleKey struct{}
type federatedKey struct{}

// RoleAdmin — le rang au-delà duquel on n'administre plus rien.
const RoleAdmin = 1

// publicKeys — la clé publique de chaque secret déjà vu : la tirer coûte une
// dérivation Ed25519, et chaque requête en demande une.
var publicKeys sync.Map

// PublicKey — la clé qui vérifie les jetons de l'installation dont c'est le
// secret ; celle que le kernel remet à ses parties.
func PublicKey(secret string) (ed25519.PublicKey, error) {
	if key, known := publicKeys.Load(secret); known {
		return key.(ed25519.PublicKey), nil
	}
	_, public, err := sharedauth.KeyPair(secret)
	if err != nil {
		return nil, err
	}
	publicKeys.Store(secret, public)

	return public, nil
}

// ParseToken validates an EdDSA JWT and returns its claims. The username
// claim is required — sessions are always owned by a user — and so is the
// expiry: every token GameFlex issues carries one.
func ParseToken(secret, tokenString string) (Claims, error) {
	var claims Claims

	public, err := PublicKey(secret)
	if err != nil {
		return Claims{}, err
	}
	parsed, err := jwt.ParseWithClaims(tokenString, &claims, func(token *jwt.Token) (any, error) {
		return public, nil
	}, jwt.WithValidMethods([]string{"EdDSA"}), jwt.WithExpirationRequired())
	if err != nil {
		return Claims{}, err
	}
	if !parsed.Valid || claims.Username == "" {
		return Claims{}, jwt.ErrTokenInvalidClaims
	}

	return claims, nil
}

// BearerToken extracts the JWT from Authorization: Bearer or from the
// ?token= query (WebSocket handshakes cannot carry headers — same pattern
// as the presence WebSocket already uses).
func BearerToken(r *http.Request) string {
	if header := r.Header.Get("Authorization"); header != "" {
		return strings.TrimPrefix(header, "Bearer ")
	}

	return r.URL.Query().Get("token")
}

// Middleware guards a route with local JWT validation and stashes the
// username claim in the request context.
func Middleware(secret string, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		tokenString := BearerToken(r)
		if tokenString == "" {
			writeUnauthorized(w)
			return
		}

		claims, err := ParseToken(secret, tokenString)
		if err != nil {
			writeUnauthorized(w)
			return
		}
		claims, admitted := Admit(claims)
		if !admitted {
			writeUnauthorized(w)
			return
		}

		ctx := context.WithValue(r.Context(), usernameKey{}, claims.Username)
		ctx = context.WithValue(ctx, roleKey{}, claims.RoleLevel)
		ctx = context.WithValue(ctx, federatedKey{}, claims.Fed != "")
		next(w, r.WithContext(ctx))
	}
}

// IsAdmin — l'appelant peut-il changer un réglage de la machine ?
//
// Absent du contexte (une route non gardée) : non. Refuser par défaut est la
// seule valeur sûre pour une question de droits.
func IsAdmin(ctx context.Context) bool {
	level, present := ctx.Value(roleKey{}).(int)

	return present && level <= RoleAdmin
}

// Federated — la session de l'appelant a-t-elle été ouverte par le jeton de son
// compte, depuis un autre serveur (chantier 20) ? Elle joue ici, et rien
// d'autre : elle ne confie pas à ce serveur ses jetons chez d'autres.
func Federated(ctx context.Context) bool {
	federated, _ := ctx.Value(federatedKey{}).(bool)

	return federated
}

// Username returns the authenticated username from the request context.
func Username(ctx context.Context) string {
	username, _ := ctx.Value(usernameKey{}).(string)
	return username
}

func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{"success": false, "errorMessage": "invalid or missing token"})
}
