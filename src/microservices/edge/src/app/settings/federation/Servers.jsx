// Route : /settings/federation/servers — les serveurs rejoints.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { useData } from "../../libs/data.jsx";
import { fact } from "../../libs/format.js";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../profile/Federation.jsx";
import { FEDERATION, JOINED, loadServers } from "../Federation.jsx";

// /settings/federation/servers : LES SERVEURS REJOINTS — ceux qui nous ont donné un accès —, une ligne chacun : son nom
// et son adresse, rien d'autre (le porteur, 2026-11-16 : « Pas besoin des longs sous-titre. Juste l'adresse suffira. Dans
// la fiche on aura toutes les info ») ; puis « Plus » : en rejoindre un ; puis le guide.
export default function Joined() {
    const { t, i18n } = useTranslation();
    const { refreshServers } = useData();
    const { open } = useSettingsNavigation();
    const [items, setItems] = useState([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState("");

    useEffect(() => {
        loadServers(refreshServers, t, i18n.language).then((rows) => {
            setItems(rows);
            setLoading(false);
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const needle = search.trim().toLowerCase();
    const filtered = needle ? items.filter((item) => `${item.label} ${item.link} ${item.listing}`.toLowerCase().includes(needle)) : items;

    return (
        <SettingsPage title={t("fed.joinedTitle")} parent={FEDERATION}>
            <Section title={t("fed.serversTitle")} help={t("fed.serversHelp")} sense="modify">
                <Listing items={filtered} loading={loading} search={search} onSearch={setSearch} placeholder={t("fed.serversSearch")} searchKey="fd-srv-search"
                    found={t("fed.serversFound", { count: filtered.length })} empty={t("fed.serversEmpty")}
                    render={(item) => (
                        <NavRow key={item.id} id={`fd-srv-${item.id}`} icon="bi-lightning-charge" label={item.label} help={fact(t("fed.urlLabel"), item.link)}
                            onOpen={() => open(`${JOINED}/${item.id}`, `fd-srv-${item.id}`, { server: item })} />
                    )} />
            </Section>
            <Section title={t("more")} help={t("fed.serversMoreHelp")} sense="add">
                <Rows>
                    <NavRow id="fd-add" icon="bi-box-arrow-in-right" label={t("fed.join")} help={t("fed.joinHelp")} onOpen={() => open(`${FEDERATION}/add`, "fd-add")} />
                </Rows>
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}
