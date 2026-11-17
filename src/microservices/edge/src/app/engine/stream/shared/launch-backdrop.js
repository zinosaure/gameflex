import { gatewayViewerOffer, gatewayViewerDelete } from "../../api/gateway.js";
import { applyReceiverLatencyHints } from "../../libs/webrtcLatencyHints.js";
import { streamHostOrigin } from "../../../libs/serverBase.js";
import { ICE_SERVERS, NO_POSTER, answerGatewayOffer } from "./gateway-helpers.js";

// LE FOND DE L'ÉCRAN DE LANCEMENT — chantier 35.
//
// Pendant un lancement, l'écran n'a pas d'image à lui : son fond est la
// vidéo de l'intro, figée sur sa première image, que la passerelle envoie sur
// le flux du lancement (le kernel l'ouvre avant la première étape). Le
// chargement fini, elle reprend, puis le jeu lui succède — c'est la passerelle
// qui décide, la page ne fait que montrer.
//
// CE FOND TRAVERSE LES ÉCRANS D'UN LANCEMENT : celui de la commande, celui de
// la page de jeu tant que la partie se résout, celui posé sur la partie
// jusqu'à la première image du jeu (screens/play/Launch.jsx). Plusieurs écrans, mais UN
// spectateur et UNE balise <video>, gardés ici : chaque écran l'accroche chez
// lui en montant, et la relâche en partant. Le dernier parti, elle vit
// encore RELEASE_GRACE_MS — le temps que l'écran suivant la reprenne, de
// l'autre côté d'une navigation.
//
// LE MÊME LANCEMENT que la page de jeu (`launch`) : la passerelle leur joue
// l'intro ensemble — la page de jeu la rejoint en cours, ou entre dans le jeu
// si elle est finie. Et le même compte : il a un canal d'entrée dès qu'il a un
// compte, comme elle — sans lui, le Remote Play le compterait comme un
// spectateur de plus. Il n'y envoie rien.
//
// MUET : le son est celui de la page de jeu.

// L'IMAGE DU FOND QUAND LA VIDÉO N'EN A PAS ENCORE (le porteur, 2026-11-06 :
// « le background de GameFlex ») : la première image de l'intro, celle-là même
// que la passerelle tient pendant le lancement — extraite de son hd.h264, que
// gatewaystreams/intro/build.sh refait avec elle. L'écran de lancement la pose
// en fond (LaunchScreen) ; la vidéo, transparente tant qu'elle n'a rien
// (NO_POSTER), la laisse voir, puis la recouvre de la même image. Un lancement
// refusé avant toute image, ou un serveur sans intro, montre donc le même fond.
export const LAUNCH_BACKGROUND = "/images/launch-backdrop.webp";

// Le même fond, en propriété CSS `background` : l'écran du lancement et
// l'attente de la page de jeu (screens/play/Launch.jsx, Play.jsx).
export const LAUNCH_SCREEN_BACKGROUND = `#0d1117 url(${LAUNCH_BACKGROUND}) center / cover no-repeat`;

const RELEASE_GRACE_MS = 3000;

const backdrops = new Map();

function createVideo() {
    const video = document.createElement("video");
    video.muted = true;
    video.autoplay = true;
    video.playsInline = true;
    // Un fond ne se « caste » pas : sans cela, le navigateur du téléphone
    // propose de l'envoyer sur un téléviseur.
    video.disableRemotePlayback = true;
    video.poster = NO_POSTER;
    Object.assign(video.style, {
        position: "absolute",
        inset: "0",
        width: "100%",
        height: "100%",
        objectFit: "cover",
        pointerEvents: "none",
    });

    return video;
}

// UNE SEULE DEMANDE (#751) : un lancement annoncé a son flux dès la première
// étape, et la passerelle attend celui qui n'est pas encore né
// (gatewaystreams.Manager.Await). Elle se redemandait trois fois, à 1,5 s, pour
// un serveur d'avant le chantier 35. Sans réponse, l'écran garde son fond noir.
async function connect(backdrop) {
    const offer = await gatewayViewerOffer(
        backdrop.streamId, Boolean(backdrop.username), backdrop.username, backdrop.host?.origin || null,
        { launch: backdrop.streamId },
    );

    if (!offer?.viewerId) {
        return;
    }

    backdrop.viewerId = offer.viewerId;

    if (backdrop.closed) {
        gatewayViewerDelete(backdrop.streamId, offer.viewerId, backdrop.viewerOrigin);

        return;
    }

    try {
        const pc = new RTCPeerConnection({ iceServers: ICE_SERVERS });

        backdrop.pc = pc;
        pc.ontrack = (event) => {
            if (event.track.kind !== "video") {
                return;
            }

            applyReceiverLatencyHints(event.receiver);
            backdrop.video.srcObject = new MediaStream([event.track]);
            backdrop.video.play().catch(() => { });
        };
        await answerGatewayOffer(
            pc,
            { sessionId: backdrop.streamId, viewerId: offer.viewerId, sdp: offer.sdp },
            backdrop.host?.origin || null,
            (socket) => {
                backdrop.socket = socket;
            },
        );
    } catch {
        // Un fond qui ne vient pas laisse le noir d'avant : le lancement,
        // lui, continue.
    }
}

function close(backdrop) {
    backdrop.closed = true;
    backdrops.delete(backdrop.key);
    backdrop.socket?.close();

    if (backdrop.viewerId) {
        gatewayViewerDelete(backdrop.streamId, backdrop.viewerId, backdrop.viewerOrigin);
    }

    backdrop.pc?.close();
    backdrop.video.srcObject?.getTracks().forEach((track) => track.stop());
    backdrop.video.srcObject = null;
    backdrop.video.remove();
}

// acquireBackdrop — le fond de ce flux, ouvert s'il ne l'est pas encore.
// `streamId` est l'identifiant du lancement ; `host`, `{origin, token}` quand
// le jeu se lance chez un ami : son flux est sur SA passerelle.
export function acquireBackdrop({ streamId, host = null, username = null }) {
    const key = `${host?.origin || ""}|${streamId}`;
    let backdrop = backdrops.get(key);

    if (!backdrop) {
        backdrop = {
            key,
            streamId,
            host,
            username,
            // Là où le spectateur se rendra : l'adresse que la page vise
            // peut avoir changé d'ici là (voir gatewayViewerDelete).
            viewerOrigin: host?.origin || streamHostOrigin(),
            video: createVideo(),
            holders: 0,
            releaseTimer: null,
            closed: false,
            viewerId: null,
            pc: null,
            socket: null,
        };
        backdrops.set(key, backdrop);
        connect(backdrop);
    }

    clearTimeout(backdrop.releaseTimer);
    backdrop.holders += 1;

    return backdrop;
}

// releaseBackdrop — un écran s'en va. Le dernier laisse au suivant le temps
// de le reprendre.
export function releaseBackdrop(backdrop) {
    backdrop.holders -= 1;

    if (backdrop.holders > 0) {
        return;
    }

    clearTimeout(backdrop.releaseTimer);
    backdrop.releaseTimer = setTimeout(() => close(backdrop), RELEASE_GRACE_MS);
}
