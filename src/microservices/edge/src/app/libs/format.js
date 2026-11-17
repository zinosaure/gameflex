// La taille d'un jeu, comme la webapp (games/helpers.js) — rien quand on ne la connaît pas.
export function formatSize(bytes) {
    if (!bytes) {
        return "";
    }
    const units = ["B", "KB", "MB", "GB", "TB"];
    let value = bytes;
    let unit = 0;

    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }

    return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

// LA TAILLE D'UN ÉMULATEUR OU D'UN BIOS, TOUJOURS EN MB (le porteur, 2026-11-13 : une ligne dit « installé ou non ·
// version · taille en MB ») : un BIOS de 512 Kio est « 0.5 MB », une clé « 0.01 MB » ; trois chiffres au plus.
export function formatMegabytes(bytes) {
    if (!bytes) {
        return "";
    }
    const value = bytes / (1024 * 1024);

    return `${Number(value.toFixed(value >= 100 ? 0 : value >= 1 ? 1 : 2)) || 0.01} MB`;
}

// Les statuts d'un jeu (chantier 30) : le serveur les calcule, `rom.status.status`.
export const GAME_STATUSES = ["playable", "crashes", "unplayable", "untested"];
// La couleur Bootstrap de chaque statut (son badge, son filtre).
export const STATUS_VARIANT = { playable: "success", crashes: "warning", unplayable: "danger", untested: "secondary" };
export const statusOf = (rom) => rom?.status?.status || "";

// LE TEMPS DEPUIS UN DÉBUT (une partie : son `startedAt`), en HH:MM:SS ; vide sans début.
export function formatDuration(startedAt, now = Date.now()) {
    if (!startedAt) {
        return "";
    }
    const seconds = Math.max(0, Math.floor((now - new Date(startedAt).getTime()) / 1000));

    return [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60].map((part) => String(part).padStart(2, "0")).join(":");
}
