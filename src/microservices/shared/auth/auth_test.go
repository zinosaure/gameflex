package auth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

const secret = "s3cr3t"

var now = time.Date(2026, 8, 25, 12, 0, 0, 0, time.UTC)

func keys(t *testing.T) (signer func(Claims) string, verify func(string) (Claims, error)) {
	t.Helper()
	private, public, err := KeyPair(secret)
	if err != nil {
		t.Fatalf("KeyPair : %v", err)
	}

	return func(claims Claims) string {
			token, err := Sign(private, claims)
			if err != nil {
				t.Fatalf("Sign : %v", err)
			}

			return token
		}, func(token string) (Claims, error) {
			return Verify(public, token, now)
		}
}

func valid() Claims {
	return Claims{Username: "gino", RoleLevel: 2, Subject: "2", ExpiresAt: now.Add(time.Hour).Unix()}
}

func TestVerifyAcceptsAGoodToken(t *testing.T) {
	sign, verify := keys(t)
	claims, err := verify(sign(valid()))
	if err != nil {
		t.Fatalf("Verify = %v, want success", err)
	}
	if claims.Username != "gino" || claims.RoleLevel != 2 {
		t.Fatalf("claims = %+v", claims)
	}
}

// La même installation retrouve la même paire, de chaque côté : le service
// d'identité signe, le kernel vérifie, sans s'échanger de fichier de clé.
func TestTheSecretAlwaysGivesTheSameKey(t *testing.T) {
	_, first, _ := KeyPair(secret)
	_, again, _ := KeyPair(secret)
	_, other, _ := KeyPair("another")
	if EncodePublicKey(first) != EncodePublicKey(again) || EncodePublicKey(first) == EncodePublicKey(other) {
		t.Fatal("la clé ne dépend pas du seul secret")
	}
	parsed, err := ParsePublicKey(EncodePublicKey(first))
	if err != nil || !first.Equal(parsed) {
		t.Fatalf("ParsePublicKey : %v", err)
	}
	for _, text := range []string{"", "pas-une-clé", EncodePublicKey(first)[:20]} {
		if _, err := ParsePublicKey(text); err == nil {
			t.Errorf("ParsePublicKey(%q) a accepté", text)
		}
	}
}

// Le piège classique : un jeton qui nomme lui-même son algorithme peut dire
// « none », ou HS256 pour qu'une clé publique serve de secret HMAC. Seul
// EdDSA est accepté.
func TestVerifyRefusesAnotherAlgorithm(t *testing.T) {
	_, verify := keys(t)
	body := base64.RawURLEncoding.EncodeToString([]byte(`{"username":"root","role_level":0,"exp":` +
		itoa(now.Add(time.Hour).Unix()) + `}`))
	none := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	if _, err := verify(none + "." + body + "."); err != ErrBadAlgorithm {
		t.Fatalf("alg=none = %v, want ErrBadAlgorithm", err)
	}

	// Signé en HS256 avec l'ancien secret : le jeton d'avant le 2026-10-28.
	hs := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(hs + "." + body))
	if _, err := verify(hs + "." + body + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))); err != ErrBadAlgorithm {
		t.Fatalf("HS256 = %v, want ErrBadAlgorithm", err)
	}
}

func TestVerifyRefusesATamperedPayload(t *testing.T) {
	sign, verify := keys(t)
	parts := strings.Split(sign(valid()), ".")
	parts[1] = base64.RawURLEncoding.EncodeToString([]byte(`{"username":"root","role_level":0,"exp":` + itoa(now.Add(time.Hour).Unix()) + `}`))

	if _, err := verify(strings.Join(parts, ".")); err != ErrBadSignature {
		t.Fatalf("tampered payload = %v, want ErrBadSignature", err)
	}
}

func TestVerifyRefusesAnotherInstallation(t *testing.T) {
	sign, _ := keys(t)
	_, other, _ := KeyPair("other")
	if _, err := Verify(other, sign(valid()), now); err != ErrBadSignature {
		t.Fatalf("another installation = %v, want ErrBadSignature", err)
	}
}

func TestVerifyRefusesAnExpiredToken(t *testing.T) {
	sign, verify := keys(t)
	claims := valid()
	claims.ExpiresAt = now.Add(-time.Minute).Unix()
	if _, err := verify(sign(claims)); err != ErrExpired {
		t.Fatalf("expired = %v, want ErrExpired", err)
	}
}

func TestVerifyRefusesATokenWithoutExpiry(t *testing.T) {
	sign, verify := keys(t)
	claims := valid()
	claims.ExpiresAt = 0
	if _, err := verify(sign(claims)); err != ErrNoExpiry {
		t.Fatalf("no exp = %v, want ErrNoExpiry", err)
	}
}

// Un service démarré sans sa clé doit échouer FERMÉ, jamais ouvert.
func TestVerifyFailsClosedWithoutAKey(t *testing.T) {
	sign, _ := keys(t)
	if _, err := Verify(nil, sign(valid()), now); err != ErrNoKey {
		t.Fatalf("no key = %v, want ErrNoKey", err)
	}
	if _, _, err := KeyPair(""); err != ErrNoSecret {
		t.Fatalf("KeyPair(\"\") = %v, want ErrNoSecret", err)
	}
}

func TestVerifyRefusesGarbage(t *testing.T) {
	_, verify := keys(t)
	for _, token := range []string{"", "a.b", "a.b.c.d", "not-a-token"} {
		if _, err := verify(token); err == nil {
			t.Fatalf("Verify(%q) accepted garbage", token)
		}
	}
}

// Le secret d'une partie n'ouvre que sa partie, et ne se confond jamais avec
// celui de l'installation.
func TestASessionSecretBelongsToItsSession(t *testing.T) {
	first := SessionSecret(secret, "retro-1")
	if first == "" || first == secret || first != SessionSecret(secret, "retro-1") {
		t.Fatalf("SessionSecret instable ou égal au secret : %q", first)
	}
	if first == SessionSecret(secret, "retro-2") || first == SessionSecret("other", "retro-1") {
		t.Fatal("deux parties, ou deux installations, partagent un secret")
	}
	if SessionSecret("", "retro-1") != "" || SessionSecret(secret, "") != "" {
		t.Fatal("un secret de partie sans secret d'installation ou sans partie")
	}
	if !SameSecret(first, first) || SameSecret(first, "") || SameSecret("", "") || SameSecret("x", first) {
		t.Fatal("SameSecret")
	}
}

func itoa(value int64) string {
	digits := ""
	for value > 0 {
		digits = string(rune('0'+value%10)) + digits
		value /= 10
	}

	return digits
}
