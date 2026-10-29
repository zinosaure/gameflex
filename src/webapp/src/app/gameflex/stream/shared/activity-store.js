// activity-store.js — P6 (roadmap phase-6-afk-guard.md): the minimal
// EXTERNAL store between the per-platform input hooks and StreamingShell.
// The hooks stamp it on every REAL input send (the physical pads are
// polling loops with no DOM events to hook), and the shell drains it into
// each beat to the child (`hadInput`) — the child only ever hears about
// activity a human actually caused. No React subscription needed: the shell
// reads it on a timer, nothing renders off it.
let stampedAt = 0;

export function markActivity() {
    stampedAt = Date.now();
}

// takeActivityStamp returns the stamp since the last drain (0 = nothing)
// and clears it — one input burst counts once per beat at most.
export function takeActivityStamp() {
    const stamp = stampedAt;
    stampedAt = 0;
    return stamp;
}
