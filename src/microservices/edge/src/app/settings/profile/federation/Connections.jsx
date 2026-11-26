// Route : /settings/profile/federation/connections — QUI SE SERT DE MON JETON (#781 ; le porteur, 2026-11-18 : « on peut voir
// l'état d'un token mais on ne sait pas sur quel lien/appareils c'est utilisé ») — une ligne par serveur ou appareil
// qui l'a échangé contre une session, le plus récent d'abord : le nom qu'il se donne, l'adresse d'où il est venu, et
// quand. Régénérer le jeton vide la liste.
//
// EST-CE MOI ? (#787 ; le porteur, 2026-11-18 : « ce qui est important c'est de voir les connexions et savoir si c'est
// bien l'utilisateur qui l'a faite ou pas ») : chaque ligne dit aussi le compte qui a collé le jeton là-bas et ce
// qu'est l'appelant (un serveur GameFlex, l'application) — déclarés par lui —, et « Depuis votre réseau actuel » quand
// la connexion vient de l'adresse d'où on lit la page, la seule marque que l'appelant ne peut pas inventer (verte).
//
// RÉVOQUER (#787 ; le porteur, 2026-11-18 : « pour savoir si en cas d'autres personnes utilisent le token sans le
// savoir ») : le nom se déclare et l'adresse change, seule la régénération du jeton coupe sûrement quelqu'un — c'est
// donc ce que fait « Révoquer les connexions » (décision du porteur) : tout le monde est coupé, mes appareils compris,
// qui recollent le nouveau jeton. Le serveur réserve ce geste aux administrateurs (chantier 20) : grisé pour un membre,
// la raison dans son aide.
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { dateTime } from "../../../libs/format.js";
import { regenerateMyToken } from "../../../libs/api.js";
import { useAuth } from "../../../libs/data.jsx";
import SettingsPage, { useQueryState, useSettingsNavigation } from "../../SettingsPage.jsx";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { Listing } from "../../../bricks/Listing.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { atUsername } from "../../../globals/accounts.js";
import { CONNECTIONS, connectionKey, FEDERATION, FEDERATION_GUIDES, useConnections } from "../Federation.jsx";

export default function Connections() {
    const { t, i18n } = useTranslation();
    const feedback = useFeedback();
    const { user } = useAuth();
    const { open } = useSettingsNavigation();
    const [reread, setReread] = useState(0);
    const items = useConnections(reread);
    const [read, update] = useQueryState({ page: "1", q: "" });
    const search = read("q");
    const needle = search.trim().toLowerCase();
    const found = (items || []).filter((row) => !needle || [row.name, row.member, row.address].some((text) => String(text || "").toLowerCase().includes(needle)));

    const revoke = async () => {
        const answer = await regenerateMyToken();

        feedback.result(answer.ok, t("myFed.revoked"), answer.detail || t("settings.actionFailed"));
        if (answer.ok) {
            setReread((count) => count + 1);
        }
    };

    return (
        <SettingsPage title={t("myFed.connections")} parent={FEDERATION}>
            <Section title={t("myFed.connectionsTitle")} help={t("myFed.connectionsListHelp")} sense="info">
                <Listing items={found} loading={items === undefined} empty={t("myFed.connectionsEmpty")} search={search} onSearch={(q) => update({ q, page: "1" })}
                    placeholder={t("myFed.connectionsSearch")} page={read("page")} onPage={(page) => update({ page })}
                    render={(row, index) => {
                        // « compte • dernière utilisation » ; le reste dans sa fiche (le porteur, 2026-11-23 : « créer des liens vers
                        // les fiches détaillées quand il y a trop d'info à faire apparaître »). Venue de mon réseau actuel :
                        // la validation verte à côté de son nom.
                        const key = `mf-con-${index}`;

                        return (
                            <NavRow key={connectionKey(row)} id={key} label={row.name || t("myFed.connectionUnnamed")} checked={Boolean(row.here)}
                                help={[row.member ? atUsername(row.member) : "", dateTime(row.last_used_at, i18n.language)].filter(Boolean)}
                                onOpen={() => open(`${CONNECTIONS}/${encodeURIComponent(connectionKey(row))}`, key, { connection: row })} />
                        );
                    }} />
            </Section>
            <DangerSection id="mf-revoke" icon="bi-x-circle" label={t("myFed.revoke")} help={t("myFed.revokeHelp")}
                question={t("myFed.revokeConfirm")} disabled={!user?.id || !(items || []).length} onConfirm={revoke} />
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}

