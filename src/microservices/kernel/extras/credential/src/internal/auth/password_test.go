package auth

import (
	"strings"
	"testing"
)

// LA RÈGLE DU LAUNCHER, LIGNE À LIGNE (chantier 55) : chaque code, la limite de
// bcrypt dite par un refus et non plus par une erreur 500, une lettre accentuée
// comptée comme caractère spécial — les tests de Skeleton ne connaissent que A-Z,
// a-z et 0-9 —, et un mot de passe fort accepté.
func TestThePasswordRuleIsTheLaunchersRule(t *testing.T) {
	cases := []struct {
		password string
		want     string
	}{
		{"Gameflex#2026", ""},
		{"Aa1!aaaa", ""},
		{"Aa1!aaaaaaaaaaaaaaaa", ""},
		{"élodieA2026", ""},
		{"Aa1!", "too_short"},
		{"Aa1!aaaaaaaaaaaaaaaaa", "too_long"},
		{"gameflex#2026", "no_uppercase"},
		{"GAMEFLEX#2026", "no_lowercase"},
		{"Gameflex#lune", "no_digit"},
		{"Gameflex2026", "no_special"},
		{"Élodie2026", "no_uppercase"},
		{"", "too_short,no_uppercase,no_lowercase,no_digit,no_special"},
		// Vingt caractères, mais 74 octets : au-delà de bcrypt.
		{"aA" + strings.Repeat("😀", 18), "too_long,no_digit"},
		// Vingt caractères, 71 octets : accepté.
		{"aA1" + strings.Repeat("😀", 17), ""},
	}
	for _, check := range cases {
		if got := strings.Join(PasswordRefusals(check.password), ","); got != check.want {
			t.Errorf("%q : %q, attendu %q", check.password, got, check.want)
		}
	}
}

// Un mot de passe que la règle accepte, bcrypt le hache : plus d'erreur 500.
func TestAnAcceptedPasswordAlwaysHashes(t *testing.T) {
	for _, password := range []string{"aA1" + strings.Repeat("😀", 17), "Aa1!aaaaaaaaaaaaaaaa"} {
		if len(PasswordRefusals(password)) > 0 {
			t.Fatalf("%q refusé", password)
		}
		if _, err := HashPassword(password); err != nil {
			t.Fatalf("%q : %v", password, err)
		}
	}
}
