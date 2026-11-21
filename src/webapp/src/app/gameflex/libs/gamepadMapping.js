// localStorage-backed physical-gamepad button remap for retro-gaming, ONE
// PER CONSOLE — the standard Gamepad API button INDEX -> libretro action
// name (see shared/gamepadOverlay.js's RETRO_BUTTON_INDEX). Same pub/sub-on-
// change shape as types/controller-mapping.js (remote-play's own keyboard
// remap), kept as a separate store: different action vocabulary (libretro
// joypad names, not PS button names) and a different underlying input (raw
// Gamepad API polling, not keydown events).
//
// This is now purely a FALLBACK DEFAULT layer: Streaming.jsx's
// useRetroPhysicalGamepadInput tries the new per-device KeyMapping (Phase 8,
// gameflex/libs/keyMapping.js) first, and only falls back to this
// per-console default when the connected device has no KeyMapping entry for
// this console's virtual controller — kept as-is (not remappable via any UI
// anymore) purely so a user who never opens profile/GamepadParameters.jsx's
// "Mapping manette" tab keeps a working zero-config gamepad.
const STORAGE_PREFIX = "retrogaming_gamepad_mapping_";
const CHANGE_EVENT = "retrogaming-gamepad-mapping-changed";

// Same defaults Streaming.jsx's useRetroPhysicalGamepadInput always used
// before this became remappable — a standard Gamepad API layout, where
// libretro's SNES-style a/b/x/y position naming happens to line up with the
// Gamepad API's own button order.
const DEFAULT_MAPPING = {
    0: "b", 1: "a", 2: "y", 3: "x",
    4: "l", 5: "r", 6: "l2", 7: "r2",
    8: "select", 9: "start",
    12: "up", 13: "down", 14: "left", 15: "right",
};

function storageKey(consoleId) {
    return `${STORAGE_PREFIX}${consoleId}`;
}

function getDefaultGamepadMapping() {
    return { ...DEFAULT_MAPPING };
}

export function loadGamepadMapping(consoleId) {
    try {
        const raw = localStorage.getItem(storageKey(consoleId));
        if (!raw) return getDefaultGamepadMapping();
        return { ...getDefaultGamepadMapping(), ...JSON.parse(raw) };
    } catch {
        return getDefaultGamepadMapping();
    }
}

// Ignores changes for any OTHER console — a change event fires globally
// (window-level), but a mounted tab only ever cares about the console of
// the session it's actually attached to.
export function subscribeGamepadMapping(consoleId, callback) {
    const handler = (event) => {
        if (event.detail?.consoleId !== consoleId) return;
        callback(event.detail?.mapping || loadGamepadMapping(consoleId));
    };
    window.addEventListener(CHANGE_EVENT, handler);
    return () => window.removeEventListener(CHANGE_EVENT, handler);
}

