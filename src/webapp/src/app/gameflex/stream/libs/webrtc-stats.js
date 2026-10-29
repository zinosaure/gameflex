// Shared WebRTC getStats() harvesting for the streaming pages — the exact
// same report scan and rate math used by remote-play (hooks/
// use-streaming-connection.js) and streamex (gameflex/streamex/
// Streaming.jsx), extracted so the two consumers can't drift apart. Pure
// functions only: each caller keeps its own state wiring.

// scanWebRTCReport walks one getStats() report and returns the raw
// counters both panels need. Frames/loss/jitter only come from the VIDEO
// inbound stream; latency from the nominated candidate pair.
export function scanWebRTCReport(report) {
    let inboundBytes = 0;
    let outboundBytes = 0;
    let videoInboundBytes = 0;
    let frameWidth = null;
    let frameHeight = null;
    let framesDecoded = null;
    let latencyMs = null;
    let videoPacketsLost = null;
    let videoJitterMs = null;

    report.forEach((entry) => {
        if (entry.type === "inbound-rtp" && !entry.isRemote) {
            const bytes =
                typeof entry.bytesReceived === "number" ? entry.bytesReceived : 0;
            inboundBytes += bytes;
            if (entry.kind === "video") {
                videoInboundBytes += bytes;
                if (typeof entry.frameWidth === "number") frameWidth = entry.frameWidth;
                if (typeof entry.frameHeight === "number")
                    frameHeight = entry.frameHeight;
                if (typeof entry.framesDecoded === "number")
                    framesDecoded = entry.framesDecoded;
                if (typeof entry.packetsLost === "number")
                    videoPacketsLost = entry.packetsLost;
                if (typeof entry.jitter === "number")
                    videoJitterMs = entry.jitter * 1000;
            }
        }
        if (
            entry.type === "outbound-rtp" &&
            !entry.isRemote &&
            typeof entry.bytesSent === "number"
        ) {
            outboundBytes += entry.bytesSent;
        }
        if (
            entry.type === "candidate-pair" &&
            entry.state === "succeeded" &&
            entry.nominated &&
            typeof entry.currentRoundTripTime === "number"
        ) {
            latencyMs = entry.currentRoundTripTime * 1000;
        }
    });

    return {
        inboundBytes,
        outboundBytes,
        videoInboundBytes,
        frameWidth,
        frameHeight,
        framesDecoded,
        latencyMs,
        videoPacketsLost,
        videoJitterMs,
    };
}

// rateDeltas turns two harvests into per-second rates (kbps) and fps.
// previous is the caller's snapshot of a prior scan (the shape returned by
// nextCounters); null when the elapsed window is not positive.
export function rateDeltas(previous, now, harvested) {
    const elapsedSeconds = (now - previous.timestamp) / 1000;
    if (elapsedSeconds <= 0) return null;

    const downloadKbps = Math.max(
        0,
        ((harvested.inboundBytes - previous.inboundBytes) * 8) /
        elapsedSeconds /
        1000,
    );
    const uploadKbps = Math.max(
        0,
        ((harvested.outboundBytes - previous.outboundBytes) * 8) /
        elapsedSeconds /
        1000,
    );
    const videoBitrateKbps = Math.max(
        0,
        ((harvested.videoInboundBytes - previous.videoInboundBytes) * 8) /
        elapsedSeconds /
        1000,
    );

    let fps = null;
    if (harvested.framesDecoded !== null && previous.framesDecoded !== null) {
        const framesDiff = Math.max(
            0,
            harvested.framesDecoded - previous.framesDecoded,
        );
        fps = framesDiff / elapsedSeconds;
        if (!Number.isFinite(fps) || fps < 0) fps = null;
    }

    return { downloadKbps, uploadKbps, videoBitrateKbps, fps };
}

// nextCounters is the snapshot to store for the NEXT tick — timestamps the
// scan and keeps only what rateDeltas needs.
export function nextCounters(now, harvested, previousFrames) {
    return {
        timestamp: now,
        inboundBytes: harvested.inboundBytes,
        outboundBytes: harvested.outboundBytes,
        videoInboundBytes: harvested.videoInboundBytes,
        framesDecoded: harvested.framesDecoded ?? previousFrames ?? 0,
    };
}
