// localStorage-backed keyboard remap for retro-gaming, ONE PER CONSOLE —
// KeyboardEvent.code -> libretro action name (see shared/gamepadOverlay.js's
// RETRO_BUTTON_INDEX), same code -> action shape as gamepadMapping.js's
// index -> action (deliberately NOT remote-play's two-table shape,
// types/controller-mapping.js's button -> key PLUS a separately
// hand-authored key -> button table — that duplication is what let the two
// tables drift out of sync and made remote-play's own keyboard remap
// silently scramble untouched bindings the first time any one binding was
// saved, see that file's own updated doc). One canonical direction only:
// code -> action, so there is nothing to keep in sync.
//
// This is now purely a FALLBACK DEFAULT layer: Streaming.jsx's
// useRetroKeyboardInput tries the new per-device KeyMapping (Phase 8,
// gameflex/libs/keyMapping.js) first, and only falls back to this
// per-console default when the keyboard has no KeyMapping entry for this
// console's virtual controller — kept as-is (not remappable via any UI
// anymore) purely so a user who never opens profile/GamepadParameters.jsx's
// "Mapping manette" tab keeps a working zero-config keyboard.
const STORAGE_PREFIX = "retrogaming_keyboard_mapping_";
const CHANGE_EVENT = "retrogaming-keyboard-mapping-changed";

// A common "Z/X for B/A" layout, as used by most browser-based SNES/NES
// emulators — independent of remote-play's own keyboard defaults
// (types/controller-mapping.js), which target PS button names, not
// libretro's a/b/x/y/l/r/l2/r2/start/select vocabulary.
const DEFAULT_MAPPING = {
    ArrowUp: "up", ArrowDown: "down", ArrowLeft: "left", ArrowRight: "right",
    KeyX: "b", KeyZ: "a", KeyS: "y", KeyA: "x",
    KeyQ: "l", KeyW: "r", KeyE: "l2", KeyR: "r2",
    Enter: "start", ShiftRight: "select",
};

function storageKey(consoleId) {
    return `${STORAGE_PREFIX}${consoleId}`;
}

function getDefaultKeyboardMapping() {
    return { ...DEFAULT_MAPPING };
}

export function loadKeyboardMapping(consoleId) {
    try {
        const raw = localStorage.getItem(storageKey(consoleId));
        if (!raw) return getDefaultKeyboardMapping();
        return { ...getDefaultKeyboardMapping(), ...JSON.parse(raw) };
    } catch {
        return getDefaultKeyboardMapping();
    }
}

// Ignores changes for any OTHER console — a change event fires globally
// (window-level), but a mounted tab only ever cares about the console of
// the session it's actually attached to.
export function subscribeKeyboardMapping(consoleId, callback) {
    const handler = (event) => {
        if (event.detail?.consoleId !== consoleId) return;
        callback(event.detail?.mapping || loadKeyboardMapping(consoleId));
    };
    window.addEventListener(CHANGE_EVENT, handler);
    return () => window.removeEventListener(CHANGE_EVENT, handler);
}

// Resolves with the pressed CHORD — the KeyboardEvent.codes of every key
// held at release, sorted and joined with "+" (order-independent, so
// ArrowDown+ArrowRight always reads the same) — a single key resolves to
// Streams the chord being built from keyboard presses: every code whose
// keydown fires while the capture is active is ADDED to the accumulated set
// (releases are ignored — the chord never resolves on its own), and onChange
// receives the sorted codes joined with "+" after every change. The caller
// (KeyMappingCaptureRow) validates with its tick button and discards with the
// trash one — chords are what make a diagonal (DPAD_DOWN_RIGHT) mappable.
// Returns a cleanup that removes the listeners; aborting the signal runs the
// same cleanup without calling onChange again.
export function captureKeyboardChord({ signal, onChange } = {}) {
    const accumulated = new Set();
    const cleanup = () => {
        document.removeEventListener("keydown", onKeyDown, true);
        signal?.removeEventListener("abort", onAbort);
    };
    const onKeyDown = (e) => {
        if (accumulated.has(e.code)) return;
        e.preventDefault();
        e.stopPropagation();
        accumulated.add(e.code);
        onChange?.([...accumulated].sort().join("+"));
    };
    const onAbort = cleanup;

    if (signal?.aborted) return cleanup;
    document.addEventListener("keydown", onKeyDown, true);
    signal?.addEventListener("abort", onAbort);
    return cleanup;
}
