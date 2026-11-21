// Keyboard input for retro-gaming — relays keydown/keyup straight to
// sendButton via KeyMapping-first-then-libs/keyboardMapping.js (see
// resolveBitIndex's own doc in use-retro-physical-gamepad.js). Always
// enabled (unlike the physical gamepad hook, there is no "is a keyboard
// connected" concept to gate on). capture:true so a key still reaches this
// handler even when the <video> element (or nothing) has focus, same
// reasoning as remote-play's own keyboard-mapping.js handler. Digital
// buttons only — no analog stick from a keyboard.
//
// Chords: a mapping entry whose physical input contains "+" (e.g.
// "ArrowDown+ArrowRight" → DPAD_DOWN_RIGHT) fires only while EVERY key of
// the chord is held, and SUPPRESSES those keys' individual bindings — so a
// diagonal chord and its two single directions can coexist without the
// chord's release fighting the still-held single direction.
import { useEffect, useRef } from "react";
import { loadKeyboardMapping, subscribeKeyboardMapping } from "../../../libs/keyboardMapping.js";
import { getKeyboardDeviceId, getCachedKeyMapping, loadKeyMapping, subscribeKeyMapping } from "../../../libs/keyMapping.js";
import { RETRO_CONTROLLER_PRESET_MAP } from "../../../controllers/Controllers.jsx";
import { resolveBitIndex, resolveVirtualBits, RETRO_PHYSICAL_BUTTON_INDEX } from "./use-retrogaming-physical-gamepad.js";
import { isGameInputSuspended, onGameInputSuspension } from "../../../libs/gameInput.js";

export function useRetroKeyboardInput(enabled, sendButton, consoleId, token) {
    const mappingRef = useRef(loadKeyboardMapping(consoleId));
    const keyMappingRef = useRef({});
    const controllerName = RETRO_CONTROLLER_PRESET_MAP[consoleId] || "default";
    const keyboardId = getKeyboardDeviceId();

    useEffect(() => {
        mappingRef.current = loadKeyboardMapping(consoleId);
        return subscribeKeyboardMapping(consoleId, (mapping) => {
            mappingRef.current = mapping;
        });
    }, [consoleId]);

    useEffect(() => {
        keyMappingRef.current = getCachedKeyMapping(keyboardId, controllerName);
        loadKeyMapping(token, keyboardId, controllerName).then((m) => {
            keyMappingRef.current = m;
        });
        return subscribeKeyMapping(keyboardId, controllerName, (mapping) => {
            keyMappingRef.current = mapping;
        });
    }, [keyboardId, controllerName, token]);

    useEffect(() => {
        if (!enabled) return;
        const pressed = new Set();
        const sent = new Set();

        const isEditable = (el) =>
            el &&
            (el.tagName === "INPUT" ||
                el.tagName === "TEXTAREA" ||
                el.isContentEditable);

        const chordInputs = () =>
            Object.keys(keyMappingRef.current).filter((input) =>
                input.includes("+"),
            );

        const codeMatters = (code) =>
            resolveBitIndex(
                code,
                keyMappingRef.current,
                mappingRef.current,
                RETRO_PHYSICAL_BUTTON_INDEX,
            ) !== undefined ||
            chordInputs().some((input) => input.split("+").includes(code));

        // Recompute the desired bit set from the held keys and diff it against
        // what is currently sent — release ordering (one chord key lifted while
        // the other is still held) then resolves itself instead of special cases.
        const refresh = () => {
            const desired = new Set();
            const chordCovered = new Set();
            for (const input of chordInputs()) {
                const codes = input.split("+");
                if (codes.every((code) => pressed.has(code))) {
                    for (const bit of resolveVirtualBits(keyMappingRef.current[input])) {
                        desired.add(bit);
                    }
                    for (const code of codes) chordCovered.add(code);
                }
            }
            for (const code of pressed) {
                if (chordCovered.has(code)) continue;
                const bitIndex = resolveBitIndex(
                    code,
                    keyMappingRef.current,
                    mappingRef.current,
                    RETRO_PHYSICAL_BUTTON_INDEX,
                );
                if (bitIndex !== undefined) desired.add(bitIndex);
            }
            for (const bit of desired) {
                if (!sent.has(bit)) {
                    sent.add(bit);
                    sendButton(bit, 1);
                }
            }
            for (const bit of [...sent]) {
                if (!desired.has(bit)) {
                    sent.delete(bit);
                    sendButton(bit, 0);
                }
            }
        };

        const handleKeyDown = (e) => {
            // Une touche FABRIQUÉE n'est pas un joueur : le pont manette-
            // clavier de Skeleton en émet pour naviguer dans l'interface
            // (sans `code`, sans relâchement). Et le menu ouvert, les flèches
            // et Entrée sont à lui (#407, #408).
            if (!e.isTrusted || isGameInputSuspended()) return;
            if (isEditable(document.activeElement)) return;
            if (pressed.has(e.code)) return;
            if (!codeMatters(e.code)) return;
            e.preventDefault();
            pressed.add(e.code);
            refresh();
        };
        const handleKeyUp = (e) => {
            if (!pressed.has(e.code)) return;
            e.preventDefault();
            pressed.delete(e.code);
            refresh();
        };
        const releaseAll = () => {
            pressed.clear();
            refresh();
        };

        document.addEventListener("keydown", handleKeyDown, true);
        document.addEventListener("keyup", handleKeyUp, true);
        window.addEventListener("blur", releaseAll);
        // Le menu qui s'ouvre relâche ce que le clavier tenait.
        const stopListening = onGameInputSuspension((suspended) => suspended && releaseAll());
        return () => {
            document.removeEventListener("keydown", handleKeyDown, true);
            document.removeEventListener("keyup", handleKeyUp, true);
            window.removeEventListener("blur", releaseAll);
            stopListening();
            releaseAll();
        };
    }, [enabled, sendButton]);
}
