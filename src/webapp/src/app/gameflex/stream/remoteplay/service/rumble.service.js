import { applyRumble } from "../../../libs/rumble.js";

// L'ADAPTATEUR PS5 — la plage de la console, et rien d'autre.
//
// Ce fichier portait sa PROPRE implémentation complète : magasin de
// réglages (`controller_rumble_enabled`/`controller_rumble_strength`, avec
// ses écouteurs), résolution d'actionneur, repli téléphone. Tout ça existait
// déjà dans `libs/rumble.js` pour le rétro, à quelques divergences près que
// personne n'avait voulues.
//
// Deux conséquences visibles, avant le 2026-09-09 :
//
//   - l'interrupteur de la page Manette écrivait dans l'AUTRE magasin, donc
//     il ne pilotait rien côté PS5. Ça ne se voyait pas : le défaut d'ici
//     était « allumé » ;
//   - une manette ancienne, qui n'expose que `hapticActuators.pulse`,
//     vibrait sur PS5 et pas sur PS1 — ce fichier connaissait cette API,
//     l'autre non.
//
// Il ne reste donc que la conversion : la PS5 compte de 0 à 255, le moteur
// partagé prend des magnitudes entre 0 et 1.

// La durée d'un effet PS5. La console renvoie une nouvelle intensité très
// souvent pendant une secousse ; 80 ms couvre l'intervalle sans traîner.
const PS5_EFFECT_MS = 80;

/**
 * Appliquer une vibration reçue de la PS5.
 *
 * @param {object} event `{left, right}` ou `{rawLeft, rawRight}`, 0..255.
 * @returns {boolean} vrai si quelque chose a effectivement vibré.
 */
export function applyControllerRumbleToGamepads(event, options = {}) {
    return applyRumble({
        strong: (event.left ?? event.rawLeft ?? 0) / 255,
        weak: (event.right ?? event.rawRight ?? 0) / 255,
        durationMs: options.duration ?? PS5_EFFECT_MS,
        gamepadIndex: typeof options.gamepadIndex === "number" ? options.gamepadIndex : null,
        // TOUTES les manettes, pas seulement la première : c'est ce que ce
        // chemin faisait, et le changer serait modifier le comportement
        // d'une plateforme que personne n'a demandé de changer.
        targets: "all",
        deviceVibrationMs: options.duration ?? PS5_EFFECT_MS,
    });
}
