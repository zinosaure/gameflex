// Route : /settings/consoles/<console>/games/<source>/appearance?ref= — ses images.
import React from "react";
import { useTranslation } from "react-i18next";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { GameAppearance } from "../../server/consoles/GameMetadata.jsx";
import { gamePagePath, gameSourceOf, isLocal, useAddressedGame, useGameTitle, useLevels } from "../Games.jsx";

// /…/games/<source>/appearance?ref= : LES IMAGES D'UN JEU (une ligne de son « Plus » ; chantier 53, P9, le modèle 31) —
// ses informations, ses images une ligne par sorte (GameAppearance, GameMetadata.jsx).
export default function Appearance() {
    const { t } = useTranslation();
    const { consoleId, slug, source, reference, game } = useAddressedGame();
    const { open } = useSettingsNavigation();
    const title = useGameTitle(consoleId, source, game);
    const levels = useLevels(consoleId, slug, title);

    return (
        <SettingsPage title={t("consoles.appearance")} parent={gamePagePath(consoleId, slug, reference)} levels={levels} guides={[["consoles"], ["library", "browse-the-catalog-and-keep-favorites"]]}>
            {!isLocal(source) || game === null ? <NotFound /> : (
                <GameAppearance consoleId={consoleId} reference={reference} game={gameSourceOf(source)} title={title}
                    onOpenImage={(kind) => open(gamePagePath(consoleId, slug, reference, "image", { kind }), `ap-kind-${kind}`, { game })} />
            )}
        </SettingsPage>
    );
}
