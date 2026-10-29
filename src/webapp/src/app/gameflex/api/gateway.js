// Canonical frontend helpers for gateway-webrtc viewer signaling — the ONE
// surface every platform converges on (Phase 2, see docs/WEBRTC_GATEWAY.md):
// the offer (POST), the negotiation socket, the release (DELETE), viewerId
// always in the query string. Proxied by webpack directly to
// gateway-webrtc (no JWT: the gateway is reachable plain like the other
// backends).
//
// serverBase-aware (C7-P2) : sur le téléphone la page vit sur l'origine de
// l'application et le gateway sur le kernel local — apiUrl() préfixe par
// http://127.0.0.1:10200. Sur le web, serverOrigin() est vide et rien ne
// change.
import { apiUrl, wsUrl } from "../libs/serverBase.js";

// Lazy, never frozen at module load : index.js pose window.GAMEFLEX_SERVER
// (127.0.0.1:10200 sur un téléphone avec gateway) AVANT le premier rendu,
// et une constante évaluée à l'import le raterait.
const BASE = () => apiUrl("/api/v1/gateway-webrtc/streams");

// `origin` — L'HÔTE DE CE FLUX, quand ce n'est pas celui que la page vise :
// une vignette du tableau de bord montre la partie d'un serveur fédéré, et
// son gateway est chez lui. Absent, tout suit `serverOrigin()` comme avant.
const streamsBase = (origin) => (origin ? `${origin}/api/v1/gateway-webrtc/streams` : BASE());

// UN ÉCHEC SE REND DANS LA FORME QUE L'APPELANT LIT DÉJÀ, jamais en rejetant :
// le kernel local (127.0.0.1:10200) peut mourir EN COURS de réponse. L'offre
// rend `null` (l'appelant teste `viewerId`).
//
// `quality` — celle que la page veut d'emblée (`sd` pour une vignette de
// *Live now*, chantier 31) ; absente : le flux de la partie.
//
// L'INTRO (chantier 35) : `preview` — une vignette, qui n'ouvre pas la
// partie : pas d'intro ; `launch` — le lancement d'où vient la page (le fond
// de son écran et la page de jeu la jouent ensemble) ; `resume` — le
// spectateur que celui-ci remplace dans la même page : une négociation
// reprise ne la rejoue pas.
export async function gatewayViewerOffer(sessionId, hasInput, username, origin = null, { quality = null, preview = false, launch = null, resume = null } = {}) {
    try {
        const res = await fetch(`${streamsBase(origin)}/${encodeURIComponent(sessionId)}/viewer-offer`, {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            // username = the browser's self-reported identity for the unified
            // connectionPool (empty for tokenless watch) — carried through
            // viewer-joined by the gateway to the child, then to the kernel.
            body: JSON.stringify({
                hasInput: !!hasInput,
                ...(username ? { username } : {}),
                ...(quality ? { quality } : {}),
                ...(preview ? { preview: true } : {}),
                ...(launch ? { launch } : {}),
                ...(resume ? { resume } : {}),
            }),
        });

        if (!res.ok) {
            return null;
        }

        return await res.json();
    } catch (failure) {
        console.error("[gateway] viewer-offer : appel ou lecture du corps interrompu", failure);

        return null;
    }
}

// LA SUITE DE LA NÉGOCIATION — la réponse SDP et les candidats des deux
// côtés — passe par UN WebSocket par viewer (voir `attach` dans
// GatewayWebRTC.jsx, et ServeViewerSignaling côté gateway). Il remplace
// `viewer-answer`, un `viewer-ice` par candidat, et `viewer-candidates`, que
// chaque page sondait toutes les 300 ms.
//
// `wsUrl` suit `serverOrigin()`, comme `apiUrl` pour l'offre : le socket
// s'ouvre pendant que la page est là, chez l'hôte qui a créé le viewer.
export function gatewayViewerSignalUrl(sessionId, viewerId, origin = null) {
    const path = `/api/v1/gateway-webrtc/streams/${encodeURIComponent(sessionId)}/viewer-ws?viewerId=${encodeURIComponent(viewerId)}`;

    return origin ? `${origin.replace(/^http/, "ws")}${path}` : wsUrl(path);
}

// `origin` — L'ADRESSE VOYAGE AVEC LE VIEWER, parce que celui-ci lui survit.
//
// Toutes les autres requêtes de ce fichier partent pendant que la page est
// là, donc `apiUrl` vise le bon serveur. Celle-ci part au DÉMONTAGE, et
// l'adresse que la page visait peut déjà avoir été rendue : le viewer était
// alors relâché chez nous, où son identifiant ne désigne personne (404 sur
// 10200, lu depuis l'application le 2026-09-07), pendant que l'hôte
// continuait d'encoder pour un spectateur parti.
//
// L'appelant retient donc l'origine au moment de créer le viewer et la rend
// ici. Un viewer appartient à un hôte ; son identifiant ne veut rien dire
// ailleurs.
export function gatewayViewerDelete(sessionId, viewerId, origin = null) {
    return fetch(
        `${streamsBase(origin)}/${encodeURIComponent(sessionId)}/viewer/${encodeURIComponent(viewerId)}`,
        { method: "DELETE" },
    ).catch(() => { });
}
