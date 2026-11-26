// Route : /desktop/disks — LES DISQUES, COMME L'APK (DisksPage) : d'autres dossiers de jeux de ce PC, rangés comme la
// bibliothèque (disks.go) ; une ligne par disque, qui ouvre sa page ; puis « Plus » : Ajouter.
import React from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { fact, formatSize } from "../../libs/format.js";
import { EmptyRow } from "../../bricks/EmptyRow.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { DesktopPage, useDesktopState } from "./shared.jsx";

export const DISK_GUIDES = [["storage", "more-disks-of-games"]];

const diskHelp = (t, disk) => (disk.present ? [fact(t("desktop.diskFolder"), disk.root), fact(t("desktop.space"), disk.totalBytes ? t("desktop.spaceValue",
    { free: formatSize(disk.freeBytes), total: formatSize(disk.totalBytes) }) : null)] : [fact(t("desktop.diskFolder"), disk.root), t("desktop.diskAbsent")]);

export default function Disks() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const { state } = useDesktopState();
    const disks = state.disks || [];

    return (
        <DesktopPage title={t("desktop.disks")} guides={DISK_GUIDES}>
            <Section title={t("desktop.disksTitle")} help={t("desktop.disksHelp")} sense="modify">
                <Rows>
                    {disks.length === 0 ? <EmptyRow icon="bi-device-hdd" text={t("desktop.disksNone")} />
                        : disks.map((disk) => (
                            <NavRow key={disk.name} id={`dc-disk-${disk.name}`} icon={disk.present ? "bi-device-hdd" : "bi-exclamation-triangle"}
                                label={disk.name} help={diskHelp(t, disk)} onOpen={() => navigate(`/desktop/disks/${encodeURIComponent(disk.name)}`)} />
                        ))}
                </Rows>
            </Section>
            <Section title={t("desktop.more")} help={t("desktop.disksMoreHelp")} sense="add">
                <Rows>
                    <NavRow id="dc-disk-add" icon="bi-plus-lg" label={t("desktop.diskAdd")} help={t("desktop.diskAddHelp")} onOpen={() => navigate("/desktop/disks/add")} />
                </Rows>
            </Section>
        </DesktopPage>
    );
}
