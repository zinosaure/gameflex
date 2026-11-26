// LES ENTRÉES DU JEU, SUSPENDUES PENDANT QU'UN PANNEAU EST OUVERT (#408).
//
// Le menu d'une partie s'ouvre désormais à la manette (L2 + Home tenus) et se
// parcourt à la croix (#407). Tant qu'il est ouvert, la croix, A et B servent
// le menu — et ne doivent pas atteindre le jeu : un « A » qui coche un
// interrupteur ne doit pas aussi sauter dans la partie. Les trois plateformes
// lisent leurs manettes et leur clavier chacune à sa façon (une boucle par
// image pour le rétro, un sondage toutes les 50 ms pour streamex, un service
// pour le jeu à distance) ; elles lisent toutes CET interrupteur, posé par la
// page de jeu quand un panneau s'ouvre ou se ferme.
//
// En se suspendant, chacune RELÂCHE ce qu'elle tenait : les deux boutons
// tenus cinq secondes pour ouvrir le menu, sinon, resteraient enfoncés dans
// le jeu jusqu'à la fermeture.

let suspended = false;
const listeners = new Set();

export function isGameInputSuspended() {
    return suspended;
}

export function setGameInputSuspended(value) {
    const next = !!value;
    if (next === suspended) {
        return;
    }
    suspended = next;
    listeners.forEach((listener) => listener(suspended));
}

// Être prévenu du changement — pour relâcher, au moment même, ce qu'on tenait.
export function onGameInputSuspension(listener) {
    listeners.add(listener);

    return () => listeners.delete(listener);
}

// LE GESTE QUI OUVRE LE MENU D'UNE PARTIE, À LA MANETTE (le porteur, 2026-11-26) — indices de la disposition standard
// de l'API Gamepad (6 : L2, 7 : R2, 16 : Home) :
//   - la RÉTRO : Home seul, un appui — la partie se met en pause et le menu s'ouvre (PlayShell.jsx). Une manette
//     libretro n'a pas de Home : il n'atteint jamais le jeu ;
//   - STREAMEX et le REMOTE PLAY : L2 + R2 + Home tenus deux secondes. Home y est au jeu (le bouton PS, le Home de la
//     Switch), sauf tant que L2 et R2 sont tenus : sans cela, chaque ouverture appuierait aussi sur Home. L2 et R2,
//     eux, restent au jeu.
// Select + Start ne convenaient pas : Start tenu ÉTEINT plusieurs manettes GameSir.
const HOME = 16;
const MENU_GESTURES = {
    home: { buttons: [HOME], holdMs: 0 },
    chord: { buttons: [6, 7, HOME], holdMs: 2000 },
};
const homeAlone = (platform) => platform === "retrogaming" || platform === "device";

// Le geste d'une plateforme : `{ buttons, holdMs }` (`holdMs` 0 : un appui suffit).
export const menuGestureOf = (platform) => (homeAlone(platform) ? MENU_GESTURES.home : MENU_GESTURES.chord);

/**
 * Ce bouton est-il le Home du geste, et non un Home de jeu ? Les trois plateformes le lisent comme lâché quand c'est le
 * cas.
 *
 * @param {Gamepad} pad la manette lue
 * @param {number} index le bouton
 * @param {string} platform la plateforme de la partie
 * @returns {boolean}
 */
export function isMenuHome(pad, index, platform) {
    if (index !== HOME) {
        return false;
    }

    return homeAlone(platform) || MENU_GESTURES.chord.buttons.every((button) => button === HOME || !!pad?.buttons?.[button]?.pressed);
}
