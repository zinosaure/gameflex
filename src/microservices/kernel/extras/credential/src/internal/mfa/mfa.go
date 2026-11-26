// Package mfa — la double authentification des joueurs (chantier 80, `#809`).
//
// UN CODE À SIX CHIFFRES D'UNE APPLICATION D'AUTHENTIFICATION (TOTP, RFC 6238 : Google Authenticator, Aegis,
// 1Password…) : SHA-1, une période de 30 s, six chiffres, les réglages que toutes ces applications lisent sans qu'on
// les leur dise. Pas de SMS ni de courriel : GameFlex n'en envoie pas.
//
// LA CLÉ EST UN FICHIER, DANS UN VOLUME À PART (le porteur, 2026-11-24 : « La clé devra être gardée sur le serveur en
// volume » ; « c'est une bonne idée pour les keys ») : `mfa.key` dans `keys/` du dossier du serveur, à côté de
// `databases/` et non dedans, pour qu'une copie de la base seule ne livre pas les secrets. Créée au premier besoin
// (32 octets aléatoires, droits 0600), gardée d'un démarrage à l'autre. Le secret de chaque compte est scellé avec
// elle (AES-256-GCM).
//
// EFFACER LA CLÉ REMET TOUT LE MONDE À ZÉRO : les secrets scellés avec l'ancienne ne s'ouvrent plus, et chaque compte
// refait son activation à sa connexion suivante, avec son mot de passe. C'est la seule issue de root s'il perd son
// téléphone et ses codes de secours (aucun administrateur ne touche à son compte) ; qui peut effacer ce fichier
// possède déjà la machine.
package mfa

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1"
	"crypto/subtle"
	"encoding/base32"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	keyFile    = "mfa.key"
	keySize    = 32
	secretSize = 20 // 160 bits, la taille que recommande la RFC 4226 pour SHA-1
	period     = 30
	digits     = 6
	// skew — un pas de part et d'autre : l'horloge d'un téléphone peut avoir quelques secondes d'écart.
	skew = 1
	// RecoveryCount — les codes de secours d'un compte, à usage unique.
	RecoveryCount = 10
	sealPrefix    = "mfa1:"
)

// ErrSealed — le secret ne s'ouvre pas avec cette clé (la clé a été effacée et refaite, ou la valeur est abîmée).
var ErrSealed = errors.New("mfa: the secret does not open with this key")

var codec = base32.StdEncoding.WithPadding(base32.NoPadding)

// LoadKey — la clé de `dir`, créée si elle n'existe pas encore.
func LoadKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, keyFile)
	raw, err := os.ReadFile(path)
	if err == nil {
		key, decodeErr := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
		if decodeErr != nil || len(key) != keySize {
			return nil, fmt.Errorf("mfa: %s is not a key", path)
		}

		return key, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("mfa: read %s: %w", path, err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("mfa: create %s: %w", dir, err)
	}
	key := make([]byte, keySize)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	// O_EXCL : deux démarrages simultanés ne s'écrasent pas leur clé ; le second relit celle du premier.
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return LoadKey(dir)
	}
	if err != nil {
		return nil, fmt.Errorf("mfa: create %s: %w", path, err)
	}
	defer file.Close()
	if _, err := file.WriteString(base64.StdEncoding.EncodeToString(key) + "\n"); err != nil {
		return nil, fmt.Errorf("mfa: write %s: %w", path, err)
	}

	return key, nil
}

func aead(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}

	return cipher.NewGCM(block)
}

// Seal — le secret scellé avec la clé, à ranger en base.
func Seal(key []byte, secret string) (string, error) {
	gcm, err := aead(key)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	sealed := gcm.Seal(nonce, nonce, []byte(secret), []byte("gameflex/mfa"))

	return sealPrefix + base64.StdEncoding.EncodeToString(sealed), nil
}

// Open — le secret d'une valeur scellée par Seal avec la même clé ; ErrSealed sinon.
func Open(key []byte, sealed string) (string, error) {
	raw, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(sealed, sealPrefix))
	if err != nil || !strings.HasPrefix(sealed, sealPrefix) {
		return "", ErrSealed
	}
	gcm, err := aead(key)
	if err != nil {
		return "", err
	}
	if len(raw) < gcm.NonceSize() {
		return "", ErrSealed
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], []byte("gameflex/mfa"))
	if err != nil {
		return "", ErrSealed
	}

	return string(plain), nil
}

// NewSecret — un secret neuf, en base32 (ce que l'application d'authentification attend).
func NewSecret() (string, error) {
	raw := make([]byte, secretSize)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}

	return codec.EncodeToString(raw), nil
}

// URI — l'adresse `otpauth://` que le code QR porte : l'application y lit le secret, le nom du compte et celui du
// serveur.
func URI(issuer, account, secret string) string {
	label := url.PathEscape(issuer) + ":" + url.PathEscape(account)
	query := url.Values{}
	query.Set("secret", secret)
	query.Set("issuer", issuer)
	query.Set("algorithm", "SHA1")
	query.Set("digits", fmt.Sprint(digits))
	query.Set("period", fmt.Sprint(period))

	return "otpauth://totp/" + label + "?" + query.Encode()
}

// codeAt — le code du pas `step` (RFC 4226 : HMAC-SHA1, la troncature dynamique).
func codeAt(secret []byte, step int64) string {
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, secret)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	return fmt.Sprintf("%0*d", digits, value%1_000_000)
}

// CodeAt — le code d'un secret à un instant (les tests, et qui veut vérifier une application d'authentification).
func CodeAt(secret string, at time.Time) (string, error) {
	raw, err := codec.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return "", err
	}

	return codeAt(raw, Step(at)), nil
}

// Step — le pas de temps d'un instant.
func Step(at time.Time) int64 {
	return at.Unix() / period
}

// Verify — si `code` est celui du secret à cet instant (à un pas près). Rend le pas reconnu : un code ne vaut qu'une
// fois, le service refuse tout pas qui n'est pas plus récent que le dernier employé (`last`).
func Verify(secret, code string, at time.Time, last int64) (int64, bool) {
	raw, err := codec.DecodeString(strings.ToUpper(strings.TrimSpace(secret)))
	if err != nil {
		return 0, false
	}
	code = strings.ReplaceAll(strings.TrimSpace(code), " ", "")
	if len(code) != digits {
		return 0, false
	}
	now := Step(at)
	for step := now - skew; step <= now+skew; step++ {
		if step <= last {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(codeAt(raw, step)), []byte(code)) == 1 {
			return step, true
		}
	}

	return 0, false
}

// recoveryAlphabet — sans 0/O ni 1/I/L, qui se confondent à la lecture.
const recoveryAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

// NewRecoveryCodes — des codes de secours, « XXXXX-XXXXX » (environ 49 bits chacun).
func NewRecoveryCodes(count int) ([]string, error) {
	codes := make([]string, count)
	for index := range codes {
		raw := make([]byte, 10)
		if _, err := rand.Read(raw); err != nil {
			return nil, err
		}
		var builder strings.Builder
		for position, b := range raw {
			if position == 5 {
				builder.WriteByte('-')
			}
			builder.WriteByte(recoveryAlphabet[int(b)%len(recoveryAlphabet)])
		}
		codes[index] = builder.String()
	}

	return codes, nil
}

// NormalizeRecovery — un code de secours tel qu'on le compare : majuscules, sans espaces ni tirets.
func NormalizeRecovery(code string) string {
	return strings.NewReplacer("-", "", " ", "").Replace(strings.ToUpper(strings.TrimSpace(code)))
}
