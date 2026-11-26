import React from "react";
import { useTranslation } from "react-i18next";
import { DangerRow } from "./DangerRow.jsx";
import { Rows } from "./Rows.jsx";
import { Section } from "./Section.jsx";

// LA SECTION « DANGER » (le porteur, 2026-11-13 : « des liens dans une section qu'on va nommer "Danger" »), en bas
// de la page : ses gestes destructifs en lignes. `children` : des DangerRow ; ou les propriétés d'une seule.
export function DangerSection({ children = null, ...row }) {
    const { t } = useTranslation();

    return (
        <Section title={t("danger")} help={t("dangerHelp")} sense="danger">
            <Rows>{children || <DangerRow {...row} />}</Rows>
        </Section>
    );
}
