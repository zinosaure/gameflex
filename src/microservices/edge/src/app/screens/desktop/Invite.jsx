// Route : /desktop/invite — INVITER, COMME L'APK : le code QR et l'adresse de ce PC sur le réseau.
import React from "react";
import { useTranslation } from "react-i18next";
import { qrImage } from "../../libs/share-link.js";
import { CopyField } from "../../bricks/CopyField.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { DesktopPage, useDesktopState } from "./shared.jsx";

export default function Invite() {
    const { t } = useTranslation();
    const { state } = useDesktopState();
    const address = state.lanUrl || state.localUrl;

    return (
        <DesktopPage title={t("desktop.invite")} guides={[["remoteAccess"]]}>
            <Section title={t("desktop.inviteTitle")} help={t("desktop.inviteHelp")} sense="info">
                <Rows>
                    <div className="list-group-item gf-row">
                        <div className="bg-white rounded-4 p-3 mx-auto gf-qr"><img className="d-block w-100" src={qrImage(address)} alt="" /></div>
                    </div>
                    <CopyField id="dc-address" label={t("desktop.address")} value={address} />
                </Rows>
            </Section>
        </DesktopPage>
    );
}
