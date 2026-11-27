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
export const GAME_STATUSES = ["playable", "patched", "glitches", "crashes", "unplayable", "untested"];
// La couleur Bootstrap de chaque statut (son badge, son filtre).
export const STATUS_VARIANT = { playable: "success", patched: "info", glitches: "primary", crashes: "warning", unplayable: "danger", untested: "secondary" };
// La même couleur en pastille (Réglages : l'historique des signalements, la fiche d'un jeu).
export const STATUS_TONE = { playable: "green", patched: "teal", glitches: "sky", crashes: "amber", unplayable: "rose", untested: "slate" };
export const statusOf = (rom) => rom?.status?.status || "";

// UNE INFORMATION D'UN SOUS-TITRE EN « CLÉ : VALEUR » (helpLines, bricks/) : la valeur seule à côté de sa clé ; rien
// quand elle est vide.
// UN COMPTE PORTE TOUJOURS SON UNITÉ (le porteur, 2026-11-23 : « il faut toujours une unité même si dans le key c'est déjà
// présent. Par exemple : no. of games: 999 games ») : « 999 jeux », « 2 consoles », « 30 fps » (le porteur, 2026-11-26 : « Ça doit être pareil partout. fps ») — `units.<unité>`
// des textes, au pluriel de la langue, l'unité telle qu'elle s'écrit (le porteur, 2026-11-25 : « Pour les unités tu peux
// les remettre en normal. Hormis les unités de calcul pour les octets et horaires » — la règle « Mets l'unité toujours en
// majuscule » du 2026-11-23 est retirée ; les octets et les heures ne passent pas par ici) ; rien quand le compte est
// inconnu.
export const counted = (t, unit, count, values = {}) => (count === null || count === undefined || count === ""
    ? null : t(`units.${unit}`, { count: Number(count), ...values }));

// UNE DATE ET SON HEURE, dans la langue de la page (« 20 nov. 2026, 00:25 ») ; un tiret quand on ne la connaît pas — la
// même partout (Membres, Fédération, Mes sauvegardes).
// `missing` — ce qui s'écrit sans date : un tiret dans une valeur, rien dans une phrase. LE JOUR SEUL (`day`) et L'HEURE
// SEULE (`timeOf`, « 14:05 ») se lisent ici aussi : aucune page n'écrit sa propre date.
const localeOf = (language) => (language === "fr" ? "fr-FR" : "en-GB");
export const dateTime = (iso, language, missing = "—") => (iso ? new Date(iso).toLocaleString(localeOf(language), { dateStyle: "medium", timeStyle: "short" }) : missing);
export const day = (iso, language, missing = "—") => (iso ? new Date(iso).toLocaleDateString(localeOf(language), { dateStyle: "medium" }) : missing);
export const timeOf = (iso, language, missing = "") => (iso ? new Date(iso).toLocaleTimeString(localeOf(language), { hour: "2-digit", minute: "2-digit" }) : missing);

export const fact = (label, value) => (value === null || value === undefined || value === "" || value === false ? null : { label, value });

// Le nom d'un serveur fédéré, celui de sa ligne dans la Fédération (son nom, sinon son adresse).
// LE NOM D'UN SERVEUR FÉDÉRÉ, une fois : son nom, son adresse, sinon « Serveur n° … » (avec `t`) ou son numéro.
// `serverName` le cherche dans la liste ; `serverNamed` le lit sur le serveur déjà trouvé.
export const serverNamed = (server, id, t = null) => server?.hostname || server?.link || (t ? t("consoles.serverNamed", { id }) : String(id));
export const serverName = (servers, id, t = null) => {
    const server = (servers || []).find((entry) => String(entry.id) === String(id));

    return serverNamed(server, id, t);
};

// D'OÙ VIENT UN JEU, LA SEULE FORME (le porteur, 2026-11-23 : « tout doit rester cohérent ») : le serveur fédéré, la
// ressource externe, le disque interne, sinon la bibliothèque — la ligne d'un jeu, sa fiche, son engrenage, ses
// sauvegardes, ses favoris.
export const originOf = (game, servers, t) => (game.peer != null ? serverName(servers, game.peer) : game.resource != null ? game.resourceName || t("catalog.resourceSource")
    : game.disk || t("games.source.library"));

// LE TEMPS DEPUIS UN DÉBUT (une partie : son `startedAt`), en HH:MM:SS ; vide sans début.
export function formatDuration(startedAt, now = Date.now()) {
    if (!startedAt) {
        return "";
    }
    const seconds = Math.max(0, Math.floor((now - new Date(startedAt).getTime()) / 1000));

    return [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60].map((part) => String(part).padStart(2, "0")).join(":");
}

export const MEGABYTE = 1024 * 1024;

// POURQUOI LA FICHE D'UN JEU FÉDÉRÉ NE SE LIT PAS : notre jeton refusé (403), un jeu qu'il ne prête pas (404), ou le
// serveur injoignable. Une fois, pour le Catalogue et les Réglages des consoles.
export const sheetRefusalOf = (status, server, t) => (status === 403 ? t("gameAdmin.sheetDenied", { server })
    : status === 404 ? t("gameAdmin.sheetNotLent", { server }) : t("gameAdmin.sheetUnreachable", { server }));
