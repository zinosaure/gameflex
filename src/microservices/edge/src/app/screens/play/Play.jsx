import React, { useCallback, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { SessionPoolProvider, isOwnSession, useResolvedSession } from "../../libs/SessionPool.js";
import { useDownloadProgress } from "../../engine/libs/deviceStream.js";
import { federatedHostOf, parseFederatedId } from "../../libs/federatedHost.js";
import { clearStreamBase, setStreamBase } from "../../libs/serverBase.js";
import { kernelSnapshot } from "../../libs/kernelChannel.js";
import { loadHostCapabilities } from "../../libs/capabilities.js";
import { useLaunchProgress } from "../../engine/stream/shared/hooks/use-launch-progress.js";
import { stopSession } from "../../libs/play.js";
import { GamepadProvider } from "../../engine/hooks/use-gamepad.jsx";
import { LAUNCH_SCREEN_BACKGROUND } from "../../engine/stream/shared/launch-backdrop.js";
import { readToken, reportPresence, tokenUsername } from "../../libs/api.js";
import { useAuth } from "../../libs/data.jsx";
import Skeleton from "../../skeleton/skeleton.js";
import { useScreenInput } from "../../libs/input.js";
import PlayLaunch from "./Launch.jsx";
import PlayShell from "./PlayShell.jsx";
import { useNotices } from "./Notices.jsx";
import { useButton } from "../../libs/ui.js";

// LA PAGE D'UNE PARTIE — /play/<session> (libs/play.js) — pour les trois plateformes, pour jouer comme
// pour regarder. Aucun rôle dans l'adresse : propriétaire, joueur ou spectateur, c'est le serveur qui le
// dit (`session.me`, puis le child). `?play=1` n'est qu'une intention.
//
// Cette route trouve la session (le pool des parties, celui de la webapp), son hôte quand elle vit
// ailleurs (un serveur fédéré, `srv<id>:…`), et suit le
// lancement en cours ; la coquille (PlayShell.jsx) fait le reste.
export default function Play() {
    // Les capacités de l'hôte, que le moteur lit (`can`) : lues une fois, comme la webapp au démarrage.
    const [ready, setReady] = useState(false);

    useEffect(() => {
        loadHostCapabilities().finally(() => setReady(true));
    }, []);

    if (!ready) {
        return <div className="play" style={{ background: LAUNCH_SCREEN_BACKGROUND }}></div>;
    }

    return (
        <SessionPoolProvider>
            <PlayRoute />
        </SessionPoolProvider>
    );
}

// LA FIN D'UNE PARTIE, OU CE QUI L'EMPÊCHE — dite en clair, et un bouton pour partir (B aussi).
function PlayEnd({ reason, onLeave }) {
    const { t } = useTranslation();
    const button = useButton();

    useScreenInput({ back: onLeave, switchTab: () => {}, select: () => {}, start: () => {} });

    useEffect(() => {
        document.querySelector('[data-key="pe-leave"]')?.focus();
    }, []);

    return (
        <div className="play d-flex flex-column align-items-center justify-content-center gap-3 text-center px-3" style={{ background: LAUNCH_SCREEN_BACKGROUND }} data-nav-layer>
            <p className="fw-semibold m-0" style={{ maxWidth: "35rem" }}>{reason}</p>
            <button type="button" className={button("main")} data-nav data-key="pe-leave" onClick={onLeave}>{t("play.back")}</button>
        </div>
    );
}

function PlayRoute() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const location = useLocation();
    const { user } = useAuth();
    const authToken = readToken();
    const { sessionId: rawSessionId } = useParams();
    const [searchParams, setSearchParams] = useSearchParams();

    // L'INTENTION DE JOUER, lue À CHAQUE RENDU (jamais figée au montage) ; dépensée, elle quitte
    // l'adresse : le lien recopié reste un lien pour regarder.
    const wantsPlay = searchParams.get("play") === "1";
    const dropPlayIntent = useCallback(() => {
        setSearchParams((params) => {
            params.delete("play");

            return params;
        }, { replace: true, state: location.state });
    }, [setSearchParams, location.state]);

    // LE LANCEMENT EN COURS (#191) : on arrive ici avec sa réponse, et ses étapes se poursuivent.
    const launchId = searchParams.get("launch");
    const { steps: launchSteps, failure: launchFailure } = useLaunchProgress(launchId, authToken);

    // UNE SESSION QUI VIT CHEZ UN SERVEUR FÉDÉRÉ (C9-P4c) : son préfixe dit chez qui ; l'adresse et le
    // jeton se demandent à notre kernel, jamais à l'adresse qu'on nous tend.
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

    // Tant que la page montre une partie d'ailleurs, TOUTES ses adresses visent l'hôte — le temps de la page.
    useEffect(() => {
        if (!federatedHost?.link) {
            return undefined;
        }
        setStreamBase(federatedHost.link, federatedHost.token, federatedHost.username);

        return () => clearStreamBase();
    }, [federatedHost]);

    // L'identifiant tel que l'hôte le connaît ; nul tant que l'hôte n'est pas résolu.
    const lookupSessionId = federatedId ? (federatedHost ? federatedId.id : null) : rawSessionId;
    // Ce qui descend (l'application Android) : l'attente de la session ne compte pas pendant ce temps.
    const downloadProgress = useDownloadProgress();
    const { session: resolvedSession, isRequestTimeout } = useResolvedSession(lookupSessionId, {
        host: federatedHost?.link ? { origin: federatedHost.link, token: federatedHost.token } : null,
        waitingOn: downloadProgress,
    });
    // L'INVITÉ ANONYME D'UN AUTRE APPAREIL EST PARTI (`#756`) : sa branche (`?guest=1&host=…`, deviceSession.js) demandait
    // un jeton de joueur au téléphone sans compte. Comme au serveur, on regarde sans compte et on joue avec le sien.
    const session = resolvedSession;

    // Chez un hôte fédéré, c'est SON jeton et SON nom qui valent (#235) — jamais le nôtre, qu'il ne sait pas lire.
    const token = federatedId ? (federatedHost?.token || "") : authToken;
    // REGARDER SANS COMPTE (`#747`, le « watch » grand public, celui du Chromecast) : aucun jeton chez l'hôte de la
    // partie — un lien ouvert sans être connecté, ou un serveur fédéré où l'on n'a pas collé de compte. On voit la
    // vidéo, on ne rejoint pas : la coquille n'ouvre ni le canal de la partie ni ses entrées, et son menu se réduit.
    const watchOnly = !token;
    const shellUser = useMemo(() => (federatedHost ? { ...user, username: federatedHost.username || "" } : user), [federatedHost, user]);

    // QUITTER LA PAGE, C'EST QUITTER LA SESSION — et cette route est la seule dont la vie veut dire ça.
    // pagehide pour la vraie page qui s'en va, le démontage pour une navigation. Toujours déconnecter,
    // jamais arrêter : la partie continue pour les autres.
    const leaveRef = useRef(null);
    useEffect(() => {
        const leave = () => leaveRef.current?.();
        window.addEventListener("pagehide", leave);

        return () => {
            window.removeEventListener("pagehide", leave);
            leave();
        };
    }, []);

    const watchingRef = useRef(false);
    const handleWatchingChange = useCallback((watching) => {
        watchingRef.current = watching;
    }, []);
    const reportPlaying = useCallback((played, watching) => reportPresence({
        type: "playing",
        sessionId: played.id,
        platform: played.platform,
        consoleId: played.consoleId,
        reference: played.reference || null,
        role: watching ? "spectator" : "player",
    }), []);
    const reportIdle = useCallback(() => reportPresence({ type: "idle" }), []);

    const [terminateReason, setTerminateReason] = useState(null);
    // LES AVIS DE LA PAGE (Notices.jsx) — ici, pour que le Remote Play (GamepadProvider) les ouvre aussi.
    const notices = useNotices();
    // Le contexte de Skeleton, pour le moteur partagé (le Remote Play lit `Skeleton.context`) : la fenêtre de
    // l'application (index.js), et les avis de la page de jeu à la place des siens.
    const skeleton = useContext(Skeleton.context);

    // PARTIR : là d'où l'on venait (l'historique le porte), sinon l'écran de la console.
    const onTerminate = useCallback(() => {
        if (watchingRef.current) {
            reportIdle();
        }
        const platform = String(session?.platform || "").toLowerCase();
        const consoleId = String(session?.consoleId || "").toLowerCase();
        const tab = platform === "device" ? "retrogaming" : platform;
        const fallback = tab && consoleId ? `/${tab}/${tab === "remoteplay" && !consoleId.startsWith("rp-") ? `rp-${consoleId}` : consoleId}` : "/live";
        navigate(location.state?.back || fallback, { replace: true });
    }, [session?.platform, session?.consoleId, navigate, location.state, reportIdle]);

    useEffect(() => {
        if (isRequestTimeout) {
            setTerminateReason(t("play.notFound"));
        }
    }, [isRequestTimeout, t]);

    // Le pincement et le double tap zooment : ils se battent avec la manette à l'écran.
    useEffect(() => {
        const prevent = (event) => event.preventDefault();
        const events = ["gesturestart", "gesturechange", "gestureend"];
        events.forEach((name) => document.addEventListener(name, prevent));

        return () => events.forEach((name) => document.removeEventListener(name, prevent));
    }, []);

    // L'ÉCHEC PASSE AVANT L'ATTENTE : une attente sans fin ne dit ni ce qu'on attend, ni qu'on a cessé.
    if (terminateReason) {
        return <PlayEnd reason={terminateReason} onLeave={onTerminate} />;
    }

    // ANNULER AVANT QUE LA PAGE N'AIT SA PARTIE (#287) : la partie existe déjà chez son hôte ; à nous,
    // elle s'arrête, à quelqu'un d'autre, elle continue sans nous.
    const cancelBeforeSession = async () => {
        const target = federatedHost?.link ? { origin: federatedHost.link, token: federatedHost.token } : { origin: "", token: authToken || "" };
        const id = lookupSessionId;
        onTerminate();

        if (!id) {
            return;
        }
        const list = await kernelSnapshot(target, "sessions");
        const found = (Array.isArray(list) ? list : []).find((entry) => entry.id === id);

        if (found && isOwnSession(found, shellUser?.username)) {
            stopSession(found.platform, found.id, target.token, federatedHost?.link ? target : null);
        }
    };

    if (!session) {
        return (
            <div className="play">
                <PlayLaunch
                    label={launchFailure ? t("play.failed") : t(launchSteps.length ? "play.launching" : "play.connecting")}
                    steps={launchSteps}
                    failure={launchFailure}
                    backdrop={launchId ? { launch: { streamId: launchId, username: tokenUsername() } } : null}
                    onCancel={cancelBeforeSession}
                />
            </div>
        );
    }

    return (
        <Skeleton.context.Provider value={{ ...skeleton, ...notices.context }}>
            <GamepadProvider>
                <PlayShell
                    key={session.id}
                    notices={notices}
            leaveRef={leaveRef}
            user={shellUser}
            token={token}
            session={session}
            setTerminateReason={setTerminateReason}
            onTerminate={onTerminate}
            reportPlaying={reportPlaying}
            reportIdle={reportIdle}
            onWatchingChange={handleWatchingChange}
            wantsPlay={wantsPlay}
            onPlayIntentSent={dropPlayIntent}
            launchSteps={launchSteps}
            launchId={launchId}
            watchOnly={watchOnly}
            hostName={federatedHost?.hostname || ""}
                />
            </GamepadProvider>
        </Skeleton.context.Provider>
    );
}
