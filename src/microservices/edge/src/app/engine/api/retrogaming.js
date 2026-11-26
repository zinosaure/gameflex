// Thin wrapper around the Go backend's ROMs scanner
// (golang/src/internal/retrogaming) — folder scan + serve only, no
// server-side emulation. Same {success, data, errorMessage} envelope as
// engine/api/remoteplay.js (same Go backend).

import { authHeaders, getStoredToken } from "../../libs/sharedHost.js";
import { invalidateAuth } from "../../libs/auth-invalidation.js";
import { apiUrl, streamAuthToken, streamHostOrigin } from "../../libs/serverBase.js";
import { kernelRequest, kernelTarget } from "../../libs/kernelChannel.js";
import { federatedServerName } from "../../libs/federatedHost.js";
import { invalidResponse, serverUnreachable } from "../../libs/error-code-translator.js";
import { can } from "../../libs/capabilities.js";
import i18n from "../../i18n.js";

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
// `settings` — le flux de la console dans ce navigateur (#150 ; `streamOf`,
// GameSettings.jsx), même panneau et même vocabulaire que streamex et
// remote-play, son débit « Auto » déjà mesuré (withNetworkBitrate). Absent :
// le kernel prend le défaut de toutes les consoles, 720p à 60 (le porteur,
// 2026-11-23) — ce n'est plus la géométrie d'époque. La cadence et le débit
// partent aussi, comme pour `launchSession` : le téléphone lance ses consoles
// streamex par ici, et les ignorait — une PS3 restait en 720p à 60 images quoi
// qu'on choisisse (2026-10-11).
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

// UN TÉLÉCHARGEMENT CONFIÉ AU NAVIGATEUR DU TÉLÉPHONE (chantier 14) : « c'est
// l'utilisateur qui décide dans son navigateur ». Dans l'application, la page
// ne peut pas suivre l'adresse elle-même : la WebView consommerait le ticket à
// usage unique avant que le navigateur ne le présente. Le kernel du téléphone
// demande donc à l'application de l'ouvrir dans le navigateur.
//
// Faux quand l'hôte ne le sait pas (un serveur) ou le refuse (une page qui
// n'est pas celle du téléphone : un invité, ou son propriétaire sur un autre
// appareil) — la page télécharge alors elle-même, le ticket intact.
//
// L'ADRESSE PART ABSOLUE (chantier 56, P8) : le kernel du téléphone n'ouvre que
// ses propres liens, `http://127.0.0.1:<port>/api/v1/retrogaming/downloads/…`
// (handleBrowse), et le navigateur d'Android ne saurait que faire d'un chemin.
// Edge passait le chemin seul : refusé (400), le jeu partait dans la WebView,
// qui ne télécharge pas — vu dans l'émulateur Android le 2026-11-15.
export async function handToBrowser(url) {
    if (!can("nativeBrowse")) {
        return false;
    }
    try {
        const response = await fetch(apiUrl("/api/v1/device/browse"), {
            method: "POST",
            headers: { "Content-Type": "application/json", ...authHeaders(getStoredToken()) },
            body: JSON.stringify({ url: new URL(url, window.location.href).href }),
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

// Deletes one save slot (its .state and sibling .png screenshot).
export function deleteSaveSlot(consoleId, reference, slot, source = "") {
    const params = new URLSearchParams({ consoleId, reference, slot });
    if (source) {
        params.set("source", source);
    }

    return api(`${BASE}/savestate?${params.toString()}`, { method: "DELETE" });
}

