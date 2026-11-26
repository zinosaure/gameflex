import React from "react";
import { useTranslation } from "react-i18next";
import { Note } from "./Note.jsx";

// « Chargement… » : la note d'une section qui se lit encore, la même partout.
export function Loading() {
    const { t } = useTranslation();

    return <Note className="mt-0">{t("settings.loading")}</Note>;
}
