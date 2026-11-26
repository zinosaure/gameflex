import { apiUrl } from "../../../../libs/serverBase.js";

const PING_ATTEMPTS = 3;
const PROBE_PAYLOAD_BYTES = 2 * 1024 * 1024; // 2MB, matches backend default

// LA SONDE DU SERVEUR QUI SERVIRA LA PARTIE : le nôtre (`apiUrl`, qui sait l'origine du serveur dans l'application
// Android), ou le serveur fédéré qui la lance chez lui (`host.origin`). Elle est servie par le service remote-play et
// par le téléphone, sans compte (`openPaths`, auth_guard.go) : on n'y envoie aucun jeton.
const probeUrl = (path, host = null) => (host?.origin ? `${host.origin}${path}` : apiUrl(path));

async function measureLatencyMs(host = null) {
    const samples = [];
    for (let i = 0; i < PING_ATTEMPTS; i++) {
        const start = performance.now();
        try {
            const response = await fetch(probeUrl("/api/v1/playstation/health", host), { cache: "no-store" });
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

async function measureThroughputKbps(host = null) {
    const start = performance.now();
    try {
        const response = await fetch(
            probeUrl(`/api/v1/playstation/network-probe?sizeBytes=${PROBE_PAYLOAD_BYTES}`, host),
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

// Rough, deliberately conservative resolution/fps suggestion — bitrate is NOT
// decided here: it stays "auto", which the launch measures itself
// (withNetworkBitrate, below). This only picks a resolution/fps ceiling the
// link can plausibly sustain. Calibrated against a single real data point so
// far (see .memory/troubleshooting.md #30): ~30ms latency + good throughput ->
// 720p/60fps fine over VPN, ~60ms (same bitrate, no VPN) -> 720p/60fps
// freezes. Treat as a starting point to refine with more real-world reports,
// not a precise model.
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
    if (throughputKbps >= 3000) {
        return { resolution: "540p", frameRate: "30" };
    }
    // Le bas de l'échelle : 360p à 30, revenu le 2026-11-06 (#636).
    return { resolution: "360p", frameRate: "30" };
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

// LE DÉBIT « AUTO » SUIT LE RÉSEAU LÀ OÙ L'ON JOUE (le porteur, 2026-11-16 : « Le auto s'align sur le débit du réseau.
// Encore une fois c'est parce qu'on peut jouer de n'importe où. »). Il se mesure à chaque lancement, entre CE navigateur
// et le serveur qui servira la partie, et part avec le lancement comme un débit choisi : un téléphone en données
// mobiles dans un bus n'a pas le réseau d'un ordinateur à la maison. Il n'était qu'un débit fixe — celui de la
// définition au serveur (le Remote Play), 10 Mbps au téléphone, celui que l'encodeur déduisait de l'image ailleurs.
//
// LA MESURE : la sonde du serveur téléchargée pendant une seconde au plus, depuis la réception de ses en-têtes — sur
// des données mobiles à 3 Mbps, ses 2 Mo en prendraient cinq, et l'écran du lancement attend. Une seconde de TCP ne
// voit pas tout ce que voit le flux (UDP, un relais TURN parfois) : le flux n'en prend donc qu'une part.
const LAUNCH_PROBE_MS = 1000;
// L'attente des en-têtes, au plus : au-delà, le serveur ne répond pas, et le lancement part sans mesure.
const LAUNCH_PROBE_TIMEOUT_MS = 3000;
// La part du débit mesuré que prend la vidéo : le reste au son, aux en-têtes, à ce que le réseau perd en route.
const AUTO_BITRATE_SHARE = 0.6;
// Les bornes : celles des deux kernels (`consoles.StreamBitrate`) en bas ; en haut, le plafond du 1080p, que le rapport
// des surfaces porte à 26 700 kbps pour un 1440p (autoBitrateCeilingOf).
const AUTO_BITRATE_MIN = 500;
const AUTO_BITRATE_MAX = 15000;

// LE PLAFOND SUIT LA RÉSOLUTION (#759, le porteur, 2026-11-17 : « sur PS4 j'ai l'impression que la qualité de la vidéo
// reste haute, peu importe la résolution choisie. J'ai même vu où le download rate était à 20 Mb/s »). Le plafond de
// 15 000 kbps valait pour TOUTES les résolutions : une partie lancée en 540p sur un réseau rapide recevait 15 Mbps, autant
// qu'un 1080p, et un rendu riche (la PS4 rend en 1080p, réduit à la fenêtre) les remplissait — mesuré : 15,0 Mbps reçus en
// 960×540, des secondes à 18 Mbps, quand le palier 540p d'une partie en 1080p en reçoit 3,7. 15 000 kbps restent le
// plafond du 1080p ; en dessous, au rapport des surfaces, la règle des paliers (`consoles.StreamTiers`) : 720p 6 700,
// 540p 3 800, 360p 1 700. Le même 540p reçoit ainsi le même débit, lancé tel quel ou choisi comme palier.
const AUTO_BITRATE_REFERENCE_HEIGHT = 1080;
const STREAM_HEIGHTS = { "360p": 360, "540p": 540, "720p": 720, "1080p": 1080, "1440p": 1440 };

// Le plafond d'« Auto » pour une résolution (« 540p ») ; une résolution inconnue garde celui du 1080p.
export function autoBitrateCeilingOf(resolution) {
    const height = STREAM_HEIGHTS[resolution] || AUTO_BITRATE_REFERENCE_HEIGHT;
    const ceiling = AUTO_BITRATE_MAX * (height / AUTO_BITRATE_REFERENCE_HEIGHT) ** 2;

    return Math.max(AUTO_BITRATE_MIN, Math.round(ceiling / 100) * 100);
}

// Le débit d'un flux « Auto » pour un débit mesuré et la résolution du flux, en kbps, arrondi à la centaine.
export function autoBitrateOf(throughputKbps, resolution = null) {
    const kbps = Math.round((throughputKbps * AUTO_BITRATE_SHARE) / 100) * 100;

    return Math.min(autoBitrateCeilingOf(resolution), Math.max(AUTO_BITRATE_MIN, kbps));
}

// Le débit descendant vers le serveur de la partie (`host`, ou le nôtre), en kbps ; null quand il ne répond pas. Une
// réponse en erreur n'est pas un débit (2026-10-02, plus haut).
export async function measureLaunchThroughputKbps(host = null) {
    const controller = new AbortController();
    const timeout = setTimeout(() => controller.abort(), LAUNCH_PROBE_TIMEOUT_MS);
    let started = null;
    let bytes = 0;

    try {
        const response = await fetch(probeUrl(`/api/v1/playstation/network-probe?sizeBytes=${PROBE_PAYLOAD_BYTES}`, host), { cache: "no-store", signal: controller.signal });

        if (!response.ok || !response.body) {
            return null;
        }
        clearTimeout(timeout);
        // La lecture a sa propre borne : un réseau qui cale ne retient pas le lancement au-delà d'une seconde.
        const stop = setTimeout(() => controller.abort(), LAUNCH_PROBE_MS + 250);
        const reader = response.body.getReader();
        started = performance.now();

        try {
            for (;;) {
                const { done, value } = await reader.read();

                if (done) {
                    break;
                }
                bytes += value.byteLength;

                if (performance.now() - started >= LAUNCH_PROBE_MS) {
                    break;
                }
            }
        } catch {
            // Coupée par sa borne : ce qui est arrivé jusque-là se mesure quand même.
        } finally {
            clearTimeout(stop);
        }
    } catch {
        return null;
    } finally {
        clearTimeout(timeout);
        controller.abort();
    }

    if (started === null || bytes === 0) {
        return null;
    }
    // Au moins une milliseconde : sur un réseau local, les 2 Mo arrivent parfois d'un bloc.
    const seconds = Math.max(performance.now() - started, 1) / 1000;

    return (bytes * 8) / seconds / 1000;
}

// LE FLUX D'UN LANCEMENT, SON DÉBIT « AUTO » MESURÉ : `stream` ({ resolution, frameRate, bitrate }) tel que ce
// navigateur le garde, rendu avec le débit que le réseau vers `host` (ou le nôtre) permet. Un débit choisi reste tel
// quel ; une mesure qui échoue laisse « auto », et le serveur prend son repli (l'encodeur, ou la définition au Remote
// Play). La mesure est dite dans la console du navigateur : c'est ce qu'on regarde quand une image déçoit.
export async function withNetworkBitrate(stream, host = null) {
    if (stream?.bitrate && stream.bitrate !== "auto") {
        return stream;
    }
    const throughput = await measureLaunchThroughputKbps(host);

    if (!throughput) {
        console.info("[stream] débit auto : réseau non mesuré, le serveur choisit");

        return stream;
    }
    const bitrate = autoBitrateOf(throughput, stream?.resolution);
    console.info(`[stream] débit auto : ${Math.round(throughput)} kbps mesurés vers ${host?.origin || "ce serveur"}, flux à ${bitrate} kbps`);

    return { ...stream, bitrate: String(bitrate) };
}
