import React from "react";
import { useTranslation } from "react-i18next";
import SettingsPage, { usePageText } from "../SettingsPage.jsx";
import { Note, Section } from "../../ui/Bricks.jsx";

// La licence : rien encore, ni ici ni dans l'interface actuelle (« à discuter ») — dit tel quel, en ligne atténuée (plus
// d'alerte, le porteur, 2026-11-16).
export function LicencePage() {
    const { t } = useTranslation();
    const page = usePageText("licence");

    return (
        <SettingsPage title={page.title}>
            <Section>
                <Note className="mt-0">{t("settings.licenceSoon")}</Note>
            </Section>
        </SettingsPage>
    );
}
