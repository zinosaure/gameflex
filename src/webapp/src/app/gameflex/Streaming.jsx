import React, { useCallback, useContext, useEffect, useMemo, useRef, useState, useSyncExternalStore } from "react";
import { useNavigate, useSearchParams } from "react-router-dom";
import { useLaunchProgress } from "./stream/shared/hooks/use-launch-progress.js";
import { reportLaunchFailure } from "./api/retrogaming.js";
import { useTranslation } from "react-i18next";
import * as bs from "react-bootstrap";
import Skeleton from "../skeleton/skeleton.js";
import { isOwnSession, useResolvedSession, useDisplayNames } from "./libs/SessionPool.js";
import { remoteSessionFrom } from "./libs/deviceSession.js";
import { onDeviceFailed, recentDeviceFailure, useDownloadProgress } from "./libs/deviceStream.js";
import { useAuth, getStoredUsername } from "./profile/auth/AuthContext.jsx";
import { usePresence } from "./contexts/PresenceContext.jsx";
import {
    LoadingScreen,
} from "./shared/templates/Essentials.jsx";
import {
    requestCastSession,
    sendSessionIdToReceiver,
    endCastSession,
    getCurrentCastSession,
} from "./stream/onscreen/cast.js";
import { useRetrogamingInput } from "./stream/retrogaming/hooks/use-retrogaming-input.js";
import { useStreamexInput } from "./stream/streamex/hooks/use-streamex-input.js";
import { useRemotePlayInput } from "./stream/remoteplay/hooks/use-remoteplay-input.js";
import { stopSession } from "./api/spawnSessions.js";
import { getRumbleEnabled, setRumbleEnabled, getHapticFeedback, setHapticFeedback } from "./libs/gamepadPreferences.js";
import { GamepadProvider } from "./hooks/use-gamepad.jsx";
import { useConsoles } from "./contexts/ConsolesContext.jsx";
import { useTouchScreen } from "./stream/shared/hooks/use-touch-screen.js";
import { useScreenRotation, usePortraitViewport, rotationSurfaceStyle } from "./stream/shared/hooks/use-screen-rotation.js";
import { useMenuButtonStyle, useMenuHotkey, useIdleVisibility } from "./stream/shared/hooks/use-streaming-menu.js";
import { setGameInputSuspended } from "./libs/gameInput.js";
import { useScreenOrientation } from "./libs/screenOrientation.js";
import { createSwitchesStore } from "./stream/shared/switches-store.js";
import { FloatingPanel, StreamingStatsPanel } from "./stream/onscreen/FloatingPanel.jsx";
// Vendor-prefixed fullscreen, already written once — the page was calling
// the raw API and ignoring this.
import { requestFullscreen, exitFullscreen, isFullscreen, isMobileDevice, isTabletDevice } from "./shared/utils/device-detection.js";
import { deviceDisplayName } from "./libs/capabilities.js";
import {
    apiUrl, clearStreamBase, homeOrigin, setStreamBase, streamHostUsername,
} from "./libs/serverBase.js";
import { kernelSnapshot } from "./libs/kernelChannel.js";
import { federatedHostOf, parseFederatedId } from "./libs/federatedHost.js";
import { usePhysicalGamepad } from "./libs/gamepadPresence.js";

// How long the platform connection waits for the page's gateway viewer before
// going ahead without it — see the gate effect in StreamingShell. Long enough
// for a child that is still attaching to the gateway (SessionStream retries
// every 2s), short enough that a session with no stream at all still reaches
// its own error path instead of hanging silently.
const PLATFORM_CONNECT_WITHOUT_VIEWER_MS = 8000;

const Form = Skeleton.Form;

// Ends the session server-side for everyone (owner-only, see stopSession's
// own doc) — distinct from disconnect()/leavePage(), which only ever tear
// down THIS browser's own connection.
function stopSessionForEveryone(session, token) {
    // Une partie de l'appareil n'a pas de session sur un serveur : c'est
    // `terminate` du hook qui l'arrête, déjà appelé juste avant.
    if (session.platform === "device") {
        return Promise.resolve();
    }

    return stopSession(session.platform, session.id, token);
}



// Attempts real Picture-in-Picture the moment a streaming/watch tab is
// backgrounded — best-effort, never throws out to the caller. Generic
// across all 3 platforms and watch mode, so it lives here, called ONCE by
// StreamingShell, instead of being duplicated inside each useXInput.
export function usePictureInPictureOnHide(videoRef) {
    useEffect(() => {
        const handleVisibilityChange = async () => {
            const video = videoRef.current;
            if (!video) {
                return;
            }
            if (document.hidden) {
                if (document.pictureInPictureEnabled && video.requestPictureInPicture) {
                    try {
                        await video.requestPictureInPicture();
                        return;
                    } catch {
                        // no PiP — fall through
                    }
                }
            } else if (document.pictureInPictureElement === video) {
                document.exitPictureInPicture().catch(() => { });
            }
        };
        document.addEventListener("visibilitychange", handleVisibilityChange);
        return () => document.removeEventListener("visibilitychange", handleVisibilityChange);
    }, [videoRef]);
}

// Generic physical-gamepad presence — every platform's virtual pad hides by
// default while a real controller is connected, same detection everywhere.
// It lives in libs/gamepadPresence.js since the clip-on half screen (#410)
// asks the same question outside a game.

// The one "connecting" surface + the shared video-first stream surface live
// in stream/shared/SessionStream.jsx — Dashboard's live thumbnails import
// the SAME SessionStream, so "live now" and /streaming share one component.
import { SessionStream, LaunchScreen, LaunchBar, OVERLAY_HIDDEN } from "./stream/shared/SessionStream.jsx";
import { useAdaptiveQuality } from "./stream/shared/hooks/use-adaptive-quality.js";
import { useChildSessionWs, CHILD_BEAT_INTERVAL_MS } from "./stream/shared/hooks/use-child-session-ws.js";
import { markActivity, takeActivityStamp } from "./stream/shared/activity-store.js";

// The per-platform input hook — returns a unified shape for all 3 platforms, 
// so StreamingShell can just spread it onto <StreamingShell> and not care about the platform.
// screenRef — the PAGE owns the one gateway viewer now (see SessionStream's
// own doc); every hook receives it instead of creating a second one.
function useStreamingInput({ screenRef, session, context, t, options = {} }) {
    // Le dispatcher suit la MÊME règle que les hooks qu'il appelle : c'est
    // lui aussi un hook partagé entre plateformes. Une règle qui s'arrête
    // avant celui qui la fait respecter ne tient pas longtemps.
    const { isWatching, drivesPad, childSlot, onConnectionError, onProgress } = options;
    switch (session.platform) {
        // L'ÉMULATEUR DU TÉLÉPHONE FAIT DU RETROGAMING (chantier 8, C8-P4).
        //
        // Il avait son propre hook, quatrième plateforme à côté de retro,
        // streamex et remoteplay — alors qu'il émule exactement comme la
        // première. Il a fallu y recopier la manette physique, l'écran
        // tactile, les disques et la pause ; le stick à l'écran n'y était
        // toujours pas, faute d'avoir voulu recopier une cinquième fois la
        // logique par console.
        //
        // Le téléphone sert maintenant le même canal de contrôle
        // (`/api/v1/retrogaming/stream/ws`), donc le même hook le pilote. Ce qui
        // reste différent est DÉCLARÉ, jamais branché ici : le rembobinage
        // n'existe pas sur ce runtime, et la capacité `rewind` le dit.
        case "device":
        case "retrogaming":
            return useRetrogamingInput({
                screenRef, session, context, t,
                options: { isWatching, childSlot, onConnectionError, onProgress },
            });
        case "streamex":
            return useStreamexInput({
                screenRef, session, t,
                options: { isWatching, drivesPad },
            });
        default:
            return useRemotePlayInput({ screenRef, session, context, t, options: {} });
    }
}

// Same frozen-props problem as FloatingPanel (see createSwitchesStore's own
// doc above) — context.modal.open's content AND its opts (title,
// confirmLabel, onConfirm...) are captured once too. skeleton.js's
// "confirm" variant builds its icon/title/footer as static jQuery HTML
// at open() time, so there's no way to make THOSE reactive from outside —
// the fix is to not use that variant at all: open with variant "normal"
// and build the icon/title/switches/footer ourselves, in one component
// that subscribes to the store, so the whole modal (title included)
// actually reflects a toggle the instant it happens.
function QuitConfirmModal({ session, isWatching, isOwner, borrowing = false, quitStore, t, onConfirm, onCancel }) {
    const context = useContext(Skeleton.context);
    const quitOptions = useSyncExternalStore(quitStore.subscribe, quitStore.getSnapshot);
    const contextForm = { values: quitOptions, setValues: quitStore.setValues };
    // Only the OWNER may end the session or write its save state — a joiner
    // is a guest (2026-08-25). isWatching was the wrong gate: it is false
    // for every player, and a joiner is a player.
    // UNE PARTIE DU TÉLÉPHONE A MAINTENANT UNE SESSION À GARDER.
    //
    // Elle n'en avait pas quand ce garde a été écrit (2026-08-27) : quitter,
    // c'était arrêter, et poser la question n'avait pas de sens. Depuis, le
    // téléphone tient sa partie en cours et on peut la reprendre depuis le
    // tableau de bord — les deux issues existent donc vraiment, et le choix
    // redevient une vraie question (2026-09-02).
    const canEndSession = isOwner && !isWatching;
    // GELER N'EST POSSIBLE QUE SUR UN CŒUR.
    //
    // « Mettre le jeu en pause » suppose de figer l'émulation sans la
    // libérer, et seul un cœur libretro sait le faire — dans notre processus
    // pour l'appareil, dans son conteneur pour le serveur. Un conteneur
    // streamex n'a aucun mécanisme de pause, et une vraie PS5 ne se gèle pas
    // depuis un navigateur : proposer l'option là revenait à annoncer ce
    // qu'on ne peut pas tenir (signalé le 2026-09-10).
    const canPause = session.platform === "retrogaming" || session.platform === "device";
    // Un choix retenu d'une session précédente peut être devenu impossible :
    // le magasin d'options est partagé par toutes les plateformes. « Geler »
    // sur une console qui ne gèle pas se lit donc comme « partir » — le plus
    // proche, et le seul qui ne mente pas.
    const stored = canEndSession ? quitOptions.onQuit : "leave";
    const choice = stored === "pause" && !canPause ? "leave" : stored;
    // L'EMPRUNTEUR NE « REGARDE » PAS : il joue avec la manette du
    // propriétaire, et partir la lui rend — son onglet se ferme, elle revient
    // d'elle-même (#274).
    const title = borrowing
        ? t("streaming.quit.titleReturn")
        : isWatching
        ? t("streaming.quit.titleWatch")
        : choice === "kill"
            ? t("streaming.quit.titleKill")
            : choice === "pause"
                ? t("streaming.quit.titlePause")
                : t("streaming.quit.title");
    const confirmLabel = borrowing
        ? t("streaming.quit.confirmReturn")
        : isWatching
        ? t("streaming.quit.confirmWatch")
        : choice === "kill"
            ? t("streaming.quit.confirmKill")
            : choice === "pause"
                ? t("streaming.quit.confirmPause")
                : t("streaming.quit.confirm");

    return (
        <div className="text-center">
            <i className="bi bi-question-circle-fill d-block mb-3 text-primary" style={{ fontSize: "4.5rem" }} />
            <h5 className="modal-title fw-bold mb-2">{title}</h5>
            {canEndSession && (
                <Form.Group
                    label={t("streaming.quit.optionsLabel")}
                    props={{ style: { textAlign: "left" } }}
                >
                    {/* TROIS ÉTATS, PAS UN INTERRUPTEUR.
                        Laisser tourner et arrêter n'épuisaient pas les
                        possibilités : geler la partie libère le joueur sans
                        libérer la machine, et c'est le SEUL moment où un jeu
                        s'arrête. Depuis que l'émulateur du téléphone tourne
                        sans fenêtre, plus rien ne le met en pause par
                        accident — cette case redevient donc un choix, et non
                        un accident (demandé le 2026-09-04). */}
                    <Form.Select
                        name="onQuit"
                        context={contextForm}
                        options={[
                            { value: "leave", label: t("streaming.quit.leaveLabel") },
                            ...(canPause
                                ? [{ value: "pause", label: t("streaming.quit.pauseLabel") }]
                                : []),
                            { value: "kill", label: t("streaming.quit.killLabel") },
                        ]}
                        helpText={t(`streaming.quit.${choice}Help`)}
                    />
                    {/* Une partie du téléphone EST de l'émulation rétro :
                        elle a des sauvegardes d'état comme les autres, et la
                        proposition manquait uniquement parce que le test
                        portait sur le nom de la plateforme. */}
                    {(session.platform === "retrogaming" || session.platform === "device") && (
                        <Form.SkeletonSwitch
                            name="saveStateOnQuit"
                            context={contextForm}
                            label={t("streaming.quit.saveLabel")}
                            helpText={t("streaming.quit.saveHelp")}
                            onText={t("app.yes")}
                            offText={t("app.no")}
                        />
                    )}
                </Form.Group>
            )}
            <div className="d-flex gap-0 mt-4 pt-1 border-top" style={{ marginLeft: "-1rem", marginRight: "-1rem", marginBottom: "-1rem" }}>
                <bs.Button
                    variant="link"
                    className="text-decoration-none flex-fill rounded-0 m-0 py-3 border-end"
                    onClick={() => {
                        context.modal.close();
                        onCancel?.();
                    }}
                >
                    {t("app.cancel")}
                </bs.Button>
                <bs.Button
                    variant="link"
                    className="text-decoration-none flex-fill rounded-0 m-0 py-3 fw-semibold"
                    onClick={() => {
                        context.modal.close();
                        onConfirm(quitStore.getSnapshot());
                    }}
                >
                    {confirmLabel}
                </bs.Button>
            </div>
        </div>
    );
}

// StreamingShell — mounted with key={session.id}, so a session/platform
// change is always a full remount: useStreamingInput's per-platform switch
// stays safe under the Rules of Hooks (exactly one branch runs for the
// whole lifetime of a given instance). Exported for the gamepad-test page (the
// Settings → Gamepad → Test page reuses it with a preset).
// sessionPhase — what the page is waiting for, DERIVED from what it already
// knows rather than stored. A stored phase is a second copy of the truth, and
// every copy in this codebase has eventually disagreed with the thing it
// copied (see chantier 4). Order matters: the first true line wins, so the
// most decisive fact is asked first.
//
// "live" means nothing this page controls is pending. Its LABEL still says
// "waiting for the video", and that is not a contradiction: the overlay is
// only ever rendered while there is no picture, so if everything else is ready
// the picture is precisely what is missing. An empty label there left a
// spinner with nothing next to it — the state that tells you least, shown
// exactly when you most want to know.
// How long each dead end is shown before the page leaves.
//
// Leaving is CHEAP, and that is what makes both numbers easy: the exit
// disconnects, it never terminates, and the child keeps the session for five
// minutes after its queue empties (IdleAfter). A console that comes back one
// minute later is still there to come back TO — the player returns through the
// Dashboard and their seat is waiting.
//
// So these delays are not a hedge against losing anything. They are reading
// time: long enough to see what happened, short enough not to sit on a frozen
// frame wondering. The ended session is a fact and needs only a glance; a
// stopped source is worth watching a little longer in case it simply resumes.
const SESSION_ENDED_REDIRECT_MS = 2500;
// Le jeu qui s'est arrêté DE LUI-MÊME (#504) mérite plus qu'un coup d'œil : ce
// n'est pas la fin qu'on a demandée, et le joueur doit pouvoir lire pourquoi.
const GAME_EXITED_REDIRECT_MS = 6000;
// Au-delà, une intro tenue sans que la partie envoie rien se dit « en attente
// de l'image du jeu » (#520) : en deçà, c'est un démarrage ordinaire.
const INTRO_HELD_SAY_MS = 6000;
// Porté de 10 s à 25 s le 2026-09-07 : depuis que l'arrêt de l'image
// DÉCLENCHE une reprise (SessionStream), partir au bout de dix secondes
// coupait la page pendant sa propre tentative — le filet de la reprise dure
// douze secondes. Une console vraiment éteinte fait donc attendre un peu plus
// longtemps, ce qui est le bon sens de l'erreur : mieux vaut regarder un écran
// noir cinq secondes de trop que perdre une partie qui allait revenir.
const SOURCE_GONE_REDIRECT_MS = 25_000;

export function sessionPhase({ ended, gameExited, authorityOpen, authorityAnswered, claiming, platformConnected, videoSilent, stalling }) {
    // Le jeu s'est arrêté de lui-même (son émulateur est sorti, #504) : une fin
    // que personne n'a demandée, et la page le dit au lieu de « la session est
    // terminée ».
    if (ended && gameExited) return "gameExited";
    if (ended) return "ending";
    if (!authorityOpen) return "connecting";
    if (!authorityAnswered) return "joining";
    if (claiming) return "waitingControl";
    // `platformConnected` est l'état du FLUX (la PeerConnection), et rien
    // d'autre. Le canal de contrôle a longtemps pu l'éteindre en tombant —
    // d'où « En attente de la vidéo… » par-dessus une image qui tournait ;
    // c'est corrigé à la source, dans le onClose du hook rétro, pas ici.
    if (!platformConnected) return "waitingVideo";
    // BEFORE stalling, and that ordering is the point. Stalling means the
    // stream is struggling; this means it stopped — a console switched off,
    // a source that died. Both look identical on screen, and only one of them
    // is worth waiting through, so the page must not offer the hopeful word
    // for the hopeless case.
    if (videoSilent) return "sourceGone";
    if (stalling) return "stalling";

    return "live";
}

// `launchSteps` — les étapes du KERNEL, franchies avant que cette coquille
// existe (#191). Elles descendent d'en haut plutôt que d'être refaites ici :
// la route les a reçues sur le canal du lancement, et les faire disparaître au
// moment où la session se résout donnerait deux listes qui se remplacent là où
// l'utilisateur en attend une seule, qui se poursuit jusqu'à l'image.
// EN SPECTATEUR, LES QUINZE ÉTAPES SONT DÉJÀ ACQUISES.
//
// Demandé avec la fonctionnalité elle-même : « c'est la même chose pour
// isWatching sauf que les étapes fetching games, emulator, etc... seront déjà
// faits, ils seront ok par défaut ». Un spectateur arrive sur une partie qui
// TOURNE : il n'a lancé aucun jeu, donc rien ne lui donne de `launchId`, donc
// l'écran ne se déclenchait jamais pour lui — mesuré le 2026-09-18 sur une
// vraie partie, zéro étape et l'image arrivant nue à 1,3 s.
//
// La dernière entrée n'est pas un ornement : `LaunchSteps` donne le tourniquet
// à la DERNIÈRE de la liste. Sans une étape réellement en cours, il tournerait
// sur « Démarrage de l'émulation », finie depuis longtemps. Ce qui est en cours
// pour un spectateur, c'est l'attache du flux — c'est donc elle qui le porte.
const WATCH_LAUNCH_STEPS = [
    "session.requested", "session.ready",
    "game.loaded",
    "core.required", "core.resolved", "core.loaded", "bios.required",
    "container.creating", "container.created", "container.waiting", "container.ready",
    "rom.ready", "bios.resolved", "emulator.ready",
    "stream.attaching",
].map((step) => ({ step }));

export function StreamingShell({ t, user, token, session, context, setTerminateReason, onTerminate, reportPlaying, reportIdle, onWatchingChange, leaveRef, wantsPlay, onPlayIntentSent, launchSteps = [], launchId = null }) {
    // isWatching is CONSUMED, never derived (roadmap P2): the kernel pushes
    // the role on {type:"private"} right after the subscribe, then on every
    // change. The full UI renders only once that first private message
    // arrived so the mount effects (presence role, pad visibility) see the
    // real value — the localStorage marker and the local decision effect
    // are gone.
    // Un jeu du téléphone n'a pas d'autorité à attendre : il EST l'autorité.
    //
    // Ces deux drapeaux disent « le kernel a ouvert le canal » et « il a
    // répondu » — deux étapes qui n'existent pas quand la source est dans le
    // même appareil que la page. Sans cette exception, la phase de session
    // reste « connexion » pour toujours : l'image arrive, et l'écran affiche
    // quand même « connexion à la session… » par-dessus, manette comprise.
    const isDeviceSession = session?.platform === "device";
    // LA PARTIE DU TÉLÉPHONE EST À SON PROPRIÉTAIRE — pas à tout compte qui
    // l'ouvre (2026-10-23). Les autres la voient dans « en direct » et y
    // arrivent par « Regarder » ou « Rejoindre », comme une partie du
    // serveur : celui qui regardait démarrait en joueur et prenait une place
    // de lui-même (le porteur : « même avec Watch je me retrouve avec une
    // manette »).
    const ownsDeviceSession = isDeviceSession && Boolean(session?.owner) && session.owner === user?.username;

    // UNE PARTIE DU TÉLÉPHONE QUI NE DÉMARRE PAS SE DIT ICI (2026-10-02).
    // L'appareil annonce l'échec sur son canal d'événements — téléchargement
    // cassé, ou jeu que le cœur refuse de charger. Seule la fiche du jeu
    // l'écoutait ; or c'est la page de jeu qui est ouverte à ce moment-là, et
    // elle attendait l'image indéfiniment devant un écran noir.
    //
    // L'échec arrive souvent AVANT que cette page ne s'abonne (19 ms mesurées) :
    // on regarde donc d'abord s'il y en a un, récent, pour CE jeu — puis on
    // écoute. Un échec d'un autre jeu ne nous concerne pas.
    const deviceGame = isDeviceSession ? String(session?.id || "").replace(/^device:/, "") : "";

    useEffect(() => {
        if (!isDeviceSession) {
            return undefined;
        }

        const end = (failure) => {
            setTerminateReason(failure?.reason
                ? `${t("streaming.launch.failed")} (${failure.reason})`
                : t("streaming.launch.failed"));
        };
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

    // C7-P2 : une partie du téléphone parle à la façade v4 du kernel, qui ne
    // connaît qu'une session fixe — « device-session » — quel que soit le jeu
    // lancé. La page garde son id propre (`device:<fichier>`) pour le reste.
    const childSessionId = session.childSessionId || session.id;
    // Et son porteur JOUE — il n'est pas spectateur d'une partie qui serait
    // ailleurs. Sans ça, la manette à l'écran ne se monte jamais : elle
    // attend un rôle que personne ne viendra donner.
    // UN INVITÉ ARRIVE EN SPECTATEUR.
    //
    // Il n'a pas de siège : la manette appartient à celui qui tient le
    // téléphone, et elle se DEMANDE (handover). Démarrer en joueur — ce que
    // faisait `!isDeviceSession`, vrai pour l'invité comme pour l'hôte —
    // sautait toute cette étape : la section « demander la manette » ne
    // s'affichait jamais, et l'invité se croyait aux commandes sans que rien
    // ne réponde (signalé le 2026-09-01).
    //
    // Il n'y a pas de « takeover » ici, et c'est voulu : reprendre la main
    // suppose qu'on sache que c'est la MÊME personne sur un autre appareil,
    // ce qu'un réseau local sans comptes ne peut pas établir. Tout passe
    // donc par le handover, qui demande à un humain.
    const [isWatching, setWatching] = useState(!ownsDeviceSession || Boolean(session?.guest));
    // Has the authority answered at all? Set by the child's first state push
    // (and from the start on the device's own page, which is its own host).
    const [kernelPrivateReady, setKernelPrivateReady] = useState(isDeviceSession);
    // chantier-4 — MY slot, straight from the child. Not read back from the
    // pool: that copy arrives several hops later (child → mirror → kernel →
    // SessionPool), and in the meantime the page would dial the platform with
    // a role it no longer has. Using the authority's own answer instead of a
    // lagging copy is the whole point of this chantier.
    const [childSlot, setChildSlot] = useState(null);
    // THE TWO ENDS OF A LOAN, as the child says them for THIS tab: the owner
    // may take their lent pad back, the borrower may give it back. Never
    // worked out here — a hand-over lends the pad, and only the child knows
    // who lent what.
    const [childLoan, setChildLoan] = useState({ canReclaim: false, canReturn: false, lentTo: "" });
    // WHICH PAD THIS TAB'S KEYS DRIVE, as the child says it (`you.controls`),
    // null until it has. Not the same question as isWatching: during a loan
    // (#274) the owner keeps their place and drives nothing, while the
    // borrower — a "playable viewer" — watches and drives the owner's pad.
    // isWatching says how this tab is CONNECTED; drivesPad, below, says
    // whether its controller does anything.
    const [childControls, setChildControls] = useState(null);
    // Has the CHILD spoken yet? kernelPrivateReady starts true on the device's
    // own page, before its child has said anything: anything that must wait
    // for the authority itself waits on this.
    const [childStateSeen, setChildStateSeen] = useState(false);
    // Said by the authority, never inferred. Drives the ending phase and the
    // redirect that follows it.
    const [sessionEnded, setSessionEnded] = useState(false);
    // Pourquoi, quand personne ne l'a arrêtée : le jeu s'est arrêté (#504).
    const [gameExited, setGameExited] = useState(false);
    // Reported by the video surface itself (GatewayWebRTC owns the element and
    // watches the frame counter), so it is true whether or not anyone opened
    // the stats panel.
    const [videoSilent, setVideoSilent] = useState(false);
    // Everything this page says out loud names a person the way they chose to
    // be named. The raw username keeps its jobs — keys, comparisons, the wire.
    const displayNames = useDisplayNames();
    const nameOf = useCallback((username) => displayNames[username] || username, [displayNames]);
    const authorityGoneRef = useRef(null);
    useEffect(() => () => clearTimeout(authorityGoneRef.current), []);
    // The pending hand-over request addressed to THIS holder (its prompt),
    // read from the child's state push.
    const [slotRequests, setSlotRequests] = useState([]);
    // The broadcast players[] — who holds which pad, for targeting the
    // handover ask at the current holder.
    const [broadcastPlayers, setBroadcastPlayers] = useState([]);
    // Present, holding nothing — what the "give the pad to…" entry draws
    // from. Absent when nobody watches, and then the give entry simply does
    // not appear — the same "absent = no section" rule the whole menu
    // already follows.
    const [broadcastWatchers, setBroadcastWatchers] = useState([]);

    const hasPhysicalGamepad = usePhysicalGamepad();
    // The surface that goes fullscreen: the picture AND everything laid
    // over it (pad, panel, overlays), which are siblings of the <video>.
    const shellRef = useRef(null);
    // A phone or a tablet — the only devices anyone holds sideways, and the
    // only ones this switch is offered on. Read once: a viewport that crosses
    // 768px mid-session is a desktop window being resized, not a device
    // turning into a phone.
    const [isHandheld] = useState(() => isMobileDevice() || isTabletDevice());
    const isPortraitViewport = usePortraitViewport();
    // The CSS fallback, not the goal. Turning the SCREEN is the browser's job
    // and it does it properly (the OS rotates, everything follows); rotating
    // the surface ourselves is what is left when the browser refuses — which
    // iOS Safari always does, and Chrome does outside fullscreen.
    // The page owns THE gateway viewer (one per page, whole page life) —
    // see SessionStream's own doc. The hooks used to each create their own
    // inside their WS onOpen, which is what made the video restart on every
    // role change and vanish for good on a missed "started".
    const screenRef = useRef(null);
    // Read once per mount, synchronously — see getStoredUsername's doc and
    // the SessionStream props below.
    const viewerUsernameRef = useRef(null);

    if (viewerUsernameRef.current === null) {
        // CHEZ UN HÔTE FÉDÉRÉ, C'EST SON NOM QUI COMPTE.
        //
        // `getStoredUsername` décode NOTRE jeton : parfait ici, faux là-bas.
        // Ce nom prend une place dans la file de joueurs de l'hôte, et le
        // nôtre n'y désigne personne (C9-P4).
        viewerUsernameRef.current = streamHostUsername() || getStoredUsername() || "";
    }

    const viewerUsername = viewerUsernameRef.current || null;
    // LES JALONS DU CONTENEUR (#191) — cœur, ROM, BIOS, jeu, géométrie. Ils
    // arrivent par le WebSocket de la session, donc après ceux du kernel, et se
    // mettent à leur suite : une seule liste, du clic à l'image.
    const [childSteps, setChildSteps] = useState([]);
    // LA PREMIÈRE IMAGE, REMONTÉE JUSQU'ICI (#191).
    //
    // `SessionStream` la connaissait déjà mais la gardait pour lui (son
    // `sawPictureRef` ne sert qu'à son filet de reprise). C'est pourtant LE
    // signal que l'utilisateur a demandé pour la bascule : « dès que l'image
    // arrive et que la manette est initialisée ». `input.isConnected`, qui
    // servait jusqu'ici, dit seulement que la PeerConnection est établie — le
    // jeu pouvait tourner à l'écran avec les étapes encore par-dessus, ce qui
    // a été constaté en vrai sur le téléphone.
    const [sawPicture, setSawPicture] = useState(false);
    // LA PREMIÈRE IMAGE N'EST PLUS CELLE DU JEU (chantier 35) : la passerelle
    // envoie d'abord l'intro — figée pendant le chargement, puis jouée. Elle
    // dit où en est ce spectateur sur le canal de contrôle, AVANT la qualité ;
    // une passerelle qui donne la qualité sans l'intro n'en joue pas, et une
    // qui ne dit rien du tout (sans canal de contrôle) a envoyé le jeu.
    const [introLive, setIntroLive] = useState(false);
    const introToldRef = useRef(false);
    // LE JEU SE CHARGE : la partie envoie des images, mais rien encore — une
    // fenêtre d'émulateur noire. La passerelle tient l'intro, et la ligne
    // d'étape le dit (le porteur, 2026-10-26 : « avec un message qui dit ce
    // qui se passe »).
    const [gameLoading, setGameLoading] = useState(false);
    // Où en est l'intro (`hold`, `play`, `switch`, `live`), et si l'hôte a
    // prévenu que le démarrage se compte en minutes (Box64 au téléphone,
    // #521) ; `heldLong` : tenue depuis plus de INTRO_HELD_SAY_MS (#520).
    const [introPhase, setIntroPhase] = useState(null);
    const [slowStart, setSlowStart] = useState(false);
    const [heldLong, setHeldLong] = useState(false);
    // Le flux de la partie, pour le fond de l'écran d'attente de celui qui la
    // rejoint : il n'a pas de lancement à suivre, c'est sa propre vidéo.
    const [shellStream, setShellStream] = useState(null);
    const noteChildStep = useCallback((step, detail) => {
        setChildSteps((previous) => [...previous, { step, detail, state: "done" }]);
    }, []);
    // L'ATTACHE NE SE DIT QU'UNE FOIS (#195).
    //
    // `noteChildStep` empile sans dédoublonner, et ça suffisait : son seul
    // appelant jusqu'ici — les jalons du kernel — n'émet chaque étape qu'une
    // fois. L'attache de la passerelle, elle, se rejoue : quand aucune image
    // n'arrive avant l'échéance, `SessionStream` relance la négociation et
    // rappelle `onViewerReady` (son `restart`, SessionStream.jsx). Et sur
    // remote-play c'est le cas ORDINAIRE, pas l'exception — une demande
    // d'image-clé perdue n'est jamais réémise. Sans ce garde, l'écran
    // afficherait « Flux connecté » deux fois sur la plateforme même pour
    // laquelle cette étape a été écrite.
    const gatewayStepSaidRef = useRef(false);
    // Une passerelle sans canal de contrôle (avant la 0.6.0, chez un ami) ne
    // dira jamais rien : l'image qu'elle envoie est celle du jeu.
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
    // CE QUE L'INTRO TENUE DIT, en une étape (#520, #521) : un démarrage qui se
    // compte en minutes, le jeu qui se charge, ou, passé quelques secondes
    // sans rien, l'image du jeu qu'on attend.
    let introStep = null;
    if (introPhase === "hold") {
        if (slowStart) {
            introStep = "game.loadingSlow";
        } else if (gameLoading) {
            introStep = "game.loading";
        } else if (heldLong) {
            introStep = "game.waitingPicture";
        }
    }
    // Le pad à l'écran, la manette physique, le tactile : tout ce qui ENVOIE
    // suit cette réponse-là. Avant que le child ait parlé, c'est l'ancienne
    // règle — jouer, c'est piloter.
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
            onConnectionError: (reason) => {
                // chantier-4: "nobody is playing yet" is a NORMAL state, not a
                // failure. Everybody arrives watching, so on a child-managed
                // session a watcher's platform connection is expected to fail
                // until somebody takes the pad — and a fatal screen there hides
                // the very panel that lets them take it.
                //
                // The session's own existence is not in doubt (the child is
                // answering on its socket); only the emulation has not started.
                if (isWatching) {
                    return;
                }

                setTerminateReason(reason);
            },
        },
    });

    // L'ÉCRAN TACTILE — la Nintendo DS, et toute console qui en a un.
    //
    // Le registre des consoles le dit (`touch`), et c'est une donnée du
    // matériel : la DS a un écran du bas, la NES n'en a pas, aucun core
    // n'annonce ça. Le savoir ici évite une liste de consoles codée dans le
    // frontend à côté de celle qui existe déjà en Go.
    //
    // Un spectateur n'y touche pas, comme il ne touche pas la manette.
    const consoles = useConsoles();
    const hasTouchScreen = consoles.some(
        (entry) => entry.id === session.consoleId && entry.touch,
    );
    useTouchScreen({
        screenRef,
        enabled: hasTouchScreen && drivesPad,
        send: input.extraProps?.touch?.send,
    });

    // P2 §1 — the façade socket drives the platform connection lifecycle:
    // onConnect at open, onLeave at error, onDisconnect at close. The
    // callbacks read `input` through a ref (input is rebuilt every render,
    // the socket must never reconnect over its identity).
    const inputRef = useRef(null);
    inputRef.current = input;
    // Platform connect gate — see the effect below the socket setup.
    const [facadeOpen, setFacadeOpen] = useState(isDeviceSession);
    const [viewerReady, setViewerReady] = useState(false);
    const platformConnectedRef = useRef(false);
    // chantier-4 — every session is spoken to through its CHILD, the only
    // authority on who plays: the page talks to it directly (through the
    // kernel's byte-for-byte proxy) and the kernel decides nothing about it.

    // QUI ÉTAIT LÀ AU DERNIER ÉTAT — pour ne dire que ce qui a CHANGÉ.
    //
    // Les annonces d'arrivée et de départ venaient d'événements du kernel, sur
    // un socket que les sessions tenues par leur child n'ouvraient pas — donc
    // plus rien ne s'annonçait, sur AUCUNE des deux plateformes (signalé le
    // 2026-09-05 sur téléphone).
    //
    // On le dérive donc de l'état que le child pousse déjà. Rien de nouveau
    // sur le fil : la liste des présents est là, il suffisait de la comparer
    // à la précédente.
    const presentsRef = useRef(null);

    const handleChildState = useCallback((message) => {
        // isWatching is the child's answer about THIS tab, not a role to
        // interpret: a tab plays if and only if its own socket holds the slot.
        const watching = !message.you?.isPlaying;
        setWatching(watching);
        setChildSlot(message.you?.slotId ?? null);
        setChildControls(message.you?.controls ?? 0);
        setChildLoan({
            canReclaim: message.you?.canReclaim === true,
            canReturn: message.you?.canReturn === true,
            // Who drives the owner's pad — "" when nobody borrowed it.
            lentTo: message.lentTo || "",
        });
        setBroadcastPlayers((message.players ?? []).map((player) => ({
            username: player.username,
            role: player.isOwner ? "owner" : "player",
            slot: player.slotId,
            lentTo: player.lentTo || "",
        })));
        // Who is here WITHOUT a pad — the list the "give the pad to…" entry
        // draws from. It has no v1 equivalent: the façade only ever published
        // players, which is why only the ASK direction could be built.
        setBroadcastWatchers(message.watchers ?? []);
        // Reused as-is by the holder's prompt below — the pending demand has
        // the same meaning, so the same UI serves it.
        setSlotRequests(
            message.pendingHandover && message.pendingHandover.to === user?.username
                ? [{ from: message.pendingHandover.from }]
                : [],
        );
        setKernelPrivateReady(true);
        setChildStateSeen(true);
        // The END arrives here, from the authority itself, while its socket is
        // still open — not later as a dead connection. A page that learns the
        // session is over only when the socket drops cannot tell "it ended"
        // from "I lost the network", and shows the wrong thing for both.
        if (message.terminated) {
            setGameExited(Boolean(message.gameExited));
            setSessionEnded(true);
        }
        // Les allées et venues, annoncées une fois chacune.
        //
        // Joueurs ET spectateurs : arriver pour regarder est une arrivée. Le
        // premier état ne dit rien — trouver trois personnes en entrant dans
        // une partie n'est pas une nouvelle, c'est la situation.
        const presents = new Set([
            ...(message.players ?? []).map((player) => player.username),
            ...(message.watchers ?? []),
        ].filter(Boolean));
        const avant = presentsRef.current;
        presentsRef.current = presents;

        if (avant) {
            for (const username of presents) {
                if (!avant.has(username) && username !== user?.username) {
                    context.toast.open(
                        t("streaming.session.playerJoined", { username: nameOf(username) }),
                        { variant: "normal" },
                    );
                }
            }
            for (const username of avant) {
                if (!presents.has(username) && username !== user?.username) {
                    context.toast.open(
                        t("streaming.session.playerLeft", { username: nameOf(username) }),
                        { variant: "normal" },
                    );
                }
            }
        }

        onWatchingChange?.(watching);
    }, [onWatchingChange, user?.username, context, t, nameOf]);

    // « TOUJOURS LÀ ? » — la question du child, adressée à CE joueur.
    //
    // C'est le child qui surveille les entrées de ses joueurs, et c'est lui
    // qui pose la question à celui qui n'a rien touché depuis vingt-neuf
    // minutes. Vingt-neuf et pas quinze : une cinématique peut durer plus
    // d'un quart d'heure, et regarder son propre jeu n'est pas s'absenter.
    //
    // TOUT EST FRONTEND JUSQU'À L'ÉCHÉANCE. Le compte à rebours n'est qu'un
    // affichage ; personne n'est exclu tant qu'il court. À la fin, c'est le
    // child qui exclut — pas cette page, qui pourrait très bien être celle
    // qu'on vient de fermer.
    //
    // Répondre ne demande pas de viser ce bouton : toucher n'importe quoi
    // suffit, le prochain battement porte `hadInput` et le child efface sa
    // question tout seul. Le bouton est là pour qui a les mains ailleurs.
    const handleChildAfk = useCallback((message) => {
        const seconds = Math.max(Math.round((message.graceMs || 60000) / 1000), 1);
        context.toast.open(t("streaming.afk.warning", { seconds }), {
            variant: "confirm",
            title: t("streaming.afk.title"),
            confirmLabel: t("streaming.afk.stillHere"),
            cancelLabel: t("app.cancel"),
            onConfirm: () => {
                markActivity();
                beatRef.current?.(true);
            },
        });
    }, [context, t]);

    const handleChildResult = useCallback((message) => {
        // Somebody handed YOU the pad. The state push already moved the slot;
        // what it cannot say is who held it out, and being handed a controller
        // by name is the whole point of the gesture.
        if (message.status === "received") {
            context.toast.open(
                t(message.lent ? "streaming.handover.lentToYou" : "streaming.handover.receivedFrom", { username: nameOf(message.from) }),
                { variant: "success" },
            );
            return;
        }
        if (message.status === "granted") {
            return; // the state push already said so — this would be noise
        }
        // "abandoned" is not a refusal: it was legitimate when asked and the
        // world moved. Saying "you were denied" there would be a lie. And a
        // refusal says WHY when the child named it — "you already play
        // elsewhere" is not "the player kept the pad" (2026-10-09).
        const reasons = {
            "already playing in another session": "streaming.handover.playsElsewhere",
            "no slot available": "streaming.handover.noSlot",
        };
        context.toast.open(
            message.status === "abandoned"
                ? t("streaming.handover.claimFailed")
                : t(reasons[message.reason] || "streaming.handover.denied"),
            { variant: "warning" },
        );
    }, [context, t, nameOf]);

    const childSocket = useChildSessionWs({
        sessionId: childSessionId,
        token,
        enabled: true,
        onState: handleChildState,
        onResult: handleChildResult,
        onAfk: handleChildAfk,
        onConnect: () => {
            setFacadeOpen(true);
            // Back on its feet: whatever the silence was, it was not the end.
            clearTimeout(authorityGoneRef.current);
        },
        onDisconnect: () => {
            setFacadeOpen(false);
            platformConnectedRef.current = false;
            // In v4 the child EXITS when its session is over, so a socket that
            // stays dead IS the end — and it is the only signal for a session
            // stopped from outside, which never gets to announce itself
            // (terminated arrives only on the child's own end paths).
            //
            // The delay is the whole point: a socket that closes and comes
            // back is a network blip, and calling that an ended session would
            // throw people out of a game over a hiccup. Only silence that
            // LASTS means the container is gone.
            clearTimeout(authorityGoneRef.current);
            authorityGoneRef.current = setTimeout(() => setSessionEnded(true), 12000);
        },
    });

    // ONE sender for the panel and the prompts. It translates the panel's
    // vocabulary into the CHILD's intents — the child alone decides who
    // plays — so FloatingPanel and the holder's prompt need to know nothing
    // about it.
    const sessionSend = useCallback((message) => {
        const action = message.type === "requestslot"
            ? (message.to ? "request-handover" : "join")
            : message.type === "cedeslot"
                ? "grant-handover"
                : message.type === "slotrequestdenied"
                    ? "deny-handover"
                    // The other direction: the holder passes the pad to
                    // somebody watching, without being asked.
                    : message.type === "giveslot"
                        ? "give-handover"
                        // The two ends of a loan (2026-10-09).
                        : message.type === "reclaimslot"
                            ? "reclaim"
                            : message.type === "returnslot"
                                ? "return"
                                : null;

        if (!action) {
            return;
        }

        // On the child's own socket (P3): it speaks for this tab, and the
        // socket not having said hello yet simply means nothing goes out.
        childSocket.intent(action, message.to ?? null);
    }, [childSocket]);

    // "I came here to play" — carried by ?play=1, set only by launching a
    // game, joining from the Dashboard, or taking a session back (see
    // buildStreamingUrl). Without it, launching a game landed on a black
    // watch screen: in v4 nobody plays until someone claims a slot, so the
    // emulator had not started and there was genuinely nothing to stream.
    //
    // It is an INTENT, never a grant. It runs one ordinary join through the
    // same POST any button uses, and the child validates it like any other —
    // a forged flag gets exactly what pressing "Take the controller" gets.
    // Fired here rather than at the button because the intent has to name the
    // socket that will hold the slot, and that socket only exists on this page.
    const playIntentRef = useRef(false);
    useEffect(() => {
        // Wait for the CHILD's own answer — childStateSeen, never
        // kernelPrivateReady. On a brand-new session the page loads before the
        // mirror lands, so the v1 kernel path answers first and briefly says
        // "you are the owner, you are playing". Trusting that here spent the
        // intent WITHOUT EVER POSTING IT: the child then took over with an
        // empty queue and the page sat in watch forever. Reported live
        // 2026-08-25 ("je lance un jeu et je suis toujours en mode watch").
        // A device game's OWNER takes slot 1 the moment the facade socket is
        // up and the child answered — there is no ?play=1 for a phone launch.
        // Anyone else asks like on a server session: ?play=1, from "Join".
        const playIntentWanted = wantsPlay || ownsDeviceSession;
        if (!playIntentWanted || playIntentRef.current || !childStateSeen) {
            return;
        }
        // Already holding a slot ACCORDING TO THE CHILD (a relaunch that
        // reused a session we still hold): satisfied, so spend it rather than
        // ask again. childSlot is written by handleChildState alone.
        if (childSlot != null) {
            playIntentRef.current = true;
            onPlayIntentSent?.();
            return;
        }

        if (!childSocket.intent("join")) {
            return; // no socket yet — this effect reruns when the state lands
        }

        playIntentRef.current = true;
        // Drop the flag from the URL once spent, so a link copied out of the
        // address bar stays a WATCH link — which is the one promise a shared
        // streaming link makes.
        onPlayIntentSent?.();
    }, [wantsPlay, ownsDeviceSession, childStateSeen, childSlot, childSocket, onPlayIntentSent]);

    // The platform connection starts once the façade socket is open AND the
    // page's gateway viewer exists — the hooks put that viewer's id on their
    // own wire (retro's start/join/spectate, streamex's WS query) instead of
    // creating a second one, so it has to be there before they speak. This
    // ordering is what lets the video be created ONCE, up front, from the
    // sessionId alone (see SessionStream) rather than inside each hook's WS
    // onOpen where a role change or a missed "started" would restart or lose it.
    //
    // The viewer is NOT a hard prerequisite though: a session the gateway has
    // no stream for (blank test session, gateway unreachable, a child that
    // never attached) would otherwise leave the page waiting forever and never
    // even attempt the platform connection — and for retro that means the
    // "start" message never goes out, so the emulator never boots. Waiting is
    // therefore bounded: past the deadline the hooks connect without a
    // viewerId and the child's own error path surfaces something the user can
    // read, exactly as it did before the page owned the viewer.
    useEffect(() => {
        if (!facadeOpen || platformConnectedRef.current) {
            return undefined;
        }

        const startPlatformConnection = () => {
            if (platformConnectedRef.current) {
                return;
            }

            platformConnectedRef.current = true;
            inputRef.current?.onConnect?.();
        };

        if (viewerReady) {
            startPlatformConnection();

            return undefined;
        }

        const deadline = setTimeout(startPlatformConnection, PLATFORM_CONNECT_WITHOUT_VIEWER_MS);

        return () => clearTimeout(deadline);
    }, [facadeOpen, viewerReady]);

    // Kept current every render; read by the beat interval, which must not
    // be rebuilt when it changes.
    const beatRef = useRef(null);
    beatRef.current = childSocket.beat;

    // THE BEAT (chantier-4): ONE message every 30s to the child, carrying BOTH
    // meanings — its arrival proves the tab's JS runs (a frozen tab keeps its
    // socket open, so the socket alone cannot tell), and hadInput proves a
    // human is behind it. The input hooks stamp the shared store on every real
    // input; the beat drains it.
    //
    // Through a ref, so this interval lives as long as the page and NOT as
    // long as these callbacks keep their identity. An interval that restarts
    // on every render never reaches its own period — measured: 0 beats in
    // 102s, and the child rightly evicted a player it never heard from.
    useEffect(() => {
        const beatInterval = setInterval(
            () => beatRef.current?.(takeActivityStamp() > 0),
            CHILD_BEAT_INTERVAL_MS,
        );

        return () => clearInterval(beatInterval);
    }, []);

    // QUI JE SUIS DANS CETTE PARTIE — dérivé de ce que le child a poussé,
    // jamais de ce que la page a écrit en arrivant.
    //
    // Le chemin appareil se déclarait propriétaire dans l'objet session
    // lui-même : tout onglet ouvrant `?device=1` l'était, quel que soit le
    // compte. Le child dit pourtant déjà qui est qui (`players[].isOwner`),
    // et la page l'écoutait déjà — il n'y avait qu'à cesser de le contredire
    // (signalé le 2026-09-04).
    //
    // Le repli sur `session.me` garde les sessions du serveur inchangées :
    // là, c'est le kernel qui remplit ce champ, et il fait autorité.
    const myRole = useMemo(() => {
        const mine = (broadcastPlayers || []).find(
            (player) => player.username === user?.username,
        );

        if (mine?.role) {
            return mine.role;
        }
        // POSSÉDER UNE SESSION ET TENIR UNE MANETTE SONT DEUX CHOSES.
        //
        // Quand la file est vide — personne n'a encore pris de siège — le
        // propriétaire n'apparaît dans aucune liste, et se retrouvait
        // spectateur de sa propre partie : plus de bouton d'arrêt, et la
        // session tournait sans que personne puisse l'éteindre (signalé le
        // 2026-09-04). C'est la session qui dit à qui elle est.
        if (session.owner && session.owner === user?.username) {
            return "owner";
        }

        return session.me?.role || "spectate";
    }, [broadcastPlayers, user?.username, session.me?.role, session.owner]);

    const quitStoreRef = useRef(null);

    if (!quitStoreRef.current) {
        quitStoreRef.current = createSwitchesStore({ onQuit: "leave", saveStateOnQuit: false });
    }

    const quitStore = quitStoreRef.current;

    // A stable per-mount store (not a plain useState) — see
    // createSwitchesStore's own doc above: FloatingPanel is opened via
    // context.offcanvas.open, which freezes whatever props it's given at
    // click time, so a plain useState here would update StreamingShell's
    // own render but never reach an already-open panel.
    const switchesStoreRef = useRef(null);
    if (!switchesStoreRef.current) {
        switchesStoreRef.current = createSwitchesStore({
            // Named for what the SWITCH says ("play the game's sound"), not
            // for the <video> attribute. It used to be `isMuted` behind a
            // "play the sound" label, so turning the switch ON muted the
            // game — reported live 2026-08-24. Sound is on by default; the
            // autoplay fallback below is the only thing that turns it off
            // on its own, and only because the browser refused.
            isSoundEnabled: true,
            isFullscreen: false,
            isChromeCasting: !!getCurrentCastSession(),
            isResourceMonitoringVisible: false,
            // LE DOUBLE FLUX (chantier 31) : le mode de la catégorie « Vidéo »
            // du menu (`auto` par défaut, `current`, `low`), et ce que la
            // passerelle dit du spectateur de la page.
            videoQuality: "auto",
            videoAvailable: false,
            videoCurrent: "hd",
            videoWant: "hd",
            // Device-level preferences (localStorage, not per-session) —
            // libs/gamepadPreferences.js, already read by the real rumble/
            // haptic dispatchers (libs/rumble.js, stream/onscreen/
            // Controllers.jsx). These switches just mirror/write that same
            // global setting, not something local to this session.
            isRumbleEnabled: getRumbleEnabled(),
            isHapticFeedback: getHapticFeedback(),
            // usePhysicalGamepad returns a BOOLEAN, not a getter — calling it
            // throws while the store is being built, which happens before the
            // first paint. Hiding the pad here (rather than in the JSX) is what
            // keeps it a DEFAULT: the menu switch can bring it back, and since
            // reading a physical pad no longer lives inside the pad component
            // (use-streamex-physical-input.js), hiding it costs no input.
            isControllerVisible: drivesPad && !hasPhysicalGamepad,
            isChainButtons300: drivesPad,
            isRewinding: false,
            isFastForwarding: false,
            // ÉTEINT PAR DÉFAUT DEPUIS LE 2026-10-11 : la PS1 démarre avec sa
            // manette d'origine, numérique (décision du porteur ; avec la
            // DualShock, X ne passait pas l'écran titre de Crash Team
            // Racing). L'interrupteur part d'accord avec le moteur et avec
            // use-retrogaming-input.js. Le récit qui suit est celui de la
            // DualShock par défaut, d'août à cette date — sa leçon tient :
            // le défaut décrit un appareil, pas un rôle.
            //
            // SANS `!isWatching`, ET C'EST LE CORRECTIF (2026-09-09).
            //
            // Tout le monde arrive en SPECTATEUR : à la construction du
            // magasin, `isWatching` est donc vrai, et l'interrupteur naissait
            // éteint. L'effet qui l'observe appelait alors `PS1.standard`,
            // ramenant le type de manette à 1 — le pad digital — alors que
            // le hook ET l'enfant sont en DualShock. Prendre la manette
            // ensuite ne change rien : les valeurs initiales du magasin sont
            // calculées UNE fois, et l'effet ne se redéclenche que sur ce
            // drapeau-là. D'où le geste que l'utilisateur décrit : éteindre
            // puis rallumer pour que la console voie enfin la DualShock.
            //
            // Ce défaut décrit un APPAREIL, pas un rôle. Qui regarde décide
            // si la section s'affiche, pas de quel type de manette la
            // console est équipée — et un spectateur qui la positionne
            // n'envoie rien de toute façon.
            isDualShockController: false,
        });
    }
    const switchesStore = switchesStoreRef.current;
    const switches = useSyncExternalStore(switchesStore.subscribe, switchesStore.getSnapshot);
    useAdaptiveQuality({
        screenRef,
        mode: switches.videoQuality,
        quality: { available: switches.videoAvailable, current: switches.videoCurrent, want: switches.videoWant },
        paused: !introLive,
    });
    const setSwitches = switchesStore.setValues;

    if (!input) {
        setTerminateReason(t("streaming.unsupportedPlatform"));
        return null;
    }

    const pipVideoRef = useRef({
        get current() {
            return screenRef.current?.videoElement ?? null;
        },
    }).current;
    usePictureInPictureOnHide(pipVideoRef);

    // leavePage on unmount (pause only, never disconnect — roadmap P2):
    // a plain unmount/remount must not send retro-gaming's explicit
    // "exit" signal. The CONNECT trigger moved to the façade WS lifecycle
    // (onConnect above) — connecting as "owner" from a watching tab
    // reattached the child session and flipped the playing tab (reported
    // live), so the role is derived inside the hook now.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    useEffect(() => () => input.leavePage?.(), []);

    // Confirm before leaving or refreshing WHILE PLAYING — the browser's
    // own prompt, all three platforms. It used to live inside remote-play's
    // connection hook, so refreshing a retro or streamex session
    // interrupted play with no warning at all; nothing about it is
    // platform-specific. Spectators are never prompted: watching costs
    // nothing to interrupt.
    useEffect(() => {
        // Celui qui pilote une manette prêtée joue aussi (#274).
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

    // v1 (chantier-3) — LEAVING THE PAGE IS LEAVING THE SESSION.
    //
    // This is the rule the whole v1 model rests on, and until now no
    // platform actually implemented it: retrogaming called leavePage() from
    // its lifecycle callbacks, while streamex and remoteplay left theirs as
    // explicit no-ops. Nothing announced the departure — the transport just
    // died and the kernel found out on its read deadline. That silent gap
    // IS the phantom socket: a pad held by a tab that no longer exists.
    //
    // pagehide rather than beforeunload: beforeunload is the CONFIRMATION
    // (below) and may be cancelled, pagehide fires when the page really
    // goes, including on mobile. The unmount path covers SPA navigation.
    //
    // disconnect(), never terminate(): leaving keeps the session alive for
    // whoever else is playing. Ending it on purpose is the Quit dialog's
    // job, and it sets quitHandledRef so we do not fire twice.
    //
    // ⚠️ The departure is bound to the ROUTE's lifetime, not this shell's.
    // Binding it here was wrong and killed sessions in seconds: `Streaming`
    // re-renders `if (!session) return <overlay>` whenever the pool snapshot
    // momentarily lacks the session, which UNMOUNTS this shell — a remount,
    // not a departure. The relay was cut ~5 s after every connect.
    // `leaveRef` lets the route (which only unmounts when the page really
    // goes) call our disconnect.
    const quitHandledRef = useRef(false);
    useEffect(() => {
        leaveRef.current = () => {
            if (quitHandledRef.current) {
                return;
            }
            quitHandledRef.current = true;
            inputRef.current?.disconnect?.();
        };

        return () => {
            leaveRef.current = null;
        };
    }, [leaveRef]);

    // Presence — the same shape for all 3 platforms and both roles. Gated
    // on kernelPrivateReady so the first report already carries the real
    // role (the shell used to mount only after the first private push).
    useEffect(() => {
        if (!kernelPrivateReady) {
            return undefined;
        }
        reportPlaying(session, isWatching);
        return () => reportIdle();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [kernelPrivateReady]);

    // A granted slot flips this page to player via the kernel role push
    // (private.role) — the per-platform hooks reconnect their own side off
    // that transition (P3/P4). No local flip remains: the old
    // wasTakenOver/ceded/slotGrantCount mirrors are gone.

    // A watching tab that acquires a pad becomes a player — the role push
    // covers every path now (P3-P5: freeSlot join, handover grant,
    // remoteplay take-back), no me.padSlot rising-edge mirror remains.

    // The controller switch was baked at mount from isWatching (watch mode
    // starts without the pad on screen) — a watcher who becomes a player
    // (join/takeover/grant) must get the pad back without touching the
    // settings panel first.
    //
    // MAIS PAS S'IL Y A UNE VRAIE MANETTE.
    //
    // Cet effet forçait le pad tactile à VISIBLE, sans condition. Comme
    // `switchesStore` est créé au premier rendu seulement (voir son propre
    // commentaire), la détection de manette n'avait donc jamais son mot à
    // dire : le pad ne disparaissait pas, quoi qu'on branche (signalé le
    // 2026-09-03).
    //
    // Pire, ça donnait un signal FAUX. On lisait « le pad ne se cache pas »
    // comme « la page ne voit pas la manette » — deux choses sans rapport,
    // et un diagnostic envoyé dans le mur.
    //
    // `hasPhysicalGamepad` est maintenant une dépendance : la manette qui
    // arrive range le pad, celle qu'on débranche le rend.
    useEffect(() => {
        if (drivesPad) {
            switchesStore.setValues({
                isControllerVisible: !hasPhysicalGamepad,
                isChainButtons300: true,
            });
        }
    }, [drivesPad, hasPhysicalGamepad, switchesStore]);

    // THIS holder's prompt for a pending hand-over demand (the child pushes
    // `pendingHandover` in its state). Accept grants it, decline denies it —
    // both are intents to the child, and the role flip rides its next state
    // push either way.
    const slotRequestPromptShownRef = useRef(null);
    useEffect(() => {
        const request = slotRequests[0];
        if (!request || isWatching) {
            return;
        }
        if (slotRequestPromptShownRef.current === request.from) {
            return;
        }
        slotRequestPromptShownRef.current = request.from;
        // DEUX GESTES (#274) : le propriétaire PRÊTE — il garde sa place et
        // reprend la manette quand il veut ; un invité DONNE sa place.
        const lending = myRole === "owner";
        context.toast.open(
            t(lending ? "streaming.handover.lendPrompt" : "streaming.handover.controlPrompt", { username: nameOf(request.from) }),
            {
                variant: "confirm",
                title: t("streaming.handover.title"),
                confirmLabel: t(lending ? "streaming.handover.lend" : "streaming.handover.accept"),
                cancelLabel: t("streaming.handover.decline"),
                onConfirm: () => {
                    slotRequestPromptShownRef.current = null;
                    sessionSend({ type: "cedeslot" });
                    inputRef.current?.onSlotGiveaway?.();
                },
                onCancel: () => {
                    slotRequestPromptShownRef.current = null;
                    sessionSend({ type: "slotrequestdenied" });
                },
            }
        );
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [slotRequests, isWatching]);


    // Below — each FloatingPanel switch actually doing something, instead
    // of just sitting in local state. Kept as one small effect per switch
    // (mirrors how each one's target is independent — global preference,
    // browser API, per-session extraProps, or a server call) rather than
    // one giant effect keying off the whole `switches` object.

    // Fullscreen the SESSION, not the document. The document carries the
    // app chrome around the picture; what the player asked to fill the
    // screen is the stream and the things laid over it. Not the <video>
    // element either: the on-screen pad, the FloatingPanel and every
    // overlay are its SIBLINGS, so promoting the video alone would take
    // fullscreen and leave the controls behind it.
    useEffect(() => {
        const surface = shellRef.current;
        if (!surface) {
            return;
        }
        if (switches.isFullscreen) {
            if (!isFullscreen()) {
                requestFullscreen(surface).catch(() => { });
            }
        } else if (isFullscreen()) {
            exitFullscreen().catch(() => { });
        }
    }, [switches.isFullscreen]);

    // The browser can leave fullscreen without us: Escape, the window
    // manager, a navigation. Nothing listened, so the switch stayed lit
    // over a windowed page — the page's copy of the truth and the
    // browser's, apart again. The browser is the authority here.
    // Tourner l'écran — voir `use-screen-rotation.js`. La page de test des
    // manettes utilise le MÊME hook : c'est le même besoin, sur la même
    // surface. L'orientation n'est plus un réglage de la partie mais de
    // l'APPAREIL, choisie dans Réglages (#406, libs/screenOrientation.js).
    const screenOrientation = useScreenOrientation();
    const rotateSurface = useScreenRotation(screenOrientation, {
        enabled: isHandheld,
        surfaceRef: shellRef,
        // Le plein écran s'ALLUME et ne s'éteint jamais ici : quelqu'un qui
        // jouait déjà en plein écran et qui tourne son écran ne demande pas
        // à en sortir. C'est un réglage à part, avec son propre interrupteur.
        onFullscreen: () => switchesStore.setValues({ isFullscreen: true }),
    });

    useEffect(() => {
        const sync = () => switchesStore.setValues({ isFullscreen: isFullscreen() });
        for (const event of ["fullscreenchange", "webkitfullscreenchange", "MSFullscreenChange"]) {
            document.addEventListener(event, sync);
        }
        return () => {
            for (const event of ["fullscreenchange", "webkitfullscreenchange", "MSFullscreenChange"]) {
                document.removeEventListener(event, sync);
            }
        };
    }, [switchesStore]);

    useEffect(() => {
        if (switches.isChromeCasting) {
            if (getCurrentCastSession()) {
                sendSessionIdToReceiver(session.id).catch(() => { });
                return;
            }
            requestCastSession()
                .then(() => sendSessionIdToReceiver(session.id))
                .catch((error) => {
                    context.toast.open(error.message || t("streaming.settings.chromecast.failed"), { variant: "error" });
                    setSwitches((prev) => ({ ...prev, isChromeCasting: false }));
                });
        } else if (getCurrentCastSession()) {
            endCastSession();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [switches.isChromeCasting]);

    useEffect(() => {
        // Guarded: these two write to localStorage and OUTLIVE the session.
        // While setValues still replaced the store instead of merging, this
        // effect ran with `undefined` every time a player got the controller
        // back and persisted rumble=false — the switch fix does not undo
        // that, the user has to re-enable it in Settings. Never write
        // anything that is not an explicit boolean again.
        if (typeof switches.isRumbleEnabled !== "boolean") {
            return;
        }

        setRumbleEnabled(switches.isRumbleEnabled);
    }, [switches.isRumbleEnabled]);

    useEffect(() => {
        if (typeof switches.isHapticFeedback !== "boolean") {
            return;
        }

        setHapticFeedback(switches.isHapticFeedback);
    }, [switches.isHapticFeedback]);

    // streamex-only (see use-streamex-input.js's own doc — the child
    // already defaults to 300ms on connect, this just lets the user widen
    // it back down to keytest's raw 50ms). extraProps is null on platforms
    // that have none (remoteplay) — the contract says so, so guard the
    // object itself, not just the callback.
    useEffect(() => {
        input.extraProps?.onChainWindowChange?.(switches.isChainButtons300 ? 300 : 100);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [switches.isChainButtons300]);

    // retro-gaming PS1 only.
    useEffect(() => {
        if (!input.extraProps?.PS1) {
            return;
        }
        if (switches.isDualShockController) {
            input.extraProps.PS1.dualshock.load();
        } else {
            input.extraProps.PS1.standard.load();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [switches.isDualShockController]);

    // retro-gaming only.
    useEffect(() => {
        input.extraProps?.timeTravelProps?.setTravelMode(
            switches.isRewinding ? "rewind" : switches.isFastForwarding ? "fastForward" : null,
        );
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [switches.isRewinding, switches.isFastForwarding]);

    // statsResult is computed on EVERY render, ready or not — the stats()
    // closure calls useSessionStats (a hook), and a render that skipped it
    // would change the hook count between the pre-private and ready
    // renders ("Rendered more hooks than during the previous render").
    // onMonitoringResource is the P3-P5 contract name every hook exposes.
    //
    // Un téléphone suit aussi la température et la batterie panneau fermé :
    // ses deux pastilles, à côté du bouton du menu, les montrent.
    const statsResult = input.onMonitoringResource(switches.isResourceMonitoringVisible, isHandheld);
    const handPower = statsResult?.device;

    // The controller (and every menu below) needs the real role, so it waits
    // for the kernel's first private push — but the VIDEO never does: the
    // SessionStream below is mounted unconditionally, from the first render to
    // unmount. It used to be swapped out for a second <GatewayWebRTC> at this
    // exact point, which is what made the picture blink on arrival and on every
    // Watch → Streaming navigation.
    // ONE derivation, read by the overlay and by the redirect below. claiming
    // is "I asked for the pad and the authority has not answered yet" — the
    // window that used to look identical to a plain watch, which is why
    // launching a game felt like nothing was happening.
    // The session ended: show it for a beat, then leave. Same destination as
    // the Quit button — a page whose session is over has nothing left to be,
    // and leaving it on a dead screen is how people end up reloading into a
    // session that no longer exists. The pause is for reading the reason, not
    // for the machine.
    useEffect(() => {
        if (!sessionEnded) {
            return undefined;
        }

        const timer = setTimeout(() => onTerminate?.(), gameExited ? GAME_EXITED_REDIRECT_MS : SESSION_ENDED_REDIRECT_MS);
        return () => clearTimeout(timer);
    }, [sessionEnded, gameExited, onTerminate]);

    // A source that stopped sending gets a longer look, then the same exit —
    // see the constants for why leaving costs nothing. Bound to videoSilent
    // alone, so the moment frames advance again the timer is torn down and
    // nobody is taken anywhere.
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
        authorityOpen: facadeOpen,
        authorityAnswered: kernelPrivateReady,
        claiming: wantsPlay && isWatching,
        platformConnected: input.isConnected,
        videoSilent,
        stalling: input.isStalling,
    });

    const controller = kernelPrivateReady ? input.controller() : null;

    // ANNULER, DEPUIS L'ÉCRAN DU LANCEMENT (#287). La partie qu'on vient de
    // lancer s'arrête pour tout le monde — personne n'y joue encore — ; celle
    // de quelqu'un d'autre continue sans nous. Le même chemin que « Quitter »
    // avec « terminer la session ».
    const cancelLaunch = () => {
        if (myRole === "owner") {
            input.terminate?.();
            stopSessionForEveryone(session, token).catch(() => {
                context.toast.open(t("streaming.quit.stopFailed"), { variant: "error" });
            });
        } else {
            input.disconnect?.();
        }
        onTerminate();
    };

    // ── LE MENU ET LA SORTIE (#408, #409) ───────────────────────────────
    //
    // Le porteur, le 2026-10-19 : un raccourci à la manette pour ouvrir le
    // menu, « Quitter » dans le menu, à la fin, et plus de titre à l'écran —
    // partout : webapp et application, avec ou sans manette ou clavier.

    // QUITTER — la même fenêtre qu'avant, ouverte maintenant depuis le menu.
    const openQuit = () => {
        context.modal.open((
            <QuitConfirmModal
                session={session}
                isWatching={isWatching}
                isOwner={myRole === "owner"}
                borrowing={childLoan.canReturn}
                quitStore={quitStore}
                t={t}
                onConfirm={async ({ onQuit, saveStateOnQuit: save }) => {
                    // Three verbs, no boolean flags (2026-08-24) — see
                    // the hooks' own doc. A spectator, and anyone leaving
                    // without ticking "end the session", takes the quiet
                    // path: the session keeps running on its grace for
                    // whoever is still playing.
                    // Owner-only, belt and braces: the
                    // switch is hidden for a joiner, and
                    // a stale store value must not end
                    // somebody else's session either. The
                    // kernel refuses it too (403).
                    const isOwnerHere = !isWatching && myRole === "owner";
                    const endsTheSession = onQuit === "kill" && isOwnerHere;
                    // GELER N'EST POSSIBLE QUE SUR UN CŒUR — et le
                    // magasin d'options est partagé par toutes les
                    // plateformes, donc une valeur retenue d'une partie
                    // rétro peut arriver ici sur une PS5. La demande
                    // partirait alors vers un conteneur streamex ou une
                    // vraie console, qui ne savent pas la satisfaire.
                    // Le sélecteur cache déjà l'option ; ceci est la
                    // ceinture (2026-09-10).
                    const canFreeze = session.platform === "retrogaming"
                        || session.platform === "device";
                    const freezesTheSession = onQuit === "pause" && isOwnerHere && canFreeze;

                    // Saving is its OWN decision now, not a flavour of
                    // ending the session: the player can keep their
                    // progress whether they leave the session running or
                    // shut it down. Awaited so the state is actually
                    // written before the connection goes.
                    if (save && !isWatching && input.saveOnQuit) {
                        await input.saveOnQuit();
                    }

                    // Mark the exit as handled so the
                    // pagehide/unmount guard above does
                    // not disconnect a second time.
                    quitHandledRef.current = true;

                    // GELER AVANT DE PARTIR, et dans cet
                    // ordre : la demande doit atteindre
                    // l'hôte pendant que la connexion
                    // vit encore.
                    if (freezesTheSession && input.extraProps?.pause) {
                        await input.extraProps.pause();
                    }

                    if (endsTheSession) {
                        input.terminate?.();
                    } else {
                        input.disconnect?.();
                    }

                    if (endsTheSession) {
                        stopSessionForEveryone(session, token).catch(() => {
                            context.toast.open(t("streaming.quit.stopFailed"), { variant: "error" });
                        });
                    }

                    onTerminate();
                }}
            />
        ), {
            variant: "normal",
            size: 600,
        });
    };
    // Le menu est un INSTANTANÉ : ses props sont figées à son ouverture
    // (skeleton.js, offcanvas.open). Il appelle la sortie par une référence,
    // pour qu'elle lise le rôle et le prêt du moment, pas ceux de l'ouverture.
    const openQuitRef = useRef(openQuit);
    openQuitRef.current = openQuit;

    const openMenu = () => {
        if (Skeleton.overlayCount() > 0) {
            return;
        }
        context.offcanvas.open(
            <FloatingPanel
                t={t}
                session={session}
                input={input}
                title={input.title}
                controller={controller}
                isWatching={isWatching}
                switchesStore={switchesStore}
                sessionSend={sessionSend}
                players={broadcastPlayers}
                watchers={broadcastWatchers}
                loan={childLoan}
                onQuit={() => openQuitRef.current()}
            />, {
            size: 600,
        });
    };
    const openMenuRef = useRef(openMenu);
    openMenuRef.current = openMenu;

    // UN PANNEAU OUVERT SUSPEND LE JEU, UNE PARTIE EN COURS SUSPEND LA
    // NAVIGATION (#407, #408). Le menu, ses sous-panneaux, la fenêtre de
    // sortie : tant qu'il y en a un, la croix, A et B le servent, et le jeu
    // n'entend rien — chaque plateforme relâche ce qu'elle tenait
    // (libs/gameInput.js). Sans panneau, et la manette en main, c'est
    // l'inverse : le pont manette-clavier et la navigation de Skeleton se
    // taisent, et plus rien n'a le focus — une touche Entrée du jeu
    // n'actionnerait pas un bouton de la page.
    const overlays = Skeleton.useOverlayCount();
    const playing = drivesPad && overlays === 0;
    useEffect(() => {
        setGameInputSuspended(overlays > 0);
        Skeleton.suspendNavigation(playing);
        if (playing) {
            document.activeElement?.blur?.();
        }
    }, [overlays, playing]);
    useEffect(() => () => {
        setGameInputSuspended(false);
        Skeleton.suspendNavigation(false);
    }, []);

    // L2 + HOME TENUS UNE SECONDE (use-streaming-menu.js).
    const menuHold = useMenuHotkey(overlays === 0, () => openMenuRef.current());

    // ÉCHAP OUVRE LE MENU, au clavier. Pris avant tout le monde (fenêtre,
    // phase de capture) et gardé : il ne part plus au jeu — où, sur un
    // émulateur de bureau, il sortait le jeu de son plein écran. Le menu
    // ouvert, il le referme, comme avant (Bootstrap).
    useEffect(() => {
        const onKeyDown = (event) => {
            if (event.key !== "Escape" || !event.isTrusted || Skeleton.overlayCount() > 0) {
                return;
            }
            event.preventDefault();
            event.stopPropagation();
            openMenuRef.current();
        };
        window.addEventListener("keydown", onKeyDown, true);

        return () => window.removeEventListener("keydown", onKeyDown, true);
    }, []);

    const menuButtonVisible = useIdleVisibility(playing);
    const menuButtonStyle = useMenuButtonStyle(Boolean(drivesPad && switches.isControllerVisible && controller?.Component));

    // LE RACCOURCI SE DIT, une fois, à qui joue avec une manette : sans
    // bouton à l'écran, il faut savoir qu'il existe.
    const gamepadTipShownRef = useRef(false);
    useEffect(() => {
        if (!hasPhysicalGamepad || !drivesPad || gamepadTipShownRef.current) {
            return;
        }
        gamepadTipShownRef.current = true;
        context.toast.open(t("streaming.menu.gamepadTip"), { variant: "info", duration: 6000 });
    }, [hasPhysicalGamepad, drivesPad]);

    return (
        <div
            ref={shellRef}
            className="overflow-hidden"
            style={{
                backgroundColor: "#000",
                // 100% once this element IS the fullscreen box: the browser
                // sizes it to the screen exactly, while 100vh is a viewport
                // unit that can still count chrome that is no longer there.
                // Otherwise the visible height: 100vh, or the top half when a
                // clip-on controller hides the bottom one (#410, app.css).
                height: switches.isFullscreen ? "100%" : "var(--gameflex-visible-height)",
                // The fallback rotation, and ONLY while the viewport is
                // actually standing up: if the person turned the device
                // themselves, the goal is already met and turning the surface
                // on top of that would stand the game back up. The box swaps
                // its dimensions (a landscape box on a portrait screen) and is
                // laid back over the viewport — the transform is read left to
                // right, so the rotation happens first and the shift puts it
                // back on screen. Touches follow the transform, so the pad
                // stays under the fingers that see it.
                ...rotationSurfaceStyle(rotateSurface, isPortraitViewport),
            }}
        >
            <SessionStream
                // Le même identifiant de flux que la vignette du tableau de
                // bord : celui du child, qui est ce que le gateway connaît.
                sessionId={session.childSessionId || session.id}
                t={t}
                screenRef={screenRef}
                // The shell is a fixed-height box (and, in fullscreen, THE
                // box) — so the stream fills it and the picture fits inside
                // instead of being sized on width alone and clipped.
                fillHeight
                // Authentication decides the input DataChannel, never the role
                // — see SessionStream's own doc: renegotiating the SDP on a
                // role change is what restarted the video. Both children drop
                // input from a viewer with no assigned pad, so an authenticated
                // spectator holding an idle channel changes nothing.
                // From the STORED TOKEN, not from `user`: the context's user
                // only lands once /api/v1/auth/me answers, and both of these are
                // baked into the WebRTC offer — a first render that believed
                // itself anonymous negotiated a throwaway viewer and a second
                // one ~50ms later (gateway log, 2026-08-24). This is read
                // synchronously on render 1, so exactly one viewer is created.
                hasInput={!!viewerUsername}
                username={viewerUsername}
                // Le lancement d'où vient la page : l'intro se joue avec le
                // fond de son écran, et non une seconde fois (chantier 35).
                launch={launchId}
                muted={!switches.isSoundEnabled}
                onAutoplayForcedMute={() => switchesStore.setValues({ isSoundEnabled: false })}
                onVideoSilent={setVideoSilent}
                onQuality={(state) => {
                    switchesStore.setValues({
                        videoAvailable: state.available, videoCurrent: state.current, videoWant: state.want,
                    });
                    if (!introToldRef.current) {
                        setIntroLive(true);
                    }
                }}
                onIntro={(phase, game, expect) => {
                    introToldRef.current = true;
                    setIntroPhase(phase);
                    setIntroLive(phase === "live");
                    setGameLoading(phase === "hold" && game === "loading");
                    setSlowStart(phase === "hold" && expect === "minutes");
                }}
                onViewerReady={() => {
                    setViewerReady(true);
                    // L'ATTACHE DE LA PASSERELLE, DITE À L'ÉCRAN (#195).
                    //
                    // Le kernel ne la voit jamais : le navigateur négocie
                    // directement avec `gateway-webrtc`, qui ignore le
                    // `launchId`. Cette étape-là ne peut donc venir que d'ici —
                    // c'est le seul endroit qui connaît les deux. Elle comble
                    // le trou entre « l'émulateur démarre » et la première
                    // image, qui n'avait aucun nom jusqu'ici.
                    if (!gatewayStepSaidRef.current) {
                        gatewayStepSaidRef.current = true;
                        noteChildStep("gateway.attached");
                    }
                    // chantier-4: a tab holds TWO identities —
                    // a gateway viewer (video and input) and a
                    // child socket (presence and intents).
                    // Only the tab knows both, so it is the tab
                    // that pairs them; without this the child
                    // cannot tell whose keystrokes are whose
                    // and gates nothing.
                    childSocket.announceViewer(screenRef.current?.viewerId);
                }}
                // Never give up while the page is open: a session whose child
                // has not attached yet (owner still loading the game) must pick
                // the picture up whenever it finally starts.
                maxAttempts={null}
                // On s'intercale sur `onFirstPicture` sans le voler à la
                // plateforme : elle en a son propre usage (remote-play y
                // accroche sa détection de gel).
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
                // The platform's own progress ("fetching the device",
                // "waking up", "waiting for ready"...) — for remote-play
                // that IS the whole feedback while the PS5 boots. Rendering
                // the picture surface unconditionally dropped it; a generic
                // "connecting" spinner for a 30s console wake tells the user
                // nothing (reported live 2026-08-24).
                showOverlay={phase !== "live"}
                // The sentinel when there is nothing left to wait for, the
                // translated phase otherwise — passing the key itself would
                // render it raw.
                connectingLabel={phase === "live" ? OVERLAY_HIDDEN : t(`streaming.connection.phase.${phase}`)}
            />

            {/* LA SOURIS, POUR LES PLATEFORMES QUI ONT UN BUREAU DERRIÈRE.
                Une surface transparente par-dessus la vidéo, et non des
                gestes posés sur le <video> : celui-ci appartient à
                GatewayWebRTC, qui le monte et le démonte quand la
                négociation le demande.

                Le hook la fournit ou non — streamex seul aujourd'hui, et
                seulement pour qui joue. « Absent = pas de section », comme
                partout ailleurs dans ce contrat. Pas pour une console à
                écran tactile (la Switch) : le doigt y est un clic absolu,
                posé sur le <video> par useTouchScreen, que cette surface
                recouvrirait. */}
            {input.extraProps?.pointerHandlers && !hasTouchScreen && (
                <div
                    className="position-absolute top-0 start-0 w-100 h-100"
                    // SOUS la manette (1002) et sous la barre du bas (1003).
                    // Les deux sont plus loin dans le DOM, donc à z-index égal
                    // elles gagneraient déjà — mais faire dépendre « le pad
                    // reste cliquable » de l'ordre des balises est une règle
                    // qu'on casse en déplaçant un bloc. On le dit.
                    //
                    // `touchAction: none` : sans ça le navigateur interprète
                    // un glissement comme un défilement et ne livre plus les
                    // `pointermove`.
                    style={{ zIndex: 1000, touchAction: "none" }}
                    {...input.extraProps.pointerHandlers}
                />
            )}

            {/* LE BOUTON DU MENU (#409) — la barre du bas (Quitter, titre,
                Réglages) n'est plus. Le porteur, le 2026-10-19 : « Quitter »
                passe DANS le menu, à la fin, et le titre quitte l'écran (il est
                en tête du menu). Reste un bouton rond, dans le coin en bas à
                gauche depuis le 2026-10-22 (useMenuButtonStyle), qui s'efface
                pendant qu'on joue et revient au premier geste
                (useIdleVisibility). Une manette l'ouvre par L2 + Home tenus,
                un clavier par Échap. */}
            <bs.Button
                variant="dark"
                className="position-absolute rounded-circle d-flex align-items-center justify-content-center p-0"
                style={{
                    ...menuButtonStyle,
                    opacity: menuButtonVisible ? 1 : 0,
                    pointerEvents: menuButtonVisible ? "auto" : "none",
                    transition: "opacity 300ms ease",
                }}
                title={t("streaming.menu.open")}
                aria-label={t("streaming.menu.open")}
                onClick={openMenu}
            >
                <i className="bi bi-list fs-4 text-white"></i>
            </bs.Button>
            {/* LA TEMPÉRATURE ET LA BATTERIE, DANS DEUX CERCLES À CÔTÉ DU BOUTON
                DU MENU — sur un appareil qu'on tient en main (le porteur,
                2026-10-28). Celles du téléphone qu'on tient (`device`), dans
                l'application seulement : un navigateur n'en a pas (le porteur,
                2026-10-29). Elles s'effacent avec le bouton pendant qu'on joue
                (#409) et reviennent avec lui. */}
            {isHandheld && (handPower?.temperatureCelsius != null || handPower?.batteryPercent != null) && (
                <div
                    className="position-absolute d-flex gap-2"
                    style={{
                        ...menuButtonStyle,
                        left: `calc(${menuButtonStyle.left} + 56px)`,
                        width: "auto",
                        background: "none",
                        border: "none",
                        opacity: menuButtonVisible ? 1 : 0,
                        pointerEvents: "none",
                        transition: "opacity 300ms ease",
                    }}
                >
                    {[
                        { value: handPower.temperatureCelsius, unit: "°", icon: "bi-thermometer-half", label: "streaming.menu.temperature" },
                        {
                            value: handPower.batteryPercent,
                            unit: "%",
                            icon: handPower.batteryPercent >= 80 ? "bi-battery-full" : handPower.batteryPercent >= 30 ? "bi-battery-half" : "bi-battery",
                            label: "streaming.menu.battery",
                        },
                    ].filter(({ value }) => typeof value === "number" && Number.isFinite(value)).map(({ value, unit, icon, label }) => (
                        <div
                            key={label}
                            className="rounded-circle d-flex flex-column align-items-center justify-content-center text-white"
                            style={{
                                width: menuButtonStyle.width,
                                height: menuButtonStyle.height,
                                background: menuButtonStyle.background,
                                border: menuButtonStyle.border,
                                fontSize: "0.7rem",
                                lineHeight: 1.1,
                            }}
                            title={t(label)}
                            aria-label={`${t(label)} ${Math.round(value)}${unit}`}
                        >
                            <i className={`bi ${icon}`}></i>
                            <span className="fw-semibold">{Math.round(value)}{unit}</span>
                        </div>
                    ))}
                </div>
            )}
            {/* L'APPUI LONG SE VOIT : sans rien voir, on lâche avant la fin.
                Rien pendant le premier dixième — un L2 + Home effleuré ne fait
                pas clignoter l'écran. */}
            {menuHold >= 0.1 && (
                <div
                    className="position-absolute top-50 start-50 translate-middle text-white text-center rounded-3 px-4 py-3"
                    style={{ zIndex: 1004, background: "rgba(0, 0, 0, 0.7)", minWidth: "260px", pointerEvents: "none" }}
                >
                    <div className="fw-semibold mb-2">{t("streaming.menu.holdHint")}</div>
                    <bs.ProgressBar now={Math.round(menuHold * 100)} style={{ height: "6px" }} />
                </div>
            )}

            {kernelPrivateReady && (
                // Collée au bas de l'écran depuis que la barre n'y est plus
                // (#409). Marquée : un doigt sur la manette à l'écran ne
                // réveille pas le bouton du menu.
                <div className="flex-shrink-0 position-absolute" data-streaming-strip="" style={{
                    bottom: "0",
                    left: "0",
                    width: "100%",
                    zIndex: 1002,
                }}>
                    {drivesPad && switches.isControllerVisible && controller?.Component && controller.Component}
                    {!isWatching && !drivesPad && childLoan.canReclaim && (
                        <div className="p-3 text-center text-white small">
                            <p>{t("streaming.handover.lentNotice", { username: nameOf(childLoan.lentTo) })}</p>
                            <div className="d-flex justify-content-center gap-2">
                                <bs.Button variant="light" onClick={() => sessionSend({ type: "reclaimslot" })}>
                                    {t("streaming.handover.reclaim")}
                                </bs.Button>
                            </div>
                        </div>
                    )}
                    {isWatching && !drivesPad && (
                        <div className="p-3 text-center text-white small">
                            {/* "You can see the stream" is a lie while nobody
                                holds a pad: in v4 the emulator has not started,
                                so the black screen is a session that has not
                                begun, not one that failed. Say which. */}
                            {broadcastPlayers.length === 0 && session.owner == user.username ? (
                                <div>
                                    <p>{t("streaming.watch.noticeIdle")}</p>
                                    <div className="d-flex justify-content-center gap-2">
                                        <bs.Button
                                            variant="light"
                                            onClick={() => sessionSend({ type: "requestslot" })}
                                        >
                                            {t("streaming.settings.join.action")}
                                        </bs.Button>
                                    </div>
                                </div>
                            ) : (
                                <div>{t("streaming.watch.notice")}</div>
                            )}
                        </div>
                    )}
                </div>
            )}

            {/* LE VOILE À 50 %, ET UNE OMBRE SOUS LE TEXTE. Il était à 65 %
            (#11) : le panneau est posé sur une vidéo dont la luminosité varie,
            le pire cas est une image blanche, et à 30 % les libellés y
            tombaient à ≈ 1,5:1 de contraste. Le porteur l'a voulu plus
            transparent le 2026-10-28, sur toute la largeur : à 50 %, l'ombre
            portée détache chaque caractère de ce qui reste d'image claire
            derrière lui. Le thème n'y change rien : ce sont des blancs fixes,
            voulus ici puisque le fond est toujours la vidéo. */}
            {switches.isResourceMonitoringVisible && statsResult && (
                <div className="position-absolute text-white p-3" style={{
                    top: "0",
                    left: "0",
                    width: "100%",
                    maxHeight: "100%",
                    zIndex: 1001,
                    backgroundColor: "rgba(0, 0, 0, 0.5)",
                    textShadow: "0 1px 2px rgba(0, 0, 0, 0.9)",
                    overflowY: "auto",
                }}>
                    <StreamingStatsPanel stats={statsResult} session={session} t={t} />
                </div>
            )}

            {/* LES ÉTAPES SONT UN ÉCRAN, PAS UN CALQUE (#191).
                Elles passaient par l'overlay d'attente de `SessionStream`, qui
                est transparent et posé sur la vidéo : constaté en vrai sur le
                téléphone, la liste se dessinait PAR-DESSUS une partie de Crash
                Team Racing déjà en train de tourner, textes superposés et
                manette visible au travers. Demande de l'utilisateur, et elle
                est juste : un écran à part, avant le jeu.

                DONC OPAQUE, ET AU-DESSUS DE TOUT — z-index supérieur à la
                manette (1002) et à la barre du bas (1003), fond noir plein.

                ET IL DISPARAÎT SUR LE BON SIGNAL : la première image ET la
                manette prête (`kernelPrivateReady` = l'enfant a parlé), et non
                plus `input.isConnected`, qui ne dit que l'état de la
                PeerConnection — c'est précisément pour ça que les étapes
                restaient affichées sur un jeu qui tournait.

                Le flux reste MONTÉ dessous, et c'est indispensable : le
                démonter empêcherait la négociation, donc la première image,
                donc la bascule. Cet écran cache, il ne remplace pas. */}
            {/* LA CONDITION EST « UN LANCEMENT EST EN COURS », PAS « J'AI REÇU
                UNE ÉTAPE ».
                Premier jet : j'attendais qu'au moins une étape soit arrivée.
                Or elles viennent par WebSocket — poignée de main, puis rejeu de
                l'historique : pendant ces premières centaines de millisecondes
                il n'y en a AUCUNE, donc rien ne couvrait l'écran et on voyait
                la coquille nue — la manette, la barre du bas et l'ancien
                « En attente de la vidéo… ». Photographié par l'utilisateur.
                La présence d'un `launchId` suffit à savoir qu'une partie
                démarre, et elle est connue AVANT le premier message. L'écran
                paraît donc tout de suite, avec son titre et son tourniquet
                (`StreamingConnectingOverlay` s'en charge quand la liste est
                vide), et se remplit à mesure. */}
            {/* ET EN SPECTATEUR, OÙ IL N'Y A PAS DE `launchId`.
                Un spectateur n'a lancé aucun jeu : rien ne lui en donne, donc
                cette condition ne se déclenchait jamais pour lui — mesuré le
                2026-09-18 sur une vraie partie, zéro étape et l'image arrivant
                nue à 1,3 s. `isWatching` le couvre, mais il ne peut PAS choisir
                le CONTENU : il vaut vrai au premier rendu de TOUTE session
                serveur et ne devient faux qu'au message privé du kernel — un
                propriétaire verrait donc un instant les étapes du spectateur.
                C'est `launchId` qui départage le contenu : lui est connu dès
                l'URL et ne change jamais. Et le titre suit : « Démarrage de la
                partie… » serait faux devant une partie qui tourne déjà.

                ET LA SORTIE AUSSI DÉPEND DE `launchId`, pour la même raison.
                Premier jet : j'ai gardé `sawPicture && kernelPrivateReady`
                pour tout le monde. Or `kernelPrivateReady` attend le message
                privé du kernel, et le WebSocket qui le porte — la façade,
                `/api/v1/facade/ws?token=…` dans `SessionPool.js` — est gardée
                par un `if (!token) return` et met le jeton dans son URL.
                Un spectateur ANONYME — un lien de partage ouvert par quelqu'un
                qui n'a pas de compte, le cas que `isPublicSpectate` autorise
                exprès — ne l'obtient donc JAMAIS : mesuré, l'image arrivait à
                1,8 s et l'écran restait posé dessus indéfiniment. J'avais
                réutilisé la moitié « manette prête » d'une condition écrite
                pour un joueur, chez quelqu'un qui n'aura jamais de manette.
                Avec un `launchId` on attend les deux ; sans, l'image suffit. */}
            {/* ET L'IMAGE ATTENDUE EST CELLE DU JEU (chantier 35) : l'intro
                passe d'abord, sur le fond de cet écran — celui du lancement
                pour qui l'a lancé, sa propre vidéo pour qui la rejoint. */}
            {(launchId || isWatching) && !(sawPicture && introLive && (kernelPrivateReady || !launchId)) && (
                <LaunchScreen
                    t={t}
                    zIndex={1010}
                    label={launchId ? t("streaming.launch.title") : t("streaming.connection.state.connecting")}
                    steps={[
                        ...(launchId ? [...launchSteps, ...childSteps] : WATCH_LAUNCH_STEPS),
                        ...(introStep ? [{ step: introStep }] : []),
                    ]}
                    backdrop={launchId
                        ? { launch: { streamId: launchId, username: viewerUsername } }
                        : (shellStream ? { stream: shellStream } : null)}
                    onCancel={cancelLaunch}
                />
            )}
            {/* L'INTRO TENUE, PAR TOUT AUTRE CHEMIN (#520) : une page de
                partie ouverte sans lancement ni « Regarder » — rouverte depuis
                l'accueil de l'application, par exemple — voyait l'intro figée
                sans un mot. La même ligne, seule, par-dessus la vidéo ; les
                touches passent au travers, jusqu'à la manette à l'écran. */}
            {!(launchId || isWatching) && introStep && (
                <div className="position-absolute top-0 start-0 w-100 h-100 pe-none" style={{ zIndex: 1010 }}>
                    <LaunchBar t={t} label={t(`streaming.connection.step.${introStep}`)} steps={[{ step: introStep }]} />
                </div>
            )}
        </div >
    );
}

// Route wrapper — the single entry point for all 3 platforms, both play
// and watch. No ?role= / ?mode= of any kind: whether you land as owner,
// player or spectator is decided entirely server-side, from session.me.role
// (enrichSessions, registry_routes.go — the kernel already knows who
// started the session and who's already joined) —
//   - session.me.role === "owner"  → you started it
//   - session.me.role === "player" → you already joined (reconnect)
//   - anything else (spectate, or not authenticated at all)  → watching
// A client-supplied role was never anything but a UX hint anyway; letting
// it decide anything was the exact hole retro_relay.go's own identity
// check just closed.

export default function Streaming() {
    const navigate = useNavigate();
    const context = useContext(Skeleton.context);
    const { t } = useTranslation();
    const { user, token: authToken } = useAuth();
    const [searchParams, setSearchParams] = useSearchParams();
    // Only ever a REAL session id: the gamepad-test page is its own route
    // with its own surface (KeytestLive.jsx), and nothing here knows about it.
    const rawSessionId = searchParams.get("sessionId");
    // See StreamingShell's play-intent effect. Read LIVE, never latched at
    // mount — that was the bug behind "I launch a game and I am still in
    // watch" (2026-08-25). /streaming?sessionId=A and ?sessionId=B are the
    // SAME route, so React Router re-renders this component instead of
    // remounting it: a value captured at mount survives every later launch.
    // Landing here once without the flag (a refresh on this page, a watch
    // link) then poisoned every SPA launch for the rest of the session.
    //
    // Safe to read live because the double-spend guard is playIntentRef in
    // the shell, which is keyed to the session by StreamingShell's own
    // key={session.id} — not this value.
    const wantsPlay = searchParams.get("play") === "1";
    const dropPlayIntent = useCallback(() => {
        setSearchParams((params) => {
            params.delete("play");
            return params;
        }, { replace: true });
    }, [setSearchParams]);

    // LE LANCEMENT EN COURS (#191).
    //
    // On arrive ici AVANT que la session existe : la page des jeux a fabriqué
    // un identifiant, ouvert ce canal, posté, et navigué aussitôt. Les étapes
    // arrivent donc pendant qu'on regarde, au lieu de se dérouler derrière un
    // bouton qui tourne sur l'écran précédent.
    const launchId = searchParams.get("launchId");
    const { steps: launchSteps, failure: launchFailure, sessionId: launchedSessionId } =
        useLaunchProgress(launchId, authToken);

    // LE `sessionId` VIENT DU CANAL, PAS DE LA RÉPONSE HTTP — c'est ce qui
    // permet de naviguer sans attendre. Dès qu'il arrive, il entre dans l'URL :
    // tout le reste de cette page part de là, et un rafraîchissement retombe
    // sur la session plutôt que sur un lancement terminé.
    useEffect(() => {
        if (!launchedSessionId || rawSessionId) {
            return;
        }
        setSearchParams((params) => {
            params.set("sessionId", launchedSessionId);
            params.delete("launchId");

            return params;
        }, { replace: true });
    }, [launchedSessionId, rawSessionId, setSearchParams]);

    const [reportState, setReportState] = useState(null);
    const sendReport = useCallback(async () => {
        setReportState("sending");

        const result = await reportLaunchFailure({
            context: `lancement ${launchId || "?"}`,
            steps: launchSteps,
            detail: launchFailure,
        });

        setReportState(result.ok ? "sent" : "failed");
        context.toast.open(
            result.ok ? t("streaming.launch.reported") : t("streaming.launch.reportFailed"),
            { variant: result.ok ? "success" : "error" },
        );
    }, [launchId, launchSteps, launchFailure, context, t]);
    // LA PARTIE D'UN AUTRE APPAREIL — le seul cas qui ne se résout pas dans
    // le pool (C7-P4).
    //
    // Un invité ouvre un lien vers un téléphone qui n'est pas le sien : la
    // session vit là-bas, dans le pool de LÀ-BAS. Il n'y a rien à chercher
    // ici, et tout ce qu'il faut savoir tient dans l'adresse.
    //
    // La partie du téléphone LUI-MÊME, elle, ne passe plus par ici (C8-P3) :
    // elle a maintenant un identifiant comme les autres et le kernel la
    // publie sur la façade. La page la reçoit, au lieu de la fabriquer.
    const guestSession = useMemo(() => remoteSessionFrom(searchParams), [searchParams]);

    // UNE SESSION QUI VIT CHEZ UN SERVEUR FÉDÉRÉ (chantier 9, C9-P4c).
    //
    // Elle se reconnaît à son identifiant préfixé — `srv4:…` — comme un jeu
    // fédéré depuis le chantier 6. Ce préfixe dit CHEZ QUI, et rien de plus :
    // l'adresse et le jeton se demandent à notre propre kernel, ils ne
    // voyagent pas dans l'URL. Une adresse qu'on nous tend récolterait le
    // jeton de compte qu'on s'apprête à présenter.
    //
    // Différence de fond avec le chemin invité juste au-dessus : celui-là
    // FABRIQUE sa session, faute d'un compte pour la demander. Ici on en a
    // un, donc on interroge la façade DE CE SERVEUR — et la règle du
    // chantier 8 (la page ne fabrique pas ce qu'elle doit recevoir) se suit
    // toute seule.
    const [federatedHost, setFederatedHost] = useState(null);
    const federatedId = useMemo(() => parseFederatedId(rawSessionId), [rawSessionId]);

    useEffect(() => {
        if (!federatedId) {
            setFederatedHost(null);

            return undefined;
        }

        let cancelled = false;
        federatedHostOf(rawSessionId).then((found) => {
            if (!cancelled) {
                setFederatedHost(found);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [rawSessionId, federatedId]);

    // Tant que cette page montre une partie d'ailleurs, TOUTES ses URL visent
    // l'hôte — API, gateway, façade — et son jeton part avec. La surcharge
    // vit exactement le temps de cette page ; les autres écrans restent sur
    // notre propre kernel.
    useEffect(() => {
        if (!federatedHost?.link) {
            return undefined;
        }

        setStreamBase(
            federatedHost.link, federatedHost.token, federatedHost.username,
        );

        return () => clearStreamBase();
    }, [federatedHost]);

    // L'IDENTIFIANT TEL QUE L'HÔTE LE CONNAÎT. Le préfixe est à nous : chez
    // lui, cette session s'appelle autrement, et lui envoyer notre écriture
    // ne rendrait jamais rien.
    //
    // Null tant que l'hôte n'est pas résolu — sinon la recherche partirait
    // vers NOTRE façade, qui ne connaît pas cette session, et le compte à
    // rebours d'échec démarrerait sur une question mal posée.
    const lookupSessionId = federatedId
        ? (federatedHost ? federatedId.id : null)
        : rawSessionId;
    // CE QUI DESCEND, s'il descend quelque chose. Lancer un jeu distant
    // commence par le télécharger : l'attente de la vidéo EST ce transfert, et
    // c'est la seule chose qui distingue « ça travaille » de « c'est mort ».
    const downloadProgress = useDownloadProgress();
    // L'HÔTE PASSE DANS L'APPEL. Une session fédérée n'est pas dans notre
    // pool sous le nom que l'hôte lui donne : il faut la lui demander, avec
    // son adresse et son jeton — pas avec les nôtres, et pas via un état
    // global (voir la note de `useResolvedSession`). Le retrait de cet appel,
    // le 2026-09-07, avait fermé le dernier chemin qui résolvait une session
    // d'ailleurs : la page tombait sur « introuvable » au bout de cinq
    // secondes, pour une session qui tournait.
    const { session: resolvedSession, isRequestTimeout } = useResolvedSession(
        guestSession ? null : lookupSessionId,
        {
            host: federatedHost?.link
                ? { origin: federatedHost.link, token: federatedHost.token }
                : null,
            waitingOn: downloadProgress,
        },
    );
    const session = guestSession || resolvedSession;
    // C7-P2 : sur le téléphone il n'y a pas de compte, mais la façade v4
    // vérifie des JWT — le kernel en frappe un pour l'appareil lui-même,
    // lu une fois à l'arrivée sur la page. Un invité (C7-P4) demande le sien
    // à l'hôte, sur la surface fédération ouverte.
    const [deviceToken, setDeviceToken] = useState(null);
    useEffect(() => {
        if (!guestSession) {
            setDeviceToken(null);

            return;
        }

        // L'invité se PRÉSENTE : son nom d'appareil part avec la demande de
        // jeton, et devient son nom dans la file de l'hôte. Sans lui, le
        // détenteur de la manette lisait « guest-a3f1 demande la manette »
        // et n'avait aucun moyen de savoir qui c'était (2026-09-01).
        const endpoint = guestSession.guest
            ? `${guestSession.host}/api/v1/federation/guest-token?name=${encodeURIComponent(deviceDisplayName())}`
            : apiUrl("/api/v1/device/token");

        let cancelled = false;
        fetch(endpoint, { cache: "no-store" })
            .then((response) => response.json())
            .then((payload) => {
                if (!cancelled) {
                    setDeviceToken(payload?.token || null);
                }
            })
            .catch(() => {
                if (!cancelled) {
                    setDeviceToken(null);
                }
            });

        return () => {
            cancelled = true;
        };
    }, [guestSession]);
    // Chez un hôte fédéré, c'est SON jeton qui vaut — celui du compte qu'on y
    // a ouvert (C9-P3). Le nôtre n'y veut rien dire.
    const token = guestSession
        ? deviceToken
        : (federatedHost?.token || authToken);
    // ET SON NOM, POUR LA MÊME RAISON (#235). Tout ce que la page compare à
    // `user.username` vient de l'hôte : les joueurs que l'enfant annonce, le
    // propriétaire, la cession de manette qu'on nous propose. Notre nom
    // d'ici n'y figure jamais quand il diffère de celui qu'on porte là-bas :
    // le propriétaire qui ouvrait sa partie par un lien partagé se voyait
    // dire « vous regardez cette session », sans moyen de la reprendre (vu
    // le 2026-10-02 entre la staging et la production). Sans compte chez
    // l'hôte, ce nom est vide : on y regarde, et c'est la vérité.
    const shellUser = useMemo(
        () => (federatedHost ? { ...user, username: federatedHost.username || "" } : user),
        [federatedHost, user],
    );

    // Tant qu'une partie d'invité est affichée, TOUTES les URL de la page
    // (API, gateway, façade v4) visent l'hôte. La surcharge vit exactement
    // le temps de cette page — les autres écrans de l'appareil restent sur
    // son propre kernel.
    useEffect(() => {
        if (!guestSession?.guest) {
            return undefined;
        }

        setStreamBase(guestSession.host);

        return () => clearStreamBase();
    }, [guestSession]);
    const { reportIdle, reportPlaying } = usePresence();

    // v1 (chantier-3) — LEAVING THE PAGE IS LEAVING THE SESSION, and this is
    // the only component whose lifetime means that. StreamingShell unmounts
    // whenever `session` momentarily resolves to null (see the early return
    // below) — binding the departure there cut the relay ~5 s after every
    // connect and killed the session behind it.
    //
    // pagehide covers the real page going away (refresh, tab close, mobile);
    // this component's own unmount covers SPA navigation out of /streaming.
    // Always disconnect(), never terminate(): leaving keeps the session
    // running for whoever else is playing. StreamingShell publishes its
    // disconnect into leaveRef and guards against firing twice.
    // v1 R2 — A RELOAD OF THIS PAGE LANDS ON THE DASHBOARD. Reloading
    // dropped the pad (the departure below fires on the way out), so coming
    // back to /streaming would show a watch view of a game you were just
    // playing, with no obvious way back. The Dashboard is where the way back
    // is.
    const leaveRef = useRef(null);
    useEffect(() => {
        const leaveSession = () => leaveRef.current?.();

        window.addEventListener("pagehide", leaveSession);

        return () => {
            window.removeEventListener("pagehide", leaveSession);
            leaveSession();
        };
    }, []);

    // The shell owns the façade WS and isWatching now (P2 §1: the socket
    // drives the platform connection, so it lives next to the input hook) —
    // the parent only needs the watching flag for the terminate
    // bookkeeping, kept in a ref.
    const watchingRef = useRef(false);
    const handleWatchingChange = useCallback((watching) => {
        watchingRef.current = watching;
    }, []);

    const [terminateReason, setTerminateReason] = useState(null);

    const onTerminate = () => {
        if (watchingRef.current) {
            reportIdle();
        }

        const backParams = new URLSearchParams();

        // `source` : le serveur fédéré choisi dans la liste (chantier 14).
        for (const key of ["page", "q", "source"]) {
            if (searchParams.has(key)) {
                backParams.set(key, searchParams.get(key));
            }
        }

        const platform = session?.platform?.toLowerCase();
        const consoleId = session?.consoleId?.toLowerCase();
        // LA ROUTE DE LA LISTE, PAS LE NOM DE LA PLATEFORME (2026-10-02). Une
        // session de jeu à distance porte le TYPE de la console (`ps5`) alors
        // que sa page est `/remoteplay/rp-ps5` ; une partie du téléphone porte
        // la plateforme `device`, qui n'a pas de page — ses jeux sont ceux de
        // `/retrogaming/<console>`. Les deux tombaient sur la route par défaut,
        // donc sur l'accueil, au lieu de la liste d'où l'on venait.
        const listPath = !platform || !consoleId
            ? null
            : platform === "remoteplay"
                ? `/remoteplay/${consoleId.startsWith("rp-") ? consoleId : `rp-${consoleId}`}`
                : platform === "device"
                    ? `/retrogaming/${consoleId}`
                    : `/${platform}/${consoleId}`;
        navigate(listPath ? `${listPath}${backParams.toString() ? `?${backParams.toString()}` : ""}` : "/");
    };

    useEffect(() => {
        // La partie d'un autre appareil n'a rien à résoudre ici : elle vit
        // dans le pool de son hôte, et l'adresse porte déjà tout.
        if (guestSession) {
            return;
        }
        if (!rawSessionId) {
            setTerminateReason(t("streaming.noSessionId"));
            return;
        }
        // UN JEU QUI DESCEND N'EST PAS UNE SESSION INTROUVABLE — et c'est
        // `waitingOn`, passé à la recherche elle-même, qui tient l'horloge
        // arrêtée pendant ce temps (voir useResolvedSession). Ici on ne fait
        // plus qu'afficher le verdict quand il tombe.
        if (isRequestTimeout) {
            setTerminateReason(t("streaming.connection.state.sessionLookupTimeout"));
        }
    }, [guestSession, rawSessionId, isRequestTimeout, t]);

    // Pinch-zoom/double-tap-zoom fights with touch input meant for the
    // virtual controller/video area.
    useEffect(() => {
        const preventGesture = (e) => e.preventDefault();
        document.addEventListener("gesturestart", preventGesture);
        document.addEventListener("gesturechange", preventGesture);
        document.addEventListener("gestureend", preventGesture);
        return () => {
            document.removeEventListener("gesturestart", preventGesture);
            document.removeEventListener("gesturechange", preventGesture);
            document.removeEventListener("gestureend", preventGesture);
        };
    }, []);


    // L'ÉCHEC PASSE AVANT L'ATTENTE, et il passait après.
    //
    // Ce garde-ci venait EN PREMIER : quand la recherche renonçait, la raison
    // était bien calculée juste au-dessus… puis jetée, parce que `!session`
    // rendait son propre écran avant qu'on n'y arrive. La page restait donc
    // sur « connexion… » pour toujours, y compris quand le code avait conclu
    // cinq secondes plus tôt qu'il n'y avait rien à trouver — c'est le
    // « ça tourne dans le vide et personne ne dit rien » signalé le
    // 2026-09-08. Une attente sans fin est le pire des deux affichages : elle
    // ne dit ni ce qu'on attend, ni qu'on a cessé d'attendre.
    if (terminateReason) {
        return <LoadingScreen textOnFailure={terminateReason} onFailure={onTerminate} />;
    }

    // ANNULER AVANT QUE LA PAGE N'AIT SA PARTIE (#287). On n'arrive ici
    // qu'avec la réponse du lancement : la partie existe déjà chez son hôte,
    // la page ne l'a simplement pas encore en main. On s'en va tout de suite,
    // puis on demande une fois à cet hôte sa liste : à nous, la partie
    // s'arrête ; à quelqu'un d'autre, elle continue sans nous.
    const cancelBeforeSession = async () => {
        const target = federatedHost?.link
            ? { origin: federatedHost.link, token: federatedHost.token }
            : { origin: homeOrigin(), token: authToken || "" };
        const id = lookupSessionId;
        onTerminate();
        if (!id || guestSession) {
            return;
        }
        const list = await kernelSnapshot(target, "sessions");
        const found = (Array.isArray(list) ? list : []).find((entry) => entry.id === id);
        if (found && isOwnSession(found, shellUser?.username)) {
            stopSession(found.platform, found.id, target.token, federatedHost?.link ? target : null);
        }
    };

    if (!session) {
        // The session is still resolving (the façade snapshot hasn't
        // carried it yet) — plain overlay, nothing to stream yet.
        // NOTE: this early return unmounts StreamingShell, which is exactly
        // why "I am leaving" must NOT be bound to that shell's lifetime.
        //
        // Et quand c'est un TÉLÉCHARGEMENT qu'on attend, on le dit : la barre
        // sous le libellé venait sans phrase, ce qui laissait « connexion… »
        // par-dessus un transfert de quarante gigaoctets.
        return (
            <div className="overflow-hidden position-relative" style={{ backgroundColor: "#000", minHeight: "var(--gameflex-visible-height)" }}>
                <LaunchScreen
                    t={t}
                    label={launchFailure
                        ? t("streaming.launch.failed")
                        : (launchSteps.length > 0
                            ? t("streaming.launch.title")
                            : (downloadProgress
                                // Un cœur qui descend avant que l'émulateur
                                // n'existe — le jeu, lui, ne descend plus
                                // (chantier 14).
                                ? t("retrogaming.detail.downloading")
                                : t("streaming.connection.state.connecting")))}
                    steps={launchSteps.length > 0 ? launchSteps : null}
                    // Le fond du lancement, repris de la page des jeux : la
                    // même balise, le même flux (chantier 35).
                    backdrop={launchId ? { launch: { streamId: launchId, username: getStoredUsername() || null } } : null}
                    failed={Boolean(launchFailure)}
                    error={launchFailure?.error || null}
                    onReport={sendReport}
                    reportState={reportState}
                    onCancel={cancelBeforeSession}
                />
            </div>
        );
    }

    return (
        <GamepadProvider>
            <StreamingShell
                key={session.id}
                leaveRef={leaveRef}
                t={t}
                user={shellUser}
                token={token}
                session={session}
                context={context}
                setTerminateReason={setTerminateReason}
                onTerminate={onTerminate}
                reportPlaying={reportPlaying}
                reportIdle={reportIdle}
                onWatchingChange={handleWatchingChange}
                wantsPlay={wantsPlay}
                onPlayIntentSent={dropPlayIntent}
                launchSteps={launchSteps}
                launchId={launchId}
            />
        </GamepadProvider>
    );
}
