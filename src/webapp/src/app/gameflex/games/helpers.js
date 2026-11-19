// LES PETITES CHOSES QUE TOUT LE DOSSIER PARTAGE — et rien d'autre.
//
// Aucun import : c'est la condition pour que ce fichier ne crée pas de cycle
// entre la carte, la fiche et la liste, qui s'en servent tous les trois.


export const ITEMS_PER_PAGE = 10;
export const SEARCH_DEBOUNCE_MS = 300;

export function formatSize(bytes) {
    if (!bytes) return "—";
    const units = ["B", "KB", "MB", "GB"];
    let value = bytes;
    let unit = 0;
    while (value >= 1024 && unit < units.length - 1) {
        value /= 1024;
        unit++;
    }
    return `${value.toFixed(unit === 0 ? 0 : 1)} ${units[unit]}`;
}

// D'où vient ce jeu (chantier 6, C6-P5).
//
// Un jeu d'ici ne porte AUCUNE mention, et c'est délibéré : le local est le
// cas ordinaire, et marquer l'ordinaire fait du bruit sur toute une liste de
// vingt mille lignes. Seul ce qui vient d'ailleurs se signale.
//
// La mention n'est pas une décoration : un jeu distant ne se lance pas comme
// un jeu d'ici — il faut d'abord le récupérer. Le voir avant de cliquer
// évite de prendre une attente pour une panne.
export const ORIGIN_ICON = {
    local: "bi bi-phone",
    server: "bi bi-hdd-network",
    smb: "bi bi-hdd-network",
    webdav: "bi bi-cloud",
    kdrive: "bi bi-cloud-arrow-down",
};

// Le nom de l'origine, pour la FICHE d'un jeu — la liste, elle, n'en compte
// que le nombre (voir GameCard). Sur un seul jeu, celui qu'on s'apprête à
// lancer, « d'où il sort » n'est pas une décoration : une copie d'ici démarre
// tout de suite, une copie d'ailleurs se télécharge d'abord, et un partage
// monté dépend d'une machine qui peut être éteinte.
export const ORIGIN_LABEL = {
    local: "retrogaming.origin.local",
    server: "retrogaming.origin.federation",
    smb: "retrogaming.origin.smb",
    webdav: "retrogaming.origin.webdav",
    // kDrive parle son API depuis le 2026-09-02, ce n'est plus du WebDAV et
    // il porte donc son propre nom. Sans cette ligne, la fiche d'un jeu du
    // drive disait « Unknown source » — le repli, pris pour un diagnostic.
    kdrive: "retrogaming.origin.kdrive",
};

// `downloadFileName` a été SUPPRIMÉE le 2026-09-18. Elle relisait le
// `Content-Disposition` de la réponse pour nommer le fichier, et retombait sur
// le nom du catalogue puis sur le titre. Ce travail n'existait QUE parce que la
// page téléchargeait elle-même et que passer par un blob perd cet en-tête :
// depuis que le navigateur télécharge (par ticket), il lit l'en-tête lui-même,
// que le serveur pose déjà (`handleRetroServeRom`).
