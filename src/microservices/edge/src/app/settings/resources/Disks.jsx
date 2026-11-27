// Route : /settings/resources/disks — les disques de ce serveur, un listing (Disks, ../Resources.jsx).
import React from "react";
import { useTranslation } from "react-i18next";
import SettingsPage from "../SettingsPage.jsx";
import { Disks as DiskListing } from "../Resources.jsx";
import { hostGuide } from "../../libs/capabilities.js";

export default function Disks() {
    const { t } = useTranslation();

    return (
        <SettingsPage title={t("resources.internalTitle")} parent="/settings/resources"
            guides={[hostGuide("disks", ["storage", "more-disks-of-games"])]}>
            <DiskListing />
        </SettingsPage>
    );
}
