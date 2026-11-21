// Thin wrapper around the Go backend's ROMs scanner
// (golang/src/internal/retrogaming) — folder scan + serve only, no
// server-side emulation. Same {success, data, errorMessage} envelope as
// gameflex/api/remoteplay.js (same Go backend).

import { authHeaders, getStoredToken } from "../profile/auth/AuthContext.jsx";
import { invalidateAuth } from "../profile/auth/auth-invalidation.js";
import { apiUrl, streamAuthToken, streamHostOrigin } from "../libs/serverBase.js";
import { kernelRequest, kernelTarget } from "../libs/kernelChannel.js";
import { federatedServerName } from "../libs/federatedHost.js";
import { invalidResponse, serverUnreachable } from "../shared/utils/error-code-translator.js";
import { can } from "../libs/capabilities.js";
import i18n from "@/i18n/index.js";

const BASE = "/api/v1/retrogaming";

// UN SEUL CONTRAT, DEUX HÔTES (chantier 8, C8-P2).
//
// Ce fichier portait sept `hasLocalLibrary()` : le téléphone servait les
// mêmes ressources sous d'autres noms et sans l'enveloppe `{success,data}`,
// et chaque fonction devait choisir. Sept endroits où les deux modèles
// pouvaient diverger sans que rien ne le signale — et ils avaient divergé.
//
// Le kernel du téléphone sert maintenant CES routes-ci, mot pour mot
// (mobileapp/kernel/retrogaming_api.go). Il n'y a plus rien à choisir, donc
// plus de branche : si une ressource manque d'un côté, c'est au serveur de
// ce côté-là de la servir, pas à la page de le contourner.

// Every route under /api/v1/retrogaming is behind the JWT middleware since
// 2026-08-17 (see retroRouter's doc, sessions/internal/api/retro_api.go) —
// the whole surface used to be reachable without a token. The token is read
// from storage rather than passed in: threading it through this module's
// fifteen functions and all their call sites would be a much larger change
// for the same result.
// `host` — chez QUI part l'appel, quand ce n'est pas chez nous (C9-P5).
//
// Même forme que `apiRequest`'s options.host et que celui de spawnSessions :
// l'adresse et le jeton voyagent ENSEMBLE, dans l'appel, jamais posés à côté
// dans un état global. Le nôtre ne veut rien dire chez lui.
//
// Absent — le cas normal — rien ne change : URL relative, jeton du stockage.
//
// LA PAGE DE JEU D'UN HÔTE FÉDÉRÉ VISE CET HÔTE (`setStreamBase`), et `apiUrl`
// y mène déjà — mais c'est SON jeton qu'il faut lui présenter, comme le fait
// `apiRequest`. On y envoyait le nôtre : l'hôte répondait 401, et ce 401, lu
// comme venant de chez nous, nous déconnectait. Signalé le 2026-10-10 : ouvrir
// les sauvegardes d'une partie fédérée (FF7 Crisis Core, PSP) renvoyait à
// l'écran de connexion.
async function api(path, options = {}, host = null) {
    const foreign = Boolean(host) || Boolean(streamHostOrigin());
    let res;
    try {
        // apiUrl : relatif comme avant dans un navigateur, absolu quand
        // l'application Android a déclaré son serveur (libs/serverBase.js).
        res = await fetch(host?.origin ? `${host.origin}${path}` : apiUrl(path), {
            ...options,
            headers: {
                ...(options.headers || {}),
                ...authHeaders(host ? host.token : (streamAuthToken() || getStoredToken())),
            },
        });
    } catch (err) {
        return serverUnreachable(host?.origin ? federatedServerName(host.origin) : "");
    }
    // UN JETON REFUSÉ FERME LA SESSION.
    //
    // Sans ça, l'app gardait un jeton mort : l'écran continuait (le kernel du
    // téléphone sert la plupart des lectures sans identité) pendant que tout
    // ce qui demande une identité échouait en silence. On voyait ses jeux et
    // on ne pouvait rien lancer — sans message (2026-09-08).
    //
    // Seulement pour NOTRE hôte : un serveur fédéré qui refuse SON jeton ne
    // dit rien de notre propre session, et nous déconnecter parce que le
    // serveur d'un ami a expiré serait absurde.
    //
    // Et seulement sur un 401. Un 403 est un geste refusé, pas un jeton
    // mort : un membre qui supprimait une sauvegarde sur le téléphone, où
    // c'est réservé à l'administrateur, se retrouvait déconnecté (relevé le
    // 2026-10-09).
    if (!foreign && res.status === 401 && getStoredToken()) {
        invalidateAuth(`${res.status} on ${path}`);
    }

    const json = await res.json().catch(() => invalidResponse());
    // LE STATUT VOYAGE AUSSI : la liste d'un serveur fédéré se dit
    // autrement quand il refuse (403) et quand il ne répond pas (502).
    return { ...json, ok: res.ok && json.success !== false, status: res.status };
}

// createSession — lancer un jeu, et rendre `{data: {sessionId}}` AVANT tout
// travail WebSocket/WebRTC : le bouton Jouer (Games.jsx) l'appelle, puis
// navigue vers la page de streaming unifiée avec ce seul identifiant — copier
// ou rafraîchir cette adresse rattache la MÊME session au lieu d'en ouvrir une
// seconde.
//
// UNE COMMANDE DU CENTRE DE MESSAGES (`launch`), plus une requête : ses étapes
// reviennent sur le même canal (`onProgress`), et la réponse porte le
// `launchId` que le kernel a frappé — la page de jeu s'y abonne pour rejouer
// l'histoire. Le serveur ET le téléphone servent cette commande, sous la même
// forme : la page ne choisit toujours pas lequel des deux lui répond.
//
// `bios` — un BIOS choisi explicitement (voir listBios plus bas) ; "" garde le
// comportement de la console.
//
// `host` — un jeu fédéré peut aussi s'ÉMULER chez celui qui l'a (C9-P5).
// `reference` doit alors être la sienne (sans le préfixe `srv<id>:`, qui
// n'existe que chez nous) et `username` le nom qu'on porte CHEZ LUI : sa file
// de joueurs est indexée par nom, et le nôtre n'y désigne personne. (Le kernel
// prend le nom du jeton de la connexion ; ce champ ne sert plus qu'à
// l'historique de lancement.)
//
// `settings` — la résolution choisie avant de lancer (#150), même panneau et
// même vocabulaire que streamex et remote-play. Absent ou vide : le conteneur
// garde la géométrie d'époque de la console (`consoles.NativeSize`).
// La cadence et le débit partent aussi, comme pour `launchSession` : le
// téléphone lance ses consoles streamex par ici, et les ignorait — une PS3
// restait en 720p à 60 images quoi qu'on choisisse (2026-10-11). Le serveur,
// pour un jeu rétro, n'en lit que la résolution.
export function createSession(
    consoleId, reference, title, core, bios, username, host = null, settings = null,
    { onProgress } = {},
) {
    return kernelRequest(kernelTarget(host, getStoredToken()), "launch", {
        platform: "retrogaming",
        consoleId, reference, title, core, bios, username,
        // LA LANGUE DE LA PAGE (chantier 21) : la console démarre dans la
        // langue de l'application, et non en anglais. Le kernel ne garde
        // aucune langue : elle voyage avec chaque lancement.
        language: i18n.language,
        ...(settings?.resolution ? { resolution: settings.resolution } : {}),
        ...(settings?.frameRate ? { fps: Number(settings.frameRate) } : {}),
        ...(settings?.bitrate && settings.bitrate !== "auto"
            ? { bitrate: Number(settings.bitrate) }
            : {}),
    }, { onProgress });
}

// reportLaunchFailure — ce que le bouton « signaler l'erreur » envoie (#191).
//
// Écrit une ligne JSON dans `logs/reports.log`, à la racine des journaux et non
// sous le jour : un signalement se lit en bloc, dans l'ordre d'arrivée. Le nom
// de l'utilisateur n'est PAS transmis — le serveur le prend du jeton, et se
// laisser dicter qui signale reviendrait à offrir une signature.
export function reportLaunchFailure({ context, steps, detail = null }) {
    return api(`${BASE}/storage/logs/report`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ context, steps, detail }),
    });
}

export function listConsoles() {
    return api(`${BASE}/consoles`);
}

// `host` — troisième et dernier de cette famille (#181). Chercher chez un pair
// le MÊME TITRE est ce qui permet de désigner la destination d'une sauvegarde :
// une référence est dérivée du nom de fichier, donc la nôtre ne vaut rien
// là-bas si son fichier s'appelle autrement. Le filtre `q` est appliqué PAR LE
// SERVEUR (`handleRetroRoms`), ce qui évite de rapatrier son catalogue entier.
// `server` — les jeux d'UN serveur fédéré plutôt que ceux de la bibliothèque
// (chantier 14) : le sélecteur de source de la liste. Le kernel répond 403
// quand ce serveur nous refuse l'accès, 502 quand il ne répond pas.
export function listRoms(consoleId, query = "", host = null, server = null, disk = null) {
    const params = new URLSearchParams();
    if (query) {
        params.set("q", query);
    }
    if (server != null) {
        params.set("server", String(server));
    }
    // UN DISQUE INTERNE (chantier 23) : ses jeux, qui se jouent ici.
    if (disk) {
        params.set("disk", disk);
    }
    const search = params.toString();

    return api(`${BASE}/${encodeURIComponent(consoleId)}/roms${search ? `?${search}` : ""}`, {}, host);
}

// SUPPRIMER un jeu — la même adresse que pour le lire, au verbe près.
//
// Réservée aux administrateurs côté serveur. Ouverte depuis le 2026-09-17,
// parce que le rapatriement l'a rendue réversible : on sait reprendre un jeu
// chez un serveur fédéré ou une ressource distante, et il se range tout seul
// au bon endroit. Un rôle `user` n'écrit ni n'efface jamais dans la
// bibliothèque.
export function deleteRom(consoleId, reference, disk = null) {
    // `disk` — la copie de CE disque interne (chantier 23) ; sans lui, celle
    // de la bibliothèque.
    const query = disk ? `?disk=${encodeURIComponent(disk)}` : "";

    return api(
        `${BASE}/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}${query}`,
        { method: "DELETE" },
    );
}

// LE STATUT D'UN JEU (chantier 30) — réservé aux administrateurs : son
// historique (versions comprises) et les émulateurs de la console, un
// signalement, ou le statut de toute une console. La liste des jeux porte
// déjà le statut de chacun (`rom.status`) ; ces routes servent à le régler.
function statusPath(consoleId, reference) {
    return `${BASE}/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}/status`;
}

export function fetchGameStatus(consoleId, reference) {
    return api(statusPath(consoleId, reference));
}

export function reportGameStatus(consoleId, reference, { status, emulators = [], note = "" }) {
    return api(statusPath(consoleId, reference), {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status, emulators, note }),
    });
}

export function fetchConsoleStatus(consoleId) {
    return api(`${BASE}/${encodeURIComponent(consoleId)}/status`);
}

export function setConsoleStatus(consoleId, status) {
    return api(`${BASE}/${encodeURIComponent(consoleId)}/status`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ status }),
    });
}

// LES RESSOURCES INTERNES (chantier 23) — les autres disques de jeux de cette
// machine, déclarés par un volume du compose (`<chemin>:/mnt/<nom>`) au
// serveur, depuis l'écran d'accueil sur le téléphone (chantier 27) : leur nom,
// s'ils sont là, et leurs jeux par console.
export function listInternalDisks() {
    return api(`${BASE}/disks`);
}

export function rescanInternalDisk(name) {
    return api(`${BASE}/disks/${encodeURIComponent(name)}/rescan`, { method: "POST" });
}

// Available BIOS file choices for this console's launch-time picker (see
// golang/internal/retrogaming/bios.go) — an empty list is a normal
// response for a console with nothing to choose (most of them today; only
// PS1 is mapped so far).
export function listBios(consoleId) {
    return api(`${BASE}/${encodeURIComponent(consoleId)}/bios`);
}

// Les CŒURS de cette console, installés ou non — chaque entrée porte
// `{name, file, sizeBytes, installed}` (CoreEntry, retro_files/cores.go).
//
// Le pendant exact de `listBios` ci-dessus, et la différence qui compte est
// dans la réponse : `/bios` liste ce que le DISQUE porte (donc tout y est
// présent par construction), tandis que `/cores` liste ce que la console
// DÉCLARE avec un drapeau de présence. C'est ce drapeau qui permet de
// proposer un émulateur en le montrant indisponible, plutôt que de le taire
// — demandé le 2026-09-17 (#163).
export function listCores(consoleId) {
    return api(`${BASE}/${encodeURIComponent(consoleId)}/cores`);
}

// `romUrl` a été SUPPRIMÉE le 2026-09-18. Elle construisait l'adresse que la
// page allait chercher elle-même, avec l'en-tête `Authorization`, pour en faire
// un blob. Depuis que le NAVIGATEUR télécharge — par ticket, voir
// `downloadTicket` juste en dessous — plus personne n'a besoin de cette adresse
// côté page : elle exige un en-tête qu'un lien ne peut pas porter, ce qui était
// précisément le problème.
//
// LA ROUTE, ELLE, RESTE : `GET /{consoleId}/roms/{ref}` sert toujours le
// fichier aux appelants qui PEUVENT poser l'en-tête (le kernel entre hôtes, la
// fédération). Ce qui disparaît est son usage depuis un navigateur.

// UN TICKET POUR QUE LE NAVIGATEUR TÉLÉCHARGE LUI-MÊME.
//
// La page téléchargeait le fichier elle-même (`fetch` + blob) parce qu'un lien
// ne peut pas porter d'en-tête `Authorization` et que toute route
// `/api/v1/retrogaming` en exige un. Tout ce que le navigateur sait faire était
// donc perdu : sa barre, la reprise, le dossier Téléchargements, et un jeu de
// plusieurs gigaoctets qui ne tient pas en mémoire.
//
// L'ÉMISSION est authentifiée (ce POST) ; la CONSOMMATION ne l'est pas, par
// nécessité de plateforme. Le ticket ne porte aucune identité, ne vaut que pour
// CE jeu, expire en une minute et se consomme au premier usage — voir
// `retro_download_ticket.go`, qui explique pourquoi c'est un ticket tenu par le
// serveur et non une signature rejouable.
export function downloadTicket(consoleId, reference) {
    return api(
        `${BASE}/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}/ticket`,
        { method: "POST" },
    );
}

// L'adresse que le navigateur ouvre avec un ticket en main. `apiUrl` et non un
// chemin nu : dans l'application Android, la page peut être servie par un AUTRE
// appareil que celui qui tient la bibliothèque.
export function ticketDownloadUrl(ticket) {
    return apiUrl(`${BASE}/downloads/${encodeURIComponent(ticket)}`);
}

// UN TÉLÉCHARGEMENT CONFIÉ AU NAVIGATEUR DU TÉLÉPHONE (chantier 14) : « c'est
// l'utilisateur qui décide dans son navigateur ». Dans l'application, la page
// ne peut pas suivre l'adresse elle-même : la WebView consommerait le ticket à
// usage unique avant que le navigateur ne le présente. Le kernel du téléphone
// demande donc à l'application de l'ouvrir dans le navigateur.
//
// Faux quand l'hôte ne le sait pas (un serveur) ou le refuse (une page qui
// n'est pas celle du téléphone : un invité, ou son propriétaire sur un autre
// appareil) — la page télécharge alors elle-même, le ticket intact.
export async function handToBrowser(url) {
    if (!can("nativeBrowse")) {
        return false;
    }
    try {
        const response = await fetch(apiUrl("/api/v1/device/browse"), {
            method: "POST",
            headers: { "Content-Type": "application/json", ...authHeaders(getStoredToken()) },
            body: JSON.stringify({ url }),
        });

        return response.ok;
    } catch {
        return false;
    }
}

// URL for the PNG captured alongside a save-state (see
// golang/internal/retrogaming/stream_session.go's captureScreenshot) — 404s
// if none exists yet. No live session needed: reads straight off disk by
// the same (consoleId, reference, username, slot) formula the backend's own
// save/load-state handlers use. slot defaults to 0 (the auto-save slot) —
// see bios.go's... no, stream_session.go's autoSaveSlot.
// `source` — une copie (chantier 38) : `local` pour les backups, ou le nom d'un
// serveur fédéré ; vide, le principal.
export function screenshotUrl(consoleId, reference, slot = 0, source = "") {
    // `apiUrl` et non un chemin nu : dans l'application Android, la page peut
    // être servie par un AUTRE appareil que celui qui tient la partie, et une
    // URL relative irait chercher la vignette chez le mauvais hôte.
    return apiUrl(screenshotPath(consoleId, reference, slot, source));
}

// screenshotPath — le chemin seul, pour la viser chez un serveur fédéré.
export function screenshotPath(consoleId, reference, slot = 0, source = "") {
    const params = new URLSearchParams({ consoleId, reference, slot });
    if (source) {
        params.set("source", source);
    }

    return `${BASE}/savestate/screenshot?${params.toString()}`;
}

// Per-slot metadata (0-9, see stream_session.go's maxSaveSlots/
// autoSaveSlot) for the Streaming menu's slot carousel — [{slot, exists,
// timestamp}], timestamp only present when exists is true. No live session
// needed, same reasoning as screenshotUrl above.
// `host` — chez QUI lister, quand ce n'est pas chez nous (#181). L'onglet
// « Récupérer » de la synchronisation doit voir les créneaux d'EN FACE, et
// c'est le compte ouvert sur ce serveur-là qui autorise : notre jeton n'y vaut
// rien. Absent, rien ne change.
export async function listSaveSlots(consoleId, reference, host = null, source = "") {
    const params = new URLSearchParams({ consoleId, reference });
    if (source) {
        params.set("source", source);
    }

    return api(`${BASE}/savestate/slots?${params.toString()}`, {}, host);
}

// Every save slot the user has across every game/console (see
// golang/internal/retrogaming/saves.go's ListUserSaveStates) — backs the
// profile page's save manager. Unlike listSaveSlots above, this needs no
// consoleId/reference: [{consoleId, reference, title, slot, timestamp}].
// `host` — pour la même raison que `listSaveSlots` juste au-dessus : l'onglet
// « Récupérer » doit voir les parties sauvegardées d'EN FACE, sous le compte
// qu'on y a ouvert.
export function listUserSaves(host = null, source = "") {
    return api(`${BASE}/savestate/user${source ? `?source=${encodeURIComponent(source)}` : ""}`, {}, host);
}

// Deletes one save slot (its .state and sibling .png screenshot).
export function deleteSaveSlot(consoleId, reference, slot, source = "") {
    const params = new URLSearchParams({ consoleId, reference, slot });
    if (source) {
        params.set("source", source);
    }

    return api(`${BASE}/savestate?${params.toString()}`, { method: "DELETE" });
}

// Deletes every slot (0-9) for one game at once.
export function deleteGameSaves(consoleId, reference, source = "") {
    const params = new URLSearchParams({ consoleId, reference });
    if (source) {
        params.set("source", source);
    }
    return api(`${BASE}/savestate/game?${params.toString()}`, { method: "DELETE" });
}

/**
 * LIRE les octets d'un créneau — ici, ou CHEZ UN PAIR (#181).
 *
 * `api()` ne peut pas servir : il finit par `res.json()`, et cette route rend
 * des octets. On reprend donc la forme du précédent maison — la vignette d'un
 * créneau (`SaveStateThumbnail`) : un `fetch` qui porte l'en-tête à la main,
 * puis `blob()`.
 *
 * `host` dit CHEZ QUI part l'appel et avec QUEL jeton. Le nôtre ne vaut rien
 * chez lui : c'est le compte ouvert sur son serveur qui autorise, et il ne vit
 * que dans ce navigateur. D'où un transfert fait par la PAGE, jamais par notre
 * kernel — il n'a pas de compte là-bas.
 */
export async function fetchSaveStateFile(consoleId, reference, slot, host = null, source = "", screenshot = false) {
    const params = new URLSearchParams({ consoleId, reference, slot });
    if (source) {
        params.set("source", source);
    }
    const path = `${BASE}/savestate/${screenshot ? "screenshot" : "file"}?${params.toString()}`;

    return fetchBytes(path, host);
}

// fetchBytes — des octets, ici ou chez un pair : la forme de la vignette
// (`SaveStateThumbnail`), un `fetch` qui porte l'en-tête à la main.
async function fetchBytes(path, host) {
    try {
        const response = await fetch(host?.origin ? `${host.origin}${path}` : apiUrl(path), {
            headers: authHeaders(host ? host.token : getStoredToken()),
        });
        if (!response.ok) {
            return { ok: false, status: response.status, errorMessage: `HTTP ${response.status}` };
        }

        return { ok: true, blob: await response.blob() };
    } catch {
        return serverUnreachable(host?.origin ? federatedServerName(host.origin) : "");
    }
}

/**
 * ÉCRIRE un créneau — chez nous ou chez un pair, TOUJOURS DANS UNE COPIE
 * (chantier 38) : `source` est le nom que la fédération donne à celui qui
 * envoie. Le principal ne s'écrit que par une restauration, sur place.
 *
 * Le corps EST les octets (voir la doc du handler : du base64 dans du JSON
 * gonflerait un état de PS2 d'un tiers en le gardant en mémoire). La RÉPONSE,
 * elle, est du JSON, donc `api()` convient — et il porte déjà le `host`.
 *
 * `timestamp` — la date de l'ORIGINAL (une date ou des millisecondes) : la
 * copie la garde, c'est la date de la partie (#395). `title` — le titre du
 * jeu, gardé avec la copie pour un jeu que l'autre côté n'a pas.
 * `screenshot` — la vignette de l'emplacement au lieu de son état.
 */
export function uploadSaveStateFile(consoleId, reference, slot, blob, { host = null, source, timestamp = null, title = "", screenshot = false }) {
    const params = new URLSearchParams({ consoleId, reference, slot, source });
    const millis = timestamp == null ? NaN : new Date(timestamp).getTime();
    if (millis > 0) {
        params.set("timestamp", String(millis));
    }
    if (title) {
        params.set("title", title);
    }
    if (screenshot) {
        params.set("screenshot", "1");
    }

    return api(`${BASE}/savestate/file?${params.toString()}`, {
        method: "POST",
        body: blob,
    }, host);
}

// ── Les sauvegardes par source (chantier 38) ─────────────────────────────
//
// Un principal, que les émulateurs jouent, et des copies : `local` pour les
// backups, une par serveur fédéré. Les mêmes routes au serveur et au
// téléphone (`/api/v1/saves`, saves_api.go des deux côtés).

const SAVES = "/api/v1/saves";

const json = (body) => ({
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
});

const gameQuery = (console, id, source) => {
    const params = new URLSearchParams({ console, id });
    if (source) {
        params.set("source", source);
    }

    return params.toString();
};

/** Le nom que ce serveur annonce à la fédération (`self`), et les copies du compte. */
export function listSaveSources() {
    return api(`${SAVES}/sources`);
}

/** Les jeux des consoles récentes, d'un rangement : `{consoles, games: [{console, id, title, updatedAt, sizeBytes}]}`. */
export function listStreamexSaves(source = "", host = null) {
    return api(`${SAVES}/streamex${source ? `?source=${encodeURIComponent(source)}` : ""}`, {}, host);
}

/** L'archive (zip) de la sauvegarde d'un jeu : `{ok, blob}`. */
export function fetchStreamexSave(console, id, source = "", host = null) {
    return fetchBytes(`${SAVES}/streamex/game?${gameQuery(console, id, source)}`, host);
}

/** Une archive, déposée dans la copie `source` — chez nous ou chez `host`. */
export function uploadStreamexSave(console, id, blob, source, host = null) {
    return api(`${SAVES}/streamex/game?${gameQuery(console, id, source)}`, { method: "POST", body: blob }, host);
}

/** La sauvegarde d'un jeu, effacée d'une copie. */
export function deleteStreamexSave(console, id, source) {
    return api(`${SAVES}/streamex/game?${gameQuery(console, id, source)}`, { method: "DELETE" });
}

/** Des jeux du principal, copiés dans `local` : `{results: [{id, error}]}`. */
export function backupStreamexSaves(console, ids) {
    return api(`${SAVES}/streamex/backup`, json({ console, ids }));
}

/** Des jeux d'une copie, remis dans le principal (le sien part dans `local`). */
export function restoreStreamexSaves(source, console, ids) {
    return api(`${SAVES}/streamex/restore`, json({ source, console, ids }));
}

/** Des emplacements du rétro (`{consoleId, reference, slot}`), copiés dans `local`. */
export function backupStates(items) {
    return api(`${SAVES}/states/backup`, json({ items }));
}

/** Des emplacements d'une copie, remis dans le principal sous `toReference`. */
export function restoreStates(source, items) {
    return api(`${SAVES}/states/restore`, json({ source, items }));
}

