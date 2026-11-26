// Route : /settings/profile/federation/servers — les serveurs fédérés de ce GameFlex, une ligne chacun : là où
// m'authentifier.
import React from "react";
import { useTranslation } from "react-i18next";
import { useData } from "../../../libs/data.jsx";
import SettingsPage, { useQueryState, useSettingsNavigation } from "../../SettingsPage.jsx";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { Listing } from "../../../bricks/Listing.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { FEDERATION, FEDERATION_GUIDES, named, SERVERS } from "../Federation.jsx";

export default function Servers() {
    const { t } = useTranslation();
    const { servers } = useData();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ page: "1", q: "" });
    const search = read("q");
    const needle = search.trim().toLowerCase();
    const items = (servers || []).map(named).filter((server) => !needle || [server.name, server.link].some((text) => String(text || "").toLowerCase().includes(needle)));

    return (
        <SettingsPage title={t("myFed.servers")} parent={FEDERATION}>
            <Section title={t("myFed.serversTitle")} help={t("myFed.serversListHelp")} sense="modify">
                <Listing items={items} empty={t("myFed.serversEmpty")} search={search} onSearch={(q) => update({ q, page: "1" })}
                    placeholder={t("myFed.serversSearch")} page={read("page")} onPage={(page) => update({ page })}
                    render={(server) => (
                        <NavRow key={server.id} id={`mf-srv-${server.id}`} label={server.name} help={server.link}
                            onOpen={() => open(`${SERVERS}/${server.id}`, `mf-srv-${server.id}`)} />
                    )} />
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}

