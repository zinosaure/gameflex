export const CONTROLLER_MAPPING_STORAGE_KEY = "controller_mappings";
const CONTROLLER_MAPPING_CHANGED_EVENT = "controller-mapping-changed";

const CONTROLLER_BUTTONS = [
    "CROSS",
    "CIRCLE",
    "TRIANGLE",
    "SQUARE",
    "L1",
    "R1",
    "L2",
    "R2",
    "L3",
    "R3",
    "DPAD_UP",
    "DPAD_DOWN",
    "DPAD_LEFT",
    "DPAD_RIGHT",
    "DPAD_UP_LEFT",
    "DPAD_UP_RIGHT",
    "DPAD_DOWN_LEFT",
    "DPAD_DOWN_RIGHT",
    "OPTIONS",
    "SHARE",
    "TOUCHPAD",
    "PS",
];


const DEFAULT_BUTTON_MAPPINGS = {
    CROSS: { button: "CROSS", key: "KeyZ" },
    CIRCLE: { button: "CIRCLE", key: "KeyX" },
    TRIANGLE: { button: "TRIANGLE", key: "KeyC" },
    SQUARE: { button: "SQUARE", key: "KeyV" },
    L1: { button: "L1", key: "KeyQ" },
    R1: { button: "R1", key: "KeyW" },
    L2: { button: "L2", key: "KeyE" },
    R2: { button: "R2", key: "KeyR" },
    L3: { button: "L3", key: "KeyA" },
    R3: { button: "R3", key: "KeyS" },
    DPAD_UP: { button: "DPAD_UP", key: "Numpad8" },
    DPAD_DOWN: { button: "DPAD_DOWN", key: "Numpad2" },
    DPAD_LEFT: { button: "DPAD_LEFT", key: "Numpad4" },
    DPAD_RIGHT: { button: "DPAD_RIGHT", key: "Numpad6" },
    DPAD_UP_LEFT: { button: "DPAD_UP_LEFT", key: "Numpad7" },
    DPAD_UP_RIGHT: { button: "DPAD_UP_RIGHT", key: "Numpad9" },
    DPAD_DOWN_LEFT: { button: "DPAD_DOWN_LEFT", key: "Numpad1" },
    DPAD_DOWN_RIGHT: { button: "DPAD_DOWN_RIGHT", key: "Numpad3" },
    OPTIONS: { button: "OPTIONS", key: "KeyP" },
    SHARE: { button: "SHARE", key: "KeyO" },
    TOUCHPAD: { button: "TOUCHPAD", key: "KeyI" },
    PS: { button: "PS", key: "KeyU" },
};

const LEFT_STICK_DIRECTIONS = ["UP", "DOWN", "LEFT", "RIGHT"];

const DEFAULT_LEFT_STICK_MAPPING = {
    UP: "ArrowUp",
    DOWN: "ArrowDown",
    LEFT: "ArrowLeft",
    RIGHT: "ArrowRight",
};

const DPAD_STREAMING_NAME_MAP = {
    DPAD_UP: "UP",
    DPAD_DOWN: "DOWN",
    DPAD_LEFT: "LEFT",
    DPAD_RIGHT: "RIGHT",
    DPAD_UP_LEFT: "UP_LEFT",
    DPAD_UP_RIGHT: "UP_RIGHT",
    DPAD_DOWN_LEFT: "DOWN_LEFT",
    DPAD_DOWN_RIGHT: "DOWN_RIGHT",
    CROSS: "CROSS",
    CIRCLE: "CIRCLE",
    TRIANGLE: "TRIANGLE",
    SQUARE: "SQUARE",
    L1: "L1",
    R1: "R1",
    L2: "L2",
    R2: "R2",
    L3: "L3",
    R3: "R3",
    OPTIONS: "OPTIONS",
    SHARE: "SHARE",
    TOUCHPAD: "TOUCHPAD",
    PS: "PS",
};

// Derived from DEFAULT_BUTTON_MAPPINGS (the single source of truth for
// default key bindings) rather than hand-duplicated. It used to be a
// separately-authored literal here, and it silently drifted out of sync
// with DEFAULT_BUTTON_MAPPINGS (e.g. this table said "Space" pressed CROSS
// while DEFAULT_BUTTON_MAPPINGS said CROSS's default key was "Enter") — the
// mismatch was harmless right up until the FIRST remap was ever saved: past
// that point sanitizeButtonMappings (below) always fully populates every
// button, not just the one just remapped, so getKeyboardToButtonMapping's
// override loop then treats every OTHER, untouched button as "explicitly
// bound" too and applies DEFAULT_BUTTON_MAPPINGS' conflicting key over
// whatever this table had — every binding but the one just changed appeared
// to silently scramble. Deriving this table instead of hand-authoring it
// makes that class of bug impossible: there is only one table to edit.
const DEFAULT_KEYBOARD_MAPPING = CONTROLLER_BUTTONS.reduce((acc, button) => {
    const key = DEFAULT_BUTTON_MAPPINGS[button]?.key;
    if (key) {
        acc[key] = DPAD_STREAMING_NAME_MAP[button] ?? button;
    }
    return acc;
}, {});

const isBrowser = () => typeof window !== "undefined";

const getDefaultButtonMappings = () =>
    CONTROLLER_BUTTONS.reduce((acc, button) => {
        const defaultMapping = DEFAULT_BUTTON_MAPPINGS[button];
        acc[button] = defaultMapping ? { ...defaultMapping } : { button };
        return acc;
    }, {});

const getDefaultLeftStickMapping = () => ({
    ...DEFAULT_LEFT_STICK_MAPPING,
});

const getStreamingButtonName = (button) => DPAD_STREAMING_NAME_MAP[button] ?? button;

export const getDefaultControllerMappings = () => ({
    buttons: getDefaultButtonMappings(),
    leftStick: getDefaultLeftStickMapping(),
});

const sanitizeButtonMappings = (input) => {
    const result = getDefaultButtonMappings();

    if (!input || typeof input !== "object") {
        return result;
    }

    CONTROLLER_BUTTONS.forEach((button) => {
        const raw = input[button];
        if (!raw || typeof raw !== "object") {
            return;
        }

        const hasKeyProp = Object.prototype.hasOwnProperty.call(raw, "key");
        const hasMouseProp = Object.prototype.hasOwnProperty.call(raw, "mouseButton");

        const mapping = { button };

        if (hasKeyProp && typeof raw.key === "string") {
            mapping.key = raw.key;
        }

        if (hasMouseProp) {
            const mouseButton =
                typeof raw.mouseButton === "number" && Number.isFinite(raw.mouseButton)
                    ? raw.mouseButton
                    : undefined;
            if (mouseButton !== undefined) {
                mapping.mouseButton = mouseButton;
                mapping.isMouse = Boolean(raw.isMouse);
            }
        }

        if (!hasKeyProp && !hasMouseProp) {
            result[button] = { button };
            return;
        }

        if (mapping.key === undefined && mapping.mouseButton === undefined) {
            result[button] = { button };
            return;
        }

        if (!mapping.mouseButton) {
            delete mapping.isMouse;
        }

        result[button] = mapping;
    });

    return result;
};

const sanitizeLeftStickMapping = (input) => {
    const defaults = getDefaultLeftStickMapping();

    if (!input || typeof input !== "object") {
        return defaults;
    }

    const raw = input;
    const result = { ...defaults };

    LEFT_STICK_DIRECTIONS.forEach((direction) => {
        const value = raw[direction];
        if (typeof value === "string" && value.trim().length > 0) {
            result[direction] = value;
        } else {
            result[direction] = undefined;
        }
    });

    return result;
};

const sanitizeMappings = (input) => {
    const defaults = getDefaultControllerMappings();

    if (!input || typeof input !== "object") {
        return defaults;
    }

    const raw = input;

    // New format
    if (raw.buttons || raw.leftStick) {
        return {
            buttons: sanitizeButtonMappings(raw.buttons),
            leftStick: sanitizeLeftStickMapping(raw.leftStick),
        };
    }

    // Compatible with old format (buttons only)
    return {
        buttons: sanitizeButtonMappings(input),
        leftStick: getDefaultLeftStickMapping(),
    };
};

const loadControllerMappings = () => {
    if (!isBrowser()) {
        return getDefaultControllerMappings();
    }

    const raw = window.localStorage.getItem(CONTROLLER_MAPPING_STORAGE_KEY);
    if (!raw) {
        return getDefaultControllerMappings();
    }

    try {
        const parsed = JSON.parse(raw);
        return sanitizeMappings(parsed);
    } catch (error) {
        console.error("Failed to parse controller mappings:", error);
        return getDefaultControllerMappings();
    }
};

const getSavedControllerMappings = () => {
    if (!isBrowser()) {
        return null;
    }

    const raw = window.localStorage.getItem(CONTROLLER_MAPPING_STORAGE_KEY);
    if (!raw) {
        return null;
    }

    try {
        const parsed = JSON.parse(raw);
        return sanitizeMappings(parsed);
    } catch (error) {
        console.error("Failed to parse controller mappings:", error);
        return null;
    }
};

export const saveControllerMappings = (mappings) => {
    if (!isBrowser()) {
        return;
    }

    const sanitized = sanitizeMappings(mappings);
    try {
        window.localStorage.setItem(CONTROLLER_MAPPING_STORAGE_KEY, JSON.stringify(sanitized));
        window.dispatchEvent(
            new CustomEvent(CONTROLLER_MAPPING_CHANGED_EVENT, {
                detail: sanitized,
            })
        );
    } catch (error) {
        console.error("Failed to save controller mappings:", error);
    }
};

export const subscribeControllerMappings = (callback) => {
    if (!isBrowser()) {
        return () => {};
    }

    const handler = (event) => {
        const customEvent = event;
        const detail = customEvent.detail;
        if (detail) {
            callback(detail);
        } else {
            callback(loadControllerMappings());
        }
    };

    window.addEventListener(CONTROLLER_MAPPING_CHANGED_EVENT, handler);
    return () => {
        window.removeEventListener(CONTROLLER_MAPPING_CHANGED_EVENT, handler);
    };
};

const applyOverride = (mapping, button, key) => {
    const streamingName = getStreamingButtonName(button);

    Object.keys(mapping).forEach((existingKey) => {
        if (mapping[existingKey] === streamingName) {
            delete mapping[existingKey];
        }
    });

    if (key) {
        mapping[key] = streamingName;
    }
};

export const getKeyboardToButtonMapping = (overrides) => {
    const mapping = { ...DEFAULT_KEYBOARD_MAPPING };
    const saved = overrides ?? getSavedControllerMappings();

    if (!saved) {
        return mapping;
    }

    for (const button of CONTROLLER_BUTTONS) {
        const buttonMapping = saved.buttons[button];
        if (!buttonMapping) {
            continue;
        }

        if (buttonMapping.isMouse) {
            applyOverride(mapping, button, undefined);
            continue;
        }

        applyOverride(mapping, button, buttonMapping.key);
    }

    return mapping;
};

const LEFT_STICK_DIRECTION_VECTORS = {
    UP: { x: 0, y: -1 },
    DOWN: { x: 0, y: 1 },
    LEFT: { x: -1, y: 0 },
    RIGHT: { x: 1, y: 0 },
    UP_LEFT: { x: -1, y: -1 },
    UP_RIGHT: { x: 1, y: -1 },
    DOWN_LEFT: { x: -1, y: 1 },
    DOWN_RIGHT: { x: 1, y: 1 },
};

const buildLeftStickKeyVectorMap = (leftStick) => {
    const map = {};

    LEFT_STICK_DIRECTIONS.forEach((direction) => {
        const key = leftStick[direction];
        if (key) {
            map[key] = LEFT_STICK_DIRECTION_VECTORS[direction];
        }
    });

    return map;
};

export const getLeftStickKeyVectorMap = (overrides) => {
    const source = overrides ?? getSavedControllerMappings();
    if (!source) {
        return buildLeftStickKeyVectorMap(getDefaultLeftStickMapping());
    }
    return buildLeftStickKeyVectorMap(source.leftStick);
};
