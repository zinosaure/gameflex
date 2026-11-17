// LA BASCULE AUTOMATIQUE ENTRE LES PALIERS — chantier 31 (le porteur,
// 2026-10-25 : « si le système détecte que mon réseau est devenu faible alors
// c'est le 540p qui prend le relai jusqu'à ce que ça redevienne stable »), et
// chantier 58 (2026-11-16 : « L'idée d'avoir les autres flux plus bas en
// parallèle était pour permettre d'avoir l'auto qui s'adapte plus facile. Il
// monte et descend sans problème »).
//
// Les flux d'une partie : 0 le sien (le plus grand), puis ses paliers, de plus
// en plus petits. La page décide pour SON spectateur, à partir de ce qu'elle
// mesure toutes les deux secondes : pertes de paquets, gigue, images gelées.
// Elle DESCEND VITE d'un palier (deux relevés mauvais de suite, environ 4 s) et
// REMONTE LENTEMENT d'un palier (30 s sans souci, puis un essai ; un essai qui
// échoue double l'attente suivante), au plus jusqu'au flux de la partie. La
// passerelle ne fait que suivre (gatewaystreams/quality.go).
//
// Des fonctions pures : l'état entre, l'état sort, avec la qualité à demander.

export const SAMPLE_MS = 2000;

// Un relevé est MAUVAIS au-delà de ces seuils.
const BAD_LOSS = 0.03; // 3 % des paquets vidéo perdus sur l'intervalle
const BAD_JITTER_MS = 60; // la gigue RTP du flux vidéo
const DOWN_AFTER = 2; // relevés mauvais de suite avant de descendre

const UP_AFTER_MS = 30_000; // le calme demandé avant un essai en HD
const UP_MAX_MS = 8 * 60_000; // l'attente ne double pas au-delà
const PROBATION_MS = 20_000; // un essai qui tient ce délai a réussi

export function createAdaptiveQuality() {
    return { bad: 0, calmSince: null, upAfter: UP_AFTER_MS, triedAt: null };
}

/**
 * Un relevé, lu dans `getStats()` : la piste vidéo reçue, comparée au relevé
 * précédent (`previous`, null au premier). Rend `{ counters, reading }` —
 * `reading` est null tant qu'il n'y a pas deux relevés à comparer.
 */
export function readVideoHealth(report, previous) {
    let counters = null;
    report.forEach((entry) => {
        if (entry.type === "inbound-rtp" && !entry.isRemote && entry.kind === "video") {
            counters = {
                lost: entry.packetsLost ?? 0,
                received: entry.packetsReceived ?? 0,
                freezes: entry.freezeCount ?? 0,
                jitterMs: typeof entry.jitter === "number" ? entry.jitter * 1000 : 0,
            };
        }
    });
    if (!counters || !previous) {
        return { counters, reading: null };
    }
    const lost = Math.max(0, counters.lost - previous.lost);
    const received = Math.max(0, counters.received - previous.received);

    return {
        counters,
        reading: {
            lossRate: lost + received > 0 ? lost / (lost + received) : 0,
            jitterMs: counters.jitterMs,
            freezes: Math.max(0, counters.freezes - previous.freezes),
        },
    };
}

/** Ce relevé est-il mauvais ? */
export function isBadReading(reading) {
    return reading.lossRate >= BAD_LOSS || reading.jitterMs >= BAD_JITTER_MS || reading.freezes > 0;
}

/**
 * Un pas de la règle. `current` — le flux que le spectateur REÇOIT (0 celui de
 * la partie, puis ses paliers) ; `lowest` — l'indice du plus petit. Rend
 * `{ state, want }` : `want` est l'indice à demander, ou null s'il n'y a rien à
 * changer.
 */
export function adaptQuality(state, reading, current, lowest, now) {
    const bad = isBadReading(reading);
    const next = { ...state, bad: bad ? state.bad + 1 : 0 };

    if (next.bad >= DOWN_AFTER && current < lowest) {
        // Un essai qui échoue vite : la prochaine attente double.
        const failedTry = next.triedAt !== null && now - next.triedAt < PROBATION_MS;

        return {
            state: {
                bad: 0,
                calmSince: null,
                triedAt: null,
                upAfter: failedTry ? Math.min(next.upAfter * 2, UP_MAX_MS) : next.upAfter,
            },
            want: current + 1,
        };
    }
    // Un essai qui a tenu : l'attente redevient celle du départ.
    if (next.triedAt !== null && now - next.triedAt >= PROBATION_MS) {
        next.triedAt = null;
        next.upAfter = UP_AFTER_MS;
    }
    if (current === 0) {
        return { state: { ...next, calmSince: null }, want: null };
    }
    const calmSince = bad ? null : (next.calmSince ?? now);
    if (calmSince !== null && now - calmSince >= next.upAfter) {
        return { state: { ...next, bad: 0, calmSince: null, triedAt: now }, want: current - 1 };
    }

    return { state: { ...next, calmSince }, want: null };
}
