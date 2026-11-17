// LES CORRESPONDANCES PAR DÉFAUT d'une manette physique vers chaque manette virtuelle — des données
// et deux fonctions pures, SANS AUCUN IMPORT : le moteur (engine/libs/keyMapping.js) et les
// Réglages (libs/keyMapping.js : Réglages › Manettes, « Auto ») lisent la même
// table — jamais une copie qui divergerait. Sorti de libs/keyMapping.js le 2026-11-12 (chantier 51) ; rapatrié de
// la webapp dans edge au chantier 56.

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
        "4": "L", "5": "R", "6": "ZL", "7": "ZR", "8": "SELECT", "9": "START",
        "10": "L3", "11": "R3",
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

export const AUTO_MAP_CONTROLLERS = Object.keys(DEFAULT_MAPPING_BY_CONTROLLER);

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
