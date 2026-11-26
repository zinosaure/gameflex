import React from "react";
import { useTranslation } from "react-i18next";
import { Note } from "./Note.jsx";

// L'avis d'une orientation que l'écran n'a pas prise, sous ses lignes.
export function OrientationFailed({ turnsTheApp }) {
    const { t } = useTranslation();

    return <Note>{t(turnsTheApp ? "settings.orientationFailed" : "settings.orientationRefused")}</Note>;
}
