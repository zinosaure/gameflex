// Route : /settings/cast/application — l'identifiant du récepteur.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { Loading } from "../../bricks/Loading.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { saveCastSettings } from "../../libs/api.js";
import { Form, FormButton, formContext } from "../../ui/Forms.jsx";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { CAST, useCastApplication } from "../Cast.jsx";

export default function CastApplication() {
    const { t } = useTranslation();
    const { back } = useSettingsNavigation();
    const applicationId = useCastApplication();

    return (
        <SettingsPage title={t("settings.cast.applicationId")} parent={CAST} guides={[["remoteAccess"]]}>
            {applicationId === undefined ? <Section><Loading /></Section> : <CastApplicationForm stored={applicationId} onSaved={() => back(CAST)} />}
        </SettingsPage>
    );
}

// Rien de changé, rien à enregistrer (`isReplaced`) ; un identifiant refusé se dit, le champ reste.
function CastApplicationForm({ stored, onSaved }) {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const [values, setValues] = useState({ applicationId: stored });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues);
    const typed = values.applicationId.trim().toUpperCase();
    const isReplaced = typed !== "" && typed !== stored;

    return (
        <Form props={{ onSubmit: async (event) => {
            event.preventDefault();

            if (!isReplaced || pending) {
                return;
            }
            setPending(true);
            const answer = await saveCastSettings(typed);
            setPending(false);

            if (answer.ok) {
                feedback.success(t("settings.cast.saved"));
                onSaved();
            } else {
                feedback.failed(t(answer.status === 400 ? "settings.cast.invalid" : "settings.saveFailed"));
            }
        } }}>
            <Section title={t("settings.cast.applicationId")} help={t("settings.cast.applicationIdHelp")} sense="modify">
                <Rows>
                    <TextRow id="applicationId" context={form} label={t("settings.cast.applicationId")}
                        props={{ autoComplete: "off", placeholder: "CC1AD845", maxLength: 8, "data-key": "cast-application-id" }} />
                    <SubmitRow>
                        <FormButton id="cast-save" label={t("settings.save")} isReplaced={isReplaced} pending={pending} />
                    </SubmitRow>
                </Rows>
            </Section>
        </Form>
    );
}
