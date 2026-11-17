package auth

// LA RÈGLE D'UN MOT DE PASSE CHOISI (chantier 55, `#692`) — le porteur,
// 2026-11-14 : « entre 8 et 20 avec les caractères spéciaux inclus
// obligatoirement. Normalement, skeleton.Password avec PasswordStrength gère
// ça. » La page la montre par la jauge de Skeleton
// (`Form.Password.StrengthEvaluation`) et ses tests, écrits UNE fois dans le
// launcher (libs/password.js) ; ce service applique EXACTEMENT la même, ligne à
// ligne, et rend un code par ligne manquée :
//
//   - 8 caractères au moins, 20 au plus — des caractères, comme les compte la
//     page (des points de code), et 72 octets au plus : la limite de bcrypt, que
//     20 caractères ne dépassent qu'avec une suite d'émojis. Au-delà, bcrypt
//     refusait, et la page recevait une erreur 500 ;
//   - une majuscule, une minuscule et un chiffre, au sens des tests de Skeleton
//     (A à Z, a à z, 0 à 9) ;
//   - un caractère spécial : tout le reste (ponctuation, espace, lettre
//     accentuée), comme le `[^A-Za-z0-9]` de Skeleton.
//
// Elle vaut pour tout mot de passe qu'une PERSONNE choisit : le lien d'un compte
// et « Mon mot de passe ». Pas pour ceux d'aujourd'hui, jusqu'à leur prochain
// changement, ni pour `GAMEFLEX_ROOT_PASSWORD`, choisi par celui qui installe, ni
// pour ceux qu'on tire au sort (la console du téléphone, ses invités).

import (
	"strings"
	"unicode/utf8"
)

const (
	PasswordMinLength = 8
	PasswordMaxLength = 20
	// passwordMaxBytes — au-delà, bcrypt refuse (ErrPasswordTooLong).
	passwordMaxBytes = 72
)

// Les codes d'un refus, que la page traduit (password.<code>).
const (
	PasswordTooShort    = "too_short"
	PasswordTooLong     = "too_long"
	PasswordNoUppercase = "no_uppercase"
	PasswordNoLowercase = "no_lowercase"
	PasswordNoDigit     = "no_digit"
	PasswordNoSpecial   = "no_special"
)

// PasswordRefusals — ce qui manque à ce mot de passe, dans l'ordre des lignes de
// la page ; vide, il est accepté.
func PasswordRefusals(password string) []string {
	refusals := []string{}
	length := utf8.RuneCountInString(password)

	if length < PasswordMinLength {
		refusals = append(refusals, PasswordTooShort)
	}

	if length > PasswordMaxLength || len(password) > passwordMaxBytes {
		refusals = append(refusals, PasswordTooLong)
	}

	for _, kind := range []struct {
		code string
		test func(rune) bool
	}{
		{PasswordNoUppercase, func(r rune) bool { return r >= 'A' && r <= 'Z' }},
		{PasswordNoLowercase, func(r rune) bool { return r >= 'a' && r <= 'z' }},
		{PasswordNoDigit, func(r rune) bool { return r >= '0' && r <= '9' }},
		{PasswordNoSpecial, func(r rune) bool { return !isASCIILetterOrDigit(r) }},
	} {
		if !strings.ContainsFunc(password, kind.test) {
			refusals = append(refusals, kind.code)
		}
	}

	return refusals
}

func isASCIILetterOrDigit(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}
