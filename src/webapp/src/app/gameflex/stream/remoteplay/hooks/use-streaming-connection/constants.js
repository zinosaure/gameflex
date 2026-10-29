export const AXIS_DEADZONE = 0.002;
export const SEND_INTERVAL_MS = 8; // Send interval
// Matches upstream's feedback cadence (on change, else max 200ms) — sending
// far faster than this floods the PS5's GMAC key rotation (each FeedbackState
// send advances the crypto key_pos) and its Takion reliable-channel ack
// window, which appears to be what causes the "buttons freeze then heartbeat
// failure" disconnect after ~15-80s of play (see .memory/troubleshooting.md #26).
export const MAX_HEARTBEAT_INTERVAL_MS = 200;
export const TRIGGER_DEADZONE = 0.01;

export const QUICK_HOLD_ACTIVATE = 0.35;
export const QUICK_HOLD_RELEASE_THRESHOLD = 0.04;
export const QUICK_HOLD_DURATION_MS = 120;
export const STICK_RADIAL_DEADZONE = 0.08;

export const MOUSE_SENSITIVITY = 4.5;
export const MOUSE_DECAY_HALF_LIFE_MS = 60;
export const MOUSE_IDLE_THRESHOLD = 0.0025;

export const clamp = (value, min = -1, max = 1) => Math.max(min, Math.min(max, value));
export const clamp01 = (value) => Math.max(0, Math.min(1, value));

export const getTimestamp = () => typeof performance !== "undefined" ? performance.now() : Date.now();
