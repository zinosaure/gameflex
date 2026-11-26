// Route : /settings/federation/pending — les invitations pas encore échangées.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Section } from "../../bricks/Section.jsx";
import { fact } from "../../libs/format.js";
import SettingsPage, { useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../profile/Federation.jsx";
import { ACCESSES, dateTime, FEDERATION, invitationKind, loadInvitations, useLevels } from "../Federation.jsx";

// /settings/federation/pending : LES ACCÈS DÉLIVRÉS EN ATTENTE — les invitations créées ici et pas encore échangées
// (valables 12 heures, une seule fois) ; une ligne ouvre sa page.
export default function Pending() {
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ page: "1", q: "" });
    const [items, setItems] = useState([]);
    const [loading, setLoading] = useState(true);
    const levels = useLevels();
    // LA RECHERCHE (chantier 81 : un listing a toujours la sienne) : le nom de l'invitation.
    const search = read("q") || "";
    const needle = search.trim().toLowerCase();
    const found = items.filter((item) => !needle || String(item.label || "").toLowerCase().includes(needle));

    useEffect(() => {
        loadInvitations().then((invitations) => {
            setItems(invitations);
            setLoading(false);
        });
    }, []);

    return (
        <SettingsPage title={t("fed.pendingTitle")} parent={ACCESSES} levels={levels}>
            <Section title={t("invitations.pendingTitle")} help={t("invitations.pendingHelp")} sense="modify">
                <Listing items={found} loading={loading} empty={t("fed.pendingEmpty")} search={search} onSearch={(q) => update({ q, page: "1" })} placeholder={t("fed.search")}
                    searchKey="fd-inv-search" page={read("page")} onPage={(page) => update({ page })}
                    render={(item) => (
                        <NavRow key={item.id} id={`fd-inv-${item.id}`} icon="bi-qr-code" label={item.label || t("invitations.kindAccount")}
                            help={[fact(t("facts.type"), [].concat(invitationKind(item, t)).join(", ")), fact(t("invitations.expiresLabel"), dateTime(item.expires_at, i18n.language))]}
                            onOpen={() => open(`${FEDERATION}/invitations/${item.id}`, `fd-inv-${item.id}`, { invitation: item })} />
                    )} />
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}
