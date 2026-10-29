// LES RESSOURCES D'UNE MACHINE, EN UNE LIGNE (2026-10-23) — ce qu'elle offre à
// ses émulateurs de bureau, tel qu'elle le déclare (`resources` de
// /api/v1/capabilities, `host` du statut d'une partie : sessionstatus.Resources
// côté serveur). Une seule mise en forme, pour la page des jeux et pour le
// panneau Performance : deux écritures finiraient par ne plus dire la même
// chose.

/** Go au-dessus du gigaoctet, Mo en dessous — comme le panneau Performance. */
function size(bytes) {
    return bytes >= 1024 ** 3 ? `${Math.round(bytes / 1024 ** 3)} Go` : `${Math.round(bytes / 1024 ** 2)} Mo`;
}

/**
 * « GPU AMD (8 Go de mémoire vidéo) · 16 cœurs logiques · 38 Go de mémoire »,
 * ou null sans ressources. Ce que la machine ne dit pas est omis.
 */
export function describeResources(resources, t) {
    if (!resources) {
        return null;
    }
    const parts = [];
    if (resources.gpu) {
        parts.push(resources.vramBytes > 0
            ? t("resources.gpuVram", { gpu: resources.gpu, vram: size(resources.vramBytes) })
            : t("resources.gpu", { gpu: resources.gpu }));
    } else {
        parts.push(t("resources.noGpu"));
    }
    if (resources.cpuThreads > 0) {
        parts.push(t("resources.threads", { count: resources.cpuThreads }));
    }
    if (resources.memoryBytes > 0) {
        parts.push(t("resources.memory", { size: size(resources.memoryBytes) }));
    }

    return parts.join(" · ");
}
