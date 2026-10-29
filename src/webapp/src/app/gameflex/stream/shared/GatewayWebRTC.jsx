import React, { forwardRef, useCallback, useEffect, useImperativeHandle, useRef, useState } from "react";
import {
    gatewayViewerOffer,
    gatewayViewerDelete,
} from "../../api/gateway.js";
import { applyReceiverLatencyHints } from "../../libs/webrtcLatencyHints.js";
import { scanWebRTCReport, rateDeltas, nextCounters } from "../libs/webrtc-stats.js";
import { answerGatewayOffer, applyLowLatencyAnswerSdp, buildIceServers } from "./gateway-helpers.js";
import { streamHostOrigin } from "../../libs/serverBase.js";

// GatewayVideoScreen — the ONE way any platform (retro-gaming, streamex,
// remote-play) pulls video/audio from gateway-webrtc. Extracted from
// use-retro-streaming-connection.js (the cleanest of the three duplicated
// implementations this replaces) — see docs/wiggly-rolling-plum plan
// "Composant GatewayVideoScreen partagé" for the full context on why three
// near-identical copies of this existed.
//
// The negotiation is two-step, not one, and that's why this is an
// IMPERATIVE-handle component (a ref), not a plain declarative
// `<GatewayVideoScreen sessionId />`:
//   1. createViewer(...) asks the gateway for a viewer+offer SDP. This can
//      happen the moment a platform knows its sessionId — no dependency on
//      anything else. The viewerId it returns has to ride the PLATFORM's
//      own "start"/"join"/session-open message (the child needs it to map
//      gateway input to the right port/pad), so the caller needs it back
//      before attach().
//   2. attach() finishes the job (RTCPeerConnection, ontrack, the ICE
//      loop, the answer) — only once the platform's own protocol has
//      acknowledged the session is ready to receive media (retro's
//      "started" message, streamex's "session" message, etc.). Calling it
//      before that ack would just mean setRemoteDescription-ing an offer
//      the child hasn't necessarily finished registering yet.
//
// What this component does NOT know about: save-states, cheats, slots,
// chain-window, PS5 control hand-off — anything past "get me the pixels
// and a way to send/receive DataChannel bytes" belongs to each platform's
// own (much smaller) hook layered on top via onMessage/sendMessage.

// How long frames may stop advancing before the picture is called dead. Well
// above a stutter, well below the patience of someone staring at a frozen
// screen wondering whether it is them.
const VIDEO_SILENCE_MS = 6000;

const GatewayWebRTC = forwardRef(function GatewayWebRTC(
    {
        sessionId,
        muted = false,
        keepMuted = false,
        // LA PS5 N'A QU'UNE SOURCE, ET ELLE NE S'INITIALISE QU'UNE FOIS.
        //
        // Retro-gaming et streamex servent deux pistes indépendantes, et on
        // les met sur DEUX éléments exprès : le tampon du son ajoute de la
        // latence à l'image, et on préfère un son légèrement décalé à une
        // vidéo en retard.
        //
        // La console, elle, envoie l'image et le son ENSEMBLE — seul l'input
        // passe ailleurs. Le tampon entre elle et la passerelle devait les
        // découper en deux sources ; ça n'a jamais été fait. Les répartir
        // quand même sur deux éléments condamne la vidéo : la console
        // n'initialise son flux qu'UNE fois, et si le son arrive le premier,
        // l'image n'a plus de seconde chance (expliqué par l'utilisateur le
        // 2026-09-07, après que l'image a cessé d'arriver sur navigateur
        // mobile).
        //
        // D'où un seul élément pour remote-play. Le coût est connu et
        // documenté — Chrome asservit la vidéo à l'horloge audio, environ une
        // seconde de latence d'entrée perçue — et il a été payé une première
        // fois le 2026-08-24 en croyant s'en débarrasser gratuitement. Tant
        // que le tampon ne découpe pas, c'est le prix de l'image.
        combinedAudioVideo = false,
        // Appelée UNE fois, à la première image décodée. Distincte de
        // `onFirstVideoTrack`, qui ne dit que « la piste est arrivée » — une
        // piste peut arriver et ne jamais rien rendre, et c'est exactement le
        // cas qu'il fallait pouvoir distinguer.
        onFirstPicture = null,
        // host — `{origin, token}` de l'hôte du flux, quand ce n'est pas
        // celui que la page vise : une vignette du tableau de bord montre la
        // partie d'un serveur fédéré, diffusée par SON gateway. Absent, tout
        // suit la page.
        host = null,
        onConnectionStateChange,
        onStatsUpdate,
        onMessage,
        children,
        // onFirstVideoTrack — fires once, the first time a video track is
        // attached (remote-play requests an initial keyframe off this;
        // retro/streamex have no equivalent need).
        onFirstVideoTrack = null,
        // Called with true when frames STOP arriving on a stream that was
        // playing, and false when they come back. Lives here because this
        // component owns the <video>: the stats panel measures the same thing,
        // but only while a debug panel is open, and whether the picture is
        // alive must not depend on someone having opened a panel.
        onVideoSilent = null,
        // Fit the whole picture inside the box the caller sized, rather than
        // sizing on width alone. See the style below.
        // onIceConnectionStateChange — wired to pc.oniceconnectionstatechange.
        // Only remote-play's own SignalR-driven ICE-restart-on-disconnect
        // logic (10s of failed/disconnected before it asks the PS5 for a
        // fresh offer) needs this; connectionState already covers
        // retro/streamex's needs.
        onIceConnectionStateChange = null,
        // onAutoplayForcedMute — called when unmuting an autoplay-started
        // element is rejected by the browser's autoplay policy (no user
        // activation yet): the element would be PAUSED by Chrome, so this
        // component re-mutes and resumes silently, and the caller's mute
        // switch must follow so the user can unmute with a real gesture.
        onAutoplayForcedMute = null,
        // onQuality — l'état du double flux (chantier 31), dit par la
        // passerelle sur le canal « control » : `{current, want, available}`.
        onQuality = null,
        // onIntro — où ce spectateur en est de l'intro (chantier 35), dit sur
        // le même canal, AVANT la qualité : `hold` (l'image tenue), `play`,
        // `switch` (l'attente de l'image clé du jeu), puis `live`. Second
        // argument : `loading` quand la partie envoie des images mais ne
        // montre encore rien. Troisième : `minutes` quand l'hôte prévient
        // que le démarrage se compte en minutes (Box64 au téléphone, #521).
        // Une passerelle plus ancienne n'en dit rien.
        onIntro = null,
    },
    ref,
) {
    const videoRef = useRef(null);
    const audioRef = useRef(null);
    const pcRef = useRef(null);
    // Keyed by DataChannel label — "input" is the one every platform uses
    // today; remote-play also opens its own "keepalive" ping/pong channel
    // alongside it (see sendMessage/onMessage's label param below).
    const dataChannelsRef = useRef(new Map());
    const gatewayRef = useRef(null);
    const detachAudioPlayRetryRef = useRef(null);
    const detachVideoPlayRetryRef = useRef(null);
    const hasVideoTrackRef = useRef(false);
    const combinedAudioVideoRef = useRef(combinedAudioVideo);
    combinedAudioVideoRef.current = combinedAudioVideo;
    const onFirstPictureRef = useRef(onFirstPicture);
    onFirstPictureRef.current = onFirstPicture;

    // Kept as refs, not effect deps — attach()/ontrack are wired once per
    // connection attempt and must always call the LATEST callback/prop
    // without tearing down and re-running the whole negotiation whenever a
    // parent re-render hands in a fresh closure (same pattern already used
    // elsewhere in this codebase, e.g. handleStreamHealthCheck's refs).
    const onConnectionStateChangeRef = useRef(onConnectionStateChange);
    onConnectionStateChangeRef.current = onConnectionStateChange;
    const onStatsUpdateRef = useRef(onStatsUpdate);
    onStatsUpdateRef.current = onStatsUpdate;
    const onMessageRef = useRef(onMessage);
    onMessageRef.current = onMessage;
    const onQualityRef = useRef(onQuality);
    onQualityRef.current = onQuality;
    const onIntroRef = useRef(onIntro);
    onIntroRef.current = onIntro;
    const keepMutedRef = useRef(keepMuted);
    keepMutedRef.current = keepMuted;
    const onFirstVideoTrackRef = useRef(onFirstVideoTrack);
    onFirstVideoTrackRef.current = onFirstVideoTrack;
    const videoSilenceTimerRef = useRef(null);
    // Le rappel « une image vient d'être affichée » se réarme lui-même : il
    // faut donc de quoi l'annuler, sinon il survit à la piste qu'il observait.
    const detachFrameCallbackRef = useRef(null);
    // L'origine chez qui vit le viewer courant — null quand c'est le nôtre.
    const viewerHostRef = useRef(null);
    // LE DERNIER VIEWER DE CE COMPOSANT, gardé après qu'il est rendu : le
    // suivant le cite (`resume`), et la passerelle ne lui rejoue pas l'intro
    // s'il était arrivé au jeu (chantier 35). Un nouveau composant — une page
    // qu'on ouvre — n'en a pas, et la voit.
    const lastViewerIdRef = useRef(null);
    // Lu au moment de négocier, pas en dépendance : l'appelant construit
    // l'objet dans son rendu.
    const hostRef = useRef(host);
    hostRef.current = host;
    // Le socket de négociation du viewer courant, le temps qu'elle dure.
    const signalSocketRef = useRef(null);
    const onVideoSilentRef = useRef(onVideoSilent);
    onVideoSilentRef.current = onVideoSilent;
    const onIceConnectionStateChangeRef = useRef(onIceConnectionStateChange);
    onIceConnectionStateChangeRef.current = onIceConnectionStateChange;
    const onAutoplayForcedMuteRef = useRef(onAutoplayForcedMute);
    onAutoplayForcedMuteRef.current = onAutoplayForcedMute;

    const [isStatsEnabled, setIsStatsEnabled] = useState(false);
    const previousStatsRef = useRef(null);
    const statsIntervalRef = useRef(null);

    const setStatsMonitoring = useCallback((enabled) => {
        setIsStatsEnabled(enabled);
    }, []);

    useEffect(() => {
        if (!isStatsEnabled) {
            if (statsIntervalRef.current !== null) {
                clearInterval(statsIntervalRef.current);
                statsIntervalRef.current = null;
            }

            previousStatsRef.current = null;
            onStatsUpdateRef.current?.(null);

            return undefined;
        }

        const tick = async () => {
            const peer = pcRef.current;
            if (!peer) {
                return;
            }

            try {
                const report = await peer.getStats();
                const harvested = scanWebRTCReport(report);
                const now = performance.now();
                const previous = previousStatsRef.current;

                let downloadKbps = null;
                let uploadKbps = null;
                let fps = null;

                if (previous) {
                    const rates = rateDeltas(previous, now, harvested);
                    if (rates) {
                        downloadKbps = rates.downloadKbps;
                        uploadKbps = rates.uploadKbps;
                        fps = rates.fps;
                    }
                }
                previousStatsRef.current = nextCounters(now, harvested, previous?.framesDecoded);

                onStatsUpdateRef.current?.({
                    downloadKbps: downloadKbps != null && downloadKbps >= 0 ? downloadKbps : null,
                    uploadKbps: uploadKbps != null && uploadKbps >= 0 ? uploadKbps : null,
                    videoBitrateKbps: downloadKbps != null ? downloadKbps : null,
                    fps: fps != null && Number.isFinite(fps) ? fps : null,
                    resolution:
                        harvested.frameWidth && harvested.frameHeight
                            ? { width: harvested.frameWidth, height: harvested.frameHeight }
                            : null,
                    latencyMs: harvested.latencyMs ?? null,
                });
            } catch {
                // getStats can throw mid-renegotiation — a skipped tick is fine
            }
        };

        tick();
        statsIntervalRef.current = setInterval(tick, 1000);

        return () => {
            if (statsIntervalRef.current !== null) {
                clearInterval(statsIntervalRef.current);
                statsIntervalRef.current = null;
            }
        };
    }, [isStatsEnabled]);

    const cleanup = useCallback(() => {
        detachAudioPlayRetryRef.current?.();

        signalSocketRef.current?.close();
        signalSocketRef.current = null;

        if (gatewayRef.current) {
            gatewayViewerDelete(gatewayRef.current.sessionId, gatewayRef.current.viewerId, viewerHostRef.current);
            gatewayRef.current = null;
        }

        for (const channel of dataChannelsRef.current.values()) {
            channel.close();
        }

        dataChannelsRef.current.clear();
        hasVideoTrackRef.current = false;

        if (pcRef.current) {
            pcRef.current.close();
            pcRef.current = null;
        }

        detachVideoPlayRetryRef.current?.();
        clearInterval(videoSilenceTimerRef.current);
        detachFrameCallbackRef.current?.();
        if (videoRef.current?.srcObject) {
            videoRef.current.srcObject.getTracks().forEach((tr) => tr.stop());
            videoRef.current.srcObject = null;
        }

        if (audioRef.current?.srcObject) {
            audioRef.current.srcObject.getTracks().forEach((tr) => tr.stop());
            audioRef.current.srcObject = null;
        }
    }, []);

    useEffect(() => () => cleanup(), [cleanup]);

    // Step 1 — see the component doc above.
    //
    // sessionId param — an override for the `sessionId` prop, for a caller
    // whose session id isn't known until partway through its OWN connect
    // flow (remote-play's owner offer only learns it from
    // streamingService.createOffer's response, deep inside connect() —
    // unlike retro/streamex, which always know it up front and never pass
    // this). Falls back to the prop, so retro/streamex are unaffected.
    const createViewer = useCallback(
        async ({ hasInput = false, username, sessionId: sessionIdOverride, quality = null, preview = false, launch = null } = {}) => {
            const effectiveSessionId = sessionIdOverride ?? sessionId;
            // One component instance owns ONE viewer at a time: a platform
            // reconnect calls createViewer again on the SAME mounted
            // component (retro's spectate→start transition, a retry loop),
            // and the gateway keeps a viewer's bridge + UDP ICE socket
            // until it is explicitly deleted — overwriting the ref without
            // deleting leaked a socket per reconnect, and the gateway's
            // ICE port range filled up completely (9201-9300, 2026-08-24:
            // no video anywhere until the gateway was restarted). Release
            // the superseded viewer first.
            if (gatewayRef.current) {
                signalSocketRef.current?.close();
                signalSocketRef.current = null;
                gatewayViewerDelete(gatewayRef.current.sessionId, gatewayRef.current.viewerId, viewerHostRef.current);
                gatewayRef.current = null;
            }

            let gateway = null;

            try {
                // L'hôte de CE viewer, retenu au moment où il naît : c'est
                // chez lui qu'il faudra le rendre, et la page aura peut-être
                // déjà rendu son adresse à ce moment-là (voir
                // gatewayViewerDelete).
                viewerHostRef.current = hostRef.current?.origin || streamHostOrigin();
                gateway = await gatewayViewerOffer(effectiveSessionId, !!hasInput, username, hostRef.current?.origin || null, {
                    quality, preview, launch, resume: lastViewerIdRef.current,
                });
            } catch {
                gateway = null;
            }

            if (!gateway?.viewerId) {
                gatewayRef.current = null;
                return null;
            }

            gatewayRef.current = {
                sessionId: effectiveSessionId,
                viewerId: gateway.viewerId,
                sdp: gateway.sdp
            };
            lastViewerIdRef.current = gateway.viewerId;

            return { viewerId: gateway.viewerId, sdp: gateway.sdp };
        },
        [sessionId],
    );

    // Step 2 — see the component doc above. Uses the SAME offer SDP
    // createViewer already got (the platform's own "started"/ack message
    // never carries a fresh one) — nothing to pass in.
    const attach = useCallback(async () => {
        const gateway = gatewayRef.current;
        if (!gateway) {
            onConnectionStateChangeRef.current?.("failed");
            return false;
        }

        if (pcRef.current) {
            pcRef.current.close();
            pcRef.current = null;
        }

        const { iceServers } = await buildIceServers(hostRef.current);

        // One PeerConnection shape for every platform. A relay-only policy
        // and per-platform constructor options used to be opt-in props that
        // only remote-play passed; both were dropped (2026-08-24) — remote
        // connections work with or without TURN on all three platforms, and
        // max-bundle / rtcp-mux are Chrome's own defaults anyway.
        const pc = new RTCPeerConnection({ iceServers });
        pcRef.current = pc;

        pc.ontrack = (event) => {
            // See webrtcLatencyHints.js's doc — without this the browser's
            // own jitter buffer can hold video back by a couple of
            // seconds before settling.
            applyReceiverLatencyHints(event.receiver);

            if (event.track.kind === "video") {
                const video = videoRef.current;
                if (!video) {
                    // The <video> unmounted while a track was still in flight
                    // (SessionStream → StreamingShell transition, viewer
                    // cleanup) — the track belongs to a peer being torn down,
                    // there is nothing to attach it to.
                    return;
                }
                const stream = video.srcObject || new MediaStream();
                stream.addTrack(event.track);

                if (video.srcObject !== stream) {
                    video.srcObject = stream;
                }

                // …and ASK it to play. autoPlay alone is a request the
                // browser may refuse, and nothing here ever checked. The audio
                // path has had this retry for months; the video path relied on
                // an attribute and lost the argument silently.
                const attemptVideoPlay = () => {
                    if (video.paused) {
                        video.play().catch(() => { });
                    }
                };

                attemptVideoPlay();
                detachVideoPlayRetryRef.current?.();
                video.addEventListener("canplay", attemptVideoPlay);
                video.addEventListener("waiting", attemptVideoPlay);
                detachVideoPlayRetryRef.current = () => {
                    video.removeEventListener("canplay", attemptVideoPlay);
                    video.removeEventListener("waiting", attemptVideoPlay);
                    detachVideoPlayRetryRef.current = null;
                };

                if (!hasVideoTrackRef.current) {
                    hasVideoTrackRef.current = true;
                    onFirstVideoTrackRef.current?.();
                }

                // Is the picture still MOVING? A dead source keeps the track,
                // the connection and the last frame's dimensions — which is
                // why a console someone switched off still reads 1920x1080
                // while the frame counter, the bitrate and the FPS all sit at
                // zero (reported live 2026-08-25). Only the count of decoded
                // frames tells the difference between a still picture and a
                // stopped one.
                //
                // Polled rather than event-driven on purpose: a MediaStream
                // does not reliably fire `waiting` when its remote peer simply
                // stops sending, and silence is the one thing that emits
                // nothing by definition.
                clearInterval(videoSilenceTimerRef.current);
                // DEUX TÉMOINS, PAS UN.
                //
                // Le compteur d'images décodées est le plus précis — et il
                // n'est pas fiable partout. Sur le navigateur d'un téléphone,
                // il se fige alors que la lecture continue : la page annonçait
                // alors « la console a cessé d'envoyer la vidéo » par-dessus
                // un jeu manifestement en train de tourner, puis quittait la
                // partie vingt-cinq secondes plus tard (signalé en jouant à
                // Crash Team Racing, 2026-09-08 — capture à l'appui).
                //
                // `currentTime` est le second témoin : un élément média qui
                // lit avance son horloge, sur tous les navigateurs, sans
                // exception ni préfixe. Il est moins fin — il avance aussi
                // pendant une image figée — mais c'est exactement ce qu'on
                // veut ici : la question n'est pas « l'image bouge-t-elle ? »
                // mais « la console envoie-t-elle encore quelque chose ? ».
                //
                // On ne déclare le silence que si les DEUX se taisent. Un
                // faux positif coûte une partie ; un faux négatif coûte
                // quelques secondes d'écran noir avant que le vrai signal
                // (la connexion qui tombe) ne prenne le relais.
                const readFrames = () => video.getVideoPlaybackQuality?.().totalVideoFrames
                    ?? video.webkitDecodedFrameCount
                    ?? 0;
                const readClock = () => (Number.isFinite(video.currentTime) ? video.currentTime : 0);

                let lastFrames = readFrames();
                let lastClock = readClock();
                let lastAdvanceAt = Date.now();

                // LE TÉMOIN QUI RÉPOND VRAIMENT À LA QUESTION POSÉE.
                //
                // Les deux témoins ci-dessus sont côté AFFICHAGE : le compteur
                // d'images décodées de l'élément, et son horloge. Ils se
                // taisent aussi quand c'est le navigateur qui arrête de
                // présenter — onglet en arrière-plan, compositeur qui bride,
                // plateforme qui ne tient pas ces compteurs à jour. Le message
                // qu'on affiche alors accuse la console d'avoir cessé
                // d'émettre, ce qui est un tout autre diagnostic.
                //
                // Vu en vrai le 2026-09-10 : « la console a cessé d'envoyer la
                // vidéo » par-dessus une partie PSP qui tournait, panneau de
                // statistiques à l'appui — 1435 kbps en réception, 60 images
                // par seconde décodées, 60 en émulation.
                //
                // Le récepteur, lui, sait ce qui ARRIVE. S'il avance, la
                // console émet : quoi qu'il arrive à l'écran, ce message-là est
                // faux. On lui donne donc un droit de veto.
                let inboundFrames = -1;
                let inboundBytes = -1;
                let inboundAdvanceAt = Date.now();
                // Tant qu'aucun relevé n'a abouti, le récepteur ne vote pas :
                // sans statistiques (vieux navigateur, connexion fermée), on
                // retombe sur les témoins de l'élément plutôt que de ne plus
                // jamais rien signaler.
                let inboundSeen = false;

                const sampleInbound = async () => {
                    const peer = pcRef.current;
                    if (!peer?.getStats) {
                        return;
                    }
                    try {
                        let frames = 0;
                        let bytes = 0;
                        (await peer.getStats()).forEach((entry) => {
                            const isVideo = entry.kind === "video" || entry.mediaType === "video";
                            if (entry.type === "inbound-rtp" && isVideo) {
                                frames += entry.framesDecoded || 0;
                                bytes += entry.bytesReceived || 0;
                            }
                        });
                        inboundSeen = true;
                        if (frames > inboundFrames || bytes > inboundBytes) {
                            inboundFrames = frames;
                            inboundBytes = bytes;
                            inboundAdvanceAt = Date.now();
                        }
                    } catch {
                        // Un relevé raté n'est pas une preuve de silence.
                    }
                };
                // NOTHING is reported until the picture has moved at least
                // once. A stream that has not started yet is silent too, and
                // calling that a stopped console is what happened to remote-
                // play: a PS5 waits seconds for its first IDR, so the counter
                // sat at zero long enough to trip this, while retro and
                // streamex decode within a second and never did (reported live
                // 2026-08-25). Silence BEFORE the first frame is a wait, and
                // the page already has a phase for that.
                let seenMotion = false;
                let reportedSilent = false;

                // UN TROISIÈME TÉMOIN, ET LE SEUL QUI NE DÉDUISE RIEN.
                //
                // Les deux autres comptent ou chronomètrent, et se trompent
                // chacun à leur façon : le compteur d'images se fige sur
                // certains navigateurs de téléphone, et l'horloge d'un élément
                // média avance ou non selon l'implémentation quand la source
                // est un MediaStream. Le faux message est revenu malgré les
                // deux réunis (signalé le 2026-09-09, capture d'un jeu
                // manifestement en train de tourner).
                //
                // `requestVideoFrameCallback` ne répond pas à la même
                // question : le navigateur appelle ce rappel QUAND IL VIENT
                // D'AFFICHER UNE IMAGE. Il n'y a rien à comparer, rien qui
                // puisse se figer — si le rappel arrive, une image est passée
                // à l'écran. C'est exactement la question posée, et cette API
                // existe pour elle.
                //
                // Les deux anciens témoins restent, en repli, pour les
                // navigateurs qui ne l'ont pas : un témoin imparfait vaut
                // mieux que pas de témoin.
                //
                // ET LA PREMIÈRE IMAGE LÈVE TOUT VERDICT DE SILENCE, y compris
                // celui d'un attachement PRÉCÉDENT. `reportedSilent` est propre
                // à cet attachement : quand l'ancien avait annoncé le silence
                // pendant une reconnexion (la reprise de manette coupe le flux
                // plus de six secondes), le nouveau ne le levait jamais — le
                // message « stopped sending video » restait sur un jeu qui
                // tournait, et la page quittait la partie vingt-cinq secondes
                // plus tard (vu sur l'écran fixe de Spider-Man 3, PS3,
                // 2026-10-08).
                const markPresented = () => {
                    lastAdvanceAt = Date.now();
                    if (!seenMotion) {
                        seenMotion = true;
                        onFirstPictureRef.current?.();
                        onVideoSilentRef.current?.(false);
                    }
                    if (reportedSilent) {
                        reportedSilent = false;
                        onVideoSilentRef.current?.(false);
                    }
                };

                detachFrameCallbackRef.current?.();
                if (typeof video.requestVideoFrameCallback === "function") {
                    let handle = null;
                    const onPresented = () => {
                        markPresented();
                        handle = video.requestVideoFrameCallback(onPresented);
                    };
                    handle = video.requestVideoFrameCallback(onPresented);
                    detachFrameCallbackRef.current = () => {
                        if (handle !== null) {
                            video.cancelVideoFrameCallback?.(handle);
                            handle = null;
                        }
                        detachFrameCallbackRef.current = null;
                    };
                }

                videoSilenceTimerRef.current = setInterval(() => {
                    // A PAUSED element decodes nothing — of course the counter
                    // stops. Calling that a dead console accused the source of
                    // a choice the viewer made, put "the console has stopped
                    // sending video" on screen and then left the session ten
                    // seconds later (reported live 2026-08-25). This became
                    // reachable the day `controls` was added to the element:
                    // pausing went from impossible to one click.
                    //
                    // The clock is pushed forward rather than merely skipped,
                    // so resuming does not immediately fire on a timestamp
                    // from before the pause.
                    // Lancé sans attendre : son résultat sert au tour suivant,
                    // et un relevé lent ne doit pas décaler l'horloge de ce
                    // détecteur.
                    void sampleInbound();

                    if (video.paused || video.ended) {
                        lastFrames = readFrames();
                        lastClock = readClock();
                        lastAdvanceAt = Date.now();
                        if (reportedSilent) {
                            reportedSilent = false;
                            onVideoSilentRef.current?.(false);
                        }

                        return;
                    }

                    const frames = readFrames();
                    const clock = readClock();
                    const now = Date.now();

                    if (frames > lastFrames || clock > lastClock) {
                        lastFrames = frames;
                        lastClock = clock;
                        lastAdvanceAt = now;
                        // LA PREMIÈRE IMAGE RÉELLEMENT DÉCODÉE — pas la piste
                        // qui arrive, pas la connexion qui s'établit. C'est le
                        // seul événement qui prouve qu'il y a une image, et le
                        // seul sur lequel un appelant puisse renoncer à
                        // réessayer.
                        if (!seenMotion) {
                            onFirstPictureRef.current?.();
                            // Lève le verdict d'un attachement précédent —
                            // voir markPresented.
                            onVideoSilentRef.current?.(false);
                        }
                        seenMotion = true;
                    }

                    // LE RÉCEPTEUR TRANCHE, DANS LES DEUX SENS, QUAND IL RÉPOND.
                    //
                    // Il n'avait qu'un veto : des octets qui arrivent encore
                    // veulent dire que la source émet, et l'écran a un autre
                    // problème. Mais l'inverse manquait — un témoin de
                    // l'élément qui avance suffisait à tout taire. Sur le S25
                    // (2026-10-17, #397), la connexion d'une partie PSP est
                    // tombée quand le réseau mobile a changé les adresses du
                    // téléphone : plus un octet reçu, et pourtant aucun silence
                    // déclaré, donc aucune reprise — « Waiting for the video… »
                    // jusqu'à ce que le joueur quitte. L'horloge de l'élément y
                    // continuait d'avancer, comme le décrit le second témoin.
                    // Reproduit dans l'émulateur en lui faisant faire de même
                    // (tools/webtest/phone-netchange.js, CLOCK=avance).
                    //
                    // La source ne se tait jamais d'elle-même : le moteur
                    // envoie chaque trame, la capture X une image toutes les
                    // 400 ms au moins sur un écran fixe, les serveurs à cadence
                    // fixe. Un récepteur muet est donc une connexion morte.
                    //
                    // Sans relevé abouti (vieux navigateur), les témoins de
                    // l'élément décident seuls, comme avant.
                    const quiet = inboundSeen
                        ? now - inboundAdvanceAt > VIDEO_SILENCE_MS
                        : now - lastAdvanceAt > VIDEO_SILENCE_MS;

                    if (!quiet) {
                        if (reportedSilent) {
                            reportedSilent = false;
                            onVideoSilentRef.current?.(false);
                        }

                        return;
                    }

                    // Long enough that a hiccup or a tab in the background is
                    // not called a dead console.
                    if (!seenMotion || reportedSilent) {
                        return;
                    }

                    reportedSilent = true;
                    onVideoSilentRef.current?.(true);
                }, 2000);

                return;
            }

            // Le son sur SON PROPRE élément par défaut — deux pistes
            // indépendantes, deux horloges, pas de latence importée. En mode
            // combiné il rejoint l'élément vidéo, parce que la source d'en
            // face n'en fait qu'une (voir combinedAudioVideo, plus haut).
            // Politique d'autoplay identique dans les deux cas : démarrer
            // muet, jouer, remonter le volume une fois la lecture partie.
            const target = combinedAudioVideoRef.current ? videoRef.current : audioRef.current;

            if (!target) {
                return;
            }

            // Mute BEFORE handing the element a stream. autoPlay fires as
            // soon as srcObject lands, and an UNMUTED autoplay with no user
            // activation is blocked outright — the "playing" event never
            // comes, so the unmute/fade-in below never runs and the audio
            // stays silent for good. Muted autoplay is always allowed, so
            // this ordering is what makes the whole sequence reachable.
            const originalVolume = target.volume;
            target.muted = true;
            target.volume = 0;

            const stream = target.srcObject || new MediaStream();
            stream.addTrack(event.track);

            if (target.srcObject !== stream) {
                target.srcObject = stream;
            }

            // Shared fallback for both autoplay-block paths below: resume
            // muted (muted playback is never blocked) and tell the caller
            // the truth so its mute switch can follow.
            const resumeMutedAfterAutoplayBlock = () => {
                target.muted = true;
                target.volume = 0;
                target.play().catch(() => { });
                onAutoplayForcedMuteRef.current?.();
            };

            const attemptPlay = () => {
                if (target.paused) {
                    target.play().catch(() => { });
                }
            };

            detachAudioPlayRetryRef.current?.();
            detachAudioPlayRetryRef.current = () => {
                target.removeEventListener("canplay", attemptPlay);
                target.removeEventListener("waiting", attemptPlay);
                detachAudioPlayRetryRef.current = null;
            };

            let hasStartedPlaying = false;
            target.addEventListener(
                "playing",
                () => {
                    if (hasStartedPlaying) {
                        return;
                    }

                    hasStartedPlaying = true;

                    if (keepMutedRef.current) {
                        // Stay muted, but put the VOLUME back: ontrack zeroed
                        // it for the fade-in that we are skipping here, and a
                        // caller that later unmutes (the user flipping the
                        // sound switch) would otherwise get a perfectly
                        // unmuted element playing at volume 0.
                        target.volume = originalVolume > 0 ? originalVolume : 1;

                        return;
                    }

                    // NE PAS TENTER CE QUE LE NAVIGATEUR VA REFUSER.
                    //
                    // Sans geste de l'utilisateur sur ce document, démuter un
                    // élément démarré en lecture automatique fait PAUSER
                    // Chrome, avec son propre avertissement dans la console
                    // (« Unmuting failed and the element was paused
                    // instead… »). Le repli ci-dessous rattrapait le coup,
                    // mais au prix d'un à-coup et d'un message alarmant pour
                    // une situation parfaitement normale — signalé comme une
                    // erreur le 2026-09-10, sur une session qui marchait.
                    //
                    // `navigator.userActivation` répond à la question avant
                    // qu'on la pose. Sans activation, on reste muet tout de
                    // suite et l'interrupteur de son dit la vérité : le
                    // premier appui du joueur vaudra activation, et le son
                    // suivra à ce moment-là.
                    if (navigator.userActivation
                        && navigator.userActivation.hasBeenActive === false) {
                        resumeMutedAfterAutoplayBlock();

                        return;
                    }

                    // Le navigateur peut quand même refuser (une activation
                    // ancienne ne vaut pas toujours), donc le filet reste :
                    // on tente, et si la lecture est rejetée on remet en muet
                    // et on relance — le flux continue.
                    target.muted = false;
                    const unmutePlay = target.play();

                    if (unmutePlay && typeof unmutePlay.catch === "function") {
                        unmutePlay.catch(() => resumeMutedAfterAutoplayBlock());
                    }

                    // Chrome's pause path rejects NO promise — it just
                    // pauses the element. Watch for that pause within a
                    // short window (a later user-driven pause must not be
                    // mistaken for the block) and fall back the same way.
                    const onPausedAfterUnmute = () => {
                        window.clearTimeout(pauseWatchTimeout);
                        target.removeEventListener("pause", onPausedAfterUnmute);
                        resumeMutedAfterAutoplayBlock();
                    };
                    target.addEventListener("pause", onPausedAfterUnmute);
                    const pauseWatchTimeout = window.setTimeout(() => {
                        target.removeEventListener("pause", onPausedAfterUnmute);
                    }, 800);

                    const targetVolume = originalVolume > 0 ? originalVolume : 1;
                    const startTime = performance.now();
                    const fadeDurationMs = 500;
                    const fadeIn = (timestamp) => {
                        const progress = Math.min(1, (timestamp - startTime) / fadeDurationMs);
                        target.volume = Math.max(0, Math.min(1, targetVolume * progress));

                        if (progress < 1) {
                            requestAnimationFrame(fadeIn);
                        }
                    };
                    requestAnimationFrame(fadeIn);
                },
                { once: true },
            );

            target.addEventListener("loadedmetadata", attemptPlay, { once: true });
            target.addEventListener("canplay", attemptPlay);
            target.addEventListener("waiting", attemptPlay);
            attemptPlay();
        };

        // A device-streaming platform (remote-play — PS5 today, other
        // consoles later) can open more than one labeled channel (e.g. a
        // "keepalive" ping/pong channel alongside "input") — retro/streamex
        // only ever open one ("input"), so onMessageRef's default label
        // param keeps their call sites unchanged.
        pc.ondatachannel = (event) => {
            const { channel } = event;
            dataChannelsRef.current.set(channel.label, channel);
            channel.onclose = () => dataChannelsRef.current.delete(channel.label);
            // LE CANAL DE CONTRÔLE DU DOUBLE FLUX (chantier 31) : ce que la
            // passerelle envoie à ce spectateur, et s'il peut avoir le 540p.
            if (channel.label === "control") {
                channel.onmessage = (msg) => {
                    try {
                        const state = JSON.parse(msg.data);
                        if (state?.type === "quality") {
                            onQualityRef.current?.({ current: state.current, want: state.want, available: !!state.available });
                        } else if (state?.type === "intro") {
                            onIntroRef.current?.(state.phase, state.game || null, state.expect || null);
                        }
                    } catch {
                        // Un message illisible ne dit rien.
                    }
                };

                return;
            }
            channel.onmessage = (msg) => onMessageRef.current?.(msg.data, channel.label);
        };

        pc.onconnectionstatechange = () => {
            onConnectionStateChangeRef.current?.(pc.connectionState);
        };

        pc.oniceconnectionstatechange = () => {
            onIceConnectionStateChangeRef.current?.(pc.iceConnectionState);
        };

        // LA NÉGOCIATION SUR UN SEUL SOCKET — voir answerGatewayOffer. Le
        // socket se garde le temps qu'elle dure, pour être fermé si la page
        // s'en va avant.
        await answerGatewayOffer(pc, gateway, hostRef.current?.origin || null, (socket) => {
            signalSocketRef.current?.close();
            signalSocketRef.current = socket;
            socket.addEventListener("close", () => {
                if (signalSocketRef.current === socket) {
                    signalSocketRef.current = null;
                }
            });
        });

        return true;
    }, []);

    // label — defaults to "input" (the one channel retro/streamex ever
    // open); a device-streaming platform opening a second channel (e.g.
    // "keepalive") passes its own label. A message for a channel that
    // isn't open yet/anymore is silently dropped, same as before.
    // Rend VRAI quand le message est réellement parti.
    //
    // Il ne rendait rien, et un canal fermé était donc indistinguable d'un
    // envoi réussi. L'appelant qui avait un second chemin (le WebSocket de sa
    // plateforme) ne pouvait pas savoir qu'il fallait le prendre : il rendait
    // la main en croyant avoir envoyé. C'est ce qui a coûté toute une soirée
    // de manette morte sur streamex (2026-09-07) — le canal n'était pas
    // ouvert, le WebSocket l'était, et personne ne le lui a demandé.
    const sendMessage = useCallback((data, label = "input") => {
        const channel = dataChannelsRef.current.get(label);

        if (channel?.readyState !== "open") {
            return false;
        }

        channel.send(data);

        return true;
    }, []);

    // renegotiate — for a device pushing a FRESH offer outside the normal
    // gateway ICE-restart-on-disconnect path (a console's own signaling
    // channel telling the browser "here's a new offer, the network
    // changed"). Applies it to the SAME RTCPeerConnection this component
    // already owns, re-applies latency hints to the resulting receivers
    // (a plain renegotiation doesn't refire ontrack for tracks that
    // already existed), and returns the answer SDP for the caller to push
    // back wherever the offer came from — NOT the negotiation socket, since
    // this offer didn't come from the gateway's own viewer-offer route.
    // Retro/streamex have no such path and never call this.
    const renegotiate = useCallback(async (offerSdp) => {
        const pc = pcRef.current;

        if (!pc) {
            return null;
        }

        await pc.setRemoteDescription({ type: "offer", sdp: offerSdp });
        const answer = await pc.createAnswer();
        const answerSdp = applyLowLatencyAnswerSdp(answer.sdp);
        await pc.setLocalDescription({ type: answer.type, sdp: answerSdp });
        pc.getReceivers().forEach((receiver) => applyReceiverLatencyHints(receiver));

        return answerSdp;
    }, []);

    // setQuality — demander à la passerelle un flux pour ce spectateur
    // (`hd`, `sd`, chantier 31) ; il basculera sur sa prochaine image clé.
    const setQuality = useCallback(
        (want) => sendMessage(JSON.stringify({ type: "quality", want }), "control"),
        [sendMessage],
    );

    // getRawStats — an escape hatch for a caller that needs getStats()
    // continuously (remote-play's network-loss sampling for bitrate
    // adaptation runs regardless of whether the debug panel is open,
    // unlike setStatsMonitoring's own gated polling above). Returns null
    // if there's no live peer connection yet.
    const getRawStats = useCallback(() => pcRef.current?.getStats() ?? null, []);

    useImperativeHandle(
        ref,
        () => ({
            createViewer,
            attach,
            cleanup,
            sendMessage,
            setStatsMonitoring,
            renegotiate,
            getRawStats,
            setQuality,
            // Real DOM access for callers that need actual browser APIs a
            // data callback can't provide (e.g. Blackscreen.jsx's
            // usePictureInPictureOnHide calling video.requestPictureInPicture()
            // directly) — a getter so it always reflects the current node,
            // not a stale one captured when the handle object was built.
            get videoElement() {
                return videoRef.current;
            },
            // viewerId of the viewer this component currently owns, or null.
            // The PAGE creates the viewer now (one per page, from the
            // sessionId alone — see SessionStream), so a platform hook that
            // must put its own viewerId on the wire (retro's start/join/
            // spectate message, streamex's WS query) reads it from here
            // instead of calling createViewer a second time and tearing the
            // video back down.
            get viewerId() {
                return gatewayRef.current?.viewerId ?? null;
            },
            get iceConnectionState() {
                return pcRef.current?.iceConnectionState ?? null;
            },
        }),
        [createViewer, attach, cleanup, sendMessage, setStatsMonitoring, renegotiate, getRawStats, setQuality],
    );

    return (
        <div>
            <video
                ref={videoRef}
                autoPlay
                playsInline
                // ALWAYS muted, and that costs nothing: the sound is on the
                // <audio> sibling below, so this element never carries an
                // audio track. It used to follow the mute switch, which meant
                // that with sound on it was an UNMUTED element trying to
                // autoplay without a user gesture — blocked outright. The
                // stream then arrived, the bytes counted up, and the element
                // sat paused: the black screen at the start of every session,
                // which a manual play() fixed and which is why poking another
                // tab sometimes "woke it up" (found by the user, 2026-08-25).
                // EN MODE COMBINÉ, C'EST CET ÉLÉMENT QUI PORTE LE SON — le
                // museler pour toujours le supprimerait purement et
                // simplement. Sinon il reste muet en permanence, sans que ça
                // coûte quoi que ce soit : la piste audio est sur l'élément
                // voisin.
                muted={combinedAudioVideo ? muted : true}
                style={{
                    width: "100%",
                    minWidth: "100%",
                    maxWidth: "100vw",
                    minHeight: "100%",
                    maxHeight: "var(--gameflex-visible-height)",
                    alignSelf: "flex-start",
                    objectFit: "contain",
                    // THE WHOLE PICTURE IN ITS BOX — the caller gave this element a box with a
                    // real height (a fullscreen surface, a thumbnail) and
                    // wants the WHOLE picture inside it. Without a height the
                    // element only obeys its width, so a 4:3 game on a 16:9
                    // screen computes taller than the screen and the surface
                    // clips it: picture off the top and bottom, and a video
                    // that looks zoomed (reported live 2026-08-25 in
                    // fullscreen). `contain` letterboxes instead of cropping —
                    // black bars are the honest answer to a shape mismatch.
                }}
            />
            {audioRef && <audio ref={audioRef} autoPlay muted={muted} />}
            {children}
        </div>
    );
});

export { GatewayWebRTC };