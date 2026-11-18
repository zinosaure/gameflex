import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useNavigate, useSearchParams } from "react-router-dom";
import { downloadTicket, listRoms, listStreamexSaves, listUserStates, metadataArchiveTicket, metadataArchiveUrl, romSource, ticketDownloadUrl } from "../../libs/api.js";
import { catalogOnly, useData } from "../../libs/data.jsx";
import { browserDownload } from "../../libs/downloads.js";
import { favoriteKey, isFavorite, setFavorite, useFavorites } from "../../libs/favorites.js";
import { useFederationSession } from "../../libs/federation.js";
import { formatSize, GAME_STATUSES, STATUS_VARIANT, statusOf } from "../../libs/format.js";
import { gameMetadataStatus, sourcesOf, summaryOf, useGameMetadata } from "../../libs/metadata.js";
import { splitTitle } from "../../libs/visuals.js";
import { toast } from "../../shell/Shell.jsx";
import { refusalText } from "../../screens/access/FederatedAccount.jsx";
import { GameBanner, GameCover } from "../../screens/library/HeroArt.jsx";
import { savesPathOf } from "../account/Saves.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { EmptyRow, GuideSection, InfoRow, Listing, Note, PlaceholderRow, RangeRow, Rows, Section, SwitchRow } from "../../ui/Bricks.jsx";

// LE CATALOGUE — Réglages › Le catalogue › Jeux (le porteur, 2026-11-16 : « on rajoute une nouvelle section tout public :
// catalogue et dans cette section on aura jeux. Ici on va avoir la liste de tout les jeux inclus. Local et fédération. Le
// filtre nous permettra de choisir en source (switches), par statut (Switches), par taille (skeletonRange), par console
// (Switches) et par nom. Et quand on clique sur un jeu, on aura sa fiche et dans une section téléchargement on pour
// télécharger le jeu, télécharger le metadata, le rajouter/retirer au favoris et aller sur la fiche du jeu en front. Les
// téléchargement sont par navigateur et non directement dans la librairie. »). Deux pages :
//
//   /settings/catalog/games                      la liste : tous les jeux, cherchés par leur nom (`?q=`) et feuilletés
//                                                (`?page=`), leurs filtres dans le panneau de la liste — mes favoris
//                                                (`?fav=1`), les sources (`?nosource=`), les statuts (`?nostatus=`), une
//                                                taille minimum (`?minSize=`, en Mo), les consoles (`?noconsole=`) ;
//   /settings/catalog/games/game?console=&ref=   un jeu : sa FICHE (GameSheet) — ses informations, comme l'engrenage
//                                                d'un jeu ; « Téléchargement » — le jeu et ses métadonnées par le
//                                                NAVIGATEUR ; « Plus » — sa fiche dans la bibliothèque, ses sauvegardes,
//                                                mes favoris.
//
// LA FICHE EST LA MÊME DANS MES FAVORIS (Réglages › Profil › Mes favoris, settings/account/Favorites.jsx ; le porteur,
// 2026-11-16 : « quand on clique sur le jeu on voit la fiche du jeu (même si c'est un jeu fédéré) et on a lien pour le
// voir directement sur le front et voir ses sauvegardes ») : une fiche, deux entrées. Ses gestes qui ne téléchargent
// rien (la bibliothèque, les sauvegardes, le favori) sont dans « Plus », comme le favori de l'engrenage d'un jeu ;
// « Téléchargement » ne garde que ce qui arrive dans le navigateur.
//
// TOUT COMPTE, QUEL QUE SOIT SON RÔLE (le porteur : « dans catalogue le joueur même s'il n'a pas accès aux consoles doit
// pouvoir voir les jeux et les télécharger si envie. C'est là la différence. Catalogue c'est tout membres/roles. ») : rien
// n'y filtre par les consoles qu'un compte lance (`user.consoles`, `onlyAdminsLaunch`), ni par le choix d'afficher les
// jeux des serveurs fédérés à l'écran — il ne vaut que pour les bibliothèques. Le kernel ne garde que le lancement :
// lister, émettre un ticket et télécharger sont à tout compte connecté.
//
// LES JEUX, D'ICI ET DE CHAQUE SERVEUR FÉDÉRÉ : ceux d'ici par console comme la bibliothèque les réunit (loadHere,
// data.jsx : la bibliothèque et ses disques, une référence une fois), ceux d'un serveur par la copie de son listing
// (listRoms `server`) — seulement là où un compte en annonce (`countOf`, `listing.consoles` de `GET /servers`). Chaque
// lecture arrive à son rythme : la liste se montre dès les premières, un serveur lent ou muet ne la retient pas, et son
// interrupteur dit pourquoi il n'a rien donné. Une ligne par référence (un disque, un fichier : ce qu'on télécharge),
// rangées par titre.

export const CATALOG = "/settings/catalog/games";
const gamePath = (consoleId, reference) => `${CATALOG}/game?${new URLSearchParams({ console: consoleId, ref: reference })}`;

const MEGABYTE = 1024 * 1024;
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
// La recherche par le nom, celle de la bibliothèque (matchesQuery, data.jsx) : sans accents ni casse — le titre plié
// une fois par jeu, pas à chaque lettre tapée sur des dizaines de milliers de jeux.
const fold = (text) => String(text || "").normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();

// LA RÉFÉRENCE D'UN JEU D'UN SERVEUR FÉDÉRÉ, `srv<id>:<sa référence>` (remoteReference, au kernel) : son serveur ; et
// sa référence chez lui, celle de ses sauvegardes là-bas.
const REMOTE_REFERENCE = /^srv(\d+):/;
const peerOfReference = (reference) => {
    const match = REMOTE_REFERENCE.exec(reference || "");

    return match ? match[1] : null;
};
const referenceThere = (reference) => String(reference || "").replace(REMOTE_REFERENCE, "");

// UN JEU DU CATALOGUE, ce que sa ligne et sa page en lisent : sa console, sa référence, son titre, sa taille, son
// statut, sa source (`local`, `disk:<nom>`, `peer:<id>` — la valeur de son interrupteur, comme dans la bibliothèque).
const entryOf = (console, rom, serverId = null) => ({
    key: favoriteKey(console.id, rom.reference),
    consoleId: console.id,
    reference: rom.reference,
    title: rom.title || rom.reference,
    folded: fold(rom.title),
    sizeBytes: rom.sizeBytes || 0,
    status: statusOf(rom) || "untested",
    disk: serverId == null ? rom.disk || "" : "",
    peer: serverId,
    series: rom.series || "",
    fileCount: (rom.files || []).length,
    source: serverId != null ? `peer:${serverId}` : rom.disk ? `disk:${rom.disk}` : "local",
});

const byTitle = (left, right) => collator.compare(left.title, right.title) || collator.compare(left.consoleId, right.consoleId) || collator.compare(left.source, right.source);

// Le nom d'un serveur fédéré, celui de sa ligne dans la Fédération.
export const serverName = (servers, id) => {
    const server = (servers || []).find((entry) => String(entry.id) === String(id));

    return server?.hostname || server?.link || String(id);
};

// D'OÙ VIENT UN JEU, en court (sa ligne, sa page) : le serveur fédéré, le disque interne, sinon la bibliothèque.
export const originOf = (game, servers, t) => (game.peer != null ? serverName(servers, game.peer) : game.disk || t("games.source.library"));

// LE REFUS D'UN SERVEUR FÉDÉRÉ, d'après le statut de sa réponse : notre jeton refusé (403), le serveur muet (502, ou
// rien) — les mots de son interrupteur (`games.peer.<refus>`).
const refusalOf = (status) => (status === 403 ? "denied" : status === 502 || status === 0 ? "offline" : null);

// LES JEUX D'UNE CONSOLE CHEZ UNE SOURCE, en jeux du Catalogue (entryOf) : d'ici (loadHere, data.jsx : la bibliothèque
// et ses disques) ou d'un serveur fédéré (`serverId` : la copie de son listing) — `{ok, refusal, games}`. La liste, la
// page d'un jeu et Mes favoris (settings/account/Favorites.jsx) les lisent ainsi, et rien d'autre.
export const readGames = (console, serverId, loadHere) => (serverId != null
    ? listRoms(console.id, { server: serverId }).then((answer) => ({
        ok: answer.ok,
        refusal: answer.ok ? null : refusalOf(answer.status),
        games: answer.ok ? (answer.data || []).map((rom) => entryOf(console, rom, serverId)) : [],
    }))
    : loadHere(console.id).then((result) => ({ ok: result.ok, refusal: null, games: (result.roms || []).map((rom) => entryOf(console, rom)) })));

// LES JEUX DU CATALOGUE, lus pour la page : `{games, reads}` — `reads`, l'état de chaque lecture (`here:<console>`,
// `peer:<serveur>:<console>`) : en cours, en échec, ou le refus d'un serveur (`denied`, `offline`).
function useCatalog() {
    const { consoles, servers, loadHere, countOf } = useData();
    const [games, setGames] = useState([]);
    const [reads, setReads] = useState({});
    const chunks = useRef(new Map());
    const timer = useRef(null);
    const playable = useMemo(() => (consoles || []).filter((entry) => entry.method !== "remoteplay"), [consoles]);

    // Les lectures qui arrivent ensemble ne trient pas la liste chacune : une fois, un instant après la dernière.
    const flush = () => {
        clearTimeout(timer.current);
        timer.current = setTimeout(() => setGames([...chunks.current.values()].flat().sort(byTitle)), 150);
    };
    const mark = (key, read) => setReads((previous) => ({ ...previous, [key]: read }));

    useEffect(() => () => clearTimeout(timer.current), []);

    // D'ICI : chaque console qui en a (son nombre inconnu, elle se lit quand même).
    useEffect(() => {
        if (!consoles) {
            return undefined;
        }
        let cancelled = false;

        for (const console of playable) {
            if (countOf(console.id) === 0) {
                continue;
            }
            const key = `here:${console.id}`;
            mark(key, { loading: true });
            readGames(console, null, loadHere).then((result) => {
                if (cancelled) {
                    return;
                }
                chunks.current.set(key, result.games);
                mark(key, { loading: false, failed: !result.ok });
                flush();
            });
        }

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [playable, countOf]);

    // DE CHAQUE SERVEUR FÉDÉRÉ : chaque console que la copie de son listing annonce. Les serveurs relus (un serveur
    // ajouté ou retiré), leurs jeux d'avant partent.
    useEffect(() => {
        if (!consoles) {
            return undefined;
        }
        let cancelled = false;
        [...chunks.current.keys()].filter((key) => key.startsWith("peer:")).forEach((key) => chunks.current.delete(key));

        for (const server of servers || []) {
            for (const console of playable) {
                if (!(server.listing?.consoles?.[console.id] > 0)) {
                    continue;
                }
                const key = `peer:${server.id}:${console.id}`;
                mark(key, { loading: true });
                readGames(console, server.id, loadHere).then((result) => {
                    if (cancelled) {
                        return;
                    }
                    chunks.current.set(key, result.games);
                    mark(key, result.ok ? { loading: false } : { loading: false, refusal: result.refusal, failed: true });
                    flush();
                });
            }
        }

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [playable, servers]);

    return { games, reads };
}

// Une liste écartée de l'adresse (`nosource`, `nostatus`, `noconsole`) : ses valeurs.
const listOf = (value) => String(value || "").split(",").filter(Boolean);

// /settings/catalog/games : LA LISTE.
function CatalogPage() {
    const { t } = useTranslation();
    const { consoles, disks, servers } = useData();
    const { open } = useSettingsNavigation();
    const favorites = useFavorites();
    const { games, reads } = useCatalog();
    const [read, update] = useQueryState({ page: "1" });
    const search = read("q");
    const [query, setQuery] = useState(fold(search.trim()));
    const onlyFavorites = read("fav") === "1";
    const hiddenSources = listOf(read("nosource"));
    const hiddenStatuses = listOf(read("nostatus"));
    const hiddenConsoles = listOf(read("noconsole"));
    const minSize = Math.max(0, Number(read("minSize")) || 0);
    const favoriteKeys = useMemo(() => new Set(favorites.items.map((item) => favoriteKey(item.consoleId, item.reference))), [favorites.items]);
    const readings = Object.values(reads);
    const pending = readings.some((entry) => entry.loading);

    // La recherche part un instant après la dernière lettre.
    useEffect(() => {
        const timer = setTimeout(() => setQuery(fold(search.trim())), 300);

        return () => clearTimeout(timer);
    }, [search]);

    const shown = useMemo(() => games.filter((game) => (!onlyFavorites || favoriteKeys.has(game.key))
        && !hiddenSources.includes(game.source) && !hiddenStatuses.includes(game.status) && !hiddenConsoles.includes(game.consoleId)
        && (!minSize || game.sizeBytes >= minSize * MEGABYTE) && (!query || game.folded.includes(query))),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [games, onlyFavorites, favoriteKeys, read("nosource"), read("nostatus"), read("noconsole"), minSize, query]);

    // LES SOURCES, UN INTERRUPTEUR CHACUNE, toutes allumées : la bibliothèque d'ici, chaque disque interne, chaque serveur
    // fédéré — qui dit, quand il n'a rien donné, pourquoi (comme le panneau de recherche de la bibliothèque).
    const peerHelp = (server) => {
        const own = Object.entries(reads).filter(([key]) => key.startsWith(`peer:${server.id}:`)).map(([, entry]) => entry);
        const refusal = own.find((entry) => entry.refusal)?.refusal;

        return own.some((entry) => entry.loading) ? t("settings.loading") : refusal ? t(`games.peer.${refusal}`) : own.some((entry) => entry.failed) ? t("games.loadFailed") : t("game.originPeer");
    };
    const sources = [
        { value: "local", label: t("games.source.library"), help: t("game.originLibrary") },
        ...(disks || []).filter((disk) => Object.values(disk.consoles || {}).some((count) => count > 0))
            .map((disk) => ({ value: `disk:${disk.name}`, label: disk.name, help: t("games.source.diskHelp") })),
        ...(servers || []).map((server) => ({ value: `peer:${server.id}`, label: serverName(servers, server.id), help: peerHelp(server) })),
    ];
    // LES CONSOLES QUI ONT DES JEUX ici ou chez un serveur, par leur nom.
    const present = new Set(games.map((game) => game.consoleId));
    const consoleChoices = (consoles || []).filter((entry) => present.has(entry.id)).sort((left, right) => collator.compare(left.name, right.name));
    const consoleName = (id) => (consoles || []).find((entry) => entry.id === id)?.name || id;
    const toggle = (name, hidden) => (value, on) => update({ [name]: (on ? hidden.filter((item) => item !== value) : [...hidden, value]).join(",") || null, page: null });
    const filterCount = (onlyFavorites ? 1 : 0) + hiddenSources.filter((value) => sources.some((source) => source.value === value)).length
        + hiddenStatuses.length + (minSize ? 1 : 0) + hiddenConsoles.length;

    return (
        <SettingsPage title={t("settings.items.catalogGames.label")} parent="/settings">
            <Section title={t("library.gamesTitle")}>
                <Listing items={shown} loading={!games.length && pending} search={search} onSearch={(q) => update({ q, page: null })} placeholder={t("games.search")} searchKey="cg-search"
                    page={read("page")} onPage={(page) => update({ page })}
                    found={t(pending ? "catalog.foundReading" : "catalog.found", { count: shown.length })}
                    empty={t(games.length ? "catalog.noneMatching" : "catalog.none")}
                    filterCount={filterCount} filterTitle={t("settings.filters")} filters={[
                        {
                            key: "favorites", title: t("catalog.filters.favorites"), rows: (
                                <SwitchRow id="cf-favorites" label={t("catalog.onlyFavorites")} help={t("catalog.onlyFavoritesHelp")} checked={onlyFavorites}
                                    onChange={(on) => update({ fav: on ? "1" : null, page: null })} />
                            ),
                        },
                        {
                            key: "sources", title: t("library.sourcesTitle"), help: t("catalog.sourcesHelp"), rows: sources.map((source) => (
                                <SwitchRow key={source.value} id={`cf-src-${source.value}`} label={source.label} help={source.help} checked={!hiddenSources.includes(source.value)}
                                    onChange={(on) => toggle("nosource", hiddenSources)(source.value, on)} />
                            )),
                        },
                        {
                            key: "statuses", title: t("games.status"), rows: GAME_STATUSES.map((status) => (
                                <SwitchRow key={status} id={`cf-st-${status}`} label={t(`status.${status}`)} checked={!hiddenStatuses.includes(status)}
                                    onChange={(on) => toggle("nostatus", hiddenStatuses)(status, on)} />
                            )),
                        },
                        {
                            key: "size", title: t("catalog.filters.size"), rows: (
                                <RangeRow id="cf-min-size" min={0} max={1000} step={10} label={t("gameAdmin.minSize")} help={t("gameAdmin.minSizeHelp")}
                                    value={minSize} onChange={(value) => update({ minSize: Number(value) > 0 ? Math.round(Number(value)) : null, page: null })} />
                            ),
                        },
                        {
                            key: "consoles", title: t("catalog.filters.consoles"), rows: consoleChoices.map((entry) => (
                                <SwitchRow key={entry.id} id={`cf-console-${entry.id}`} label={entry.name} checked={!hiddenConsoles.includes(entry.id)}
                                    onChange={(on) => toggle("noconsole", hiddenConsoles)(entry.id, on)} />
                            )),
                        },
                    ]}
                    placeholderRow={() => <PlaceholderRow lead="cover" />}
                    render={(game) => (
                        <NavRow key={game.key} id={`cg-${game.key}`} label={game.title} lead={<GameCover consoleId={game.consoleId} reference={game.reference} game={romSource(game)} />}
                            help={[consoleName(game.consoleId), originOf(game, servers, t), t(`status.${game.status}`), formatSize(game.sizeBytes)].filter(Boolean).join(" · ")}
                            onOpen={() => open(gamePath(game.consoleId, game.reference), `cg-${game.key}`, { game })} />
                    )} />
            </Section>
            {/* LE GUIDE (le porteur, 2026-11-16 : « Toujours proposer des liens vers la documentation si possible »). */}
            <GuideSection guides={[["library", "browse-the-catalog-and-keep-favorites"], ["federation"]]} />
        </SettingsPage>
    );
}

// POURQUOI LA FICHE D'UN JEU FÉDÉRÉ NE SE LIT PAS (le relais du kernel, chantier 57) : notre jeton refusé (403), le
// serveur muet (502, ou rien), ou un serveur qui ne la prête pas (404) — les mots de la page d'un jeu fédéré des Réglages.
const sheetRefusalOf = (status, server, t) => (status === 403 ? t("gameAdmin.sheetDenied", { server })
    : status === 404 ? t("gameAdmin.sheetNotLent", { server }) : t("gameAdmin.sheetUnreachable", { server }));

// POURQUOI UN TÉLÉCHARGEMENT NE PART PAS : « pas encore sur cet appareil », un jeu sans métadonnées (`no_metadata`,
// déjà dit dans la langue de la page : kernelErrors), le refus du serveur fédéré du jeu, sinon l'échec.
const downloadRefusalOf = (answer, game, server, t) => {
    if (answer.errorCode) {
        return answer.errorMessage;
    }

    if (game.peer != null && answer.status === 403) {
        return t("catalog.download.denied", { server });
    }

    if (game.peer != null && answer.status === 404) {
        return t("catalog.download.notLent", { server });
    }

    if (game.peer != null && (answer.status === 502 || answer.status === 0)) {
        return t("catalog.download.unreachable", { server });
    }

    return t("catalog.download.failed");
};

// OÙ SONT SES SAUVEGARDES (Mes sauvegardes, Saves.jsx) : la page du jeu chez sa source — ici, ou chez son serveur
// fédéré, sous notre compte de là-bas (là où ses parties se jouent). Ce que la page de destination montre, lu comme
// elle le lit : les emplacements du rétro par sa référence (chez son serveur, la sienne là-bas), le jeu d'une console
// récente par l'identifiant de sa fiche (`gameId`, comme la bibliothèque, Library.jsx). `{path, help}` quand il en a,
// `{reason}` sinon (la ligne est grisée, sa raison dessous) ; undefined tant qu'on ne sait pas.
function useSavesLink(console, game, metadata) {
    const { t } = useTranslation();
    const { servers } = useData();
    const peer = game.peer != null ? (servers || []).find((entry) => String(entry.id) === String(game.peer)) || null : null;
    const federation = useFederationSession(peer ? peer.id : null);
    const [found, setFound] = useState(undefined);
    const retro = console.method === "retrogaming";
    const gameId = retro ? null : metadata?.gameId || null;
    const reference = referenceThere(game.reference);
    const host = useMemo(() => (peer && federation.session ? { origin: peer.link, token: federation.session.token } : null), [peer, federation.session]);
    const ready = (game.peer == null || Boolean(host)) && (retro || metadata !== undefined);

    useEffect(() => {
        if (!ready) {
            return undefined;
        }
        let cancelled = false;
        const serverId = peer ? peer.id : null;
        const lookup = retro
            ? listUserStates("", host).then((answer) => (!answer.ok ? { failed: true }
                : { path: (answer.data || []).some((slot) => slot.consoleId === console.id && slot.reference === reference) ? savesPathOf({ serverId, consoleId: console.id, reference }) : null }))
            : !gameId ? Promise.resolve({ path: null })
                : listStreamexSaves("", host).then((answer) => {
                    const saved = (answer.ok ? answer.data?.games || [] : []).find((entry) => entry.id === gameId && String(entry.console).toLowerCase() === console.id.toLowerCase());

                    return !answer.ok ? { failed: true } : { path: saved ? savesPathOf({ serverId, consoleId: saved.console, gameId: saved.id }) : null };
                });
        lookup.then((result) => {
            if (!cancelled) {
                setFound(result);
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [ready, host, console.id, reference, gameId]);

    const server = peer ? { name: serverName(servers, peer.id) } : null;

    if (game.peer != null && !peer) {
        return { reason: t("favorites.gone.server") };
    }

    if (peer && !federation.loading && !federation.session) {
        return { reason: refusalText(t, federation.code, server) };
    }

    if (!ready || found === undefined) {
        return undefined;
    }

    if (found.failed) {
        return { reason: t("catalog.saves.unreadable") };
    }

    return found.path ? { path: found.path, help: server ? t("catalog.saves.at", { server: server.name }) : t("catalog.saves.here") }
        : { reason: server ? t("catalog.saves.noneAt", { server: server.name }) : t("catalog.saves.none") };
}

// LA FICHE D'UN JEU — celle du Catalogue et de Mes favoris (Favorites.jsx), la même : ses informations, « Téléchargement »
// (ce qui arrive dans le navigateur), puis « Plus » — sa fiche dans la bibliothèque, ses sauvegardes, mes favoris —, puis
// « Guide ». `onRemoved` : retiré de mes favoris ici, la page qui la montre s'en va (Mes favoris : retour à la liste).
export function GameSheet({ console, game, onRemoved = null }) {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { open } = useSettingsNavigation();
    const { capabilities, servers, hiddenServers } = useData();
    const favorites = useFavorites();
    const source = romSource(game);
    const metadata = useGameMetadata(console.id, game.reference, source);
    const failure = metadata === null && game.peer != null ? gameMetadataStatus(console.id, game.reference, source) : null;
    const known = summaryOf(metadata);
    const cited = sourcesOf(metadata);
    const origin = originOf(game, servers, t);
    const favorite = isFavorite(favorites, console.id, game.reference);
    const saves = useSavesLink(console, game, metadata);
    const [pending, setPending] = useState(null);
    const joined = (values) => (values || []).join(", ");
    const name = splitTitle(metadata?.title || game.title).main;
    // LA BIBLIOTHÈQUE CACHE LES JEUX D'UN SERVEUR FÉDÉRÉ que le compte a éteint (Réglages › Application, `hiddenServers`,
    // data.jsx) : la fiche du jeu n'y serait pas, la ligne le dit au lieu d'ouvrir celle d'un autre jeu. Le Catalogue,
    // lui, les montre tous.
    const hiddenThere = game.peer != null && Boolean(hiddenServers?.has?.(String(game.peer)));
    // UN SERVEUR QUI NE LANCE RIEN (gameflex-federation) : son jeu n'est dans aucune bibliothèque — il se télécharge.
    const servedOnly = game.peer != null && catalogOnly((servers || []).find((server) => String(server.id) === String(game.peer)));

    // LE JEU OU SES MÉTADONNÉES, PAR LE NAVIGATEUR : le ticket, puis l'adresse qu'il suit (browserDownload) — au
    // téléphone, Chrome, qui le dit.
    const download = async (which, mint, address) => {
        setPending(which);
        const answer = await mint(console.id, game.reference);
        const ticket = answer.ok ? answer.data?.ticket : null;

        if (!ticket) {
            setPending(null);
            toast(downloadRefusalOf(answer, game, origin, t));

            return;
        }
        const handed = await browserDownload(address(ticket), capabilities);
        setPending(null);

        if (handed) {
            toast(t("catalog.download.inBrowser"));
        }
    };

    const toggleFavorite = async () => {
        setPending("favorite");
        const answer = await setFavorite({ consoleId: console.id, reference: game.reference, serverId: game.peer, title: game.title }, !favorite);
        setPending(null);
        toast(answer.ok ? t(favorite ? "favorites.removed" : "favorites.added", { name }) : answer.detail || t("favorites.failed"));

        if (answer.ok && favorite) {
            onRemoved?.();
        }
    };

    const openLibrary = () => navigate(`/${console.method === "streamex" ? "streamex" : "retrogaming"}/${encodeURIComponent(console.id)}/games/${encodeURIComponent(game.reference)}`);

    return (
        <>
            <div className="mt-3">
                <GameBanner consoleId={console.id} reference={game.reference} source={source} title={name} />
            </div>
            <Section title={t("gear.informations")}>
                <Rows>
                    <InfoRow icon="bi-joystick" label={t("catalog.console")} value={console.name} />
                    <InfoRow icon={game.peer != null ? "bi-arrow-left-right" : "bi-hdd"} tone={game.peer != null ? "green" : "slate"} label={t("game.origin")} value={origin} />
                    <InfoRow icon="bi-patch-check" label={t("games.status")} value={<span className={`badge rounded-pill text-bg-${STATUS_VARIANT[game.status] || "secondary"}`}>{t(`status.${game.status}`)}</span>} />
                    {metadata === undefined && <PlaceholderRow help={false} chevron={false} />}
                    {failure !== null && <EmptyRow icon="bi-journal-x" text={sheetRefusalOf(failure, origin, t)} />}
                    <InfoRow icon="bi-calendar3" label={t("gameAdmin.fields.releaseDate")} value={metadata?.releaseDate} />
                    <InfoRow icon="bi-building" label={t("gameAdmin.fields.publishers")} value={joined(metadata?.publishers)} />
                    <InfoRow icon="bi-code-slash" label={t("gameAdmin.fields.developers")} value={joined(metadata?.developers)} />
                    <InfoRow icon="bi-tags" label={t("gameAdmin.fields.genres")} value={joined(metadata?.genres)} />
                    <InfoRow icon="bi-collection" label={t("gameAdmin.fields.series")} value={metadata?.series || game.series} />
                    {known.players > 0 && <InfoRow icon="bi-people" label={t("game.players")} value={String(known.players)} />}
                    <InfoRow icon="bi-upc" label={t("gameAdmin.fields.gameId")} value={metadata?.gameId} />
                    <InfoRow icon="bi-file-earmark-zip" label={t("game.size")} value={formatSize(game.sizeBytes)} />
                    <InfoRow icon="bi-files" label={t("game.files")} value={game.fileCount ? t("game.fileCount", { count: game.fileCount }) : ""} />
                </Rows>
                {cited.length > 0 && (
                    <Note>
                        <b className="text-body">{t("game.sources")}</b>{" "}
                        {cited.map((entry, index) => (
                            <React.Fragment key={entry.label}>
                                {index > 0 && " · "}
                                {entry.href ? <a href={entry.href} target="_blank" rel="noopener noreferrer" data-nav>{entry.label}</a> : entry.label}
                            </React.Fragment>
                        ))}
                    </Note>
                )}
            </Section>
            <Section title={t("catalog.download.title")} help={t("catalog.download.help")}>
                <Rows>
                    <NavRow id="cg-download-game" icon="bi-download" tone="indigo" label={t("catalog.download.game")}
                        help={[formatSize(game.sizeBytes), game.peer != null ? t("catalog.download.fromServer", { server: origin }) : ""].filter(Boolean).join(" · ") || null}
                        pending={pending === "game"} opens="action" onOpen={() => download("game", downloadTicket, ticketDownloadUrl)} />
                    <NavRow id="cg-download-metadata" icon="bi-file-earmark-zip" tone="violet" label={t("catalog.download.metadata")} help={t("catalog.download.metadataHelp")}
                        pending={pending === "metadata"} opens="action" onOpen={() => download("metadata", metadataArchiveTicket, metadataArchiveUrl)} />
                </Rows>
            </Section>
            {/* LES GESTES QUI NE TÉLÉCHARGENT RIEN, des lignes du groupe « Plus » (le porteur, 2026-11-13 ; le favori de
                l'engrenage d'un jeu y est aussi) : la fiche à l'écran, ses sauvegardes — grisée, sa raison dessous, quand
                il n'en a pas ou qu'on ne peut pas les lire —, mes favoris. */}
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="cg-library" icon="bi-collection-play" tone={hiddenThere || servedOnly ? "slate" : "green"} label={t("catalog.download.library")}
                        help={servedOnly ? t("catalog.download.libraryCatalogOnly") : hiddenThere ? t("catalog.download.libraryHidden")
                            : t("catalog.download.libraryHelp", { console: console.name })} disabled={hiddenThere || servedOnly} onOpen={openLibrary} />
                    <NavRow id="cg-saves" icon="bi-shield-check" tone={saves?.path ? "teal" : "slate"} label={t("catalog.saves.label")}
                        help={saves === undefined ? t("settings.loading") : saves.path ? saves.help : saves.reason} pending={saves === undefined} disabled={!saves?.path}
                        onOpen={() => open(saves.path, "cg-saves")} />
                    <NavRow id="cg-favorite" icon={favorite ? "bi-star-fill" : "bi-star"} tone="amber" label={t(favorite ? "favorites.remove" : "favorites.add")} help={t("favorites.help")}
                        pending={pending === "favorite" || !favorites.loaded} opens="action" onOpen={toggleFavorite} />
                </Rows>
            </Section>
            {/* LE GUIDE (le porteur, 2026-11-16 : « Toujours proposer des liens vers la documentation si possible »). */}
            <GuideSection guides={[["library", "browse-the-catalog-and-keep-favorites"], ["storage", "my-saves"]]} />
        </>
    );
}

// /settings/catalog/games/game?console=&ref= : UN JEU — celui que la liste a passé (l'historique), sinon relu chez sa
// source (un lien direct, un rechargement) : d'ici par la liste de sa console, d'un serveur fédéré par la sienne.
function CatalogGamePage() {
    const { t } = useTranslation();
    const [params] = useSearchParams();
    const location = useLocation();
    const { consoles, loadHere } = useData();
    const consoleId = params.get("console") || "";
    const reference = params.get("ref") || "";
    const console = (consoles || []).find((entry) => entry.id === consoleId && entry.method !== "remoteplay") || null;
    const given = location.state?.game?.consoleId === consoleId && location.state.game.reference === reference ? location.state.game : null;
    const [game, setGame] = useState(given || undefined);
    const metadata = useGameMetadata(console && game ? console.id : null, game?.reference, romSource(game));

    useEffect(() => {
        if (given || !consoles) {
            return undefined;
        }

        if (!console || !reference) {
            setGame(null);

            return undefined;
        }
        let cancelled = false;
        readGames(console, peerOfReference(reference), loadHere).then((result) => {
            if (!cancelled) {
                setGame(result.games.find((entry) => entry.reference === reference) || null);
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoles, consoleId, reference]);

    return (
        <SettingsPage title={metadata?.title || game?.title || reference} parent={CATALOG} levels={{ [CATALOG]: t("settings.items.catalogGames.label") }}>
            {game === undefined ? <Section title={t("gear.informations")}><Note className="mt-0">{t("settings.loading")}</Note></Section>
                : game === null || !console ? <NotFound /> : <GameSheet console={console} game={game} />}
        </SettingsPage>
    );
}

// /settings/catalog/games/* (Settings.jsx).
export function CatalogRoutes() {
    return (
        <Routes>
            <Route index element={<CatalogPage />} />
            <Route path="game" element={<CatalogGamePage />} />
            <Route path="*" element={<Navigate to={CATALOG} replace />} />
        </Routes>
    );
}
