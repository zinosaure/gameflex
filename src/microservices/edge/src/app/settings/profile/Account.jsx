// Route : /settings/profile/account — mes informations personnelles (le porteur avait posé la ligne vers elle, sans la
// page, sur sa branche ui/porteur-2026-11-24) : mon identifiant, qui ne change pas — désactivé, avec sa raison, jamais
// caché —, et mon nom affiché, de 3 à 25 caractères comme la webapp.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { updateMe } from "../../libs/api.js";
import { useAuth } from "../../libs/data.jsx";
import { Form, formContext, FormButton, Ruleset } from "../../ui/Forms.jsx";
import SettingsPage, { PROFILE } from "../SettingsPage.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";

export default function Account() {
    const { t } = useTranslation();
    const { user, refreshMe } = useAuth();
    const feedback = useFeedback();
    const [values, setValues] = useState({ username: user?.username || "", display_name: user?.display_name || "" });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues, {
        username: () => Ruleset.isDisabled(t("profile.usernameFixed")),
        display_name: (data) => {
            const name = (data.display_name || "").trim();

            if (name.length < 3) {
                return Ruleset.isInvalid(t("settings.profileMin"));
            }

            return name.length > 25 ? Ruleset.isInvalid(t("settings.profileMax")) : null;
        },
    });
    // Rien de changé n'est pas une erreur (CODE_CONVENTIONS.md, `isReplaced`).
    const isReplaced = (values.display_name || "").trim() !== (user?.display_name || "");
    const invalid = Boolean(form.rulesets.display_name(values));

    const save = async () => {
        setPending(true);
        const result = await updateMe({ display_name: values.display_name.trim() });
        setPending(false);

        if (result.ok) {
            await refreshMe();
            feedback.success(t("settings.saved"));
        } else {
            feedback.failed(result.detail || t("settings.saveFailed"));
        }
    };

    return (
        <SettingsPage title={t("settings.items.account.label")} parent={PROFILE}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (isReplaced && !invalid && !pending) {
                    save();
                }
            } }}>
                <Section title={t("profile.personal")} help={t("profile.accountHelp")} sense="modify">
                    <Rows>
                        <TextRow id="username" label={t("settings.username")} help={t("settings.usernameHelp")} context={form} />
                        <TextRow id="display_name" label={t("settings.displayName")} help={t("settings.displayNameHelp")} context={form} props={{ "data-key": "pr-display-name" }} />
                        <SubmitRow>
                            <FormButton id="pr-save" label={t("settings.save")} isReplaced={isReplaced} pending={pending} disabled={invalid} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
            <GuideSection guides={[["install", "6-sign-in"], ["remoteAccess", "keep-it-safe"]]} />
        </SettingsPage>
    );
}
