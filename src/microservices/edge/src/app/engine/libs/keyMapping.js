// KeyMapping (Phase 8 of the GameFlex refactor) — physical device -> VIRTUAL
// CONTROLLER preset (CONTROLLER_PRESETS in ControllerLayout.jsx), not
// per-console like the older gamepadMapping.js/keyboardMapping.js — since
// several consoles already share one virtual controller (RETRO_CONTROLLER_MAP
// in Controllers.jsx, e.g. PS1+PS2 both use "dualshock"), mapping a physical
// device once against that preset covers every console using it.
//
// This is an ADDITIVE layer, not a replacement: a device with no saved
// KeyMapping for a given controllerName resolves to nothing here, and the
// caller (Streaming.jsx's input hooks) falls back to the older per-console
// gamepadMapping.js/keyboardMapping.js defaults — so a user who never opens
// the new "Mapping manette" tab keeps today's zero-config behavior exactly
// as it was; KeyMapping only kicks in once they've actually configured one.
//
// mapping shape: { [physicalInput]: virtualButtonName } — physicalInput is a
// Gamepad API button INDEX (number, stored as a string key) for a gamepad
// device, or a KeyboardEvent.code (string) for the keyboard device.
// virtualButtonName matches CONTROLLER_PRESETS[controllerName].buttons[].name
// (e.g. "CROSS", "DPAD_UP", "L1").
import { getKeyMapping as fetchKeyMapping } from "../../libs/profileApi.js";
import { streamAuthToken } from "../../libs/serverBase.js";

// LE JETON DE L'HÔTE QUAND LA PAGE EN VISE UN, sinon celui de l'appelant.
//
// Ces requêtes suivent la surcharge de la page (apiUrl) : sur une partie
// jouée chez un serveur fédéré, elles partent CHEZ LUI. Y présenter notre
// jeton ne pouvait que revenir 401 — et l'envoyer à une machine que ça ne
// regarde pas (lu dans la console d'un téléphone le 2026-09-07).
//
// Posé ici plutôt que chez les quatre appelants : c'est le même besoin pour
// tous, et un oubli dans un seul d'entre eux se manifesterait par une manette
// qui ne répond pas — la panne la plus coûteuse à relier à sa cause.
function playerToken(token) {
    return streamAuthToken() || token;
}

import { positionalButtonIndex } from "../controllers/default-mapping.js";

// Les tables par défaut vivent dans controllers/default-mapping.js (partagées avec le launcher).
export { positionalButtonIndex };

// Stable device id for a pad — some browsers report an EMPTY gamepad.id,
// which would silently skip auto-mapping and never match the saved rows.
export function gamepadDeviceId(pad) {
    return pad?.id || `pad-${pad?.index ?? 0}`;
}

// LA MANETTE DU SERVEUR N'EST PAS CELLE DU JOUEUR.
//
// Un conteneur de session streamex crée une manette virtuelle (uinput) pour y
// injecter les entrées : c'est ELLE que l'émulateur lit. Or uinput n'est pas
// cloisonné par conteneur — le périphérique apparaît dans le /dev/input de la
// MACHINE. Un navigateur ouvert sur cette machine-là l'énumère donc comme
// n'importe quelle manette branchée.
//
// Ce qui s'ensuit, constaté le 2026-09-07 : la page croit qu'une manette
// physique est connectée alors qu'il n'y en a aucune, elle range la manette à
// l'écran, elle enregistre une disposition pour ce fantôme
// (`physical_device_id=streamex pad ps3-p2 …` dans les journaux), et surtout
// elle renvoie l'état au repos de ce pad dans la session — la sortie de
// l'émulateur rebouclée sur son entrée.
//
// Sur un vrai déploiement le joueur est ailleurs et ne voit rien de tout
// cela ; sur la machine qui héberge, c'est systématique. Le nom suffit à
// trancher : ces manettes sont nommées par le conteneur lui-même.
const SESSION_PAD_PREFIX = "streamex pad";

/** Cette manette est-elle celle d'un conteneur de session, et non d'un joueur ? */
function isSessionVirtualPad(pad) {
    return String(pad?.id || "").toLowerCase().startsWith(SESSION_PAD_PREFIX);
}

/**
 * Les manettes RÉELLEMENT branchées — à utiliser partout où l'on énumère.
 *
 * `navigator.getGamepads()` rend des trous (null) et, sur une machine qui
 * héberge des sessions, les manettes virtuelles de ses propres conteneurs.
 */
export function connectedGamepads() {
    if (typeof navigator === "undefined" || !navigator.getGamepads) {
        return [];
    }

    return [...navigator.getGamepads()].filter((pad) => pad && !isSessionVirtualPad(pad));
}

const KEYBOARD_DEVICE_STORAGE_KEY = "gameflex_keyboard_device_id";

// Browsers expose no stable hardware id for a keyboard at all (unlike
// gamepad.id, a real — if not perfectly unique — Gamepad API string) — this
// generates one once and persists it, representing "this keyboard on this
// browser", not a true hardware identity. Stable across reloads/sessions on
// the same browser profile; a different browser or a cleared localStorage
// is, from KeyMapping's point of view, a different device.
export function getKeyboardDeviceId() {
    try {
        let id = localStorage.getItem(KEYBOARD_DEVICE_STORAGE_KEY);
        if (!id) {
            id = `keyboard-${typeof crypto !== "undefined" && crypto.randomUUID ? crypto.randomUUID() : `${Date.now()}-${Math.random().toString(36).slice(2)}`}`;
            localStorage.setItem(KEYBOARD_DEVICE_STORAGE_KEY, id);
        }
        return id;
    } catch {
        // localStorage unavailable (private mode, quota) — a per-tab id
        // still lets the current session work, just doesn't persist.
        return "keyboard-transient";
    }
}

// In-memory cache + pub/sub, keyed by `${physicalDeviceId}::${controllerName}`
// — mirrors gamepadMapping.js/keyboardMapping.js's own change-event shape,
// but scoped in-process (no window CustomEvent) since, unlike those two,
// every consumer of a given KeyMapping lives in the same Streaming/
// GamepadParameters render tree, never a separate tab.
const cache = new Map();
const listeners = new Map();

function cacheKey(physicalDeviceId, controllerName) {
    return `${physicalDeviceId}::${controllerName}`;
}

export function getCachedKeyMapping(physicalDeviceId, controllerName) {
    return cache.get(cacheKey(physicalDeviceId, controllerName)) || {};
}

function setCachedKeyMapping(physicalDeviceId, controllerName, mapping) {
    const key = cacheKey(physicalDeviceId, controllerName);
    cache.set(key, mapping);
    for (const fn of listeners.get(key) || []) fn(mapping);
}

export function subscribeKeyMapping(physicalDeviceId, controllerName, callback) {
    const key = cacheKey(physicalDeviceId, controllerName);
    if (!listeners.has(key)) listeners.set(key, new Set());
    listeners.get(key).add(callback);
    return () => listeners.get(key)?.delete(callback);
}

// Fetches the user's saved mapping for (device, controller) and seeds the
// local cache — a no-op (empty mapping, meaning "fall back to per-console
// defaults") if the user never configured one for this pair.
export async function loadKeyMapping(token, physicalDeviceId, controllerName) {
    const result = await fetchKeyMapping(playerToken(token), physicalDeviceId, controllerName);
    const mapping = (result.ok && result.items?.[0]?.mapping) || {};
    setCachedKeyMapping(physicalDeviceId, controllerName, mapping);
    return mapping;
}

// Resolves a physical input (button index or keyboard code, always looked up
// as a string key) to a virtual button name, or undefined if this device has
// no KeyMapping entry for it (caller should fall back to its own per-console
// default in that case).
export function resolveVirtualButton(mapping, physicalInput) {
    return mapping[String(physicalInput)];
}
