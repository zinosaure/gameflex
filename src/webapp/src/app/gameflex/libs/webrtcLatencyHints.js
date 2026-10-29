// A live WebRTC video/audio receiver builds its OWN jitter buffer by
// default, sized adaptively for smooth playback under variable network
// conditions — which for a real internet connection is the right trade-off,
// but on a session that's actually low-jitter (same LAN, or this project's
// own low-latency encoder settings, see encoder/ffmpeg.go's `-tune
// zerolatency`) it can still grow to a few SECONDS before settling, with
// nothing in the app signaling that it's happening. That shows up as
// "I pressed the button and nothing happened... then it did, late" — input
// itself isn't slow, the PICTURE confirming it landed is delayed.
//
// remote-play's use-streaming-connection.js already carries this exact
// fix (`applyReceiverLatencyHints`) — extracted here so retro-gaming's own
// hook (which never had it, see GAMEFLEX_JOURNAL.md's 2026-08 external-
// emulator entry, found via a Switch session that made the delay obvious
// enough to notice) can use the same technique without duplicating it.
// remote-play's own copy is left as-is deliberately — that hook is large
// and easy to regress (see its own file), not worth the risk to touch for
// a pure refactor.
export function applyReceiverLatencyHints(receiver) {
    const isAudioTrack = receiver.track?.kind === "audio";
    // A small non-zero floor for audio, not 0: a handful of samples of
    // slack absorbs normal jitter without an audible buffer-empty click,
    // which 0 was observed to cause on remote-play. Video has no such
    // failure mode (a late frame just holds the previous one), so 0 there.
    const preferredDelay = isAudioTrack ? 0.12 : 0;
    // Feature-detect by PRESENCE, not by the default value's type: Chrome's
    // own default for playoutDelayHint is `null` (typeof "object"), not a
    // number, until something assigns it — `typeof x === "number"` was a
    // false negative that made this whole function a no-op on every session
    // (confirmed live 2026-08-13: chrome://webrtc-internals showed
    // jitterBufferTargetDelay=420ms on audio, the browser's own unaided
    // default, despite this code "requesting" 120ms every single track).
    // `in` only asks "does this property exist on the object", which is the
    // real question. jitterBufferDelayHint is a separate, non-standard
    // property that genuinely doesn't exist on this receiver at all
    // (logged as `undefined`, not `null`) — correctly left alone.
    try {
        if ("playoutDelayHint" in receiver) {
            receiver.playoutDelayHint = preferredDelay;
        }
        if ("jitterBufferDelayHint" in receiver) {
            receiver.jitterBufferDelayHint = preferredDelay;
        }
    } catch {
        // Best-effort — an unsupported browser just keeps its own default
        // adaptive buffer, not a reason to fail the connection.
    }
}
