import { deleteKeyMapping, saveKeyMapping } from "./api.js";
import { AUTO_MAP_CONTROLLERS, defaultMappingForController } from "../engine/controllers/default-mapping.js";

// LA CORRESPONDANCE DES TOUCHES D'UNE MANETTE PHYSIQUE (libs/keyMapping.js de la webapp) : pour un
// appareil (`gamepad.id`) et une manette virtuelle, `{ "<bouton ou accord>": "<nom du bouton virtuel>" }`,
// gardée sur le compte (`/key-mapping`, le service des comptes) — elle vaut donc aussi pour les parties
// que la webapp ouvre. Les correspondances par défaut (« Auto ») sont celles du moteur, lues dans
// son propre fichier (engine/controllers/default-mapping.js).

// L'IDENTIFIANT DU CLAVIER, celui d'une manette, les manettes branchées : ceux du moteur (engine/libs/keyMapping.js),
// une seule définition — les Réglages et une partie reconnaissent un appareil de la même façon, sinon une correspondance
// enregistrée ici ne serait jamais relue là. Les retours tactiles, les vibrations et les sticks : une seule source,
// engine/libs/gamepadPreferences.js (#758).
// LA LECTURE AUSSI, une seule (chantier 85) : celle du moteur, qui garde la correspondance lue pour la partie.
export { connectedGamepads, gamepadDeviceId, getKeyboardDeviceId as keyboardDeviceId, loadKeyMapping } from "../engine/libs/keyMapping.js";
export { deleteKeyMapping };

// Lier un bouton virtuel à une entrée : celle qu'il avait part, l'entrée prend ce seul bouton.
export function remapKey(mapping, deviceId, deviceType, controllerName, buttonName, input) {
    const next = Object.fromEntries(Object.entries(mapping).filter(([, name]) => name !== buttonName));
    next[input] = buttonName;

    return { mapping: next, saved: saveKeyMapping(deviceId, deviceType, controllerName, next) };
}

// « Auto » : la correspondance par défaut, pour chaque manette virtuelle qui en a une. Le nombre écrit.
export async function applyAutoMapping(deviceId, deviceType = "gamepad") {
    let applied = 0;

    for (const controllerName of AUTO_MAP_CONTROLLERS) {
        const mapping = defaultMappingForController(controllerName, deviceId);

        if (mapping && (await saveKeyMapping(deviceId, deviceType, controllerName, mapping)).ok) {
            applied += 1;
        }
    }

    return applied;
}

// ÉCOUTER UN ACCORD AU CLAVIER : les touches enfoncées ensemble (« ControlLeft+KeyA »), finies au
// relâchement de la dernière ; rien pendant `idleMs` : `onDone("")`. Les touches ne vont pas à la
// navigation pendant l'écoute (écoutées avant elle, et arrêtées). Rend de quoi l'arrêter.
export function captureKeyboardChord({ onChange, onDone, idleMs = 8000 }) {
    const chord = new Set();
    const held = new Set();
    const text = () => [...chord].sort().join("+");
    const timer = setTimeout(() => {
        if (!chord.size) {
            stop();
            onDone("");
        }
    }, idleMs);
    const down = (event) => {
        event.preventDefault();
        event.stopPropagation();

        if (!event.repeat && !chord.has(event.code)) {
            chord.add(event.code);
            onChange(text());
        }
        held.add(event.code);
    };
    const up = (event) => {
        event.preventDefault();
        event.stopPropagation();
        held.delete(event.code);

        if (chord.size && !held.size) {
            stop();
            onDone(text());
        }
    };
    function stop() {
        clearTimeout(timer);
        window.removeEventListener("keydown", down, true);
        window.removeEventListener("keyup", up, true);
    }
    window.addEventListener("keydown", down, true);
    window.addEventListener("keyup", up, true);

    return stop;
}

// ÉCOUTER UN ACCORD sur une manette : chaque bouton enfoncé s'ajoute (« 4+5 ») ; l'accord est fini
// quand tous sont relâchés — à la manette, aucun clic n'est nécessaire pour le valider (la webapp
// attend un clic). `onChange` à chaque bouton ajouté, `onDone` à la fin ; rien d'enfoncé pendant
// `idleMs` : `onDone("")`, l'écoute s'abandonne (B serait un bouton de l'accord). Rend de quoi l'arrêter.
export function captureGamepadChord(gamepadIndex, { onChange, onDone, idleMs = 8000 }) {
    const started = performance.now();
    let raf = 0;
    let previous = null;
    const chord = new Set();
    const pressed = (button) => Boolean(button?.pressed) || (typeof button?.value === "number" && button.value > 0.5);
    const text = () => [...chord].map(Number).sort((a, b) => a - b).join("+");

    const check = () => {
        const pad = (navigator.getGamepads?.() || [])[gamepadIndex];

        if (pad) {
            const buttons = pad.buttons || [];

            if (previous === null || previous.length !== buttons.length) {
                previous = buttons.map(pressed);
            }
            let held = false;
            buttons.forEach((button, index) => {
                const down = pressed(button);
                held = held || down;

                if (down && !previous[index]) {
                    chord.add(String(index));
                    onChange(text());
                }
                previous[index] = down;
            });

            if (chord.size && !held) {
                onDone(text());

                return;
            }
        }

        if (!chord.size && performance.now() - started > idleMs) {
            onDone("");

            return;
        }
        raf = requestAnimationFrame(check);
    };
    raf = requestAnimationFrame(check);

    return () => cancelAnimationFrame(raf);
}
