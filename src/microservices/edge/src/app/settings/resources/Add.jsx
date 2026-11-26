// Route : /settings/resources/add — ajouter une ressource externe.
import React from "react";
import { useTranslation } from "react-i18next";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { sortedKinds } from "../people/externalResources.jsx";
import { EXTERNAL_GUIDES } from "../Resources.jsx";

// AJOUTER UNE RESSOURCE EXTERNE : un genre par ligne, par ordre alphabétique (la page « Plus » de gameflex-federation).
export default function AddExternal() {
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();

    return (
        <SettingsPage title={t("resources.external.add")} parent="/settings/resources/external">
            <Section title={t("resources.external.kindsTitle")} help={t("fedService.more.help")} sense="modify">
                <Rows>
                    {sortedKinds(t, i18n.language).map(({ kind, icon, tone }) => (
                        <NavRow key={kind} id={`rx-${kind}`} icon={icon} label={t(`fedService.kinds.${kind}.label`)} help={t(`fedService.kinds.${kind}.help`)}
                            onOpen={() => open(`/settings/resources/add/${kind}`, `rx-${kind}`)} />
                    ))}
                </Rows>
            </Section>
            <GuideSection guides={EXTERNAL_GUIDES} />
        </SettingsPage>
    );
}
