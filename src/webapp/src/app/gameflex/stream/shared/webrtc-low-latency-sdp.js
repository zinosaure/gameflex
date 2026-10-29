import { debugTrace } from "@/gameflex/shared/utils/log-store";

/**
 * Optimize SDP for lower latency
 */
const AUDIO_MIN_BUFFERED_PLAYBACK_TIME = 0.3;

export function optimizeSdpForLowLatency(sdp, options) {
    try {
        if (!sdp || typeof sdp !== "string" || sdp.length < 10) {
            return sdp;
        }

        // Check if optimization markers already exist (avoid duplicate insertion)
        if (
            sdp.includes("a=x-google-flag:low-latency") &&
            sdp.includes("a=minBufferedPlaybackTime")
        ) {
            return sdp; // Already optimized
        }

        const lines = sdp.split(/\r?\n/);
        const optimizedLines = [];
        let foundVideoMedia = false;
        let foundAudioMedia = false;
        let videoOptimized = false;
        let audioOptimized = false;

        for (let i = 0; i < lines.length; i++) {
            const line = lines[i];
            optimizedLines.push(line);

            // Detect media lines
            if (line.trim().startsWith("m=video ")) {
                foundVideoMedia = true;
                foundAudioMedia = false;
                videoOptimized = false;
            } else if (line.trim().startsWith("m=audio ")) {
                foundAudioMedia = true;
                foundVideoMedia = false;
                audioOptimized = false;
            } else if (line.trim().startsWith("m=")) {
                // Other media types, reset state
                foundVideoMedia = false;
                foundAudioMedia = false;
            }

            // Add optimization after the first attribute line in the video media section (ensure correct format)
            if (
                foundVideoMedia &&
                !videoOptimized &&
                line.trim().startsWith("a=") &&
                !line.trim().startsWith("a=rtcp:") &&
                line.trim().length > 2
            ) {
                // Only add after the first valid attribute line
                if (!sdp.includes("a=x-google-flag:low-latency")) {
                    optimizedLines.push("a=x-google-flag:low-latency");
                }
                if (!sdp.includes("a=minBufferedPlaybackTime")) {
                    optimizedLines.push(
                        `a=minBufferedPlaybackTime:${AUDIO_MIN_BUFFERED_PLAYBACK_TIME}`
                    );
                }
                videoOptimized = true;
            }

            // Add optimization after the first attribute line in the audio media section
            if (
                foundAudioMedia &&
                !audioOptimized &&
                line.trim().startsWith("a=") &&
                !line.trim().startsWith("a=rtcp:") &&
                line.trim().length > 2
            ) {
                if (!sdp.includes("a=minBufferedPlaybackTime")) {
                    optimizedLines.push("a=minBufferedPlaybackTime:0");
                }
                audioOptimized = true;
            }
        }

        const preferLan = options?.preferLanCandidates ?? true;
        const finalLines = preferLan ? reorderCandidatesForLan(optimizedLines) : optimizedLines;
        const result = finalLines.join("\r\n");

        // Validate result
        if (!result || result.length < sdp.length * 0.5) {
            // If result is significantly shorter than original SDP, something may have gone wrong
            return sdp;
        }

        // Ensure basic SDP structure is intact
        if (!result.includes("v=0") || !result.includes("m=")) {
            return sdp;
        }

        return result;
    } catch (error) {
        console.error("SDP optimization error:", error);
        return sdp; // Return original SDP on error
    }
}

/**
 * Optimize video element for lower latency (zero-buffer mode)
 */
export function optimizeVideoForLowLatency(video) {
    // Zero-buffer initialization settings
    video.preload = "none"; // Disable preload
    video.autoplay = true; // Auto-play
    video.playsInline = true; // Inline playback

    // Monitor buffer and actively reduce latency (zero-buffer strategy)
    let lastBufferCheck = 0;
    const bufferCheckInterval = 50; // Check every 50ms (more frequent)
    const maxBufferTime = 0.05; // Maximum allowed buffer time: 50ms (near zero-buffer)

    const checkBufferAndOptimize = () => {
        if (video.buffered && video.buffered.length > 0) {
            const bufferedEnd = video.buffered.end(video.buffered.length - 1);
            const currentTime = video.currentTime;
            const bufferAhead = bufferedEnd - currentTime;

            // ✅ Zero-buffer strategy: if buffer exceeds 50ms, seek immediately to reduce latency
            if (bufferAhead > maxBufferTime && currentTime > 0.01) {
                // Seek to end of buffer, keeping only minimal buffer (10ms)
                const targetTime = bufferedEnd - 0.01; // Keep only 10ms buffer
                if (targetTime > currentTime && targetTime < bufferedEnd) {
                    try {
                        video.currentTime = targetTime;
                        if (lastBufferCheck % 20 === 0) {
                            debugTrace(
                                `⚡ Zero-buffer optimization: ${(bufferAhead * 1000).toFixed(0)}ms -> 10ms`
                            );
                        }
                    } catch (e) {
                        // Ignore seek errors (possibly due to buffer being too small)
                    }
                }
            }

            // Periodically log buffer state (every 2 seconds)
            if (lastBufferCheck % 40 === 0) {
                debugTrace(`📊 Video buffer: ${(bufferAhead * 1000).toFixed(0)}ms`);
            }
        }
        lastBufferCheck++;
    };

    // Start high-frequency buffer monitoring (zero-buffer mode)
    const bufferMonitor = setInterval(checkBufferAndOptimize, bufferCheckInterval);

    // Return cleanup function
    return () => {
        clearInterval(bufferMonitor);
    };
}

function reorderCandidatesForLan(lines) {
    const optimizedLines = [];
    let candidateBuffer = [];
    let collectingCandidates = false;

    const flushBuffer = () => {
        if (candidateBuffer.length === 0) return;
        candidateBuffer = candidateBuffer.sort((a, b) => scoreCandidate(b) - scoreCandidate(a));
        optimizedLines.push(...candidateBuffer);
        candidateBuffer = [];
    };

    for (const line of lines) {
        const trimmed = line.trim();

        if (trimmed.startsWith("m=")) {
            flushBuffer();
            optimizedLines.push(line);
            collectingCandidates = false;
            continue;
        }

        if (trimmed.startsWith("a=candidate:")) {
            collectingCandidates = true;
            candidateBuffer.push(line);
            continue;
        }

        if (collectingCandidates && !trimmed.startsWith("a=candidate:")) {
            flushBuffer();
            collectingCandidates = false;
        }

        optimizedLines.push(line);
    }

    flushBuffer();
    return optimizedLines;
}

function scoreCandidate(candidateLine) {
    const parts = candidateLine.split(/\s+/);
    const protocol = (parts[2] || "").toLowerCase();
    const address = parts[4] || "";
    const component = parts[1] || "";
    const typeIndex = parts.findIndex((part) => part === "typ");
    const candidateType = typeIndex >= 0 ? (parts[typeIndex + 1] || "").toLowerCase() : "";

    let score = 0;

    if (candidateType === "host" && isPrivateAddress(address)) {
        score += 400;
    } else if (candidateType === "host") {
        score += 320;
    } else if (candidateType === "srflx") {
        score += 200;
    } else if (candidateType === "prflx") {
        score += 150;
    } else if (candidateType === "relay") {
        score += 50;
    }

    if (protocol === "udp") {
        score += 40;
    }

    if (component === "1") {
        score += 10;
    }

    return score;
}

function isPrivateAddress(address) {
    if (!address) {
        return false;
    }

    // IPv6 link-local or local prefix
    if (address.includes(":")) {
        const lower = address.toLowerCase();
        return (
            lower.startsWith("fe80") || // link-local
            lower.startsWith("fd") || // ULA
            lower.startsWith("fc")
        );
    }

    // IPv4
    if (address.startsWith("10.")) return true;
    if (address.startsWith("192.168.")) return true;
    if (address.startsWith("169.254.")) return true;

    if (address.startsWith("172.")) {
        const second = parseInt(address.split(".")[1], 10);
        if (!Number.isNaN(second) && second >= 16 && second <= 31) {
            return true;
        }
    }

    return false;
}
