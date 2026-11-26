// Route : /settings/consoles/<console>/games/<source>/image?ref=&kind= — une image.
import React from "react";
import { useTranslation } from "react-i18next";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { GameImage, IMAGE_KINDS } from "../../server/consoles/GameMetadata.jsx";
import { gamePagePath, gameSourceOf, isLocal, useAddressedGame, useAppearanceLevels, useGameTitle } from "../Games.jsx";

// /…/games/<source>/image?ref=&kind= : une image de la fiche d'un jeu (le modèle 33) — en grand, d'où elle vient ; la
// remplacer (la recherche ; GameImage, GameMetadata.jsx).
export default function Image() {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, params, game } = useAddressedGame();
    const { open } = useSettingsNavigation();
    const title = useGameTitle(consoleId, source, game);
    const levels = useAppearanceLevels(consoleId, slug, title);
    const kind = params.get("kind") || "";
    const known = IMAGE_KINDS.includes(kind);

    return (
        <SettingsPage title={known ? t(`gameAdmin.meta.kinds.${kind}`) : kind} parent={gamePagePath(consoleId, slug, reference, "appearance")} levels={levels} guides={[["consoles"], ["library", "browse-the-catalog-and-keep-favorites"]]}>
            {known && isLocal(source) && game !== null ? (
                <GameImage consoleId={consoleId} reference={reference} game={gameSourceOf(source)} kind={kind}
                    onReplace={() => open(gamePagePath(consoleId, slug, reference, "image-search", { kind }), "ap-replace", { game })} />
            ) : <NotFound />}
        </SettingsPage>
    );
}
