// Route : /desktop/disks/:name — UN DISQUE, COMME L'APK (DiskPage) : son dossier, sa présence, sa place ; Retirer, en
// « Danger ».
import React from "react";
import { useTranslation } from "react-i18next";
import { useNavigate, useParams } from "react-router-dom";
import { formatSize } from "../../../libs/format.js";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { EmptyRow } from "../../../bricks/EmptyRow.jsx";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import { DISK_GUIDES } from "../Disks.jsx";
import { call, DesktopPage, useDesktopState } from "../shared.jsx";

export default function Disk() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const navigate = useNavigate();
    const { state, setState } = useDesktopState();
    const name = decodeURIComponent(useParams().name || "");
    const disk = (state.disks || []).find((candidate) => candidate.name === name);

    if (!disk) {
        return (
            <DesktopPage title={t("desktop.disks")} parent="/desktop/disks" guides={DISK_GUIDES}>
                <Section title={t("desktop.details")} help={t("desktop.diskDetailsHelp")} sense="info"><EmptyRow icon="bi-device-hdd" text={t("desktop.disksNone")} /></Section>
            </DesktopPage>
        );
    }

    return (
        <DesktopPage title={disk.name} parent="/desktop/disks" guides={DISK_GUIDES}>
            <Section title={t("desktop.details")} help={t("desktop.diskDetailsHelp")} sense="info">
                <Rows>
                    <InfoRow icon="bi-folder" label={t("desktop.diskFolder")} value={disk.root} mono />
                    <InfoRow icon={disk.present ? "bi-plug" : "bi-exclamation-triangle"} label={t("desktop.diskState")}
                        value={t(disk.present ? "desktop.diskPresent" : "desktop.diskAbsent")} />
                    {disk.present && <InfoRow icon="bi-pie-chart" label={t("desktop.space")}
                        value={disk.totalBytes ? t("desktop.spaceValue", { free: formatSize(disk.freeBytes), total: formatSize(disk.totalBytes) }) : t("desktop.spaceUnreadable")} />}
                    {disk.present && <InfoRow icon="bi-pencil" label={t("desktop.diskAccess")} value={t(disk.writable ? "desktop.diskReadWrite" : "desktop.diskReadOnly")} />}
                </Rows>
            </Section>
            <DangerSection id="dc-disk-remove" label={t("desktop.diskRemove")} help={t("desktop.diskRemoveHelp")} question={t("desktop.diskRemoveConfirm")}
                confirmLabel={t("desktop.remove")} onConfirm={async () => {
                    const answer = await call(`/disks/${encodeURIComponent(disk.name)}`, "DELETE");

                    if (answer.ok) {
                        setState(answer);
                        navigate("/desktop/disks");
                    } else {
                        feedback.failed(t("settings.actionFailed"));
                    }
                }} />
        </DesktopPage>
    );
}
