// Route : /settings/resources/external/<id>/games — ses jeux.
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { ConsoleList } from "../../Consoles.jsx";
import { EXTERNAL_GUIDES, resourceGamesPath, useExternalResources } from "../../Resources.jsx";

// LES JEUX D'UNE RESSOURCE PAR CONSOLE (le porteur, 2026-11-23 : « Games » de sa page mène à ses jeux, et cette page
// « doit être comme pour settings/consoles ») : la liste des consoles des Réglages (ConsoleList), le nombre de jeux de la
// ressource sur chacune ; une console ouvre ses jeux, la ressource pour source.
export default function ExternalGames() {
    const { t } = useTranslation();
    const { id } = useParams();
    const { open } = useSettingsNavigation();
    const list = useExternalResources();
    const resource = (list || []).find((entry) => String(entry.id) === id);
    const parent = `/settings/resources/external/${id}`;

    return (
        <SettingsPage title={t("consoles.games")} parent={parent} levels={{ [parent]: resource?.name || "…" }} guides={EXTERNAL_GUIDES}>
            {list !== undefined && !resource ? <NotFound /> : (
                <ConsoleList countOf={(consoleId) => (resource ? resource.consoles?.[consoleId] || 0 : null)}
                    onOpen={(entry, rowId) => resource && open(resourceGamesPath(entry.id, resource), rowId)} />
            )}
        </SettingsPage>
    );
}
