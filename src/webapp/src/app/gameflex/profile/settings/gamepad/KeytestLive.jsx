import React, { useCallback, useContext, useEffect, useRef, useState, useSyncExternalStore } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useTranslation } from "react-i18next";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import { useAuth } from "../../../profile/auth/AuthContext.jsx";
import { usePresence } from "../../../contexts/PresenceContext.jsx";
import { useResolvedSession } from "../../../libs/SessionPool.js";
import { SessionStream, OVERLAY_HIDDEN } from "../../../stream/shared/SessionStream.jsx";
import { useChildSlot } from "../../../stream/shared/hooks/use-child-slot.js";
import { useStreamexInput } from "../../../stream/streamex/hooks/use-streamex-input.js";
import { useRetrogamingInput } from "../../../stream/retrogaming/hooks/use-retrogaming-input.js";
import { useKeytestPad } from "../../../stream/streamex/KeyTestPad.jsx";
import { CONTROLLER_PRESETS } from "../../../controllers/presets.js";
import { launchSession, stopSession } from "../../../api/spawnSessions.js";
import { createSession } from "../../../api/retrogaming.js";
import { homeOrigin } from "../../../libs/serverBase.js";
import { kernelSnapshot } from "../../../libs/kernelChannel.js";
import { can } from "../../../libs/capabilities.js";
import { LoadingScreen } from "../../../shared/templates/Essentials.jsx";
import { GamepadProvider } from "../../../hooks/use-gamepad.jsx";
import { useScreenRotation, usePortraitViewport, rotationSurfaceStyle } from "../../../stream/shared/hooks/use-screen-rotation.js";
import { useMenuButtonStyle, useMenuHotkey } from "../../../stream/shared/hooks/use-streaming-menu.js";
import { useScreenOrientation } from "../../../libs/screenOrientation.js";
import { createSwitchesStore } from "../../../stream/shared/switches-store.js";
import { requestFullscreen, exitFullscreen, isFullscreen, isMobileDevice, isTabletDevice } from "../../../shared/utils/device-detection.js";
import { SectionPanel, OffCanvas, ListElement } from "../../../shared/templates/Essentials.jsx";

const Form = Skeleton.Form;

// useKeytestSessionId — resolves the user's keytest session: reuse the one
// the idle grace may still be running from a previous preview, otherwise
// spawn a fresh one. ONE session per user on purpose: anyone joining it
// later (Dashboard, invite) lands on the SAME container and pads.
function useKeytestSessionId(token, user) {
    const [sessionId, setSessionId] = useState(null);
    const [failed, setFailed] = useState(false);

    useEffect(() => {
        let cancelled = false;

        // Une question ponctuelle à NOTRE kernel — « ai-je déjà une session de
        // test ? » —, posée par un instantané du sujet `sessions` : le canal
        // ne sert ici qu'à cette réponse.
        kernelSnapshot({ origin: homeOrigin(), token: token || "" }, "sessions").then((list) => {
            if (cancelled) {
                return;
            }
            // « keytest chez moi » ne dit pas de quelle plateforme il s'agit :
            // un conteneur streamex sur un serveur, une vue dessinée sur le
            // téléphone (KeytestHost.java). Les deux sont la même console.
            const existing = (Array.isArray(list) ? list : []).find(
                (entry) =>
                    (entry.platform === "streamex" || entry.platform === "device") &&
                    entry.consoleId === "keytest" &&
                    entry.owner === user?.username,
            ) || null;

            if (existing) {
                setSessionId(existing.id);
                return;
            }

            // UN HÔTE QUI NE PEUT PAS OUVRIR DE SESSION AILLEURS HÉBERGE LA SIENNE.
            //
            // `spawnsSessions` pose exactement cette question — un serveur
            // ouvre un conteneur, un téléphone n'en ouvre pas. Ne pas la
            // confondre avec `localLibrary`, qui dit seulement qu'on lit des
            // jeux sur l'appareil : les deux coïncident sur le téléphone
            // d'aujourd'hui, pas par construction.
            //
            // Le téléphone n'héberge pas de session streamex, faute de
            // conteneurs. Il sert en revanche la porte unique
            // (la commande `launch`, #195) : `createSession` y envoie
            // `platform: "retrogaming"`, et son kernel reconnaît « keytest »
            // comme une console sans jeu.
            if (!can("spawnsSessions")) {
                createSession("keytest", "keytest", "KeyTest", "", "", user?.username).then((made) => {
                    if (cancelled) {
                        return;
                    }
                    const madeId = made.ok ? made.data?.sessionId : null;
                    if (!madeId) {
                        setFailed(true);
                        return;
                    }
                    setSessionId(madeId);
                }).catch((failure) => {
                    if (cancelled) {
                        return;
                    }

                    console.error("échec de l'ouverture de la session keytest sur l'appareil", failure);
                    setFailed(true);
                });

                return;
            }

            launchSession("keytest", "keytest", token).then((spawned) => {
                if (cancelled) {
                    return;
                }
                const spawnedId = spawned.ok ? spawned.data?.session_id : null;
                if (!spawnedId) {
                    setFailed(true);
                    return;
                }
                setSessionId(spawnedId);
            }).catch((failure) => {
                if (cancelled) {
                    return;
                }

                console.error("échec du lancement de la session keytest", failure);
                setFailed(true);
            });
        }).catch((failure) => {
            if (cancelled) {
                return;
            }

            console.error("échec de la lecture des sessions keytest existantes", failure);
            setFailed(true);
        });

        return () => {
            cancelled = true;
        };
        // token/user are stable for this page's life.
    }, [token, user?.username]);

    return { sessionId, failed };
}

// KeytestShell — keytest's OWN surface, deliberately not StreamingShell.
//
// The two pages look alike and mean different things. /streaming hosts a
// SESSION: it has an authority to obey, a pad to claim, a hand-over to answer,
// an AFK clock, an idle clock, an end to redirect on. Keytest has none of
// that — it is one person looking at their own controller. Borrowing the
// session shell made this page inherit machinery it cannot satisfy, and it
// showed: the shell writes into a `leaveRef` the route is expected to own, and
// this page never passed one, so opening it threw "Cannot set properties of
// undefined" before anything rendered (2026-08-25). It also passed a `preview`
// prop the shell does not declare and silently dropped.
//
// What IS shared stays shared, and it is the part that has no opinion about
// sessions: SessionStream for the picture, and useStreamexInput for the pad —
// which already accepts `preview` precisely for this page.
/**
 * Le hook d'entrée de CETTE session — un seul appel, jamais deux.
 *
 * Appeler les deux et choisir ensuite respecterait les règles des hooks, mais
 * ouvrirait DEUX connexions pour une seule image : c'est exactement la panne
 * « duplicate attempt » déjà corrigée une fois ici. Le `switch` est sûr pour
 * la même raison que celui de Streaming.jsx — le shell est monté avec
 * `key={session.id}`, donc la plateforme ne change jamais en cours de vie.
 */
function useKeytestInput({ screenRef, session, context, t, options = {} }) {
    // Même règle que le dispatcher de Streaming.jsx, pour la même raison :
    // c'est un hook partagé entre plateformes.
    const { slot, preview, keytestPad } = options;
    switch (session.platform) {
        // Le téléphone : même canal de contrôle que le rétro, et un pad qui
        // vient d'ici plutôt que de la console (elle n'en a pas).
        case "device":
            return useRetrogamingInput({
                screenRef, session, context, t,
                options: {
                    isWatching: false,
                    virtualPad: keytestPad,
                    virtualPadLabel: preview
                        ? (CONTROLLER_PRESETS[preview]?.label || preview)
                        : t("streaming.controller.keyTest"),
                },
            });
        default:
            return useStreamexInput({
                screenRef, session, t,
                options: { isWatching: slot == null, preview },
            });
    }
}

// LE MÊME MENU QUE LE STREAMING, EN PLUS PETIT.
//
// Cette page n'avait qu'une flèche de retour : changer de manette obligeait
// à quitter la session et à y revenir, ce qui rallume un conteneur pour
// regarder un autre pad. Trois gestes suffisent à l'éviter — le plein écran,
// la rotation, et le choix de la manette.
//
// Elle ne réutilise PAS `FloatingPanel` : celui-là parle de joueurs, de
// spectateurs, de sauvegardes et de triche, dont rien n'existe ici. Les
// morceaux qui comptaient vraiment — tourner l'écran — sont partagés au
// niveau du HOOK (`use-screen-rotation.js`), là où le partage a du sens.
//
// Même forme que le menu d'une partie depuis le 2026-10-19 (#408) : le titre
// en tête, « Quitter » à la fin — la barre du bas n'est plus. L'orientation
// s'est réglée dans Réglages (#406).
function KeytestMenu({ t, title, switchesStore, presets, preset, onPresetChange, onExit }) {
    const context = useContext(Skeleton.context);
    // ABONNÉ, PAS NOURRI. Cf. `switches-store.js` : ce panneau vit dans un
    // offcanvas, donc ses props sont un instantané gelé au moment de
    // l'ouverture. Recevoir `values`/`setValues` en prop faisait un
    // interrupteur qui ne bougeait jamais — « le plein écran reste sur off »,
    // signalé le 2026-09-09.
    const values = useSyncExternalStore(switchesStore.subscribe, switchesStore.getSnapshot);
    const switchesForm = { values, setValues: switchesStore.setValues };

    return (
        <OffCanvas icon="bi bi-controller" label={title}>
        <SectionPanel>
            <Form.Group>
                <Form.SkeletonSwitch
                    reverse
                    variant="md"
                    name="isFullscreen"
                    context={switchesForm}
                    label={t("streaming.settings.fullscreen.label")}
                    helpText={t("streaming.settings.fullscreen.help")}
                />
            </Form.Group>
            {/* CHANGER DE MANETTE SANS QUITTER. Le préréglage vit dans
                l'adresse (`/keytest/:preset`), donc en changer est une
                navigation — la session, elle, ne bouge pas : le conteneur
                reste, seul le pad rendu change. C'est tout l'intérêt. */}
            <Form.Group
                label={t("keytest.controllerLabel")}
                helpText={t("keytest.controllerHelp")}>
                <Form.Select
                    name="preset"
                    context={{
                        values: { preset: preset || "" },
                        setValues: (next) => onPresetChange(next.preset),
                    }}
                    options={presets} />
            </Form.Group>
        </SectionPanel>
        <SectionPanel label={t("streaming.menu.quit.title")}>
            <ListElement
                light={true}
                icon="bi bi-box-arrow-left"
                label={t("keytest.exitLabel")}
                onClick={() => {
                    context.offcanvas.close();
                    onExit();
                }}
            />
        </SectionPanel>
        </OffCanvas>
    );
}

function KeytestShell({ t, session, token, context, preview, username, onExit, onPresetChange }) {
    const screenRef = useRef(null);
    // A pad, claimed. The child gates input on its queue, so without this the
    // picture plays and every button press is dropped — which is the entire
    // thing this page exists to show.
    const viewerIdOf = useCallback(() => screenRef.current?.viewerId, []);
    // L'IDENTIFIANT DU CHILD, PAS CELUI DE LA SESSION.
    //
    // Ce sont deux choses, et elles ne coïncident que par accident sur le
    // serveur : la session s'appelle comme le gateway l'appelle. Sur le
    // téléphone non — la session est « device:keytest », le flux
    // « device-session ». Cette page passait `session.id` aux deux, donc elle
    // demandait au gateway un flux inexistant : aucune image, aucune erreur,
    // alors que /streaming affichait la MÊME session sans peine (il fait déjà
    // ce repli, voir son propre montage de SessionStream).
    const childId = session.childSessionId || session.id;
    const { slot } = useChildSlot({
        sessionId: childId,
        token,
        enabled: Boolean(session),
        viewerIdOf,
    });
    // Keytest has no session phases to derive — one person, one controller,
    // nothing to obey. The only thing worth waiting for is the picture, so
    // that is the only thing the overlay ever says.
    const [hasVideo, setHasVideo] = useState(false);
    // isWatching false: the whole point is to press buttons and see them land.
    // isWatching follows the CHILD's answer, not an assumption: the pad is
    // only live once a seat is actually held.
    // LE PAD EST LE MÊME, LE TRANSPORT NON.
    //
    // Sur un serveur, KeyTest est une session streamex ; sur le téléphone,
    // c'est une session « device », que la page de streaming route déjà vers
    // le hook rétro (Streaming.jsx : « le téléphone sert maintenant le même
    // canal de contrôle »). On suit la même règle plutôt que d'en inventer
    // une pour cette page.
    const keytestPad = useKeytestPad(preview);
    const input = useKeytestInput({
        screenRef, session, context, t,
        options: { slot, preview, keytestPad },
    });
    const controller = input.controller();
    const menuButtonStyle = useMenuButtonStyle(Boolean(controller?.Component));

    // Les trois réglages de cette page. Un objet, pas un magasin externe :
    // il n'y a qu'un composant qui les lit, et rien à tenir d'accord entre
    // deux arbres React comme sur la page de streaming.
    const shellRef = useRef(null);
    const [isHandheld] = useState(() => isMobileDevice() || isTabletDevice());
    const isPortraitViewport = usePortraitViewport();
    // Un MAGASIN, pas un `useState` — le menu vit dans un offcanvas, dont les
    // props sont figées à l'ouverture (voir `switches-store.js`).
    const switchesStoreRef = useRef(null);
    if (!switchesStoreRef.current) {
        switchesStoreRef.current = createSwitchesStore({ isFullscreen: false });
    }
    const switchesStore = switchesStoreRef.current;
    const switches = useSyncExternalStore(switchesStore.subscribe, switchesStore.getSnapshot);

    useEffect(() => {
        if (switches.isFullscreen && shellRef.current) {
            requestFullscreen(shellRef.current).catch(() => { });
        } else if (!switches.isFullscreen && isFullscreen()) {
            exitFullscreen().catch(() => { });
        }
    }, [switches.isFullscreen]);

    // L'orientation de l'APPAREIL, choisie dans Réglages (#406).
    const screenOrientation = useScreenOrientation();
    const rotateSurface = useScreenRotation(screenOrientation, {
        enabled: isHandheld,
        surfaceRef: shellRef,
        onFullscreen: () => switchesStore.setValues({ isFullscreen: true }),
    });

    // Les autres préréglages, pour en changer sans quitter. Triés par
    // libellé : la table les déclare dans l'ordre où elles ont été écrites,
    // qui n'est celui de personne.
    const presets = Object.entries(CONTROLLER_PRESETS)
        .map(([value, entry]) => ({ value, label: entry.label || value }))
        .sort((left, right) => left.label.localeCompare(right.label));

    const openMenu = () => {
        if (Skeleton.overlayCount() > 0) {
            return;
        }
        context.offcanvas.open(
            <KeytestMenu
                t={t}
                title={controller?.label || t("streaming.controller.keyTest")}
                switchesStore={switchesStore}
                presets={presets}
                preset={preview}
                onPresetChange={onPresetChange}
                onExit={() => confirmExitRef.current()}
            />,
            { size: 480 },
        );
    };
    const openMenuRef = useRef(openMenu);
    openMenuRef.current = openMenu;

    // QUITTER ARRÊTE LA SESSION, donc on demande. Il n'y a pas de « quitter
    // sans arrêter » ici, contrairement au streaming : une session de test
    // n'a pas d'autres joueurs à qui elle survivrait. La confirmation existe
    // parce que le geste est sans retour, pas parce qu'il y a un choix.
    // LA MANETTE EST À L'ESSAI, PAS À L'INTERFACE (#407). Sans panneau ouvert,
    // la croix, A et B montrent ce qu'ils font sur le pad à l'écran ; la
    // navigation de Skeleton se tait — A actionnerait sinon le bouton qui a le
    // focus. Le menu ouvert, elle le sert. Il s'ouvre au doigt, ou par
    // L2 + Home tenus une seconde, comme en partie. Échap n'en est
    // pas : le pavé d'essai le garde (KeyTestPad).
    const overlays = Skeleton.useOverlayCount();
    useEffect(() => {
        Skeleton.suspendNavigation(overlays === 0);
        if (overlays === 0) {
            document.activeElement?.blur?.();
        }
    }, [overlays]);
    useEffect(() => () => Skeleton.suspendNavigation(false), []);
    const menuHold = useMenuHotkey(overlays === 0, () => openMenuRef.current());

    const confirmExit = () => context.modal.open(
        <div className="text-center">{t("keytest.exitConfirm")}</div>,
        {
            variant: "confirm",
            title: t("keytest.exitTitle"),
            confirmLabel: t("keytest.exitLabel"),
            cancelLabel: t("app.cancel"),
            onConfirm: onExit,
        },
    );
    const confirmExitRef = useRef(confirmExit);
    confirmExitRef.current = confirmExit;

    return (
        <div
            ref={shellRef}
            className="d-flex flex-column overflow-hidden"
            style={{
                backgroundColor: "#000",
                // Pas la classe vh-100 : son !important passait devant la
                // hauteur de la rotation de repli, et elle ignorait le
                // demi-écran des manettes clipsées (#410, app.css).
                height: "var(--gameflex-visible-height)",
                ...rotationSurfaceStyle(rotateSurface, isPortraitViewport),
            }}
        >
            <SessionStream
                sessionId={childId}
                t={t}
                screenRef={screenRef}
                hasInput
                username={username}
                muted
                // Through gatewayProps: SessionStream forwards the gateway's
                // own callbacks from there, and a top-level prop of the same
                // name is simply not read.
                gatewayProps={{ onFirstVideoTrack: () => setHasVideo(true) }}
                connectingLabel={hasVideo ? OVERLAY_HIDDEN : t("streaming.connection.phase.waitingVideo")}
                // LE FEU VERT DU SOCKET STREAMEX, comme sur la page de
                // streaming : il ouvre une fois le viewer créé, pour l'annoncer
                // au child — sans lui, le child ne route aucune entrée de ce
                // socket. Pas sur le téléphone, dont le canal rétro se branche
                // autrement ici.
                onViewerReady={session.platform === "device" ? null : () => input.onConnect?.()}
            />

            {controller?.Component && (
                <div className="flex-shrink-0 position-absolute w-100" style={{ bottom: "0", left: 0, zIndex: 1002 }}>
                    {controller.Component}
                </div>
            )}

            {/* LE BOUTON DU MENU, comme sur la page de jeu (#409) : la barre
                du bas — retour, nom de la manette, réglages — n'est plus. Le
                nom est en tête du menu, « Quitter » à sa fin. Toujours visible
                ici : rien ne se joue qu'il faudrait dégager. */}
            <bs.Button
                variant="dark"
                className="position-absolute rounded-circle d-flex align-items-center justify-content-center p-0"
                style={menuButtonStyle}
                title={t("streaming.menu.open")}
                aria-label={t("streaming.menu.open")}
                onClick={openMenu}
            >
                <i className="bi bi-list fs-4 text-white"></i>
            </bs.Button>
            {menuHold >= 0.1 && (
                <div
                    className="position-absolute top-50 start-50 translate-middle text-white text-center rounded-3 px-4 py-3"
                    style={{ zIndex: 1004, background: "rgba(0, 0, 0, 0.7)", minWidth: "260px", pointerEvents: "none" }}
                >
                    <div className="fw-semibold mb-2">{t("streaming.menu.holdHint")}</div>
                    <bs.ProgressBar now={Math.round(menuHold * 100)} style={{ height: "6px" }} />
                </div>
            )}
        </div>
    );
}

// KeytestLive — the Settings → Gamepad → "Test" page: a REAL route (the
// stream must survive a reload, and the offcanvas's own close fought the
// keytest pad's Escape blocking). It spawns (or reuses) the user's keytest
// session and renders it with the preset's controller. An unknown preset suffix streams the bare input-layer pad,
// same as the old ?preview= validation. Joiners from the Dashboard land on
// /streaming with the REAL id and get the bare pad there.
export default function KeytestLive() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const context = useContext(Skeleton.context);
    const { user, token } = useAuth();
    const { preset } = useParams();
    const preview = preset && CONTROLLER_PRESETS[preset] ? preset : null;
    const { sessionId, failed } = useKeytestSessionId(token, user);
    const { session, isRequestTimeout } = useResolvedSession(sessionId);
    // reportIdle only: the shell that used to flip presence to "playing" is
    // gone, and a controller test is not a game anyone watches.
    const { reportIdle } = usePresence();
    const [terminateReason, setTerminateReason] = useState(null);

    // Leaving ENDS it. A keytest session is not a game anyone comes back to —
    // it exists to look at one controller, and its five-minute idle grace
    // would otherwise hold a container (and this user's one-session-at-a-time
    // slot) long after the person walked away. Unlike /streaming, where
    // leaving deliberately keeps the session alive for the others, there are
    // no others here.
    const onTerminate = () => {
        reportIdle();
        if (sessionId) {
            // Fire and forget: the page is leaving either way, and a failed
            // stop still gets swept by the idle watchdog. Waiting on it would
            // only make the exit feel broken.
            //
            // Le téléphone n'a pas de veilleuse d'inactivité pour ça : sa vue
            // de test continuerait à dessiner et à encoder — d'où l'arrêt
            // explicite, pour l'appareil comme pour un conteneur.
            stopSession(session?.platform === "device" ? "device" : "streamex", sessionId, token).catch(() => { });
        }
        // A keytest session has no console catalog entry behind it
        // (/streamex/keytest doesn't exist as a Games route) — back to the
        // gamepad settings page it came from.
        navigate("/");
    };

    useEffect(() => {
        if (failed) {
            setTerminateReason(t("streaming.connection.state.failed"));
            return;
        }
        if (isRequestTimeout) {
            setTerminateReason(t("streaming.connection.state.sessionLookupTimeout"));
        }
    }, [failed, isRequestTimeout, t]);

    if (!session || !sessionId) {
        // The keytest session is still spawning (or being reused) — plain
        // overlay, nothing to stream yet.
        return (
            <div className="overflow-hidden" style={{ backgroundColor: "#000", minHeight: "var(--gameflex-visible-height)" }}>
                <LoadingScreen label={t("streaming.connection.state.connecting")} />
            </div>
        );
    }

    return (terminateReason ? (
        <LoadingScreen textOnFailure={terminateReason} onFailure={onTerminate} />
    ) : (
        <GamepadProvider>
            <KeytestShell
                key={session.id}
                t={t}
                session={session}
                token={token}
                context={context}
                preview={preview}
                username={user?.username}
                onExit={onTerminate}
                // CHANGER DE PRÉRÉGLAGE EST UNE NAVIGATION, pas un
                // remontage : `key={session.id}` ne bouge pas, donc le shell
                // vit, le flux tient, et seul le pad rendu change. `replace`
                // pour que le bouton Retour ne parcoure pas la liste des
                // manettes qu'on a essayées.
                onPresetChange={(next) => navigate(`/keytest/${next}`, { replace: true })}
            />
        </GamepadProvider>
    ));
}
