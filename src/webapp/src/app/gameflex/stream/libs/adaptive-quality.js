// LA BASCULE AUTOMATIQUE ENTRE LES DEUX FLUX — chantier 31 (le porteur,
// 2026-10-25 : « si le système détecte que mon réseau est devenu faible alors
// c'est le 540p qui prend le relai jusqu'à ce que ça redevienne stable »).
//
// La page décide pour SON spectateur, à partir de ce qu'elle mesure toutes les
// deux secondes : pertes de paquets, gigue, images gelées. Elle DESCEND VITE
// (deux relevés mauvais de suite, environ 4 s) et REMONTE LENTEMENT (30 s sans
// souci, puis un essai en HD ; un essai qui échoue double l'attente suivante).
// La passerelle ne fait que suivre (gatewaystreams/quality.go).
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
 * Un pas de la règle. `current` — la qualité que le spectateur REÇOIT
 * (`hd`, `sd`). Rend `{ state, want }` : `want` est la qualité à demander, ou
 * null s'il n'y a rien à changer.
 */
export function adaptQuality(state, reading, current, now) {
    const bad = isBadReading(reading);

    if (current === "hd") {
        const next = { ...state, bad: bad ? state.bad + 1 : 0 };
        if (next.bad >= DOWN_AFTER) {
            // Un essai en HD qui échoue vite : la prochaine attente double.
            const failedTry = next.triedAt !== null && now - next.triedAt < PROBATION_MS;

            return {
                state: {
                    bad: 0,
                    calmSince: null,
                    triedAt: null,
                    upAfter: failedTry ? Math.min(next.upAfter * 2, UP_MAX_MS) : next.upAfter,
                },
                want: "sd",
            };
        }
        // Un essai qui a tenu : l'attente redevient celle du départ.
        if (next.triedAt !== null && now - next.triedAt >= PROBATION_MS) {
            return { state: { ...next, triedAt: null, upAfter: UP_AFTER_MS }, want: null };
        }

        return { state: next, want: null };
    }

    const calmSince = bad ? null : (state.calmSince ?? now);
    if (calmSince !== null && now - calmSince >= state.upAfter) {
        return { state: { ...state, bad: 0, calmSince: null, triedAt: now }, want: "hd" };
    }

    return { state: { ...state, calmSince }, want: null };
}
