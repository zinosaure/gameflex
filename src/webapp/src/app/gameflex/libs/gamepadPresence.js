import { useSyncExternalStore } from "react";
import { connectedGamepads } from "./keyMapping.js";

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

function check() {
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
