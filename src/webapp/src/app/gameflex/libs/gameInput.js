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

// L2 + HOME OUVRENT LE MENU (le porteur, 2026-10-28) — 6 et 16 dans la
// disposition standard de l'API Gamepad. Select + Start ne convenaient pas :
// Start tenu ÉTEINT plusieurs manettes GameSir. Home seul reste au jeu — c'est
// le bouton PS du Remote Play et de la PS3 —, mais pas tant que L2 est tenu :
// sans cela, chaque ouverture du menu appuierait aussi sur PS. D'où l'ordre du
// geste, L2 d'abord.
export const MENU_CHORD = [6, 16];
const MENU_MODIFIER = 6;
const MENU_HOME = 16;

/**
 * Ce bouton est-il le Home du raccourci, et non un Home de jeu ? Les trois
 * plateformes le lisent comme lâché quand c'est le cas.
 *
 * @param {Gamepad} pad la manette lue
 * @param {number} index le bouton
 * @returns {boolean}
 */
export function isMenuHome(pad, index) {
    return index === MENU_HOME && !!pad?.buttons?.[MENU_MODIFIER]?.pressed;
}
