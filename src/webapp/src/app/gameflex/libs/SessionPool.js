import React, { createContext, useContext, useEffect, useMemo, useRef, useState } from "react";
import { useAuth } from "../profile/auth/AuthContext.jsx";
import { resolveUsers } from "../profile/api.js";
import { getPublicSession } from "../api/spawnSessions.js";
import { homeOrigin, streamAuthToken } from "./serverBase.js";
import { federatedId, federatedServers } from "./federatedHost.js";
import { onDeviceGameStarted } from "./deviceStream.js";
import { federationSession, onFederationSessionChanged } from "./federationSession.js";
import { kernelChannel, kernelReachable, useKernelChannel, useKernelTopic } from "./kernelChannel.js";

// LA LISTE DES SESSIONS, pour toute l'application (montée une fois dans
// l'AppShell d'index.js, pas par page) — toutes les sessions, quelle que soit
// leur plateforme, sous une seule forme. Lue par le carrousel du tableau de
// bord, « Mes sessions », la cloche de notifications et la page de streaming.
//
// UN ABONNEMENT DU CENTRE DE MESSAGES (`sessions`, libs/kernelChannel.js),
// sur le même socket que tout ce que la page demande à notre kernel — et un
// par serveur fédéré qui nous ouvre une session (chantier 20). Le kernel pousse la liste à
// l'abonnement puis à chaque changement ; rien n'est redemandé.
const SessionPoolContext = createContext({ sessions: [], displayNames: {} });

export function SessionPoolProvider({ children }) {
    const { token } = useAuth();
    const [sessions, setSessions] = useState([]);
    // initiator AND every entry in players are only ever plain usernames —
    // see internal/sessions' own doc on why (no server-side identity
    // verification for either platform). Resolved to actual display_names
    // here, once, for every consumer (LiveThumbnail.jsx, NotificationBell,
    // Watch.jsx's per-player "request slot" list) instead of each one
    // fetching independently — resolvedRef tracks which usernames have
    // already been requested (found or not) so a WS push that doesn't
    // introduce any NEW username is a no-op, not a re-fetch of the same
    // names over and over.
    const [displayNames, setDisplayNames] = useState({});
    const resolvedRef = useRef(new Set());

    useEffect(() => {
        if (!token) return;
        const usernames = new Set();
        for (const s of sessions) {
            if (s.owner) usernames.add(s.owner);
            const poolUsernames = (s.connectionPool || []).map((e) => e.username).filter(Boolean);
            for (const p of poolUsernames) usernames.add(p);
            // …and whoever holds a pad. Since chantier-4 the players come from
            // the CHILD's own state, mirrored into padSlots — they are not
            // necessarily in connectionPool, which is the v1 view. Without
            // this, the one list built entirely of players (the hand-over
            // chooser) was the one list that never had a name to show.
            for (const holder of Object.values(s.padSlots || {})) {
                if (typeof holder === "string" && holder) usernames.add(holder);
            }
        }
        const missing = [...usernames].filter((u) => !resolvedRef.current.has(u));
        if (missing.length === 0) return;
        missing.forEach((u) => resolvedRef.current.add(u));
        resolveUsers(token, missing).then((result) => {
            if (result.ok) setDisplayNames((prev) => ({ ...prev, ...result.item }));
        });
    }, [sessions, token]);

    // initiatorDisplayName always has something sensible to render even
    // before resolution finishes (or if it fails, or the username has no
    // matching account) — falls back to the raw username rather than
    // consumers needing their own `|| session.initiator` everywhere. Every
    // OTHER player's username (session.players) isn't enriched the same way
    // here — displayNames is exposed directly (see useDisplayNames below) so
    // a consumer that needs to keep the raw username around too (e.g. the
    // façade slot-request flows, which must send the real username over
    // the wire, only the LABEL should show a display name) can resolve
    // just the label without losing the underlying value.
    // LES PARTIES D'AILLEURS, DANS LA MÊME LISTE (chantier 9, C9-P5).
    //
    // Le tableau de bord ne montrait que nos propres sessions : une partie
    // lancée sur un serveur fédéré n'apparaissait ni dans « Live Now » ni dans
    // « Mes sessions », donc il n'y avait aucun moyen d'y revenir — quitter la
    // page, c'était perdre la partie de vue (signalé le 2026-09-07).
    //
    // CÔTÉ NAVIGATEUR : la page écoute l'hôte avec la session que notre kernel
    // échange chez lui contre le jeton de NOTRE compte de chez lui
    // (federationSession, chantier 20), comme elle y jouera.
    //
    // UNE ÉCOUTE, PLUS UN SONDAGE. Cette liste était redemandée toutes les cinq
    // secondes, au motif qu'un canal permanent vers chaque serveur ami
    // coûterait plus cher. La règle du projet a tranché depuis (« rien en
    // polling », 2026-10-08), et un canal coûte maintenant ce qu'il doit : le
    // même socket que la page ouvrira de toute façon pour y jouer.
    const [federatedSessions, setFederatedSessions] = useState({});

    useEffect(() => {
        let cancelled = false;
        let subscriptions = [];

        const drop = () => {
            subscriptions.forEach((leave) => leave());
            subscriptions = [];
        };

        const listen = async () => {
            const servers = await federatedServers();
            // TOUS À LA FOIS : un ami éteint ne retient pas les autres.
            const opened = await Promise.all((servers || []).map(
                async (server) => [server, await federationSession(server.id)],
            ));
            if (cancelled) {
                return;
            }
            drop();
            setFederatedSessions({});

            for (const [server, result] of opened) {
                if (!result.ok) {
                    // Pas de session chez lui — pas de jeton de notre compte de
                    // chez lui, il le refuse, ou il ne répond pas : on n'y
                    // verrait rien.
                    continue;
                }

                const channel = kernelChannel({ origin: server.link, token: result.session.token });
                channel.retain();
                const unsubscribe = channel.subscribe("sessions", null, (list) => {
                    // PRÉFIXÉ, comme partout : deux serveurs frappent les
                    // mêmes identifiants, et c'est ce préfixe qui dit à la
                    // page de streaming chez qui aller.
                    const found = (Array.isArray(list) ? list : []).map((session) => ({
                        ...session,
                        id: federatedId(server.id, session.id),
                        childSessionId: session.childSessionId
                            ? federatedId(server.id, session.childSessionId)
                            : undefined,
                        source: server.hostname,
                    }));
                    setFederatedSessions((previous) => ({ ...previous, [server.id]: found }));
                });
                subscriptions.push(() => {
                    unsubscribe();
                    channel.release();
                });
            }
        };
        listen();

        // Une session refusée chez quelqu'un se redemande : ses parties
        // reviennent dès que notre kernel en a ré-échangé une.
        const stopListening = onFederationSessionChanged(listen);

        return () => {
            cancelled = true;
            stopListening();
            drop();
        };
    }, []);

    const enrichedSessions = useMemo(
        () => [...sessions, ...Object.values(federatedSessions).flat()].map(
            (s) => ({ ...s, ownerDisplayName: displayNames[s.owner] || s.owner }),
        ),
        [sessions, federatedSessions, displayNames]
    );

    // SUR UN APPAREIL, LE POOL EST LA PARTIE EN COURS.
    //
    // C8-P1 : PLUS DE CHEMIN PARALLÈLE POUR LE TÉLÉPHONE.
    //
    // Il y avait ici un sondage de `deviceCurrentSession()` toutes les trois
    // secondes, parce que le kernel du téléphone ne servait pas la façade. Ce
    // repli FABRIQUAIT l'objet session — rôle, propriétaire, places, joueurs —
    // et c'est de là que sortaient les quatre pannes du 2026-09-04 : un
    // invité propriétaire de la partie d'un autre, « 1/1 » sur un jeu à
    // quatre manettes, le porteur devenu spectateur de sa propre partie, et
    // une session que plus personne ne pouvait arrêter.
    //
    // Le téléphone sert maintenant `/api/v1/facade/ws` comme le serveur, avec
    // la même forme et un `me` calculé depuis le jeton. Il n'y a donc plus
    // qu'un seul chemin, et c'est celui d'en dessous.

    // NOTRE KERNEL — chez nous, quoi que regarde la page (`homeOrigin`, qui ne
    // suit pas la surcharge d'une page de streaming fédérée : ce qui vit plus
    // longtemps qu'une page ne suit pas la surcharge d'une page).
    //
    // Sur un téléphone sans comptes, la page n'a pas de jeton : son propre
    // kernel la reconnaît, et le canal s'ouvre quand même (`kernelReachable`).
    const home = useKernelChannel(
        kernelReachable(token) ? { origin: homeOrigin(), token: token || "" } : null,
    );
    useKernelTopic(home, "sessions", null, (list) => setSessions(Array.isArray(list) ? list : []));

    return (
        <SessionPoolContext.Provider value={{ sessions: enrichedSessions, displayNames }}>
            {children}
        </SessionPoolContext.Provider>
    );
}

// useSessionPool() — the whole live list (both platforms).
export function useSessionPool() {
    return useContext(SessionPoolContext).sessions;
}

// useDisplayNames() — the raw {username: displayName} map, for a caller that
// needs to resolve several usernames at once (e.g. Watch.jsx's per-player
// "request slot" list, mapping over session.players) — a hook per username
// in a loop would violate the Rules of Hooks; plain object lookups on this
// map don't.
export function useDisplayNames() {
    return useContext(SessionPoolContext).displayNames;
}

// useSession(id) — a single session by id, or undefined if it doesn't exist
// (ended, or never did) — for a page that already knows which sessionId it
// cares about (Watch.jsx's ?sessionId=, a Dashboard thumbnail).
export function useSession(id) {
    const sessions = useSessionPool();
    return sessions.find((s) => s.id === id);
}

// hasOpenSlot(session) — true if a new player could join right now (used to
// decide "Rejoindre" vs "Regarder" — see the plan's take-control flow).
// openSlots is computed server-side in the unified model.
export function hasOpenSlot(session) {
    return !!session && session.openSlots > 0;
}

// nameIn(session, username) — MON NOM LÀ OÙ VIT CETTE PARTIE.
//
// Le kernel qui sert une session y joint `me.username` : le nom sous lequel
// IL connaît la personne qui demande. Chez nous, c'est le nôtre ; chez un
// serveur fédéré, celui de notre compte DE LÀ-BAS — et c'est lui que portent
// le propriétaire, les joueurs et les spectateurs de cette partie. Les
// comparer à notre nom d'ici rendait la partie étrangère à celui qui l'avait
// lancée : absente de « Vos sessions », sans moyen d'y revenir (vu le
// 2026-10-09, d'un serveur à un autre en fédération). Notre nom n'est qu'un
// repli, pour une session servie sans jeton.
export function nameIn(session, username) {
    return session?.me?.username || username || "";
}

// isOwnSession(session, username) — cette partie est-elle la mienne ?
export function isOwnSession(session, username) {
    const name = nameIn(session, username);

    return !!name && session?.owner === name;
}

// How long to wait for a sessionId to show up before concluding it genuinely
// does not exist. A session created an instant ago (Games.jsx/RemotePlay.jsx's
// Play button, right before navigating) needs a moment for the WS push to
// arrive; a stale/wrong/already-ended id never resolves at all.
const SESSION_LOOKUP_TIMEOUT_MS = 5000;

// useResolvedSession(sessionId) — resolves a sessionId coming from the URL to
// its session record, and reports when it is safe to conclude the session does
// not exist. Lives here, next to the pool it reads, rather than in a new shared
// module: both entry points already import this file, and a fresh module shared
// between two lazily-routed pages is exactly what broke the bundle once before
// (see the LoadingScreen duplication note in the streaming pages).
//
// Three things happen at once, which is why this is worth having in one place:
//
//  1. The pool lookup (useSession).
//  2. A REST fallback. The façade WS push and the session creation are
//     independent events, and reverse proxies (NPM) recycle long-lived WS
//     connections — a just-spawned session can be missing from the snapshot
//     when the page mounts. Poll the point-in-time snapshot until the pool
//     catches up.
//  3. A latch. The façade pushes a FULL fresh snapshot on ANY session change
//     anywhere in the system, and a snapshot can arrive with THIS session
//     momentarily missing while it is genuinely still running (confirmed live
//     2026-08-05: a real player stuck in a "connexion…" loop, the backend
//     showing a fresh WS every few seconds — the fingerprint of the page
//     unmounting and remounting on every flicker, tearing down a working
//     WebRTC connection each time). Once a session has been found, keep
//     returning it even if a later snapshot omits it; only the timeout below
//     concludes on a session that was NEVER found.
// `host` — cette page regarde-t-elle une session qui vit chez un AUTRE
// serveur ? (chantier 9, C9-P4d)
//
// Le pool app-wide écoute notre kernel et ceux des serveurs fédérés où l'on a
// un compte — sous des identifiants PRÉFIXÉS. La page de streaming, elle,
// cherche la session sous l'identifiant que l'hôte lui donne : elle écoute
// donc la liste de l'hôte directement, sur le canal qu'elle ouvre de toute
// façon chez lui pour y jouer (même origine, même jeton, même socket).
//
// L'ADRESSE ET LE JETON ARRIVENT DANS L'APPEL, jamais lus dans un état
// global : même règle que `api()` (voir api/spawnSessions.js, qui explique ce
// que la forme ambiante a coûté le 2026-09-08 — notre hôte recevant le jeton
// d'un autre, un 401 toutes les cinq secondes).
//
// UNE ÉCOUTE, PLUS UNE REPRISE BORNÉE. Il y avait ici huit questions à deux
// secondes d'intervalle, tant que l'enfant n'avait pas enregistré son miroir
// (`childManaged`). Ce drapeau n'existe plus — toute session est tenue par son
// child dès son lancement — et la liste de l'hôte arrive d'elle-même, à
// chaque changement.
//
// `waitingOn` — quelque chose de LÉGITIME est encore en cours, ne conclus pas.
//
// Le compte à rebours vaut cinq secondes, et c'est le bon ordre de grandeur
// pour « cet identifiant ne désigne rien ». Il est absurde pour « le jeu
// descend encore » : un jeu distant met des minutes à arriver avant que quoi
// que ce soit ne démarre. Sans cette réserve, la page renonçait — et, pire,
// elle renonçait à la SECONDE EXACTE où le téléchargement se terminait, parce
// que le verdict était déjà tombé cinq secondes après le clic et n'attendait
// qu'un rendu pour s'afficher (vu le 2026-09-08 : « Could not find the
// session in time » apparu pile à la fin du transfert).
//
// Ce n'est donc pas un interrupteur mais une HORLOGE : tant que l'appelant
// tient `waitingOn`, elle ne tourne pas ; quand il le lâche, elle repart de
// zéro — la session a droit à ses cinq secondes à partir du moment où on peut
// vraiment l'attendre.
export function useResolvedSession(sessionId, { host = null, waitingOn = null } = {}) {
    const { token: ownToken } = useAuth();
    // LE JETON DE L'HÔTE QU'ON VISE, quand la page en vise un (C9-P4c).
    //
    // Sans lui, la façade d'un serveur fédéré nous refusait — et le repli
    // SANS jeton prenait le relais : il rend bien la session, mais dépouillée
    // de `me`, puisque personne ne sait qui demande. La page se croyait donc
    // spectatrice de la partie qu'elle venait de lancer, et s'attachait à la
    // passerelle en `has_input: false, username: ""` (lu dans ses journaux le
    // 2026-09-07). Aucune erreur nulle part : juste une partie sans manette.
    // ET SURTOUT PAS LE NÔTRE QUAND LA PAGE VISE AILLEURS.
    //
    // Le repli sur notre jeton est juste chez nous et faux là-bas : la façade
    // d'en face le refuse (401 en boucle, lu dans la console d'un téléphone le
    // 2026-09-07), et on lui aura tout de même envoyé un jeton qui ne le
    // regarde pas. Sans compte chez lui, il reste la recherche publique —
    // qui rend la session mais pas `me` : on regarde, on ne joue pas, ce qui
    // est exactement la vérité de la situation.
    const hostToken = streamAuthToken();
    const token = hostToken || ownToken;
    const liveSession = useSession(sessionId);
    // Lazy initializer — when the session already sits in the app-wide pool
    // at mount time (the common case: navigated from Dashboard/Games/Watch,
    // not a session created this instant), the state is correct on the VERY
    // first render: no null → LoadingScreen → effect → re-render cycle.
    const [session, setSession] = useState(() => liveSession ?? null);
    const [isRequestTimeout, setRequestTimeout] = useState(false);

    useEffect(() => {
        if (liveSession) setSession(liveSession);
    }, [liveSession]);

    // PAS DE SONDAGE : LE CANAL POUSSE DÉJÀ.
    //
    // Il y avait ici une boucle REST à 1 Hz, censée couvrir « la session vient
    // d'être créée et n'a pas encore atteint le pool ». Elle ne servait à rien
    // pour quelqu'un d'authentifié : le centre de messages pousse la liste à
    // chaque changement ET se reconnecte seul (libs/kernelChannel.js). Attendre un
    // push en redemandant la même chose, c'est demander deux fois.
    //
    // Elle nuisait, même : elle ne distinguait pas « pas encore là » de
    // « refusé », donc un jeton non accepté donnait une boucle sans fin de
    // 401 — bruit dans lequel la vraie cause se perdait (2026-09-08).
    //
    // RESTE LE CAS QUE LE CANAL NE COUVRE PAS : arriver par un lien, sans
    // compte. Le centre de messages exige un jeton, un spectateur
    // n'en a pas — il demande donc SA session, UNE fois. Une seule requête,
    // pas une cadence : si elle n'existe pas, la redemander ne la créera pas.
    //
    // UNE FOIS NE SUFFISAIT PAS, ET LA SECONDE FOIS N'EST PAS UN SONDAGE.
    //
    // Le raisonnement ci-dessus tient — redemander en boucle ne fera pas
    // exister une session qui n'existe pas — mais il manquait un cas :
    // celui où elle n'existe PAS ENCORE. Quelqu'un ouvre le lien pendant que
    // l'hôte télécharge le jeu ; la question est posée trop tôt, la réponse
    // est « non », et plus personne ne la repose. Une fois la partie lancée,
    // sa page reste sur « connexion… » indéfiniment (reproduit le 2026-09-08
    // sur l'émulateur : le jeu tournait, la façade le servait, la page ne le
    // savait pas).
    //
    // Alors on écoute au lieu de redemander : l'appareil annonce déjà
    // « ça tourne » sur son flux de nouvelles, ouvert à tous, sans jeton. On
    // repose la question à CE moment-là, et à aucun autre.
    useEffect(() => {
        if (!sessionId || token || liveSession || session) {
            return undefined;
        }

        let cancelled = false;

        const ask = () => {
            getPublicSession(sessionId).then((result) => {
                if (!cancelled && result.ok && result.data) {
                    setSession(result.data);
                }
            });
        };

        ask();
        const unsubscribe = onDeviceGameStarted(ask);

        return () => {
            cancelled = true;
            unsubscribe();
        };
    }, [sessionId, token, liveSession, session]);

    // DES PRIMITIVES EN DÉPENDANCE, PAS L'OBJET. L'appelant construit
    // `{origin, token}` dans son rendu : s'accrocher à cet objet rouvrirait le
    // canal à chaque rendu, puisque son identité change à chaque fois.
    const hostOrigin = host?.origin || "";
    const hostAuth = host?.token || "";
    const hostChannel = useKernelChannel(
        sessionId && hostOrigin ? { origin: hostOrigin, token: hostAuth } : null,
    );

    // L'identifiant reçu est DÉJÀ celui de l'hôte (Streaming.jsx retire le
    // préfixe qui n'a de sens que chez nous), donc on compare tel quel.
    useKernelTopic(hostChannel, "sessions", null, (list) => {
        const found = Array.isArray(list) ? list.find((entry) => entry.id === sessionId) : null;
        if (found) {
            setSession(found);
        }
    });

    useEffect(() => {
        // `session` too, not just liveSession: the REST fallback can resolve
        // the session while the façade WS is down — the timer must not
        // conclude "not found" over a session it already holds. A null
        // sessionId (keytest synthetic id still spawning its real session,
        // see Streaming.jsx's useKeytestSessionId) must not start the clock
        // either.
        if (!sessionId || liveSession || session || waitingOn) {
            // Et on RETIRE un verdict déjà rendu : sans ça, un « introuvable »
            // prononcé avant que l'attente ne soit déclarée resterait vrai
            // pour toujours, et s'afficherait au premier rendu suivant.
            setRequestTimeout(false);

            return undefined;
        }

        const timeout = setTimeout(() => setRequestTimeout(true), SESSION_LOOKUP_TIMEOUT_MS);

        return () => clearTimeout(timeout);
    }, [sessionId, liveSession, session, waitingOn]);

    return { session, isRequestTimeout };
}
