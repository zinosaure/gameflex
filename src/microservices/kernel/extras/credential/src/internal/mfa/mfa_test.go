package mfa

import (
	"encoding/base32"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// LES VECTEURS DE LA RFC 6238 (annexe B, SHA-1, secret « 12345678901234567890 ») : les huit chiffres de la RFC, dont
// une application d'authentification garde les six derniers.
func TestCodesFollowRFC6238(t *testing.T) {
	secret := []byte("12345678901234567890")
	for _, vector := range []struct {
		at   int64
		code string
	}{{59, "94287082"}, {1111111109, "07081804"}, {1111111111, "14050471"}, {1234567890, "89005924"}, {2000000000, "69279037"}} {
		if got := codeAt(secret, vector.at/period); got != vector.code[2:] {
			t.Errorf("at %d: %s, want %s", vector.at, got, vector.code[2:])
		}
	}
}

func TestVerifyAcceptsOneStepOfDriftAndNeverTheSameStepTwice(t *testing.T) {
	secret := base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString([]byte("12345678901234567890"))
	at := time.Unix(1234567890, 0)
	code := codeAt([]byte("12345678901234567890"), Step(at))

	step, ok := Verify(secret, code, at, 0)
	if !ok || step != Step(at) {
		t.Fatalf("the code of now is refused")
	}
	if _, ok := Verify(secret, code, at, step); ok {
		t.Fatal("the same code is accepted twice")
	}
	if _, ok := Verify(secret, code, at.Add(30*time.Second), 0); !ok {
		t.Error("a code one step old is refused")
	}
	if _, ok := Verify(secret, code, at.Add(90*time.Second), 0); ok {
		t.Error("a code three steps old is accepted")
	}
	if _, ok := Verify(secret, "000000", at, 0); ok && codeAt([]byte("12345678901234567890"), Step(at)) != "000000" {
		t.Error("a wrong code is accepted")
	}
	if _, ok := Verify(secret, " "+code[:3]+" "+code[3:], at, 0); !ok {
		t.Error("a code typed with spaces is refused")
	}
}

// LA CLÉ : créée une fois, 0600, relue telle quelle ; un secret ne s'ouvre qu'avec elle.
func TestTheKeyIsKeptAndSealsOnlyForItself(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "keys")
	key, err := LoadKey(dir)
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, keyFile))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key file: %v, mode %v", err, info.Mode().Perm())
	}
	again, err := LoadKey(dir)
	if err != nil || string(again) != string(key) {
		t.Fatal("the key changed when read again")
	}

	secret, _ := NewSecret()
	sealed, err := Seal(key, secret)
	if err != nil || strings.Contains(sealed, secret) {
		t.Fatalf("sealed %q: %v", sealed, err)
	}
	if opened, err := Open(key, sealed); err != nil || opened != secret {
		t.Fatalf("open: %q, %v", opened, err)
	}

	other, _ := LoadKey(t.TempDir())
	if _, err := Open(other, sealed); err != ErrSealed {
		t.Fatalf("another key opens the secret: %v", err)
	}
}

func TestRecoveryCodesAreDistinctAndReadable(t *testing.T) {
	codes, err := NewRecoveryCodes(RecoveryCount)
	if err != nil || len(codes) != RecoveryCount {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, code := range codes {
		if len(code) != 11 || code[5] != '-' || strings.ContainsAny(code, "01IOL") || seen[code] {
			t.Errorf("code %q", code)
		}
		seen[code] = true
	}
	if NormalizeRecovery(" abcde-fghjk ") != "ABCDEFGHJK" {
		t.Error("normalize")
	}
}

func TestTheURICarriesTheSecretAndTheServer(t *testing.T) {
	uri := URI("GameFlex - Staging", "gino", "ABC")
	for _, part := range []string{"otpauth://totp/GameFlex%20-%20Staging:gino?", "secret=ABC", "issuer=GameFlex", "digits=6", "period=30"} {
		if !strings.Contains(uri, part) {
			t.Errorf("%s lacks %s", uri, part)
		}
	}
}
