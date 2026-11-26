import React from "react";
import { useTranslation } from "react-i18next";
import { RangeRow } from "./RangeRow.jsx";

// L'OPACITÉ DE LA MANETTE À L'ÉCRAN (#758) : la même glissière dans l'éditeur de disposition (Réglages › Manettes ›
// Modifier la disposition) et dans le menu d'une partie — l'opacité de la disposition, de 1 à 100 % par pas de 1 % (le
// porteur, 2026-11-17 : « la limite de l'opacité des boutons soit de 1% à 100% dans les glisseurs » ; 25 % au plus bas
// jusque-là), son pourcentage dans le libellé. Ce qu'elle règle (un emplacement en cours d'édition, ou celui en jeu) est
// l'affaire de la page.
export const PAD_OPACITY_MIN = 0.01;

export function PadOpacityRow({ id, value, onChange }) {
    const { t } = useTranslation();

    return (
        <RangeRow id={id} label={`${t("layout.opacity")} — ${Math.round(value * 100)} %`} help={t("layout.opacityHelp")} value={value} min={PAD_OPACITY_MIN} max={1} step={0.01}
            onChange={(next) => onChange(Number(next))} />
    );
}
