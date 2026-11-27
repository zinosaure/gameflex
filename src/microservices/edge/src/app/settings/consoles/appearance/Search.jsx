// Route : /settings/consoles/<console>/appearance/search — chercher un fond.
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams, useSearchParams } from "react-router-dom";
import { Loading } from "../../../bricks/Loading.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { searchConsoleBackgrounds, setConsoleBackground } from "../../../libs/api.js";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { ImageSearch } from "../../server/consoles/ImageSearch.jsx";
import { appearancePath, forgetSlots, SlotPreview, useAppearanceLevels, useBackgroundSlots } from "../../Consoles.jsx";

// /settings/consoles/<console>/appearance/search?slot=<n> : REMPLACER L'IMAGE D'UN EMPLACEMENT (le modèle 34, la
// recherche à la place de l'envoi) — « L'image actuelle » d'abord (le porteur, 2026-11-14 : « rajoute l'image de base en
// haut pour savoir quelle image on est en train de remplacer ») ; puis un titre de jeu de la console, ses images chez les
// fournisseurs ; une image choisie, la fenêtre, puis le serveur la récupère dans l'emplacement ; retour d'où l'on venait.
// Un emplacement vide n'a pas de page : on ne fait que remplacer (« On ne peut que les remplacer »).
export default function SlotSearch() {
    const { t } = useTranslation();
    const { consoleId } = useParams();
    const [params] = useSearchParams();
    const { back } = useSettingsNavigation();
    const read = useBackgroundSlots(consoleId);
    const slot = Number(params.get("slot"));
    const valid = Number.isInteger(slot) && slot >= 1 && slot <= 8;
    const name = read?.slots.find((item) => item.slot === slot)?.name || "";
    const parent = `${appearancePath(consoleId)}/${slot}`;
    const levels = useAppearanceLevels(consoleId, slot);

    return (
        <SettingsPage title={t("appearance.replaceSlotTitle", { slot })} parent={parent} levels={levels} guides={[["consoles"], ["library", "3-get-the-emulators"]]}>
            {!valid ? <NotFound /> : read === undefined ? <Section title={t("appearance.searchTitle")} sense="modify"><Loading /></Section> : !name ? <NotFound /> : (
                <>
                    <Section title={t("appearance.current")} sense="info">
                        <SlotPreview consoleId={consoleId} name={name} server={read.server} />
                    </Section>
                    <Section title={t("appearance.searchTitle")} help={t("appearance.searchConsoleHelp")} sense="modify">
                        <ImageSearch fit="cover" search={(query) => searchConsoleBackgrounds(consoleId, query)}
                            confirmOf={(image) => ({
                                title: t("appearance.replaceSlotConfirmTitle", { slot }),
                                message: t("appearance.chosenConfirm", { game: image.game, source: t(`images.services.${image.source}`) }),
                                confirmLabel: t("appearance.replaceAction"),
                            })}
                            success={() => t("appearance.slotSet", { slot })}
                            onChoose={async (image) => {
                                const answer = await setConsoleBackground(consoleId, slot, image);

                                if (answer.ok) {
                                    forgetSlots(consoleId);
                                    back(parent);
                                }

                                return answer;
                            }} />
                    </Section>
                </>
            )}
        </SettingsPage>
    );
}
