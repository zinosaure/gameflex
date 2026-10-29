// Client for the sessions service (/api/v1 — see docs/GAMEFLEX.md and
// sessions/). Same {success, data, errorMessage} envelope and error handling
// as gameflex/api/retrogaming.js. Calls carry the user JWT — the service
// validates it locally, no auth round-trip.
import { authHeaders } from "../profile/auth/AuthContext.jsx";
import { apiUrl } from "../libs/serverBase.js";
import { kernelRequest, kernelTarget } from "../libs/kernelChannel.js";
import { federatedServerName } from "../libs/federatedHost.js";
import { invalidResponse, serverUnreachable } from "../shared/utils/error-code-translator.js";
import i18n from "@/i18n/index.js";

const BASE = "/api/v1";

// `host` — chez QUI part l'appel, quand ce n'est pas chez nous (C9-P5).
//
// Même forme que `apiRequest`'s options.host (service/api-client.js), et pour
// la même raison : l'adresse et le jeton voyagent ENSEMBLE, dans l'appel, pas
// posés à côté dans un état global qu'un autre onglet lirait. Le nôtre ne veut
// rien dire chez lui, et l'y envoyer le ferait fuiter pour rien.
//
// Absent — le cas normal — rien ne change : URL relative, jeton ambiant.
async function api(path, options = {}, token, host = null) {
    let res;
    try {
        // apiUrl : relatif comme avant dans un navigateur, absolu quand
        // l'application Android a déclaré son serveur (libs/serverBase.js).
        res = await fetch(host?.origin ? `${host.origin}${path}` : apiUrl(path), {
            ...options,
            headers: {
                ...(options.headers || {}),
                ...authHeaders(host ? host.token : token),
            },
        });
    } catch (err) {
        return serverUnreachable(host?.origin ? federatedServerName(host.origin) : "");
    }
    const json = await res.json().catch(() => invalidResponse());
    // status rides along: callers need to tell "this session is gone" (404)
    // from "no pad free right now" (409) — the same failed intent, but two
    // very different things to say to someone.
    return { ...json, status: res.status, ok: res.ok && json.success !== false };
}

// Tokenless single-session lookup for the watch route — spectating is
// protected only by the session id itself (kernel GET /api/v1/sessions/{id},
// no JWT). Un spectateur sans compte n'a pas de canal : il demande SA session,
// une fois.
export function getPublicSession(sessionId) {
    return api(`${BASE}/sessions/${encodeURIComponent(sessionId)}`, {}, null);
}

// launchSession — ouvrir un conteneur streamex pour un jeu, et rendre
// `{data: {session_id, console, game, …}}`. `game` est le titre que la
// page des jeux affiche (résolu sans casse par le catalogue du service) et
// `consoleId` choisit la console (« switch » | « ps3 »…) — sa propre image,
// son volume de jeux et ses bornes. `settings` sont les réglages de flux
// choisis avant de lancer (résolution, images par seconde, débit), revalidés
// côté service.
//
// UNE COMMANDE DU CENTRE DE MESSAGES (`launch`), comme le lancement rétro :
// ses étapes reviennent sur le même canal (`onProgress`), et la réponse porte
// le `launchId` que le kernel a frappé.
export function launchSession(game, consoleId, token, settings = null, host = null, { onProgress, core } = {}) {
    return kernelRequest(kernelTarget(host, token), "launch", {
        platform: "streamex",
        game,
        console_id: consoleId,
        // L'ÉMULATEUR CHOISI DANS LA FICHE : une console peut en déclarer
        // plusieurs, et un serveur qui n'a pas celui-là le refuse au lieu de
        // lancer le sien.
        ...(core ? { core } : {}),
        // La langue de la page, comme pour le lancement rétro (chantier 21).
        language: i18n.language,
        ...(settings?.resolution ? { resolution: settings.resolution } : {}),
        ...(settings?.frameRate ? { fps: Number(settings.frameRate) } : {}),
        ...(settings?.bitrate && settings.bitrate !== "auto"
            ? { bitrate: Number(settings.bitrate) }
            : {}),
    }, { onProgress });
}

// stopSession — arrêter une session pour TOUT LE MONDE. Réservé au
// propriétaire : c'est le kernel qui juge (`mayEndSession`), avec le jeton de
// la connexion — un 403 « not your session » sinon.
//
// `host` : une session refusée par un serveur fédéré (« tu joues déjà
// ailleurs ») vit CHEZ LUI, et son identifiant est un identifiant de chez lui.
// L'arrêter sur notre kernel ne trouverait rien — ou l'homonyme d'ici, deux
// serveurs frappant les mêmes identifiants (C9-P5).
//
// Une commande du centre de messages (`session.stop`), plus trois routes :
// la plateforme fait partie du message, le kernel aiguille.
export function stopSession(platform, sessionId, token, host = null) {
    return kernelRequest(kernelTarget(host, token), "session.stop", { platform, sessionId });
}
