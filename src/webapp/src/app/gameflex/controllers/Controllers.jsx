import React from "react";
import ControllerLayout from "../stream/onscreen/Controllers.jsx";
import { CONTROLLER_PRESETS } from "./presets.js";

// ControllerLayout's `callbacks` prop is keyed BY BUTTON NAME (see
// its ColumnZone -> PlaceButton: `callback = callbacks[b.name] || {}`), not
// a single flat {onmousedown,onmouseup,onanalogmove} object — that mismatch
// (every wrapper below used to build the flat shape) meant callbacks[name]
// was always undefined, so callback.onmousedown was always undefined too:
// every press silently went nowhere, on every controller. Built once here,
// generically, from whichever preset's own button list, instead of
// duplicating a (wrong) shape in each wrapper.
function buildCallbacks(presetName, onButtonDown, onButtonUp, onAnalogMove, onTouch) {
    const preset = CONTROLLER_PRESETS[presetName];
    const callbacks = {};
    for (const b of preset?.buttons || []) {
        callbacks[b.name] = {
            onmousedown: () => onButtonDown?.(b.name),
            onmouseup: () => onButtonUp?.(b.name),
            // Analog sticks call `callback.onmove` (not onanalogmove — see
            // PlaceButton's updateFromEvent/onPointerUp/onPointerCancel),
            // with just (x, y); the button name is closed over here instead
            // of being passed through, since onMove isn't itself a real
            // DOM/React event and never fires from the JSX prop of that
            // name a few lines below it in ControllerLayout.jsx.
            onmove: (x, y) => onAnalogMove?.(b.name, x, y),
        };
        // LE PAVÉ TACTILE, QUAND LA PLATEFORME SAIT LE SERVIR. ControllerLayout
        // dessine alors une surface au lieu d'un bouton (TouchpadSurface) ; le
        // clic, lui, reste onmousedown/onmouseup. Sans `onTouch`, TOUCHPAD
        // reste le simple clic qu'il a toujours été.
        if (b.name === "TOUCHPAD" && onTouch) {
            callbacks[b.name].ontouch = onTouch;
        }
    }
    return callbacks;
}

function makeController(presetName) {
    // canDrawButton : transmis tel quel — un gabarit peut porter un bouton
    // que la console de la session ne sait pas servir (voir son propre
    // commentaire dans ControllerLayout). Absent, tout est dessiné.
    // onTouch(doigt, posé, x, y) : voir buildCallbacks.
    return function Controller({ onButtonDown, onButtonUp, onAnalogMove, onTouch, opacity, canDrawButton }) {
        return (
            <ControllerLayout
                name={presetName}
                callbacks={buildCallbacks(presetName, onButtonDown, onButtonUp, onAnalogMove, onTouch)}
                opacity={opacity}
                canDrawButton={canDrawButton}
            />
        );
    };
}

// Retro Gaming
export const DefaultGamepad = makeController("default");
// PS1
export const PS1 = makeController("ps1");
// PS1/PS2 DualShock
export const DualShock = makeController("dualshock");
// PS3 DualShock3
export const DualShock3 = makeController("dualshock3");
// PS4/PS5 DualSense
export const DualSense = makeController("dualsense");
// PSP/PSVita
export const PSP = makeController("psp");
// Gameboy Advance (GBA)
export const GBA = makeController("gba");
// Genesis
export const Genesis = makeController("genesis");
// Nintendo Entertainment System (NES)
export const NES = makeController("nes");
// Super Nintendo Entertainment System (SNES)
export const SNES = makeController("snes");
// Nintendo 64 (N64)
export const N64 = makeController("n64");
// Nintendo Switch (Joy-Con)
export const JoyCon = makeController("joycon");
// Xbox (360 layout)
export const Xbox = makeController("xbox");

// Retro-gaming — inverted from libs/gamepadMapping.js's own DEFAULT_MAPPING
// (Gamepad API index -> libretro action name), so a real controller lights
// up the right marker here too instead of guessing a second mapping.
export const RETRO_GAMEPAD_INDEX = {
    B: 0,
    A: 1,
    Y: 2,
    X: 3,
    L: 4,
    R: 5,
    L2: 6,
    R2: 7,
    SELECT: 8,
    START: 9,
    L3: 10,
    R3: 11,
    DPAD_UP: 12,
    DPAD_DOWN: 13,
    DPAD_LEFT: 14,
    DPAD_RIGHT: 15,
};


// Retro-gaming — libretro RETRO_DEVICE_ID_JOYPAD_* bit index per name (see
// the deleted gameflex/shared/gamepadOverlay.js's own RETRO_BUTTON_INDEX for
// the original 14, uppercased/DPAD_-prefixed to match this file's button
// naming). This is what actually has to be sent over the WebRTC DataChannel
// (stream/retrogaming/hooks/use-retro-streaming-connection.js's sendButton -> Go backend's
// s.inputStates[player][ButtonID], read by the libretro core itself) — NOT
// RETRO_GAMEPAD_INDEX above, which is a completely different index space
// (a real Gamepad API button position, only meaningful for live-
// highlighting a physical controller). Mixing the two up sends the wrong
// input to the emulator (e.g. DPAD_UP, index 12 here vs RETRO_GAMEPAD_
// INDEX's 12, is RETRO_DEVICE_ID_JOYPAD_L2 to the core).
// PS-style face/shoulder button names (dualshock/dualshock3/psp presets —
// ControllerLayout.jsx) alias the SAME libretro bit as their SNES-
// style equivalent, matched by PHYSICAL POSITION on the pad (the standard
// RetroArch/libretro convention for every PSX/PSP core): Cross (bottom) =
// B, Circle (right) = A, Square (left) = Y, Triangle (top) = X, L1 = L,
// R1 = R. Missing before — those 4 face buttons and L1/R1 always looked up
// as undefined, so the outgoing message had no `button` field at all
// (JSON.stringify drops it) and the Go backend silently no-opped it
// (`if in.ButtonID != nil && ...`) — confirmed via a real report: Triangle/
// Circle/Square/Cross dead on PSX/PSP (and, before this fix, L1/R1 too).
export const RETRO_BUTTON_INDEX = {
    B: 0,
    Y: 1,
    SELECT: 2,
    START: 3,
    DPAD_UP: 4,
    DPAD_DOWN: 5,
    DPAD_LEFT: 6,
    DPAD_RIGHT: 7,
    A: 8,
    X: 9,
    L: 10,
    R: 11,
    L2: 12,
    R2: 13,
    // L3/R3 (stick-click) — real libretro bits, per spec (RETRO_DEVICE_ID_
    // JOYPAD_L3/R3 = 14/15), just never wired before since the old retro-
    // gaming UI had no stick-click button at all. Go's inputStates is
    // [32]int16 per player (stream_session.go), plenty of room.
    L3: 14,
    R3: 15,
    CROSS: 0,
    SQUARE: 1,
    CIRCLE: 8,
    TRIANGLE: 9,
    L1: 10,
    R1: 11,
    // LA MANETTE PS4/PS5 DESSINÉE (dualsense) nomme Start et Select OPTIONS
    // et SHARE, et son pavé tactile TOUCHPAD : absents de cette table, ils
    // envoyaient un index vide, et Options ne faisait rien sur la PS4 et la
    // PS5 du téléphone (le porteur, 2026-10-27, FF7 Remake). Share et le
    // pavé pressent tous deux SELECT, la touche du pavé tactile des tables
    // de GlibcGuest — comme au serveur, où Share devient « select », que
    // shadPS4 lit comme le pavé (`touchpad_center = back`).
    OPTIONS: 3,
    SHARE: 2,
    TOUCHPAD: 2,
    // LE BOUTON PS — pas un bit libretro (ils s'arrêtent à 15) : l'index du
    // bouton « guide » de la Gamepad API, que keyMapping.js lui donne déjà
    // (dualshock3, dualsense). Un cœur libretro ne le demande jamais ; le
    // pont X du téléphone en fait la touche que le profil de RPCS3 lui
    // attribue (GlibcGuest.RPCS3_KEYS). Sans lui, le PS de la DualShock 3
    // dessinée ne partait nulle part.
    PS: 16,
};

// 8-way d-pad corners: libretro cores combine two held directions natively,
// so a corner sends BOTH direction bits — the same trick streamex's server
// does in its padHatDirs. Shared by Streaming.jsx's on-screen
// pad and by keyboard chords (ArrowDown+ArrowRight → DPAD_DOWN_RIGHT).
export const RETRO_DIAGONAL_BUTTONS = {
    DPAD_UP_LEFT: ["DPAD_UP", "DPAD_LEFT"],
    DPAD_UP_RIGHT: ["DPAD_UP", "DPAD_RIGHT"],
    DPAD_DOWN_LEFT: ["DPAD_DOWN", "DPAD_LEFT"],
    DPAD_DOWN_RIGHT: ["DPAD_DOWN", "DPAD_RIGHT"],
};

// Some presets used to reuse a button LABEL for a different physical/
// RetroPad position than this table assumes (Genesis's own A/C/X/Y/Z,
// N64's Z) — fixed at the SOURCE instead (ControllerLayout.jsx's
// CONTROLLER_PRESETS `name` fields renamed to whichever key here is
// actually correct for that position, `icon`/`text` left alone so the
// button still LOOKS like what the player expects) rather than by adding a
// second per-preset lookup layer here. See those presets' own comments for
// the genesis_plus_gx/mupen64plus_next mapping each rename is based on.

// Map consoleId → virtual controller PRESET NAME (CONTROLLER_PRESETS key) —
// the single source of truth RETRO_CONTROLLER_MAP (component) below AND
// KeyMapping (Phase 8, gameflex/libs/keyMapping.js — a physical device is
// mapped against a preset NAME, not a component) both derive from, so the
// two can never drift apart.
export const RETRO_CONTROLLER_PRESET_MAP = {
    nes: "nes",
    snes: "snes",
    gb: "nes",
    gbc: "nes",
    gba: "gba",
    megadrive: "genesis",
    genesis: "genesis",
    sms: "genesis",
    // PS1 defaults to the DUALSHOCK (2026-08-25). It is what the child
    // switches port 0 to on every ps1 session, so it is also what the
    // keyboard/physical-gamepad mapping and the rumble flag must assume —
    // this map feeds all three, plus the settings page's preset list.
    // Leaving it on the digital pad made the on-screen controller and the
    // mapping profile disagree about the same session, which is exactly the
    // failure resolveControllerLayout was written to prevent. The digital
    // pad is still one click away in the session's own PS1 picker.
    ps1: "dualshock",
    psx: "dualshock",
    // PS2 renders through the same DualShock preset as psx — both cores
    // (pcsx2/play) read the same libretro button bits.
    ps2: "dualshock",
    psp: "psp",
    n64: "n64",
    // gc — le preset n64 s'appelle lui-même « Nintendo 64/GameCube », et il
    // porte ce qu'une GameCube demande : Z sur une gâchette, et les C sur le
    // stick droit (c'est le C-stick, sur cette console-là). Sans cette
    // ligne, la GameCube tombait sur le pad générique.
    gc: "n64",
    nds: "snes",
    switch: "joycon",
    // LES CONSOLES DE BUREAU DU TÉLÉPHONE (2026-10-11). Sur le serveur, ce
    // sont des sessions streamex, et leur manette vient de
    // streamex/controllers (CONSOLE_PADS) ; sur le téléphone, une PS3 tourne
    // dans l'application et sa session est « device » — elle passe donc par
    // CETTE table, qui ne la connaissait pas : le porteur avait le pad
    // générique (X/Y/A/B, L/R seuls) au lieu d'une DualShock 3. Les mêmes
    // dessins que le serveur, à la Wii U près, qui y emprunte déjà celui de
    // la Switch.
    ps3: "dualshock3",
    ps4: "dualsense",
    ps5: "dualsense",
    xbox: "xbox",
    xbox360: "xbox",
    wiiu: "joycon",
};

// Exported so stream/shared/controller-layout.js (the single resolver
// every platform's Player.jsx now goes through) can look up a component by
// preset name without a second copy of this table.
export const PRESET_COMPONENT_BY_NAME = {
    default: DefaultGamepad,
    ps1: PS1,
    dualshock: DualShock,
    dualshock3: DualShock3,
    dualsense: DualSense,
    psp: PSP,
    gba: GBA,
    genesis: Genesis,
    nes: NES,
    snes: SNES,
    n64: N64,
    gamecube: N64,
    joycon: JoyCon,
    xbox: Xbox,
};

// Map consoleId → controller component (new system).
export const RETRO_CONTROLLER_MAP = Object.fromEntries(
    Object.entries(RETRO_CONTROLLER_PRESET_MAP).map(([consoleId, presetName]) => [consoleId, PRESET_COMPONENT_BY_NAME[presetName]])
);