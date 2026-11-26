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
//                                                taille minimum (`?minSize=`, en Mo) et les plus lourds d'abord
//                                                (`?sort=size`, comme les jeux d'une console), les consoles (`?noconsole=`) ;
//   /settings/catalog/games/game?console=&ref=   un jeu : sa FICHE (GameSheet) — ses informations, comme l'engrenage
//                                                d'un jeu ; « Téléchargement » — le jeu et ses métadonnées par le
//                                                NAVIGATEUR ; « Plus » — sa fiche dans la bibliothèque, ses sauvegardes,
//                                                mes favoris.
//
// LA FICHE EST LA MÊME DANS MES FAVORIS (Réglages › Profil › Mes favoris, settings/profile/Favorites.jsx ; le porteur,
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
// LES JEUX DES RESSOURCES EXTERNES DE CE SERVEUR (`#808` ; le porteur, 2026-11-23 : « dans /settings/catalog/games on
// ne voit pas les resources externes dans le filtre ») : chacune une source, ses jeux par console sous la référence
// `res<id>:` ; ils se téléchargent par le navigateur comme les autres (le ticket les sert depuis leur ressource).
//
// LES JEUX, D'ICI ET DE CHAQUE SERVEUR FÉDÉRÉ : ceux d'ici par console comme la bibliothèque les réunit (loadHere,
// data.jsx : la bibliothèque et ses disques, une référence une fois), ceux d'un serveur par la copie de son listing
// (listRoms `server`) — seulement là où un compte en annonce (`countOf`, `listing.consoles` de `GET /servers`). Chaque
// lecture arrive à son rythme : la liste se montre dès les premières, un serveur lent ou muet ne la retient pas, et son
// interrupteur dit pourquoi il n'a rien donné. Une ligne par référence (un disque, un fichier : ce qu'on télécharge),
// rangées par titre.

import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { sizeGroup, statusGroup, switchGroup } from "../../bricks/FilterGroups.jsx";
import { GameFacts } from "../../bricks/GameFacts.jsx";
import { GameImages } from "../../bricks/GameImages.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { PlaceholderRow } from "../../bricks/PlaceholderRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SwitchRow } from "../../bricks/SwitchRow.jsx";
import { hiddenOf, toggleHidden } from "../../globals/filters.js";
import { downloadTicket, listLentResources, listRoms, listStreamexSaves, listUserStates, metadataArchiveTicket, metadataArchiveUrl, romSource, ticketDownloadUrl } from "../../libs/api.js";
import { fromResource, useData } from "../../libs/data.jsx";
import { browserDownload } from "../../libs/downloads.js";
import { favoriteKey, isFavorite, setFavorite, useFavorites } from "../../libs/favorites.js";
import { useFederationSession } from "../../libs/federation.js";
import { fact, formatSize, originOf, serverName, statusOf } from "../../libs/format.js";
import { gameMetadataStatus, sourcesOf, useGameMetadata } from "../../libs/metadata.js";
import { useDebounced } from "../../libs/ui.js";
import { splitTitle } from "../../libs/visuals.js";
import { refusalText } from "../../screens/access/FederatedAccount.jsx";
import { GameCover } from "../../screens/library/HeroArt.jsx";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import { savesPathOf } from "../profile/Saves.jsx";
import { originView } from "../../bricks/Origin.jsx";

export const CATALOG = "/settings/catalog/games";
export const gamePath = (consoleId, reference) => `${CATALOG}/game?${new URLSearchParams({ console: consoleId, ref: reference })}`;

export const MEGABYTE = 1024 * 1024;
export const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
// La recherche par le nom, celle de la bibliothèque (matchesQuery, data.jsx) : sans accents ni casse — le titre plié
// une fois par jeu, pas à chaque lettre tapée sur des dizaines de milliers de jeux.
export const fold = (text) => String(text || "").normalize("NFD").replace(/[̀-ͯ]/g, "").toLowerCase();

// LA RÉFÉRENCE D'UN JEU D'UN SERVEUR FÉDÉRÉ, `srv<id>:<sa référence>` (remoteReference, au kernel) : son serveur ; et
// sa référence chez lui, celle de ses sauvegardes là-bas.
export const REMOTE_REFERENCE = /^srv(\d+):/;
export const peerOfReference = (reference) => {
    const match = REMOTE_REFERENCE.exec(reference || "");

    return match ? match[1] : null;
};
export const referenceThere = (reference) => String(reference || "").replace(REMOTE_REFERENCE, "");
// LA RÉFÉRENCE D'UN JEU D'UNE RESSOURCE EXTERNE, `res<id>:<sa référence>` (resources.Reference, au kernel) : sa ressource.
export const RESOURCE_REFERENCE = /^res(\d+):/;
export const resourceOfReference = (reference) => RESOURCE_REFERENCE.exec(reference || "")?.[1] || null;

// LES RESSOURCES EXTERNES telles que tout compte les voit (leur nom, leur genre, leurs jeux par console), lues une fois
// par page qui les demande.
export function useLentResources() {
    const [resources, setResources] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        listLentResources().then((answer) => !cancelled && setResources(answer.ok ? answer.data || [] : []));

        return () => {
            cancelled = true;
        };
    }, []);

    return resources;
}

// UN JEU DU CATALOGUE, ce que sa ligne et sa page en lisent : sa console, sa référence, son titre, sa taille, son
// statut, sa source (`local`, `disk:<nom>`, `peer:<id>`, `resource:<id>` — la valeur de son interrupteur, comme dans la
// bibliothèque). `resource` : la ressource externe d'où il vient (`{id, name}`).
export const entryOf = (console, rom, serverId = null, resource = null) => ({
    key: favoriteKey(console.id, rom.reference),
    consoleId: console.id,
    reference: rom.reference,
    title: rom.title || rom.reference,
    folded: fold(rom.title),
    sizeBytes: rom.sizeBytes || 0,
    status: statusOf(rom) || "untested",
    disk: serverId == null && !resource ? rom.disk || "" : "",
    peer: serverId,
    resource: resource ? resource.id : null,
    resourceName: resource ? resource.name : "",
    series: rom.series || "",
    source: serverId != null ? `peer:${serverId}` : resource ? `resource:${resource.id}` : rom.disk ? `disk:${rom.disk}` : "local",
});

export const byTitle = (left, right) => collator.compare(left.title, right.title) || collator.compare(left.consoleId, right.consoleId) || collator.compare(left.source, right.source);

// LE REFUS D'UN SERVEUR FÉDÉRÉ, d'après le statut de sa réponse : notre jeton refusé (403), le serveur muet (502, ou
// rien) — les mots de son interrupteur (`games.peer.<refus>`).
export const refusalOf = (status) => (status === 403 ? "denied" : status === 502 || status === 0 ? "offline" : null);

// LES JEUX D'UNE CONSOLE CHEZ UNE SOURCE, en jeux du Catalogue (entryOf) : d'ici (loadHere, data.jsx : la bibliothèque
// et ses disques) ou d'un serveur fédéré (`serverId` : la copie de son listing) — `{ok, refusal, games}`. La liste, la
// page d'un jeu et Mes favoris (settings/profile/Favorites.jsx) les lisent ainsi, et rien d'autre.
export const readGames = (console, serverId, loadHere) => (serverId != null
    ? listRoms(console.id, { server: serverId }).then((answer) => ({
        ok: answer.ok,
        refusal: answer.ok ? null : refusalOf(answer.status),
        games: answer.ok ? (answer.data || []).map((rom) => entryOf(console, rom, serverId)) : [],
    }))
    : loadHere(console.id).then((result) => ({ ok: result.ok, refusal: null, games: (result.roms || []).map((rom) => entryOf(console, rom)) })));

// LES JEUX D'UNE CONSOLE QU'UNE RESSOURCE EXTERNE PORTE (`resource` : `{id, name}`), en jeux du Catalogue.
export const readResourceGames = (console, resource) => listRoms(console.id, { resource: resource.id }).then((answer) => ({
    ok: answer.ok,
    refusal: null,
    games: answer.ok ? (answer.data || []).map((rom) => entryOf(console, rom, null, resource)) : [],
}));

// LES JEUX DU CATALOGUE, lus pour la page : `{games, reads}` — `reads`, l'état de chaque lecture (`here:<console>`,
// `peer:<serveur>:<console>`) : en cours, en échec, ou le refus d'un serveur (`denied`, `offline`).
export function useCatalog() {
    const { consoles, servers, loadHere, countOf } = useData();
    const resources = useLentResources();
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

    // DE CHAQUE RESSOURCE EXTERNE : chaque console où elle a des jeux.
    useEffect(() => {
        if (!consoles || !resources) {
            return undefined;
        }
        let cancelled = false;
        [...chunks.current.keys()].filter((key) => key.startsWith("resource:")).forEach((key) => chunks.current.delete(key));

        for (const resource of resources) {
            for (const console of playable) {
                if (!(resource.consoles?.[console.id] > 0)) {
                    continue;
                }
                const key = `resource:${resource.id}:${console.id}`;
                mark(key, { loading: true });
                readResourceGames(console, resource).then((result) => {
                    if (cancelled) {
                        return;
                    }
                    chunks.current.set(key, result.games);
                    mark(key, { loading: false, failed: !result.ok });
                    flush();
                });
            }
        }

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [playable, resources]);

    return { games, reads, resources };
}

// Route : /settings/catalog/games — le Catalogue : tous les jeux d'ici et des serveurs fédérés.
// /settings/catalog/games : LA LISTE.
export default function Catalog() {
    const { t } = useTranslation();
    const { consoles, disks, servers } = useData();
    const { open } = useSettingsNavigation();
    const favorites = useFavorites();
    const { games, reads, resources } = useCatalog();
    const [read, update] = useQueryState({ page: "1" });
    const search = read("q");
    const query = useDebounced(fold(search.trim()));
    const onlyFavorites = read("fav") === "1";
    const hiddenSources = hiddenOf(read("nosource"));
    const hiddenStatuses = hiddenOf(read("nostatus"));
    const hiddenConsoles = hiddenOf(read("noconsole"));
    const minSize = Math.max(0, Number(read("minSize")) || 0);
    const heaviest = read("sort") === "size";
    const favoriteKeys = useMemo(() => new Set(favorites.items.map((item) => favoriteKey(item.consoleId, item.reference))), [favorites.items]);
    const readings = Object.values(reads);
    const pending = readings.some((entry) => entry.loading);

    const shown = useMemo(() => games.filter((game) => (!onlyFavorites || favoriteKeys.has(game.key))
        && !hiddenSources.includes(game.source) && !hiddenStatuses.includes(game.status) && !hiddenConsoles.includes(game.consoleId)
        && (!minSize || game.sizeBytes >= minSize * MEGABYTE) && (!query || game.folded.includes(query)))
        .sort((left, right) => (heaviest ? (right.sizeBytes || 0) - (left.sizeBytes || 0) : 0)),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [games, onlyFavorites, favoriteKeys, read("nosource"), read("nostatus"), read("noconsole"), minSize, heaviest, query]);

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
        ...(servers || []).map((server) => ({ value: `peer:${server.id}`, label: serverName(servers, server.id),
            help: server.relayedBy ? t("fed.relayedBy", { name: server.relayedBy }) : peerHelp(server) })),
        // Chaque ressource externe qui a des jeux (`#808`).
        ...(resources || []).filter((resource) => Object.values(resource.consoles || {}).some((count) => count > 0))
            .map((resource) => ({ value: `resource:${resource.id}`, label: resource.name, help: t("catalog.resourceHelp", { kind: t(`fedService.kinds.${resource.kind}.label`) }) })),
    ];
    // LES CONSOLES QUI ONT DES JEUX ici ou chez un serveur, par leur nom.
    const present = new Set(games.map((game) => game.consoleId));
    const consoleChoices = (consoles || []).filter((entry) => present.has(entry.id)).sort((left, right) => collator.compare(left.name, right.name));
    const consoleName = (id) => (consoles || []).find((entry) => entry.id === id)?.name || id;
    const toggle = (name, hidden) => (value, on) => update({ [name]: toggleHidden(hidden, value, on), page: null });
    const filterCount = (onlyFavorites ? 1 : 0) + hiddenSources.filter((value) => sources.some((source) => source.value === value)).length
        + hiddenStatuses.length + (minSize ? 1 : 0) + (heaviest ? 1 : 0) + hiddenConsoles.length;

    return (
        <SettingsPage title={t("settings.items.catalogGames.label")} parent="/settings">
            <Section title={t("library.gamesTitle")} sense="info">
                <Listing items={shown} loading={!games.length && pending} search={search} onSearch={(q) => update({ q, page: null })} placeholder={t("games.search")} searchKey="cg-search"
                    page={read("page")} onPage={(page) => update({ page })}
                    found={t(pending ? "catalog.foundReading" : "catalog.found", { count: shown.length })}
                    empty={t(games.length ? "catalog.noneMatching" : "catalog.none")}
                    filterCount={filterCount} filters={[
                        {
                            key: "favorites", title: t("catalog.filters.favorites"), rows: (
                                <SwitchRow id="cf-favorites" label={t("catalog.onlyFavorites")} help={t("catalog.onlyFavoritesHelp")} checked={onlyFavorites}
                                    onChange={(on) => update({ fav: on ? "1" : null, page: null })} />
                            ),
                        },
                        switchGroup({ key: "sources", title: t("library.sourcesTitle"), id: "cf-src", choices: sources, hidden: hiddenSources, onChange: toggle("nosource", hiddenSources) }),
                        statusGroup({ t, id: "cf-st", hidden: hiddenStatuses, onChange: toggle("nostatus", hiddenStatuses) }),
                        sizeGroup({ t, id: "cf", minSize, onMinSize: (value) => update({ minSize: value, page: null }), heaviest, onHeaviest: (on) => update({ sort: on ? "size" : null, page: null }) }),
                        switchGroup({ key: "consoles", title: t("catalog.filters.consoles"), id: "cf-console", hidden: hiddenConsoles, onChange: toggle("noconsole", hiddenConsoles),
                            choices: consoleChoices.map((entry) => ({ value: entry.id, label: entry.name })) }),
                    ]}
                    placeholderRow={() => <PlaceholderRow lead="cover" />}
                    render={(game) => (
                        <NavRow key={game.key} id={`cg-${game.key}`} label={game.title} lead={<GameCover consoleId={game.consoleId} reference={game.reference} game={romSource(game)} />}
                            help={[fact(t("catalog.console"), consoleName(game.consoleId)), fact(t("game.origin"), originView(game, servers, t))]}
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
export const sheetRefusalOf = (status, server, t) => (status === 403 ? t("gameAdmin.sheetDenied", { server })
    : status === 404 ? t("gameAdmin.sheetNotLent", { server }) : t("gameAdmin.sheetUnreachable", { server }));

// POURQUOI UN TÉLÉCHARGEMENT NE PART PAS : « pas encore sur cet appareil », un jeu sans métadonnées (`no_metadata`,
// déjà dit dans la langue de la page : kernelErrors), le refus du serveur fédéré du jeu, sinon l'échec.
export const downloadRefusalOf = (answer, game, server, t) => {
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
export function useSavesLink(console, game, metadata) {
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
    const cited = sourcesOf(metadata);
    const origin = originOf(game, servers, t);
    const favorite = isFavorite(favorites, console.id, game.reference);
    const saves = useSavesLink(console, game, metadata);
    const [pending, setPending] = useState(null);
    const name = splitTitle(metadata?.title || game.title).main;
    // LA BIBLIOTHÈQUE CACHE LES JEUX D'UN SERVEUR FÉDÉRÉ que le compte a éteint (Réglages › Application, `hiddenServers`,
    // data.jsx) : la fiche du jeu n'y serait pas, la ligne le dit au lieu d'ouvrir celle d'un autre jeu. Le Catalogue,
    // lui, les montre tous.
    const hiddenThere = game.peer != null && Boolean(hiddenServers?.has?.(String(game.peer)));
    // UN JEU D'UNE RESSOURCE EXTERNE (`#808`), d'un ami ou d'ici : il n'est dans aucune bibliothèque — il se télécharge.
    const servedOnly = fromResource(game) || game.resource != null;

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
            {/* LES SECTIONS D'UN JEU, CELLES DES JEUX D'UNE CONSOLE (settings/consoles/games/Game.jsx ; le porteur, 2026-11-25 : « les
                sections dépendent du fait qu'on est dans catalogue/jeux ou consoles/jeux. Par exemple le download c'est
                différent ») : ses images, ses détails, jouer et mes favoris, puis — au Catalogue — TÉLÉCHARGER, par le
                navigateur, le jeu ou ses métadonnées ; puis « Plus », ses sauvegardes. */}
            <GameImages consoleId={console.id} reference={game.reference} game={source} />
            <Section title={t("details")} help={t("gameAdmin.sections.detailsHelp")} sense="info">
                <GameFacts facts={{ console: console.name, origin: originView(game, servers, t), status: game.status, series: game.series, sizeBytes: game.sizeBytes }}
                    metadata={metadata} refusal={failure !== null ? sheetRefusalOf(failure, origin, t) : null} sources={cited} />
            </Section>
            {/* JOUER, ET MES FAVORIS : PAS POUR UN JEU D'UNE SOURCE EXTERNE (une ressource externe, d'ici ou d'un serveur
                fédéré ; le porteur, 2026-11-25 : « la section "Play now or later" n'est pas valide pour les sources externes.
                Devrait pas être apparue ») — il ne se joue pas d'ici, il se télécharge (la section qui suit). Un favori déjà
                posé sur un tel jeu se retire de Mes favoris. */}
            {!servedOnly && (
                <Section title={t("gameAdmin.sections.play")} help={t("gameAdmin.sections.playHelp")} sense="play">
                    <Rows>
                        <NavRow id="cg-library" icon="bi-play" label={t("gameAdmin.play")}
                            help={hiddenThere ? t("catalog.download.libraryHidden") : t("catalog.download.libraryHelp", { console: console.name })} disabled={hiddenThere} onOpen={openLibrary} />
                        <NavRow id="cg-favorite" icon={favorite ? "bi-star-fill" : "bi-star"} label={t(favorite ? "favorites.remove" : "favorites.add")} help={t("favorites.help")}
                            pending={pending === "favorite" || !favorites.loaded} opens="action" onOpen={toggleFavorite} />
                    </Rows>
                </Section>
            )}
            <Section title={t("catalog.download.title")} help={t("catalog.download.help")} sense="play">
                <Rows>
                    <NavRow id="cg-download-game" icon="bi-download" label={t("catalog.download.game")}
                        help={[formatSize(game.sizeBytes), game.peer != null ? origin : null].filter(Boolean)}
                        pending={pending === "game"} opens="action" onOpen={() => download("game", downloadTicket, ticketDownloadUrl)} />
                    <NavRow id="cg-download-metadata" icon="bi-file-earmark-zip" label={t("catalog.download.metadata")} help={t("catalog.download.metadataHelp")}
                        pending={pending === "metadata"} opens="action" onOpen={() => download("metadata", metadataArchiveTicket, metadataArchiveUrl)} />
                </Rows>
            </Section>
            <Section title={t("more")} help={t("catalog.moreHelp")} sense="optional">
                <Rows>
                    <NavRow id="cg-saves" icon="bi-shield-check" label={t("catalog.saves.label")}
                        help={saves === undefined ? t("settings.loading") : saves.path ? saves.help : saves.reason} pending={saves === undefined} disabled={!saves?.path}
                        onOpen={() => open(saves.path, "cg-saves")} />
                </Rows>
            </Section>
            {/* LE GUIDE (le porteur, 2026-11-16 : « Toujours proposer des liens vers la documentation si possible »). */}
            <GuideSection guides={[["library", "browse-the-catalog-and-keep-favorites"], ["storage", "my-saves"]]} />
        </>
    );
}
