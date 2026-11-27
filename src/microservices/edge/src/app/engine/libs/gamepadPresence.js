import { useSyncExternalStore } from "react";
import { connectedGamepads, gamepadDeviceId } from "./keyMapping.js";
import { getPadPreferences, onPadPreferences } from "./gamepadPreferences.js";

// UNE MANETTE PHYSIQUE EST-ELLE BRANCHÉE ? — une seule détection pour toute
// l'application (2026-10-21).
//
// Elle vivait dans Streaming.jsx, où elle cache la manette à l'écran quand une
// vraie est là. Le demi-écran des manettes clipsées (libs/screenOrientation.js,
// #410) pose la même question, hors d'une partie : deux détections, et l'une
// finirait par voir une manette que l'autre ne voit pas.
//
// ON SONDE, on ne se contente pas d'écouter. `gamepadconnected` n'est pas
// fiable : certains navigateurs et beaucoup de tablettes ne l'émettent jamais
// et ne révèlent la manette dans `getGamepads()` qu'après sa première
// pression. N'écouter que l'événement laissait la page à « aucune manette »
// pour toujours, quoi qu'on branche — le pad tactile ne se cachait jamais
// (signalé le 2026-09-03). use-auto-map-gamepads.js sonde pour la même raison.
//
// Le sondage ne tourne que tant que quelqu'un écoute.
const GAMEPAD_POLL_MS = 2000;

let present = connectedGamepads().length > 0;
let poll = null;
const listeners = new Set();

// LES MANETTES DE L'APPAREIL, NUMÉROTÉES (chantier 84) — la même détection, une fois. Une manette se reconnaît par son
// identifiant, suivi de son rang parmi les manettes identiques (`<id>#<rang>`) : deux manettes du même modèle restent
// deux. Les manettes ACTIVÉES se numérotent à partir de 1 dans l'ordre choisi (Réglages › Manettes › Manettes
// connectées), les inconnues à la suite, dans l'ordre du navigateur ; une désactivée n'a pas de numéro.
// `devicePads()` : `[{key, gamepad, index, name, enabled, number}]`, dans l'ordre des numéros, puis les désactivées.
let pads = [];
let padsSignature = "";

function readPads() {
    const { padOrder, padsOff } = getPadPreferences();
    const ranks = new Map();
    const connected = connectedGamepads().filter((gamepad) => gamepad?.connected !== false).sort((a, b) => a.index - b.index).map((gamepad) => {
        const id = gamepadDeviceId(gamepad);
        const rank = ranks.get(id) || 0;
        ranks.set(id, rank + 1);

        return { key: `${id}#${rank}`, gamepad, index: gamepad.index, name: String(gamepad.id || "").replace(/\s*\(.*$/, "") || id };
    });
    const position = (entry) => {
        const at = padOrder.indexOf(entry.key);

        return at < 0 ? padOrder.length + entry.index : at;
    };
    const ordered = connected.sort((a, b) => position(a) - position(b));
    let number = 0;
    const enabled = ordered.filter((entry) => !padsOff.includes(entry.key)).map((entry) => ({ ...entry, enabled: true, number: ++number }));
    const disabled = ordered.filter((entry) => padsOff.includes(entry.key)).map((entry) => ({ ...entry, enabled: false, number: null }));

    return [...enabled, ...disabled];
}

function refreshPads() {
    const next = readPads();
    const signature = next.map((entry) => `${entry.key}:${entry.number}`).join("|");
    if (signature !== padsSignature) {
        padsSignature = signature;
        pads = next;
        padListeners.forEach((listener) => listener());
    }
}

const padListeners = new Set();

// devicePads — les manettes de l'appareil maintenant (les objets Gamepad relus : le navigateur les remplace à chaque
// lecture).
export const devicePads = () => readPads();

// L'APPUI D'UNE MANETTE PORTE SON NUMÉRO (chantier 84) : `#<n>|` devant celui de la manette 2 et des suivantes ; celui de la
// manette 1 (le clavier, la manette à l'écran, une seule manette branchée) part tel quel. La partie le lit
// (session.SplitPad) et le donne au siège de cette manette.
export const withPad = (number, payload) => (number > 1 ? `#${number}|${payload}` : payload);

// LES SIÈGES DE CET APPAREIL (chantier 84), tels que la partie les dit (`you.pads` : `[{pad, slotId}]`) : la vibration
// d'un siège va à la manette qui le tient.
let seats = [];
export const setPadSeats = (list) => {
    seats = Array.isArray(list) ? list : [];
};
// L'index (Gamepad API) de la manette qui tient ce siège, ou null : aucune manette de cet appareil ne le tient, ou
// c'est le clavier, la manette à l'écran.
export const gamepadOfSeat = (slot) => {
    const number = seats.find((entry) => entry.slotId === slot)?.pad;
    const pad = number ? readPads().find((entry) => entry.number === number) : null;

    return pad ? pad.index : null;
};

// La manette numéro `number` de l'appareil, ou null.
export const padNumbered = (number) => devicePads().find((entry) => entry.number === number) || null;

function check() {
    refreshPads();
    const next = connectedGamepads().length > 0;
    if (next === present) {
        return;
    }
    present = next;
    listeners.forEach((listener) => listener(present));
}

export function hasPhysicalGamepad() {
    return present;
}

// Être prévenu quand une manette arrive ou s'en va.
export function onGamepadPresence(listener) {
    listeners.add(listener);
    if (listeners.size === 1) {
        poll = setInterval(check, GAMEPAD_POLL_MS);
        window.addEventListener("gamepadconnected", check);
        window.addEventListener("gamepaddisconnected", check);
    }
    check();

    return () => {
        listeners.delete(listener);
        if (listeners.size === 0) {
            clearInterval(poll);
            poll = null;
            window.removeEventListener("gamepadconnected", check);
            window.removeEventListener("gamepaddisconnected", check);
        }
    };
}

export function usePhysicalGamepad() {
    return useSyncExternalStore(onGamepadPresence, hasPhysicalGamepad);
}

// LES MANETTES DE L'APPAREIL, SUIVIES (chantier 84) : une manette branchée, débranchée, activée ou renumérotée redessine.
function subscribePads(listener) {
    padListeners.add(listener);
    const stopPresence = onGamepadPresence(() => {});
    const stopPreferences = onPadPreferences(refreshPads);
    refreshPads();

    return () => {
        padListeners.delete(listener);
        stopPresence();
        stopPreferences();
    };
}

export function useDevicePads() {
    return useSyncExternalStore(subscribePads, () => pads);
}
