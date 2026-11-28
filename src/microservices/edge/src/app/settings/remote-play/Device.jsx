// Route : /settings/remote-play/<id> — une console.
import React from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Note } from "../../bricks/Note.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { deleteDevice } from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../SettingsPage.jsx";
import { backgroundConsole, deviceName, RemoteBackgroundImage, ROOT, useDevices, useRemoteBackground, useStatus } from "../RemotePlay.jsx";

// UNE CONSOLE (DeviceDetailPanel) : ses détails, « Plus » — son image de fond —, puis la dissocier (« Danger ») —
// l'appairage oublié, il faudra la réenregistrer pour s'y connecter. Ni le flux ni le réveil (plus haut).
function DeviceView({ device }) {
    const { t } = useTranslation();
    const { back, open } = useSettingsNavigation();
    const background = useRemoteBackground(device);
    const { refreshDevices } = useData();
    const status = useStatus(device);

    const unlink = async () => {
        try {
            const result = await deleteDevice(device.id);

            if (result.ok) {
                refreshDevices();
                back(ROOT);

                return;
            }
            toast(result.errorMessage || t("rp.unlinkFailed"));
        } catch (failure) {
            console.error("échec de la dissociation de l'appareil", failure);
            toast(t("rp.unlinkFailed"));
        }
    };

    return (
        <SettingsPage title={deviceName(device)} parent={ROOT} guides={[["remotePlay"]]}>
            <Section title={t("details")} help={t("remoteplay.device.detailsHelp")} sense="info">
                <Rows>
                    <InfoRow icon={status.icon} tone={status.tone} label={t("remoteplay.device.status")} value={status.label} />
                    <InfoRow icon="bi-playstation" label={t("remoteplay.device.type")} value={device.hostType} />
                    <InfoRow icon="bi-router" label={t("remoteplay.device.ip")} value={device.ipAddress} />
                </Rows>
            </Section>
            <Section title={t("more")} sense="modify">
                <Rows>
                    <NavRow id="rp-background" lead={<RemoteBackgroundImage consoleId={backgroundConsole(device)} name={background?.name} server={background?.server} width={256} thumb />} label={t("rp.background")}
                        help={t("rp.backgroundHelp", { console: device.hostType })} onOpen={() => open(`${ROOT}/${encodeURIComponent(device.id)}/background`, "rp-background", { device })} />
                </Rows>
            </Section>
            <DangerSection id="rp-unlink" label={t("rp.unlink")}
                question={t("rp.unlinkConfirm", { name: deviceName(device) })} confirmLabel={t("rp.unlinkAction")} onConfirm={unlink} />
        </SettingsPage>
    );
}

// La page d'une console : celle que la liste a passée le temps de relire la liste (un lien direct, un rechargement),
// puis celle de la liste, tenue à jour. `View` : la console elle-même, ou son image de fond.
export function DevicePage({ View = DeviceView }) {
    const { t } = useTranslation();
    const { deviceId } = useParams();
    const location = useLocation();
    const devices = useDevices();
    const passed = location.state?.device?.id === deviceId ? location.state.device : null;
    const device = Array.isArray(devices) ? devices.find((entry) => entry.id === deviceId) : passed;

    if (device) {
        return <View device={device} />;
    }

    return (
        <SettingsPage title={t("settings.items.remotePlay.label")} parent={ROOT} guides={[["remotePlay"]]}>
            {devices === null ? <Loading /> : devices === false ? <Note>{t("remoteplay.loadFailed")}</Note> : <NotFound />}
        </SettingsPage>
    );
}

export default function Device() {
    return <DevicePage />;
}
