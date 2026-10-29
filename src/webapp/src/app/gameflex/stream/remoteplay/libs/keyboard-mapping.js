import {
    CONTROLLER_MAPPING_STORAGE_KEY,
    getKeyboardToButtonMapping,
    getLeftStickKeyVectorMap,
    subscribeControllerMappings,
} from "@/types/controller-mapping";
import { debugTrace } from "@/gameflex/shared/utils/log-store";
import { isGameInputSuspended, onGameInputSuspension } from "@/gameflex/libs/gameInput.js";

// Unmapped keys are logged at most once per distinct key — holding a
// movement key generates OS key-repeat events that would otherwise flood
// the trace on every repeat.
const loggedUnmappedKeys = new Set();

// Skeleton's global spatial navigation (skeleton.js's useKeyboard →
// navigateFocus) listens on window and moves focus onto the menu buttons
// on every arrow press. Streaming pages own the arrows: they belong to the
// game, never to the UI (see the stopPropagation in handleKeyDown).
const NAV_KEYS = ["ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"];

/**
 * Keyboard event handler
 */

export function createKeyboardHandler(onButtonPress, options = {}) {
    const { onLeftStickChange } = options;
    let keyboardMapping = getKeyboardToButtonMapping();
    let leftStickKeyMap = getLeftStickKeyVectorMap();

    const pressedKeys = new Set();
    const activeLeftStickKeys = new Set();

    const refreshKeyboardMapping = (mappings) => {
        keyboardMapping = getKeyboardToButtonMapping(mappings ?? undefined);
        leftStickKeyMap = getLeftStickKeyVectorMap(mappings ?? undefined);
    };

    const unsubscribeMappingChange = subscribeControllerMappings((mappings) => {
        refreshKeyboardMapping(mappings);
    });

    const handleStorageChange = (event) => {
        if (event.key && event.key !== CONTROLLER_MAPPING_STORAGE_KEY) {
            return;
        }
        refreshKeyboardMapping();
    };

    if (typeof window !== "undefined") {
        window.addEventListener("storage", handleStorageChange);
    }

    const recomputeLeftStick = () => {
        if (!onLeftStickChange) {
            return;
        }

        let x = 0;
        let y = 0;

        activeLeftStickKeys.forEach((key) => {
            const vector = leftStickKeyMap[key];
            if (vector) {
                x += vector.x;
                y += vector.y;
            }
        });

        if (x !== 0 || y !== 0) {
            const magnitude = Math.hypot(x, y);
            if (magnitude > 1) {
                x /= magnitude;
                y /= magnitude;
            }
        }

        onLeftStickChange(x, y);
    };

    const handleKeyDown = (event) => {
        // Ni une touche FABRIQUÉE — le pont manette-clavier de Skeleton, qui
        // navigue dans l'interface : ses flèches devenaient ici un stick tenu
        // sans relâchement —, ni une touche du menu ouvert, à qui reviennent
        // les flèches et Entrée (#407, #408).
        if (!event.isTrusted || isGameInputSuspended()) {
            return;
        }
        refreshKeyboardMapping();

        // Skip keyboard events when an input or textarea is focused (except Escape)
        const activeElement = document.activeElement;
        const isInputFocused =
            activeElement &&
            (activeElement.tagName === "INPUT" ||
                activeElement.tagName === "TEXTAREA" ||
                activeElement.isContentEditable);

        // Always handle Escape
        if (event.key === "Escape" || event.code === "Escape") {
            if (isInputFocused && activeElement instanceof HTMLElement) {
                activeElement.blur();
            }
        } else if (isInputFocused) {
            // Other keys are ignored when an input is focused
            return;
        }

        // This handler runs in the document CAPTURE phase — stopping the
        // arrows here kills them before they can reach Skeleton's window-
        // level spatial navigation, for real keyboard arrows AND for
        // Skeleton.useGamepad's synthetic keydowns (a physical gamepad's
        // d-pad/left stick) alike: in a game page they must never move
        // focus onto the menu buttons.
        if (NAV_KEYS.includes(event.key)) {
            event.stopPropagation();
        }

        // Prevent default browser behaviour for navigational keys
        const keyCode = event.code || event.key;
        if (keyboardMapping[keyCode]) {
            if (
                ["Space", "Tab", "ArrowUp", "ArrowDown", "ArrowLeft", "ArrowRight"].includes(
                    keyCode
                )
            ) {
                event.preventDefault();
            }
        }

        if (leftStickKeyMap[keyCode]) {
            if (!activeLeftStickKeys.has(keyCode)) {
                activeLeftStickKeys.add(keyCode);
                recomputeLeftStick();
            }
        }

        // Handle key press
        const buttonName = keyboardMapping[keyCode];
        if (buttonName) {
            if (!pressedKeys.has(keyCode)) {
                pressedKeys.add(keyCode);
                debugTrace("⌨️ Key down:", keyCode, "->", buttonName, "press");
                void onButtonPress(buttonName, "press");
            }
        } else if (!loggedUnmappedKeys.has(keyCode)) {
            // Log unmapped keys for debugging (once per distinct key)
            loggedUnmappedKeys.add(keyCode);
            debugTrace("⌨️ Unmapped key:", keyCode, event.key);
        }
    };

    const handleKeyUp = (event) => {
        refreshKeyboardMapping();

        const keyCode = event.code || event.key;
        const buttonName = keyboardMapping[keyCode];

        if (buttonName && pressedKeys.has(keyCode)) {
            pressedKeys.delete(keyCode);
            debugTrace("⌨️ Key up:", keyCode, "->", buttonName, "release");
            void onButtonPress(buttonName, "release");
        }

        if (leftStickKeyMap[keyCode] && activeLeftStickKeys.has(keyCode)) {
            activeLeftStickKeys.delete(keyCode);
            recomputeLeftStick();
        }
    };

    const handleBlur = () => {
        // Release all pressed buttons
        pressedKeys.forEach((keyCode) => {
            const buttonName = keyboardMapping[keyCode];
            if (buttonName) {
                void onButtonPress(buttonName, "release");
            }
        });
        pressedKeys.clear();

        if (activeLeftStickKeys.size > 0) {
            activeLeftStickKeys.clear();
            recomputeLeftStick();
        }
    };

    // Use capture mode so keyboard events are caught even when a video element is focused
    document.addEventListener("keydown", handleKeyDown, true);
    document.addEventListener("keyup", handleKeyUp, true);
    window.addEventListener("blur", handleBlur);
    // Le menu qui s'ouvre relâche ce que le clavier tenait, comme une perte
    // du focus.
    const stopListening = onGameInputSuspension((suspended) => suspended && handleBlur());

    // Return cleanup function
    return () => {
        document.removeEventListener("keydown", handleKeyDown, true);
        document.removeEventListener("keyup", handleKeyUp, true);
        window.removeEventListener("blur", handleBlur);
        stopListening();
        unsubscribeMappingChange();
        if (typeof window !== "undefined") {
            window.removeEventListener("storage", handleStorageChange);
        }
        // Release all pressed buttons
        pressedKeys.forEach((keyCode) => {
            const buttonName = keyboardMapping[keyCode];
            if (buttonName) {
                void onButtonPress(buttonName, "release");
            }
        });
        pressedKeys.clear();

        if (activeLeftStickKeys.size > 0) {
            activeLeftStickKeys.clear();
            recomputeLeftStick();
        } else if (onLeftStickChange) {
            onLeftStickChange(0, 0);
        }
    };
}
