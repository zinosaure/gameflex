import React, { useEffect, useMemo, useRef, useState } from "react";
import { Carousel } from "react-bootstrap";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate } from "react-router-dom";
import { LAUNCH_BACKGROUND } from "../../engine/stream/shared/launch-backdrop.js";
import { SessionStream } from "../../engine/stream/shared/SessionStream.jsx";
import { stopSession } from "../../engine/api/spawnSessions.js";
import { kernelCommand, romSource } from "../../libs/api.js";
import { isOwnSession, useAuth, useData } from "../../libs/data.jsx";
import { useDownloadNotices } from "../../libs/downloads.js";
import { federatedHostOf, parseFederatedId } from "../../libs/federatedHost.js";
import { counted, fact, formatDuration } from "../../libs/format.js";
import { probeGameImage } from "../../libs/images.js";
import { focusElement, useScreenInput } from "../../libs/input.js";
import { playPath } from "../../libs/play.js";
import { useFederatedSessions } from "../../libs/SessionPool.js";
import { useNames } from "../../libs/displayNames.js";
import { gameScreenPath, rememberTab } from "../../libs/tabs.js";
import { MUTED, SQUARE_ICON_BUTTON, useButton } from "../../libs/ui.js";
import { hash, randomFrom, splitTitle } from "../../libs/visuals.js";
import { DownloadRow } from "../../bricks/DownloadRow.jsx";
import { EmptyRow } from "../../bricks/EmptyRow.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { PlaceholderRow } from "../../bricks/PlaceholderRow.jsx";
import { RecentNotices, RequestRow } from "../../bricks/RecentNotices.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SplitRow } from "../../bricks/SplitRow.jsx";
import { useConfirm } from "../../bricks/useConfirm.jsx";
import { helpLines } from "../../globals/subtitles.jsx";
import { favoriteKey } from "../../libs/favorites.js";
import { GameBanner, GameCover } from "../library/HeroArt.jsx";
import { FavoriteRow, favoritePath, useFavoriteList } from "../../settings/profile/Favorites.jsx";
import { useSearchParam } from "../../settings/SettingsPage.jsx";
import { Topbar, toast } from "../../shell/Shell.jsx";

// L'ONGLET EN DIRECT, l'accueil (Start y ramène). Une colonne, dans la boîte des Réglages, dans cet ordre (le porteur,
// 2026-11-14 : « Il faut qu'on ait en premier ma session et les lives en cours […] Côté notifications et download, on
// mets par défaut aussi des placeholders. Mais c'est plus bas. ») :
//   1. MA PARTIE EN COURS, quand vous en avez une (« Vos sessions » de l'ancien tableau de bord) : une ligne par partie,
//      comme une ligne de Mes favoris (le porteur, 2026-11-16 ; OwnSessionRow) — la jaquette, le titre, la durée et le
//      temps restant avant l'arrêt pour inactivité ; la toucher rouvre la page de jeu pour y jouer ; l'arrêt (une icône
//      dans un bouton carré, au bord droit : le porteur, 2026-11-14) arrête la partie pour tout le monde, après UNE
//      fenêtre de confirmation de Skeleton. Pas le sélecteur de « Quitter la partie » : on n'est pas dans la partie, il
//      n'y a rien à quitter, et l'arrêt est le seul geste du bouton ;
//   2. EN DIRECT, les parties des autres (« Live now ») : Rejoindre quand il reste une manette, Regarder toujours ; un
//      administrateur peut arrêter la partie d'un autre, après DEUX fenêtres (useConfirm, `then`), la seconde disant qui
//      sera coupé et ce qui est perdu. Le kernel le permet (mayEndSession, la file de la partie : ByAdmin). Personne
//      d'autre ne joue : la ligne grisée d'une liste vide ;
//      LES PARTIES DES SERVEURS FÉDÉRÉS où ce compte a collé son jeton (useFederatedSessions, comme le tableau de bord
//      de la webapp) s'y mêlent, avec le nom du serveur sur leur ligne : elles se regardent et se rejoignent chez lui,
//      sous notre compte de là-bas, et la nôtre s'y arrête ; nos droits d'administrateur s'arrêtent à nos parties.
//      SANS JETON COLLÉ (`#747`), notre kernel les relaie (le sujet `servers/<id>/sessions`) : elles se regardent
//      seulement (`watchOnly`) — « Regarder », jamais « Rejoindre », et jamais « votre partie », même sous un nom
//      pareil au nôtre ;
//      QUAND AUCUNE PARTIE NE TOURNE, la vôtre comprise (le porteur : « Seulement quand il n'y a rien en cours que
//      affiche des images de jeux random »), la vitrine : un carrousel de cinq jeux de la bibliothèque (rétro ou
//      Streamex), pris parmi ceux qui ont un fond ou une jaquette ; « Voir le jeu » mène à sa fiche, comme dans la
//      Bibliothèque ; sans aucun jeu, l'image de l'intro de GameFlex (le fond de l'écran de lancement) ;
//   3. MES FAVORIS (le porteur, 2026-11-16 : « dans l'accueil 'Live', rajoute en dessous de live now, la liste des favoris
//      (pagination de 10 par 10) avec lien direct sur la fiche du jeu ») : ceux du compte, lus chez leur source comme
//      Réglages › Profil › Mes favoris les lit (useFavoriteList), dix par page (sa page dans l'adresse, `?favorites=`) ;
//      chaque ligne mène à la fiche du jeu dans la Bibliothèque — un favori dont le jeu n'est plus là, à sa page dans Mes
//      favoris, qui dit pourquoi et permet de le retirer. Vide, sa boîte reste : la ligne grisée, et le Catalogue ;
//   4. LES NOTIFICATIONS : les invitations reçues sur le canal de présence (la cloche n'existe plus ; elle compte sur
//      l'onglet et mène ici, `#notifications`) — une liste vide garde sa boîte et sa ligne grisée ;
//   5. LES TÉLÉCHARGEMENTS de ce compte, tels que le kernel les garde (libs/downloads.js) : ceux qui tournent et leur
//      progression, puis la file des jeux et leur rang (le porteur, 2026-11-16) ; chacun sa ligne « Annuler », ou
//      « Retirer de la file » ; vide, sa ligne grisée.
// La partie d'un autre et un jeu de la vitrine ont le même dessin (LiveCard) : l'image en 16:9, puis le titre, ses
// lignes et ses gestes. UNE PARTIE SE MONTRE PAR SA VIDÉO, MUETTE (le porteur, 2026-11-14 : « On voit les images des sessions sans
// sons », comme les vignettes de l'ancien tableau de bord) — SessionStream en spectateur sans compte, 540p, sans l'intro,
// sur l'image du jeu (GameBanner) tant que la vidéo n'est pas là ; un jeu de la vitrine, par son image. Toucher l'image
// ne fait rien, seul un bouton agit. L'historique n'est pas écrit par le kernel (B1) : rien n'est montré pour lui. À LA
// MANETTE, la croix est libre ; dans un carrousel, Gauche et Droite passent d'un bouton à l'autre, puis à la carte
// voisine.

// LE DESSIN D'UNE CARTE, le même pour une partie et pour un jeu de la vitrine : une seule colonne, l'image puis le texte
// dessous, à toutes les largeurs (le porteur, 2026-11-24 : « le live now en une seule colonne », l'accueil de l'appli
// comme le site). Sans gouttière horizontale (`gx-0`) : ses marges négatives débordaient de la colonne, et toute la page
// défilait à l'horizontale au téléphone (le porteur, 2026-11-14). `badge` : posé sur l'image ;
// `actions` : les gestes, sur une ligne qui prend la largeur du texte.
function LiveCard({ visual, badge = null, title, children, actions = null }) {
    return (
        <div className="row gx-0 gy-3 align-items-center">
            <div className="col-12">
                <div className="position-relative">
                    {visual}
                    {badge && <span className="badge text-bg-danger position-absolute top-0 start-0 m-2">{badge}</span>}
                </div>
            </div>
            <div className="col-12">
                <div className="ptext">
                    <h2 className="text-break">{title}</h2>
                    {children}
                    {actions && <div className="d-flex flex-wrap align-items-center gap-2 mt-2">{actions}</div>}
                </div>
            </div>
        </div>
    );
}

// L'IMAGE D'UNE PARTIE : SA VIDÉO, MUETTE — la vignette de « Live now » de l'ancien tableau de bord, telle quelle :
// SessionStream, le flux de la partie (`childSessionId` : le nom que lui donne son hôte, `device-session` sur un
// téléphone), sans compte ni manette, sur le plus petit palier de la partie (`lowest`, chantier 58 : « la vignette du live s'est
// toujours le plus petit flux »), sans l'intro (`preview`), qui réessaie tant qu'il est là. Dessous,
// l'image du jeu, que la vidéo couvre dès sa première image, sur un fond noir (les bandes d'une image 4:3). `live` à faux
// (une carte cachée du carrousel) : l'image seule, aucun flux ouvert.
// UNE PARTIE D'AILLEURS SE REGARDE CHEZ ELLE (`host`, useFederatedHost) : sa passerelle la connaît sous l'identifiant
// que son hôte lui donne, sans notre préfixe ; tant que l'hôte n'est pas résolu, l'image seule.
function SessionVisual({ session, title, live, host = null }) {
    const { t } = useTranslation();
    const [picture, setPicture] = useState(false);
    const gatewayProps = useMemo(() => ({ onFirstPicture: () => setPicture(true) }), []);
    const reference = session.reference || session.romReference;
    const streamId = session.childSessionId || session.id;
    const federated = parseFederatedId(streamId);
    const streamHost = useMemo(() => (host ? { origin: host.link, token: host.token } : null), [host]);

    useEffect(() => {
        if (!live) {
            setPicture(false);
        }
    }, [live]);

    return (
        <div className="position-relative z-0">
            <GameBanner consoleId={session.consoleId} reference={reference} source={romSource(session)} title={title} />
            {live && (!federated || streamHost) && (
                <div className={`live-video position-absolute top-0 start-0 w-100 h-100 rounded-4 overflow-hidden ${picture ? "bg-black" : ""}`}>
                    <SessionStream sessionId={federated ? federated.id : streamId} host={streamHost} t={t} maxAttempts={null} showOverlay={false} fillHeight muted quality="lowest" preview gatewayProps={gatewayProps} />
                </div>
            )}
        </div>
    );
}

// L'HÔTE D'UNE PARTIE D'AILLEURS (`srv<id>:…`), résolu par notre kernel (federatedHostOf) : son adresse, le jeton de
// notre compte de là-bas, l'identifiant qu'il lui donne. Nul pour une partie d'ici, et le temps de le résoudre.
function useFederatedHost(sessionId) {
    const [host, setHost] = useState(null);

    useEffect(() => {
        if (!parseFederatedId(sessionId)) {
            setHost(null);

            return undefined;
        }
        let cancelled = false;
        federatedHostOf(sessionId).then((found) => {
            if (!cancelled) {
                setHost(found);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [sessionId]);

    return host;
}

// La page de jeu du launcher (Play.jsx) : y jouer (`play`), ou y regarder ; partir ramène ici.
const usePlayOpener = (sessionId) => {
    const navigate = useNavigate();

    return (play) => navigate(playPath(sessionId, { play }), { state: { back: "/live" } });
};

// L'ARRÊT POUR TOUT LE MONDE : la commande `session.stop` du canal du kernel, qui juge (le propriétaire, ou un
// administrateur). Réussi, le sujet `sessions` retire la partie, et sa ligne ou sa carte part avec elle. Une partie
// d'ailleurs (`host`, useFederatedHost) s'arrête chez son hôte, sous son identifiant de là-bas. Rend `[stopping, stop]`.
function useStopSession(session, host) {
    const { t } = useTranslation();
    const [stopping, setStopping] = useState(false);

    const stop = async () => {
        setStopping(true);
        const answer = !parseFederatedId(session.id) ? await kernelCommand("session.stop", { platform: session.platform, sessionId: session.id })
            : host?.token ? await stopSession(session.platform, host.id, null, { origin: host.link, token: host.token })
                : { ok: false };

        if (!answer.ok) {
            setStopping(false);
            toast(answer.errorMessage || t("play.quit.stopFailed"));
        }
    };

    return [stopping, stop];
}

// L'heure qui avance, seconde par seconde : la durée d'une partie, son compte à rebours.
function useNow() {
    const [now, setNow] = useState(Date.now());

    useEffect(() => {
        const timer = setInterval(() => setNow(Date.now()), 1000);

        return () => clearInterval(timer);
    }, []);

    return now;
}

// VOTRE PARTIE, UNE LIGNE (le porteur, 2026-11-16 : « tu peux mettre "Ma vidéo en cours" comme pour mes favoris. Juste un
// élément avec le titre, le sous-titre : temps du jeu et temps restant. Et à droite le bouton power off. ») : la ligne d'un
// favori (FavoriteRow) — la jaquette du jeu, son titre sans ses étiquettes — et, dessous, la durée de la partie, puis le
// temps qui lui reste avant l'arrêt pour inactivité quand personne n'y joue (idleSince + idleTimeout, servis par le
// kernel). Toucher la ligne REPREND la partie (la page de jeu, pour y jouer) ; au bord droit, l'ARRÊT, carré, en danger
// (le porteur, 2026-11-14 : « le bouton power off doit être à droite », « doit être carré et non round »), après UNE
// fenêtre de confirmation de Skeleton (SplitRow, bricks/).
function OwnSessionRow({ session }) {
    const { t } = useTranslation();
    const confirm = useConfirm();
    const now = useNow();
    const host = useFederatedHost(session.id);
    const [stopping, stop] = useStopSession(session, host);
    const open = usePlayOpener(session.id);
    const title = splitTitle(session.title).main;
    const duration = formatDuration(session.startedAt, now);
    const idle = session.idleSince && session.idleTimeout;
    const left = idle ? Math.max(0, session.idleTimeout - Math.floor((now - new Date(session.idleSince).getTime()) / 1000)) : 0;
    const time = `${String(Math.floor(left / 60)).padStart(2, "0")}:${String(left % 60).padStart(2, "0")}`;

    return (
        <SplitRow id={`lv-resume-${session.id}`} label={title} help={[fact(t("play.info.duration"), duration), idle ? fact(t("facts.comeBack"), time) : null]}
            lead={<GameCover consoleId={session.consoleId} reference={session.reference || session.romReference} game={romSource(session)} />}
            onOpen={() => open(true)}
            action={{ id: `lv-stop-${session.id}`, icon: "bi-power", label: t("live.stop"), kind: "danger", pending: stopping,
                onClick: () => confirm({ title: t("live.stopTitle"), message: t("live.stopMessage", { title }), confirmLabel: t("live.stop"), onConfirm: stop }) }} />
    );
}

// LA PARTIE D'UN AUTRE : Rejoindre, Regarder ; l'arrêt pour un administrateur. `nameOf` : le nom affiché d'un compte, son
// identifiant sinon — celui d'un serveur fédéré tel quel : nos comptes ne sont pas les siens. `live` : sa vidéo se montre
// (SessionVisual).
function SessionCard({ session, own = false, consoleName, nameOf: localName, live = true }) {
    const { t, i18n } = useTranslation();
    const button = useButton();
    const { isAdmin } = useAuth();
    const confirm = useConfirm();
    const now = useNow();
    const federated = Boolean(parseFederatedId(session.id));
    const host = useFederatedHost(session.id);
    const [stopping, stop] = useStopSession(session, host);
    const open = usePlayOpener(session.id);
    const nameOf = federated ? (username) => username : localName;
    const seats = session.maxPlayers || (session.playerCount ?? 0) + (session.openSlots ?? 0);
    const players = counted(t, "playersOf", seats, { players: `${session.playerCount ?? 0}/${seats}` });
    const owner = nameOf(session.owner || "");
    // Le titre sans ses étiquettes (« (Europe) (En,Fr) »), comme l'engrenage d'un jeu.
    const title = splitTitle(session.title).main;
    // Rejoindre : une manette libre, et pas déjà la mienne (un autre onglet y joue) — « une partie, un joueur ».
    const canJoin = !own && !session.watchOnly && session.me?.padSlot == null && session.openSlots > 0;
    const key = (name) => `lv-${name}-${session.id}`;

    // QUI SERA COUPÉ : le propriétaire, puis les joueurs et les spectateurs que la partie connaît (`connectionPool`), par
    // leur nom. Les spectateurs sans compte ne sont pas comptés : une vignette muette d'En direct (celle-ci comprise) peut
    // en être un.
    const people = () => new Intl.ListFormat(i18n.language, { type: "conjunction" })
        .format([...new Set([session.owner, ...(session.connectionPool || []).map((entry) => entry.username)].filter((name) => name && name !== "anonymous"))].map(nameOf));

    const askStop = () => confirm({
        title: t("live.stopOtherTitle"),
        message: t("live.stopOtherMessage", { owner, title }),
        confirmLabel: t("live.stopContinue"),
        then: { title: t("live.stopOtherSureTitle"), message: t("live.stopOtherSureMessage", { people: people(), title }), confirmLabel: t("live.stop"), onConfirm: stop },
    });

    // Des gestes : des boutons de texte seul, l'arrêt d'icône seule (la règle des boutons, bricks/), au bord droit
    // (`ms-auto`), carré — comme celui de votre partie.
    const actions = (
        <>
            {own ? <button type="button" className={button("main")} data-nav data-key={key("resume")} onClick={() => open(true)}>{t("game.resume")}</button> : (
                <>
                    {canJoin && <button type="button" className={button("main")} data-nav data-key={key("join")} onClick={() => open(true)}>{t("live.join")}</button>}
                    <button type="button" className={button(canJoin ? "other" : "main")} data-nav data-key={key("watch")} onClick={() => open(false)}>{t("live.watch")}</button>
                </>
            )}
            {isAdmin && !federated && !own && (
                <button type="button" className={`${SQUARE_ICON_BUTTON} ${button("danger")} ms-auto`} data-nav data-key={key("stop")} disabled={stopping} aria-label={t("live.stop")} title={t("live.stop")} onClick={askStop}>
                    {stopping ? <span className="spinner-border spinner-border-sm" role="status"></span> : <i className="bi bi-power"></i>}
                </button>
            )}
        </>
    );

    return (
        <LiveCard visual={<SessionVisual session={session} title={title} live={live} host={host} />} badge={t("live.badge")} title={title} actions={actions}>
            {/* Une information par ligne, comme le sous-titre d'une ligne (helpLines, bricks/). */}
            <div className={MUTED}>
                {helpLines([fact(t("catalog.console"), consoleName), fact(t("facts.owner"), owner), fact(t("game.players"), players), fact(t("play.info.duration"), formatDuration(session.startedAt, now)), fact(t("game.origin"), session.source)])}
            </div>
        </LiveCard>
    );
}

// UN CARROUSEL DE CARTES, celui de Bootstrap (react-bootstrap) : les parties des autres, ou la vitrine. Sans flèches posées
// sur les cartes (elles couvriraient le texte et les boutons) : on glisse au doigt, on touche un des tirets dessous, ou
// Gauche / Droite à la manette (`stepRef`, que l'écran appelle). `auto` : il avance seul (la vitrine), sauf quand on le
// tient — la souris dessus (une vraie souris : le doigt laisse derrière lui un survol qui ne part plus), la lumière de la
// croix dedans, un toucher récent. Suivi par la clé de la carte, pas par sa place : une partie qui part ne fait pas
// sauter la carte montrée sur une autre.
const AUTO_STEP_MS = 6000;

function LiveCarousel({ items, render, auto = false, stepRef }) {
    const { t } = useTranslation();
    const [activeKey, setActiveKey] = useState(null);
    const box = useRef(null);
    const hovered = useRef(false);
    const touchedAt = useRef(0);
    const refocus = useRef(false);
    const latest = useRef(items);
    const found = items.findIndex((item) => item.key === activeKey);
    const active = found >= 0 ? found : 0;
    const count = items.length;
    latest.current = items;

    const show = (index) => {
        const list = latest.current;

        if (list.length) {
            setActiveKey(list[((index % list.length) + list.length) % list.length].key);
        }
    };
    const holding = () => !document.documentElement.classList.contains("pointing") && Boolean(box.current?.contains(document.activeElement));
    const hover = (event, inside) => {
        if (event.pointerType === "mouse") {
            hovered.current = inside;
        }
    };

    stepRef.current = count > 1 ? (delta) => {
        refocus.current = holding();
        show(active + delta);
    } : null;

    useEffect(() => {
        if (!auto || count < 2) {
            return undefined;
        }
        const timer = setInterval(() => {
            if (!hovered.current && !holding() && Date.now() - touchedAt.current > AUTO_STEP_MS) {
                setActiveKey((current) => {
                    const list = latest.current;
                    const at = list.findIndex((item) => item.key === current);

                    return list[((at < 0 ? 0 : at) + 1) % list.length].key;
                });
            }
        }, AUTO_STEP_MS);

        return () => clearInterval(timer);
    }, [auto, count]);

    // La carte arrivée : la lumière de la croix passe sur son premier bouton.
    const slid = () => {
        if (refocus.current) {
            refocus.current = false;
            focusElement(box.current?.querySelector(".carousel-item.active [data-nav]"));
        }
    };

    return (
        <div ref={box} className="live-stage" onPointerEnter={(event) => hover(event, true)} onPointerLeave={(event) => hover(event, false)}
            onPointerDown={() => { touchedAt.current = Date.now(); }}>
            <Carousel activeIndex={active} onSelect={show} onSlid={slid} interval={null} controls={false} indicators={false} keyboard={false} touch={count > 1}>
                {items.map((item, index) => <Carousel.Item key={item.key}>{render(item, index, index === active)}</Carousel.Item>)}
            </Carousel>
            {count > 1 && (
                <div className="carousel-indicators position-static m-0 mt-3">
                    {items.map((item, index) => (
                        <button key={item.key} type="button" data-bs-target="" className={index === active ? "active" : ""} aria-current={index === active || undefined}
                            aria-label={t("live.slide", { position: index + 1, count })} onClick={() => show(index)}></button>
                    ))}
                </div>
            )}
        </div>
    );
}

// LA VITRINE : CINQ JEUX DE LA BIBLIOTHÈQUE, LES MÊMES TOUT UN JOUR. Les consoles qui ont des jeux ici (rétro, Streamex)
// dans un ordre tiré du jour, puis, console après console, un jeu tiré au hasard (un premier disque ; les listes rangées
// par référence d'abord, le kernel ne les rend pas toujours dans le même ordre) — gardé s'il a un
// fond ou une jaquette (probeGameImage), un second essayé sinon. Un jeu par console tant qu'il y en a : cinq consoles
// différentes. Les mêmes jeux tout le jour : une image demandée une fois ne se redemande pas aux sources (le kernel la
// garde), et la vitrine ne fait pas récupérer les images de cinq jeux nouveaux à chaque retour sur l'accueil. Les listes
// sont celles de la Bibliothèque (loadHere), gardées pour elle. Les jeux arrivent un à un ; `done` quand c'est fini.
const SHOWCASE_SIZE = 5;
const SHOWCASE_CONSOLES = 12;
const SHOWCASE_TRIES = 2;
const showcases = new Map();

const pickSome = (list, count, random) => {
    const picked = new Set();

    while (picked.size < Math.min(count, list.length)) {
        picked.add(Math.floor(random() * list.length));
    }

    return [...picked].map((index) => list[index]);
};

const shuffled = (list, random) => {
    const copy = [...list];

    for (let index = copy.length - 1; index > 0; index--) {
        const other = Math.floor(random() * (index + 1));
        [copy[index], copy[other]] = [copy[other], copy[index]];
    }

    return copy;
};

async function pickShowcase(seed, consoles, loadHere, onFound) {
    const random = randomFrom(hash(seed));
    const games = [];

    for (const entry of shuffled(consoles, random).slice(0, SHOWCASE_CONSOLES)) {
        if (games.length >= SHOWCASE_SIZE) {
            break;
        }
        const { roms = [] } = await loadHere(entry.id);
        const firsts = roms.filter((item) => (item.disc || 1) === 1).sort((a, b) => (a.reference < b.reference ? -1 : a.reference > b.reference ? 1 : 0));

        for (const rom of pickSome(firsts, SHOWCASE_TRIES, random)) {
            const source = romSource(rom);

            if (await probeGameImage(entry.id, rom.reference, "background", 1280, source) || await probeGameImage(entry.id, rom.reference, "cover", 512, source)) {
                games.push({ key: `${entry.id}/${rom.reference}`, console: entry, rom });
                onFound([...games]);
                break;
            }
        }
    }

    return games;
}

function useShowcase(enabled) {
    const { consoles, countOf, loadHere } = useData();
    const [state, setState] = useState({ games: [], done: false });
    const today = new Date();
    const eligible = (consoles || []).filter((entry) => (entry.method === "retrogaming" || entry.method === "streamex") && countOf(entry.id) > 0)
        .sort((a, b) => (a.id < b.id ? -1 : 1));
    const seed = `${today.getFullYear()}-${today.getMonth() + 1}-${today.getDate()}|${eligible.map((entry) => entry.id).join(",")}`;

    useEffect(() => {
        if (!enabled) {
            return undefined;
        }

        if (showcases.has(seed)) {
            setState(showcases.get(seed));

            return undefined;
        }
        let cancelled = false;
        setState({ games: [], done: false });
        pickShowcase(seed, eligible, loadHere, (games) => !cancelled && setState({ games, done: false })).then((games) => {
            showcases.set(seed, { games, done: true });

            if (!cancelled) {
                setState({ games, done: true });
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [enabled, seed]);

    return state;
}

// LA FICHE D'UN JEU DANS LA BIBLIOTHÈQUE, celle où l'on joue : l'onglet de sa console, puis sa référence.

// UN JEU DE LA VITRINE : son image, son titre, sa console ; « Voir le jeu » ouvre sa fiche dans la Bibliothèque.
function ShowcaseCard({ game, index }) {
    const { t } = useTranslation();
    const button = useButton();
    const navigate = useNavigate();
    const title = splitTitle(game.rom.title).main;

    return (
        <LiveCard visual={<GameBanner consoleId={game.console.id} reference={game.rom.reference} source={romSource(game.rom)} title={title} />} title={title}
            actions={<button type="button" className={button("main")} data-nav data-key={`lv-game-${index}`} onClick={() => navigate(gameScreenPath(game.console, game.rom.reference))}>{t("live.openGame")}</button>}>
            <div className={MUTED}>{game.console.name}</div>
        </LiveCard>
    );
}

// RIEN À MONTRER, NI PARTIE NI JEU (ou la vitrine qui se cherche encore) : l'image de l'intro de GameFlex, celle du fond
// de l'écran de lancement.
function IntroCard() {
    const { t } = useTranslation();

    return (
        <LiveCard visual={<div className="gbanner ratio ratio-16x9 rounded-4 overflow-hidden"><img className="object-fit-cover" src={LAUNCH_BACKGROUND} alt="" /></div>} title={t("live.none")}>
            <div className={MUTED}>{t("resume.noneHelp")}</div>
        </LiveCard>
    );
}

const FAVORITES_PER_PAGE = 10;

// MES FAVORIS, sous les parties en direct : une liste de Réglages (Listing), dix par page, ses lignes fantômes le temps de
// les lire ; chacune mène à la fiche de son jeu (FavoriteRow, Favorites.jsx). Vide, la ligne grisée et le Catalogue, où un
// jeu se met en favori.
function FavoritesSection() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { favorites, items, stateOf, loading } = useFavoriteList();
    const [page, setPage] = useSearchParam("favorites", "1");

    return (
        <Section title={t("settings.items.myFavorites.label")} help={t("favorites.help")} sense="modify">
            {favorites.loaded && !items.length ? (
                <Rows>
                    <EmptyRow icon="bi-star" text={t(favorites.failed ? "favorites.unreadable" : "favorites.empty")} />
                    <NavRow id="lv-catalog" icon="bi-disc" label={t("live.catalog")} help={t("settings.items.catalogGames.help")} onOpen={() => navigate("/settings/catalog/games")} />
                </Rows>
            ) : (
                <Listing items={items} loading={loading} page={page} onPage={setPage} pageSize={FAVORITES_PER_PAGE} placeholderRow={() => <PlaceholderRow lead="cover" />}
                    render={(item) => {
                        const state = stateOf(item);
                        const key = favoriteKey(item.consoleId, item.reference);

                        return <FavoriteRow key={key} id={`lv-fav-${key}`} item={item} state={state} onOpen={() => navigate(state?.game ? gameScreenPath(state.console, item.reference) : favoritePath(item))} />;
                    }} />
            )}
        </Section>
    );
}

export default function Live() {
    const { t } = useTranslation();
    const { sessions: hereSessions, ownSessions: hereOwnSessions, consoles, invites, answerInvite } = useData();
    const consoleName = (session) => consoles?.find((entry) => entry.id === session.consoleId)?.name || session.consoleId || "";
    const { user } = useAuth();
    const federatedSessions = useFederatedSessions();
    const sessions = useMemo(() => [...hereSessions, ...federatedSessions], [hereSessions, federatedSessions]);
    const ownSessions = useMemo(() => [...hereOwnSessions, ...federatedSessions.filter((session) => !session.watchOnly && isOwnSession(session, user?.username))],
        [hereOwnSessions, federatedSessions, user?.username]);
    // Les tâches suivies de ce compte : les téléchargements, et les suppressions de jeux (`kind: remove`), qui sont des
    // NOTIFICATIONS (le porteur, 2026-11-17 : « Il faudra le mettre dans notifications avec la progression ») — celles qui
    // tournent, et celles qui viennent de finir.
    const tasks = useDownloadNotices();
    const location = useLocation();
    const navigate = useNavigate();
    const stepRef = useRef(null);
    const showcase = useShowcase(sessions.length === 0);
    const owners = [...new Set(hereSessions.flatMap((session) => [session.owner, ...(session.connectionPool || []).map((entry) => entry.username)]).filter((name) => name && name !== "anonymous"))].sort().join(",");

    useEffect(() => {
        rememberTab("live");
    }, []);

    // Les noms affichés des comptes des parties (SessionPool, qui redemande ce qui a échoué) ; leur identifiant tant
    // qu'on ne les a pas.
    const names = useNames(owners ? owners.split(",") : []);

    // L'arrivée : depuis la cloche, la section Notifications (sa première invitation, sinon son titre ramené en vue) ;
    // sinon le premier bouton de la page. Une page de favoris tournée (`?favorites=`) n'est pas une arrivée : la lumière
    // reste sur la pagination (Listing).
    const arrival = useRef(null);
    useEffect(() => {
        const previous = arrival.current;
        arrival.current = location;

        if (previous && previous.pathname === location.pathname && previous.hash === location.hash && previous.search !== location.search) {
            return;
        }
        if (location.hash === "#notifications") {
            const section = document.getElementById("notifications");
            const invite = section?.querySelector("[data-nav]");

            if (invite) {
                focusElement(invite, true);
            } else {
                section?.scrollIntoView({ block: "start" });
                document.activeElement?.blur?.();
            }

            return;
        }
        // Rien à viser : le point neutre — la croix n'entre pas dans l'en-tête.
        if (!focusElement(document.querySelector(".live-box [data-nav]"), true)) {
            document.activeElement?.blur?.();
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [location.key]);

    // UNE INVITATION ouvre la partie pour la regarder — on y prend la manette depuis son menu.
    const openInvite = (invite) => {
        answerInvite(invite.id, "watched");
        navigate(playPath(invite.sessionId), { state: { back: "/live" } });
    };

    // LES NOTIFICATIONS, UN SEUL BLOC (le porteur, 2026-11-25 : « C'est un seul bloc, plusieurs types de notification ») :
    // les téléchargements et les suppressions, puis les invitations — les dix dernières (RecentNotices). Une invitation
    // se répond sur place (RequestRow : Ignorer, Regarder) ; finie, chaque notification dit comment.
    const TASK_ENDS = { done: ["downloads.state.done", "success"], failed: ["downloads.state.failed", "danger"], cancelled: ["downloads.state.cancelled", "warning"] };
    const notices = [
        ...tasks.map((entry) => {
            const label = entry.label || entry.key;
            const [text, tone] = TASK_ENDS[entry.state] || TASK_ENDS.failed;

            return {
                key: `task-${entry.id}`, changedAt: entry.endedAt || entry.at, endedAt: entry.endedAt || null,
                row: entry.endedAt
                    ? <InfoRow icon={entry.kind === "remove" ? "bi-trash" : "bi-download"} label={label} value={t(text)} pill={tone} help={entry.error || null} />
                    : <DownloadRow entry={entry} id={`n-task-${entry.id}`} label={label} />,
            };
        }),
        ...invites.map((invite) => {
            const label = t("notifications.invite", { name: invite.from?.displayName || "?" });
            const help = fact(t("facts.game"), invite.reference);

            return {
                key: `invite-${invite.id}`, changedAt: invite.endedAt || invite.at, endedAt: invite.endedAt || null,
                row: invite.endedAt
                    ? <InfoRow icon="bi-eye-fill" label={label} value={t(`notifications.inviteState.${invite.state}`)} pill={invite.state === "watched" ? "success" : "secondary"} help={help} />
                    : <RequestRow id={`n-invite-${invite.id}`} label={label} help={help} acceptLabel={t("notifications.watch")}
                        onIgnore={() => answerInvite(invite.id, "ignored")} onAccept={() => openInvite(invite)} />,
            };
        }),
    ];

    // GAUCHE / DROITE DANS UN CARROUSEL : d'un bouton de la carte à l'autre (l'algorithme de la croix), puis, au bord de
    // la carte, la carte voisine.
    useScreenInput({
        back: () => {},
        direction: (direction, current) => {
            if ((direction !== "left" && direction !== "right") || !stepRef.current || !current?.closest?.(".live-stage")) {
                return false;
            }
            const buttons = [...(current.closest(".carousel-item")?.querySelectorAll("[data-nav]") || [])];

            if (buttons[buttons.indexOf(current) + (direction === "right" ? 1 : -1)]) {
                return false;
            }
            stepRef.current(direction === "right" ? 1 : -1);

            return true;
        },
    });

    const nameOf = (username) => names[username] || username;

    return (
        <main className="screen tabbed">
            <Topbar tab="live" />
            <div className="gbody d-flex flex-column">
                <div className="lcol px-0 flex-grow-1" data-scroller>
                    <div className="settings-box live-box w-100 mx-auto">
                        {ownSessions.length > 0 && (
                            <Section title={t("live.mine")} sense="modify">
                                <Rows>
                                    {ownSessions.map((session) => <OwnSessionRow key={session.id} session={session} />)}
                                </Rows>
                            </Section>
                        )}
                        <Section title={t("live.others")} help={sessions.length === 0 && showcase.games.length > 0 ? t("live.showcaseHelp") : null}>
                            {sessions.length > 0 ? (
                                <LiveCarousel items={sessions.map((session) => ({ key: session.id, session }))} stepRef={stepRef}
                                    render={(item, index, active) => <SessionCard session={item.session} own={ownSessions.includes(item.session)} consoleName={consoleName(item.session)} nameOf={nameOf} live={active} />} />
                            ) : showcase.games.length > 0 ? (
                                <LiveCarousel items={showcase.games} auto stepRef={stepRef} render={(game, index) => <ShowcaseCard game={game} index={index} />} />
                            ) : <IntroCard />}
                        </Section>
                        <FavoritesSection />
                        <div id="notifications">
                            <Section title={t("notifications.title")} help={t("notifications.help")} className="mb-4" sense="modify">
                                <RecentNotices notices={notices} limit={10} />
                            </Section>
                        </div>
                    </div>
                </div>
            </div>
        </main>
    );
}
