import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { changePassword } from "../../libs/api.js";
import { passwordAccepted, refusalText } from "../../libs/password.js";
import { toast } from "../../shell/Shell.jsx";
import { Form, formContext, FormButton, Ruleset } from "../../ui/Forms.jsx";
import SettingsPage, { PROFILE, usePageText } from "../SettingsPage.jsx";
import { Rows, Section, SubmitRow, TextRow } from "../../ui/Bricks.jsx";

// /settings/profile/password — MON MOT DE PASSE : l'actuel, le nouveau, sa confirmation. Le nouveau suit LA RÈGLE d'un
// mot de passe choisi (chantier 55 ; le porteur : « ça reste, et pareil comme pour l'initialisation du mot de passe, il
// faut que ça soit sur les mêmes recommandations ») : le champ et la jauge de Skeleton (TextRow `new-password`), la règle
// de libs/password.js, que le service des comptes applique aussi. Changé, il déconnecte les AUTRES appareils du compte :
// celui-ci reçoit un jeton neuf (changePassword) et reste connecté.
export function PasswordPage() {
    const { t } = useTranslation();
    const page = usePageText("password");
    const [values, setValues] = useState({ current: "", next: "", confirm: "" });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues, {
        confirm: (data) => (data.confirm && data.confirm !== data.next ? Ruleset.isInvalid(t("settings.passwordMismatch")) : null),
    });
    const ready = values.current && passwordAccepted(values.next) && values.confirm === values.next;
    // Rien de tapé, rien de changé (CODE_CONVENTIONS.md, `isReplaced`).
    const isReplaced = Boolean(values.current || values.next || values.confirm);

    const save = async () => {
        setPending(true);
        const result = await changePassword(values.current, values.next);
        setPending(false);

        if (result.ok) {
            setValues({ current: "", next: "", confirm: "" });
            toast(t("settings.passwordChanged"));
        } else {
            toast(refusalText(t, result) || t("settings.saveFailed"));
        }
    };

    return (
        <SettingsPage title={page.title} parent={PROFILE}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (ready && !pending) {
                    save();
                }
            } }}>
                <Section title={t("settings.passwordFormTitle")} help={t("settings.passwordFormHelp")}>
                    <Rows>
                        <TextRow id="current" kind="password" label={t("settings.currentPassword")} help={t("settings.currentPasswordHelp")} context={form} props={{ autoComplete: "current-password", "data-key": "pw-current" }} />
                        <TextRow id="next" kind="new-password" label={t("settings.newPassword")} help={t("settings.newPasswordHelp")} context={form} props={{ autoComplete: "new-password", "data-key": "pw-next" }} />
                        <TextRow id="confirm" kind="password" label={t("settings.confirmPassword")} help={t("settings.confirmPasswordHelp")} context={form} props={{ autoComplete: "new-password", "data-key": "pw-confirm" }} />
                        <SubmitRow>
                            <FormButton id="sc-password" label={t("settings.save")} isReplaced={isReplaced} pending={pending} disabled={!ready} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
        </SettingsPage>
    );
}
