// Route : /desktop/location — l'emplacement du dossier GameFlex de ce PC ; le changer redémarre le serveur.
import React from "react";
import { useTranslation } from "react-i18next";
import { SettingForm, useDesktopState } from "./shared.jsx";

export default function Location() {
    const { t } = useTranslation();
    const { save } = useDesktopState();

    return <SettingForm field="library" title={t("desktop.location")} help={t("desktop.locationHelp")} guides={[["library"], ["storage"]]}
        apply={(value) => save({ library: value })} />;
}
