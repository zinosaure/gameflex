import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { logoutAllDevices, logoutOtherDevices } from "../../libs/api.js";
import { useAuth } from "../../libs/data.jsx";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { usePageText } from "../SettingsPage.jsx";
import NavRow, { Rows, Section, useConfirm } from "../../ui/Bricks.jsx";

// ─── La fin de la liste ─────────────────────────────────────────────────────
// Un geste y est une LIGNE, l'aide de son entrée en sous-titre — un geste qui n'appartient à aucun groupe est une ligne,
// pas un bouton (le porteur, 2026-11-13 : « Les boutons orphelins […] ne doivent pas être des boutons mais des liens ») ;
// seul, il est dans le groupe « Plus » (l'accueil de l'application), sinon dans le groupe qui le nomme (l'application
// Android, Se déconnecter). Aucun ne détruit rien : aucun n'est dans « Danger ».

// SE DÉCONNECTER (le porteur, 2026-11-14 : « c'est possible de rajouter "déconnexion des autres appareils seulement" et
// "déconnexion de tous les appareils" ») : cet appareil (son jeton seul s'efface : rien ne se coupe ailleurs) ; les
// autres appareils seulement (celui-ci reçoit un jeton neuf et reste connecté) ; tous les appareils (celui-ci compris).
// Les deux dernières coupent les sessions du compte — la génération des sessions, le mécanisme de la réinitialisation et
// de « Mon mot de passe » —, et demandent d'abord, par la fenêtre de Skeleton (« ? » à leur bord droit). « Tous les
// appareils » arrête aussi les parties en cours, comme une réinitialisation (le porteur, 2026-11-14 : « Oui ») ; « les
// autres appareils » les laisse, comme « Mon mot de passe » : rien ne dit de quel appareil une partie vient. Leur aide le
// dit. Rien ne détruit les données : aucune n'est dans « Danger ».
//
// DEUX GROUPES (le porteur, 2026-11-14, chantier 53, P13 : « dans sign out, mets log out other devices et log out all
// devices dans une autre section. Et du coup ne pas mettre sign out en couleur danger. ») : « Cet appareil », Se
// déconnecter seul, de la couleur d'une ligne ordinaire ; « Mes appareils », les deux qui coupent les sessions, rouges.
export function LogoutPage() {
    const { t } = useTranslation();
    const page = usePageText("logout");
    const { logout } = useAuth();
    const confirm = useConfirm();
    const [pending, setPending] = useState("");

    const cut = (key, call, done) => () => confirm({
        title: t(`settings.${key}`),
        message: t(`settings.${key}Confirm`),
        onConfirm: async () => {
            setPending(key);
            const answer = await call();
            setPending("");

            if (!answer.ok) {
                toast(answer.detail || t("settings.actionFailed"));

                return;
            }
            done();
        },
    });

    return (
        <SettingsPage title={page.title}>
            <Section title={t("settings.logoutThisDevice")}>
                <Rows>
                    <NavRow id="sc-logout" icon="bi-box-arrow-right" tone="indigo" label={t("settings.logout")} help={page.help} opens="action" onOpen={logout} />
                </Rows>
            </Section>
            <Section title={t("settings.logoutDevices")}>
                <Rows>
                    <NavRow id="sc-logout-others" icon="bi-phone" tone="rose" danger label={t("settings.logoutOthers")} help={t("settings.logoutOthersHelp")} opens="confirm"
                        pending={pending === "logoutOthers"} disabled={Boolean(pending)} onOpen={cut("logoutOthers", logoutOtherDevices, () => toast(t("settings.logoutOthersDone")))} />
                    <NavRow id="sc-logout-all" icon="bi-power" tone="rose" danger label={t("settings.logoutAll")} help={t("settings.logoutAllHelp")} opens="confirm"
                        pending={pending === "logoutAll"} disabled={Boolean(pending)} onOpen={cut("logoutAll", logoutAllDevices, logout)} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}
