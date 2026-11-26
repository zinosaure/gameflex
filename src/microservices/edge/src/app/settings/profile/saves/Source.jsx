// Route : /settings/profile/saves/<source> : SES SAUVEGARDES — un serveur fédéré, sous quel compte d'abord.
import React from "react";
import { useTranslation } from "react-i18next";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { Section } from "../../../bricks/Section.jsx";
import SettingsPage from "../../SettingsPage.jsx";
import { BASE, PlaceBody, SaveList, SAVES_GUIDES, usePlace } from "../Saves.jsx";

export default function Source() {
    const { t } = useTranslation();
    const place = usePlace();

    return (
        <SettingsPage title={place.name} parent={BASE} levels={place.levels}>
            <PlaceBody place={place}>
                <Section title={t("saves.listTitle")} help={t("saves.listHelp")} sense="info">
                    <SaveList where={place.where} base={place.base} />
                </Section>
            </PlaceBody>
            <GuideSection guides={SAVES_GUIDES} />
        </SettingsPage>
    );
}
