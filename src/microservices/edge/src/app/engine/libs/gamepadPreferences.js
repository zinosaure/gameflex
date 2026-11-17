import { useSyncExternalStore } from "react";

// Device-level gamepad UX preferences (localStorage, NOT per-user): haptic
// feedback on virtual button presses and rumble/vibration output. Kept out
// of the server because both describe the physical device in your hands —
// a phone-buzz toggle means nothing on someone else's laptop.
// UNE SEULE SOURCE, CE MODULE (#758) : Réglages › Manettes et le menu d'une
// partie lisent et écrivent ici, jamais le stockage directement. La valeur
// vit en mémoire dès le chargement de l'app ; un écrivain qui passait par le
// seul stockage (libs/keyMapping.js, jusqu'au 2026-11-17) laissait la mémoire
// à l'ancienne valeur, que la partie suivante réécrivait par-dessus son choix.
const HAPTIC_KEY = "gameflex_haptic_feedback";
// LA CLÉ A CHANGÉ DE NOM, ET C'EST LE CORRECTIF.
//
// Un ancien défaut écrivait `false` ici tout seul : l'effet qui persiste
// l'interrupteur tournait avec `undefined` chaque fois qu'un joueur
// reprenait la manette (voir Streaming.jsx, où le garde a été posé). Le
// garde a arrêté l'hémorragie ; il n'a rien réparé. La valeur empoisonnée
// est restée dans le navigateur de tous ceux qui avaient joué avant, et la
// vibration y est morte en silence — sur toutes les consoles à la fois,
// puisque c'est un interrupteur global. Signalé le 2026-09-08 : « le rumble
// ne fonctionne ni sur PS1, PS2 ni remoteplay », les trois d'un coup.
//
// On ne peut pas distinguer « éteint par le défaut » de « éteint
// volontairement ». Changer de nom règle les deux d'un même geste : tout le
// monde repart du défaut une fois, et qui n'en veut pas le réétaint. Un
// aller-retour d'un clic contre une fonctionnalité morte sans le dire.
const RUMBLE_KEY = "gameflex_rumble_v2";
// UN SEUL ENDROIT POUR LE RUMBLE, DEPUIS LE 2026-09-09.
//
// Remote-play avait son propre magasin — `controller_rumble_enabled` et
// `controller_rumble_strength`, dans son service — avec ses propres
// écouteurs et ses propres accesseurs. Deux magasins pour un seul
// interrupteur : celui de la page Manette écrivait ici, remote-play lisait
// là-bas, et le bouton ne pilotait donc rien côté PS5. Ça ne se voyait pas,
// parce que le défaut de l'autre magasin était « allumé ».
//
// L'intensité vient de là aussi. Aucune interface ne l'écrit encore (elle
// vaut 1), mais elle a maintenant UN emplacement, prêt pour le curseur du
// jour où on l'ajoutera — plutôt qu'un second à ne pas oublier.
const RUMBLE_STRENGTH_KEY = "gameflex_rumble_strength";
// LES STICKS INVISIBLES (#758) : les sticks de la manette à l'écran sont toujours flottants (onscreen/Controllers.jsx) ;
// allumé, aucun stick n'est dessiné au repos, il n'apparaît que sous le pouce. Coupé par défaut : les sticks se
// dessinent à leur place. Réglé dans Réglages › Manettes et le menu d'une partie, qui l'écrivent ICI (voir plus haut).
// Il remplace l'interrupteur du stick flottant (le porteur, 2026-11-17 : « maintenant, on aura toujours les sticks
// flottant et ce qui passe en option ce serait le fait de les afficher ou pas »).
const INVISIBLE_STICKS_KEY = "gameflex_invisible_sticks";

let preferences = {
    hapticFeedback: readStoredBool(HAPTIC_KEY, true),
    rumble: readStoredBool(RUMBLE_KEY, true),
    rumbleStrength: readStoredRatio(RUMBLE_STRENGTH_KEY, 1),
    invisibleSticks: readStoredBool(INVISIBLE_STICKS_KEY, false),
};
const listeners = new Set();

function readStoredBool(key, fallback) {
    try {
        if (typeof window === "undefined") return fallback;
        const raw = window.localStorage.getItem(key);
        return raw === null ? fallback : raw !== "false";
    } catch {
        return fallback;
    }
}

// readStoredRatio — un nombre entre 0 et 1, borné à la lecture.
//
// Borné ICI et pas à l'usage : une valeur hors bornes vient forcément d'un
// stockage trafiqué ou d'une version antérieure, et la corriger au plus près
// de son entrée évite d'avoir à s'en méfier partout ailleurs.
function readStoredRatio(key, fallback) {
    try {
        if (typeof window === "undefined") return fallback;
        const raw = window.localStorage.getItem(key);
        if (raw === null) return fallback;
        const value = parseFloat(raw);
        if (!Number.isFinite(value)) return fallback;

        return Math.min(1, Math.max(0, value));
    } catch {
        return fallback;
    }
}

function store(key, value) {
    try {
        window.localStorage.setItem(key, value ? "true" : "false");
    } catch {
        /* storage unavailable — the in-memory value still works */
    }
}

function update(next) {
    preferences = next;
    listeners.forEach((listener) => listener());
}

export function getHapticFeedback() {
    return preferences.hapticFeedback;
}

export function setHapticFeedback(enabled) {
    store(HAPTIC_KEY, enabled);
    update({ ...preferences, hapticFeedback: !!enabled });
}

export function getRumbleEnabled() {
    return preferences.rumble;
}

export function setRumbleEnabled(enabled) {
    store(RUMBLE_KEY, enabled);
    update({ ...preferences, rumble: !!enabled });
}

export function getRumbleStrength() {
    return preferences.rumbleStrength;
}

export function setInvisibleSticks(enabled) {
    store(INVISIBLE_STICKS_KEY, enabled);
    update({ ...preferences, invisibleSticks: !!enabled });
}

// LES PRÉFÉRENCES, SUIVIES : le menu d'une partie, Réglages › Manettes et la manette à l'écran se redessinent
// quand l'un des deux endroits en change une — les sticks invisibles s'appliquent à la partie en cours, sans rechargement.
function subscribe(listener) {
    listeners.add(listener);

    return () => listeners.delete(listener);
}

export function useGamepadPreferences() {
    return useSyncExternalStore(subscribe, () => preferences);
}
