// Package secretbox chiffre les identifiants que ce service garde pour d'autres
// serveurs : le mot de passe d'un partage Samba ou WebDAV, le jeton d'un kDrive.
//
// AVEC LE SECRET DE GAMEFLEX (#459, le porteur, 2026-10-24 : « tu peux chiffrer
// le mot de passe des réseaux avec le GameFlex secret, c'est fait exprès
// d'avoir ça »). Ils étaient en clair dans la base, alors que l'écran disait
// « Stored encrypted » : une copie de `databases/` les livrait.
//
// AES-256-GCM, la clé tirée par SHA-256, sous une étiquette propre à cet usage,
// du secret qui signe les jetons (GAMEFLEX_SECRET sur un serveur, le secret
// de l'appareil sur le téléphone) : une clé dérivée ne sert qu'à une chose.
// Changer le secret rend ces identifiants illisibles : il faut alors les
// saisir de nouveau, comme on se reconnecte.
package secretbox

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

// prefix marque une valeur scellée. Une valeur sans lui n'est pas lue : seule
// la migration du démarrage scelle ce qui ne l'était pas encore (cmd/auth).
const prefix = "enc1:"

// ErrNoSecret — pas de secret, rien ne se scelle ni ne s'ouvre.
var ErrNoSecret = errors.New("the GameFlex secret is not set: share credentials cannot be stored")

func aead(secret string) (cipher.AEAD, error) {
	if secret == "" {
		return nil, ErrNoSecret
	}
	key := sha256.Sum256([]byte("gameflex/remote-mounts\x00" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

// Seal — la valeur à ranger en base. Vide reste vide : un partage invité n'a
// pas de mot de passe.
func Seal(secret, plain string) (string, error) {
	if plain == "" {
		return "", nil
	}
	gcm, err := aead(secret)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	return prefix + base64.RawStdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plain), nil)), nil
}

// Open — la valeur en clair d'une valeur scellée.
func Open(secret, stored string) (string, error) {
	if stored == "" {
		return "", nil
	}
	if !Sealed(stored) {
		return "", errors.New("credential is not sealed")
	}
	gcm, err := aead(secret)
	if err != nil {
		return "", err
	}
	raw, err := base64.RawStdEncoding.DecodeString(strings.TrimPrefix(stored, prefix))
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", errors.New("credential is unreadable")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	if err != nil {
		return "", errors.New("credential cannot be opened: GAMEFLEX_SECRET changed since it was stored")
	}

	return string(plain), nil
}

// Sealed — la valeur est-elle scellée (ou vide, donc rien à sceller) ?
func Sealed(stored string) bool {
	return stored == "" || strings.HasPrefix(stored, prefix)
}
