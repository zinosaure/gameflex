// Package auth signs and verifies the JWTs this app issues, for the services
// that must authenticate a caller THEMSELVES.
//
// It exists because of chantier-4: once a runtime child owns its session, it
// is the child that decides who may play — and the party that decides is the
// party that must authenticate. Handing an already-verified identity down to
// it would put us back where retro_relay.go was on 2026-08-23, where a
// self-declared username granted full control of anybody's session.
//
// Deliberately stdlib-only and deliberately small: three runtimes need it, and
// a shared hundred lines is better than three drifting copies.
//
// Ed25519 (`alg: EdDSA`), PLUS HS256, DEPUIS LE 2026-10-28 (chantier 34,
// P10). Un secret HMAC qui vérifie un jeton sait aussi en signer un : chaque
// conteneur de partie recevait donc de quoi fabriquer un jeton root, et une
// exécution de code dans un émulateur suffisait (revue de sécurité du kernel,
// #598). La clé privée ne quitte plus le service d'identité ; les parties ne
// reçoivent que la publique. La paire se DÉRIVE du secret de l'installation
// (KeyPair) : rien de nouveau à régler pour installer GameFlex.
package auth

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"time"
)

var (
	ErrMalformed = errors.New("malformed token")
	// ErrBadAlgorithm guards the classic JWT trap: a token that names its own
	// algorithm can name "none", or name HS256 so a public key gets used as
	// an HMAC secret. Only EdDSA is ever accepted here.
	ErrBadAlgorithm = errors.New("unexpected signing algorithm")
	ErrBadSignature = errors.New("bad signature")
	ErrExpired      = errors.New("token expired")
	// ErrNoExpiry — un jeton sans expiration vaudrait pour toujours : tous
	// ceux que GameFlex émet en portent une, un jeton qui n'en a pas n'est
	// pas des nôtres (revue de sécurité, 2026-10-28).
	ErrNoExpiry     = errors.New("token without expiry")
	ErrNoSecret     = errors.New("no signing secret configured")
	ErrNoKey        = errors.New("no verification key configured")
	ErrMalformedKey = errors.New("malformed public key")
)

// Algorithm — le seul que GameFlex signe et accepte.
const Algorithm = "EdDSA"

// Claims is the subset this app actually uses.
type Claims struct {
	Username  string `json:"username"`
	RoleLevel int    `json:"role_level"`
	Subject   string `json:"sub,omitempty"`
	ExpiresAt int64  `json:"exp"`
	// Fed — la session a été ouverte par le jeton du compte, depuis un autre
	// serveur (chantier 20) : la génération de ce jeton. Vide, une connexion
	// ordinaire.
	Fed string `json:"fed,omitempty"`
}

// KeyPair — la paire qui signe et vérifie les jetons d'une installation,
// tirée de son secret. Le même secret donne toujours la même paire : le
// service d'identité et le kernel la retrouvent chacun de leur côté, sans
// fichier de clé à fabriquer, à ranger ni à transmettre.
//
// An empty secret is refused rather than turned into a key everyone can
// compute: a service started without its secret must fail closed.
func KeyPair(secret string) (ed25519.PrivateKey, ed25519.PublicKey, error) {
	if secret == "" {
		return nil, nil, ErrNoSecret
	}
	seed := hmac.New(sha256.New, []byte(secret))
	seed.Write([]byte("gameflex/auth: jwt ed25519 v1"))
	private := ed25519.NewKeyFromSeed(seed.Sum(nil))

	return private, private.Public().(ed25519.PublicKey), nil
}

// EncodePublicKey — la clé publique telle qu'elle voyage dans
// l'environnement d'un conteneur (GAMEFLEX_JWT_PUBLIC_KEY).
func EncodePublicKey(key ed25519.PublicKey) string {
	return base64.RawURLEncoding.EncodeToString(key)
}

// ParsePublicKey — l'inverse d'EncodePublicKey.
func ParsePublicKey(text string) (ed25519.PublicKey, error) {
	key, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(text))
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, ErrMalformedKey
	}

	return ed25519.PublicKey(key), nil
}

// PublicKeyFromEnv — la clé qu'un conteneur de partie reçoit du kernel
// (GAMEFLEX_JWT_PUBLIC_KEY). Nil quand elle manque ou ne se lit pas : Verify
// refuse alors tout jeton, et la partie est fermée, pas ouverte.
func PublicKeyFromEnv() ed25519.PublicKey {
	key, err := ParsePublicKey(os.Getenv("GAMEFLEX_JWT_PUBLIC_KEY"))
	if err != nil {
		return nil
	}

	return key
}

// Sign — un jeton EdDSA pour ces claims.
func Sign(key ed25519.PrivateKey, claims Claims) (string, error) {
	if len(key) != ed25519.PrivateKeySize {
		return "", ErrNoSecret
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	signed := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"`+Algorithm+`","typ":"JWT"}`)) +
		"." + base64.RawURLEncoding.EncodeToString(payload)

	return signed + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(signed))), nil
}

// Verify checks an EdDSA token and returns its claims.
//
// A missing key is refused rather than treated as "no check": a service
// started without its key must fail closed, not open. That exact mistake —
// a missing secret silently disabling validation — is why every call site
// here is explicit about it.
func Verify(key ed25519.PublicKey, token string, now time.Time) (Claims, error) {
	if len(key) != ed25519.PublicKeySize {
		return Claims{}, ErrNoKey
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return Claims{}, ErrMalformed
	}

	var header struct {
		Algorithm string `json:"alg"`
	}
	headerJSON, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(headerJSON, &header) != nil {
		return Claims{}, ErrMalformed
	}
	if header.Algorithm != Algorithm {
		return Claims{}, ErrBadAlgorithm
	}

	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return Claims{}, ErrMalformed
	}
	if !ed25519.Verify(key, []byte(parts[0]+"."+parts[1]), signature) {
		return Claims{}, ErrBadSignature
	}

	payloadJSON, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return Claims{}, ErrMalformed
	}

	var claims Claims
	if json.Unmarshal(payloadJSON, &claims) != nil {
		return Claims{}, ErrMalformed
	}
	if claims.ExpiresAt <= 0 {
		return Claims{}, ErrNoExpiry
	}
	if now.Unix() >= claims.ExpiresAt {
		return Claims{}, ErrExpired
	}
	if claims.Username == "" {
		return Claims{}, ErrMalformed
	}

	return claims, nil
}

// SessionSecret — le secret interne d'UNE partie (chantier 34, P10).
//
// Un conteneur de partie recevait le secret interne de l'installation : de
// quoi appeler les routes /internal/* de tous les services, pour toutes les
// parties. Il reçoit maintenant celui-ci, que le kernel et la passerelle
// recalculent à partir du secret de l'installation et de l'identifiant de la
// partie : il n'ouvre que les routes de SA partie.
func SessionSecret(installSecret, sessionID string) string {
	if installSecret == "" || sessionID == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(installSecret))
	mac.Write([]byte("gameflex/auth: session secret v1\x00" + sessionID))

	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

// SameSecret — comparer un secret présenté au secret attendu, en temps
// constant, et jamais vrai pour un secret attendu vide : un service démarré
// sans secret refuse tout, il n'ouvre pas tout.
func SameSecret(presented, expected string) bool {
	return expected != "" && subtle.ConstantTimeCompare([]byte(presented), []byte(expected)) == 1
}
