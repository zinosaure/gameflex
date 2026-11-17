import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useSearchParams } from "react-router-dom";
import { listServers, romSource } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { favoriteKey, setFavorite, useFavorites } from "../../libs/favorites.js";
import { useGameMetadata } from "../../libs/metadata.js";
import { splitTitle } from "../../libs/visuals.js";
import { toast } from "../../shell/Shell.jsx";
import { GameCover } from "../../screens/library/HeroArt.jsx";
import { GameSheet, originOf, readGames, serverName } from "../catalog/Games.jsx";
import SettingsPage, { NotFound, PROFILE, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { EmptyRow, GuideSection, InfoRow, Listing, Note, PlaceholderRow, Rows, Section } from "../../ui/Bricks.jsx";

// MES FAVORIS — Réglages › Profil › Mes favoris (le porteur, 2026-11-16 : « dans profile il faut le lien pour : mes
// favoris. Dedans il y aura la liste des favoris et quand on clique sur le jeu on voit la fiche du jeu (même si c'est un
// jeu fédéré) et on a lien pour le voir directement sur le front et voir ses sauvegardes. ») — ceux du COMPTE
// (libs/favorites.js : le service des comptes), posés depuis le Catalogue ou l'engrenage d'un jeu. Deux pages :
//
//   /settings/profile/favorites                      la liste : chaque favori, sa jaquette, son titre, « console ·
//                                                    provenance » — et, quand son jeu n'est plus là, pourquoi ;
//   /settings/profile/favorites/game?console=&ref=   un favori : LA FICHE DU CATALOGUE (GameSheet, catalog/Games.jsx),
//                                                    la même, jeu fédéré compris — ses informations, son téléchargement,
//                                                    puis « Plus » : la fiche à l'écran, ses sauvegardes (ici, ou chez son
//                                                    serveur fédéré), le retirer de mes favoris. Un favori dont le jeu
//                                                    n'est plus là a sa page aussi : ce qu'on en sait, et le retirer.
//
// SON JEU SE LIT CHEZ SA SOURCE comme le Catalogue le lit (readGames) : d'ici, la bibliothèque et ses disques ; d'un
// serveur fédéré, la copie de son listing — une lecture par console et par source, quel que soit le nombre de favoris.
// Rien n'est inventé : un jeu absent de sa source, une console partie, un serveur qui n'est plus fédéré le disent ; une
// source qui ne se lit pas (son refus, son silence) le dit aussi, sans déclarer le jeu disparu.

export const FAVORITES = `${PROFILE}/favorites`;
export const favoritePath = (item) => `${FAVORITES}/game?${new URLSearchParams({ console: item.consoleId, ref: item.reference })}`;
const sourceKey = (item) => `${item.serverId ?? ""}|${item.consoleId}`;
const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: "base" });
const titleOf = (item) => item.title || item.reference;

// CE QUE DEVIENT CHAQUE FAVORI : une fonction qui rend, pour un favori, `{console, game}` (son jeu trouvé), `{console,
// gone}` (`console` : sa console n'est plus ici ; `server` : son serveur n'est plus fédéré ; `game` : son jeu n'est
// plus chez sa source), `{console, refusal}` (sa source ne s'est pas lue : `denied`, `offline`, `failed`), ou undefined
// tant qu'on ne sait pas. Les serveurs fédérés se relisent ici une fois (`GET /servers`) : ceux de la page, vides tant
// qu'ils se lisent, feraient croire qu'un serveur est parti.
function useFavoriteGames(items) {
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

            if (!console || asked.current.has(source) || (serverId && (federated === null || !known(serverId)))) {
                continue;
            }
            asked.current.add(source);
            readGames(console, serverId || null, loadHere).then((result) => alive.current && setReads((previous) => ({ ...previous, [source]: result })));
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
const problemOf = (state, t) => (state?.gone ? t(`favorites.gone.${state.gone}`)
    : state?.refusal ? (state.refusal === "failed" ? t("games.loadFailed") : t(`games.peer.${state.refusal}`)) : null);

// D'où vient un favori, en court : celle de son jeu (originOf), sinon son serveur — nommé, ou par son numéro s'il n'est
// plus fédéré ; un jeu d'ici qui n'est plus là ne dit pas de quel disque il venait.
const provenanceOf = (item, state, servers, t) => (state?.game ? originOf(state.game, servers, t)
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

// LA LIGNE D'UN FAVORI : sa jaquette, son titre, « console · provenance » — et, quand son jeu n'est plus là, pourquoi.
// `onOpen` : où elle mène (sa page ici, sa fiche à l'écran depuis « En direct »).
export function FavoriteRow({ id, item, state, onOpen }) {
    const { t } = useTranslation();
    const { servers } = useData();

    return (
        <NavRow id={id} label={state?.game?.title || titleOf(item)}
            lead={<GameCover consoleId={item.consoleId} reference={item.reference} game={romSource(state?.game || { peer: item.serverId })} />}
            help={[state?.console?.name || item.consoleId, provenanceOf(item, state, servers, t), problemOf(state, t)].filter(Boolean).join(" · ")}
            onOpen={onOpen} />
    );
}

// /settings/profile/favorites : LA LISTE — par titre, feuilletée (sa page dans l'adresse : ses lignes ouvrent une page).
function FavoritesPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const { favorites, items, stateOf, loading } = useFavoriteList();
    const [read, update] = useQueryState({ page: "1" });

    return (
        <SettingsPage title={t("settings.items.myFavorites.label")} parent={PROFILE}>
            <Section title={t("library.gamesTitle")} help={t("favorites.help")}>
                <Listing items={items} loading={loading} page={read("page")} onPage={(page) => update({ page })}
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

// UN FAVORI SANS FICHE (son jeu, sa console ou son serveur partis ; sa source muette) : ce qu'on en sait, pourquoi, et
// le retirer — le même geste que celui de la fiche, à la même place.
function MissingFavorite({ item, state, onRemoved }) {
    const { t } = useTranslation();
    const { servers } = useData();
    const [pending, setPending] = useState(false);
    const provenance = provenanceOf(item, state, servers, t);

    const remove = async () => {
        setPending(true);
        const answer = await setFavorite(item, false);
        setPending(false);
        toast(answer.ok ? t("favorites.removed", { name: splitTitle(titleOf(item)).main }) : answer.detail || t("favorites.failed"));

        if (answer.ok) {
            onRemoved();
        }
    };

    return (
        <>
            <Section title={t("gear.informations")}>
                <Rows>
                    <InfoRow icon="bi-joystick" label={t("catalog.console")} value={state.console?.name || item.consoleId} />
                    {provenance && <InfoRow icon="bi-arrow-left-right" tone="green" label={t("game.origin")} value={provenance} />}
                    <EmptyRow icon="bi-slash-circle" text={problemOf(state, t)} />
                </Rows>
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="cg-favorite" icon="bi-star-fill" tone="amber" label={t("favorites.remove")} help={t("favorites.help")} pending={pending} opens="action" onOpen={remove} />
                </Rows>
            </Section>
            <GuideSection guides={[["library", "browse-the-catalog-and-keep-favorites"]]} />
        </>
    );
}

// /settings/profile/favorites/game?console=&ref= : UN FAVORI — son jeu, celui que la liste a passé (l'historique), sinon
// relu chez sa source (un lien direct, un rechargement). Retiré de mes favoris, retour à la liste : la page le garde le
// temps de partir, au lieu de dire « Introuvable ».
function FavoritePage() {
    const { t } = useTranslation();
    const [params] = useSearchParams();
    const location = useLocation();
    const { consoles } = useData();
    const { back } = useSettingsNavigation();
    const favorites = useFavorites();
    const consoleId = params.get("console") || "";
    const reference = params.get("ref") || "";
    const listed = favorites.items.find((entry) => entry.consoleId === consoleId && entry.reference === reference) || null;
    const kept = useRef(null);
    const item = listed || (kept.current?.consoleId === consoleId && kept.current.reference === reference ? kept.current : null);
    const items = useMemo(() => (item ? [item] : []), [item]);
    const stateOf = useFavoriteGames(items);
    const console = (consoles || []).find((entry) => entry.id === consoleId && entry.method !== "remoteplay") || null;
    const given = location.state?.game?.consoleId === consoleId && location.state.game.reference === reference ? location.state.game : null;
    const state = (item ? stateOf(item) : undefined) ?? (given && console ? { console, game: given } : undefined);
    const metadata = useGameMetadata(state?.game ? consoleId : null, reference, romSource(state?.game));
    const leave = () => back(FAVORITES);

    // Le favori vu en dernier, gardé le temps de partir une fois retiré.
    useEffect(() => {
        if (listed) {
            kept.current = listed;
        }
    });

    return (
        <SettingsPage title={metadata?.title || state?.game?.title || (item ? titleOf(item) : reference)} parent={FAVORITES} levels={{ [FAVORITES]: t("settings.items.myFavorites.label") }}>
            {!favorites.loaded || (item && state === undefined) ? <Section title={t("gear.informations")}><Note className="mt-0">{t("settings.loading")}</Note></Section>
                : !item ? <NotFound />
                    : state.game ? <GameSheet console={state.console} game={state.game} onRemoved={leave} />
                        : <MissingFavorite item={item} state={state} onRemoved={leave} />}
        </SettingsPage>
    );
}

// /settings/profile/favorites/* (Profile.jsx).
export function MyFavoritesRoutes() {
    return (
        <Routes>
            <Route index element={<FavoritesPage />} />
            <Route path="game" element={<FavoritePage />} />
            <Route path="*" element={<Navigate to={FAVORITES} replace />} />
        </Routes>
    );
}
