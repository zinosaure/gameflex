// Route : /settings/gamepads/connected/<manette> — une manette connectée à cet appareil.
import React from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import NavRow from "../../../bricks/NavRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { SaveSection } from "../../../bricks/SaveSection.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { SelectRow } from "../../../bricks/SelectRow.jsx";
import { SwitchRow } from "../../../bricks/SwitchRow.jsx";
import { useDevicePads } from "../../../engine/libs/gamepadPresence.js";
import { getPadPreferences, setPadEnabled, setPadOrder } from "../../../engine/libs/gamepadPreferences.js";
import { gamepadDeviceId } from "../../../libs/keyMapping.js";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { CONNECTED } from "../Connected.jsx";

// LE NUMÉRO DE CETTE MANETTE DEVIENT `number` : l'ordre des manettes activées, cette manette déplacée ; les manettes
// connues mais débranchées gardent leur place à la suite.
function moveTo(pads, key, number) {
    const enabled = pads.filter((entry) => entry.enabled && entry.key !== key).map((entry) => entry.key);
    enabled.splice(number - 1, 0, key);
    const known = getPadPreferences().padOrder.filter((one) => !enabled.includes(one));

    return [...enabled, ...known];
}

// UNE MANETTE DE CET APPAREIL (chantier 84) : activée ou non, son numéro — l'ordre dans lequel les manettes prennent les
// sièges d'une partie —, par le bouton de la section ; puis la correspondance de ses touches, sur la page Manettes.
export default function ConnectedPad() {
    const { t } = useTranslation();
    const { key } = useParams();
    const { open } = useSettingsNavigation();
    const pads = useDevicePads();
    const pad = pads.find((entry) => entry.key === key);

    if (!pad) {
        return <SettingsPage title={t("gamepads.connected.title")} parent={CONNECTED}><NotFound /></SettingsPage>;
    }
    const count = pads.filter((entry) => entry.enabled || entry.key === key).length;

    return (
        <SettingsPage title={pad.name} parent={CONNECTED}>
            <SaveSection id="gp-pad" title={t("gamepads.connected.padTitle")} help={t("gamepads.connected.padHelp")}
                saved={{ enabled: pad.enabled, number: String(pad.number || count) }}
                onSave={(values) => {
                    setPadEnabled(key, values.enabled);
                    if (values.enabled) {
                        setPadOrder(moveTo(pads, key, Number(values.number)));
                    }

                    return { ok: true };
                }}>
                {(values, set) => (
                    <>
                        <SwitchRow id="gp-pad-enabled" label={t("gamepads.connected.enabled")} texts="enabled" help={t("gamepads.connected.enabledHelp")}
                            checked={values.enabled} onChange={(enabled) => set({ enabled })} />
                        <SelectRow id="gp-pad-number" label={t("gamepads.connected.number")} help={t("gamepads.connected.numberHelp")} value={values.number}
                            disabled={!values.enabled} onChange={(number) => set({ number })}
                            options={Array.from({ length: count }, (_, at) => ({ value: String(at + 1), label: t("play.pads.pad", { number: at + 1 }) }))} />
                    </>
                )}
            </SaveSection>
            <Section title={t("gamepads.connected.mappingTitle")} help={t("gamepads.connected.mappingHelp")} sense="modify">
                <Rows>
                    <NavRow id="gp-pad-mapping" icon="bi-keyboard" label={t("gamepads.connected.mapping")} help={t("gamepads.connected.mappingRowHelp")}
                        onOpen={() => open(`/settings/gamepads?device=${encodeURIComponent(gamepadDeviceId(pad.gamepad))}`, "gp-pad-mapping")} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}
