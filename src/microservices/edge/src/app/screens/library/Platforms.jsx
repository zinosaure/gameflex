import React, { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import { can, isOwnSession, useAuth, useData, usePlatforms } from "../../libs/data.jsx";
import { focusElement, useScreenInput } from "../../libs/input.js";
import { goDown, recalledGame } from "../../libs/memory.js";
import { resolveUsers, subscribeFederatedDevices, wakeDevice } from "../../libs/api.js";
import { useDeviceStatus } from "../../engine/stream/remoteplay/hooks/use-device-status.js";
import { federationSession, peerHost, PLAY_BLOCK_LABELS, playBlock, useFederationSession } from "../../libs/federation.js";
import { federatedAccountPath } from "../../settings/profile/Federation.jsx";
import { federatedId } from "../../libs/federatedHost.js";
import { toast } from "../../shell/Shell.jsx";
import { streamingService } from "../../engine/stream/remoteplay/service/streaming.service.js";
import { playPath } from "../../libs/play.js";
import PlayLaunch from "../play/Launch.jsx";
import { useVerticalPager } from "../../libs/pager.js";
import { recalledConsole, rememberConsole, rememberTab, tabById } from "../../libs/tabs.js";
import { Topbar } from "../../shell/Shell.jsx";
import { ICON_BUTTON, STATUS_DOT, useButton } from "../../libs/ui.js";
import { ConsoleBanner, useVisual } from "./ConsoleVisual.jsx";
import SidePanel from "../../shell/SidePanel.jsx";
import { StreamPanel, streamOf, streamSummaryOf } from "./GameSettings.jsx";
import { withNetworkBitrate } from "../../engine/stream/shared/libs/network-probe.js";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { QuickTiles } from "../../bricks/QuickTiles.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { ITEM_SETTINGS_ICON } from "../../globals/icons.js";

// NIVEAU 1 — LES CONSOLES D'UN ONGLET, UNE PAR ÉCRAN, EN VERTICAL.
//
// Le porteur (2026-11-12) : « au lieu de faire le défilement en horizontal on va
// le faire en vertical » ; HAUT et BAS changent de console, sans jamais bouger
// le focus ; au doigt, l'écran suit le doigt et s'aimante ; à la molette, une
// console par geste (libs/pager.js, le même mouvement que la bibliothèque).
// Plus de bande des consoles en bas. « Bibliothèque » est fixée en bas à gauche ; à la
// manette, rien n'y prend la lumière — GAUCHE et DROITE ne font rien, tout passe par les
// raccourcis. Toucher l'image n'ouvre rien : seul le bouton agit (chantier 53, P12). Sous le nom, l'éditeur et l'année
// du registre, et l'avertissement d'une console expérimentale en petit dessous ;
// derrière, la bande des fonds de la console (ConsoleVisual.jsx). À la manette (le porteur, 2026-11-12) : A ouvre la
// bibliothèque où que soit la lumière (ses réglages : Réglages › Administration › Consoles) ;
// L2/R2 parcourent l'en-tête, Select les réglages, Start En direct (Shell.jsx).
//
// LES CONSOLES SANS JEU (Réglages › Application › Apparence, le porteur, 2026-11-12) — ni ici, ni chez un serveur
// fédéré (data.jsx, libraryCountOf) : si on les cache, le pager, HAUT/BAS et l'adresse ne voient plus que les autres ;
// l'adresse d'une console cachée mène à la plus proche qui reste ; un onglet qui n'en garde aucune le dit. Remote Play
// n'est pas concerné : ses écrans sont des appareils, pas des jeux.
//
// L'ÉCRAN D'UNE CONSOLE REMOTE PLAY (le porteur, 2026-11-14, chantier 53, P12 ; le déroulé de RemotePlay.jsx de la
// webapp) : sa grande image derrière (ConsoleVisual.jsx), puis, en bas, le bouton principal et l'engrenage — pas de
// recherche (« il n'y a pas de bouton search »). LE BOUTON SUIT L'ÉTAT DE LA CONSOLE, poussé par le canal d'état du
// service (use-device-status.js, pas de sondage) — pour celle d'un serveur fédéré, par le sujet `servers/<id>/devices`
// de notre kernel, qui relaie ce que ce serveur pousse (`#750`) :
//   - en veille : « Allumer » (« quand la console est stand by, le bouton Play devient "Turn on" »), qui envoie le
//     réveil ; puis « Allumage… », grisé, jusqu'à ce qu'elle se dise en ligne — 30 secondes au plus, comme la webapp ;
//   - en ligne : « Jouer » (« Quand le statut a changé pour online, alors le bouton Play s'affiche ») ;
//   - une partie en cours sur elle, à quelqu'un d'autre : « Occupée · Regarder », qui la rejoint en spectateur (le
//     porteur : « rajouter le texte "regarder {nom} jouer" risque d'être trop longue. Le mieux c'est de remplacer par un
//     texte générique ») ; la mienne : « Reprendre » ;
//   - hors d'atteinte : « Jouer », grisé (« si la console est hors d'atteinte. Alors le bouton est grisé »).
// L'ENGRENAGE (« le bouton paramètres (info) permet de choisir le flux et de voir les informations de la console
// seulement. Comme pour les game info mais avec moins d'information ») : RemotePanel, plus bas. Le flux y est celui
// de l'utilisateur, retenu dans ce navigateur pour CET appareil, comme celui d'un jeu pour sa console, et il part avec
// la connexion (le porteur, 2026-11-16 : « le flux des Remote Play, même en fédération, se règle dans la fiche de la
// console côté front. C'est l'utilisateur qui choisit. »).

// Le réveil attend au plus 30 s que la console se dise en ligne (WAKE_TIMEOUT_MS de RemotePlay.jsx).
const WAKE_TIMEOUT_MS = 30_000;

// D'OÙ VIENT UNE CONSOLE REMOTE PLAY : « Ce serveur », ou le serveur fédéré qui la prête — sur son écran, la petite ligne
// au-dessus de son nom, comme la provenance d'un jeu sur sa fiche (le porteur, 2026-11-16 : « pour Remoteplay, rajoute
// la source de la console comme pour les jeux » ; Library.jsx, Sheet), et la ligne « Provenance » de son engrenage.
const deviceOrigin = (device, servers, t) => {
    if (!device?.serverId) {
        return t("rp.sourceHere");
    }
    const server = (servers || []).find((entry) => String(entry.id) === String(device.serverId));

    return server?.hostname || server?.link || device.source || "";
};

export default function Platforms({ tab }) {
    const { t } = useTranslation();
    const button = useButton();
    const navigate = useNavigate();
    const location = useLocation();
    const params = useParams();
    const family = tabById(tab).family;
    const everything = usePlatforms();
    const { libraryCountOf, refreshCounts, servers, showEmpty, sessions, capabilities, consoles } = useData();
    // L'ÉTAT EN DIRECT DES CONSOLES REMOTE PLAY D'ICI (le canal d'état du service, comme la webapp) : il passe devant la
    // liste lue au démarrage.
    const [live, setLive] = useState({});
    const onDevices = useCallback((list) => setLive(Object.fromEntries((list || []).map((device) => [device.id, device]))), []);
    useDeviceStatus({ onDevicesUpdate: onDevices, onStatusUpdate: onDevices, enabled: family === "remote" && can(capabilities, "spawnsSessions") });
    const inTab = everything.filter((plat) => plat.family === family);

    // L'ÉTAT EN DIRECT DES CONSOLES D'UN SERVEUR FÉDÉRÉ (`#750`, le porteur : « Quand j'allume la PS5 d'un serveur
    // fédéré, ça me dit allumage mais l'état ne change jamais en "jouer" ») : un abonnement par serveur qui en prête une,
    // tant que l'onglet est ouvert. Sa liste, sous ses identifiants, se préfixe ici comme ses parties. Une console qu'il ne
    // pousse plus (lui perdu, elle retirée) n'est plus prête ; un serveur qui ne pousse rien garde l'état de la liste.
    const [federatedLive, setFederatedLive] = useState({});
    const lenders = [...new Set(inTab.map((plat) => plat.device?.serverId).filter(Boolean).map(String))].sort().join("|");

    useEffect(() => {
        if (family !== "remote" || !lenders) {
            return undefined;
        }
        const stops = lenders.split("|").map((serverId) => subscribeFederatedDevices(serverId, (list) => setFederatedLive((previous) => ({
            ...previous,
            [serverId]: Object.fromEntries(list.map((device) => [federatedId(serverId, device.id), device])),
        }))));

        return () => {
            stops.forEach((stop) => stop());
            setFederatedLive({});
        };
    }, [family, lenders]);

    const deviceOf = (plat) => {
        if (!plat?.device) {
            return null;
        }
        const pushed = plat.device.serverId ? federatedLive[String(plat.device.serverId)] : null;

        if (pushed) {
            const { status = "offline", hasActiveSession = false } = pushed[plat.device.id] || {};

            return { ...plat.device, status, hasActiveSession };
        }

        return { ...plat.device, ...(live[plat.device.id] || {}) };
    };
    const plats = showEmpty ? inTab : inTab.filter((plat) => plat.kind !== "console" || libraryCountOf(plat.console.id) !== 0);
    const { user } = useAuth();
    const currentId = useRef(null);
    const platIds = plats.map((plat) => plat.id).join("|");

    useEffect(() => {
        rememberTab(tab);
    }, [tab]);

    useEffect(() => {
        refreshCounts();
    }, [refreshCounts]);

    // L'index d'une console parmi celles affichées ; cachée (sans jeu), celui de la plus
    // proche qui reste dans l'ordre de l'onglet — la suivante d'abord, à égale distance.
    const locate = (id) => {
        const index = id ? plats.findIndex((plat) => plat.id === id) : -1;
        const position = index < 0 && id ? inTab.findIndex((plat) => plat.id === id) : -1;

        if (position < 0) {
            return index;
        }

        for (let distance = 1; distance < inTab.length; distance += 1) {
            for (const near of [inTab[position + distance], inTab[position - distance]]) {
                const found = near ? plats.indexOf(near) : -1;

                if (found >= 0) {
                    return found;
                }
            }
        }

        return -1;
    };

    const pager = useVerticalPager({
        count: plats.length,
        onTarget: (index) => {
            const plat = plats[index];
            currentId.current = plat?.id || null;

            if (plat) {
                rememberConsole(tab, plat.id);

                if (params.id !== plat.id) {
                    navigate(`/${tab}/${plat.id}`, { replace: true });
                }
            }
        },
    });

    // La position de départ : l'adresse (`/<onglet>/<id>`), sinon la console
    // retenue pour l'onglet, sinon la première ; une liste qui change (les
    // appareils Remote Play arrivent, une console perd son dernier jeu) garde la
    // console affichée, ou la plus proche. `jump` réécrit l'adresse (onTarget).
    useLayoutEffect(() => {
        let index = locate(currentId.current);

        if (index < 0) {
            index = locate(params.id);
        }

        if (index < 0) {
            index = Math.max(0, locate(recalledConsole(tab)));
        }
        pager.jump(index);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [platIds]);

    // Une navigation venue d'ailleurs (le Retour du navigateur) vers une autre console de l'onglet.
    useEffect(() => {
        const index = locate(params.id);

        if (index >= 0 && index !== pager.state.current.target) {
            pager.goTo(index);
        } else if (index >= 0 && plats[index].id !== params.id) {
            navigate(`/${tab}/${plats[index].id}`, { replace: true });
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [params.id]);

    // L'écran reste au point neutre : tout passe par les raccourcis.
    useEffect(() => {
        document.activeElement?.blur?.();
    }, []);

    const plat = plats[pager.target];
    // LA CONSOLE REMOTE PLAY D'UN AMI SANS COMPTE CHEZ LUI : « Connexion requise » (libs/federation.js).
    const shownPeer = plat?.kind === "device" ? deviceOf(plat)?.serverId || null : null;
    const shownPeerSession = useFederationSession(shownPeer);

    // LE NOMBRE DE JEUX de la console affichée, ceux que montre sa bibliothèque — ici et chez
    // chaque serveur fédéré (data.jsx, libraryCountOf, la même source que le choix de cacher
    // les consoles vides) : « Bibliothèque (0) » grisée sans jeu ; si le kernel n'a pas
    // répondu, « Bibliothèque » sans nombre, qui s'ouvre quand même.
    const count = plat?.kind === "console" ? libraryCountOf(plat.console.id) : null;

    // ALLUMER une console Remote Play (RemotePlay.jsx de la webapp, wakeDevice) : celle d'ici, ou celle
    // d'un serveur fédéré sous notre compte de là-bas (son identifiant chez lui, sans notre préfixe srv<id>:).
    // Le réveil envoyé, la console a 30 secondes pour se dire en ligne (WAKE_TIMEOUT_MS de la webapp) : le bouton dit
    // « Allumage… » d'ici là, puis « Jouer » dès que son état poussé la dit allumée ; l'attente finie sans elle, il
    // revient à son état.
    const [waking, setWaking] = useState({});

    useEffect(() => {
        const ids = Object.keys(waking);

        if (!ids.length) {
            return undefined;
        }
        const next = Math.min(...ids.map((id) => waking[id]));
        const timer = setTimeout(() => {
            const now = Date.now();
            setWaking((previous) => Object.fromEntries(Object.entries(previous).filter(([, until]) => until > now)));
        }, Math.max(0, next - Date.now()));

        return () => clearTimeout(timer);
    }, [waking]);

    // Allumée, l'attente est finie : une console qui se rendort ensuite redit « Allumer », pas « Allumage… ».
    useEffect(() => {
        const lit = Object.keys(waking).filter((id) => plats.some((entry) => entry.device?.id === id && deviceOf(entry)?.status === "awake"));

        if (lit.length) {
            setWaking((previous) => Object.fromEntries(Object.entries(previous).filter(([id]) => !lit.includes(id))));
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [waking, live, federatedLive]);

    const wake = async (device) => {
        let host = null;

        if (device.serverId) {
            const server = (servers || []).find((entry) => String(entry.id) === String(device.serverId));
            const opened = server ? await peerHost(server) : { ok: false };

            if (!opened.ok) {
                toast(t("rp.wakeNoAccount"));

                return;
            }
            host = opened.host;
        }
        const answer = await wakeDevice(String(device.id).replace(/^srv\d+:/, ""), host);
        toast(answer.ok ? t("rp.wakeSent") : answer.errorMessage || t("rp.wakeFailed"));

        if (answer.ok) {
            setWaking((previous) => ({ ...previous, [device.id]: Date.now() + WAKE_TIMEOUT_MS }));
        }
    };

    // CE QUE FAIT LE BOUTON D'UNE CONSOLE REMOTE PLAY (voir l'en-tête) : une partie qui tourne sur elle (le pool des
    // parties du kernel, la même preuve que Réglages › Remote Play), sinon son état.
    const remoteAction = (plat) => {
        const device = deviceOf(plat);
        const session = device ? sessions.find((entry) => entry.id === device.id) || null : null;

        if (session) {
            return isOwnSession(session, user?.username) ? { kind: "resume", session } : { kind: "watch", session };
        }

        // POURQUOI ELLE NE SE LANCE PAS (chantier 66) : console désactivée, fédération désactivée, connexion requise, accès
        // restreint — les mêmes mots que le bouton d'un jeu (playBlock). Sa session n'est lue que pour la console affichée.
        if (device) {
            const consoleId = `rp-${String(device.hostType || "").toLowerCase()}`;
            const registered = (consoles || []).find((entry) => entry.id === consoleId);
            const peer = device.serverId ? (servers || []).find((entry) => String(entry.id) === String(device.serverId)) || null : null;
            const block = playBlock({
                console: { id: consoleId, method: "remoteplay", disabled: Boolean(registered?.disabled) },
                user,
                peer,
                peerSession: peer && String(device.serverId) === String(shownPeer) ? shownPeerSession : null,
            });

            if (block) {
                return { kind: block, serverId: device.serverId };
            }
        }

        // UNE CONSOLE REMOTE PLAY EST À TOUT COMPTE VALIDE, sur un serveur comme sur un téléphone (`#757` ; le porteur,
        // 2026-11-17 : « Les consoles fédérées sont accessibles à tout le monde avec un compte. Comme pour le serveur ») :
        // `onlyAdminsLaunch` ne garde que ce qui tourne SUR le téléphone (hostRefusal, les jeux).
        if (device?.status === "awake") {
            return { kind: "play" };
        }

        if (device && waking[device.id] > Date.now()) {
            return { kind: "waking" };
        }

        return device?.status === "standby" ? { kind: "wake" } : { kind: "unreachable" };
    };
    const runRemote = (plat) => {
        const action = remoteAction(plat);
        const back = { state: { back: `${location.pathname}${location.search}` } };

        if (action.kind === "resume") {
            navigate(playPath(action.session.id, { play: true }), back);
        } else if (action.kind === "watch") {
            navigate(playPath(action.session.id), back);
        } else if (action.kind === "play") {
            connect(plat);
        } else if (action.kind === "wake") {
            wake(deviceOf(plat));
        } else if (action.kind === "signIn") {
            navigate(federatedAccountPath(action.serverId));
        }
    };
    const REMOTE_LABELS = { resume: "game.resume", watch: "rp.watchBusy", play: "game.play", waking: "rp.turningOn", wake: "rp.turnOn", unreachable: "game.play", ...PLAY_BLOCK_LABELS };
    // Les états où le bouton ne fait rien : il dit seulement pourquoi.
    const REMOTE_INERT = ["waking", "unreachable", "consoleDisabled", "federationDisabled", "relayDisabled", "accessRestricted"];
    const [panel, setPanel] = useState(null);

    // SE CONNECTER À UN APPAREIL D'ICI (chantier 52, P6) : la commande `launch` du kernel (streamingService
    // de la webapp, la même porte que sa page Remote Play — idempotente : une session qui tourne déjà pour
    // cet appareil revient telle quelle), puis la page de jeu du launcher. Le flux part avec elle : celui que
    // l'utilisateur a choisi dans l'engrenage de CET appareil dans ce navigateur, le défaut (720p à 60) pour le reste
    // (streamOf, comme le lancement d'un jeu), et un débit « Auto » mesuré vers le serveur qui tient la console — le
    // nôtre, ou celui de l'ami (withNetworkBitrate) : la console reçoit ce débit une fois, à l'ouverture de la partie.
    // Le kernel revalide le tout. L'écran du lancement le temps de la réponse (la console peut s'éveiller) ; Annuler
    // arrête la session qui en naîtrait.
    const [connecting, setConnecting] = useState(null);
    const connectRun = useRef(null);
    const connect = async (plat) => {
        const device = deviceOf(plat);
        const run = { cancelled: false };
        connectRun.current = run;
        setConnecting({ failure: null, conflict: null });
        let answer = null;
        let failure = null;

        // L'APPAREIL D'UN AMI : la même porte chez lui, sous notre compte de là-bas ; la session repart préfixée.
        let host = null;

        if (device.serverId) {
            const server = (servers || []).find((entry) => String(entry.id) === String(device.serverId));
            const opened = server ? await federationSession(server.id) : { ok: false };

            if (!opened.ok) {
                setConnecting({ failure: { step: null, error: t("rp.wakeNoAccount") }, conflict: null });

                return;
            }
            host = { serverId: server.id, origin: server.link, token: opened.session.token, username: opened.session.username };
        }
        const stream = await withNetworkBitrate(streamOf(plat.id), host);

        if (run.cancelled) {
            return;
        }

        try {
            answer = await streamingService.startSession(device.hostId, host ? host.username : user?.username, host, stream);
        } catch (error) {
            const result = error?.result;
            failure = result?.activeSessionId
                ? { conflict: { id: result.activeSessionId, title: result.activeSessionTitle } }
                : { error: /(init|ctrl) rejected/.test(error?.message || "") ? t("rp.cooldown") : error?.message || null };
        }
        const data = answer?.data || {};
        const sessionId = data.id || data.sessionId || data.session_id || null;

        if (run.cancelled) {
            return;
        }
        if (!sessionId) {
            setConnecting({ failure: failure?.conflict ? { step: null, error: null } : { step: null, error: failure?.error || answer?.errorMessage || null }, conflict: failure?.conflict || null });

            return;
        }
        setConnecting(null);
        navigate(playPath(host ? federatedId(host.serverId, sessionId) : sessionId, { play: true }), { state: { back: `${location.pathname}${location.search}` } });
    };

    // LE GESTE PRINCIPAL d'une plateforme — celui de son bouton, et de A : la bibliothèque d'une console (sur le dernier
    // jeu regardé, s'il y en a un) ; pour un appareil Remote Play (d'ici ou d'un ami), ce que dit son bouton
    // (remoteAction). Une console Remote Play s'enregistre dans Réglages › Remote Play, plus depuis cet écran.
    const openPlat = (item) => {
        if (!item) {
            return;
        }

        if (item.kind === "console") {
            const game = recalledGame(item.id);
            goDown(navigate, location, `/${tab}/${item.id}/games${game ? `/${encodeURIComponent(game)}` : ""}`);
        } else if (item.kind === "device") {
            runRemote(item);
        }
    };

    useScreenInput({
        // HAUT et BAS changent de console ; GAUCHE et DROITE ne font rien (« on passe que par des shortcuts »).
        direction: (direction) => {
            document.activeElement?.blur?.();

            if (plats.length && (direction === "up" || direction === "down")) {
                pager.step(direction === "down" ? 1 : -1);
            }

            return true;
        },
        // A : la bibliothèque (ou le bouton d'une console Remote Play), où que soit la lumière — sauf sur un autre
        // bouton touché ou cliqué, qui fait le sien.
        activate: (element) => {
            // Un bouton de l'en-tête en lumière (L2 / R2) fait le sien.
            if (!plat || element?.closest(".topbar") || (element?.closest(".pactions") && element.dataset.key !== "b-open" && element.dataset.key !== "b-rp")) {
                return false;
            }

            if (plat.kind !== "console" || count !== 0) {
                openPlat(plat);
            }

            return true;
        },
        // Y : l'engrenage d'une console Remote Play, comme celui d'un jeu.
        y: () => {
            if (plat?.kind === "device") {
                setPanel("remote");
            }
        },
        back: () => {},
    });


    if (!plats.length) {
        return (
            <main className="screen tabbed">
                <Topbar tab={tab} />
                <div className="gbody">
                    {inTab.length ? (
                        <div className="py-4">
                            <p className="mb-2">{t("platforms.noneWithGames")}</p>
                            <p className="small text-body-secondary mb-0">{t("platforms.noneWithGamesHint")}</p>
                        </div>
                    ) : <p className="text-body-secondary py-4">{t(family === "remote" ? "rp.noneRegistered" : "platforms.noneInTab")}</p>}
                </div>
            </main>
        );
    }

    return (
        <main className="screen platforms">
            <Topbar tab={tab} />
            {/* TOUCHER L'IMAGE NE FAIT RIEN (le porteur, 2026-11-14, chantier 53, P12 : « Toucher l'image ne dois pas lancer
                la console non plus. C'est pareil pour choix console. Seul le bouton doit ouvrir la page ») : un toucher sur
                l'écran d'une console envoyait le réveil d'une console Remote Play. Le doigt fait défiler les consoles ;
                seuls le bouton principal et l'engrenage agissent, et A à la manette. */}
            <div className="pager" ref={pager.hostRef}>
                {pager.visible.map((index) => (
                    <section key={plats[index].id} className="pslide" ref={pager.slideRef(index)}>
                        <Slide plat={plats[index]} device={deviceOf(plats[index])} />
                    </section>
                ))}
            </div>
            {/* Le geste de la console affichée, fixé en bas à gauche : Bibliothèque. Ses réglages sont dans
                Réglages › Administration › Consoles (le porteur, 2026-11-12) ; un appareil Remote Play a son bouton et son
                engrenage. */}
            <div className="pactions d-flex align-items-center justify-content-between gap-2">
                {/* UN BOUTON PORTE SOIT UN TEXTE, SOIT UNE ICÔNE (le porteur, 2026-11-13, bricks/) : Bibliothèque,
                    Jouer, Allumer sont des gestes, en texte ; la clé, une icône seule. */}
                {plat?.kind === "console" ? (
                    <>
                        <button type="button" className={`${button("main")} w-50`} data-nav data-key="b-open" disabled={count === 0} onClick={() => openPlat(plat)}>
                            {count === null ? t("console.library") : t("console.libraryCount", { count })}
                        </button>
                    </>
                ) : plat?.kind === "device" ? (
                    <>
                        {/* Le bouton suit l'état de la console (remoteAction) ; la clé, ses informations et son flux. */}
                        <button type="button" className={`${button("main")} w-50 text-truncate`} data-nav data-key="b-rp"
                            disabled={REMOTE_INERT.includes(remoteAction(plat).kind)} onClick={() => runRemote(plat)}>
                            {t(REMOTE_LABELS[remoteAction(plat).kind])}
                        </button>
                        <button type="button" className={`${ICON_BUTTON} ${button("round")}`} data-nav data-key="b-info" aria-haspopup="dialog"
                            aria-label={t("gear.title")} title={t("gear.title")} onClick={() => setPanel("remote")}>
                            <i className={`bi ${ITEM_SETTINGS_ICON}`}></i>
                        </button>
                    </>
                ) : null}
            </div>
            {panel === "remote" && plat?.kind === "device" && (
                <RemotePanel plat={plat} device={deviceOf(plat)} session={sessions.find((entry) => entry.id === plat.device.id) || null}
                    onClose={() => {
                        setPanel(null);
                        // Refermé, l'écran revient au point neutre : A y fait le bouton de la console.
                        document.activeElement?.blur?.();
                    }} />
            )}
            {connecting && (
                <div className="play">
                    <PlayLaunch label={t("rp.connecting")} failure={connecting.failure} conflict={connecting.conflict}
                        onConflict={() => {
                            const other = connecting.conflict;
                            setConnecting(null);
                            navigate(playPath(other.id, { play: true }), { state: { back: `${location.pathname}${location.search}` } });
                        }}
                        onCancel={() => {
                            if (connectRun.current) {
                                connectRun.current.cancelled = true;
                            }
                            setConnecting(null);
                        }} />
                </div>
            )}
        </main>
    );
}

// Une plateforme : son visuel, son nom, sa ligne (éditeur · année), l'avertissement
// d'une console expérimentale ; pour un appareil Remote Play, son état. Plus d'icône
// de console (le porteur, 2026-11-12 : « on les retire »).
function Slide({ plat, device }) {
    const { t } = useTranslation();
    const { sessions, servers } = useData();
    const visual = useVisual(plat);
    // Une partie en cours sur la console l'emporte sur « En ligne » : quelqu'un y joue.
    const busy = Boolean(device) && sessions.some((session) => session.id === device.id);

    return (
        <>
            <div className="pvis">{visual}</div>
            <div className="pshade"></div><div className="pdim"></div>
            <div className="pinfo">
                {/* Une console Remote Play : sa provenance, au-dessus de son nom, comme celle d'un jeu sur sa fiche. */}
                {device && <div className="fseries" data-key="p-origin">{deviceOrigin(device, servers, t)}</div>}
                <div className="ptitle">
                    <h1 className="pname">{plat.kind === "console" ? plat.console.name : plat.name}</h1>
                </div>
                {plat.kind === "console" && (plat.console.publisher || plat.console.year) && (
                    <div className="pline">
                        {plat.console.publisher && <span>{plat.console.publisher}</span>}
                        {plat.console.year && <span>{plat.console.year}</span>}
                    </div>
                )}
                {/* Une console expérimentale : son avertissement, toujours là, en petit (la manette n'ouvrait pas la bulle). */}
                {plat.kind === "console" && plat.console.experimental && <p className="pnote small mt-2 mb-0">{t("console.experimentalNote")}</p>}
                {device && (
                    <div className="pline">
                        <span><span className={`${STATUS_DOT} ${busy ? "bg-danger" : device.status === "awake" ? "bg-success" : device.status === "standby" ? "bg-warning" : "bg-secondary"}`}></span>{busy ? t("rp.status.busy") : t(`rp.status.${device.status}`, { defaultValue: device.status || "" })}</span>
                    </div>
                )}
            </div>
        </>
    );
}

// L'ENGRENAGE D'UNE CONSOLE REMOTE PLAY (le porteur, 2026-11-14, chantier 53, P12 : « le bouton paramètres (info) permet
// de choisir le flux et de voir les informations de la console seulement. Comme pour les game info mais avec moins
// d'information. ») — le panneau de l'engrenage d'un jeu, réduit :
//   - en tête, sa grande image (ConsoleBanner) ;
//   - UNE tuile, « Flux », qui ouvre le panneau du flux d'un jeu (StreamPanel, GameSettings.jsx) : le choix de
//     l'UTILISATEUR, retenu dans ce navigateur pour cet appareil (la clé de son écran, `plat.id`), qui part avec la
//     connexion (`connect`, plus haut). Une console d'ici et une console d'un serveur fédéré se règlent de la même façon,
//     par tout compte : plus de réglage au serveur, posé par un administrateur pour tout le monde (le porteur,
//     2026-11-16 : « C'est l'utilisateur qui choisit. ») ;
//   - « Informations » : son type, son adresse, son état, qui y joue (« En jeu », le nom que le service des comptes donne),
//     d'où elle vient.
function RemotePanel({ plat, device, session, onClose }) {
    const { t } = useTranslation();
    const { user } = useAuth();
    const { servers } = useData();
    const [sub, setSub] = useState(null);
    const [, redraw] = useState(0);
    const [player, setPlayer] = useState("");
    const busy = Boolean(session);

    // Qui y joue : son nom affiché, que le service des comptes donne ; son identifiant sinon.
    useEffect(() => {
        if (!session?.owner) {
            setPlayer("");

            return undefined;
        }
        let cancelled = false;
        setPlayer(session.owner);
        resolveUsers([session.owner]).then((answer) => {
            if (!cancelled && answer.ok && answer.item?.[session.owner]) {
                setPlayer(answer.item[session.owner]);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [session?.owner]);

    const closeSub = () => {
        setSub(null);
        setTimeout(() => focusElement(document.querySelector('.npanel [data-key="rt-stream"]'), true), 0);
    };
    const status = busy ? t("rp.status.busy") : t(`rp.status.${device.status}`, { defaultValue: device.status || "" });
    const tiles = [
        { id: "rt-stream", icon: "bi-broadcast", tone: "sky", label: t("gear.streamTile"), wide: true,
            value: streamSummaryOf(streamOf(plat.id), t), onClick: () => setSub("stream") },
    ];

    return (
        <>
            <SidePanel label={plat.name} className="gear" firstKey="p-close" onClose={onClose}>
                <ConsoleBanner plat={plat} />
                <QuickTiles items={tiles} />
                <Section title={t("details")} sense="info">
                    <Rows>
                        <InfoRow icon="bi-playstation" label={t("remoteplay.device.type")} value={device.hostType} />
                        <InfoRow icon="bi-router" label={t("remoteplay.device.ip")} value={device.ipAddress} />
                        <InfoRow icon={busy ? "bi-people" : "bi-power"} label={t("remoteplay.device.status")} value={status} />
                        {busy && <InfoRow icon="bi-person" label={t("rp.playing")} value={isOwnSession(session, user?.username) ? t("rp.playingYou") : player} />}
                        <InfoRow icon="bi-hdd-network" label={t("game.origin")} value={deviceOrigin(device, servers, t)} />
                    </Rows>
                </Section>
            </SidePanel>
            {sub === "stream" && (
                <StreamPanel choiceKey={plat.id} remotePlay onClose={closeSub} onChanged={() => redraw((value) => value + 1)} />
            )}
        </>
    );
}
