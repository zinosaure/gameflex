import { useSyncExternalStore } from "react";

// Device-level gamepad UX preferences (localStorage, NOT per-user): haptic
// feedback on virtual button presses and rumble/vibration output. Kept out
// of the server because both describe the physical device in your hands —
// a phone-buzz toggle means nothing on someone else's laptop.
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

let preferences = {
    hapticFeedback: readStoredBool(HAPTIC_KEY, true),
    rumble: readStoredBool(RUMBLE_KEY, true),
    rumbleStrength: readStoredRatio(RUMBLE_STRENGTH_KEY, 1),
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

function subscribeGamepadPreferences(listener) {
    listeners.add(listener);
    return () => listeners.delete(listener);
}

// Hooks — useSyncExternalStore keeps live session components in sync when
// the Controller page toggles a switch, without a context provider.
export function useHapticFeedback() {
    return useSyncExternalStore(subscribeGamepadPreferences, getHapticFeedback);
}

export function useRumbleEnabled() {
    return useSyncExternalStore(subscribeGamepadPreferences, getRumbleEnabled);
}
