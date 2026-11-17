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
    // which 0 was observed to cause on remote-play.
    // 60 ms, not 120 (2026-11-03): audio and video are separate streams, so
    // the browser never lines them up, and every ms of this floor puts the
    // sound that much behind the picture (heard on PS5 cutscenes).
    //
    // VIDEO: 50 ms, not 0 (2026-11-07, chantier 46, journal 473). At 0 the
    // browser shows each frame the moment it is decoded, ignoring its
    // timestamp; frames that arrive two at a time land in the same screen
    // refresh, and only the last one is shown. Measured on Wukong at
    // 540p@30, camera turning: the server sent 30 distinct frames a second,
    // Chrome decoded 30 and SHOWED 25.5 — 185 frames out of 1,244 never on
    // screen, in pairs 17 ms apart then 83 ms holes: the « sauts d'images »
    // the owner saw. At 50 ms, same scene: 1,248 shown out of 1,250, every
    // 33 ms, for a jitter buffer of 25 ms instead of 7 on average. It still
    // bounds the browser's own buffer, which could grow to seconds.
    const preferredDelay = isAudioTrack ? 0.06 : 0.05;
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
