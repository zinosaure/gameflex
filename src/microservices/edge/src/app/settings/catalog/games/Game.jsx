// Route : /settings/catalog/games/game?console=&ref= — un jeu du Catalogue. Son guide est celui de la fiche (GameSheet) ;
// la page en pose un tant que la fiche n'est pas là (elle se lit, ou le jeu est introuvable).
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useSearchParams } from "react-router-dom";
import { Loading } from "../../../bricks/Loading.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { listLentResources, romSource } from "../../../libs/api.js";
import { useData } from "../../../libs/data.jsx";
import { useGameMetadata } from "../../../libs/metadata.js";
import SettingsPage, { NotFound } from "../../SettingsPage.jsx";
import { CATALOG, catalogConsole, GameSheet, peerOfReference, readGames, readResourceGames, resourceOfReference } from "../Games.jsx";

// /settings/catalog/games/game?console=&ref= : UN JEU — celui que la liste a passé (l'historique), sinon relu chez sa
// source (un lien direct, un rechargement) : d'ici par la liste de sa console, d'un serveur fédéré par la sienne.
export default function CatalogGame() {
    const { t } = useTranslation();
    const [params] = useSearchParams();
    const location = useLocation();
    const { consoles, loadHere } = useData();
    const consoleId = params.get("console") || "";
    const reference = params.get("ref") || "";
    const console = catalogConsole(consoles, consoleId);
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
        // La source que dit la référence : une ressource externe (`res<id>:`), un serveur fédéré (`srv<id>:`), sinon ici.
        const resourceId = resourceOfReference(reference);
        const reading = resourceId
            ? listLentResources().then((answer) => {
                const resource = (answer.ok ? answer.data || [] : []).find((entry) => String(entry.id) === resourceId);

                return resource ? readResourceGames(console, resource) : { ok: false, games: [] };
            })
            : readGames(console, peerOfReference(reference), loadHere);
        reading.then((result) => {
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
        <SettingsPage title={metadata?.title || game?.title || reference} parent={CATALOG} levels={{ [CATALOG]: t("settings.items.catalogGames.label") }}
            guides={game ? null : [["library", "browse-the-catalog-and-keep-favorites"], ["federation"]]}>
            {game === undefined ? <Section title={t("details")} sense="info"><Loading /></Section>
                : game === null || !console ? <NotFound /> : <GameSheet console={console} game={game} />}
        </SettingsPage>
    );
}
