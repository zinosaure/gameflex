// Route : /settings/consoles/<console>/appearance/<fond> — un fond.
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import NavRow from "../../../bricks/NavRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { appearancePath, SlotPreview, slotSearchPath, useAppearanceLevels, useBackgroundSlots } from "../../Consoles.jsx";

// /settings/consoles/<console>/appearance/<n> : UN EMPLACEMENT PRIS (le modèle 33) — l'image en grand, ses détails ;
// « Remplacer cette image » (la recherche). Pas de « Supprimer » ni de section « Danger » : une image ne se supprime pas
// seule, elle se remplace (le porteur, 2026-11-14). Vide ou hors de 1 à 8, la page n'existe pas.
export default function Slot() {
    const { t } = useTranslation();
    const { consoleId, slot: slotParam } = useParams();
    const { open } = useSettingsNavigation();
    const read = useBackgroundSlots(consoleId);
    const slot = Number(slotParam);
    const name = read?.slots.find((item) => item.slot === slot)?.name || "";
    const parent = appearancePath(consoleId);
    const levels = useAppearanceLevels(consoleId);

    return (
        <SettingsPage title={t("appearance.slot", { slot: slotParam })} parent={parent} levels={levels} guides={[["consoles"], ["library", "3-get-the-emulators"]]}>
            {read === undefined ? <Section title={t("appearance.preview")} sense="info"><Loading /></Section> : !name ? <NotFound /> : (
                <>
                    <Section title={t("appearance.preview")} sense="info">
                        <SlotPreview consoleId={consoleId} name={name} server={read.server} />
                    </Section>
                    <Section title={t("details")} sense="info">
                        <Rows>
                            <InfoRow icon="bi-collection" label={t("appearance.slotLabel")} value={t("appearance.slotOf", { slot })} />
                            <InfoRow icon="bi-file-earmark-image" label={t("consoles.paramsFileLabel")} value={name} mono />
                            {read.server && <InfoRow icon="bi-arrow-left-right" label={t("game.origin")} value={read.serverName} />}
                        </Rows>
                    </Section>
                    <Section title={t("more")} sense="modify">
                        <Rows>
                            <NavRow id="ap-replace" icon="bi-search" label={t("appearance.replace")} help={t("appearance.replaceSlotHelp")}
                                onOpen={() => open(slotSearchPath(consoleId, slot), "ap-replace")} />
                        </Rows>
                    </Section>
                </>
            )}
        </SettingsPage>
    );
}
