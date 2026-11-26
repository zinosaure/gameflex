// Route : /desktop/port — le port du serveur GameFlex de ce PC ; le changer redémarre le serveur.
import React from "react";
import { useTranslation } from "react-i18next";
import { SettingForm, useDesktopState } from "./shared.jsx";

export default function Port() {
    const { t } = useTranslation();
    const { save } = useDesktopState();

    return <SettingForm field="port" kind="number" title={t("desktop.port")} help={t("desktop.portHelp")} guides={[["remoteAccess"]]}
        apply={(value) => save({ port: Number(value) })} />;
}
