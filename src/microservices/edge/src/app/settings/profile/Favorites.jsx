import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { listLentResources, listServers, romSource } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { favoriteKey, useFavorites } from "../../libs/favorites.js";
import { GameCover } from "../../screens/library/HeroArt.jsx";
import { readGames, readResourceGames, resourceOfReference } from "../catalog/Games.jsx";
import { originOf, serverName } from "../../libs/format.js";
import SettingsPage, { PROFILE, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { PlaceholderRow } from "../../bricks/PlaceholderRow.jsx";
import { Section } from "../../bricks/Section.jsx";

// Route : /settings/profile/favorites — mes favoris, Réglages › Profil › Mes favoris (le porteur, 2026-11-16 : « dans profile il faut le lien pour : mes
// favoris. Dedans il y aura la liste des favoris et quand on clique sur le jeu on voit la fiche du jeu (même si c'est un
// jeu fédéré) et on a lien pour le voir directement sur le front et voir ses sauvegardes. ») — ceux du COMPTE
// (libs/favorites.js : le service des comptes), posés depuis le Catalogue ou l'engrenage d'un jeu. Ici, la liste : chaque
// favori, sa jaquette, son titre, « console • provenance » — et, quand son jeu n'est plus là, pourquoi. Un favori a sa
// page (favorites/Game.jsx, /settings/profile/favorites/game?console=&ref=).
//
// SON JEU SE LIT CHEZ SA SOURCE comme le Catalogue le lit (readGames) : d'ici, la bibliothèque et ses disques ; d'un
// serveur fédéré, la copie de son listing — une lecture par console et par source, quel que soit le nombre de favoris.
// Rien n'est inventé : un jeu absent de sa source, une console partie, un serveur qui n'est plus fédéré le disent ; une
// source qui ne se lit pas (son refus, son silence) le dit aussi, sans déclarer le jeu disparu.

export const FAVORITES = `${PROFILE}/favorites`;
export const favoritePath = (item) => `${FAVORITES}/game?${new URLSearchParams({ console: item.consoleId, ref: item.reference })}`;
// La source d'un favori : son serveur fédéré, sa ressource externe (`res<id>:`, `#808`), sinon ici — une lecture chacune.
const sourceKey = (item) => (item.serverId == null && resourceOfReference(item.reference) ? `res${resourceOfReference(item.reference)}` : item.serverId ?? "") + `|${item.consoleId}`;
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
export const titleOf = (item) => item.title || item.reference;

// CE QUE DEVIENT CHAQUE FAVORI : une fonction qui rend, pour un favori, `{console, game}` (son jeu trouvé), `{console,
// gone}` (`console` : sa console n'est plus ici ; `server` : son serveur n'est plus fédéré ; `game` : son jeu n'est
// plus chez sa source), `{console, refusal}` (sa source ne s'est pas lue : `denied`, `offline`, `failed`), ou undefined
// tant qu'on ne sait pas. Les serveurs fédérés se relisent ici une fois (`GET /servers`) : ceux de la page, vides tant
// qu'ils se lisent, feraient croire qu'un serveur est parti.
export function useFavoriteGames(items) {
    const { consoles, loadHere } = useData();
    const [reads, setReads] = useState({});
    const [federated, setFederated] = useState(null);
    const asked = useRef(new Set());
    const alive = useRef(true);
    const sources = [...new Set(items.map(sourceKey))];
    const wantsServers = items.some((item) => item.serverId != null);
    const playable = (consoleId) => (consoles || []).find((entry) => entry.id === consoleId && entry.method !== "remoteplay") || null;
    const known = (serverId) => Boolean(federated?.some((server) => String(server.id) === String(serverId)));

    useEffect(() => {
        alive.current = true;

        return () => {
            alive.current = false;
        };
    }, []);

    useEffect(() => {
        if (wantsServers && federated === null && !asked.current.has("servers")) {
            asked.current.add("servers");
            listServers().then((answer) => alive.current && setFederated(answer.ok ? answer.data || [] : []));
        }
    }, [wantsServers, federated]);

    useEffect(() => {
        if (!consoles) {
            return;
        }

        for (const source of sources) {
            const [serverId, consoleId] = source.split("|");
            const console = playable(consoleId);

            if (!console || asked.current.has(source) || (serverId && !serverId.startsWith("res") && (federated === null || !known(serverId)))) {
                continue;
            }
            asked.current.add(source);
            // Une ressource externe (`res<id>`) : ses jeux de la console, sous son nom.
            const reading = serverId.startsWith("res")
                ? listLentResources().then((answer) => {
                    const resource = (answer.ok ? answer.data || [] : []).find((entry) => `res${entry.id}` === serverId);

                    return resource ? readResourceGames(console, resource) : { ok: true, games: [] };
                })
                : readGames(console, serverId || null, loadHere);
            reading.then((result) => alive.current && setReads((previous) => ({ ...previous, [source]: result })));
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoles, federated, sources.join("\n")]);

    return (item) => {
        if (!consoles || (item.serverId != null && federated === null)) {
            return undefined;
        }
        const console = playable(item.consoleId);

        if (!console) {
            return { console: null, gone: "console" };
        }

        if (item.serverId != null && !known(item.serverId)) {
            return { console, gone: "server" };
        }
        const read = reads[sourceKey(item)];

        if (!read) {
            return undefined;
        }

        if (!read.ok) {
            return { console, refusal: read.refusal || "failed" };
        }
        const game = read.games.find((entry) => entry.reference === item.reference);

        return game ? { console, game } : { console, gone: "game" };
    };
}

// Pourquoi un favori n'a pas de fiche : son jeu, sa console ou son serveur partis, ou sa source qui ne se lit pas.
export const problemOf = (state, t) => (state?.gone ? t(`favorites.gone.${state.gone}`)
    : state?.refusal ? (state.refusal === "failed" ? t("games.loadFailed") : t(`games.peer.${state.refusal}`)) : null);

// D'où vient un favori, en court : celle de son jeu (originOf), sinon son serveur — nommé, ou par son numéro s'il n'est
// plus fédéré ; un jeu d'ici qui n'est plus là ne dit pas de quel disque il venait.
export const provenanceOf = (item, state, servers, t) => (state?.game ? originOf(state.game, servers, t)
    : item.serverId != null ? (state?.gone === "server" ? t("consoles.serverNamed", { id: item.serverId }) : serverName(servers, item.serverId)) : null);

// MES FAVORIS, RANGÉS ET LUS CHEZ LEUR SOURCE : `{favorites, items, stateOf, loading}` — par titre, puis par console ;
// `stateOf(item)` dit ce que chaque favori est devenu (useFavoriteGames) ; `loading` tant que la liste ou l'un d'eux se
// lit. La liste de cette page et celle de l'accueil « En direct » (Live.jsx) en sont faites.
export function useFavoriteList() {
    const favorites = useFavorites();
    const items = useMemo(() => [...favorites.items].sort((left, right) => collator.compare(titleOf(left), titleOf(right)) || collator.compare(left.consoleId, right.consoleId)), [favorites.items]);
    const stateOf = useFavoriteGames(items);

    return { favorites, items, stateOf, loading: !favorites.loaded || items.some((item) => stateOf(item) === undefined) };
}

// LA LIGNE D'UN FAVORI : sa jaquette, son titre, « console • provenance » — et, quand son jeu n'est plus là, pourquoi.
// `onOpen` : où elle mène (sa page ici, sa fiche à l'écran depuis « En direct »).
export function FavoriteRow({ id, item, state, onOpen }) {
    const { t } = useTranslation();
    const { servers } = useData();

    return (
        <NavRow id={id} label={state?.game?.title || titleOf(item)}
            lead={<GameCover consoleId={item.consoleId} reference={item.reference} game={romSource(state?.game || { peer: item.serverId })} />}
            help={[state?.console?.name || item.consoleId, problemOf(state, t) || provenanceOf(item, state, servers, t)].filter(Boolean)}
            onOpen={onOpen} />
    );
}

// LA LISTE — par titre, feuilletée (sa page dans l'adresse : ses lignes ouvrent une page) ; la recherche sur le titre et la
// console (chantier 81 : un listing a toujours sa recherche).
export default function Favorites() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const { favorites, items, stateOf, loading } = useFavoriteList();
    const [read, update] = useQueryState({ page: "1", q: "" });
    const search = read("q");
    const needle = search.trim().toLowerCase();
    const found = needle ? items.filter((item) => [stateOf(item)?.game?.title, titleOf(item), stateOf(item)?.console?.name, item.consoleId]
        .some((text) => String(text || "").toLowerCase().includes(needle))) : items;

    return (
        <SettingsPage title={t("settings.items.myFavorites.label")} parent={PROFILE}>
            <Section title={t("library.gamesTitle")} help={t("favorites.help")} sense="play">
                <Listing items={found} loading={loading} search={search} onSearch={(q) => update({ q, page: "1" })} placeholder={t("favorites.search")}
                    page={read("page")} onPage={(page) => update({ page })}
                    empty={t(favorites.failed ? "favorites.unreadable" : "favorites.empty")}
                    placeholderRow={() => <PlaceholderRow lead="cover" />}
                    render={(item) => {
                        const state = stateOf(item);
                        const key = favoriteKey(item.consoleId, item.reference);

                        return <FavoriteRow key={key} id={`fv-${key}`} item={item} state={state} onOpen={() => open(favoritePath(item), `fv-${key}`, state?.game ? { game: state.game } : {})} />;
                    }} />
            </Section>
            {/* LE GUIDE (le porteur, 2026-11-16 : « Toujours proposer des liens vers la documentation si possible »). */}
            <GuideSection guides={[["library", "browse-the-catalog-and-keep-favorites"]]} />
        </SettingsPage>
    );
}
