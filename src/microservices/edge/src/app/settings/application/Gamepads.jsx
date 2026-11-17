import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes } from "react-router-dom";
import { CONTROLLER_PRESETS } from "../../engine/controllers/presets.js";
import { setHapticFeedback, setRumbleEnabled, useGamepadPreferences } from "../../engine/libs/gamepadPreferences.js";
import { focusElement, suspendPads } from "../../libs/input.js";
import {
    applyAutoMapping, captureGamepadChord, captureKeyboardChord, connectedGamepads, deleteKeyMapping, gamepadDeviceId, keyboardDeviceId, loadKeyMapping, remapKey,
} from "../../libs/keyMapping.js";
import { LayoutPage } from "./Layout.jsx";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { useSearchParam, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { DangerSection, InvisibleSticksRow, GuideSection, IconAddon, InfoRow, Rows, SelectRow, Section, SwitchRow } from "../../ui/Bricks.jsx";

// LES MANETTES (profile/settings/Gamepad.jsx de la webapp ; 10-manettes des modèles) : la manette virtuelle ;
// les retours tactiles et le rumble de cet appareil ; et la correspondance des touches du clavier ou d'une
// manette physique vers elle — gardée sur le compte, elle vaut pour toutes les parties. Une ligne par
// bouton virtuel : A dessus écoute un accord (les touches ou boutons enfoncés ensemble), qui
// s'enregistre au relâchement ; la toucher de nouveau abandonne l'écoute. « Auto », l'icône accolée au choix
// de la manette, applique la correspondance par défaut d'une manette à toutes les manettes virtuelles ; on
// réinitialise celle de l'appareil choisi, en bas de la page, dans « Danger ». Chaque groupe a son titre (le
// porteur, 2026-11-13 : « chaque section doit toujours avoir un titre si possible »). Les réglages de l'appareil
// (retours tactiles, rumble, stick flottant) sont ceux de ce navigateur, dans la seule source que lisent aussi le menu
// d'une partie et la manette à l'écran (engine/libs/gamepadPreferences.js, #758).
//
// La DISPOSITION de la manette à l'écran (ButtonPositioning) s'édite sur sa propre page (Layout.jsx),
// sur la manette tactile des parties elle-même.
//
// Une entrée des Réglages à elle, pour tout compte (le porteur, 2026-11-13 : « ce n'est pas un réglage de
// l'application »).
//
//   /settings/gamepads                   la manette virtuelle (`?controller=`), cet appareil, la correspondance
//   /settings/gamepads/layout/<manette>  la disposition de la manette à l'écran

const DEFAULT_CONTROLLER = "dualshock";

// Les manettes branchées, suivies (branchement, débranchement).
function useConnectedPads() {
    const [pads, setPads] = useState(connectedGamepads);

    useEffect(() => {
        const update = () => setPads(connectedGamepads());
        window.addEventListener("gamepadconnected", update);
        window.addEventListener("gamepaddisconnected", update);
        const timer = setInterval(update, 1500);

        return () => {
            window.removeEventListener("gamepadconnected", update);
            window.removeEventListener("gamepaddisconnected", update);
            clearInterval(timer);
        };
    }, []);

    return pads;
}

// Une ligne : le bouton virtuel, et l'entrée qui le déclenche — A l'écoute, et la ligne touchée de nouveau
// l'abandonne (pas de bouton dans une ligne) ; l'écoute s'abandonne aussi seule, sans accord, au bout de 8 s.
function CaptureRow({ name, input, device, listening, onListen, onCaptured, onCancel }) {
    const { t } = useTranslation();
    const [chord, setChord] = useState("");

    useEffect(() => {
        if (!listening) {
            return undefined;
        }
        setChord("");
        suspendPads(true);
        const capture = device.keyboard ? captureKeyboardChord : (handlers) => captureGamepadChord(device.pad.index, handlers);
        const stop = capture({
            onChange: setChord,
            onDone: (done) => {
                suspendPads(false);

                if (done) {
                    onCaptured(done);
                } else {
                    onCancel();
                }
            },
        });

        return () => {
            stop();
            suspendPads(false);
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [listening, device?.id]);

    // L'entrée en pastille (ou l'accord qu'on écoute) ; aucune : un tiret.
    const shown = listening ? (chord ? chord.replace(/\+/g, " + ") : t("gamepads.listening")) : input;

    return (
        <NavRow id={`gp-${name}`} icon="bi-record-circle" tone="violet" label={name.replace(/_/g, " ")} disabled={!device}
            value={shown ? <span className="badge text-bg-primary font-monospace">{shown}</span> : "—"} opens="action" onOpen={listening ? onCancel : onListen} />
    );
}

function GamepadsPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const pads = useConnectedPads();
    const [controllerParam, setControllerParam] = useSearchParam("controller", DEFAULT_CONTROLLER);
    const controllerName = CONTROLLER_PRESETS[controllerParam] ? controllerParam : DEFAULT_CONTROLLER;
    const [deviceId, setDeviceId] = useState(null);
    const [mapping, setMapping] = useState({});
    const [listening, setListening] = useState(null);
    const [auto, setAuto] = useState(false);
    const { hapticFeedback: haptic, rumble } = useGamepadPreferences();
    // Le clavier d'abord, là où il y en a un (pas au doigt), puis les manettes branchées.
    const devices = [
        ...(matchMedia("(pointer: coarse)").matches ? [] : [{ id: keyboardDeviceId(), keyboard: true, standard: true }]),
        ...pads.map((pad) => ({ id: gamepadDeviceId(pad), pad, standard: pad.mapping === "standard" })),
    ];
    const device = devices.find((entry) => entry.id === deviceId) || devices[0] || null;
    const preset = CONTROLLER_PRESETS[controllerName];
    const buttonNames = [...new Set((preset?.buttons || []).map((button) => button.name))].filter((name) => name !== "LEFT_STICK" && name !== "RIGHT_STICK");

    useEffect(() => {
        if (!device) {
            setMapping({});

            return undefined;
        }
        let cancelled = false;
        loadKeyMapping(device.id, controllerName).then((loaded) => !cancelled && setMapping(loaded));

        return () => {
            cancelled = true;
        };
    }, [device?.id, controllerName]);

    // L'écoute finie (un accord, ou abandonnée), la lumière revient sur la ligne.
    const stopListening = (name) => {
        setListening(null);
        requestAnimationFrame(() => focusElement(document.querySelector(`[data-key="gp-${CSS.escape(name)}"]`)));
    };

    const captured = async (name, input) => {
        stopListening(name);
        const { mapping: next, saved } = remapKey(mapping, device.id, device.keyboard ? "keyboard" : "gamepad", controllerName, name, input);
        setMapping(next);
        const answer = await saved;

        if (!answer.ok) {
            toast(answer.detail || t("gamepads.saveFailed"));
        }
    };

    const autoMap = async () => {
        setAuto(true);
        const applied = await applyAutoMapping(device.id, "gamepad");
        setAuto(false);

        if (!applied) {
            toast(t("gamepads.saveFailed"));

            return;
        }
        setMapping(await loadKeyMapping(device.id, controllerName));
        toast(t("gamepads.autoMappingDone"));
    };

    const label = (entry) => (entry.keyboard ? t("gamepads.keyboard") : entry.standard ? entry.id : `${entry.id} — ${t("gamepads.nonStandardLayout")}`);
    const rumbleUnsupported = preset?.rumble === false;

    return (
        <SettingsPage title={t("settings.items.gamepads.label")}>
            <Section title={t("gamepads.virtualController")}>
                <Rows>
                    <SelectRow id="gp-controller" label={t("gamepads.virtualController")} help={t("gamepads.virtualControllerHelp")} value={controllerName} onChange={setControllerParam}
                        options={Object.entries(CONTROLLER_PRESETS).map(([key, entry]) => ({ value: key, label: entry.label || key }))} />
                </Rows>
            </Section>
            <Section title={t("gamepads.deviceSettings")} help={t("gamepads.deviceSettingsHelp")}>
                <Rows>
                    <SwitchRow id="gp-haptic" label={t("gamepads.hapticFeedback")} help={t("gamepads.hapticFeedbackHelp")} checked={haptic} onChange={setHapticFeedback} />
                    <SwitchRow id="gp-rumble" label={t("gamepads.rumble")} help={rumbleUnsupported ? t("gamepads.rumbleUnsupported") : t("gamepads.rumbleHelp")}
                        checked={rumble && !rumbleUnsupported} disabled={rumbleUnsupported} onChange={setRumbleEnabled} />
                    <InvisibleSticksRow id="gp-invisible-sticks" />
                </Rows>
            </Section>
            <Section title={t("gamepads.layoutTitle")}>
                <Rows>
                    <NavRow id="gp-layout" icon="bi-arrows-move" tone="violet" label={t("gamepads.editLayout")} help={t("gamepads.editLayoutHelp")}
                        onOpen={() => open(`/settings/gamepads/layout/${encodeURIComponent(controllerName)}`, "gp-layout")} />
                </Rows>
            </Section>
            <Section title={device ? t("gamepads.mappingTitle", { device: label(device), controller: preset?.label || controllerName }) : t("gamepads.noDeviceTitle")} help={t("gamepads.mappingDescription")}>
                {/* SANS MANETTE, la ligne de l'appareil le dit, à la place de son choix : un renseignement (InfoRow), plus une
                    alerte (le porteur, 2026-11-16 : « retirer les alerts »). */}
                {!device && (
                    <Rows className="mb-3">
                        <InfoRow icon="bi-controller" label={t("gamepads.device")} value={t("gamepads.noDeviceValue")} help={t("gamepads.noDevice")} />
                    </Rows>
                )}
                {/* Le choix de l'appareil est un formulaire, les touches des liens : deux groupes, les touches gardent leurs
                    traits (Bricks.jsx, Rows). */}
                {device && (
                    <Rows className="mb-3">
                        <SelectRow id="gp-device" label={t("gamepads.device")} help={t("gamepads.deviceHelp")} value={device.id} onChange={setDeviceId}
                            options={devices.map((entry) => ({ value: entry.id, label: label(entry) }))}
                            addon={<IconAddon id="gp-auto" icon="bi-magic" label={t("gamepads.autoMapping")} disabled={auto || device.keyboard} onClick={autoMap} />} />
                    </Rows>
                )}
                <Rows>
                    {buttonNames.map((name) => (
                        <CaptureRow key={`${device?.id || "none"}:${controllerName}:${name}`} name={name} device={device} listening={listening === name}
                            input={Object.entries(mapping).find(([, value]) => value === name)?.[0]}
                            onListen={() => setListening(name)} onCancel={() => stopListening(name)} onCaptured={(input) => captured(name, input)} />
                    ))}
                </Rows>
            </Section>
            <DangerSection id="gp-reset" icon="bi-arrow-counterclockwise" label={t("gamepads.reset")} help={device ? t("gamepads.resetHelp") : t("gamepads.noDevice")} question={t("gamepads.resetConfirm")}
                disabled={!device} onConfirm={async () => {
                    setMapping({});
                    const answer = await deleteKeyMapping(device.id, controllerName);
                    toast(answer.ok ? t("gamepads.resetDone") : answer.detail || t("gamepads.saveFailed"));
                }} />
            <GuideSection guides={[["controllers", "a-real-controller"]]} />
        </SettingsPage>
    );
}

// /settings/gamepads/* (Settings.jsx) : une entrée des Réglages à elle, pour tout compte — « ce n'est pas un
// réglage de l'application » (le porteur, 2026-11-13).
export function GamepadsRoutes() {
    return (
        <Routes>
            <Route index element={<GamepadsPage />} />
            <Route path="layout/:controller" element={<LayoutPage />} />
            <Route path="*" element={<Navigate to="/settings/gamepads" replace />} />
        </Routes>
    );
}
