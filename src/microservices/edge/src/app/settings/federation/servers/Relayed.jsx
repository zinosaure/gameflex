// Route : /settings/federation/servers/<id>/relayed — les serveurs qu'un serveur rejoint nous relaie.
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { useData } from "../../../libs/data.jsx";
import SettingsPage from "../../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../../profile/Federation.jsx";
import { JOINED, loadServers, RelayedServersSection, relayedThrough, useLevels, usePassed, Waiting } from "../../Federation.jsx";

// LES SERVEURS RELAYÉS D'UN SERVEUR REJOINT, SUR LEUR ROUTE (chantier 83 ; le porteur, 2026-11-26 : « serveurs relayés doit
// être un listing car on peut en avoir un nombre infini » ; un seul listing par route) : la ligne « Serveurs relayés » de
// sa fiche y mène.
export default function RelayedServers() {
    const { t, i18n } = useTranslation();
    const { serverId } = useParams();
    const { refreshServers, servers } = useData();
    const levels = useLevels();
    const server = usePassed("server", serverId, () => loadServers(refreshServers, t, i18n.language));
    const parent = `${JOINED}/${encodeURIComponent(serverId)}`;

    if (!server) {
        return <Waiting item={server} title={t("fed.grant.relayed.label")} parent={parent} />;
    }

    return (
        <SettingsPage title={t("fed.grant.relayed.label")} parent={parent} levels={{ ...levels, [parent]: server.label }} guides={FEDERATION_GUIDES}>
            <RelayedServersSection side="received" servers={relayedThrough(servers, server.id)} />
        </SettingsPage>
    );
}
