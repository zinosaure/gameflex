// Route : /settings/profile/federation/connections/<clé> — LA FICHE D'UNE CONNEXION (le porteur, 2026-11-23) : tout ce qu'elle a donné et ce que le serveur en a vu — son compte, son
// genre, son adresse (« Depuis votre réseau actuel » quand elle vient d'où on lit la page), combien de fois et quand.
import React from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import { counted, dateTime } from "../../../../libs/format.js";
import SettingsPage, { NotFound } from "../../../SettingsPage.jsx";
import { InfoRow } from "../../../../bricks/InfoRow.jsx";
import { Loading } from "../../../../bricks/Loading.jsx";
import { Rows } from "../../../../bricks/Rows.jsx";
import { Section } from "../../../../bricks/Section.jsx";
import { atUsername } from "../../../../globals/accounts.js";
import { CONNECTIONS, connectionIcon, connectionKey, FEDERATION_GUIDES, useConnections } from "../../Federation.jsx";

export default function Connection() {
    const { t, i18n } = useTranslation();
    const { key } = useParams();
    const location = useLocation();
    const items = useConnections();
    const row = location.state?.connection || (items || []).find((entry) => connectionKey(entry) === key) || (items === undefined ? undefined : null);
    const kind = row?.kind === "server" ? t("myFed.connectionServer") : row?.kind === "phone" ? t("myFed.connectionPhone") : "";

    return (
        <SettingsPage title={row ? row.name || t("myFed.connectionUnnamed") : t("myFed.connectionsTitle")} parent={CONNECTIONS} levels={{ [CONNECTIONS]: t("myFed.connections") }} guides={FEDERATION_GUIDES}>
            {row === undefined ? <Section><Loading /></Section> : row === null ? <NotFound /> : (
                <Section title={t("details")} help={t("myFed.connectionDetailsHelp")} sense="info">
                    <Rows>
                        <InfoRow icon="bi-person" label={t("members.username")} value={row.member ? atUsername(row.member) : ""} />
                        <InfoRow icon={connectionIcon({ kind: row.kind })} label={t("facts.type")} value={kind} />
                        <InfoRow icon={row.here ? "bi-house-check" : "bi-link-45deg"} label={t("fed.urlLabel")}
                            value={row.address || t("myFed.connectionNoAddress")} help={row.here ? t("myFed.connectionHere") : null} />
                        <InfoRow icon="bi-box-arrow-in-right" label={t("myFed.connectionsTitle")} value={counted(t, "connections", row.use_count || 0)} />
                        <InfoRow icon="bi-calendar3" label={t("facts.firstUse")} value={dateTime(row.first_used_at, i18n.language)} />
                        <InfoRow icon="bi-clock-history" label={t("fed.lastUseLabel")} value={dateTime(row.last_used_at, i18n.language)} />
                    </Rows>
                </Section>
            )}
        </SettingsPage>
    );
}
