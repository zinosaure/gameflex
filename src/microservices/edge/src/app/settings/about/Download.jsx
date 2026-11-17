import React from "react";
import { useTranslation } from "react-i18next";
import { can, useData } from "../../libs/data.jsx";
import { followLink, openExternal, REPOSITORY_URL } from "../../libs/links.js";
import SettingsPage, { usePageText } from "../SettingsPage.jsx";
import NavRow, { GuideSection, InfoRow, Rows, Section } from "../../ui/Bricks.jsx";

// La dernière version publiée de l'application (Settings.jsx de la webapp, RELEASE_URL), sur le dépôt public.
const RELEASE_URL = `${REPOSITORY_URL}/releases/latest`;

// ─── La fin de la liste ─────────────────────────────────────────────────────
// Un geste y est une LIGNE, l'aide de son entrée en sous-titre — un geste qui n'appartient à aucun groupe est une ligne,
// pas un bouton (le porteur, 2026-11-13 : « Les boutons orphelins […] ne doivent pas être des boutons mais des liens ») ;
// seul, il est dans le groupe « Plus » (l'accueil de l'application), sinon dans le groupe qui le nomme (l'application
// Android, Se déconnecter). Aucun ne détruit rien : aucun n'est dans « Danger ».

// L'APPLICATION ANDROID (le porteur, 2026-11-14, chantier 53, P13 : « dans Android App, mets les mêmes texte que sur
// webapp. Explique ce qu'il faut faire. ») : les textes de la page de téléchargement de la webapp
// (profile/settings/Download.jsx, `download.*`), en deux groupes — le téléchargement, l'installation pas à pas —, puis
// le guide de l'application. PLUS DE « CE QUE FAIT CETTE VERSION » (le porteur, 2026-11-16 : « Le texte avec ce serveur
// est ambiguë […]. Pas besoin de la section : Ce que fait cette version ») : le guide le dit. Un banc d'essai (`staging`)
// donne l'APK qu'il construit, la version d'essai ; toute autre installation renvoie à la dernière version publiée sur
// GitHub, où le fichier s'appelle gameflex.apk — la première étape le nomme.
export function DownloadPage() {
    const { t } = useTranslation();
    const page = usePageText("download");
    const { capabilities } = useData();
    const staging = can(capabilities, "staging");
    const steps = [
        ["bi-1-circle", "installDownload", staging ? "installFromPhone" : "installFromRelease"],
        ["bi-2-circle", "installAllowTitle", "installAllow"],
        ["bi-3-circle", "installOpenTitle", "installOpen"],
    ];

    return (
        <SettingsPage title={page.title}>
            <Section title={t("download.title")}>
                <Rows>
                    <NavRow id="sc-download" icon="bi-download" tone="indigo" label={t(staging ? "settings.downloadApk" : "settings.downloadRelease")} help={page.help}
                        opens={staging ? "action" : "link"} onOpen={() => (staging
                            ? followLink("/downloads/gameflex.apk", { download: "gameflex.apk" })
                            : openExternal(RELEASE_URL))} />
                </Rows>
            </Section>
            <Section title={t("download.installTitle")}>
                <Rows>
                    {steps.map(([icon, label, text]) => <InfoRow key={label} icon={icon} tone="slate" label={t(`download.${label}`)} value={t(`download.${text}`)} />)}
                </Rows>
            </Section>
            <GuideSection guides={[["android", "install-the-app"]]} />
        </SettingsPage>
    );
}
