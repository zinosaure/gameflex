// Route : /desktop/disks/add — AJOUTER UN DISQUE : son nom et son dossier, vérifiés par le lanceur (disks.go).
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { SubmitRow } from "../../../bricks/SubmitRow.jsx";
import { TextRow } from "../../../bricks/TextRow.jsx";
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import { Form, formContext, FormButton } from "../../../ui/Forms.jsx";
import { DISK_GUIDES } from "../Disks.jsx";
import { call, DesktopPage, useDesktopState } from "../shared.jsx";

const DISK_ERRORS = ["disk_name_refused", "disk_not_absolute", "disk_folder_unreadable", "disk_not_a_folder", "disk_name_taken", "disk_already_declared",
    "disk_overlaps", "disk_overlaps_library", "disk_brings_nothing"];

export default function DiskAdd() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const navigate = useNavigate();
    const { setState } = useDesktopState();
    const [values, setValues] = useState({ name: "", root: "" });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues);
    const ready = Boolean(values.name.trim() && values.root.trim());

    return (
        <DesktopPage title={t("desktop.diskAdd")} parent="/desktop/disks" guides={DISK_GUIDES}>
            <Form props={{ onSubmit: async (event) => {
                event.preventDefault();

                if (!ready || pending) {
                    return;
                }
                setPending(true);
                const answer = await call("/disks", "POST", { name: values.name.trim(), root: values.root.trim() });
                setPending(false);

                if (answer.ok) {
                    setState(answer);
                    feedback.success(t("desktop.diskAdded", { name: values.name.trim() }));
                    navigate("/desktop/disks");
                } else {
                    feedback.failed(DISK_ERRORS.includes(answer.code) ? t(`desktop.errors.${answer.code}`, { detail: answer.detail || "" }) : t("settings.actionFailed"));
                }
            } }}>
                <Section title={t("desktop.diskAdd")} help={t("desktop.diskAddText")} sense="add">
                    <Rows>
                        <TextRow id="name" label={t("desktop.diskName")} help={t("desktop.diskNameHelp")} context={form} props={{ autoComplete: "off", placeholder: "games", "data-key": "dc-disk-name" }} />
                        <TextRow id="root" label={t("desktop.diskFolder")} help={t("desktop.diskFolderHelp")} context={form}
                            props={{ autoComplete: "off", placeholder: "/media/games", "data-key": "dc-disk-root" }} />
                        <SubmitRow>
                            <FormButton id="dc-disk-save" label={t("desktop.diskAddConfirm")} isReplaced={ready} pending={pending} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
        </DesktopPage>
    );
}
