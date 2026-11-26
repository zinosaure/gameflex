// Route : /settings/resources/external — les ressources externes, un listing (ExternalResources, ../Resources.jsx), puis
// en ajouter une.
import React from "react";
import { useTranslation } from "react-i18next";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { EXTERNAL_GUIDES, ExternalResources } from "../Resources.jsx";

export default function Externals() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();

    return (
        <SettingsPage title={t("resources.external.title")} parent="/settings/resources" guides={EXTERNAL_GUIDES}>
            <ExternalResources />
            <Section title={t("more")} help={t("resources.external.addSectionHelp")} sense="add">
                <Rows>
                    <NavRow id="rs-add-external" icon="bi-plus-lg" label={t("resources.external.add")} help={t("resources.external.addHelp")}
                        onOpen={() => open("/settings/resources/add", "rs-add-external")} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}
