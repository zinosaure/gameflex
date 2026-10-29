import { API_BASE_URL } from "@/service/api-client";

const PING_ATTEMPTS = 3;
const PROBE_PAYLOAD_BYTES = 2 * 1024 * 1024; // 2MB, matches backend default

async function measureLatencyMs() {
    const samples = [];
    for (let i = 0; i < PING_ATTEMPTS; i++) {
        const start = performance.now();
        try {
            const response = await fetch(`${API_BASE_URL}/playstation/health`, { cache: "no-store" });
            // Une réponse en erreur n'est pas un aller-retour vers le serveur
            // qu'on veut mesurer : on ne la compte pas.
            if (response.ok) {
                samples.push(performance.now() - start);
            }
        } catch {
            // ignore a single failed attempt, keep trying the rest
        }
    }
    if (samples.length === 0) {
        return null;
    }
    samples.sort((a, b) => a - b);
    return samples[Math.floor(samples.length / 2)]; // median
}

async function measureThroughputKbps() {
    const start = performance.now();
    try {
        const response = await fetch(
            `${API_BASE_URL}/playstation/network-probe?sizeBytes=${PROBE_PAYLOAD_BYTES}`,
            { cache: "no-store" }
        );
        // UNE ERREUR N'EST PAS UN DÉBIT (2026-10-02). Le téléphone ne servait
        // pas cette route : sa page 404, quelques octets rendus aussitôt,
        // était mesurée comme un transfert — et le préréglage qui en sortait
        // était appliqué et annoncé comme tel. Il la sert depuis (remoteplay_api.go),
        // mais la règle reste : pas de mesure plutôt qu'une fausse.
        if (!response.ok) {
            return null;
        }
        const blob = await response.blob();
        const elapsedSeconds = (performance.now() - start) / 1000;
        if (elapsedSeconds <= 0 || blob.size === 0) {
            return null;
        }
        return (blob.size * 8) / elapsedSeconds / 1000;
    } catch {
        return null;
    }
}

// Rough, deliberately conservative resolution/fps suggestion — bitrate
// itself is intentionally NOT decided here, it should always stay "auto" so
// the PS5's own real-time bitrate adaptation (StreamSession.congestionLoop,
// fed by ReportClientNetworkLoss) handles the rest; this only picks a
// resolution/fps ceiling the link can plausibly sustain. Calibrated against
// a single real data point so far (see .memory/troubleshooting.md #30):
// ~30ms latency + good throughput -> 720p/60fps fine over VPN, ~60ms
// (same bitrate, no VPN) -> 720p/60fps freezes. Treat as a starting point to
// refine with more real-world reports, not a precise model.
function suggestPreset(latencyMs, throughputKbps) {
    if (latencyMs === null || throughputKbps === null) {
        return null;
    }
    if (throughputKbps >= 20000 && latencyMs <= 40) {
        return { resolution: "1080p", frameRate: "60" };
    }
    if (throughputKbps >= 12000 && latencyMs <= 60) {
        return { resolution: "720p", frameRate: "60" };
    }
    if (throughputKbps >= 6000) {
        return { resolution: "540p", frameRate: "60" };
    }
    // Le bas de l'échelle est 540p à 30 : le 360p n'existe plus (2026-10-26).
    return { resolution: "540p", frameRate: "30" };
}

/**
 * Probes latency + download throughput to OUR backend (not the PS5 — the
 * backend<->PS5 leg is local/LAN and never the real bottleneck, see
 * troubleshooting.md #30) and suggests a resolution/fps preset. Caller
 * decides what to do with the suggestion (e.g. pre-fill a form field) —
 * this never applies or saves anything itself.
 */
export async function probeNetworkAndSuggestPreset() {
    const [latencyMs, throughputKbps] = await Promise.all([
        measureLatencyMs(),
        measureThroughputKbps(),
    ]);
    return {
        latencyMs,
        throughputKbps,
        suggestion: suggestPreset(latencyMs, throughputKbps),
    };
}
