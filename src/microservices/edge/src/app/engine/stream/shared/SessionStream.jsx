import React, { useEffect, useImperativeHandle, useLayoutEffect, useRef } from "react";
import { GatewayWebRTC } from "./GatewayWebRTC.jsx";
import { acquireBackdrop, releaseBackdrop } from "./launch-backdrop.js";
import { NO_POSTER } from "./gateway-helpers.js";

// The one "connecting" surface — shared between the video-first preview
// (before the session resolves) and StreamingShell's own overlay (while the
// platform hook negotiates), so the page never visibly swaps components at
// the resolution boundary: identical pixels before and after.
// What a caller passes as connectingLabel when the picture is there and the
// overlay should get out of the way.
export const OVERLAY_HIDDEN = "live";

// What the video surface shows while no picture has arrived yet: a spinner
// and what is being waited for.
export function StreamingConnectingOverlay({ label }) {
    return (
        <div className="position-absolute top-0 start-0 w-100 h-100 text-center d-flex flex-column align-items-center justify-content-center text-white gap-3 px-3">
            <div className="spinner-border" role="status"></div>
            <div>{label}</div>
        </div>
    );
}

// LE FOND DE L'ÉCRAN DE LANCEMENT (chantier 35) — la vidéo de l'intro, que la
// passerelle tient figée pendant le chargement puis fait jouer avant le jeu.
// L'écran n'a pas d'image à lui.
//
// `launch` — `{streamId, host, username}` : le flux du lancement, dont la
// balise <video> passe d'un écran à l'autre sans s'interrompre
// (launch-backdrop.js). `stream` — un MediaStream déjà reçu : celui de la page
// de jeu, pour qui la rejoint en spectateur.
export function LaunchBackdrop({ launch = null, stream = null }) {
    const holderRef = useRef(null);
    const videoRef = useRef(null);
    const streamId = launch?.streamId || null;

    // AVANT LA PEINTURE, et c'est ce qui rend le passage invisible : l'écran
    // qui part et celui qui arrive changent dans le même rendu, et la balise
    // change de parent sans avoir quitté la page assez longtemps pour
    // s'arrêter. Si elle s'est arrêtée (une page chargée à la demande), elle
    // repart ici.
    useLayoutEffect(() => {
        if (!streamId) {
            return undefined;
        }

        const backdrop = acquireBackdrop(launch);
        holderRef.current?.appendChild(backdrop.video);
        backdrop.video.play().catch(() => { });

        return () => releaseBackdrop(backdrop);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [streamId, launch?.host?.origin, launch?.username]);

    useEffect(() => {
        const video = videoRef.current;

        if (video && stream && video.srcObject !== stream) {
            video.srcObject = stream;
            video.play().catch(() => { });
        }
    }, [stream]);

    if (streamId) {
        return <div ref={holderRef} className="position-absolute top-0 start-0 w-100 h-100" />;
    }

    if (stream) {
        return (
            <video
                ref={videoRef}
                muted
                autoPlay
                playsInline
                disableRemotePlayback
                poster={NO_POSTER}
                className="position-absolute top-0 start-0 w-100 h-100"
                style={{ objectFit: "cover", pointerEvents: "none" }}
            />
        );
    }

    return null;
}

// How many 2s cycles the video-first preview retries before giving up — a
// fresh session registers on the gateway only once its child attaches (a
// few seconds), but an invalid sessionId must not retry forever (the
// lookup timeout surfaces the real error instead).
const MAX_PREVIEW_ATTEMPTS = 30;

// SessionStream — THE video surface, for every platform and every role.
//
// The video attaches from the sessionId ALONE: no authentication, no role,
// no platform WebSocket. If the session has frames, they play — YouTube
// style: the picture never waits for anything else, the menus and the
// controller do. One viewer per mounted instance, created once and kept
// until unmount.
//
// This is deliberately the SAME component behind the Dashboard's "live now"
// thumbnails and behind /streaming. Before (2026-08-24) the streaming page
// mounted this, then threw it away at the kernel's first private push and
// built a SECOND viewer inside the platform hook's WS onOpen — that swap was
// the visible "screen exchange" going Watch → Streaming, and that second
// viewer attached once on the child's "started" message with no retry, so a
// missed "started" meant no picture at all while the input DataChannel was
// up ("no video but the controller works"). The thumbnails never had the bug
// precisely because they only ever used this path.
//
// hasInput — whether the gateway negotiates an "input" DataChannel in the
// SDP offer. This is decided by AUTHENTICATION, never by role: the role can
// change at any moment (takeover, handover, join) and renegotiating the SDP
// would restart the video, which is exactly the bug we are removing. Both
// children already drop input arriving from a viewer with no assigned pad
// (retro: handleGatewayInput's viewerPorts lookup, stream_session.go;
// streamex: OnInput's gatewaySlots lookup, cmd/streamex/main.go), so an
// authenticated spectator holding an unused channel changes nothing. Anonymous
// viewers never get a channel at all.
//
// screenRef — optional. Pass one to share this component's viewer with a
// platform hook (which sends input through it and reads `.viewerId` for its
// own wire message). Omit it for a read-only surface like a thumbnail.
// maxAttempts null = keep retrying while mounted (a dashboard card must
// recover whenever the owner finally starts streaming).
export function SessionStream({
    sessionId,
    t,
    screenRef = null,
    hasInput = false,
    username = null,
    muted = true,
    // LA VOIX (chantier 71) : `voice`, la page de jeu la négocie (un compte, jamais une vignette) ; `voicesMuted`,
    // « Entendre les autres » coupé. Voir GatewayWebRTC.
    voice = false,
    voicesMuted = false,
    // Forwarded to the surface that can actually see it — GatewayWebRTC owns
    // the <video> and watches whether its frames still advance.
    onVideoSilent = null,
    onViewerReady = null,
    maxAttempts = MAX_PREVIEW_ATTEMPTS,
    // Combien de temps on laisse à une négociation RÉUSSIE pour produire une
    // image avant de tout recommencer.
    //
    // IL A VALU 5 s, ET C'ÉTAIT UN PIÈGE. Une console qui n'émet une image-clé
    // que sur demande met plusieurs secondes à répondre ; recommencer au bout
    // de cinq revenait à démolir le viewer pendant que son image arrivait, et
    // à en ouvrir un autre pour refaire exactement la même course. Vu en
    // direct le 2026-09-07 dans le journal du gateway : un viewer ajouté,
    // connecté en 0,7 s, retiré 5 s plus tard, à l'infini — une boucle qui ne
    // pouvait pas converger, et qui coûtait un encodage de plus à chaque tour.
    //
    // La cause a été corrigée en amont (l'image-clé se rejoue maintenant quand
    // le pair est joignable, pas à la réponse SDP). Ce délai n'est plus le
    // mécanisme principal : c'est le filet, et un filet doit laisser le temps
    // de réussir avant de tirer.
    pictureTimeoutMs = 12000,
    showOverlay = true,
    // connectingLabel — what the overlay says while no picture has arrived
    // yet. The streaming page passes the PLATFORM's own state here ("waking
    // the console up", "waiting for the session"...), which is the only
    // feedback a user gets during a PS5 wake; a thumbnail leaves it null and
    // gets the generic wording.
    connectingLabel = null,
    fillHeight = false,
    children = null,
    // gatewayProps — per-platform <GatewayWebRTC> callbacks. The browser-to-
    // gateway leg is UNIVERSAL (the gateway serves the same fan-out contract
    // to every platform), so this should only ever carry callbacks, never
    // transport settings: remote-play's former audio/video, TURN-relay and
    // RTCPeerConnection options were all dropped on 2026-08-24 once they
    // turned out to be pre-gateway habits rather than requirements. What is
    // genuinely platform-specific belongs on the buffer-service-to-console
    // leg, not here. Spread last so a platform wins over the defaults above;
    // onFirstVideoTrack is chained, not replaced, so this component still
    // knows when the picture arrived.
    gatewayProps = null,
    // host — `{origin, token}` de l'hôte du flux quand ce n'est pas celui de
    // la page (vignette fédérée du tableau de bord). Voir GatewayWebRTC.
    host = null,
    // quality — le flux voulu d'emblée : `sd` (540p à 30) pour une vignette
    // de *Live now* (chantier 31) ; absent, celui de la partie.
    quality = null,
    // onQuality — l'état du double flux, dit par la passerelle (GatewayWebRTC).
    onQuality = null,
    // preview — une vignette de *Live now* : elle n'ouvre pas la partie, la
    // passerelle ne lui joue pas l'intro (chantier 35).
    preview = false,
    // onIntro — où ce spectateur en est de l'intro, dit par la passerelle
    // (GatewayWebRTC).
    onIntro = null,
    // launch — le lancement d'où vient la page : son fond et elle jouent
    // l'intro ensemble (chantier 35).
    launch = null,
}) {
    const ownRef = useRef(null);
    const attemptRef = useRef(0);
    // Recommencer depuis l'extérieur de l'effet — c'est la surveillance du
    // mouvement de l'image qui le demande, et elle vit dans le composant
    // vidéo. Posé par l'effet ci-dessous, retiré avec lui.
    const restartRef = useRef(null);
    // Une image a-t-elle été DÉCODÉE ? Distinct de « la piste est arrivée » :
    // c'est la différence entre attendre et attendre pour rien.
    const sawPictureRef = useRef(false);
    const pictureDeadlineRef = useRef(null);
    // The caller's ref (when given) and our own must point at the SAME
    // component instance — one viewer, one PeerConnection.
    useImperativeHandle(screenRef, () => ownRef.current, []);

    // onViewerReady may change identity every render (an inline arrow from
    // the page); the attach effect must not restart because of it.
    const onViewerReadyRef = useRef(onViewerReady);
    onViewerReadyRef.current = onViewerReady;

    useEffect(() => {
        let cancelled = false;
        let retryTimer = null;
        attemptRef.current = 0;
        sawPictureRef.current = false;

        const canRetry = () => maxAttempts === null || attemptRef.current < maxAttempts;

        // TOUT RECOMMENCER — le seul chemin de reprise, quelle qu'en soit la
        // raison. On rend le viewer AVANT : sinon chaque tentative en laisse
        // un derrière elle, et l'hôte encode pour des fantômes.
        const restart = () => {
            if (cancelled) {
                return;
            }

            clearTimeout(retryTimer);
            clearTimeout(pictureDeadlineRef.current);
            ownRef.current?.cleanup?.();
            // L'image d'avant ne prouve rien sur celle d'après : la prochaine
            // négociation doit refaire ses preuves, sinon son échéance
            // s'annule d'elle-même au premier tour.
            sawPictureRef.current = false;

            if (canRetry()) {
                attemptRef.current += 1;
                retryTimer = setTimeout(attempt, 0);
            }
        };

        const attempt = async () => {
            if (cancelled) {
                return;
            }

            const viewer = await ownRef.current?.createViewer({ hasInput, username, quality, preview, launch, voice });

            if (cancelled) {
                return;
            }

            if (!viewer) {
                // PAS DE SPECTATEUR, ET CE N'EST PLUS UNE ATTENTE (#751). La
                // passerelle garde la demande ouverte jusqu'à l'inscription du
                // flux (gatewaystreams.Manager.Await, vingt secondes) : un refus
                // ici est une panne (pas de partie, passerelle injoignable), et
                // la reprise bornée ci-dessous n'est plus un sondage du flux.
                if (canRetry()) {
                    attemptRef.current += 1;
                    retryTimer = setTimeout(attempt, 2000);
                }

                return;
            }

            const attached = await ownRef.current?.attach();

            if (cancelled) {
                return;
            }

            if (!attached) {
                // ON REND LE VIEWER AVANT DE RECOMMENCER.
                //
                // `createViewer` a réussi — le gateway a donc ouvert un
                // viewer pour nous — et c'est `attach` qui a échoué. Sans ce
                // ménage, la tentative suivante en ouvre un SECOND et
                // personne ne ferme le premier : chaque viewer fantôme est un
                // encodage et un envoi de plus, à la charge de l'hôte.
                //
                // Sur un serveur ça se voyait peu (le conteneur démarre une
                // fois, la fenêtre d'échec est courte). Sur le téléphone,
                // depuis que la porte « prêt » a disparu (C8-P4), cette
                // boucle s'exerce à chaque ouverture de partie — et l'hôte
                // qui encode est celui qui joue.
                ownRef.current?.cleanup?.();

                if (canRetry()) {
                    attemptRef.current += 1;
                    retryTimer = setTimeout(attempt, 2000);
                }

                return;
            }

            // UN VIEWER ATTACHÉ N'EST PAS UNE IMAGE.
            //
            // La boucle ne reprenait que sur un échec VISIBLE — pas de flux
            // enregistré, ou une négociation refusée. Le cas qui manquait est
            // celui qui ne dit rien : tout réussit, et aucune image n'arrive
            // jamais. La page attendait alors indéfiniment, sans erreur.
            //
            // Sur remote-play c'est le cas ordinaire, pas l'exception : la
            // console n'émet une image-clé QUE sur demande, et une demande
            // qui se perd n'est jamais réémise. D'où le symptôme signalé le
            // 2026-09-07 — l'image n'arrivait qu'au moment où un AUTRE
            // spectateur ouvrait la session ailleurs, parce que son arrivée
            // provoquait la demande que cette page-ci ne refaisait pas.
            //
            // Le tableau de bord n'a jamais montré le problème : ses
            // vignettes réessaient jusqu'à ce qu'une image arrive.
            const deadline = setTimeout(() => {
                if (cancelled || sawPictureRef.current) {
                    return;
                }

                restart();
            }, pictureTimeoutMs);
            pictureDeadlineRef.current = deadline;

            // The platform hook can now put this viewerId on its own wire
            // (retro's start/join/spectate, streamex's WS query) instead of
            // creating a second viewer of its own.
            onViewerReadyRef.current?.(viewer.viewerId);
        };

        restartRef.current = restart;
        attempt();

        return () => {
            cancelled = true;
            restartRef.current = null;
            clearTimeout(retryTimer);
            clearTimeout(pictureDeadlineRef.current);
            // ON REND LE VIEWER. Il ne se rendait PAS.
            //
            // Cet effet se rejoue dès que l'identité change — et elle change
            // normalement : `username` est nul au premier rendu, puis vaut le
            // porteur du jeton une fois lu. Chaque passage ouvrait donc un
            // viewer de plus sur l'hôte, et aucun ne repartait.
            //
            // Sur un serveur ça se paie en encodages inutiles. Sur le
            // téléphone, l'hôte qui encode EST celui qui joue : relevé le
            // 2026-09-08 dans le journal d'un vrai appareil — trois viewers
            // ajoutés en 1,4 s, ZÉRO retiré, et l'application abattue deux
            // secondes après le lancement. Le ménage que la boucle de reprise
            // faisait déjà entre deux tentatives (`restart`) manquait au seul
            // endroit qui compte vraiment : la sortie.
            ownRef.current?.cleanup?.();
        };
    }, [sessionId, hasInput, username, voice, maxAttempts, pictureTimeoutMs]);

    return (
        <div style={{
            zIndex: 1000,
            position: "relative",
            overflow: "hidden",
            ...(fillHeight
                ? {
                    height: "100%",
                }
                : {
                    maxHeight: "var(--gameflex-visible-height)",
                })
        }}>
            <GatewayWebRTC
                ref={ownRef}
                sessionId={sessionId}
                host={host}
                muted={muted}
                voices={voice}
                voicesMuted={voicesMuted}
                // A caller that asks for a muted surface means it: the
                // autoplay fade-in would otherwise unmute the element on
                // its own the moment playback starts, so the Dashboard
                // thumbnails started making noise (reported live
                // 2026-08-24). Once the caller flips `muted` to false the
                // element unmutes through the prop above, as it should.
                keepMuted={muted}
                onQuality={onQuality}
                onIntro={onIntro}
                // UNE IMAGE QUI S'ARRÊTE EST UNE RAISON DE RECOMMENCER, pas
                // une consigne à donner au joueur. La page disait « attendez
                // quelques secondes, ou actualisez » — c'est-à-dire qu'elle
                // demandait à la main ce qu'elle savait faire toute seule, et
                // un rafraîchissement fait perdre la manette au passage.
                //
                // Toutes plateformes : le compteur d'images décodées est la
                // même preuve partout, et une console qui se tait ne dit pas
                // pourquoi. La négociation reprend donc à zéro — c'est aussi
                // ce qui redemande une image-clé à la source.
                onVideoSilent={(silent) => {
                    onVideoSilent?.(silent);

                    if (silent) {
                        restartRef.current?.();
                    }
                }}
                {...(gatewayProps || {})}
                onFirstVideoTrack={(...args) => {
                    gatewayProps?.onFirstVideoTrack?.(...args);
                }}
                onFirstPicture={(...args) => {
                    // La boucle s'arrête ICI, et nulle part ailleurs : c'est
                    // le seul signal qui prouve qu'il y a une image.
                    sawPictureRef.current = true;
                    clearTimeout(pictureDeadlineRef.current);
                    gatewayProps?.onFirstPicture?.(...args);
                }}
            >
                {/* "live" is a SENTINEL, not a caption: it means the caller
                    has nothing left to wait for, so no overlay is drawn. Every
                    other value is shown as text. Callers pass a translated
                    string — never a phase key, which would be rendered raw. */}
                {showOverlay && connectingLabel !== OVERLAY_HIDDEN && (
                    <StreamingConnectingOverlay
                        label={connectingLabel || t("streaming.connection.state.connecting")}
                    />
                )}
                {children}
            </GatewayWebRTC>
        </div>
    );
}
