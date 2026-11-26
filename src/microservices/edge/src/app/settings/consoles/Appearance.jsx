// Route : /settings/consoles/<console>/appearance — son nom, son fabricant, ses fonds.
import { useFeedback } from "../../bricks/useFeedback.jsx";
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Listing, useListSearch } from "../../bricks/Listing.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Note } from "../../bricks/Note.jsx";
import { PlaceholderRow } from "../../bricks/PlaceholderRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { resetConsoleBackgrounds } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../SettingsPage.jsx";
import { appearancePath, BackgroundThumb, consolePath, forgetSlots, useAppearanceLevels, useBackgroundSlots, useEmulated } from "../Consoles.jsx";
import { CONSOLE_ICON } from "../../globals/icons.js";

// L'APPARENCE D'UNE CONSOLE (chantier 53, P9, et les choix du porteur qui suivent) : ses huit images, toujours là —
// « on n'a pas besoin du lien "Ajouter une image" car on aura toujours 8 images. On ne peut que les remplacer. » —, une
// ligne chacune, qui ouvre sa page (Remplacer). Un emplacement que les fournisseurs n'ont pas pu remplir n'a pas de
// ligne : ni « Libre », ni « Ajouter ». En « Danger », LA VRAIE REMISE À ZÉRO (« le serveur vide les 8 emplacements, puis
// les remplit des images par défaut »), derrière la fenêtre de confirmation ; la liste se relit, et sa lecture refait le
// remplissage par défaut (metadata/console_backgrounds.go). UNE CONSOLE SANS AUCUNE IMAGE ICI montre celles du premier
// serveur fédéré qui en prête (`#740` ; le porteur, 2026-11-16 : « On prend le premier fédéré pour ne pas se prendre la
// tête ») : « Le dossier » dit leur provenance, et l'aide des images ce qu'un geste change — remplacer en écrit une ici,
// et la console montre alors les siennes ; réinitialiser redemande celles des fournisseurs de cette machine.
export default function Appearance() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { consoleId } = useParams();
    const { consoles } = useData();
    const { open } = useSettingsNavigation();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const [epoch, setEpoch] = useState(0);
    const read = useBackgroundSlots(consoleId, epoch);
    const filled = (read?.slots || []).filter((slot) => slot.name);
    // Les images en plus, qui peuvent être nombreuses : un listing, qui se cherche (les huit emplacements, eux, sont fixes).
    const extra = useListSearch(read?.extra || [], (name) => name);
    const levels = useAppearanceLevels(consoleId);

    const reset = async () => {
        const answer = await resetConsoleBackgrounds(consoleId);
        feedback.result(answer.ok, t("appearance.resetConsoleDone"), answer.errorMessage || t("settings.actionFailed"));

        if (answer.ok) {
            forgetSlots(consoleId);
            setEpoch((value) => value + 1);
        }
    };

    return (
        <SettingsPage title={t("consoles.appearance")} parent={consolePath(consoleId)} levels={levels} guides={[["consoles"], ["library", "3-get-the-emulators"]]}>
            {consoles && !entry ? <NotFound /> : (
                <>
                    <Section title={t("appearance.consoleTitle")} sense="modify">
                        {!entry ? <Loading /> : (
                            <Rows>
                                <InfoRow icon={CONSOLE_ICON} label={t("appearance.name")} value={entry.name} />
                                <InfoRow icon="bi-building" label={t("appearance.maker")} value={entry.publisher} />
                                <InfoRow icon="bi-calendar3" label={t("appearance.released")} value={entry.year ? String(entry.year) : null} />
                            </Rows>
                        )}
                    </Section>
                    <Section title={t("consoles.backgroundsFolderTitle")} help={t("consoles.backgroundsFolderHelp")} sense="modify">
                        <Rows>
                            <InfoRow icon="bi-folder2" label={t("consoles.backgroundsFolder")} value={`metadata/${consoleId}/background/`} mono />
                            {read?.server && <InfoRow icon="bi-arrow-left-right" label={t("game.origin")} value={read.serverName} />}
                        </Rows>
                    </Section>
                    <Section title={t("appearance.slotsTitle")} help={read?.server ? t("appearance.slotsLentHelp", { server: read.serverName }) : t("appearance.slotsHelp")} sense="modify">
                        {read?.failure !== undefined ? <Note className="mt-0">{read.failure || t("settings.actionFailed")}</Note> : (
                            <Listing items={filled} loading={read === undefined} empty={t("appearance.slotsNone")} placeholderRow={() => <PlaceholderRow lead="thumb" />}
                                render={(slot) => {
                                    const key = `ap-slot-${slot.slot}`;

                                    return (
                                        <NavRow key={slot.slot} id={key} lead={<BackgroundThumb consoleId={consoleId} name={slot.name} server={read.server} />} label={t("appearance.slot", { slot: slot.slot })}
                                            help={slot.name} onOpen={() => open(`${appearancePath(consoleId)}/${slot.slot}`, key)} />
                                    );
                                }} />
                        )}
                    </Section>
                    {read?.extra.length > 0 && (
                        <Section title={t("appearance.extraTitle")} help={t("appearance.extraHelp")} sense="modify">
                            <Listing items={extra.found} search={extra.search} onSearch={extra.onSearch} placeholder={t("appearance.searchImage")} searchKey="ap-extra-search"
                                render={(name) => (
                                <InfoRow key={name} lead={<BackgroundThumb consoleId={consoleId} name={name} />} label={name} value={t("appearance.extraValue")} />
                            )} />
                        </Section>
                    )}
                    <DangerSection id="ap-reset" icon="bi-arrow-counterclockwise" label={t("appearance.reset")} help={t("appearance.resetConsoleHelp")}
                        question={t("appearance.resetConsoleConfirm")} confirmLabel={t("appearance.resetAction")} onConfirm={reset} />
                </>
            )}
        </SettingsPage>
    );
}
