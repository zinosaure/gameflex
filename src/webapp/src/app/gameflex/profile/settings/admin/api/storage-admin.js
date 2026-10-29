// GESTION DU STOCKAGE — le client des routes d'administration du kernel
// (kernel/internal/api/storage_admin_api.go).
//
// Les MÊMES routes des deux côtés : le kernel du téléphone les sert mot pour
// mot. C'est ce qui permet à un seul écran de servir un serveur et un
// appareil, alors que ce qu'ils contiennent n'a rien à voir — un serveur a
// une bibliothèque froide garnie à la main, un téléphone n'a que du chaud.

import { authHeaders, getStoredToken } from "../../../auth/AuthContext.jsx";
import { apiUrl } from "../../../../libs/serverBase.js";
import { serverUnreachable } from "../../../../shared/utils/error-code-translator.js";
import { rescanRemoteMount, mountListing } from "../../../api.js";

const BASE = "/api/v1/retrogaming/storage";

async function api(path, options = {}) {
    let response;
    try {
        response = await fetch(apiUrl(path), {
            ...options,
            headers: { ...authHeaders(getStoredToken()), ...(options.headers || {}) },
        });
    } catch (unreachable) {
        return serverUnreachable();
    }
    const json = await response.json().catch(() => ({}));

    return { ...(json.data || json), ok: response.ok, errorMessage: json.errorMessage };
}

// UN PAIR SE DÉSIGNE PAR SON IDENTIFIANT, JAMAIS PAR SON ADRESSE.
//
// Le navigateur ne joint pas le pair lui-même : le jeton qui ouvre son
// catalogue ne sort pas de la base du serveur. On demande donc à NOTRE
// kernel, qui relaie — même forme que pour les consoles à distance
// (`/api/v1/servers/remoteplay/devices`). C'est aussi pourquoi il n'y a pas
// de paramètre `host` ici, contrairement aux appels de lancement : il n'y a
// pas de jeton de compte à porter là-bas.
//
// Un serveur fédéré et un disque interne (chantier 24) peuvent fournir des
// cœurs, des BIOS, des paramètres et des correctifs ; un montage Samba,
// WebDAV ou kDrive ne porte que des ROMs.
const peerBase = (serverId) => `/api/v1/servers/${encodeURIComponent(serverId)}/storage`;
const diskBase = (name) => `/api/v1/retrogaming/disks/${encodeURIComponent(name)}/storage`;

// sourceBase — où se lit une section de cette source : ici, chez un pair, ou
// sur un disque interne. Les trois servent la même forme.
function sourceBase(descriptor) {
    const { kind, id } = parseSource(descriptor);
    if (kind === "peer") {
        return peerBase(id);
    }
    if (kind === "disk") {
        return diskBase(id);
    }

    return BASE;
}

// UNE SOURCE EST UN DESCRIPTEUR, PAS UN IDENTIFIANT.
//
// La matrice des sources (#154) mêle des natures différentes — « local »,
// « officielle » (buildbot), un pair fédéré, un montage distant — et un simple
// identifiant de pair ne pouvait pas les porter. D'où une chaîne :
// `local` | `official` | `peer:<id>` | `mount:<id>` | `disk:<nom>`.
//
// `official` ne demande AUCUNE route : la liste locale énumère déjà tout ce que
// le registre déclare, chaque ligne portant son adresse buildbot. Choisir cette
// source change l'origine du téléchargement, pas la liste (décidé le
// 2026-09-17 : « Local garde les absents »).
export function parseSource(descriptor) {
    const [kind, id] = String(descriptor || "local").split(":");

    return { kind, id: id || null };
}

// Les cores qu'il FAUT avoir, console par console, et lesquels sont là.
export function fetchRequiredCores(descriptor = "local") {
    return api(`${sourceBase(descriptor)}/cores`);
}

// Descend un core manquant depuis le buildbot libretro, dans
// cores/<consoleId>/. C'est le serveur qui descend, pas le navigateur : le
// fichier doit atterrir sur la machine qui fait tourner l'émulateur.
export function downloadCore(consoleId, core) {
    return api(`${BASE}/cores`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ consoleId, core }),
    });
}

// RAPATRIER un élément depuis un pair ou un disque interne, dans notre propre
// dossier.
//
// L'échange dans notre sens (#154) : les routes de listage disent ce qu'une
// source possède, celle-ci le dépose ici. C'est le kernel qui va le chercher —
// le jeton qui ouvre le catalogue d'un pair ne sort jamais de sa base, et un
// disque interne n'est lisible que par lui.
//
// Un élément déjà présent est REFUSÉ (409) plutôt qu'écrasé : un téléchargement
// automatique avait remplacé deux cœurs qui marchaient, et deux consoles
// avaient été perdues en une nuit (#142, #143).
// UNE CHARGE UTILE, PAS DEUX ARGUMENTS FIGÉS : un cœur et un BIOS se
// désignent par un nom (`{consoleId, name}`), un jeu par une RÉFÉRENCE de
// catalogue accompagnée de son titre et de ses fichiers
// (`{consoleId, reference, title, files}`) — le serveur en dérive le nom du
// fichier à écrire, et `files` ne sert qu'à connaître l'extension d'un jeu
// mono-fichier.
export function importFromSource(descriptor, section, payload) {
    return api(`${sourceBase(descriptor)}/${section}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
    });
}

// LES JEUX D'UN MONTAGE (Samba, WebDAV, kDrive), console par console.
//
// Une requête par console, comme pour le disque local : c'est la forme de la
// route, et le parcours a déjà tout classé — on lit son listing, pas le
// partage.
//
// PAS le helper `api()` de ce fichier, et la raison vaut d'être écrite : il
// fait `{...(json.data || json)}`, ce qui ÉTALE un tableau en `{0:…, 1:…}` et
// détruit la liste en silence. `mountListing` conserve le tableau sous `data`
// (enveloppe `{...json, ok}`), et c'est déjà ainsi que `ResourceContents.jsx`
// le consomme — `answer.item || answer.data || []`.
//
// Un jeu de montage se désigne par son CHEMIN et non par une référence de
// catalogue : c'est nous qui irons lire le fichier, donc il faut savoir où.
export async function fetchMountRoms(mountId, consoleIds, query = "") {
    const token = getStoredToken();
    const needle = (query || "").trim().toLowerCase();
    const wanted = consoleIds || [];
    const pages = await Promise.all(wanted.map(
        (consoleId) => mountListing(token, mountId, { console: consoleId })
            .catch(() => ({}))));

    return pages.flatMap((answer, index) => {
        const consoleId = wanted[index];

        return (answer.item || answer.data || [])
            .filter((item) => !needle || (item.name || "").toLowerCase().includes(needle))
            .map((item) => ({
                consoleId,
                reference: item.path,
                // L'UUID du jeu (chantier 14) : celui qu'il aura dans notre
                // bibliothèque, qui dit s'il y est déjà.
                id: item.id || "",
                title: item.name,
                sizeBytes: item.sizeBytes || 0,
                // Un montage ne dit pas de quels fichiers un jeu est fait dans
                // son listing ; le serveur le redemande au moment de copier.
                files: [],
            }));
    });
}

// RAPATRIER un jeu d'un montage dans notre bibliothèque (#157).
//
// C'est le serveur qui copie, et qui décide de la FORME du jeu : il relit son
// propre listing pour savoir si c'est un dossier et quels fichiers le
// composent. La page n'envoie que le chemin et la console — un « dossier »
// choisi ici désignerait n'importe quel coin du partage.
export function importFromMount(mountId, consoleId, path) {
    return api(`/api/v1/mounts/${encodeURIComponent(mountId)}/import`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ consoleId, path }),
    });
}

// SUPPRIMER un cœur ou un BIOS installé ici.
//
// Ce que ça débloque (#146) : le noyau tourne en root, donc les dossiers qu'il
// a créés dans la bibliothèque ne se laissaient plus gérer sans accès shell.
// Et le téléchargement refuse d'écraser un cœur en place (#143) en demandant
// de « le supprimer d'abord » — une consigne que le produit ne savait pas
// suivre. C'est maintenant le cycle complet : supprimer, puis retélécharger.
//
// Les deux sections ne nomment pas leur élément de la même façon (`core` pour
// un cœur, `name` pour un firmware) : c'est la forme des routes, et la
// dissimuler ici obligerait à la redécouvrir côté serveur.
export function deleteInstalled(section, consoleId, name) {
    const key = section === "cores" ? "core" : "name";

    return api(`${BASE}/${section}?consoleId=${encodeURIComponent(consoleId)}`
        + `&${key}=${encodeURIComponent(name)}`, { method: "DELETE" });
}

// METTRE À JOUR ce qu'une source liste — l'icône à côté du sélecteur.
//
// Chaque source a son listing (chantier 15) : cette machine, le sien, que
// l'icône ne refait que pour la section de l'écran (« Rescan all », dans les
// Réglages, refait tout) ; un pair, la copie du sien, que l'icône
// re-télécharge ; un montage, son parcours enregistré. La source officielle
// n'annonce aucune liste — rien à y mettre à jour, et le bouton ne s'affiche
// pas pour elle.
//
// Même nom des deux côtés (`rescan`), parce que c'est le même geste — mais
// pas le même travail : un montage se parcourt en tâche de fond, un pair et
// cette machine répondent d'un coup.
export function refreshSource(descriptor, section = "") {
    const { kind, id } = parseSource(descriptor);
    if (kind === "local") {
        return rescanSelfListing(section);
    }
    if (kind === "peer") {
        return api(`/api/v1/servers/${encodeURIComponent(id)}/rescan`, { method: "POST" });
    }
    if (kind === "disk") {
        return api(`/api/v1/retrogaming/disks/${encodeURIComponent(id)}/rescan`, { method: "POST" });
    }
    if (kind === "mount") {
        // PAS une seconde écriture de la même URL : `rescanRemoteMount` la
        // porte déjà, et deux exemplaires finissent toujours par ne plus dire
        // la même chose. Son enveloppe diffère (`exception.detail` au lieu
        // d'`errorMessage`), mais elle porte bien `ok` — et l'appelant retombe
        // déjà sur un texte traduit quand le serveur n'en fournit pas.
        return rescanRemoteMount(getStoredToken(), id);
    }

    return Promise.resolve({ ok: true, rescanned: false });
}

// UN DISQUE INTERNE N'A EN CACHE QUE LE CATALOGUE DE SES JEUX : ses cœurs,
// BIOS, paramètres et correctifs se lisent à chaque affichage (chantier 24).
// Ailleurs que sur l'écran des jeux, il n'a rien à relire.
export function canRefreshSource(descriptor, section = "") {
    const { kind } = parseSource(descriptor);
    if (kind === "disk") {
        return !section || section === "roms";
    }

    return kind === "local" || kind === "peer" || kind === "mount";
}

// LE LISTING DE CETTE MACHINE (chantier 15) : son en-tête — sa date, celle de
// chaque section —, et le refaire, en entier (« Rescan all ») ou une section.
export function fetchSelfListing() {
    return api("/api/v1/retrogaming/listing");
}

export function rescanSelfListing(section = "") {
    const query = section ? `?section=${encodeURIComponent(section)}` : "";

    return api(`/api/v1/retrogaming/listing/rescan${query}`, { method: "POST" });
}

// LA DATE DE CE QU'UNE SOURCE LISTE, pour le texte d'aide sous le sélecteur :
// la section de cette machine, ou la copie du listing d'un serveur fédéré.
// Rien pour un partage — son parcours se suit dans son propre écran — ni pour
// la source officielle.
export async function listingDateOf(descriptor, section) {
    const { kind, id } = parseSource(descriptor);
    if (kind === "local") {
        const header = await fetchData("/api/v1/retrogaming/listing");

        return header?.sections?.[section]?.updatedAt || null;
    }
    if (kind === "peer") {
        const servers = await fetchData("/api/v1/servers");
        const server = (servers || []).find((candidate) => String(candidate.id) === String(id));

        return server?.listing?.updatedAt || null;
    }
    if (kind === "disk" && section === "roms") {
        const disks = await fetchData("/api/v1/retrogaming/disks");
        const disk = (disks || []).find((candidate) => candidate.name === id);

        return disk?.scannedAt || null;
    }

    return null;
}

// fetchData — le `data` d'une réponse, tel quel : une LISTE y reste une liste,
// là où `api` l'étalerait dans un objet. Rien quand ça échoue.
async function fetchData(path) {
    try {
        const response = await fetch(apiUrl(path), { headers: authHeaders(getStoredToken()) });
        const json = await response.json().catch(() => ({}));

        return response.ok ? json.data : null;
    } catch {
        return null;
    }
}

// Les BIOS qu'il faut avoir. Pas de source officielle ici : personne ne les
// distribue légitimement — ils viennent d'un pair, ou de la console qu'on
// possède (matrice #154).
export function fetchRequiredBios(descriptor = "local") {
    return api(`${sourceBase(descriptor)}/bios`);
}

// Les ROMs d'un pair — TOUTES ses consoles d'un coup, contrairement à
// `listRoms` qui en demande une par requête.
//
// Lues dans la copie de son listing, que le kernel garde (chantier 15) et
// remet en forme (`handleRemoteStorageRoms`).
export function fetchPeerRoms(serverId) {
    return api(`${peerBase(serverId)}/roms`);
}

// LES CORRECTIFS (chantier 15) — ceux d'ici, d'un pair ou d'un disque
// interne, console par console (`{consoles: [{consoleId, console, items}]}`) ;
// une autre source dit pour chacun s'il est aussi ici (`installed`).
export function fetchPatches(descriptor = "local") {
    return api(`${sourceBase(descriptor)}/patches`);
}

export function deletePatch(consoleId, name) {
    return api(`${BASE}/patches?consoleId=${encodeURIComponent(consoleId)}`
        + `&name=${encodeURIComponent(name)}`, { method: "DELETE" });
}

// CE QU'UN PAIR OU UN DISQUE INTERNE PRÊTE DE SES PARAMÈTRES (chantiers 15 et
// 24) : ses valeurs choisies et ses préréglages perso, chacun avec le
// préréglage qu'il deviendrait ici (`as`) et s'il y est déjà (`installed`).
export function fetchSourceParams(descriptor) {
    return api(`${sourceBase(descriptor)}/params`);
}

export function fetchLogs() {
    return api(`${BASE}/logs`);
}

// Les cent dernières lignes d'un journal.
export function fetchLogTail(path) {
    return api(`${BASE}/logs/tail?path=${encodeURIComponent(path)}`);
}

export function deleteLog(path) {
    return api(`${BASE}/logs?path=${encodeURIComponent(path)}`, { method: "DELETE" });
}

export function fetchUserSaves() {
    return api(`${BASE}/saves`);
}

export function deleteUserSaves(username) {
    return api(`${BASE}/saves?user=${encodeURIComponent(username)}`, { method: "DELETE" });
}

// LE CACHE DU DOSSIER (#448) : ce que le premier lancement d'un jeu installe
// (`cache/installs/<console>/`), console par console. Une entrée se retire par
// son nom ; les deux kernels refusent pendant une partie de sa console (409).
export function fetchCache() {
    return api(`${BASE}/cache`);
}

export function deleteCacheEntry(consoleId, name) {
    return api(`${BASE}/cache?consoleId=${encodeURIComponent(consoleId)}&name=${encodeURIComponent(name)}`,
        { method: "DELETE" });
}

// LES PARAMÈTRES DES ÉMULATEURS — `params/<console>/<émulateur>.json`, servis
// par les deux kernels (gameflex/emuparams, #347) : chaque lanceur les
// applique avant l'émulateur. Le PUT pose l'interrupteur et les valeurs du
// formulaire — et, avec `preset`, les garde aussi en préréglage sous ce nom ;
// les préréglages arrivent avec la liste et ne font que remplir les champs.
const emulatorPath = (consoleId, emulator) =>
    `${BASE}/params/${encodeURIComponent(consoleId)}/${encodeURIComponent(emulator)}`;

export function fetchEmulatorParams() {
    return api(`${BASE}/params`);
}

// `engines` — le préréglage que chaque moteur de jeu pose (chantier 36) ;
// absent, le kernel n'y touche pas.
export function writeEmulatorParams(consoleId, emulator, { enabled, values, preset = "", engines }) {
    return api(emulatorPath(consoleId, emulator), {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ enabled, values, preset, ...(engines ? { engines } : {}) }),
    });
}

// Supprimer un préréglage qui se remplace (`override`) ; ceux en lecture
// seule — ceux de GameFlex — sont refusés par le kernel.
export function deleteEmulatorPreset(consoleId, emulator, presetId) {
    return api(`${emulatorPath(consoleId, emulator)}/presets/${encodeURIComponent(presetId)}`, {
        method: "DELETE",
    });
}

// Un TÉLÉCHARGEMENT ne passe pas par `fetch` : le fichier doit arriver dans
// les téléchargements du navigateur, pas en mémoire. Le jeton voyage donc
// dans l'URL — la même raison que pour les WebSocket, un lien ne porte pas
// d'en-tête `Authorization`.
export function logFileUrl(path) {
    return apiUrl(`${BASE}/logs/file?path=${encodeURIComponent(path)}`
        + `&token=${encodeURIComponent(getStoredToken() || "")}`);
}

export function savesArchiveUrl(username) {
    return apiUrl(`${BASE}/saves/archive?user=${encodeURIComponent(username)}`
        + `&token=${encodeURIComponent(getStoredToken() || "")}`);
}
