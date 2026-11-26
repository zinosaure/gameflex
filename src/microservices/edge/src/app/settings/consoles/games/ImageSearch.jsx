// Route : /settings/consoles/<console>/games/<source>/image-search?ref=&kind= — remplacer une image.
import React from "react";
import { useTranslation } from "react-i18next";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { GameImageSearch, IMAGE_KINDS } from "../../server/consoles/GameMetadata.jsx";
import { gamePagePath, gameSourceOf, isLocal, sourcePath, useAddressedGame, useAppearanceLevels, useGameTitle } from "../Games.jsx";

// /…/games/<source>/image-search?ref=&kind= : REMPLACER L'IMAGE D'UNE SORTE (depuis sa page ; l'image actuelle
// au-dessus) — la recherche, préremplie du titre du jeu (GameImageSearch, GameMetadata.jsx). Posée, retour à la
// page de l'image.
export default function ImageSearchRoute() {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, params, game } = useAddressedGame();
    const { back } = useSettingsNavigation();
    const title = useGameTitle(consoleId, source, game);
    const kind = params.get("kind") || "";
    const known = IMAGE_KINDS.includes(kind);
    const levels = { ...useAppearanceLevels(consoleId, slug, title), ...(known ? { [`${sourcePath(consoleId, slug)}/image`]: t(`gameAdmin.meta.kinds.${kind}`) } : {}) };
    const parent = gamePagePath(consoleId, slug, reference, "image", { kind });

    return (
        <SettingsPage title={t("appearance.replaceTitle")} parent={parent} levels={levels} guides={[["consoles"], ["library", "browse-the-catalog-and-keep-favorites"]]}>
            {known && isLocal(source) && game !== null ? (
                <GameImageSearch consoleId={consoleId} reference={reference} game={gameSourceOf(source)} kind={kind} title={title} onDone={() => back(parent)} />
            ) : <NotFound />}
        </SettingsPage>
    );
}
