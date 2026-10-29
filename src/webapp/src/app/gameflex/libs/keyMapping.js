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
import { captureKeyboardChord } from "./keyboardMapping.js";
import { getKeyMapping as fetchKeyMapping, saveKeyMapping as persistKeyMapping } from "../profile/api.js";
import { streamAuthToken } from "./serverBase.js";

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

// ── Auto-mapping (Phase 8 follow-up) ─────────────────────────────────────
//
// Every physical gamepad that connects gets a DEFAULT KeyMapping saved for
// each virtual-controller preset the retro-gaming consoles use, so the pad
// works out of the box on every console — the user can then change (or
// delete) any mapping from profile/GamepadParameters.jsx's "Mapping
// manette" tab. Auto-mapping is create-only: a pair that already has a
// saved mapping is never overwritten, and once created it behaves exactly
// like a hand-made one.
//
// The defaults below mirror what the per-console fallback layer
// (gamepadMapping.js's DEFAULT_MAPPING) already does today — same physical
// Gamepad API indices, just expressed in each preset's own button
// vocabulary instead of libretro names.

// Gamepad API standard-layout index → preset button name, per preset. Only
// the presets the retro consoles actually render through
// (RETRO_CONTROLLER_PRESET_MAP values + "default" fallback) need a table —
// the rest have no input hook looking them up.
const STANDARD_DIAMOND = {
    "0": "B", "1": "A", "2": "Y", "3": "X",
    "4": "L", "5": "R", "8": "SELECT", "9": "START",
    "12": "DPAD_UP", "13": "DPAD_DOWN", "14": "DPAD_LEFT", "15": "DPAD_RIGHT",
};
const STANDARD_PS = {
    "0": "CROSS", "1": "CIRCLE", "2": "SQUARE", "3": "TRIANGLE",
    "4": "L1", "5": "R1", "6": "L2", "7": "R2",
    "8": "SELECT", "9": "START", "10": "L3", "11": "R3",
    "12": "DPAD_UP", "13": "DPAD_DOWN", "14": "DPAD_LEFT", "15": "DPAD_RIGHT",
};
// 8-way d-pad corners: standard-layout indices 12-15 combined into chords
// (the same "+" shape the capture produces, see the chord runtime) — every
// preset with a d-pad gets them, so the diagonals work out of the box on a
// physical pad too, not just on the virtual one.
const DPAD_DIAGONAL_CHORDS = {
    "12+14": "DPAD_UP_LEFT",
    "12+15": "DPAD_UP_RIGHT",
    "13+14": "DPAD_DOWN_LEFT",
    "13+15": "DPAD_DOWN_RIGHT",
};
const DEFAULT_MAPPING_BY_CONTROLLER = {
    default: { ...STANDARD_DIAMOND, ...DPAD_DIAGONAL_CHORDS },
    nes: { ...STANDARD_DIAMOND, ...DPAD_DIAGONAL_CHORDS, "2": undefined, "3": undefined },
    snes: { ...STANDARD_DIAMOND, ...DPAD_DIAGONAL_CHORDS },
    gba: { ...STANDARD_DIAMOND, ...DPAD_DIAGONAL_CHORDS },
    genesis: { ...STANDARD_DIAMOND, ...DPAD_DIAGONAL_CHORDS },
    ps1: { ...STANDARD_PS, ...DPAD_DIAGONAL_CHORDS },
    dualshock: { ...STANDARD_PS, ...DPAD_DIAGONAL_CHORDS },
    psp: {
        "0": "CROSS", "1": "CIRCLE", "2": "SQUARE", "3": "TRIANGLE",
        "4": "L", "5": "R", "8": "SELECT", "9": "START",
        "12": "DPAD_UP", "13": "DPAD_DOWN", "14": "DPAD_LEFT", "15": "DPAD_RIGHT",
        ...DPAD_DIAGONAL_CHORDS,
    },
    n64: {
        "0": "B", "1": "A", "4": "L", "5": "R", "6": "L2", "9": "START",
        "12": "DPAD_UP", "13": "DPAD_DOWN", "14": "DPAD_LEFT", "15": "DPAD_RIGHT",
        ...DPAD_DIAGONAL_CHORDS,
    },
    joycon: {
        "0": "A", "1": "B", "2": "X", "3": "Y",
        "4": "L", "5": "R", "7": "ZR", "8": "SELECT", "9": "START",
        "11": "R3",
        "12": "DPAD_UP", "13": "DPAD_DOWN", "14": "DPAD_LEFT", "15": "DPAD_RIGHT",
        ...DPAD_DIAGONAL_CHORDS,
    },
    // 16 is the standard layout's guide button. Only the presets that
    // actually DRAW a PS button (presets.js) claim it — naming a button the
    // target does not have is worse than leaving it unmapped.
    dualshock3: { ...STANDARD_PS, ...DPAD_DIAGONAL_CHORDS, "16": "PS" },
    dualsense: {
        "0": "CROSS", "1": "CIRCLE", "2": "SQUARE", "3": "TRIANGLE",
        "4": "L1", "5": "R1", "6": "L2", "7": "R2",
        "8": "SHARE", "9": "OPTIONS", "10": "L3", "11": "R3",
        "12": "DPAD_UP", "13": "DPAD_DOWN", "14": "DPAD_LEFT", "15": "DPAD_RIGHT",
        "16": "PS", "17": "TOUCHPAD",
        ...DPAD_DIAGONAL_CHORDS,
    },
    gamecube: {
        "0": "A", "1": "B", "2": "X", "3": "Y",
        "4": "L", "5": "R", "7": "Z", "9": "START",
        "12": "DPAD_UP", "13": "DPAD_DOWN", "14": "DPAD_LEFT", "15": "DPAD_RIGHT",
        ...DPAD_DIAGONAL_CHORDS,
    },
    xbox: {
        "0": "A", "1": "B", "2": "X", "3": "Y",
        "4": "LB", "5": "RB", "6": "LT", "7": "RT",
        "8": "BACK", "9": "START", "10": "L3", "11": "R3",
        "12": "DPAD_UP", "13": "DPAD_DOWN", "14": "DPAD_LEFT", "15": "DPAD_RIGHT",
        ...DPAD_DIAGONAL_CHORDS,
    },
};

const AUTO_MAP_CONTROLLERS = Object.keys(DEFAULT_MAPPING_BY_CONTROLLER);

// LES MANETTES QUI NUMÉROTENT LEURS BOUTONS PAR ÉTIQUETTE NINTENDO (le
// porteur, 2026-10-28 : « c'est le mappage auto qui est en erreur »).
//
// Les tables ci-dessus lisent la disposition « standard » de l'API Gamepad,
// qui est une affaire de POSITION : 0 en bas, 1 à droite, 2 à gauche, 3 en
// haut. Une manette à étiquettes Nintendo (A à droite, B en bas, X en haut,
// Y à gauche) qui se présente à Android comme une manette générique envoie
// pourtant son A comme le A d'Android — le bouton du bas. Le navigateur la
// dit « standard » quand même, et rien dans l'API ne la distingue : seul son
// nom la trahit. Sur la PS3 du téléphone, avec une GameSir Pocket Taco,
// Triangle faisait Carré et Rond faisait Croix.
//
// Pour elles, la table échange le bas et la droite, la gauche et le haut :
// chaque bouton retrouve la fonction de SA POSITION, comme sur toute autre
// manette. Un nom s'ajoute ici quand une manette de plus le montre.
const NINTENDO_LABELLED_PADS = [/pocket taco/i];
const NINTENDO_FACE_SWAP = { "0": "1", "1": "0", "2": "3", "3": "2" };

function hasNintendoLabels(padId) {
    return NINTENDO_LABELLED_PADS.some((pattern) => pattern.test(padId || ""));
}

/**
 * L'index d'un bouton selon sa POSITION — celui que supposent les tables
 * d'ici et l'ordre des consoles streamex (CONSOLE_GAMEPAD_ORDER).
 *
 * @param {string} padId l'identifiant de la manette (gamepad.id)
 * @param {number} index le bouton tel que la manette l'envoie
 * @returns {number}
 */
export function positionalButtonIndex(padId, index) {
    return hasNintendoLabels(padId) ? Number(NINTENDO_FACE_SWAP[index] ?? index) : index;
}

// Default mapping for a virtual-controller preset, or null when the preset
// has no auto-mapping table (undefined-valued entries are dropped — e.g.
// NES has no X/Y face buttons). padId — the gamepad's id, for the pads that
// number their face buttons by Nintendo label (see NINTENDO_LABELLED_PADS).
export function defaultMappingForController(controllerName, padId = "") {
    const raw = DEFAULT_MAPPING_BY_CONTROLLER[controllerName];
    if (!raw) return null;
    const swap = hasNintendoLabels(padId) ? NINTENDO_FACE_SWAP : {};
    const mapping = {};
    for (const [index, name] of Object.entries(raw)) {
        if (name) mapping[swap[index] ?? index] = name;
    }
    return mapping;
}

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

// Auto-maps ONE gamepad against every preset that has a default table —
// create-only (a pair that already has a saved mapping is left alone).
export async function ensureAutoMappedGamepad(token, gamepadId) {
    for (const controllerName of AUTO_MAP_CONTROLLERS) {
        const existing = await loadKeyMapping(token, gamepadId, controllerName);
        if (Object.keys(existing).length > 0) continue;
        const mapping = defaultMappingForController(controllerName, gamepadId);
        if (!mapping) continue;
        setCachedKeyMapping(gamepadId, controllerName, mapping);
        await persistKeyMapping(playerToken(token), gamepadId, "gamepad", controllerName, mapping);
    }
}

// Applies the default mapping to EVERY virtual controller for one device,
// OVERWRITING whatever is saved — the explicit "auto-mapping" action behind
// profile/GamepadParameters.jsx's button (the user can then re-map anything).
export async function applyAutoMapping(token, physicalDeviceId, physicalDeviceType = "gamepad") {
    let applied = 0;
    for (const controllerName of AUTO_MAP_CONTROLLERS) {
        const mapping = defaultMappingForController(controllerName, physicalDeviceId);
        if (!mapping) continue;
        setCachedKeyMapping(physicalDeviceId, controllerName, mapping);
        const result = await persistKeyMapping(playerToken(token), physicalDeviceId, physicalDeviceType, controllerName, mapping);
        if (result.ok) applied++;
    }
    return applied;
}

export { captureKeyboardChord };

// Streams the chord being built from ONE gamepad's presses: every button
// whose down-edge fires while the capture is active is ADDED to the
// accumulated set (never removed — the chord never resolves on its own), and
// onChange receives the sorted indices joined with "+" after every change.
// The caller (KeyMappingCaptureRow) validates with its tick button and
// discards with the trash one. Returns a cleanup that stops the polling;
// aborting the signal runs the same cleanup.
//
// Scoped to ONE specific gamepad index — the whole point is letting several
// physical devices each have their own saved mapping, so capture must watch
// the SAME index the user selected in the UI, not whichever pad reports
// first. Polls on rAF + a 25 ms interval (rAF alone stalls when the tab
// loses focus, e.g. the PS button opening a system overlay).
export function captureGamepadChordForIndex(gamepadIndex, { signal, onChange } = {}) {
    let raf;
    let interval;
    let previousPressed = null;
    const accumulated = new Set();

    const cleanup = () => {
        cancelAnimationFrame(raf);
        clearInterval(interval);
        signal?.removeEventListener("abort", onAbort);
    };
    const onAbort = cleanup;

    // A button counts as pressed when `pressed` is true OR its analog value
    // reports one — some drivers expose buttons through the value channel.
    const isPressed = (button) =>
        !!button?.pressed ||
        (typeof button?.value === "number" && button.value > 0.5);

    const check = () => {
        const pad = (navigator.getGamepads?.() || [])[gamepadIndex];
        if (!pad) return;
        const buttons = pad.buttons || [];
        if (previousPressed === null || previousPressed.length !== buttons.length) {
            previousPressed = buttons.map(isPressed);
        }
        for (let index = 0; index < buttons.length; index++) {
            const pressed = isPressed(buttons[index]);
            if (pressed && !previousPressed[index]) {
                accumulated.add(String(index));
                onChange?.(
                    [...accumulated]
                        .map(Number)
                        .sort((a, b) => a - b)
                        .join("+"),
                );
            }
            previousPressed[index] = pressed;
        }
    };

    if (signal?.aborted) return cleanup;
    signal?.addEventListener("abort", onAbort);
    raf = requestAnimationFrame(function loop() {
        check();
        raf = requestAnimationFrame(loop);
    });
    interval = setInterval(check, 25);
    return cleanup;
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

// remapKey assigns physicalInput to virtualButtonName — clears any OTHER
// physicalInput currently bound to the same virtual button first (one
// physical input per virtual button; re-pressing over an existing binding
// for a DIFFERENT button just overwrites it), same last-write-wins model as
// the older gamepadMapping.js/keyboardMapping.js remap functions. Persists
// to the backend (fire-and-forget besides the caller's own error handling)
// and updates the local cache/subscribers immediately so the UI reflects it
// without waiting on the round-trip.
export async function remapKey(token, physicalDeviceId, physicalDeviceType, controllerName, virtualButtonName, physicalInput) {
    const mapping = { ...getCachedKeyMapping(physicalDeviceId, controllerName) };
    for (const key of Object.keys(mapping)) {
        if (mapping[key] === virtualButtonName) delete mapping[key];
    }
    mapping[physicalInput] = virtualButtonName;
    setCachedKeyMapping(physicalDeviceId, controllerName, mapping);
    return persistKeyMapping(playerToken(token), physicalDeviceId, physicalDeviceType, controllerName, mapping);
}

// Resolves a physical input (button index or keyboard code, always looked up
// as a string key) to a virtual button name, or undefined if this device has
// no KeyMapping entry for it (caller should fall back to its own per-console
// default in that case).
export function resolveVirtualButton(mapping, physicalInput) {
    return mapping[String(physicalInput)];
}
