// Route : /settings/profile/saves/<source>/console/<console> : les jeux d'une console qui ont des sauvegardes — ceux du rétro
// (leurs emplacements), ou ceux d'une console récente (un dossier par jeu).
import React from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import NavRow from "../../../../bricks/NavRow.jsx";
import { Rows } from "../../../../bricks/Rows.jsx";
import { GuideSection } from "../../../../bricks/GuideSection.jsx";
import { Section } from "../../../../bricks/Section.jsx";
import { useData } from "../../../../libs/data.jsx";
import { counted, fact } from "../../../../libs/format.js";
import SettingsPage, { useSettingsNavigation } from "../../../SettingsPage.jsx";
import { childPath, PlaceBody, SaveConsoleGames, SaveRetroGames, SAVES_GUIDES, useConsoleGames, useConsoleName, usePlace } from "../../Saves.jsx";

export default function Console() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const place = usePlace();
    const { consoles } = useData();
    const { open } = useSettingsNavigation();
    const location = useLocation();
    const title = useConsoleName()(consoleId);
    const retro = (consoles || []).find((entry) => entry.id === consoleId)?.method === "retrogaming";
    const [games] = useConsoleGames(consoleId, retro ? null : place.where);

    return (
        <SettingsPage title={title} parent={place.base} levels={place.levels}>
            <PlaceBody place={place}>
                {retro ? (
                    <Section title={t("saves.gamesTitle")} help={t("saves.consoleHelp")} sense="info">
                        <SaveRetroGames consoleId={consoleId} where={{ ...place.where, base: place.base }} />
                    </Section>
                ) : (
                    <Section title={t("saves.gamesTitle")} help={t("saves.consoleHelp")} sense="info">
                        <SaveConsoleGames consoleId={consoleId} where={place.where} games={games} />
                    </Section>
                )}
                {!retro && (games || []).length > 1 && (
                    <Section title={t("more")} help={t("saves.consoleMoreHelp")} sense="modify">
                        <Rows>
                            <NavRow id="sx-all" icon="bi-collection" label={t("saves.wholeConsole", { name: title })} value={fact(t("facts.games"), counted(t, "games", games.length))}
                                onOpen={() => open(childPath(location, "all", "gamesPage"), "sx-all")} />
                        </Rows>
                    </Section>
                )}
            </PlaceBody>
            <GuideSection guides={SAVES_GUIDES} />
        </SettingsPage>
    );
}
