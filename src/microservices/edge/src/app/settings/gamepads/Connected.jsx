// Route : /settings/gamepads/connected — les manettes connectées à cet appareil.
import React from "react";
import { useTranslation } from "react-i18next";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Section } from "../../bricks/Section.jsx";
import { useDevicePads } from "../../engine/libs/gamepadPresence.js";
import { fact } from "../../libs/format.js";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";

export const CONNECTED = "/settings/gamepads/connected";

// L'état d'une manette, en un mot : son numéro (« Pad 2 »), ou désactivée.
export const padState = (entry, t) => (entry.enabled ? t("play.pads.pad", { number: entry.number }) : t("play.pads.off"));

// LES MANETTES CONNECTÉES (chantier 84 ; le porteur, 2026-11-26 : « une nouvelle page qui liste les manettes connectées
// pour après faire l'activation ou désactivation, le mapping et aussi changer l'index ») : une ligne par manette branchée
// sur cet appareil, son nom et son numéro ; elle ouvre sa page. Les manettes activées prennent les sièges d'une partie
// dans l'ordre de leurs numéros ; une désactivée n'en prend jamais.
export default function ConnectedPads() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const pads = useDevicePads();

    return (
        <SettingsPage title={t("gamepads.connected.title")} parent="/settings/gamepads">
            <Section title={t("gamepads.connected.title")} help={t("gamepads.connected.help")} sense="modify">
                <Listing items={pads} empty={t("play.pads.none")}
                    render={(entry) => (
                        <NavRow key={entry.key} id={`gp-pad-${entry.index}`} icon="bi-controller" label={entry.name}
                            help={fact(t("facts.state"), padState(entry, t))}
                            onOpen={() => open(`${CONNECTED}/${encodeURIComponent(entry.key)}`, `gp-pad-${entry.index}`)} />
                    )} />
            </Section>
            <GuideSection guides={[["controllers", "a-real-controller"]]} />
        </SettingsPage>
    );
}
