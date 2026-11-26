import React, { useCallback, useEffect, useLayoutEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { useTranslation } from "react-i18next";
import { SessionStream, OVERLAY_HIDDEN } from "../../engine/stream/shared/SessionStream.jsx";
import { useVoiceChat } from "../../engine/stream/shared/hooks/use-voice-chat.js";
import { useRetrogamingInput } from "../../engine/stream/retrogaming/hooks/use-retrogaming-input.js";
import { useStreamexInput } from "../../engine/stream/streamex/hooks/use-streamex-input.js";
import { useRemotePlayInput } from "../../engine/stream/remoteplay/hooks/use-remoteplay-input.js";
import { useTouchScreen } from "../../engine/stream/shared/hooks/use-touch-screen.js";
import { rotationFallback, usePortraitViewport, rotationSurfaceStyle } from "../../engine/stream/shared/hooks/use-screen-rotation.js";
import { MENU_BUTTON_STYLE, MENU_IDLE_OPACITY, MENU_SLIDE_DISTANCE, useIdleVisibility, useSlideToOpen } from "../../engine/stream/shared/hooks/use-streaming-menu.js";
import { useAdaptiveQuality } from "../../engine/stream/shared/hooks/use-adaptive-quality.js";
import { useChildSessionWs, CHILD_BEAT_INTERVAL_MS } from "../../engine/stream/shared/hooks/use-child-session-ws.js";
import { markActivity, takeActivityStamp } from "../../engine/stream/shared/activity-store.js";
import { createSwitchesStore } from "../../engine/stream/shared/switches-store.js";
import { castErrorKey, castSender } from "../../engine/stream/onscreen/cast.js";
import { TouchpadOnlyContext, PAD_COLUMN_WIDTH, PadLookContext } from "../../engine/stream/onscreen/Controllers.jsx";
import { standbyConsole, stopSession } from "../../engine/api/spawnSessions.js";
import { setGameInputSuspended } from "../../engine/libs/gameInput.js";
import { connectedGamepads } from "../../engine/libs/keyMapping.js";
import { useScreenOrientation } from "../../libs/screenOrientation.js";
import { usePhysicalGamepad } from "../../engine/libs/gamepadPresence.js";
import { useNames } from "../../libs/displayNames.js";
import { onDeviceFailed, recentDeviceFailure } from "../../engine/libs/deviceStream.js";
import { streamHostUsername } from "../../libs/serverBase.js";
import { requestFullscreen, exitFullscreen, isFullscreen, isMobileDevice, isTabletDevice } from "../../libs/device-detection.js";
import { getStoredUsername, useConsoles } from "../../libs/sharedHost.js";
import { PAD_LOOK } from "../../libs/padLook.jsx";
import { useMenuHotkey } from "../../libs/play.js";
import { menuGestureOf } from "../../engine/libs/gameInput.js";
import { can } from "../../libs/capabilities.js";
import { fetchCastSettings } from "../../libs/api.js";
import { focusElement, suspendNavigation, useScreenInput } from "../../libs/input.js";
import PlayLaunch from "./Launch.jsx";
import { Bell, useSessionNotifications } from "./Bell.jsx";
import PlayNotices from "./Notices.jsx";
import { StatsPanel, StatsStrip, STATS_STRIP_HEIGHT } from "./Stats.jsx";
import { PlayMenu, useQuitFlow } from "./Menu.jsx";
import ShaderSpinner from "../../engine/stream/onscreen/ShaderSpinner.jsx";
import Skeleton from "../../skeleton/skeleton.js";
import { useButton } from "../../libs/ui.js";
import { fact } from "../../libs/format.js";

// LA PAGE DE JEU — sa coquille (StreamingShell de la webapp, chantier 52) : la vidéo, l'écran du
// lancement, la manette à l'écran, le menu et la sortie, au style du launcher. Le MOTEUR est celui de
// la webapp, repris tel quel (app/engine/) : la connexion à la passerelle, le canal de la session (le
// child, seule autorité sur qui joue), les entrées de chaque plateforme. Cette coquille ne fait que
// les relier, et dessiner.
//
// Montée avec key={session.id} (Play.jsx) : un changement de session est un remontage complet, ce qui
// garde sûr l'aiguillage par plateforme (useStreamingInput) au regard des règles des hooks.

// Combien de temps on attend la passerelle avant de brancher la plateforme sans elle : une session
// sans flux du tout (passerelle injoignable, child jamais attaché) doit atteindre son propre chemin
// d'erreur au lieu d'attendre en silence.
const PLATFORM_CONNECT_WITHOUT_VIEWER_MS = 8000;
// Le temps de lire ce qui s'est passé avant de partir — partir ne coûte rien : la sortie déconnecte,
// elle n'arrête pas, et le child garde la session cinq minutes. Un jeu sorti de lui-même (#504) se lit
// plus longtemps ; une source muette (une console éteinte) laisse sa chance à la reprise de
// SessionStream, qui dure douze secondes.
const SESSION_ENDED_REDIRECT_MS = 2500;
const GAME_EXITED_REDIRECT_MS = 6000;
const SOURCE_GONE_REDIRECT_MS = 25_000;
// Une intro tenue au-delà se dit « en attente de l'image du jeu » (#520).
const INTRO_HELD_SAY_MS = 6000;

// CE QUE LA PAGE ATTEND, DÉRIVÉ de ce qu'elle sait déjà, jamais rangé : la première ligne vraie gagne,
// la plus décisive est donc posée d'abord. « live » : plus rien d'attendu côté page — l'image manque
// seule, et c'est elle qu'on dit attendre.
export function sessionPhase({ ended, gameExited, authorityOpen, authorityAnswered, claiming, platformConnected, videoSilent, stalling }) {
    if (ended && gameExited) return "gameExited";
    if (ended) return "ending";
    if (!authorityOpen) return "connecting";
    if (!authorityAnswered) return "joining";
    if (claiming) return "waitingControl";
    if (!platformConnected) return "waitingVideo";
    // AVANT « stalling » : une source arrêtée et un flux qui peine se ressemblent à l'écran, et un seul
    // vaut qu'on l'attende.
    if (videoSilent) return "sourceGone";
    if (stalling) return "stalling";

    return "live";
}

// EN SPECTATEUR, LES ÉTAPES SONT DÉJÀ ACQUISES : il arrive sur une partie qui tourne. La dernière est
// celle qui est en cours pour lui — l'attache du flux.
const WATCH_LAUNCH_STEPS = [
    "session.requested", "session.ready", "game.loaded",
    "core.required", "core.resolved", "core.loaded", "bios.required",
    "container.creating", "container.created", "container.waiting", "container.ready",
    "rom.ready", "bios.resolved", "emulator.ready", "stream.attaching",
].map((step) => ({ step }));

// L'AIGUILLAGE PAR PLATEFORME — une seule forme pour les trois (useStreamingInput de la webapp). La
// partie du téléphone (`device`) fait du rétro : le même hook la pilote.
function useStreamingInput({ screenRef, session, context, t, options = {} }) {
    const { isWatching, drivesPad, childSlot, onConnectionError, onProgress } = options;

    switch (session.platform) {
        case "device":
        case "retrogaming":
            return useRetrogamingInput({ screenRef, session, context, t, options: { isWatching, childSlot, onConnectionError, onProgress } });
        case "streamex":
            return useStreamexInput({ screenRef, session, t, options: { isWatching, drivesPad } });
        default:
            return useRemotePlayInput({ screenRef, session, context, t, options: {} });
    }
}

// L'IMAGE DANS L'IMAGE quand l'onglet passe en arrière-plan — au mieux, sans jamais lever.
function usePictureInPictureOnHide(screenRef) {
    useEffect(() => {
        const onVisibility = async () => {
            const video = screenRef.current?.videoElement;

            if (!video) {
                return;
            }
            if (document.hidden) {
                if (document.pictureInPictureEnabled && video.requestPictureInPicture) {
                    await video.requestPictureInPicture().catch(() => {});
                }
            } else if (document.pictureInPictureElement === video) {
                document.exitPictureInPicture().catch(() => {});
            }
        };
        document.addEventListener("visibilitychange", onVisibility);

        return () => document.removeEventListener("visibilitychange", onVisibility);
    }, [screenRef]);
}

// L'ARRÊT POUR TOUT LE MONDE (réservé au propriétaire, le kernel en juge). Une partie de l'appareil
// n'a pas de session sur un serveur : le `terminate` du hook l'arrête.
function stopForEveryone(session, token) {
    return session.platform === "device" ? Promise.resolve() : stopSession(session.platform, session.id, token);
}

// `watchOnly` (`#747`) : LE « WATCH » GRAND PUBLIC, celui que le Chromecast montre — personne n'a de jeton chez l'hôte
// de la partie (Play.jsx). La vidéo vient de la passerelle, publique ; la coquille n'ouvre ni le canal de la partie (le
// child, qui distribue les manettes) ni les entrées de la plateforme, elle ne se présente pas à la passerelle sous un
// nom, et son menu se réduit à l'information, au son, au plein écran, au lien pour regarder, à la vidéo et à Quitter.
// `hostName` : le serveur fédéré qui la tient, pour le dire.
export default function PlayShell({ user, token, session, notices, setTerminateReason, onTerminate, reportPlaying, reportIdle, onWatchingChange, leaveRef, wantsPlay, onPlayIntentSent, launchSteps = [], launchId = null, watchOnly = false, hostName = "" }) {
    const { t } = useTranslation();
    const button = useButton();
    // Ce que le moteur attend de Skeleton (`context.toast.open`), servi par les avis de la page (Play.jsx).
    const context = notices.context;

    // UNE PARTIE DU TÉLÉPHONE n'a pas d'autorité à attendre : elle EST l'autorité. Elle est à son
    // propriétaire ; les autres y arrivent en spectateurs (2026-10-23).
    const isDeviceSession = session?.platform === "device";
    const ownsDeviceSession = isDeviceSession && Boolean(session?.owner) && session.owner === user?.username;
    const deviceGame = isDeviceSession ? String(session?.id || "").replace(/^device:/, "") : "";

    // UNE PARTIE DU TÉLÉPHONE QUI NE DÉMARRE PAS SE DIT ICI : l'appareil annonce l'échec sur son canal,
    // souvent avant que la page ne s'abonne — on regarde d'abord s'il y en a un, récent, pour CE jeu.
    useEffect(() => {
        if (!isDeviceSession) {
            return undefined;
        }
        const end = (failure) => setTerminateReason(failure?.reason ? `${t("play.failed")} (${failure.reason})` : t("play.failed"));
        const already = recentDeviceFailure(deviceGame);

        if (already) {
            end(already);

            return undefined;
        }

        return onDeviceFailed((failure) => {
            if (!failure?.game || failure.game === deviceGame) {
                end(failure);
            }
        });
    }, [isDeviceSession, deviceGame, setTerminateReason, t]);

    // La façade du téléphone ne connaît qu'une session fixe ; la page garde son propre identifiant.
    const childSessionId = session.childSessionId || session.id;

    // TOUT LE MONDE ARRIVE EN SPECTATEUR, et c'est le child qui dit qui joue (son état poussé) :
    // isWatching se REÇOIT, il ne se déduit pas. Un compte du téléphone qui n'en est pas le propriétaire regarde, et
    // demande la manette.
    const [isWatching, setWatching] = useState(!ownsDeviceSession);
    const [kernelPrivateReady, setKernelPrivateReady] = useState(isDeviceSession);
    // MA place, de la bouche du child — pas la copie du pool, qui arrive plusieurs relais plus tard.
    const [childSlot, setChildSlot] = useState(null);
    // Les deux bouts d'un prêt (#274), tels que le child les dit pour CET onglet.
    const [childLoan, setChildLoan] = useState({ canReclaim: false, canReturn: false, lentTo: "" });
    // QUELLE MANETTE pilotent les touches de cet onglet (`you.controls`) ; pendant un prêt, le
    // propriétaire garde sa place et ne pilote rien, l'emprunteur regarde et pilote.
    const [childControls, setChildControls] = useState(null);
    const [childStateSeen, setChildStateSeen] = useState(false);
    const [sessionEnded, setSessionEnded] = useState(false);
    const [gameExited, setGameExited] = useState(false);
    // Dit par la surface vidéo elle-même (GatewayWebRTC suit le compteur d'images).
    const [videoSilent, setVideoSilent] = useState(false);
    const authorityGoneRef = useRef(null);
    useEffect(() => () => clearTimeout(authorityGoneRef.current), []);
    const [slotRequests, setSlotRequests] = useState([]);
    const [broadcastPlayers, setBroadcastPlayers] = useState([]);
    const [broadcastWatchers, setBroadcastWatchers] = useState([]);
    // Les noms affichés de qui joue et de qui regarde, demandés au besoin (un spectateur qui vient d'arriver n'est pas
    // dans la liste des parties).
    const displayNames = useNames([session.owner, ...broadcastPlayers.map((player) => player.username), ...broadcastWatchers]);
    const nameOf = useCallback((username) => displayNames[username] || username, [displayNames]);
    // LA LISTE DES JOUEURS DU MENU (chantier 68) : les manettes de la console, son propriétaire, et depuis quand plus
    // personne ne joue (le décompte de son arrêt).
    const [roster, setRoster] = useState({ maxPlayers: 0, owner: "", idleSince: null });
    // LA VOIX (chantier 71) : ce que la file en dit à son dernier état (`{ room, you }`) ; null avant le premier.
    const [voiceState, setVoiceState] = useState(null);
    const voiceResultRef = useRef(null);
    // UN JOUEUR PARTI RESTE 30 SECONDES « DÉCONNECTÉ » DANS LA LISTE (le porteur, 2026-11-19 : « c'est juste pour la
    // visuelle. En backend on ne change rien […] pour éviter de voir la disparition d'un joueur d'un seul coup ») — sa
    // place est déjà libre pour le child ; il revient, sa ligne redevient la sienne.
    const [departed, setDeparted] = useState([]);
    const departedTimers = useRef(new Map());
    useEffect(() => () => departedTimers.current.forEach(clearTimeout), []);
    const playersRef = useRef([]);
    // LES NOTIFICATIONS DE LA PARTIE ET LA CLOCHE (Bell.jsx) ; le son suit celui de la partie (posé plus bas, une fois
    // ses réglages lus).
    const soundRef = useRef(true);
    const notifications = useSessionNotifications(soundRef);
    const notify = notifications.push;
    // MA DEMANDE DE MANETTE EN COURS : sa réponse (ou son silence) devient une notification.
    const outgoingRef = useRef(null);
    useEffect(() => () => clearTimeout(outgoingRef.current?.timer), []);

    const hasPhysicalGamepad = usePhysicalGamepad();
    // La surface qui passe en plein écran : l'image ET ce qui est posé dessus.
    const shellRef = useRef(null);
    // Un téléphone ou une tablette, lu une fois : une fenêtre qui traverse 768 px est un bureau.
    const [isHandheld] = useState(() => isMobileDevice() || isTabletDevice());
    const isPortraitViewport = usePortraitViewport();
    // LE viewer de la passerelle de cette page (un par page, toute sa vie) : les hooks le reçoivent.
    const screenRef = useRef(null);
    // Lu une fois, au premier rendu : il entre dans l'offre WebRTC. Chez un hôte fédéré, c'est le nom
    // qu'on y porte qui compte.
    const viewerUsernameRef = useRef(null);

    if (viewerUsernameRef.current === null) {
        viewerUsernameRef.current = watchOnly ? "" : streamHostUsername() || getStoredUsername() || "";
    }
    const viewerUsername = viewerUsernameRef.current || null;

    // LES JALONS DU CONTENEUR (#191), à la suite de ceux du kernel : une liste, du clic à l'image.
    const [childSteps, setChildSteps] = useState([]);
    // LA PREMIÈRE IMAGE, et celle du JEU (chantier 35) : la passerelle envoie d'abord l'intro, et dit
    // où en est ce spectateur ; une passerelle qui ne dit rien a envoyé le jeu.
    const [sawPicture, setSawPicture] = useState(false);
    const [introLive, setIntroLive] = useState(false);
    const introToldRef = useRef(false);
    const [gameLoading, setGameLoading] = useState(false);
    const [introPhase, setIntroPhase] = useState(null);
    const [slowStart, setSlowStart] = useState(false);
    const [heldLong, setHeldLong] = useState(false);
    // Le flux de la partie, pour le fond de l'écran d'attente de qui la rejoint.
    const [shellStream, setShellStream] = useState(null);
    const noteChildStep = useCallback((step, detail) => {
        setChildSteps((previous) => [...previous, { step, detail, state: "done" }]);
    }, []);
    // L'attache de la passerelle ne se dit qu'une fois (#195) : SessionStream la rejoue à chaque reprise.
    const gatewayStepSaidRef = useRef(false);

    useEffect(() => {
        if (!sawPicture || introLive) {
            return undefined;
        }
        const timer = setTimeout(() => {
            if (!introToldRef.current) {
                setIntroLive(true);
            }
        }, 2500);

        return () => clearTimeout(timer);
    }, [sawPicture, introLive]);

    useEffect(() => {
        if (introPhase !== "hold") {
            setHeldLong(false);

            return undefined;
        }
        const timer = setTimeout(() => setHeldLong(true), INTRO_HELD_SAY_MS);

        return () => clearTimeout(timer);
    }, [introPhase]);

    // CE QUE L'INTRO TENUE DIT (#520, #521) : un démarrage en minutes, le jeu qui se charge, ou
    // l'image du jeu qu'on attend.
    const introStep = introPhase !== "hold" ? null : slowStart ? "game.loadingSlow" : gameLoading ? "game.loading" : heldLong ? "game.waitingPicture" : null;

    // Tout ce qui ENVOIE suit cette réponse-là ; avant que le child ait parlé, jouer, c'est piloter.
    const drivesPad = childControls == null ? !isWatching : childControls > 0;
    const input = useStreamingInput({
        screenRef,
        session,
        context,
        t,
        options: {
            isWatching,
            drivesPad,
            childSlot,
            onProgress: noteChildStep,
            // « Personne ne joue encore » est un état NORMAL pour qui regarde : le flux de la plateforme
            // échoue tant que personne n'a pris la manette.
            onConnectionError: (reason) => {
                if (!isWatching) {
                    setTerminateReason(reason);
                }
            },
        },
    });

    // L'ÉCRAN TACTILE (la DS…) : une donnée du registre des consoles (`touch`).
    const consoles = useConsoles();
    const hasTouchScreen = consoles.some((entry) => entry.id === session.consoleId && entry.touch);
    useTouchScreen({ screenRef, enabled: hasTouchScreen && drivesPad, send: input.extraProps?.touch?.send });

    // Le canal de la session mène la connexion de la plateforme ; ses rappels lisent `input` par une
    // référence (il est refait à chaque rendu, le canal ne doit jamais se reconnecter pour ça).
    const inputRef = useRef(null);
    inputRef.current = input;
    const [facadeOpen, setFacadeOpen] = useState(isDeviceSession);
    const [viewerReady, setViewerReady] = useState(false);
    const platformConnectedRef = useRef(false);
    // Qui était là au dernier état — pour n'annoncer que ce qui a changé.
    const presentsRef = useRef(null);

    const handleChildState = useCallback((message) => {
        const watching = !message.you?.isPlaying;
        setWatching(watching);
        setChildSlot(message.you?.slotId ?? null);
        setChildControls(message.you?.controls ?? 0);
        setChildLoan({ canReclaim: message.you?.canReclaim === true, canReturn: message.you?.canReturn === true, lentTo: message.lentTo || "", otherTabs: message.you?.otherTabs || 0 });
        const players = (message.players ?? []).map((player) => ({ username: player.username, role: player.isOwner ? "owner" : "player", slot: player.slotId, lentTo: player.lentTo || "", joinedAt: player.joinedAt || null }));
        const maxPlayers = message.maxPlayers || 0;
        setBroadcastPlayers(players);
        setBroadcastWatchers(message.watchers ?? []);
        setRoster({ maxPlayers, owner: message.owner || "", idleSince: message.idleSince || null });
        setVoiceState({ room: message.voice || null, you: message.you?.voice || null });
        const pending = message.pendingHandover;
        setSlotRequests(pending && pending.to === user?.username ? [{ from: pending.from, expiresAt: Date.parse(pending.expiresAt) || Date.now() + 30000 }] : []);

        // Les joueurs partis (pas le propriétaire : sa place l'attend, sa ligne le dit d'elle-même), et ceux revenus.
        const held = new Set(players.map((player) => player.username));
        const gone = playersRef.current.filter((player) => !held.has(player.username) && player.username !== message.owner);
        playersRef.current = players;
        setDeparted((current) => {
            const kept = current.filter((entry) => !held.has(entry.username) && !gone.some((player) => player.username === entry.username));
            return [...kept, ...gone.map((player) => ({ username: player.username, slot: player.slot }))];
        });
        for (const player of gone) {
            clearTimeout(departedTimers.current.get(player.username));
            departedTimers.current.set(player.username, setTimeout(() => {
                departedTimers.current.delete(player.username);
                setDeparted((current) => current.filter((entry) => entry.username !== player.username));
            }, 30000));
        }
        setKernelPrivateReady(true);
        setChildStateSeen(true);

        // LA FIN arrive d'ici, de l'autorité elle-même, socket encore ouvert.
        if (message.terminated) {
            setGameExited(Boolean(message.gameExited));
            setSessionEnded(true);
        }
        // Les allées et venues, une fois chacune ; le premier état ne dit rien. SEULEMENT CELLES DES JOUEURS (le porteur,
        // 2026-11-26 : « quand une personne rejoint une partie comme watcher il ne faut pas mettre de notification ou même
        // le son. Si c'est un watcher ça reste silencieux ») : un spectateur qui arrive ou part ne dit rien ; il ne compte
        // qu'en prenant une manette.
        const presents = new Set((message.players ?? []).map((player) => player.username).filter(Boolean));
        const before = presentsRef.current;
        presentsRef.current = presents;

        if (before) {
            for (const username of presents) {
                if (!before.has(username) && username !== user?.username) {
                    const slot = players.find((player) => player.username === username)?.slot;
                    notify({ title: t("play.notifications.joined"), subtitle: (nameOf) => (slot ? [nameOf(username), fact(t("facts.place"), t("play.players.place", { slot, max: maxPlayers }))] : nameOf(username)) });
                }
            }
            for (const username of before) {
                if (!presents.has(username) && username !== user?.username) {
                    notify({ title: t("play.notifications.left"), subtitle: (nameOf) => nameOf(username) });
                }
            }
        }

        onWatchingChange?.(watching);
    }, [onWatchingChange, user?.username, notify, t, nameOf]);

    // « TOUJOURS LÀ ? » — la question du child à qui n'a rien touché depuis vingt-neuf minutes.
    // Toucher n'importe quoi y répond (le battement suivant porte `hadInput`) ; le bouton est pour qui
    // a les mains ailleurs.
    const handleChildAfk = useCallback((message) => {
        const seconds = Math.max(Math.round((message.graceMs || 60000) / 1000), 1);
        context.toast.open(t("play.afk", { seconds }), {
            variant: "confirm",
            confirmLabel: t("play.afkHere"),
            onConfirm: () => {
                markActivity();
                beatRef.current?.(true);
            },
        });
    }, [context, t]);

    // LA COMPILATION DES SHADERS (chantier 50) : la rafale en cours, poussée par la partie ; la roue en haut à gauche.
    const [shaders, setShaders] = useState({ compiling: false, count: 0 });
    const handleChildShaders = useCallback((message) => {
        setShaders({ compiling: Boolean(message.compiling), count: Number(message.count) || 0 });
    }, []);

    // LA RÉPONSE À MA DEMANDE, une notification : « Demande pour récupérer la manette 3/4 — François a refusé ».
    const settleOutgoing = useCallback((subtitle) => {
        const outgoing = outgoingRef.current;
        clearTimeout(outgoing?.timer);
        outgoingRef.current = null;
        notify({ title: outgoing ? t("play.notifications.asked", { slot: outgoing.slot, max: outgoing.max }) : t("play.notifications.askedAny"), subtitle });
    }, [notify, t]);

    const handleChildResult = useCallback((message) => {
        // LA VOIX a son propre résultat (use-voice-chat.js) : jamais un refus de manette.
        if (message.action?.startsWith("voice-")) {
            voiceResultRef.current?.(message);

            return;
        }
        if (message.status === "received") {
            if (outgoingRef.current?.to === message.from) {
                settleOutgoing((nameOf) => t("play.notifications.accepted", { name: nameOf(message.from) }));
            } else {
                notify({ title: t("play.notifications.received"), subtitle: (nameOf) => t(message.lent ? "play.handover.lentToYou" : "play.handover.receivedFrom", { name: nameOf(message.from) }) });
            }

            return;
        }
        if (message.status === "granted") {
            return;
        }
        // MA PLACE EST SUR UN AUTRE APPAREIL (chantier 65) : le child refuse un second `join` du même compte. La page
        // demande alors à l'autre appareil de la libérer — c'est lui qui répond.
        if (message.action === "join" && message.reason === "already in the players queue") {
            if (childSocketRef.current?.intent("request-handover", user?.username)) {
                context.toast.open(t("play.handover.moveAsk"), { variant: "warning", duration: 6000 });
            }

            return;
        }
        // Un refus du détenteur : le mien sur l'autre appareil, ou celui de la personne à qui je l'ai demandée.
        if (message.status === "denied") {
            if (message.move) {
                notify({ title: t("play.notifications.moveTitle"), subtitle: t("play.handover.moveDenied") });
            } else {
                const to = outgoingRef.current?.to;
                settleOutgoing(to ? (nameOf) => t("play.notifications.refused", { name: nameOf(to) }) : t("play.handover.denied"));
            }

            return;
        }
        // « abandoned » n'est pas un refus : le monde a bougé. Un refus dit pourquoi quand le child l'a nommé.
        const reasons = { "already playing in another session": "play.handover.playsElsewhere", "no slot available": "play.handover.noSlot" };
        settleOutgoing(t(message.status === "abandoned" ? "play.handover.claimFailed" : reasons[message.reason] || "play.handover.denied"));
    }, [context, t, nameOf, user?.username, notify, settleOutgoing]);

    const childSocket = useChildSessionWs({
        sessionId: childSessionId,
        token,
        enabled: !watchOnly,
        onState: handleChildState,
        onResult: handleChildResult,
        onAfk: handleChildAfk,
        onShaders: handleChildShaders,
        onConnect: () => {
            setFacadeOpen(true);
            clearTimeout(authorityGoneRef.current);
        },
        // Le child SORT quand sa session est finie : un socket qui RESTE mort est la fin. Le délai
        // sépare la coupure réseau de l'arrêt.
        onDisconnect: () => {
            setFacadeOpen(false);
            platformConnectedRef.current = false;
            clearTimeout(authorityGoneRef.current);
            authorityGoneRef.current = setTimeout(() => setSessionEnded(true), 12000);
        },
    });

    // UN SEUL ENVOI pour le menu et les questions : le vocabulaire du menu, traduit en intentions du
    // child — lui seul décide qui joue.
    const sessionSend = useCallback((message) => {
        const actions = { cedeslot: "grant-handover", slotrequestdenied: "deny-handover", reclaimslot: "reclaim", returnslot: "return", releaseslot: "leave" };
        const action = message.type === "requestslot" ? (message.to ? "request-handover" : "join") : actions[message.type];

        if (action) {
            childSocket.intent(action, message.to ?? null);
        }
        // MA DEMANDE À UN JOUEUR : sans réponse avant que le child ne l'oublie (HandoverTTL, 30 s), « n'a pas donné suite ».
        if (action === "request-handover" && message.to && message.to !== user?.username) {
            clearTimeout(outgoingRef.current?.timer);
            const slot = playersRef.current.find((player) => player.username === message.to)?.slot ?? "?";
            const to = message.to;
            outgoingRef.current = {
                to, slot, max: roster.maxPlayers || "?",
                timer: setTimeout(() => settleOutgoing((nameOf) => t("play.notifications.noAnswer", { name: nameOf(to) })), 31000),
            };
        }
    }, [childSocket, user?.username, roster.maxPlayers, settleOutgoing, t]);

    // LA VOIX DE LA PARTIE (chantier 71) : un compte qui joue ou regarde, jamais le spectateur sans compte (la file ne le
    // connaît pas). Un geste refusé se dit en toast.
    const voiceIntent = useCallback((action, target) => childSocket.intent(action, target), [childSocket]);
    const voiceRefused = useCallback((reason) => {
        const reasons = {
            denied: "play.voice.refused.denied",
            missing: "play.voice.refused.missing",
            busy: "play.voice.refused.busy",
            "too many open microphones": "play.voice.refused.full",
            "nobody else is in the conversation": "play.voice.refused.alone",
            "the conversation is not open to spectators": "play.voice.refused.closed",
        };
        context.toast.open(t(reasons[reason] || "play.voice.refused.other"), { variant: "warning", duration: 6000 });
    }, [context, t]);
    const voice = useVoiceChat({ enabled: !watchOnly && Boolean(viewerUsername), state: voiceState, intent: voiceIntent, screenRef, onRefused: voiceRefused });
    voiceResultRef.current = voice.onResult;

    // « JE SUIS VENU JOUER » (?play=1) — une INTENTION, jamais un droit : un join ordinaire, que le
    // child valide comme n'importe quel autre. Elle attend la réponse du CHILD lui-même
    // (childStateSeen), pas celle du kernel, qui arrive avant lui sur une session neuve.
    const playIntentRef = useRef(false);
    useEffect(() => {
        if (!(wantsPlay || ownsDeviceSession) || playIntentRef.current || !childStateSeen) {
            return;
        }
        if (childSlot != null) {
            playIntentRef.current = true;
            onPlayIntentSent?.();

            return;
        }
        if (!childSocket.intent("join")) {
            return;
        }
        playIntentRef.current = true;
        onPlayIntentSent?.();
    }, [wantsPlay, ownsDeviceSession, childStateSeen, childSlot, childSocket, onPlayIntentSent]);

    // LA PLATEFORME SE BRANCHE une fois le canal ouvert ET le viewer de la page créé — les hooks
    // mettent son identifiant sur leur fil ; l'attente est bornée.
    useEffect(() => {
        if (watchOnly || !facadeOpen || platformConnectedRef.current) {
            return undefined;
        }
        const startPlatform = () => {
            if (!platformConnectedRef.current) {
                platformConnectedRef.current = true;
                inputRef.current?.onConnect?.();
            }
        };

        if (viewerReady) {
            startPlatform();

            return undefined;
        }
        const deadline = setTimeout(startPlatform, PLATFORM_CONNECT_WITHOUT_VIEWER_MS);

        return () => clearTimeout(deadline);
    }, [facadeOpen, viewerReady]);

    // LE BATTEMENT (chantier 4) : toutes les 30 s, la preuve que l'onglet vit ET qu'un humain y touche.
    // Par une référence : un intervalle refait à chaque rendu n'atteint jamais sa période.
    const beatRef = useRef(null);
    beatRef.current = childSocket.beat;
    useEffect(() => {
        const interval = setInterval(() => beatRef.current?.(takeActivityStamp() > 0), CHILD_BEAT_INTERVAL_MS);

        return () => clearInterval(interval);
    }, []);

    // QUI JE SUIS DANS CETTE PARTIE — ce que le child a poussé ; sinon la session dit à qui elle est.
    const myRole = useMemo(() => {
        const mine = broadcastPlayers.find((player) => player.username === user?.username);

        if (mine?.role) {
            return mine.role;
        }
        if (session.owner && session.owner === user?.username) {
            return "owner";
        }

        return session.me?.role || "spectate";
    }, [broadcastPlayers, user?.username, session.me?.role, session.owner]);

    const quitStoreRef = useRef(null);
    // Quitter arrête la partie par défaut (le porteur : « par défaut quitter et tuer la session car j'ai remarqué que beaucoup
    // de gens ne tue pas la session après avoir quitté ») ; un autre choix est retenu le temps de la page.
    quitStoreRef.current ??= createSwitchesStore({ onQuit: "kill", saveStateOnQuit: false });
    const quitStore = quitStoreRef.current;

    // LES INTERRUPTEURS DE LA PARTIE (createSwitchesStore de la webapp), lus par le menu.
    const switchesStoreRef = useRef(null);
    switchesStoreRef.current ??= createSwitchesStore({
        isSoundEnabled: true,
        // LE PLEIN ÉCRAN SUIT CE QUI EST DÉJÀ (le porteur, 2026-11-16 : « Si on est déjà en Fullscreen alors on le reste. Si
        // on ne l'est pas alors ça ne le viens pas automatiquement ») : l'état du navigateur à l'ouverture de la partie. Un
        // `false` posé d'office faisait sortir l'effet du plein écran ci-dessous (exitFullscreen) dès le montage, et une
        // partie lancée depuis un launcher en plein écran (Réglages › Application) le quittait.
        isFullscreen: isFullscreen(),
        isChromeCasting: castSender(can("castLink")).casting(),
        // L'IDENTIFIANT DU RÉCEPTEUR GOOGLE CAST (Réglages › Google Cast) : vide, le menu ne propose pas le cast.
        castApplicationId: "",
        isResourceMonitoringVisible: false,
        isCompactStatsVisible: false,
        // LES FLUX DE LA PARTIE (chantier 58) : le choix de cet écran (« auto » ou un indice), puis ce que dit la
        // passerelle — les flux (0 celui de la partie, puis ses paliers), celui reçu, celui demandé.
        videoQuality: "auto",
        videoStreams: [],
        videoCurrent: 0,
        videoWant: 0,
        // Les retours tactiles, les vibrations et le stick flottant ne sont pas ici : des réglages de l'APPAREIL, que le
        // menu lit et écrit dans leur seule source (engine/libs/gamepadPreferences.js, #758). Les recopier ici, lus au
        // montage puis réécrits, annulait un choix fait dans Réglages › Manettes depuis.
        // La manette à l'écran se cache par défaut quand une vraie est branchée ; le menu la rend.
        isControllerVisible: drivesPad && !hasPhysicalGamepad,
        isRewinding: false,
        isFastForwarding: false,
        // La PS1 démarre avec sa manette d'origine (décision du 2026-10-11).
        isDualShockController: false,
    });
    const switchesStore = switchesStoreRef.current;
    const switches = useSyncExternalStore(switchesStore.subscribe, switchesStore.getSnapshot);
    soundRef.current = Boolean(switches.isSoundEnabled);
    useAdaptiveQuality({ screenRef, mode: switches.videoQuality, quality: { streams: switches.videoStreams, current: switches.videoCurrent, want: switches.videoWant }, paused: !introLive });

    usePictureInPictureOnHide(screenRef);

    // En se démontant, la page se met en retrait (jamais une déconnexion : un remontage n'est pas un départ).
    // eslint-disable-next-line react-hooks/exhaustive-deps
    useEffect(() => () => input.leavePage?.(), []);

    // QUI JOUE est prévenu avant de quitter ou de rafraîchir ; qui regarde, non.
    useEffect(() => {
        if (!drivesPad && isWatching) {
            return undefined;
        }
        const confirmLeaving = (event) => {
            event.preventDefault();
            event.returnValue = "";

            return "";
        };
        window.addEventListener("beforeunload", confirmLeaving);

        return () => window.removeEventListener("beforeunload", confirmLeaving);
    }, [isWatching, drivesPad]);

    // QUITTER LA PAGE, C'EST QUITTER LA SESSION — lié à la vie de la ROUTE (Play.jsx), pas de cette
    // coquille, qui se démonte dès que le pool perd la session un instant.
    const quitHandledRef = useRef(false);
    useEffect(() => {
        leaveRef.current = () => {
            if (!quitHandledRef.current) {
                quitHandledRef.current = true;
                inputRef.current?.disconnect?.();
            }
        };

        return () => {
            leaveRef.current = null;
        };
    }, [leaveRef]);

    // La présence, avec le vrai rôle dès le premier envoi.
    useEffect(() => {
        if (!kernelPrivateReady) {
            return undefined;
        }
        reportPlaying(session, isWatching);

        return () => reportIdle();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [kernelPrivateReady]);
    // Le socket, pour un résultat qui doit répondre par une autre intention (handleChildResult, chantier 65).
    const childSocketRef = useRef(null);
    childSocketRef.current = childSocket;

    // Un spectateur qui devient joueur retrouve la manette à l'écran — sauf s'il tient une vraie manette.
    useEffect(() => {
        if (drivesPad) {
            switchesStore.setValues({ isControllerVisible: !hasPhysicalGamepad });
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [drivesPad, hasPhysicalGamepad, switchesStore]);

    // LA DEMANDE DE MANETTE adressée à celui qui la tient : le propriétaire PRÊTE (#274), un invité DONNE. Plus de fenêtre
    // par-dessus le jeu (chantier 68) : une notification, la cloche le temps qu'elle vaut, la réponse depuis le menu.
    // MON AUTRE APPAREIL DEMANDE MA PLACE (chantier 65) : la libérer, ou la garder ; sans réponse en 15 s, gardée. Une
    // demande d'un autre joueur s'éteint avec celle du child (HandoverTTL) : son silence n'est pas un refus envoyé.
    const incomingRef = useRef(null);
    useEffect(() => {
        const request = slotRequests[0];
        const open = incomingRef.current;

        // Retirée (le demandeur est parti, la place a changé de main) : la notification ne se répond plus.
        if (open && (!request || request.from !== open.from)) {
            notifications.settle(open.id, { state: "expired" });
            incomingRef.current = null;
        }
        if (!request || isWatching || incomingRef.current) {
            return;
        }
        const moves = request.from === user?.username;
        const lending = myRole === "owner";
        const id = notify({
            asks: true,
            title: t(moves ? "play.notifications.moveAsked" : lending ? "play.notifications.lendAsked" : "play.notifications.giveAsked"),
            subtitle: moves ? t("play.notifications.otherDevice") : (nameOf) => nameOf(request.from),
            expiresAt: moves ? Date.now() + 15000 : request.expiresAt,
            confirm: {
                title: t(moves ? "play.notifications.moveAsked" : lending ? "play.handover.lendTitle" : "play.handover.giveTitle"),
                message: t(moves ? "play.handover.movePrompt" : lending ? "play.handover.lendPrompt" : "play.handover.givePrompt", { name: nameOf(request.from) }),
                confirmLabel: t(moves ? "play.handover.release" : lending ? "play.handover.lend" : "play.handover.give"),
                cancelLabel: t("play.handover.keep"),
            },
            onConfirm: () => {
                sessionSend({ type: "cedeslot" });
                inputRef.current?.onSlotGiveaway?.();
            },
            onCancel: () => sessionSend({ type: "slotrequestdenied" }),
            onExpire: moves ? () => sessionSend({ type: "slotrequestdenied" }) : null,
        });
        incomingRef.current = { id, from: request.from };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [slotRequests, isWatching]);

    // LE PLEIN ÉCRAN DE LA SESSION — l'image et ce qui est posé dessus, pas le document ni la <video>.
    useEffect(() => {
        if (!shellRef.current) {
            return;
        }
        if (switches.isFullscreen) {
            if (!isFullscreen()) {
                requestFullscreen(shellRef.current).catch(() => {});
            }
        } else if (isFullscreen()) {
            exitFullscreen().catch(() => {});
        }
    }, [switches.isFullscreen]);

    // L'ORIENTATION EST CELLE DES RÉGLAGES (Réglages › Application, #406 ; le menu de la partie la règle aussi, #758) : la
    // page de jeu ne tourne pas l'écran en s'ouvrant (chantier 53, P12) — seule l'image tourne, en repli d'un paysage
    // choisi que l'écran n'a pas pris (use-screen-rotation.js). Suivie : choisie dans le menu, elle vaut tout de suite.
    const screenOrientation = useScreenOrientation();
    const rotateSurface = rotationFallback(screenOrientation, { enabled: isHandheld });
    const isPortraitLayout = isPortraitViewport && !rotateSurface;

    // Le navigateur sort du plein écran sans nous (Échap, une navigation) : il fait autorité.
    useEffect(() => {
        const sync = () => switchesStore.setValues({ isFullscreen: isFullscreen() });
        const events = ["fullscreenchange", "webkitfullscreenchange"];
        events.forEach((name) => document.addEventListener(name, sync));

        return () => events.forEach((name) => document.removeEventListener(name, sync));
    }, [switchesStore]);

    // L'identifiant du récepteur, lu une fois à l'ouverture de la partie, là où l'hôte sait caster (`castToTv`).
    useEffect(() => {
        if (watchOnly || !can("castToTv")) {
            return undefined;
        }
        let cancelled = false;
        fetchCastSettings().then((answer) => {
            if (!cancelled && answer.ok) {
                switchesStore.setValues({ castApplicationId: answer.item?.application_id || "" });
            }
        });

        return () => {
            cancelled = true;
        };
    }, [switchesStore, watchOnly]);

    // LE CAST : l'envoyeur que l'hôte déclare (`castLink` : celui de l'application Android, sinon le SDK de Chrome ;
    // cast.js). Arrêté hors de la page (la notification du téléphone), l'interrupteur le suit.
    useEffect(() => {
        if (watchOnly || !can("castToTv")) {
            return undefined;
        }

        return castSender(can("castLink")).watch((casting) => switchesStore.setValues({ isChromeCasting: casting }));
    }, [switchesStore, watchOnly]);

    useEffect(() => {
        const sender = castSender(can("castLink"));

        if (switches.isChromeCasting) {
            sender.start(switches.castApplicationId, session.id).catch((error) => {
                // Le sélecteur de TV refermé sans choix n'est pas une erreur.
                if (!error.cancelled) {
                    context.toast.open(t(castErrorKey(error)), { variant: "error" });
                }
                switchesStore.setValues({ isChromeCasting: false });
            });
        } else if (sender.casting()) {
            sender.stop();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [switches.isChromeCasting]);

    // PS1 : la manette d'origine ou la DualShock.
    useEffect(() => {
        if (input.extraProps?.PS1) {
            (switches.isDualShockController ? input.extraProps.PS1.dualshock : input.extraProps.PS1.standard).load();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [switches.isDualShockController]);
    // Rétro : le voyage dans le temps.
    useEffect(() => {
        input.extraProps?.timeTravelProps?.setTravelMode(switches.isRewinding ? "rewind" : switches.isFastForwarding ? "fastForward" : null);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [switches.isRewinding, switches.isFastForwarding]);

    // Le relevé des performances, À CHAQUE RENDU (il contient un hook) ; ouvert par le panneau ou la vue discrète.
    const statsResult = input.onMonitoringResource(switches.isResourceMonitoringVisible || switches.isCompactStatsVisible, isHandheld);

    // LA FIN, montrée un instant, puis la sortie — celle de « Quitter ».
    useEffect(() => {
        if (!sessionEnded) {
            return undefined;
        }
        const timer = setTimeout(() => onTerminate?.(), gameExited ? GAME_EXITED_REDIRECT_MS : SESSION_ENDED_REDIRECT_MS);

        return () => clearTimeout(timer);
    }, [sessionEnded, gameExited, onTerminate]);
    useEffect(() => {
        if (!videoSilent) {
            return undefined;
        }
        const timer = setTimeout(() => onTerminate?.(), SOURCE_GONE_REDIRECT_MS);

        return () => clearTimeout(timer);
    }, [videoSilent, onTerminate]);

    const phase = sessionPhase({
        ended: sessionEnded,
        gameExited,
        // Sans jeton, aucune autorité à attendre : il n'y a rien à lui demander.
        authorityOpen: facadeOpen || watchOnly,
        authorityAnswered: kernelPrivateReady || watchOnly,
        claiming: wantsPlay && isWatching && !watchOnly,
        platformConnected: input.isConnected,
        videoSilent,
        stalling: input.isStalling,
    });
    const controller = kernelPrivateReady ? input.controller() : null;

    // ANNULER DEPUIS L'ÉCRAN DU LANCEMENT (#287) : la partie qu'on vient de lancer s'arrête pour tout le
    // monde ; celle d'un autre continue sans nous.
    const cancelLaunch = () => {
        if (myRole === "owner") {
            input.terminate?.();
            stopForEveryone(session, token).catch(() => context.toast.open(t("play.quit.stopFailed"), { variant: "error" }));
        } else {
            input.disconnect?.();
        }
        onTerminate();
    };

    // QUITTER — trois verbes, sans drapeau : partir (la partie continue), geler (un cœur seulement),
    // arrêter pour tout le monde (le propriétaire seulement). Sauver est une décision à part.
    const quit = async ({ onQuit, saveStateOnQuit: save }) => {
        const ownerHere = !isWatching && myRole === "owner";
        // METTRE LA CONSOLE EN VEILLE ET QUITTER (Remote Play, chantier 69 ; le porteur, 2026-11-19 : « c'est meilleur de
        // voir la console s'éteindre avant de quitter la session […] tant qu'on a de l'image la session ne se coupe pas ») :
        // la console s'endort sous nos yeux, la page reste ; la console éteinte, la partie se termine d'elle-même et la
        // page revient en arrière.
        if (onQuit === "standby" && ownerHere && session.platform === "remoteplay") {
            const answer = await standbyConsole(session.id, token).catch(() => null);
            const failed = !answer || answer.ok === false;
            closeMenu();
            context.toast.open(t(failed ? "play.quit.standbyFailed" : "play.quit.standbySent"), { variant: failed ? "error" : undefined, duration: 6000 });

            return;
        }
        const ends = onQuit === "kill" && ownerHere;
        const freezes = onQuit === "pause" && ownerHere && (session.platform === "retrogaming" || session.platform === "device");

        if (save && !isWatching && input.saveOnQuit) {
            await input.saveOnQuit();
        }
        quitHandledRef.current = true;

        if (freezes && input.extraProps?.pause) {
            freezingRef.current = true;
            await input.extraProps.pause();
        }
        if (ends) {
            input.terminate?.();
            stopForEveryone(session, token).catch(() => context.toast.open(t("play.quit.stopFailed"), { variant: "error" }));
        } else {
            input.disconnect?.();
        }
        onTerminate();
    };

    // ── LE MENU ET LA SORTIE (#408, #409 ; décision 3 du porteur) ──────────────────────────────────
    // `panels` : les panneaux ouverts, du menu à ses sous-panneaux — un tableau, la croix sert celui du dessus.
    // Refermer un sous-panneau rend la lumière à l'entrée qui l'avait ouvert (`returns`, en pile).
    const [panels, setPanels] = useState([]);
    const returnsRef = useRef([]);
    const openPanel = useCallback((panel) => {
        returnsRef.current.push(document.activeElement?.dataset?.key || null);
        setPanels((open) => [...open, panel]);
    }, []);
    const closePanel = useCallback(() => {
        const key = returnsRef.current.pop();
        setPanels((open) => open.slice(0, -1));
        requestAnimationFrame(() => {
            const back = key && document.querySelector(`.npanel [data-key="${CSS.escape(key)}"]`);

            if (back) {
                focusElement(back, true);
            }
        });
    }, []);
    const closeMenu = useCallback(() => {
        returnsRef.current = [];
        setPanels([]);
    }, []);
    const openMenu = useCallback(() => setPanels((open) => (open.length ? open : ["menu"])), []);
    // LA RÉTRO SE MET EN PAUSE LE MENU OUVERT (le porteur, 2026-11-26 : « Un appui et ça met la console en pause et ouvre le
    // menu ») — la partie du propriétaire qui joue ; elle reprend le menu refermé, et en partant si le jeu continue. Partir
    // « en pause » la laisse figée (`freezingRef`, quit).
    const freezingRef = useRef(false);
    const menuOpen = panels.length > 0;
    const pausesWithMenu = (session.platform === "retrogaming" || session.platform === "device") && !isWatching && myRole === "owner";
    const extraRef = useRef(input.extraProps);
    extraRef.current = input.extraProps;
    useEffect(() => {
        if (!menuOpen || !pausesWithMenu || !extraRef.current?.pause) {
            return undefined;
        }
        extraRef.current.pause();

        return () => {
            if (!freezingRef.current) {
                extraRef.current?.resume?.();
            }
        };
    }, [menuOpen, pausesWithMenu]);
    // QUITTER : la fenêtre de confirmation de Skeleton, le choix dans son corps (Menu.jsx, useQuitFlow) ; Quitter quitte.
    // D'autres jouent : un autre compte tient une manette, ou la mienne est prêtée.
    const me = session.me?.username || user?.username;
    const othersPlay = broadcastPlayers.some((player) => player.username !== me || (player.lentTo && player.lentTo !== me));
    const openQuit = useQuitFlow({ session, isWatching, isOwner: myRole === "owner", borrowing: childLoan.canReturn, othersPlay, store: quitStore, onConfirm: quit });

    // UN PANNEAU OUVERT SUSPEND LE JEU, UNE PARTIE EN COURS SUSPEND LA NAVIGATION (#407, #408) : tant
    // qu'il y en a un, la croix, A et B le servent et le jeu n'entend rien (chaque plateforme relâche ce
    // qu'elle tenait, libs/gameInput.js) ; sans panneau et la manette en main, la navigation du launcher
    // se tait et plus rien n'a le focus. Une question qui demande la manette (« X demande votre
    // manette ») compte comme un panneau, et une fenêtre de Skeleton (la sortie) aussi.
    const overlays = panels.length + Skeleton.useOverlayCount();
    const playing = drivesPad && overlays === 0;
    useEffect(() => {
        suspendNavigation(playing);

        if (playing) {
            document.activeElement?.blur?.();
        }
    }, [playing]);
    // LE JEU NE REPREND LA MANETTE QU'UNE FOIS TOUT LÂCHÉ : le B qui referme le menu est encore enfoncé
    // quand il se ferme, et partait au jeu comme un appui (vu en partie le 2026-11-13 : le A de la SNES).
    useEffect(() => {
        if (overlays > 0) {
            setGameInputSuspended(true);

            return undefined;
        }
        // Le A qui a lancé la partie aussi, s'il est encore tenu à l'arrivée.
        let raf = 0;
        const held = () => connectedGamepads().some((pad) => pad?.buttons?.some((button) => button.pressed));
        const resume = () => {
            if (held()) {
                raf = requestAnimationFrame(resume);

                return;
            }
            setGameInputSuspended(false);
        };

        if (held()) {
            setGameInputSuspended(true);
        }
        resume();

        return () => cancelAnimationFrame(raf);
    }, [overlays]);
    useEffect(() => () => {
        setGameInputSuspended(false);
        suspendNavigation(false);
    }, []);

    // L'écran de la page : B referme le panneau du dessus (SidePanel le sert) ; les gestes de la barre
    // du launcher n'ont rien à faire ici.
    useScreenInput({ back: () => {}, switchTab: () => {}, select: () => {}, start: () => {} });

    const menuHold = useMenuHotkey(overlays === 0, openMenu, session.platform);
    const panelsRef = useRef(panels);
    panelsRef.current = panels;

    // ÉCHAP OUVRE LE MENU au clavier — pris avant tout le monde et gardé : il ne part plus au jeu. Avant tout
    // le monde pour de bon : posé dès le rendu (useLayoutEffect, avant les effets des entrées du moteur et du
    // test des touches, qui écoutent aussi la fenêtre en capture), et arrêté net (stopImmediatePropagation).
    // Le menu ouvert, il le referme (la navigation du launcher : Retour).
    useLayoutEffect(() => {
        const onKeyDown = (event) => {
            if (event.key !== "Escape" || !event.isTrusted || panelsRef.current.length > 0) {
                return;
            }
            event.preventDefault();
            event.stopImmediatePropagation();
            openMenu();
        };
        window.addEventListener("keydown", onKeyDown, true);

        return () => window.removeEventListener("keydown", onKeyDown, true);
    }, [openMenu]);

    const menuButtonVisible = useIdleVisibility(playing);
    // LES TEXTES POSÉS SUR LA VIDÉO S'ESTOMPENT COMME LE BOUTON DU MENU (chantier 68 ; le porteur, 2026-11-19 : « les textes
    // comme "you are watching" doivent disparaître auto (fade out) comme le bouton du menu ») — qu'on joue ou qu'on
    // regarde : ils reviennent au même geste. Les boutons qui les suivent (reprendre la manette, la prendre) restent.
    const overlayTextVisible = useIdleVisibility(true);
    const overlayTextStyle = { opacity: overlayTextVisible ? 1 : 0, transition: "opacity 300ms ease" };
    const isPadVisible = Boolean(drivesPad && switches.isControllerVisible && controller?.Component);
    const menuButtonStyle = MENU_BUTTON_STYLE;
    const menuSlide = useSlideToOpen(openMenu);
    // Le voyage dans le temps à l'écran (chantier 69) : ce que le cœur sait faire (`timeTravelProps`), pour le
    // propriétaire qui joue ; les deux s'excluent, comme dans le menu d'avant.
    const ownerPlays = myRole === "owner" && !isWatching && (session.platform === "retrogaming" || session.platform === "device");
    const travel = input.extraProps?.timeTravelProps;
    const timeTravel = !ownerPlays ? [] : [
        can("rewind") && travel?.rewindSupported && {
            key: "pw-rewind", icon: "bi-skip-backward", label: t("play.timeTravel.rewind"), on: Boolean(switches.isRewinding),
            toggle: () => switchesStore.setValues({ isRewinding: !switches.isRewinding, ...(!switches.isRewinding ? { isFastForwarding: false } : {}) }),
        },
        travel?.fastForwardSupported && {
            key: "pw-forward", icon: "bi-fast-forward", label: t("play.timeTravel.fastForward"), on: Boolean(switches.isFastForwarding),
            toggle: () => switchesStore.setValues({ isFastForwarding: !switches.isFastForwarding, ...(!switches.isFastForwarding ? { isRewinding: false } : {}) }),
        },
    ].filter(Boolean);
    // LE MICRO À L'ÉCRAN, à côté (chantier 71 ; le porteur, 2026-11-23 : « comme les boutons rewind et fast forward, le
    // micro doit être sur l'écran aussi ») : la même bascule que « Mon micro » du menu, pour qui peut parler.
    const micButton = voice.you?.usable && voice.microphone ? {
        key: "pw-mic", icon: voice.you.mic ? "bi-mic-fill" : "bi-mic-mute", label: t("play.voice.mic"), on: Boolean(voice.you.mic),
        toggle: () => voice.toggleMic(!voice.you.mic),
    } : null;
    const leftButtons = [...timeTravel, micButton].filter(Boolean);

    // L'écran du lancement couvre la page jusqu'à l'image du JEU et la manette prête ; un spectateur
    // (pas de lancement) attend l'image seule — il n'aura jamais de manette.
    const launchShown = (launchId || isWatching) && !(sawPicture && introLive && (kernelPrivateReady || !launchId));

    // LE RACCOURCI SE DIT, une fois, à qui joue avec une manette — quand le jeu est à l'écran.
    const gamepadTipRef = useRef(false);
    useEffect(() => {
        if (hasPhysicalGamepad && drivesPad && !launchShown && !gamepadTipRef.current) {
            gamepadTipRef.current = true;
            context.toast.open(t(menuGestureOf(session.platform).holdMs > 0 ? "play.gamepadTipChord" : "play.gamepadTipHome"), { duration: 6000 });
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [hasPhysicalGamepad, drivesPad, launchShown]);

    return (
        <div ref={shellRef} className={`play ${switches.isFullscreen ? "fullscreen" : ""}`} style={{
            // La vue discrète sous la vidéo, en portrait : le pavé tactile seul descend d'autant (Controllers.jsx).
            "--gameflex-stats-strip-height": isPortraitLayout && switches.isCompactStatsVisible ? `${STATS_STRIP_HEIGHT}px` : "0px",
            ...rotationSurfaceStyle(rotateSurface, isPortraitViewport),
        }}>
                <SessionStream
                    sessionId={childSessionId}
                    t={t}
                    screenRef={screenRef}
                    fillHeight
                    // L'AUTHENTIFICATION décide du canal d'entrée, jamais le rôle : renégocier au changement
                    // de rôle redémarrait la vidéo. Lu du jeton, au premier rendu.
                    hasInput={Boolean(viewerUsername)}
                    username={viewerUsername}
                    // La voix : la page d'un compte la négocie ; « Entendre les autres » coupé, ses voix se taisent ici.
                    voice={!watchOnly && Boolean(viewerUsername)}
                    voicesMuted={!voice.hearing}
                    launch={launchId}
                    muted={!switches.isSoundEnabled}
                    onVideoSilent={setVideoSilent}
                    onQuality={(state) => {
                        switchesStore.setValues({ videoStreams: state.streams, videoCurrent: state.current, videoWant: state.want });

                        if (!introToldRef.current) {
                            setIntroLive(true);
                        }
                    }}
                    onIntro={(introAt, game, expect) => {
                        introToldRef.current = true;
                        setIntroPhase(introAt);
                        setIntroLive(introAt === "live");
                        setGameLoading(introAt === "hold" && game === "loading");
                        setSlowStart(introAt === "hold" && expect === "minutes");
                    }}
                    onViewerReady={() => {
                        setViewerReady(true);

                        if (!gatewayStepSaidRef.current) {
                            gatewayStepSaidRef.current = true;
                            noteChildStep("gateway.attached");
                        }
                        // Un onglet tient DEUX identités — le viewer de la passerelle et le socket du child :
                        // lui seul connaît les deux, il les apparie.
                        childSocket.announceViewer(screenRef.current?.viewerId);
                    }}
                    maxAttempts={null}
                    gatewayProps={{
                        ...input.gatewayProps,
                        onFirstVideoTrack: (...args) => {
                            setShellStream(screenRef.current?.videoElement?.srcObject || null);
                            input.gatewayProps?.onFirstVideoTrack?.(...args);
                        },
                        onFirstPicture: (...args) => {
                            setSawPicture(true);
                            input.gatewayProps?.onFirstPicture?.(...args);
                        },
                    }}
                    showOverlay={phase !== "live" && !launchShown}
                    connectingLabel={phase === "live" ? OVERLAY_HIDDEN : t(`play.phase.${phase}`)}
                >
                    {/* Sous la vidéo, en portrait : la vue discrète, puis la marque (le porteur, 2026-11-03 et
                        2026-11-10). Une surface tournée est un paysage : la vidéo y prend tout. */}
                    {isPortraitLayout && (
                        <>
                            {switches.isCompactStatsVisible && <StatsStrip stats={statsResult} session={session} layout="portrait" />}
                            <div className="text-center text-white text-opacity-50 small py-1 pe-none">{t("play.brand")}</div>
                        </>
                    )}
                </SessionStream>

                {/* LES PERFORMANCES : le panneau en haut, sur un voile ; en paysage, la vue discrète en haut — dans la
                    colonne centrale quand la manette à l'écran est là (le haut de L2/R2 sortirait de l'écran). */}
                {switches.isResourceMonitoringVisible && statsResult && <StatsPanel stats={statsResult} session={session} />}
                {!isPortraitLayout && switches.isCompactStatsVisible && statsResult && (
                    <div className="position-absolute top-0 start-0 w-100 pe-none" style={{
                        zIndex: 1001,
                        paddingTop: "max(2px, env(safe-area-inset-top))",
                        paddingLeft: isPadVisible ? `${PAD_COLUMN_WIDTH}px` : "max(8px, env(safe-area-inset-left))",
                        paddingRight: isPadVisible ? `${PAD_COLUMN_WIDTH}px` : "max(8px, env(safe-area-inset-right))",
                    }}>
                        <StatsStrip stats={statsResult} session={session} layout="landscape" />
                    </div>
                )}

                {/* LA ROUE DE COMPILATION DES SHADERS (chantier 50) : en haut à gauche, sous la vue discrète en paysage, à côté
                    de la colonne de la manette à l'écran quand elle est là. */}
                {shaders.compiling && shaders.count > 0 && (
                    <ShaderSpinner count={shaders.count} style={{
                        zIndex: 1001,
                        top: `calc(max(8px, env(safe-area-inset-top)) + ${!isPortraitLayout && switches.isCompactStatsVisible && statsResult ? 28 : 0}px)`,
                        left: !isPortraitLayout && isPadVisible ? `${PAD_COLUMN_WIDTH}px` : "max(8px, env(safe-area-inset-left))",
                    }} />
                )}

                {/* LA SOURIS, pour une plateforme qui a un bureau derrière (streamex) : une surface par-dessus
                    la vidéo, sous la manette. Pas pour un écran tactile : le doigt y est un clic absolu. */}
                {input.extraProps?.pointerHandlers && !hasTouchScreen && (
                    <div className="position-absolute top-0 start-0 w-100 h-100" style={{ zIndex: 1000, touchAction: "none" }} {...input.extraProps.pointerHandlers} />
                )}

                {kernelPrivateReady && (
                    // Collée au bas de l'écran ; marquée : un doigt sur la manette ne réveille pas le bouton du menu.
                    <div className="position-absolute bottom-0 start-0 w-100" data-streaming-strip="" style={{ zIndex: 1002 }}>
                        <PadLookContext.Provider value={PAD_LOOK}>
                            {isPadVisible && controller.Component}
                            {drivesPad && !switches.isControllerVisible && hasPhysicalGamepad && controller?.Component && (
                                <TouchpadOnlyContext.Provider value={isPortraitLayout ? "portrait" : "landscape"}>{controller.Component}</TouchpadOnlyContext.Provider>
                            )}
                        </PadLookContext.Provider>
                        {!isWatching && !drivesPad && childLoan.canReclaim && (
                            <div className="p-3 text-center small">
                                <p style={overlayTextStyle}>{t("play.handover.lentNotice", { name: nameOf(childLoan.lentTo) })}</p>
                                <button type="button" className={button("main")} data-nav data-key="pw-reclaim" onClick={() => sessionSend({ type: "reclaimslot" })}>{t("play.handover.reclaim")}</button>
                            </div>
                        )}
                        {isWatching && !drivesPad && !launchShown && (
                            <div className="p-3 text-center small">
                                {/* Personne ne tient de manette : l'émulateur n'a pas démarré, l'écran noir est une
                                    partie qui n'a pas commencé, pas une partie en panne. */}
                                {broadcastPlayers.length === 0 && session.owner === user?.username ? (
                                    <>
                                        <p style={overlayTextStyle}>{t("play.watchIdle")}</p>
                                        <button type="button" className={button("main")} data-nav data-key="pw-join" onClick={() => sessionSend({ type: "requestslot" })}>{t("play.join")}</button>
                                    </>
                                ) : <span style={overlayTextStyle}>{t("play.watching")}</span>}
                            </div>
                        )}
                    </div>
                )}

                {/* LE BOUTON DU MENU (#409) : rond, en bas à droite ; il s'estompe pendant qu'on joue et revient
                    au premier geste. C'est une poignée (useSlideToOpen) : glissé vers la gauche le long de son rail,
                    il ouvre le menu ; un appui simple ne fait rien. Le rail et la poignée tenue prennent le dessin de
                    la manette à l'écran (PAD_LOOK). Entrée au clavier ou la navigation à la manette le « cliquent »
                    (detail 0) et l'ouvrent ; une manette l'ouvre aussi par L2 + Home tenus trois secondes, un
                    clavier par Échap. */}
                {menuSlide.held && (
                    <div className="position-absolute pe-none" aria-hidden="true" style={{
                        ...PAD_LOOK.surface({ touched: false }),
                        bottom: menuButtonStyle.bottom,
                        right: menuButtonStyle.right,
                        zIndex: menuButtonStyle.zIndex,
                        width: `calc(3rem + ${MENU_SLIDE_DISTANCE}px)`,
                        height: "3rem",
                        borderRadius: "1.5rem",
                    }} />
                )}
                <button type="button" className="play-round position-absolute" data-nav data-key="pw-menu"
                    style={{
                        ...menuButtonStyle, width: undefined, height: undefined, background: undefined, border: undefined,
                        ...(menuSlide.held ? PAD_LOOK.face({ pressed: true }) : {}),
                        transform: `translateX(${-menuSlide.travel}px)`,
                        touchAction: "none",
                        // Jamais tout à fait effacé (le porteur, 2026-11-17 : « toujours visible », puis « mets l'opacité à
                        // 10% ») : au repos, il reste à 10 %, et on peut toujours le saisir pour le glisser.
                        opacity: menuButtonVisible ? 1 : MENU_IDLE_OPACITY,
                    }}
                    title={t("play.menu.slide")} aria-label={t("play.menu.open")} {...menuSlide.handlers}
                    onClick={(event) => {
                        if (event.detail === 0) {
                            openMenu();
                        }
                    }}>
                    <i className="bi bi-list"></i>
                </button>
                {/* RETOUR RAPIDE ET AVANCE RAPIDE, À L'ÉCRAN, EN BAS À GAUCHE (chantier 69 ; le porteur, 2026-11-19 : « il est
                    préférable de mettre les boutons "rewind" et "fast forward" sur l'écran, comme le menu […] mais à gauche.
                    […] des boutons enfoncés, et quand ils sont activés on doit voir ce qui se passe sur l'écran ») : deux
                    bascules rondes (aria-pressed), pleines tant qu'elles agissent — elles ne s'estompent pas alors —, sinon
                    elles suivent le bouton du menu. Les pouvoirs du propriétaire qui joue, quand le cœur les a. */}
                {leftButtons.length > 0 && (
                    <div className="position-absolute d-flex gap-2" style={{ bottom: menuButtonStyle.bottom, left: "max(12px, env(safe-area-inset-left))", zIndex: 1003 }}>
                        {leftButtons.map(({ key, icon, label, on, toggle }) => (
                            <button key={key} type="button" className="play-round" data-nav data-key={key} aria-pressed={on} aria-label={label} title={label}
                                style={{ opacity: on || menuButtonVisible ? 1 : MENU_IDLE_OPACITY }} onClick={toggle}>
                                <i className={`bi ${icon}`}></i>
                            </button>
                        ))}
                    </div>
                )}
                {/* LA CHALEUR ET LA BATTERIE DE L'APPAREIL QU'ON TIENT, à gauche du bouton du menu, dans l'application
                    seulement (un navigateur n'en a pas) ; elles s'effacent et reviennent avec lui. */}
                {isHandheld && !menuSlide.held && (statsResult?.device?.temperatureCelsius != null || statsResult?.device?.batteryPercent != null) && (
                    <div className="position-absolute d-flex gap-2 pe-none" style={{ bottom: menuButtonStyle.bottom, right: `calc(${menuButtonStyle.right} + 3.5rem)`, zIndex: 1003, opacity: menuButtonVisible ? 1 : 0, transition: "opacity 300ms ease" }}>
                        {[
                            { value: statsResult.device.temperatureCelsius, unit: "°", icon: "bi-thermometer-half", label: "play.stats.temperature" },
                            { value: statsResult.device.batteryPercent, unit: "%", icon: statsResult.device.batteryPercent >= 80 ? "bi-battery-full" : statsResult.device.batteryPercent >= 30 ? "bi-battery-half" : "bi-battery", label: "play.stats.battery" },
                        ].filter(({ value }) => typeof value === "number" && Number.isFinite(value)).map(({ value, unit, icon, label }) => (
                            <div key={label} className="play-power" title={t(label)} aria-label={`${t(label)} ${Math.round(value)}${unit}`}>
                                <i className={`bi ${icon}`}></i>
                                <span className="fw-semibold">{Math.round(value)}{unit}</span>
                            </div>
                        ))}
                    </div>
                )}

                {/* L'APPUI LONG SE VOIT (le porteur, 2026-11-13) : un anneau à la place du bouton du menu, qui se
                    remplit en trois secondes et s'efface si l'on lâche avant — sans lui, on lâche trop tôt. */}
                {menuHold > 0 && (
                    <div className="play-hold position-absolute pe-none" role="progressbar" aria-label={t("play.menu.hold")} aria-valuenow={Math.round(menuHold * 100)} aria-valuemin={0} aria-valuemax={100}
                        style={{ bottom: menuButtonStyle.bottom, right: menuButtonStyle.right, "--hold": menuHold }}>
                        <svg viewBox="0 0 36 36" aria-hidden="true"><circle className="track" cx="18" cy="18" r="16" /><circle className="fill" cx="18" cy="18" r="16" pathLength="100" /></svg>
                        <i className="bi bi-list"></i>
                    </div>
                )}

                {launchShown && (
                    <PlayLaunch
                        label={t(launchId ? "play.launching" : "play.connecting")}
                        steps={[...(launchId ? [...launchSteps, ...childSteps] : WATCH_LAUNCH_STEPS), ...(introStep ? [{ step: introStep }] : [])]}
                        backdrop={launchId ? { launch: { streamId: launchId, username: viewerUsername } } : (shellStream ? { stream: shellStream } : null)}
                        onCancel={cancelLaunch}
                    />
                )}
                {/* L'INTRO TENUE, par tout autre chemin (#520) : la même ligne, seule, par-dessus la vidéo. */}
                {!launchShown && !(launchId || isWatching) && introStep && (
                    <div className="play-launch pe-none" style={{ background: "none" }}>
                        <div className="play-line"><span className="fw-semibold">{t(`streaming.connection.step.${introStep}`)}</span></div>
                    </div>
                )}

                <PlayNotices notices={notices} />
                <Bell visible={notifications.bellVisible && !panels.includes("menu")} onOpen={openMenu} />

                {panels.includes("menu") && (
                    <PlayMenu
                        session={session}
                        user={user}
                        input={input}
                        isWatching={isWatching}
                        switchesStore={switchesStore}
                        sessionSend={sessionSend}
                        players={broadcastPlayers}
                        roster={roster}
                        departed={departed}
                        notifications={notifications}
                        loan={childLoan}
                        context={context}
                        panels={panels}
                        openPanel={openPanel}
                        closePanel={closePanel}
                        closeMenu={closeMenu}
                        onQuit={openQuit}
                        watchOnly={watchOnly}
                        hostName={hostName}
                        voice={voice}
                    />
                )}
        </div>
    );
}
