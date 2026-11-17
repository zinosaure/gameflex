// LA RÈGLE D'UN MOT DE PASSE CHOISI (chantier 55, `#692`) — le porteur, 2026-11-14 : « entre 8 et 20 avec les
// caractères spéciaux inclus obligatoirement. Normalement, skeleton.Password avec PasswordStrength gère ça. Regarde
// comment l'utiliser. »
//
// ÉCRITE ICI UNE FOIS, et lue par la jauge de Skeleton (`Form.Password.StrengthEvaluation`, ses `tests`) partout où une
// personne choisit son mot de passe : la page du lien (Join.jsx) et « Mon mot de passe ». Ce sont les tests par défaut
// de Skeleton — 8 caractères, une majuscule, une minuscule, un chiffre, un caractère spécial — dits dans la langue de la
// page, et la borne des 20 au plus. Le service des comptes applique EXACTEMENT la même, ligne à ligne
// (credential/internal/auth/password.go), et refuse en 422 avec un code par ligne manquée (PASSWORD_CODES).
//
// Les longueurs se comptent en caractères (des points de code, comme le serveur), et 72 octets au plus : la limite de
// bcrypt, que 20 caractères ne dépassent qu'avec une suite d'émojis. Une lettre accentuée compte comme caractère
// spécial, comme dans les tests de Skeleton (`[^A-Za-z0-9]`).

const MIN_LENGTH = 8;
const MAX_LENGTH = 20;
const MAX_BYTES = 72;

const length = (value) => [...(value || "")].length;
const bytes = (value) => new TextEncoder().encode(value || "").length;

// Une ligne : son code (celui du refus du serveur), et ce qu'elle vérifie. Skeleton appelle `regex.test(valeur)` : une
// ligne lui donne un objet qui sait `test`.
const RULES = [
    { code: "too_short", test: (value) => length(value) >= MIN_LENGTH },
    { code: "too_long", test: (value) => length(value) > 0 && length(value) <= MAX_LENGTH && bytes(value) <= MAX_BYTES },
    { code: "no_uppercase", test: (value) => /[A-Z]/.test(value || "") },
    { code: "no_lowercase", test: (value) => /[a-z]/.test(value || "") },
    { code: "no_digit", test: (value) => /[0-9]/.test(value || "") },
    { code: "no_special", test: (value) => /[^A-Za-z0-9]/.test(value || "") },
];

export const PASSWORD_CODES = RULES.map((rule) => rule.code);

// Le mot de passe suit-il la règle entière ?
export const passwordAccepted = (value) => RULES.every((rule) => rule.test(value));

// Les tests de la jauge de Skeleton, dans la langue de la page (`password.<code>`).
export const strengthTests = (t) => RULES.map((rule) => ({
    regex: { test: rule.test },
    message: t(`password.${rule.code}`, { min: MIN_LENGTH, max: MAX_LENGTH }),
}));

// UN REFUS DU SERVEUR (422, `codes`), dit dans la langue de la page : ses lignes manquées, sinon son texte anglais.
export const refusalText = (t, answer) => {
    const known = (answer?.codes || []).filter((code) => PASSWORD_CODES.includes(code));

    return known.length
        ? t("password.refused", { reasons: known.map((code) => t(`password.${code}`, { min: MIN_LENGTH, max: MAX_LENGTH })).join(" · ") })
        : answer?.detail || "";
};
