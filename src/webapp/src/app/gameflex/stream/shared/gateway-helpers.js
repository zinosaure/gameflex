import { streamingService } from "../remoteplay/service/streaming.service.js";
import { can } from "../../libs/capabilities.js";
import { streamHostOrigin } from "../../libs/serverBase.js";
import { gatewayViewerSignalUrl } from "../../api/gateway.js";
import { optimizeSdpForLowLatency } from "./webrtc-low-latency-sdp.js";

// buildIceServers — the TURN-best-effort-plus-STUN recipe every local
// RTCPeerConnection in the app builds (GatewayWebRTC, the viewer hook, the
// legacy preview paths): fetch the operator's TURN config, map it to
// RTCIceServer shape, always fall back to a public STUN server. Also
// returns the config's forceUseTurn flag so a caller that opted into
// relay-only ICE (device streaming) can apply iceTransportPolicy — one
// fetch, one mapping, no per-caller drift.
export async function buildIceServers(host = null) {
    const defaultIceServers = [{ urls: "stun:stun.l.google.com:19302" }];
    let turnServers = [];
    let forceUseTurn = false;

    // ON NE DEMANDE PAS CE QUE L'HÔTE DIT NE PAS AVOIR.
    //
    // Le repli STUN couvrait déjà l'absence de réponse, donc rien ne cassait
    // — mais un 404 à chaque négociation vidéo remplit la console de
    // quelqu'un qui y cherche autre chose (2026-09-05).
    //
    // MAIS LA CAPACITÉ DÉCRIT *CET* HÔTE-CI, et quand la page en vise un
    // autre, ce n'est plus la bonne question.
    //
    // Un téléphone déclare `turnConfig: false` — vrai de lui comme HÔTE, il
    // ne distribue pas de relais. En jouant sur la console d'un serveur
    // fédéré il n'est pas l'hôte, il est le CLIENT : le relais dont la
    // négociation a besoin est celui du serveur d'en face, et lui seul sait
    // s'il en a un. La garde répondait donc à « est-ce que J'AI un relais ? »
    // là où il fallait « est-ce que CELUI QUE JE VISE en a un ? ».
    //
    // Ce n'était sans conséquence que sur un réseau local, où STUN suffit.
    // En 5G — deux NAT, aucune route directe — la négociation n'aboutit
    // jamais et la page reste sur « Connecting… », sans erreur, sans image
    // (constaté le 2026-09-07, C9-P4).
    //
    // `host` — l'hôte du flux quand ce n'est pas celui de la page (une
    // vignette fédérée) : la même question, posée à lui.
    if (!host?.origin && !streamHostOrigin() && !can("turnConfig")) {
        return { iceServers: defaultIceServers, forceUseTurn: false };
    }

    try {
        const turnConfigResponse = await streamingService.getTurnConfig(host?.origin ? host : null);
        if (turnConfigResponse.success && turnConfigResponse.data?.turnServers) {
            turnServers = turnConfigResponse.data.turnServers
                .filter((s) => s.url)
                .map((s) => ({
                    urls: s.url,
                    ...(s.username ? { username: s.username } : {}),
                    ...(s.credential ? { credential: s.credential } : {}),
                }));
        }
        forceUseTurn = turnConfigResponse.success && turnConfigResponse.data?.forceUseTurn === true;
    } catch {
        // best-effort — fall back to STUN-only below.
    }

    return { iceServers: [...turnServers, ...defaultIceServers], forceUseTurn };
}

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
}
