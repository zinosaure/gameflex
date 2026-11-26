import { gatewayViewerSignalUrl } from "../../api/gateway.js";
import { optimizeSdpForLowLatency } from "./webrtc-low-latency-sdp.js";

// NO_POSTER — l'affiche de TOUTE vidéo de flux : une image transparente d'un
// pixel. Sans affiche, la WebView d'Android dessine la sienne tant qu'aucune
// image n'est arrivée — un gris uni et un grand triangle de lecture, étiré par
// « cover » —, là où la page voulait montrer ce qu'il y a derrière (le fond de
// l'écran de lancement, LAUNCH_BACKGROUND). Vu sur le téléphone le
// 2026-11-06, un lancement refusé avant la première image.
export const NO_POSTER = "data:image/gif;base64,R0lGODlhAQABAIAAAAAAAP///yH5BAEAAAAALAAAAAABAAEAAAIBRAA7";

// ICE_SERVERS — les serveurs ICE de TOUTE RTCPeerConnection de la page
// (GatewayWebRTC, l'arrière-plan du lancement) : un STUN public, assez pour
// que deux pairs trouvent un chemin direct. La passerelle tient le même côté
// serveur (remote-play, internal/ice_servers).
//
// Plus de relais TURN depuis le 2026-11-13 (#117, décision du porteur) : la
// page ne demande donc plus rien à l'hôte, ni au sien ni à un serveur fédéré.
// Deux réseaux qui ne se joignent pas en direct (5G derrière deux NAT) n'ont
// pas de repli ; c'est assumé.
export const ICE_SERVERS = [{ urls: "stun:stun.l.google.com:19302" }];

// Low-latency hints applied to every answer SDP, on every platform. These
// are generic receiver hints (a=x-google-flag:low-latency,
// a=minBufferedPlaybackTime) that any real-time game stream wants; they were
// remote-play-only purely because that is where they happened to be written.
// Never throws: a malformed rewrite must fall back to the original SDP rather
// than break the negotiation.
export function applyLowLatencyAnswerSdp(sdp) {
    try {
        const optimized = optimizeSdpForLowLatency(sdp);

        return optimized && optimized.length > 10 ? optimized : sdp;
    } catch (sdpError) {
        console.warn("low-latency SDP rewrite failed, using the original:", sdpError);

        return sdp;
    }
}

// LA PISTE DU MICRO (chantier 71, gatewaystreams/voice.go) : la seule piste audio que l'offre de la passerelle attend de
// la page (`recvonly`). Repérée par son `mid`, elle passe en envoi avant la réponse ; le micro s'y branche ensuite sans
// renégocier (`replaceTrack`). Une offre sans voix (une vignette, le fond du lancement) n'en a pas : null.
export function micTransceiverOf(pc, offerSdp) {
    const section = (offerSdp || "").split(/\r?\nm=/).slice(1).find((media) => media.startsWith("audio") && /\r?\na=recvonly/.test(media));
    const mid = section?.match(/\r?\na=mid:(\S+)/)?.[1];
    const transceiver = mid ? pc.getTransceivers().find((candidate) => candidate.mid === mid) : null;

    if (transceiver) {
        transceiver.direction = "sendonly";
    }

    return transceiver || null;
}

// answerGatewayOffer — LA NÉGOCIATION D'UN VIEWER SUR UN SEUL SOCKET, voir
// ServeViewerSignaling côté gateway. Elle passait par trois routes : la
// réponse en POST, un POST par candidat, et les candidats du gateway relevés
// toutes les 300 ms tant que durait leur collecte — par chaque page, pour
// chaque viewer, vignettes du tableau de bord comprises. Le gateway pousse
// maintenant les siens à mesure, et reçoit les nôtres sur le même fil.
//
// Le socket ne vit que le temps de la négociation : il se ferme quand les deux
// collectes sont finies et la réponse partie. Le viewer lui survit, et se rend
// par son DELETE. `onSocket` le reçoit dès qu'il existe, pour qui doit pouvoir
// le fermer plus tôt.
//
// Deux appelants : la vidéo d'une partie (GatewayWebRTC) et le fond de l'écran
// de lancement (launch-backdrop.js, chantier 35).
//
// Rend `{ mic }` : la piste du micro quand l'offre en prévoit une (micTransceiverOf), sinon null.
export async function answerGatewayOffer(pc, gateway, origin, onSocket = null) {
    let socket = null;
    const outbox = [];
    let answered = false;
    let gatewayGathered = false;
    let browserGathered = false;
    const send = (message) => {
        if (socket?.readyState === WebSocket.OPEN) {
            socket.send(JSON.stringify(message));
        } else if (!socket || socket.readyState === WebSocket.CONNECTING) {
            outbox.push(message);
        }
    };
    const closeWhenSettled = () => {
        if (answered && gatewayGathered && browserGathered && socket?.readyState === WebSocket.OPEN) {
            socket.close();
        }
    };

    pc.onicecandidate = (event) => {
        if (!event.candidate) {
            browserGathered = true;
            closeWhenSettled();

            return;
        }

        send({ type: "candidate", candidate: event.candidate.toJSON() });
    };
    await pc.setRemoteDescription({ type: "offer", sdp: gateway.sdp });
    const mic = micTransceiverOf(pc, gateway.sdp);

    // Ouvert APRÈS l'offre posée : le gateway rejoue tout de suite les
    // candidats qu'il a déjà, et un candidat ajouté avant la description
    // distante est refusé — donc perdu.
    socket = new WebSocket(gatewayViewerSignalUrl(gateway.sessionId, gateway.viewerId, origin));
    onSocket?.(socket);
    socket.onopen = () => {
        for (const message of outbox.splice(0)) {
            socket.send(JSON.stringify(message));
        }
        closeWhenSettled();
    };
    socket.onmessage = async (event) => {
        let message;

        try {
            message = JSON.parse(event.data);
        } catch {
            return;
        }

        if (message.type === "candidate" && message.candidate) {
            try {
                await pc.addIceCandidate(message.candidate);
            } catch {
                // duplicate, or a peer already torn down — fine
            }
        } else if (message.type === "done") {
            gatewayGathered = true;
            closeWhenSettled();
        } else if (message.type === "error") {
            console.warn("gateway refused the answer:", message.error);
        }
    };
    socket.onerror = () => {
        console.warn("gateway signaling socket failed", gateway.sessionId);
    };

    const answer = await pc.createAnswer();
    // Low-latency hints on every answer, for every platform: the
    // browser-to-gateway leg is the same contract everywhere, so a hint
    // that helps one game stream helps all three. This used to be an
    // opt-in prop only remote-play passed, which is why it read as a
    // "PS5 setting" — it never was one.
    const answerSdp = applyLowLatencyAnswerSdp(answer.sdp);
    await pc.setLocalDescription({ type: answer.type, sdp: answerSdp });
    send({ type: "answer", sdp: answerSdp });
    answered = true;
    closeWhenSettled();

    return { mic };
}
