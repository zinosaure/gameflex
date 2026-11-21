import React, { useEffect, useMemo } from "react";
import { defaultMappingForController, positionalButtonIndex } from "../../libs/keyMapping.js";
import { PRESET_COMPONENT_BY_NAME } from "../../controllers/Controllers.jsx";

// Gamepad API standard-layout index → profile button id — the same order as
// the streamex pads (components/NintendoSwitch.jsx), so a physical controller
// lands on the same buttons.
const GAMEPAD_ORDER = [
    "a",
    "b",
    "x",
    "y",
    "l",
    "r",
    "zl",
    "zr",
    "minus",
    "plus",
    "lstick",
    "rstick",
    "dup",
    "ddown",
    "dleft",
    "dright",
];

const GAMEPAD_POLL_MS = 50;
// Hall-effect triggers (L2/R2) report continuous values; a plain digital
// button only ever reports 0 or 1. Once a trigger has been seen BETWEEN 0
// and 1, it is treated as analog: press/release thresholds move to 5 % and
// every ≥1 % change is relayed as a pressure row (ab). Digital triggers
// keep the plain kd/ku press/release.
const TRIGGER_ANALOG_PRESS = 0.05;
const TRIGGER_PERCENT_STEP = 10; // 1 % of the 0..1000 scale

// Stick names from the virtual pad → the wire command's stick id.
const STICK_WHICH = { LEFT_STICK: "l", RIGHT_STICK: "r" };

// Two held d-pad indices (standard order) → the preset's diagonal name — the
// same chord vocabulary the mapping capture/runtime uses.
const DPAD_CHORD_BY_DIRECTIONS = {
    "12+14": "DPAD_UP_LEFT",
    "12+15": "DPAD_UP_RIGHT",
    "13+14": "DPAD_DOWN_LEFT",
    "13+15": "DPAD_DOWN_RIGHT",
};

// The keytest pad: the input layer for the keytest console. It relays the
// keyboard (kd/ku by event.code) AND the physical gamepads (Gamepad API,
// resolved through the preset's button names so a real pad logs exactly like
// the virtual one, diagonals included), plus both sticks (st). An optional
// ControllerComponent renders the preset's virtual controller on top — pages
// that have no preset (a joiner in the generic streaming page) pass nothing
// and get the input layer alone.
export default function KeyTestPad({
    profileButtons = [],
    enabled,
    sendInput,
    ControllerComponent,
    controllerName,
    // Les rappels du chemin rétro (le hook les passe à toute manette qu'il
    // rend) — c'est par eux que `emit` sort quand il n'y a pas de `sendInput`.
    onButtonDown,
    onButtonUp,
    onAnalogMove,
}) {
    // LE PAD NE CONNAÎT PLUS LE FIL.
    //
    // Il n'écrivait que des lignes streamex (`kd,CROSS`) via `sendInput`.
    // Depuis que le téléphone héberge KeyTest, la même manette sert AUSSI le
    // chemin rétro, qui n'a pas de `sendInput` mais des rappels nommés — et
    // chaque pression y levait, faute des deux props qu'elle attendait.
    //
    // `emit` traduit au dernier mètre, exactement comme la convention le veut
    // (le protocole est une vraie différence de plateforme, la FORME vue par
    // le pad ne doit pas l'être). Une ligne qu'on ne sait pas traduire est
    // ignorée : côté rétro, le clavier a déjà son propre chemin.
    const emit = (line) => {
        if (sendInput) {
            sendInput(line);

            return;
        }
        const [kind, first, second, third] = String(line).split(",");
        if (kind === "kd") {
            onButtonDown?.(first);
        } else if (kind === "ku") {
            onButtonUp?.(first);
        } else if (kind === "st") {
            onAnalogMove?.(
                first === "r" ? "RIGHT_STICK" : "LEFT_STICK",
                Number(second) / 1000,
                Number(third) / 1000,
            );
        }
    };

    // Keyboard pass-through — physical keys relayed verbatim (kd/ku by
    // event.code). Code-less events (some gamepad→keyboard drivers dispatch
    // them) are dropped, and keydown stops propagation in the capture phase
    // so a driver-emulated Escape/Back cannot close the offcanvas mid-test.
    useEffect(() => {
        if (!enabled) return;
        const keydown = (event) => {
            if (event.repeat || !event.code) return;
            event.preventDefault();
            event.stopPropagation();
            emit(`kd,${event.code}`);
        };
        const keyup = (event) => {
            if (!event.code) return;
            emit(`ku,${event.code}`);
        };
        window.addEventListener("keydown", keydown, true);
        window.addEventListener("keyup", keyup, true);
        return () => {
            window.removeEventListener("keydown", keydown, true);
            window.removeEventListener("keyup", keyup, true);
        };
    }, [enabled, sendInput]);

    // Physical gamepads — logical inputs computed from the HELD buttons:
    // singles resolve through the preset's own button names (the same table
    // the auto-mapping uses), and a held d-pad diagonal resolves as ONE
    // chord (DPAD_DOWN_RIGHT) that suppresses its two directions — so a
    // physical pad logs exactly like the virtual one.
    useEffect(() => {
        if (!enabled) return;
        const nameByIndex = defaultMappingForController(controllerName) || {};
        const active = new Set(); // wire names currently sent down
        const lastStick = new Map(); // `${gamepad.index}-${which}` → last "x,y" sent
        const analogTriggers = new Set(); // trigger indices seen between 0 and 1 (hall effect)
        const triggerLast = new Map(); // `${gamepad.index}-${index}` → last percent sent
        const timer = setInterval(() => {
            for (const gamepad of navigator.getGamepads ? navigator.getGamepads() : []) {
                if (!gamepad || !gamepad.connected) continue;
                const held = new Set();
                gamepad.buttons.forEach((button, index) => {
                    const value =
                        typeof button.value === "number" ? button.value : button.pressed ? 1 : 0;
                    if ((index === 6 || index === 7) && value > 0 && value < 1) {
                        analogTriggers.add(index);
                    }
                    const down = analogTriggers.has(index)
                        ? value >= TRIGGER_ANALOG_PRESS
                        : button.pressed || value > 0.5;
                    if (down) held.add(String(index));
                });
                const heldDirections = ["12", "13", "14", "15"].filter((index) => held.has(index));
                const chord =
                    heldDirections.length === 2
                        ? DPAD_CHORD_BY_DIRECTIONS[[...heldDirections].sort().join("+")]
                        : null;
                const desired = new Set();
                if (chord) desired.add(chord);
                for (let index = 0; index < GAMEPAD_ORDER.length; index++) {
                    if (!held.has(String(index))) continue;
                    if (chord && index >= 12 && index <= 15) continue; // covered by the chord
                    const name = nameByIndex[String(positionalButtonIndex(gamepad.id, index))];
                    const key =
                        name ||
                        profileButtons.find((entry) => entry.id === GAMEPAD_ORDER[positionalButtonIndex(gamepad.id, index)])?.key;
                    if (key) desired.add(key);
                }
                for (const key of [...active]) {
                    if (!desired.has(key)) {
                        active.delete(key);
                        emit(`ku,${key}`);
                    }
                }
                for (const key of desired) {
                    if (!active.has(key)) {
                        active.add(key);
                        emit(`kd,${key}`);
                    }
                }
                // Hall-effect trigger pressure — one ab row per ≥1 % change
                // while held. The release itself goes through ku (the display
                // renders a "(Released)" row), so ab,0 is never sent.
                for (const index of analogTriggers) {
                    const key =
                        nameByIndex[String(positionalButtonIndex(gamepad.id, index))] ||
                        profileButtons.find((entry) => entry.id === GAMEPAD_ORDER[positionalButtonIndex(gamepad.id, index)])?.key;
                    if (!key) continue;
                    const value =
                        typeof gamepad.buttons[index]?.value === "number"
                            ? gamepad.buttons[index].value
                            : 0;
                    const percent = Math.round(value * 1000);
                    if (percent <= 0) continue;
                    const stamp = `${gamepad.index}-${index}`;
                    const last = triggerLast.get(stamp);
                    if (last === undefined || Math.abs(percent - last) >= TRIGGER_PERCENT_STEP) {
                        triggerLast.set(stamp, percent);
                        emit(`ab,${key},${percent}`);
                    }
                }
                // Standard Gamepad API axes: [0,1] = left stick, [2,3] =
                // right stick. Only CHANGED positions are sent (a held stick
                // re-sending every poll would spam the WS for nothing).
                const sticks = {
                    l: [gamepad.axes[0] || 0, gamepad.axes[1] || 0],
                    r: [gamepad.axes[2] || 0, gamepad.axes[3] || 0],
                };
                for (const [which, axes] of Object.entries(sticks)) {
                    const x = Math.round(axes[0] * 1000);
                    const y = Math.round(axes[1] * 1000);
                    const stamp = `${gamepad.index}-${which}`;
                    const value = `${x},${y}`;
                    if (lastStick.get(stamp) !== value) {
                        lastStick.set(stamp, value);
                        emit(`st,${which},${x},${y}`);
                    }
                }
            }
        }, GAMEPAD_POLL_MS);
        return () => clearInterval(timer);
    }, [enabled, controllerName, profileButtons, sendInput]);

    if (!ControllerComponent) {
        return null; // input layer only (no preset to draw)
    }

    return (
        <ControllerComponent
            onButtonDown={(name) => {
                if (!enabled) return;
                // Without a preset (joiner) the virtual pad's names are the
                // profile ids — resolve them to the profile's wire keys so
                // the display logs the same label as a physical pad.
                const key = profileButtons.find((entry) => entry.id === name)?.key || name;
                emit(`kd,${key}`);
            }}
            onButtonUp={(name) => {
                if (!enabled) return;
                const key = profileButtons.find((entry) => entry.id === name)?.key || name;
                emit(`ku,${key}`);
            }}
            onAnalogMove={(name, x, y) => {
                if (!enabled) return;
                const which = STICK_WHICH[name];
                if (!which) return;
                const clamp = (value) => Math.max(-1000, Math.min(1000, Math.round(value * 1000)));
                emit(`st,${which},${clamp(x)},${clamp(y)}`);
            }}
        />
    );
}


/**
 * Le pad de KeyTest pour un preset donné — la décision, sortie du hook.
 *
 * Elle vivait dans `useStreamexInput`, ce qui la rendait invisible à toute
 * plateforme qui ne serait pas streamex. Or KeyTest n'appartient pas à une
 * plateforme : c'est un conteneur sur le serveur et une vue dessinée sur le
 * téléphone (KeytestHost.java), et la page qui l'affiche est la même. Le pad
 * ne dépend que du preset qu'on teste.
 *
 * Sans preset — quelqu'un qui rejoint depuis le tableau de bord — c'est le
 * pad nu de la couche d'entrée.
 */
export function useKeytestPad(preview) {
    return useMemo(() => {
        if (!preview) {
            return KeyTestPad;
        }
        const PresetComponent = PRESET_COMPONENT_BY_NAME[preview];

        return (padProps) => (
            <KeyTestPad
                {...padProps}
                ControllerComponent={PresetComponent}
                controllerName={preview}
            />
        );
    }, [preview]);
}
