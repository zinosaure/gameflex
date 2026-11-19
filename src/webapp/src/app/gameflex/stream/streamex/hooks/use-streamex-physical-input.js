import { useEffect, useRef } from "react";
import { connectedGamepads, positionalButtonIndex } from "../../../libs/keyMapping.js";
import { isGameInputSuspended, onGameInputSuspension, isMenuHome } from "../../../libs/gameInput.js";

// Reading a physical controller must NOT depend on drawing one.
//
// This lived inside NintendoSwitch.jsx / PlayStation3.jsx / Xbox.jsx — three
// copies of the same two effects, mounted with the on-screen pad. And the page
// removes the on-screen pad exactly when a real controller is connected
// (Streaming.jsx: `!hasPhysicalGamepad`), so plugging a pad in unmounted the
// only code that read it. Streamex was the only platform with the bug because
// it was the only one reading from inside a visual component: retro-gaming has
// use-retrogaming-physical-gamepad.js, remote-play has gamepad.service.js, and
// neither is mounted by a pad (reported live 2026-08-25).
//
// So the rule this file exists to enforce: input is a session concern, the
// on-screen pad is a display concern. Hiding one must never silence the other.
//
// Keyboard travels with it for the same reason — it was in the same component,
// so it died with it. A hidden pad silenced the keyboard too; nobody noticed
// because hiding only happens when a gamepad is present.

const GAMEPAD_POLL_MS = 50;

export function useStreamexPhysicalInput({ enabled, order, profileButtons, sendInput }) {
    const pressedRef = useRef(new Set());

    // Keyboard pass-through — physical keys are relayed verbatim (kd/ku by
    // event.code): the emulator receives the same events as a local keyboard,
    // so its whole menu (Esc, arrows…) works, not just the mapped buttons.
    useEffect(() => {
        if (!enabled) {
            return undefined;
        }

        const pressed = pressedRef.current;
        const releaseAll = () => {
            for (const code of pressed) {
                sendInput(`ku,${code}`);
            }
            pressed.clear();
        };
        const handleKeyDown = (event) => {
            // Ni une touche FABRIQUÉE — le pont manette-clavier de Skeleton,
            // qui navigue dans l'interface et partait ici en « kd, » sans
            // code, jamais relâché —, ni une touche du menu ouvert (#407,
            // #408).
            if (!event.isTrusted || !event.code || isGameInputSuspended()) {
                return;
            }
            if (event.repeat || pressed.has(event.code)) {
                return;
            }
            pressed.add(event.code);
            sendInput(`kd,${event.code}`);
        };
        const handleKeyUp = (event) => {
            if (!pressed.has(event.code)) {
                return;
            }
            pressed.delete(event.code);
            sendInput(`ku,${event.code}`);
        };

        window.addEventListener("keydown", handleKeyDown);
        window.addEventListener("keyup", handleKeyUp);
        // Le menu qui s'ouvre relâche ce que le clavier tenait.
        const stopListening = onGameInputSuspension((suspended) => suspended && releaseAll());

        return () => {
            window.removeEventListener("keydown", handleKeyDown);
            window.removeEventListener("keyup", handleKeyUp);
            stopListening();
            // Whatever was held when this stops is released: losing the pad
            // mid-press must not leave a key stuck down on the emulator.
            releaseAll();
        };
    }, [enabled, sendInput]);

    // Physical gamepads (Gamepad API, standard layout) — button indices map
    // onto profile ids through `order`, axes 0/1 and 2/3 travel as `st`.
    useEffect(() => {
        if (!enabled || !order) {
            return undefined;
        }

        const padState = new Map();
        const stickLast = new Map();
        const sendStick = (which, axisX, axisY) => {
            const x = Math.round((axisX || 0) * 1000);
            const y = Math.round((axisY || 0) * 1000);
            const key = `${x},${y}`;
            if (stickLast.get(which) === key) {
                return;
            }
            stickLast.set(which, key);
            sendInput(`st,${which},${x},${y}`);
        };

        const timer = setInterval(() => {
            // LE MENU OUVERT, LA MANETTE EST LUE COMME LÂCHÉE (#408) : les
            // boutons tenus repartent relâchés, les sticks au centre.
            const suspended = isGameInputSuspended();
            for (const gamepad of connectedGamepads()) {
                if (!gamepad || !gamepad.connected) {
                    continue;
                }
                gamepad.buttons.forEach((button, index) => {
                    // Par position : une manette à étiquettes Nintendo envoie
                    // son A (à droite) comme le bouton du bas (keyMapping.js).
                    const id = order[positionalButtonIndex(gamepad.id, index)];
                    if (!id) {
                        return;
                    }
                    // Le Home du raccourci du menu (L2 tenu) n'est pas au jeu.
                    const down = !suspended && !isMenuHome(gamepad, index)
                        && (button.pressed || (typeof button.value === "number" && button.value > 0.5));
                    if (down === !!padState.get(id)) {
                        return;
                    }
                    padState.set(id, down);
                    const buttonProfile = profileButtons.find((entry) => entry.id === id);
                    if (buttonProfile) {
                        sendInput(down ? `kd,${buttonProfile.key}` : `ku,${buttonProfile.key}`);
                    }
                });
                if (gamepad.axes && gamepad.axes.length >= 2) {
                    sendStick("l", suspended ? 0 : gamepad.axes[0], suspended ? 0 : gamepad.axes[1]);
                    if (gamepad.axes.length >= 4) {
                        sendStick("r", suspended ? 0 : gamepad.axes[2], suspended ? 0 : gamepad.axes[3]);
                    }
                }
            }
        }, GAMEPAD_POLL_MS);

        return () => {
            clearInterval(timer);
            // Center both sticks so the emulator does not keep a deflection
            // held after the pad stops being read.
            sendInput("st,l,0,0");
            sendInput("st,r,0,0");
        };
    }, [enabled, order, profileButtons, sendInput]);
}
