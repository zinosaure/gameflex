// UN TEXTE EN TOUCHES (#420) : ce que l'entrée « Clavier » du menu tape dans
// l'émulateur.
//
// streamex ne reçoit que des touches (`kd,<code>` / `ku,<code>`, le
// KeyboardEvent.code d'un clavier branché), qu'il injecte par XTest dans le
// serveur X de la partie — un clavier QWERTY américain. Une lettre s'y écrit
// donc comme on la taperait là : sa touche, avec Maj pour une majuscule ou un
// signe du haut d'une touche.

// Les signes, par touche : le premier sans Maj, le second avec.
const SIGNS = {
    Backquote: "`~",
    Digit1: "1!",
    Digit2: "2@",
    Digit3: "3#",
    Digit4: "4$",
    Digit5: "5%",
    Digit6: "6^",
    Digit7: "7&",
    Digit8: "8*",
    Digit9: "9(",
    Digit0: "0)",
    Minus: "-_",
    Equal: "=+",
    BracketLeft: "[{",
    BracketRight: "]}",
    Backslash: "\\|",
    Semicolon: ";:",
    Quote: "'\"",
    Comma: ",<",
    Period: ".>",
    Slash: "/?",
};

const KEYS = new Map([[" ", { code: "Space", shift: false }]]);
for (const [code, [plain, shifted]] of Object.entries(SIGNS)) {
    KEYS.set(plain, { code, shift: false });
    KEYS.set(shifted, { code, shift: true });
}
for (let letter = 65; letter <= 90; letter += 1) {
    const upper = String.fromCharCode(letter);
    KEYS.set(upper.toLowerCase(), { code: `Key${upper}`, shift: false });
    KEYS.set(upper, { code: `Key${upper}`, shift: true });
}

/**
 * Les touches d'un texte, dans l'ordre, et ce qu'un clavier américain n'a pas
 * (un accent, un emoji) — jamais tapé à la place d'autre chose.
 *
 * @param {string} text
 * @returns {{keys: {code: string, shift: boolean}[], missing: string[]}}
 */
export function keysOfText(text) {
    const keys = [];
    const missing = [];
    for (const character of text) {
        const key = KEYS.get(character);
        if (key) {
            keys.push(key);
        } else if (!missing.includes(character)) {
            missing.push(character);
        }
    }

    return { keys, missing };
}
