import React, { useEffect, useLayoutEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import { fromResource, hostRefusal, matchesQuery, useAuth, useData } from "../../libs/data.jsx";
import { listRoms, romSource } from "../../libs/api.js";
import { useScreenInput } from "../../libs/input.js";
import { goUp, rememberGame } from "../../libs/memory.js";
import { federationSession, PLAY_BLOCK_LABELS, playBlock, useFederationSession } from "../../libs/federation.js";
import { federatedAccountPath } from "../../settings/profile/Federation.jsx";
import { playPath, useLaunch } from "../../libs/play.js";
import { fact, originOf, STATUS_VARIANT, statusOf } from "../../libs/format.js";
import { widthFor } from "../../libs/images.js";
import { useLayoutKind } from "../../libs/layout.js";
import { useVerticalPager } from "../../libs/pager.js";
import { discName, gameIndexOf, groupGames, sourceOf } from "../../libs/games.js";
import { summaryOf, useGameMetadata } from "../../libs/metadata.js";
import { splitTitle } from "../../libs/visuals.js";
import { toast, Topbar } from "../../shell/Shell.jsx";
import SidePanel from "../../shell/SidePanel.jsx";
import { statusGroup, switchGroup } from "../../bricks/FilterGroups.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { PlaceholderRow } from "../../bricks/PlaceholderRow.jsx";
import { Section } from "../../bricks/Section.jsx";
import { hiddenOf, toggleHidden } from "../../globals/filters.js";
import { ICON_BUTTON, useButton, useDebounced } from "../../libs/ui.js";
import HeroArt, { GameCover, useGameVisual } from "./HeroArt.jsx";
import { biosChoice, ContinuePanel, coreChoice, GearPanel, streamOf, useConsoleFacts, useSaveRAM, useUserSaves } from "./GameSettings.jsx";
import PlayLaunch from "../play/Launch.jsx";
import { ITEM_SETTINGS_ICON } from "../../globals/icons.js";
import { pill, STATUS_PILL } from "../../bricks/Pill.jsx";

// NIVEAU 2 — LA BIBLIOTHÈQUE D'UNE CONSOLE : ses fiches, une par écran, qui
// défilent en vertical comme les consoles (le porteur, 2026-11-12 : « plus de
// listing par grille / ce sont les fiches des jeux qui défilent un par un »).
//
// En haut, une ligne : le retour au bord gauche, le nom de la console en titre,
// la recherche au bord droit (le porteur, 2026-11-12 : laisser voir l'image).
// La recherche ouvre le panneau de la liste — tous les jeux de ce qu'on voit, d'où
// l'on saute à une fiche —, qui porte aussi les sources, les statuts et la
// recherche (GameList). Une
// recherche ou un filtre actif se signale sur le bouton de recherche : les fiches
// ne suivent plus que les jeux retenus.
// En bas, Jouer à gauche (à la place et dans le style de
// Bibliothèque sur l'écran des consoles) et l'engrenage à droite — le disque à
// lancer se choisit dans l'engrenage.
//
// Une fiche ne montre que le titre, une ligne (éditeur · genres · année) et les
// badges. Le reste — ce qu'on sait du jeu, ses sources, et ses réglages (ceux de
// la fiche d'un jeu de la webapp) — est dans le panneau de l'engrenage,
// « Informations et réglages » (GameSettings.jsx).
//
// LA MANETTE (le porteur, 2026-11-12, à la vraie manette) : TOUT PASSE PAR LES
// RACCOURCIS. HAUT et BAS changent de jeu ; GAUCHE et DROITE ne font rien ; la
// fiche reste au point neutre (rien en lumière). A LANCE LE JEU (Jouer, Continuer,
// Reprendre : ce que ferait le bouton), Y ouvre les réglages du jeu, X la
// recherche, B ferme le panneau ouvert, sinon revient aux consoles. Au doigt et à
// la souris, tout se touche directement.
//
// JOUER dit ce qu'il fera. « Reprendre » quand une de MES parties tourne déjà avec
// ce jeu (elle se rejoint, comme depuis En direct) ; grisé sur tout autre jeu tant
// qu'une de mes parties tourne — une seule à la fois, la fiche dit laquelle.
// Sinon « Continuer » quand le jeu a des sauvegardes de ce compte : en rétro, avec
// des états, un panneau pour choisir l'état à reprendre (ou démarrer le jeu) ;
// avec sa seule carte mémoire, et sur une console récente, le jeu part tout de
// suite (la sauvegarde se charge d'elle-même). Sinon « Jouer ».
//
// L'ordre est alphabétique, le même que la liste. L'adresse suit le jeu
// (`/<onglet>/<console>/games/<ref>`, remplacée à chaque pas) : recharger la
// page garde le jeu ; la recherche, les sources et les statuts écartés restent
// dans la requête (`?q=`, `?nosource=`, `?nostatus=`). Seules la fiche courante et ses deux voisines existent (libs/pager.js),
// leurs images se chargent avec elles.
//
// JOUER LANCE LA PARTIE ET OUVRE LA PAGE DE JEU DU LAUNCHER (chantier 52 : libs/play.js, Play.jsx), avec
// ce qui a été réglé dans l'engrenage (GameSettings.jsx) — un jeu rétro ou d'une console récente, de la
// bibliothèque, d'un disque, ou d'un ami (lancé chez lui, sous notre compte de là-bas).

// La lettre d'un titre, « # » pour un titre qui ne commence pas par une lettre.
const letterOf = (title) => {
    const first = String(title || "").normalize("NFD").replace(/[̀-ͯ]/g, "").trim().charAt(0).toUpperCase();

    return first >= "A" && first <= "Z" ? first : "#";
};

// SON STATUT, pour les interrupteurs : celui que le serveur a calculé ; sans statut, le jeu n'a pas été essayé.
const statusKey = (rom) => statusOf(rom) || "untested";



export default function Library({ tab }) {
    const { t } = useTranslation();
    const button = useButton();
    const navigate = useNavigate();
    const location = useLocation();
    const { consoleId, reference } = useParams();
    const [searchParams] = useSearchParams();
    // LES SERVEURS FÉDÉRÉS DONT CE COMPTE VOIT LES JEUX (`shownServers`, data.jsx) : ceux qu'il a éteints dans Réglages ›
    // Application n'ont ni jeux, ni interrupteur de source ici, et ne se lisent pas (le porteur, 2026-11-16 : « chaque
    // utilisateur choisit quel serveur fédéré il a envie de voir les jeux »). Le Catalogue et les Réglages, eux, montrent
    // toujours tout.
    const { consoles, disks, shownServers: servers, serversRead, loadHere, ownSessions, capabilities } = useData();
    const { user, isAdmin } = useAuth();
    const console = consoles.find((entry) => entry.id === consoleId);
    const [query, setQuery] = useState(searchParams.get("q") || "");
    const debounced = useDebounced(query);
    // LES SOURCES ET LES STATUTS ÉCARTÉS (leurs interrupteurs coupés : tous allumés par défaut, le porteur, 2026-11-14,
    // chantier 53, P12) sont dans l'adresse, comme la recherche : revenir des réglages (l'engrenage du haut), ou
    // recharger, retrouve la même fiche, filtrée pareil.
    const hiddenSources = useMemo(() => hiddenOf(searchParams.get("nosource")), [searchParams]);
    const hiddenStatuses = useMemo(() => hiddenOf(searchParams.get("nostatus")), [searchParams]);
    const [answer, setAnswer] = useState({ loading: true, roms: [] });
    // Les jeux de chaque serveur fédéré, par son identifiant : `{ loading, roms, refusal }`.
    const [peers, setPeers] = useState({});
    const [panel, setPanel] = useState(null);
    const [, setSettingsVersion] = useState(0);
    const currentRef = useRef(reference || null);
    const home = `/${tab}/${consoleId}`;

    // La requête suit la recherche et la provenance. Construite sur l'adresse
    // d'à présent (et non celle du rendu) : la fiche courante peut avoir changé
    // le chemin juste avant, dans le même tour.
    const setParams = (changes) => {
        const next = new URLSearchParams(window.location.search);

        for (const [name, value, fallback] of changes) {
            if (value && value !== fallback) {
                next.set(name, String(value));
            } else {
                next.delete(name);
            }
        }
        const search = next.toString();
        navigate(`${window.location.pathname}${search ? `?${search}` : ""}`, { replace: true, state: location.state });
    };

    const firstQuery = useRef(true);

    useEffect(() => {
        if (firstQuery.current) {
            firstQuery.current = false;

            return;
        }
        setParams([["q", debounced, ""]]);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [debounced]);

    // LES SOURCES, UN INTERRUPTEUR CHACUNE (le porteur, 2026-11-14 : « des switches pour les sources (par défaut tous
    // sont checked) ») : la bibliothèque de ce serveur, chaque disque interne qui a des jeux de la console, chaque serveur
    // fédéré — ceux que le sélecteur de provenance proposait, un à la fois.
    const consoleDisks = useMemo(() => disks.filter((disk) => (disk.consoles?.[consoleId] || 0) > 0), [disks, consoleId]);
    const sources = [
        { value: "local", label: t("games.source.library"), help: t("game.originLibrary") },
        ...consoleDisks.map((disk) => ({ value: `disk:${disk.name}`, label: disk.name, help: t("games.source.diskHelp") })),
        ...servers.map((server) => {
            const read = peers[server.id];

            return {
                value: `peer:${server.id}`,
                label: server.hostname || server.link,
                help: !read || read.loading ? t("settings.loading") : read.refusal ? t(`games.peer.${read.refusal}`) : read.failed ? t("games.loadFailed") : t("game.originPeer"),
            };
        }),
    ];

    // Les jeux d'ici : la bibliothèque et les disques internes, réunis une fois pour la page (data.jsx, loadHere).
    useEffect(() => {
        let cancelled = false;
        setAnswer((previous) => ({ ...previous, loading: true }));
        loadHere(consoleId).then((result) => {
            if (!cancelled) {
                setAnswer({ loading: false, roms: result.roms || [], failed: !result.ok });
            }
        });

        return () => {
            cancelled = true;
        };
    }, [consoleId, loadHere]);

    // Les jeux de chaque serveur fédéré, chacun à son rythme : un serveur lent ou éteint ne retient ni la liste ni les
    // fiches, et son interrupteur dit pourquoi il n'a rien donné.
    useEffect(() => {
        let cancelled = false;
        setPeers(Object.fromEntries(servers.map((server) => [server.id, { loading: true, roms: [] }])));
        servers.forEach((server) => {
            listRoms(consoleId, { server: server.id }).then((result) => {
                if (cancelled) {
                    return;
                }
                const read = result.ok
                    ? { roms: (result.data || []).map((rom) => ({ ...rom, peer: server.id })) }
                    : result.status === 403 || result.status === 502 ? { roms: [], refusal: result.status === 403 ? "denied" : "offline" } : { roms: [], failed: true };
                setPeers((previous) => ({ ...previous, [server.id]: { loading: false, ...read } }));
            });
        });

        return () => {
            cancelled = true;
        };
    }, [consoleId, servers]);

    // Les jeux de ce qu'on voit : les entrées des sources allumées et des statuts allumés, cherchées, leurs disques
    // réunis, rangées par série (libs/games.js). UNE LETTRE SEULE TAPÉE garde les jeux qui commencent par elle, comme
    // la grille des lettres qu'elle remplace (le porteur, 2026-11-14 : « On ne gardera plus les alphabet. En typant la
    // lettre dans le search bar ça fera la même effet. ») ; davantage cherche dans le titre.
    const searched = debounced.trim();
    const letter = searched.length === 1 ? letterOf(searched) : null;
    const games = useMemo(() => {
        // Un jeu qu'un ami tient d'une de ses ressources externes ne se joue pas chez lui : il n'est que dans le catalogue.
        const roms = [...answer.roms, ...Object.values(peers).flatMap((read) => read.roms || [])]
            .filter((rom) => !fromResource(rom) && !hiddenSources.includes(sourceOf(rom)) && !hiddenStatuses.includes(statusKey(rom)) && (letter || matchesQuery(rom, searched)));
        const grouped = groupGames(roms);

        return letter ? grouped.filter((game) => letterOf(game.sortKey) === letter) : grouped;
    }, [answer.roms, peers, hiddenSources, hiddenStatuses, letter, searched]);
    // Le disque choisi de chaque jeu (par sa clé) ; le premier par défaut. Doublé
    // d'une référence, lue tout de suite par le pager (l'état n'arrive qu'au rendu suivant).
    const [chosenDiscs, setChosenDiscs] = useState({});
    const chosenRef = useRef({});
    const setDisc = (game, disc) => {
        chosenRef.current = { ...chosenRef.current, [game.key]: disc };
        setChosenDiscs(chosenRef.current);
    };
    const discOf = (game) => (game ? Math.min(chosenRef.current[game.key] ?? chosenDiscs[game.key] ?? 0, game.discs.length - 1) : 0);
    const romOf = (game) => game?.discs[discOf(game)];
    // L'adresse porte la référence du disque choisi : la suivre à chaque pas.
    const showRom = (rom) => {
        currentRef.current = rom.reference;
        rememberGame(consoleId, rom.reference);

        if (rom.reference !== reference) {
            navigate(`/${tab}/${consoleId}/games/${encodeURIComponent(rom.reference)}${window.location.search}`, { replace: true, state: location.state });
        }
    };

    const pager = useVerticalPager({
        count: games.length,
        onTarget: (index) => {
            const game = games[index];

            if (!game) {
                return;
            }
            showRom(romOf(game));
        },
    });

    // UNE FICHE DEMANDÉE ATTEND SES SOURCES (une adresse ouverte telle quelle, un lien, un rechargement) : tant qu'une
    // source lit encore et que le jeu de l'adresse n'est pas dans ce qui est arrivé, la fiche ne se cale pas sur un autre
    // jeu — elle oublierait la référence demandée, et le jeu d'un ami, qui arrive après ceux d'ici, ne se montrerait
    // jamais (défaut relevé le 2026-11-16, #748). La liste des serveurs elle-même compte : encore vide, elle ne dit pas
    // « aucun ami ».
    // Un serveur sans lecture encore posée (`peers` se remplit après le rendu où la liste des serveurs arrive) lit aussi.
    const sourcesReading = answer.loading || !serversRead || servers.some((server) => !peers[server.id] || peers[server.id].loading);
    const awaited = Boolean(currentRef.current) && sourcesReading && gameIndexOf(games, currentRef.current) < 0;

    // La fiche à montrer quand la liste change (arrivée, recherche, filtre) :
    // le jeu du disque courant s'il y est encore — ce disque choisi —, sinon le premier.
    useLayoutEffect(() => {
        if (answer.loading || awaited || !games.length) {
            return;
        }
        const index = currentRef.current ? gameIndexOf(games, currentRef.current) : -1;

        if (index >= 0) {
            setDisc(games[index], Math.max(0, games[index].discs.findIndex((rom) => rom.reference === currentRef.current)));
        }
        pager.jump(Math.max(0, index));
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [games, answer.loading, awaited]);

    const chooseDisc = (game, disc) => {
        setDisc(game, disc);
        showRom(game.discs[disc]);
    };

    // En entrant, rien n'est en lumière : le point neutre.
    useEffect(() => {
        document.activeElement?.blur?.();
    }, []);

    const game = games[pager.target];
    const rom = romOf(game);
    // Le compte lance-t-il cette console, et l'hôte la lance-t-il (un téléphone : son administrateur seul, ses propres
    // émulateurs — hostRefusal, chantier 56) ? La raison se lit dans l'engrenage (GameSettings.jsx, noLaunch).
    // L'hôte lance-t-il cette console (un téléphone : son administrateur seul, ses propres émulateurs) ? Le reste — coupée,
    // fédération, compte — se dit sur le bouton (playBlock).
    const playable = Boolean(console) && !hostRefusal(console, rom, capabilities, isAdmin);
    // UN JEU D'UN AMI SANS COMPTE CHEZ LUI : « Connexion requise », qui mène à la page de notre compte fédéré là-bas.
    const peerSession = useFederationSession(rom?.peer != null ? rom.peer : null);
    const peerServer = rom?.peer != null ? (servers || []).find((server) => String(server.id) === String(rom.peer)) || null : null;
    // LE BOUTON DIT POURQUOI IL NE JOUE PAS (chantier 66) : console désactivée, fédération désactivée, connexion requise,
    // accès restreint. Seule « Connexion requise » se touche : elle mène à la page du compte fédéré.
    const block = playBlock({ console, user, peer: peerServer, peerSession });
    const signIn = playable && block === "signIn";

    // CE QUE FERA JOUER (voir l'en-tête) : une de mes parties, les sauvegardes du compte.
    const facts = useConsoleFacts(consoleId);
    const saves = useUserSaves();
    const metadata = useGameMetadata(consoleId, game?.discs[0].reference, romSource(game?.discs[0]));
    const retro = console?.method === "retrogaming";
    const sram = useSaveRAM(consoleId, rom?.reference, Boolean(rom) && retro && !rom.peer);
    const sessionOf = (session) => session.reference || session.romReference;
    const running = game ? ownSessions.find((session) => session.consoleId === consoleId && game.discs.some((disc) => disc.reference === sessionOf(session))) : null;
    const busy = running ? null : ownSessions[0] || null;
    const states = rom && saves ? saves.states.filter((state) => state.consoleId === consoleId && state.reference === rom.reference).sort((left, right) => left.slot - right.slot) : [];
    const streamexSave = !retro && metadata?.gameId && saves
        ? saves.streamex.find((entry) => entry.id === metadata.gameId && String(entry.console).toLowerCase() === String(consoleId).toLowerCase()) || null
        : null;
    const hasSaves = retro ? states.length > 0 || Boolean(sram?.exists) : Boolean(streamexSave);

    // Lancer, avec ce qui a été réglé ici (GameSettings.jsx) — et l'état choisi ; l'écran du lancement
    // (screens/play/Launch.jsx) dès le clic, puis la page de jeu, qui ramène ici en partant.
    const launcher = useLaunch();
    const openPlay = (sessionId, options = {}) => navigate(playPath(sessionId, { play: true, ...options }), { state: { back: `${location.pathname}${location.search}` } });
    const launch = async (state = null) => {
        // UN JEU D'UN AMI se lance chez lui, sous notre compte de là-bas (libs/play.js, `host`).
        let host = null;

        if (rom.peer != null) {
            const server = (servers || []).find((entry) => String(entry.id) === String(rom.peer));
            const opened = server ? await federationSession(server.id) : { ok: false, code: "serverUnreachable" };

            if (!opened.ok) {
                toast(t(`games.peer.${opened.code}`, { defaultValue: t("games.peer.offline") }));

                return;
            }
            host = { serverId: server.id, origin: server.link, token: opened.session.token, username: opened.session.username };
        }
        const result = await launcher.start({
            host,
            console,
            rom,
            core: coreChoice(console, facts, game, rom).selected,
            bios: biosChoice(console, facts, game).selected,
            // Le flux de cette console dans ce navigateur, le défaut pour le reste ; son débit « Auto » se mesure
            // vers le serveur de la partie dans `start` (libs/play.js), l'écran du lancement déjà là.
            settings: streamOf(console.id),
            username: user?.username,
        });

        if (result.ok) {
            launcher.close();
            openPlay(result.sessionId, { launch: result.launchId, state });
        }
    };

    const play = () => {
        if (signIn && !running) {
            navigate(federatedAccountPath(rom.peer));
        } else if (running) {
            openPlay(running.id);
        } else if (retro && states.length) {
            setPanel("continue");
        } else {
            launch();
        }
    };

    // Un interrupteur basculé : la liste des écartés (`nosource`, `nostatus`) gagne ou perd sa valeur.
    const toggleIn = (name, hidden) => (value, on) => setParams([[name, toggleHidden(hidden, value, on) || "", ""]]);

    useScreenInput({
        // HAUT et BAS changent de jeu ; GAUCHE et DROITE ne font rien (le porteur, 2026-11-12 :
        // « on passe que par des shortcuts »).
        direction: (direction) => {
            document.activeElement?.blur?.();

            if (direction === "up" || direction === "down") {
                pager.step(direction === "down" ? 1 : -1);
            }

            return true;
        },
        // A lance le jeu ; un bouton de l'en-tête en lumière (L2 / R2) fait le sien.
        activate: (element) => {
            if (rom && !element?.closest(".topbar")) {
                if (running || signIn || (!busy && playable && !block)) {
                    play();
                }

                return true;
            }

            return false;
        },
        y: () => {
            if (rom) {
                setPanel("gear");
            }
        },
        x: () => setPanel("list"),
        // B : éteint une lumière (un bouton touché, cliqué), sinon revient aux consoles.
        back: () => {
            const element = document.activeElement;

            if (element && element !== document.body && element.closest(".screen")) {
                element.blur();
            } else {
                goUp(navigate, location, home);
            }
        },
    });

    if (!console) {
        return <main className="screen tabbed"><Topbar tab={tab} /><p className="text-body-secondary py-4">{t("games.unknownConsole")}</p></main>;
    }
    const narrowed = hiddenSources.filter((value) => sources.some((item) => item.value === value)).length + hiddenStatuses.length + (query.trim() ? 1 : 0);
    // TANT QU'UNE SOURCE LIT ENCORE, une liste vide n'est pas « Aucun jeu » : un téléphone sans jeu à lui n'a que ceux de
    // ses serveurs, qui arrivent après les siens — la première liste d'un serveur re-télécharge sa copie quand elle a
    // vieilli (2026-11-15). Des jeux déjà là se montrent sans attendre les autres sources — sauf quand l'adresse en demande
    // un qui n'est pas encore arrivé (`awaited`).
    const reading = answer.loading || awaited || (!games.length && Object.values(peers).some((read) => read.loading));
    // Rien à montrer, sans filtre, et un serveur fédéré n'a pas répondu : le dire plutôt qu'« Aucun jeu trouvé » — le
    // bouton de la console comptait ses jeux (data.jsx, libraryCountOf). Son interrupteur, dans la liste, dit lequel.
    const silent = !narrowed && Object.values(peers).some((read) => read.refusal === "offline" || read.failed);
    let empty = null;

    if (reading) {
        empty = t("games.loading");
    } else if (!games.length) {
        empty = answer.failed ? t("games.loadFailed") : silent ? t("games.peersSilent") : t("games.none");
    }

    return (
        <main className="screen library">
            <Topbar tab={tab} />
            {/* La ligne du haut : le retour, le nom de la console, la recherche (qui ouvre la liste). */}
            <div className="lbar d-flex align-items-center justify-content-between gap-3">
                <button type="button" className={`${ICON_BUTTON} ${button("round")}`} data-nav data-key="lb-back" data-sound="back" aria-label={t("back")} title={t("back")} onClick={() => goUp(navigate, location, home)}><i className="bi bi-chevron-left"></i></button>
                <h1 className="ltitle flex-grow-1 m-0 text-truncate">{console.name}</h1>
                <button type="button" className={`${ICON_BUTTON} position-relative ${button("round")}`} data-nav data-key="search" aria-haspopup="dialog"
                    aria-label={narrowed ? t("library.searchActive", { count: narrowed }) : t("games.search")} title={t("games.search")} onClick={() => setPanel("list")}>
                    <i className="bi bi-search"></i>
                    {narrowed > 0 && <span className="position-absolute top-0 start-100 translate-middle badge rounded-pill text-bg-danger">{narrowed}</span>}
                </button>
            </div>
            {empty && <p className="position-absolute top-50 start-0 end-0 px-4 text-center text-body-secondary">{empty}</p>}
            <div className="lstage">
                <div className="pager" ref={pager.hostRef}>
                    {!empty && pager.visible.map((index) => (
                        <section key={games[index].key} className="pslide" ref={pager.slideRef(index)}>
                            <Sheet console={console} game={games[index]} position={index + 1} rom={romOf(games[index])} />
                        </section>
                    ))}
                </div>
                {rom && !empty && (
                    <div className="pactions d-flex align-items-center justify-content-between gap-2">
                        {/* UN BOUTON PORTE SOIT UN TEXTE, SOIT UNE ICÔNE (le porteur, 2026-11-13, bricks/) : Jouer est un
                            geste, en texte ; la clé, une icône seule. */}
                        <button type="button" className={`${button("main")} w-50 text-truncate`} data-nav data-key="f-play" disabled={!running && !signIn && (Boolean(busy) || !playable || Boolean(block))} onClick={play}>
                            {t(running ? "game.resume" : block ? PLAY_BLOCK_LABELS[block] : hasSaves ? "game.continue" : "game.play")}
                        </button>
                        {/* La clé : les informations et les réglages du jeu (GameSettings.jsx). */}
                        <button type="button" className={`${ICON_BUTTON} ${button("round")}`} data-nav data-key="f-settings" aria-haspopup="dialog"
                            aria-label={t("gear.title")} title={t("gear.title")} onClick={() => setPanel("gear")}>
                            <i className={`bi ${ITEM_SETTINGS_ICON}`}></i>
                        </button>
                    </div>
                )}
            </div>
            {launcher.screen && (
                <div className="play">
                    <PlayLaunch label={t("play.launching")} steps={launcher.screen.steps} failure={launcher.screen.failure} backdrop={launcher.screen.backdrop}
                        conflict={launcher.screen.conflict} onConflict={() => {
                            const other = launcher.screen.conflict;
                            launcher.close();
                            openPlay(other.id);
                        }} onCancel={launcher.cancel} />
                </div>
            )}
            {panel === "gear" && game && <GearPanel console={console} game={game} rom={rom} metadata={metadata} facts={facts} playable={playable}
                states={states} sram={sram} streamexSave={streamexSave} onChanged={() => setSettingsVersion((value) => value + 1)}
                disc={discOf(game)} discsLocked={Boolean(busy)} onDisc={(disc) => chooseDisc(game, disc)}
                onClose={() => {
                    setPanel(null);
                    // Refermé, la fiche revient au point neutre : A y lance le jeu.
                    document.activeElement?.blur?.();
                }} />}
            {panel === "continue" && game && <ContinuePanel console={console} rom={rom} states={states}
                onChoose={(slot) => {
                    setPanel(null);
                    launch(slot);
                }}
                onClose={() => {
                    setPanel(null);
                    // Refermé, la fiche revient au point neutre : A y lance le jeu.
                    document.activeElement?.blur?.();
                }} />}
            {panel === "list" && <GameList console={console} games={games} current={pager.target} loading={reading} failed={Boolean(answer.failed)}
                query={query} onQuery={setQuery} sources={sources} hiddenSources={hiddenSources} onSource={toggleIn("nosource", hiddenSources)}
                hiddenStatuses={hiddenStatuses} onStatus={toggleIn("nostatus", hiddenStatuses)}
                onChoose={(index) => {
                    setPanel(null);
                    pager.jump(index);
                    requestAnimationFrame(() => document.activeElement?.blur?.());
                }}
                onClose={() => {
                    setPanel(null);
                    // Refermé, la fiche revient au point neutre : A y lance le jeu.
                    document.activeElement?.blur?.();
                }} />}
        </main>
    );
}

// UNE FICHE : le visuel du jeu, son titre, son logo, ce qu'on sait de lui.
// Le fond suit la règle de repli du plan 51 : background → capture → la jaquette
// ou l'icône en panneau sur son reflet → un dégradé tiré de l'identifiant, le titre
// en filigrane. Le titre est TOUJOURS là, logo ou pas (le porteur, 2026-11-12 : « ça
// restera cohérent ») — raccourci sur deux lignes, plus petit quand il est long,
// jamais coupé —, ses mentions (disque, région) dessous, une ligne (éditeur · genres ·
// année · joueurs) et les badges ; le reste est
// dans l'engrenage. Un jeu de plusieurs disques : le visuel et les informations
// sont ceux du premier (la fiche ne change pas d'image quand on choisit un
// disque) ; notre titre est celui du disque choisi. Au-dessus du titre, une petite
// ligne : « #460 • Gamebox • Final Fantasy » — la place du jeu dans ce qu'on voit, sa
// provenance (les noms du sélecteur de provenance, en court), sa série. Une autre de mes parties qui tourne grise
// Jouer, sans message (le porteur, 2026-11-14 : « Le message current running game. Tu l'enlève aussi. Tu juste grisé
// les boutons. Et seul le jeu en cours le bouton Play devient "Reprendre". On n'a pas besoin de message car dans la page
// d'accueil on voit déjà notre session en cours »).
//
// LE LOGO, une image en plus, jamais à la place du titre : en portrait, en tête de
// la fiche ; en paysage, à droite, centré dans la hauteur libre entre la barre et la
// ligne du bas, le texte à gauche. Sans logo, le texte seul.
// La fiche tient entre la ligne de la position et la ligne du bas : jamais sous
// la barre. Les écarts sont ceux de Bootstrap (gap-*), les mêmes entre chaque ligne.
function Sheet({ console, game, position, rom }) {
    const { t } = useTranslation();
    const { servers } = useData();
    const portrait = useLayoutKind() === "portrait";
    const first = game.discs[0].reference;
    // La source du jeu (un disque, un serveur fédéré) sur chacun de ses appels (libs/api.js, romSource).
    const source = romSource(game.discs[0]);
    const width = widthFor(Math.max(innerWidth, innerHeight * 16 / 9));
    const parts = splitTitle(rom.title);
    // Le visuel et le logo, la règle du jeu définie une fois (HeroArt.jsx) : le filigrane est le titre du disque choisi.
    const { visual, logo } = useGameVisual(console.id, first, source, parts.main, width);
    const known = summaryOf(useGameMetadata(console.id, first, source));
    const discIndex = Math.max(0, game.discs.indexOf(rom));
    const status = statusOf(rom);
    // LE NOMBRE DE JOUEURS, quand une source le sait (le porteur, 2026-11-14 : « si nombre de joueurs dispo, on le rajoute
    // aussi sur la fiche du jeu ») — sur la ligne des informations, après l'année.
    const subtitle = [known.publisher, known.genres.join(", "), known.year, known.players ? t("game.playerCount", { count: known.players }) : ""].filter(Boolean);
    const origin = originOf(rom, servers, t);

    return (
        <>
            <div className="pvis">{visual}</div>
            <div className="pshade"></div><div className="pdim"></div>
            <div className={`pinfo sbody d-flex ${portrait ? "flex-column justify-content-end gap-2" : "flex-row gap-4"}`}>
                {/* Le texte, le même avec ou sans logo : la petite ligne, le titre (le nôtre,
                    celui de la liste), ses mentions (disque, région), la ligne d'informations,
                    les badges. */}
                <div className="stext d-flex flex-column justify-content-end align-items-start gap-2">
                    {/* Une seule petite ligne au-dessus du titre : la place, la provenance, la série. */}
                    <div className="fseries">{[`#${position}`, origin, game.seriesSize >= 2 ? game.series : ""].filter(Boolean).join(" • ")}</div>
                    <h1 className={`htitle ${parts.main.length > 24 ? "long" : ""}`}>{parts.main}</h1>
                    {parts.tags && <p className="ownTitle m-0">{parts.tags}</p>}
                    {subtitle.length > 0 && <div className="fmeta m-0">{subtitle.map((part) => <span key={part}>{part}</span>)}</div>}
                    <div className="d-flex flex-wrap gap-2">
                        {status && <span className={`badge rounded-pill text-bg-${STATUS_VARIANT[status] || "secondary"}`}>{t(`status.${status}`)}</span>}
                        {/* Les disques : « 4 disques », ou le disque choisi quand ce n'est pas le premier (« Disque 2 / 4 »). */}
                        {game.discs.length > 1 && (
                            <span className="badge rounded-pill text-bg-secondary d-inline-flex align-items-center gap-2"><i className="bi bi-disc"></i>{discIndex > 0
                                ? t("library.discOf", { name: discName(rom, discIndex), count: game.discs.length })
                                : t("library.discCount", { count: game.discs.length })}</span>
                        )}
                        {console.experimental && <span className="badge rounded-pill text-bg-primary">{t("console.experimental")}</span>}
                    </div>
                </div>
                {/* Le logo : en tête en portrait, à droite en paysage (HeroArt.jsx). */}
                <HeroArt url={logo} portrait={portrait} />
            </div>
        </>
    );
}

// LA LISTE DE TOUS LES JEUX DE CE QU'ON VOIT, ouverte par la recherche — et ses filtres, réunis en un seul panneau (le
// porteur, 2026-11-14, chantier 53, P12 : « dans search. On rajoute une section avec: des switches pour les sources (par
// défaut tous sont checked), les 4 switches pour le status (par défaut tous checked) et le search bar. On ne gardera plus
// les alphabet. En typant la lettre dans le search bar ça fera la même effet. — puis dans le listing on aura comme quand
// on liste des jeux depuis console sauf que les sous-titres, on aura la source, le status et le nom de la licence (série)
// si existe. — ce sera un listing qu'on utilise par défaut, du coup la pagination sera déjà présente. — et plus besoin du
// nombre de jeux dans le titre. »). De haut en bas :
//   - « Sources » puis « Statut » (« au dessus de la section status il faut mettre le titre ») : un interrupteur par source
//     et par statut, tous allumés au départ (des formulaires, sans traits) ; ils filtrent la liste ET les fiches, et
//     refermer le panneau les garde (l'adresse) ;
//   - « Les jeux » : la Listing des Réglages (bricks/) — sa recherche et « N jeux », un trait, puis les lignes : la
//     jaquette, le titre, en sous-titre « source · statut · série » (la série quand elle réunit plusieurs jeux, comme sur
//     la fiche), sa pagination et ses fantômes ; elle s'ouvre sur la page du jeu affiché.
// Choisir un jeu y saute directement. À la manette, la croix parcourt les interrupteurs, la recherche, les jeux et la
// pagination ; la lumière part du jeu affiché ; A choisit, B referme (SidePanel).
const PAGE_SIZE = 10;
const pageOf = (index) => Math.floor(Math.max(0, index) / PAGE_SIZE) + 1;

function GameList({ console, games, current, loading, failed, query, onQuery, sources, hiddenSources, onSource, hiddenStatuses, onStatus, onChoose, onClose }) {
    const { t } = useTranslation();
    const { servers } = useData();
    const [page, setPage] = useState(() => pageOf(current));
    const items = useMemo(() => games.map((game, index) => ({ game, index })), [games]);
    const firstResults = useRef(true);

    // D'autres résultats (une recherche, un interrupteur), ou un autre jeu affiché (la fiche suit la recherche) : la page
    // du jeu affiché.
    useEffect(() => {
        if (firstResults.current) {
            firstResults.current = false;

            return;
        }
        setPage(pageOf(current));
    }, [games, current]);

    return (
        <SidePanel label={t("library.gamesTitle")} className="glist" firstKey={games[current] ? `gl-${games[current].key}` : "gl-search"} onClose={onClose}>
            {/* LE LISTING D'ABORD, SES FILTRES DERRIÈRE SON BOUTON (chantier 81 : le listing en premier, avec sa recherche et
                son filtre, comme toute liste — Listing, FilterGroups) : la recherche et « N jeux », un trait, puis les jeux. */}
            <Section title={t("library.gamesTitle")} help={t("library.gamesHelp")} sense="play">
                <Listing items={items} loading={loading} search={query} onSearch={onQuery} placeholder={t("games.search")} searchKey="gl-search"
                    filterCount={hiddenSources.length + hiddenStatuses.length} filters={[
                        switchGroup({ key: "sources", title: t("library.sourcesTitle"), id: "src", choices: sources, hidden: hiddenSources, onChange: onSource }),
                        statusGroup({ t, id: "st", hidden: hiddenStatuses, onChange: onStatus }),
                    ]}
                    page={page} onPage={setPage} pageSize={PAGE_SIZE}
                    found={t("library.total", { count: games.length })} empty={failed ? t("games.loadFailed") : t("games.none")}
                    placeholderRow={() => <PlaceholderRow lead="cover" />}
                    render={({ game, index }) => {
                        const rom = game.discs[0];

                        return (
                            <NavRow key={game.key} id={`gl-${game.key}`} lead={<GameCover consoleId={console.id} reference={rom.reference} game={romSource(rom)} />}
                                label={game.discs.length > 1 ? splitTitle(game.title).main : game.title}
                                help={[fact(t("game.origin"), originOf(rom, servers, t)), pill(t(`status.${statusKey(rom)}`), STATUS_PILL[statusKey(rom)])]}
                                onOpen={() => onChoose(index)} />
                        );
                    }} />
            </Section>
        </SidePanel>
    );
}
