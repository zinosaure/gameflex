import { useEffect, useRef } from "react";
import { loadGamepadMapping, subscribeGamepadMapping } from "../../../libs/gamepadMapping.js";
import { connectedGamepads, getCachedKeyMapping, loadKeyMapping, subscribeKeyMapping, resolveVirtualButton, gamepadDeviceId, positionalButtonIndex } from "../../../libs/keyMapping.js";
import { isGameInputSuspended, isMenuHome } from "../../../libs/gameInput.js";
import {
    RETRO_BUTTON_INDEX,
    RETRO_DIAGONAL_BUTTONS,
    RETRO_CONTROLLER_PRESET_MAP,
} from "../../../controllers/Controllers.jsx";

// Physical-gamepad polling for retro-gaming, used by the Streaming page.
//
// It lives in its own module for a concrete reason, not tidiness — and la
// raison vaut toujours, même si le second appelant (le mode console) a
// disparu le 2026-09-08. Ce hook était importé DEPUIS Streaming.jsx par une
// autre page. Les deux passent par le contexte d'import dynamique de
// skeleton.js (chaque .jsx sous app/ devient son propre morceau asynchrone) :
// le morceau de l'appelant s'exécutait avant que celui de Streaming ne soit
// initialisé, l'espace de noms revenait `undefined`, et la page mourait sur la
// première chose qu'elle y touchait. Un module de PAGE n'est pas une
// bibliothèque : ce que deux pages partagent appartient à un endroit qui
// n'est ni l'une ni l'autre.

// Polls the first connected physical gamepad every animation frame and
// relays it over the same sendButton/sendAnalog path the on-screen virtual
// controller uses — server-side emulation has no built-in gamepad polling
// pipeline (unlike the old client-side EmulatorJS/WASM version), so without
// this a physical controller does nothing at all once connected.
//
// libs/gamepadMapping.js and libs/keyboardMapping.js (both unchanged by the
// controller-visual refactor) still store/produce LOWERCASE, un-prefixed
// libretro action names ("up"/"a"/"select"/...), matching their own
// DEFAULT_MAPPING — a different case/naming convention than Controllers.
// jsx's RETRO_BUTTON_INDEX (uppercase, DPAD_-prefixed — matches
// ControllerLayout.jsx's on-screen button names instead). Kept
// local here rather than exported, since only the physical-gamepad/
// keyboard paths below consume this shape; same values as the deleted
// gameflex/shared/gamepadOverlay.js's own RETRO_BUTTON_INDEX.
export const RETRO_PHYSICAL_BUTTON_INDEX = {
    b: 0, y: 1, select: 2, start: 3,
    up: 4, down: 5, left: 6, right: 7,
    a: 8, x: 9, l: 10, r: 11, l2: 12, r2: 13,
};

// One virtual button name → the libretro bit(s) it drives — diagonals send
// both direction bits (same trick as the on-screen pad). Shared by the
// keyboard hook (Streaming.jsx) and the gamepad hook below, so
// chords resolve identically on both device types.
export function resolveVirtualBits(name) {
    if (name in RETRO_DIAGONAL_BUTTONS) {
        return RETRO_DIAGONAL_BUTTONS[name].map(
            (direction) => RETRO_BUTTON_INDEX[direction],
        );
    }
    const bit = RETRO_BUTTON_INDEX[name];
    return bit === undefined ? [] : [bit];
}

// Resolves one physical input (a Gamepad API button index, or a
// KeyboardEvent.code) to the libretro bit index sendButton needs — KeyMapping
// (Phase 8, per physical device + virtual controller preset, see
// gameflex/libs/keyMapping.js) is tried FIRST; a device with no saved
// KeyMapping entry for this input falls back to the older per-console
// gamepadMapping.js/keyboardMapping.js default (oldMapping, oldNameToIndex),
// so a user who's never opened profile/GamepadParameters.jsx's "Mapping
// manette" tab keeps today's zero-config behavior exactly as it was.
// fallbackInput — the input as the fallback table knows it: a button by its
// POSITION (keyMapping.js's positionalButtonIndex), where a KeyMapping row
// keeps the index the pad sends.
export function resolveBitIndex(physicalInput, keyMapping, oldMapping, oldNameToIndex, fallbackInput = physicalInput) {
    const virtualName = resolveVirtualButton(keyMapping, physicalInput);
    if (virtualName !== undefined) {
        const bitIndex = RETRO_BUTTON_INDEX[virtualName];
        if (bitIndex !== undefined) return bitIndex;
    }
    const oldName = oldMapping[fallbackInput];
    return oldName !== undefined ? oldNameToIndex[oldName] : undefined;
}

// Button INDEX -> libretro bit index comes from KeyMapping-first-then-
// libs/gamepadMapping.js (see resolveBitIndex above) instead of a fixed
// table — both read fresh into a ref on mount/change rather than on every
// tick, since this loop already runs 60x/sec and a mapping change is rare.
// gamepadIndex — null (default, Streaming.jsx's own single-player use) picks
// whichever gamepad slot is found first, exactly as before; console mode
// (gameflex/shared, one hook instance per physical gamepad hub slot) passes
// an explicit index instead so each instance drives ONLY its own assigned
// pad, never accidentally reading someone else's. Exported for that reuse —
// otherwise identical/private to this file for the normal Streaming flow.
export function useRetroPhysicalGamepadInput(enabled, sendButton, sendAnalog, consoleId, token, gamepadIndex = null) {
    const mappingRef = useRef(loadGamepadMapping(consoleId));
    const keyMappingRef = useRef({});
    const padIdRef = useRef(null);
    const unsubscribeKeyMappingRef = useRef(null);
    const controllerName = RETRO_CONTROLLER_PRESET_MAP[consoleId] || "default";

    useEffect(() => {
        mappingRef.current = loadGamepadMapping(consoleId);
        return subscribeGamepadMapping(consoleId, (mapping) => { mappingRef.current = mapping; });
    }, [consoleId]);

    useEffect(() => {
        if (!enabled) return;
        let raf;
        const pressed = {};
        let stickX = 0, stickY = 0;
        let rightX = 0, rightY = 0;

        const tick = () => {
            const pads = connectedGamepads();
            const pad = gamepadIndex === null ? Array.from(pads).find(Boolean) : pads[gamepadIndex];
            if (pad) {
                // A real pad swap (or the first pad found at all) — re-load
                // this pad's own KeyMapping for the session's virtual
                // controller and re-subscribe to live changes (e.g. edited
                // in profile/GamepadParameters.jsx in another tab).
                if (gamepadDeviceId(pad) !== padIdRef.current) {
                    padIdRef.current = gamepadDeviceId(pad);
                    unsubscribeKeyMappingRef.current?.();
                    keyMappingRef.current = getCachedKeyMapping(padIdRef.current, controllerName);
                    loadKeyMapping(token, padIdRef.current, controllerName).then((m) => { keyMappingRef.current = m; });
                    unsubscribeKeyMappingRef.current = subscribeKeyMapping(padIdRef.current, controllerName, (m) => { keyMappingRef.current = m; });
                }
                // Held indices (strings) → desired bits, with chord support:
                // a chord entry ("12+15" → DPAD_UP_RIGHT) fires only while
                // EVERY key of the chord is held and suppresses those keys'
                // individual bindings — same semantics as the keyboard hook.
                //
                // LE MENU OUVERT, LA MANETTE EST LUE COMME LÂCHÉE (#408) : la
                // différence avec ce qui était envoyé relâche alors tout, puis
                // plus rien ne part, jusqu'à la fermeture du menu. Le Home du
                // raccourci du menu (L2 tenu) n'est pas au jeu non plus.
                const suspended = isGameInputSuspended();
                const held = new Set();
                pad.buttons.forEach((button, index) => {
                    if (button.pressed && !suspended && !isMenuHome(pad, index)) held.add(String(index));
                });
                const desired = new Set();
                const chordCovered = new Set();
                for (const input of Object.keys(keyMappingRef.current)) {
                    if (!input.includes("+")) continue;
                    const codes = input.split("+");
                    if (!codes.every((code) => held.has(code))) continue;
                    for (const bit of resolveVirtualBits(keyMappingRef.current[input])) {
                        desired.add(bit);
                    }
                    for (const code of codes) chordCovered.add(code);
                }
                for (const code of held) {
                    if (chordCovered.has(code)) continue;
                    const bitIndex = resolveBitIndex(
                        code, keyMappingRef.current, mappingRef.current, RETRO_PHYSICAL_BUTTON_INDEX,
                        String(positionalButtonIndex(pad.id, Number(code))),
                    );
                    if (bitIndex !== undefined) desired.add(bitIndex);
                }
                for (const bit of desired) {
                    if (!pressed[bit]) {
                        pressed[bit] = true;
                        sendButton(bit, 1);
                    }
                }
                for (const bit of Object.keys(pressed)) {
                    const index = Number(bit);
                    if (!desired.has(index)) {
                        delete pressed[index];
                        sendButton(index, 0);
                    }
                }
                // Left stick (axes[0]/[1] in the standard mapping) — axis 0
                // = RETRO_DEVICE_INDEX_ANALOG_LEFT, same as VirtualJoystick
                // (see its own doc + handleAnalogMove above): a single
                // physical analog input (PSP's nub, N64's stick, ...) maps
                // to LEFT by libretro convention, matching what those cores
                // actually read. Cores that never poll RETRO_DEVICE_ANALOG
                // at all just ignore this either way, so this is harmless
                // for consoles with no analog input. Not remapped by
                // KeyMapping (see its own LEFT_STICK/RIGHT_STICK exclusion
                // doc) — stays on this fixed axis convention.
                const x = suspended ? 0 : pad.axes[0] || 0, y = suspended ? 0 : pad.axes[1] || 0;
                if (Math.abs(x - stickX) > 0.02 || Math.abs(y - stickY) > 0.02) {
                    stickX = x;
                    stickY = y;
                    sendAnalog(0, x, y);
                }

                // LE STICK DROIT AUSSI (axes[2]/[3] → RETRO_DEVICE_INDEX_
                // ANALOG_RIGHT), qui n'était pas lu du tout.
                //
                // Ce n'est pas un manque cosmétique. Sur N64, les boutons C
                // n'ont AUCUN bit numérique : les cores les lisent comme une
                // déviation du stick droit — c'est déjà ce que fait le pad
                // tactile (voir sendN64CState). Avec une manette physique,
                // les quatre C étaient donc muets, et « le B ne marche pas,
                // et d'autres aussi » (signalé le 2026-09-03).
                //
                // Et ça vaut au-delà : une DualShock sur PS1/PS2 n'avait pas
                // son second stick non plus.
                const rx = suspended ? 0 : pad.axes[2] || 0, ry = suspended ? 0 : pad.axes[3] || 0;
                if (Math.abs(rx - rightX) > 0.02 || Math.abs(ry - rightY) > 0.02) {
                    rightX = rx;
                    rightY = ry;
                    sendAnalog(1, rx, ry);
                }
            }
            raf = requestAnimationFrame(tick);
        };
        raf = requestAnimationFrame(tick);
        return () => {
            cancelAnimationFrame(raf);
            unsubscribeKeyMappingRef.current?.();
            unsubscribeKeyMappingRef.current = null;
            padIdRef.current = null;
        };
    }, [enabled, sendButton, sendAnalog, controllerName, token, gamepadIndex]);
}
