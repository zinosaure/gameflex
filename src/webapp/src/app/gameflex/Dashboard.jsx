import React, {
    useEffect,
    useMemo,
    useState,
    useContext,
} from "react";
import { useNavigate } from "react-router-dom";
import { useTranslation } from "react-i18next";
import * as bs from "react-bootstrap";
import Skeleton from "../skeleton/skeleton.js";
import { useSessionPool, hasOpenSlot, isOwnSession } from "./libs/SessionPool.js";
import { useAuth } from "./profile/auth/AuthContext.jsx";
import { SessionStream } from "./stream/shared/SessionStream.jsx";
import { stopSession } from "./api/spawnSessions.js";
import { ListElement } from "./shared/templates/Essentials.jsx";
import { buildStreamingUrl } from "./libs/sessionLinks.js";
import { federatedHostOf, parseFederatedId } from "./libs/federatedHost.js";


// Both ways back to a pad — JOIN and "take my session back" — are the same
// gesture: navigate with the intent to play (`?play=1`). The CHILD that runs
// the session decides; the streaming page carries the intent to it over the
// socket that will actually hold the slot. That socket does not exist yet at
// this point, which is precisely why the child, and not this button, is what
// decides. (The kernel used to grant a pad here itself — the v1 fallback —
// and answered 409 for every session its child owned; it no longer decides
// anything about who plays.)

// SessionDuration — depuis combien de temps la partie tourne (le porteur,
// 2026-10-25 : « dans Live now, il manque la durée de la session »). Lue sur
// `startedAt`, que le kernel publie — celle du conteneur pour une partie
// retrouvée après un redémarrage. Un chronomètre, `00:00:00`, qui avance à la
// seconde, à droite de la carte (le porteur : « sans l'icône, juste au format
// 00:00:00 ») ; sans texte, il n'a rien à traduire.
function SessionDuration({ startedAt }) {
    const [now, setNow] = useState(() => Date.now());

    useEffect(() => {
        const timer = setInterval(() => setNow(Date.now()), 1000);

        return () => clearInterval(timer);
    }, []);

    const started = new Date(startedAt).getTime();
    if (!startedAt || Number.isNaN(started)) {
        return null;
    }

    const seconds = Math.max(0, Math.floor((now - started) / 1000));
    const clock = [Math.floor(seconds / 3600), Math.floor(seconds / 60) % 60, seconds % 60]
        .map((part) => String(part).padStart(2, "0"))
        .join(":");

    return (
        <div className="text-nowrap font-monospace" title={new Date(started).toLocaleString()}>
            {clock}
        </div>
    );
}

function LiveThumbnail({ session }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { user, token, isAdmin } = useAuth();
    const context = useContext(Skeleton.context);
    // v1 (chantier-3): joinable when a slot is free and the session is not
    // mine. NO platform whitelist any more — openSlots alone decides, which
    // is what makes the rule identical everywhere: the PS5 has a single pad
    // and its owner always holds it, so openSlots is 0 and only WATCH shows,
    // without a line of remoteplay-specific code. Joining your own game
    // makes no sense either way — an owner comes back through "Your
    // sessions" (reported live: "I joined myself, which is illogical").
    const isMine = isOwnSession(session, user?.username);
    // Already holding a pad on this session? Then there is nothing to join —
    // "one session, one player" applies to joiners too, so a second browser
    // must offer WATCH like any other extra tab (reported live 2026-08-25:
    // "a joiner still sees the join button on another browser").
    const alreadyPlaying = session.me?.padSlot != null;
    const canJoin = !isMine && !alreadyPlaying && hasOpenSlot(session);

    const streamingUrl = buildStreamingUrl({ sessionId: session.id });

    // UNE PARTIE D'AILLEURS SE REGARDE CHEZ ELLE. Son flux est diffusé par le
    // gateway de son hôte, sous l'identifiant que lui lui donne : demander
    // `srv1:…` au nôtre rendait un 404, et la vignette réessayait sans fin,
    // noire (relevé le 2026-10-09). L'adresse et le jeton se demandent à
    // notre kernel (`federatedHostOf`), comme pour arrêter la partie.
    const streamId = session.childSessionId || session.id;
    const federated = Boolean(parseFederatedId(streamId));
    const [remote, setRemote] = useState(null);
    useEffect(() => {
        if (!federated) {
            setRemote(null);

            return undefined;
        }
        let cancelled = false;
        federatedHostOf(streamId).then((found) => {
            if (!cancelled) {
                setRemote(found);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [streamId, federated]);
    const streamHost = useMemo(
        () => (remote ? { origin: remote.link, token: remote.token } : null),
        [remote],
    );

    // JOIN declares the intent to play — never inferred from the connection.
    // The child decides once the page's socket to it exists; a refusal comes
    // back there, and the page simply stays watching.
    const goJoin = () => {
        navigate(buildStreamingUrl({ sessionId: session.id, play: true }));
    };
    const goWatch = () => navigate(streamingUrl);

    // UN ADMINISTRATEUR ARRÊTE LA PARTIE D'UN AUTRE (le porteur, 2026-10-25),
    // avec DEUX confirmations, « pour éviter les erreurs ». Pas celle d'un
    // serveur ami : nos droits s'arrêtent à nos parties. La sienne s'arrête
    // depuis « Vos sessions », plus haut.
    //
    // Le nom du propriétaire et le titre passent par un élément React, qui les
    // échappe : le titre et un texte de la fenêtre sont insérés tels quels
    // dans le HTML, et un nom d'affichage se choisit librement.
    const canStop = isAdmin && !isMine && !federated;
    const stopForEveryone = async () => {
        const result = await stopSession(session.platform, session.id, token);
        if (!result.ok) {
            context.toast.open(t("dashboard.live.stopFailed"), { variant: "error" });
        }
    };
    const askStop = () => {
        context.modal.open(
            <span>{t("dashboard.live.stopFirstMessage", { owner: session.ownerDisplayName, title: session.title })}</span>,
            {
                variant: "confirm",
                title: t("dashboard.live.stopFirstTitle"),
                confirmLabel: t("dashboard.live.stopContinue"),
                cancelLabel: t("app.cancel"),
                onConfirm: () => context.modal.open(
                    <span>{t("dashboard.live.stopSecondMessage")}</span>,
                    {
                        variant: "confirm",
                        title: t("dashboard.live.stopSecondTitle"),
                        confirmLabel: t("dashboard.live.stopConfirm"),
                        cancelLabel: t("app.cancel"),
                        onConfirm: stopForEveryone,
                    },
                ),
            },
        );
    };

    return (
        <bs.Card className="bg-black border-0 overflow-hidden w-100">
            {/* A fixed (viewport-relative, not container-relative) height
                rather than aspect-ratio — every slide's box must be
                IDENTICAL regardless of which one is the carousel's current
                "active" item: react-bootstrap positions the inactive
                slide(s) with position:absolute while transitioning, and an
                aspect-ratio computed off each slide's own (possibly
                momentarily different) content-box width doesn't stay in
                sync with the active one the same way a plain vh-based
                height does. */}
            <div className="position-relative w-100" style={{ height: "240px", zIndex: 0 }}>
                {/* L'IDENTIFIANT DU FLUX EST CELUI DU CHILD.

                    Le gateway connaît le flux sous le nom que l'hôte qui le
                    pousse lui a donné. Sur un serveur c'est la session
                    elle-même (`childSessionId` n'y est pas publié, le repli
                    s'applique) ; sur le téléphone c'est `device-session`,
                    fixe — un cœur par processus.

                    Demander `session.id` revenait à réclamer au gateway un
                    flux qui n'existe pas sous ce nom : la vignette
                    réessayait sans fin et restait noire (2026-09-04). Les
                    parties du téléphone n'apparaissaient ici que depuis
                    C8-P1, d'où un défaut qui n'avait jamais pu se voir.
                    Même idiome que Streaming.jsx pour le canal child. */}
                {(!federated || remote) && (
                    <SessionStream
                        sessionId={federated ? remote.id : streamId}
                        host={streamHost}
                        t={t}
                        maxAttempts={null}
                        showOverlay={false}
                        fillHeight
                        muted={true}
                        // LE 540p À 30 POUR UNE VIGNETTE (chantier 31, le
                        // porteur) : là où la partie sait l'encoder.
                        quality="sd"
                        // Une vignette n'ouvre pas la partie : pas d'intro
                        // (chantier 35).
                        preview
                    />
                )}
                <bs.Badge bg="danger" className="position-absolute top-0 start-0 m-2">
                    {t("dashboard.live.badge")}
                </bs.Badge>
            </div>

            <bs.Card.Body className="bg-body-secondary d-flex flex-column gap-2">
                {/* col 1: console id — col 2: game title + who started it —
                    col 3: current/max participants. Deliberately the raw
                    consoleId (not the display name) — a short, stable
                    label that fits this tight a column even for long
                    console names. */}
                <div className="d-flex align-items-start gap-3">
                    <div
                        className="text-uppercase small fw-semibold flex-shrink-0"
                        style={{ minWidth: 48 }}
                    >
                        {session.consoleId}
                    </div>
                    <div className="flex-grow-1 text-truncate">
                        <div className="fw-semibold text-truncate">{session.title}</div>
                        <div className=" small text-truncate">
                            {session.ownerDisplayName}
                        </div>
                    </div>
                    {/* LE MAXIMUM DE LA CONSOLE, pas la somme des places
                        libres. `openSlots` RÉSERVE le siège 1 tant que le
                        propriétaire ne joue pas : l'addition perdait donc ce
                        siège, et un jeu PS1 à quatre manettes s'affichait
                        « 0/3 » — après avoir affiché « 1/1 » pour la même
                        raison (2026-09-04). Le maximum est servi par la
                        façade ; la somme ne reste qu'un repli. */}
                    <div className="small flex-shrink-0 text-end">
                        <div>{session.playerCount}/{session.maxPlayers || (session.playerCount + session.openSlots)}</div>
                        <SessionDuration startedAt={session.startedAt} />
                    </div>
                </div>

                <div className="d-flex gap-2 mt-2">
                    {canJoin ? (
                        <>
                            <bs.Button variant="warning" className="w-50" onClick={goJoin}>
                                {t("dashboard.live.join")}
                            </bs.Button>
                            <bs.Button variant="info" className="w-50" onClick={goWatch}>
                                {t("dashboard.live.watch")}
                            </bs.Button>
                        </>
                    ) : (
                        <bs.Button variant="info" className="w-100" onClick={goWatch}>
                            {t("dashboard.live.watch")}
                        </bs.Button>
                    )}
                    {canStop && (
                        <bs.Button
                            variant="danger"
                            className="flex-shrink-0"
                            aria-label={t("dashboard.live.stop")}
                            title={t("dashboard.live.stop")}
                            onClick={askStop}
                        >
                            <i className="bi bi-power"></i>
                        </bs.Button>
                    )}
                </div>
            </bs.Card.Body>
        </bs.Card>
    );
}

// Icon per platform — same set buildMenuItems (index.js) already uses for
// the matching menu sections, so a session's icon here matches where it
// came from (bi-controller: retro-gaming, bi-tv: streamex/"High
// Definition", bi-broadcast: remote-play).
const PLATFORM_ICON = {
    retrogaming: "bi-controller",
    streamex: "bi-tv",
    remoteplay: "bi-broadcast",
};

// IdleCountdown — how long an abandoned session has left before the kernel
// stops it. The remaining time is computed from the SERVER's own two values
// (idleSince + idleTimeout, both on the wire) rather than a rule copied into
// this file: a client-side copy of a server threshold drifts silently the
// day the server's changes, which is exactly how the AFK warning ended up
// firing at the wrong moment.
//
// Ticks on its own so the number actually counts down; renders nothing at
// all while somebody is playing (idleSince null = not idle).
function IdleCountdown({ session, t }) {
    const [now, setNow] = useState(() => Date.now());

    useEffect(() => {
        if (!session.idleSince) {
            return undefined;
        }

        const timer = setInterval(() => setNow(Date.now()), 1000);

        return () => clearInterval(timer);
    }, [session.idleSince]);

    if (!session.idleSince || !session.idleTimeout) {
        return null;
    }

    const elapsed = Math.floor((now - new Date(session.idleSince).getTime()) / 1000);
    const left = session.idleTimeout - elapsed;

    if (left <= 0) {
        return <span className="text-danger">{t("dashboard.mySessions.endingNow")}</span>;
    }

    const minutes = Math.floor(left / 60);
    const seconds = String(left % 60).padStart(2, "0");

    return (
        <span className={`${left <= 60 ? "text-danger" : "text-muted"}`}>
            {t("dashboard.mySessions.timeLeft", { time: `${minutes}:${seconds}` })}
        </span>
    );
}

// MySession — the user's OWN in-progress sessions, listed above Live Now.
// Exists because a session no longer dies the moment its owner disconnects
// (5-minute idle grace, see manager.go's watchdog): closing the tab by
// accident, or just navigating away to browse Games.jsx, used to mean the
// session was simply gone — now it's still there, reachable by clicking
// back into it (same sessionId → the Streaming page re-attaches instead of
// spawning a fresh container, both for retro-gaming AND remote-play, see
// use-retro-streaming-connection.js/use-streaming-connection.js's own
// reconnect handling). This list is what makes that discoverable instead of
// requiring the exact URL to still be in history — and the trash icon is
// what makes it possible to actually END one instead of just leaving it to
// expire on its own after 5 minutes idle.
//
// Deliberately renders nothing (not even an empty state) when the user has
// no session of their own — unlike Live Now, which is a permanent landmark,
// this is a personal reminder that shouldn't take up space when unused.
function MySession({ sessions, username, token, t }) {
    const navigate = useNavigate();
    const context = useContext(Skeleton.context);
    // Y compris une partie lancée chez un serveur fédéré : elle appartient à
    // notre compte de là-bas (voir isOwnSession).
    const mine = sessions.filter((s) => isOwnSession(s, username));

    if (mine.length === 0) return null;

    // Opening the link WATCHES — it never gives the pad back (v1 R3). That
    // is what makes a streaming link safe to hand around: it cannot grant
    // control to anyone, its owner included.
    const handleWatch = (session) => {
        navigate(buildStreamingUrl({ sessionId: session.id }));
    };

    // …and this is the ONE way back to the pad: a declared intent. Same
    // endpoint and same server-side check as a spectator joining — the only
    // difference is the label, because "take my session back" and "join a
    // free slot" are the same operation once control stopped being
    // something you got by showing up.
    const handleResume = (session) => {
        navigate(buildStreamingUrl({ sessionId: session.id, play: true }));
    };

    const handleStop = (session) => {
        context.modal.open(t("dashboard.mySessions.stopConfirmMessage"), {
            variant: "confirm",
            title: t("dashboard.mySessions.stopConfirmTitle"),
            confirmLabel: t("app.delete"),
            cancelLabel: t("app.cancel"),
            onConfirm: async () => {
                // UNE PARTIE S'ARRÊTE PAREIL, quelle qu'elle soit (C8-P4).
                //
                // Il y avait ici une branche pour le téléphone, qui appelait
                // sa propre route d'arrêt : router vers remote-play envoyait
                // une requête qui ne pouvait qu'échouer, et le bouton rouge
                // disait « impossible d'arrêter » sur un jeu qu'on venait de
                // lancer (2026-09-02). Le téléphone sert maintenant la route
                // du serveur, donc il n'y a plus qu'un chemin.
                //
                // UNE PARTIE D'AILLEURS S'ARRÊTE CHEZ ELLE (#234). L'arrêt
                // partait chez NOUS, avec notre jeton et l'identifiant
                // préfixé : notre kernel ne connaît pas cette session, il
                // répondait 404, le bouton disait « impossible d'arrêter » —
                // et la partie continuait chez son hôte (vu le 2026-09-28).
                const federated = await federatedHostOf(session.id);
                const id = federated ? federated.id : session.id;
                const host = federated ? { origin: federated.link, token: federated.token } : null;
                const result = await stopSession(session.platform, id, token, host);
                if (!result.ok) {
                    context.toast.open(t("dashboard.mySessions.stopFailed"), {
                        variant: "error",
                    });
                }
                // No manual list update needed — the `sessions` push
                // (useSessionPool) removes it as soon as the backend drops
                // the record, same as every other session-pool consumer.
            },
        });
    };

    return (
        <div className="mb-5">
            <h6 className="fw-bold text-uppercase text-muted small mb-3">
                {t("dashboard.mySessions.title")}
            </h6>
            <div className="d-flex flex-column gap-2">
                {mine.map((session) => (
                    <ListElement
                        key={session.id}
                        onClick={() => {
                            if (session.me?.padSlot == null) {
                                handleResume(session);
                            } else {
                                handleWatch(session);
                            }
                        }}
                        icon={session.me?.padSlot == null ? "bi bi-arrow-right" : `bi ${PLATFORM_ICON[session.platform] || "bi-controller"}`}
                        label={session.title}
                        helpText={
                            <div className="d-flex gap-2 align-items-center justify-content-between">
                                <div>{session.consoleId?.toUpperCase()}</div>
                                {session.idleSince && <> <IdleCountdown session={session} t={t} /></>}
                            </div>
                        }
                        next={
                            <div className="d-flex gap-2">
                                <bs.Button
                                    variant="danger"
                                    label={t("dashboard.mySessions.stop")}
                                    onClick={(e) => {
                                        e.stopPropagation();
                                        handleStop(session);
                                    }}
                                >
                                    <i className="bi bi-power"></i>
                                </bs.Button>
                            </div>
                        }
                    />
                ))}
            </div>
        </div>
    );
}

export default function Dashboard() {
    const { t } = useTranslation();
    const { token, user } = useAuth();
    const sessions = useSessionPool();
    // Tracked by SESSION ID, not array position — the façade's
    // ordering is now sorted server-side (see internal/sessions/pool.go's
    // List()) so this mostly matters as defense in depth, but react-
    // bootstrap's Carousel is otherwise an uncontrolled, index-based
    // component: if `sessions` ever reorders between renders (a session
    // ending/starting shifts array positions) while tracking a raw index,
    // the "active" slide would silently jump to a DIFFERENT session's video
    // — this was the actual cause of "I started 2 sessions and only ever
    // saw one preview" before the backend sort was added.
    const [activeSessionId, setActiveSessionId] = useState(null);
    const activeIndex = Math.max(
        0,
        sessions.findIndex((s) => s.id === activeSessionId),
    );

    return (
        <bs.Container className="pt-3">
            <div className="text-center mb-5">
                <div style={{ fontSize: "5rem", }} className="mb-2">
                    <i className="bi bi-controller"></i>
                </div>
                <h1 className="fw-bold mb-2">
                    {t("dashboard.greeting", {
                        name: user?.display_name || user?.username,
                    })}
                </h1>
            </div>

            <MySession sessions={sessions} username={user?.username} token={token} t={t} />

            <div className="mb-5">
                <h6 className="fw-bold text-uppercase text-muted small mb-3">
                    {t("dashboard.live.title")}
                </h6>
                {sessions.length > 0 ? (
                    <>
                        {/* interval={null} — no auto-advance: each slide holds a
                            live WebRTC connection someone may be actively
                            watching, yanking it away on a timer would be
                            jarring. indicators={false} — react-bootstrap's own
                            indicators are absolutely-positioned OVER the active
                            slide, which for this card (image + text + buttons
                            below it, not just an image) land on top of the
                            Join/Watch buttons — a plain row of dots in normal
                            flow right below the carousel instead. */}
                        <bs.Carousel
                            activeIndex={activeIndex}
                            onSelect={(idx) => setActiveSessionId(sessions[idx]?.id)}
                            interval={null}
                            controls={sessions.length > 1}
                            indicators={false}
                            variant="dark"
                        >
                            {sessions.map((session) => (
                                <bs.Carousel.Item key={session.id}>
                                    <LiveThumbnail session={session} />
                                </bs.Carousel.Item>
                            ))}
                        </bs.Carousel>

                        {sessions.length > 1 && (
                            <div className="d-flex justify-content-center gap-2 mt-3">
                                {sessions.map((session, i) => (
                                    <button
                                        key={session.id}
                                        type="button"
                                        aria-label={t("dashboard.live.slideLabel", {
                                            position: i + 1,
                                            count: sessions.length,
                                        })}
                                        className="rounded-circle border-0 p-0"
                                        style={{
                                            width: 8,
                                            height: 8,
                                            background:
                                                i === activeIndex ? "#fff" : "rgba(255,255,255,0.35)",
                                        }}
                                        onClick={() => setActiveSessionId(session.id)}
                                    />
                                ))}
                            </div>
                        )}
                    </>
                ) : (
                    <div className="text-center py-5 bg-body-secondary rounded">
                        {/* No dedicated "broadcast, slashed" glyph exists in
                            bootstrap-icons (only bi-broadcast/bi-broadcast-pin)
                            — bi-slash-lg stacked on top of bi-broadcast at the
                            same size/position is the standard composition
                            trick for an "-off" variant the icon set doesn't
                            ship. */}
                        <div
                            className="position-relative d-inline-block mb-2"
                            style={{ fontSize: "3rem", opacity: 0.35 }}
                        >
                            <i className="bi bi-broadcast"></i>
                            <i className="bi bi-slash-lg position-absolute top-0 start-0"></i>
                        </div>
                        <h6 className="fw-bold mb-0 text-muted">
                            {t("dashboard.live.empty")}
                        </h6>
                    </div>
                )}
            </div>
        </bs.Container>
    );
}
