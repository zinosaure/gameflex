// Route : /settings/federation/access — les accès délivrés.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { counted, fact } from "../../libs/format.js";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../profile/Federation.jsx";
import { FEDERATION, lastUseOf, loadInvitations, loadTokens, PENDING } from "../Federation.jsx";

// /settings/federation/access : LES ACCÈS DÉLIVRÉS — les jetons émis ici —, puis « Plus » : en créer un, et ceux qui
// attendent encore d'être échangés ; puis le guide.
export default function Accesses() {
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const [items, setItems] = useState([]);
    const [pending, setPending] = useState(null);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState("");

    useEffect(() => {
        Promise.all([loadTokens(), loadInvitations()]).then(([tokens, invitations]) => {
            setItems(tokens);
            setPending(invitations.length);
            setLoading(false);
        });
    }, []);

    const needle = search.trim().toLowerCase();
    const filtered = needle ? items.filter((item) => String(item.label || "").toLowerCase().includes(needle)) : items;

    return (
        <SettingsPage title={t("fed.accessesTitle")} parent={FEDERATION}>
            <Section title={t("fed.tokensTitle")} help={t("fed.tokensHelp")} sense="modify">
                <Listing items={filtered} loading={loading} search={search} onSearch={setSearch} placeholder={t("fed.search")} searchKey="fd-tok-search"
                    found={t("fed.found", { count: filtered.length })} empty={t("fed.empty")}
                    render={(item) => (
                        <NavRow key={item.id} id={`fd-tok-${item.id}`} icon="bi-key" label={item.label} help={fact(t("fed.lastUseLabel"), lastUseOf(item, t, i18n.language))}
                            onOpen={() => open(`${FEDERATION}/tokens/${item.id}`, `fd-tok-${item.id}`, { token: item })} />
                    )} />
            </Section>
            <Section title={t("more")} help={t("fed.accessMoreHelp")} sense="add">
                <Rows>
                    <NavRow id="fd-generate" icon="bi-key" label={t("fed.newAccess")} help={t("fed.newAccessHelp")} onOpen={() => open(`${FEDERATION}/new-token`, "fd-generate")} />
                    <NavRow id="fd-pairing" icon="bi-123" label={t("pairing.code")} help={t("pairing.codeRowHelp")} onOpen={() => open(`${FEDERATION}/new-pairing`, "fd-pairing")} />
                    <NavRow id="fd-pending" icon="bi-qr-code" label={t("fed.pendingTitle")} help={t("fed.pendingHelp")} value={fact(t("facts.accesses"), counted(t, "accesses", pending))}
                        onOpen={() => open(PENDING, "fd-pending")} />
                </Rows>
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}
