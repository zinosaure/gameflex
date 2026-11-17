import React from "react";
import { useTranslation } from "react-i18next";
import { useData } from "../../libs/data.jsx";
import { qrImage } from "../../libs/share-link.js";
import { useShareOrigin } from "../../screens/access/Invitation.jsx";
import SettingsPage, { usePageText } from "../SettingsPage.jsx";
import { CopyField, EmptyState, GuideSection, Rows, Section, TextSection } from "../../ui/Bricks.jsx";

// INVITER (le porteur, 2026-11-16 : « rajoute un 4eme tuile : Inviter. Ce lien ouvre une page du settings avec un QR pour
// rejoindre le serveur et aussi le lien à copier » ; « Le tuile Inviter c'est pour tout le monde et ça permet d'avoir
// seulement le lien du serveur ») : l'ADRESSE de ce serveur, pour l'ouvrir sur un autre appareil — son QR code (le même
// dessin que celui d'une partie, libs/share-link.js), puis le lien à copier (CopyField). Rien ne se crée : ce n'est ni le
// lien d'un compte (/join/…, Membres), ni une invitation de serveur (/invite/…, Fédération) ; on s'y connecte avec son
// compte. Puis ce qu'il faut savoir, en section de texte (TextSection, comme le Guide), et les guides.
//
// L'ADRESSE est celle que l'hôte déclare pour être partagée (`shareOrigin` : sur le téléphone, son adresse sur le réseau
// local, jamais 127.0.0.1), sinon celle de la page — la règle de Jeton et de Membres (useShareOrigin). Un téléphone qui
// n'est sur aucun réseau la déclare VIDE : personne ne peut le joindre, et la page le dit plutôt que de montrer un lien
// qui mènerait chez celui qui le scanne.
export function InvitePage() {
    const { t } = useTranslation();
    const page = usePageText("invite");
    const { capabilities } = useData();
    const origin = useShareOrigin();
    const reachable = capabilities.shareOrigin !== "";

    return (
        <SettingsPage title={page.title}>
            {reachable ? (
                <>
                    <Section title={t("invite.qrTitle")} help={t("invite.qrHelp")}>
                        <div className="bg-white rounded-4 p-3 mx-auto gf-qr">
                            <img className="d-block w-100" src={qrImage(origin)} alt={t("invite.qrAlt")} />
                        </div>
                    </Section>
                    <Section title={t("invite.linkTitle")} help={t("invite.linkHelp")}>
                        <Rows><CopyField id="inv-link" value={origin} /></Rows>
                    </Section>
                </>
            ) : (
                <Section title={t("invite.linkTitle")}>
                    <EmptyState icon="bi-wifi-off" text={t("invite.offline")} />
                </Section>
            )}
            <TextSection title={t("invite.goodToKnow")} icon="bi-info-circle" items={[
                { key: "inv-network", title: t("invite.networkTitle"), text: t("invite.network") },
                { key: "inv-account", title: t("invite.accountTitle"), text: t("invite.account") },
            ]} />
            <GuideSection guides={[["remoteAccess"], ["android", "play-together-your-phone-as-the-server"]]} />
        </SettingsPage>
    );
}
