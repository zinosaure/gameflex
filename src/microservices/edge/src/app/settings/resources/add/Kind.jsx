// Route : /settings/resources/add/<genre> — la connexion d'un genre.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, useNavigate, useParams } from "react-router-dom";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { SubmitRow } from "../../../bricks/SubmitRow.jsx";
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import { addResource } from "../../../libs/api.js";
import { FormButton, Ruleset } from "../../../ui/Forms.jsx";
import SettingsPage from "../../SettingsPage.jsx";
import { emptyResource, KINDS, ResourceFields, resourceForm, resourceReady, sentValues } from "../../people/externalResources.jsx";
import { EXTERNAL_GUIDES } from "../../Resources.jsx";

// LE FORMULAIRE D'UN GENRE : celui de gameflex-federation (externalResources.jsx), anonyme ou avec un compte. Enregistrée,
// la ressource se lit en fond, et sa page s'ouvre.
export default function AddExternalKind() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { kind } = useParams();
    const navigate = useNavigate();
    const [values, setValues] = useState(() => emptyResource(kind));
    const [pending, setPending] = useState(false);
    const form = resourceForm(values, setValues, kind, t);

    if (!KINDS.some((entry) => entry.kind === kind)) {
        return <Navigate to="/settings/resources/add" replace />;
    }

    return (
        <SettingsPage title={t(`fedService.kinds.${kind}.add`)} parent="/settings/resources/add" levels={{ "/settings/resources/add": t("resources.external.add") }}>
            <Section help={t(`fedService.kinds.${kind}.intro`)} sense="modify">
                <form onSubmit={async (event) => {
                    event.preventDefault();
                    if (!resourceReady(values, kind) || Ruleset.hasFeedbacks(form)) {
                        return;
                    }
                    setPending(true);
                    const answer = await addResource({ ...sentValues(values, kind), kind });
                    setPending(false);
                    if (!answer.ok) {
                        feedback.failed(answer.errorMessage || t("fedService.failed"));

                        return;
                    }
                    feedback.success(t("fedService.connections.added"));
                    navigate(`/settings/resources/external/${answer.data.id}`, { replace: true });
                }}>
                    <Rows>
                        <ResourceFields kind={kind} form={form} values={values} setValues={setValues} />
                        <SubmitRow><FormButton id="rx-add" label={t("fedService.connections.add")} isReplaced={resourceReady(values, kind)} disabled={Boolean(Ruleset.hasFeedbacks(form))} pending={pending} /></SubmitRow>
                    </Rows>
                </form>
            </Section>
            <GuideSection guides={EXTERNAL_GUIDES} />
        </SettingsPage>
    );
}
