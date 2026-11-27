// Route : /settings/profile/favorites/game?console=&ref= — un favori : LA FICHE DU CATALOGUE (GameSheet,
// catalog/Games.jsx), la même, jeu fédéré compris — ses informations, son téléchargement, puis « Plus » : la fiche à
// l'écran, ses sauvegardes (ici, ou chez son serveur fédéré), le retirer de mes favoris. Un favori dont le jeu n'est plus
// là a sa page aussi : ce qu'on en sait, et le retirer.
import React, { useEffect, useMemo, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useSearchParams } from "react-router-dom";
import { romSource } from "../../../libs/api.js";
import { useData } from "../../../libs/data.jsx";
import { setFavorite, useFavorites } from "../../../libs/favorites.js";
import { useGameMetadata } from "../../../libs/metadata.js";
import { splitTitle } from "../../../libs/visuals.js";
import { toast } from "../../../shell/Shell.jsx";
import { catalogConsole, GameSheet } from "../../catalog/Games.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { EmptyRow } from "../../../bricks/EmptyRow.jsx";
import { GameFacts } from "../../../bricks/GameFacts.jsx";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { FAVORITES, problemOf, provenanceOf, titleOf, useFavoriteGames } from "../Favorites.jsx";

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
            <Section title={t("details")} help={t("favorites.goneHelp")} sense="info">
                {/* Pourquoi il n'est plus là, en tête comme dans l'engrenage d'un jeu (pourquoi il ne partirait pas). */}
                <GameFacts facts={{ console: state.console?.name || item.consoleId, origin: provenance }}>
                    <EmptyRow icon="bi-slash-circle" text={problemOf(state, t)} />
                </GameFacts>
            </Section>
            <Section title={t("more")} help={t("favorites.removeSectionHelp")} sense="modify">
                <Rows>
                    <NavRow id="cg-favorite" icon="bi-star-fill" label={t("favorites.remove")} help={t("favorites.help")} pending={pending} opens="action" onOpen={remove} />
                </Rows>
            </Section>
            <GuideSection guides={[["library", "browse-the-catalog-and-keep-favorites"]]} />
        </>
    );
}

// /settings/profile/favorites/game?console=&ref= : UN FAVORI — son jeu, celui que la liste a passé (l'historique), sinon
// relu chez sa source (un lien direct, un rechargement). Retiré de mes favoris, retour à la liste : la page le garde le
// temps de partir, au lieu de dire « Introuvable ».
export default function FavoriteGame() {
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
    const console = catalogConsole(consoles, consoleId);
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
            {!favorites.loaded || (item && state === undefined) ? <Section title={t("details")} sense="info"><Loading /></Section>
                : !item ? <NotFound />
                    : state.game ? <GameSheet console={state.console} game={state.game} onRemoved={leave} />
                        : <MissingFavorite item={item} state={state} onRemoved={leave} />}
        </SettingsPage>
    );
}
