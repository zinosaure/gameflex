// Route : /settings/profile — mon compte, d'après la mise en forme du porteur (branche ui/porteur-2026-11-24) : mes
// informations, mes jeux, ce qui protège mon compte. Chaque ligne ouvre sa page sous /settings/profile/ (PROFILE), que
// Retour et le fil d'Ariane ramènent ici.
import React from "react";
import { useTranslation } from "react-i18next";
import { useAuth } from "../libs/data.jsx";
import SettingsPage, { PROFILE, useSettingsNavigation } from "./SettingsPage.jsx";
import { atUsername } from "../globals/accounts.js";
import { AccountName } from "../bricks/AccountName.jsx";
import { GuideSection } from "../bricks/GuideSection.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { Section } from "../bricks/Section.jsx";

// Les lignes de « Jeux » et de « Sessions et sécurité » : leur page, leur entrée (settings.items), leur icône.
const GAMES = [["saves", "mySaves", "bi-shield-check"], ["favorites", "myFavorites", "bi-star"]];
const SECURITY = [["federation", "myFederation", "bi-arrow-left-right"], ["password", "password", "bi-lock"], ["mfa", "mfa", "bi-shield-lock"]];

export default function Profile() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const { user, isAdmin } = useAuth();
    const rows = (lines) => lines.map(([path, key, icon]) => (
        <NavRow key={path} id={`pr-${path}`} icon={icon} label={t(`settings.items.${key}.label`)} help={t(`settings.items.${key}.help`)}
            onOpen={() => open(`${PROFILE}/${path}`, `pr-${path}`)} />
    ));

    // « Informations personnelles » : mon nom, l'insigne de l'administration à côté (AccountName, comme la liste des
    // membres), mon identifiant dessous.
    return (
        <SettingsPage title={t("settings.items.profile.label")}>
            <Section title={t("profile.personal")} help={t("profile.personalHelp")} sense="info">
                <Rows>
                    <NavRow id="pr-account" icon="bi-person" label={<AccountName name={user?.display_name || user?.username || ""} admin={isAdmin} />}
                        help={atUsername(user?.username || "")} onOpen={() => open(`${PROFILE}/account`, "pr-account")} />
                </Rows>
            </Section>
            <Section title={t("profile.games")} help={t("profile.gamesHelp")} sense="play">
                <Rows>{rows(GAMES)}</Rows>
            </Section>
            <Section title={t("profile.security")} help={t("profile.securityHelp")} sense="modify">
                <Rows>{rows(SECURITY)}</Rows>
            </Section>
            <GuideSection guides={[["library", "browse-the-catalog-and-keep-favorites"], ["remoteAccess", "keep-it-safe"]]} />
        </SettingsPage>
    );
}
