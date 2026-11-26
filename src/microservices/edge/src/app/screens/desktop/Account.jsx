// Route : /desktop/account — LE COMPTE ROOT, COMME L'APK (AccountPage) : les comptes, puis « Réinitialiser le mot de
// passe root » — un mot de passe tiré au sort, montré une fois ; il ne sert qu'à créer le premier administrateur.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { CopyField } from "../../bricks/CopyField.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Note } from "../../bricks/Note.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { useConfirm } from "../../bricks/useConfirm.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { call, DesktopPage, useDesktopState } from "./shared.jsx";

export default function Account() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const confirm = useConfirm();
    const { state } = useDesktopState();
    const [password, setPassword] = useState("");
    const accounts = state.accounts || {};

    return (
        <DesktopPage title={t("desktop.rootAccount")} guides={[["install"]]}>
            <Section title={t("desktop.accounts")} help={t("desktop.accountsHelp")} sense="info">
                <Rows>
                    <InfoRow icon="bi-person-badge" label={t("desktop.root")} value={accounts.root || "root"} />
                    <InfoRow icon="bi-shield-lock" label={t("desktop.admins")} value={String(accounts.admins ?? 0)} />
                    <InfoRow icon="bi-people" label={t("desktop.players")} value={String(accounts.players ?? 0)} />
                </Rows>
                {(accounts.admins ?? 0) === 0 && <Note>{t("desktop.firstUse")}</Note>}
            </Section>
            <Section title={t("desktop.rootPassword")} help={t("desktop.rootPasswordHelp")} sense="modify">
                <Rows>
                    {password ? <CopyField id="dc-root-password" label={t("desktop.newPassword")} help={t("desktop.newPasswordHelp")} value={password} />
                        : <NavRow id="dc-root-reset" icon="bi-key" label={t("desktop.resetRoot")} help={t("desktop.resetRootHelp")} opens="confirm"
                            onOpen={() => confirm({ title: t("desktop.resetRoot"), message: t("desktop.resetRootConfirm"), onConfirm: async () => {
                                const answer = await call("/root-password", "POST", {});

                                if (answer.ok) {
                                    setPassword(answer.password);
                                } else {
                                    feedback.failed(t("settings.actionFailed"));
                                }
                            } })} />}
                </Rows>
            </Section>
        </DesktopPage>
    );
}
