// Route : /settings/resources/external/<id>/edit — modifier la connexion.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { SubmitRow } from "../../../bricks/SubmitRow.jsx";
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import { updateResource } from "../../../libs/api.js";
import { FormButton, Ruleset } from "../../../ui/Forms.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "../../SettingsPage.jsx";
import { accessOf, composeAddress, emptyResource, ResourceFields, resourceForm, resourceReady, sentValues, splitAddress } from "../../people/externalResources.jsx";
import { EXTERNAL_GUIDES, useExternalResources } from "../../Resources.jsx";

// MODIFIER LA CONNEXION (`…/external/<id>/edit`) : le formulaire de son genre, rempli ; un mot de passe laissé vide garde
// l'ancien. Enregistrée, la ressource se relit, et la page revient à sa fiche.
export default function ExternalEdit() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { id } = useParams();
    const { back } = useSettingsNavigation();
    const list = useExternalResources();
    const resource = (list || []).find((entry) => String(entry.id) === id);
    const parent = `/settings/resources/external/${id}`;
    const [values, setValues] = useState(() => emptyResource(""));
    const [pending, setPending] = useState(false);

    useEffect(() => {
        if (resource) {
            setValues({ name: resource.name, ...splitAddress(resource.kind, resource.address), folder: resource.folder, access: accessOf(resource), username: resource.username, password: "" });
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [resource?.id, resource?.name, resource?.address, resource?.folder, resource?.username]);

    if (list === undefined || !resource) {
        return (
            <SettingsPage title={t("resources.external.edit")} parent={parent} guides={EXTERNAL_GUIDES}>
                {list === undefined ? <Section><Loading /></Section> : <NotFound />}
            </SettingsPage>
        );
    }
    const form = resourceForm(values, setValues, resource.kind, t);
    const changed = values.name !== resource.name || composeAddress(resource.kind, values) !== resource.address || values.folder !== resource.folder
        || values.access !== accessOf(resource) || (values.access === "account" && values.username !== resource.username) || values.password !== "";

    const save = async (event) => {
        event.preventDefault();
        if (!changed || !resourceReady(values, resource.kind) || Ruleset.hasFeedbacks(form)) {
            return;
        }
        setPending(true);
        const answer = await updateResource(resource.id, { ...sentValues(values, resource.kind), kind: resource.kind });
        setPending(false);
        feedback.result(answer.ok, t("fedService.connections.saved"), answer.errorMessage || t("fedService.failed"));
        if (answer.ok) {
            back(parent);
        }
    };

    return (
        <SettingsPage title={t("resources.external.edit")} parent={parent} levels={{ [parent]: resource.name }}>
            <Section help={t(`fedService.kinds.${resource.kind}.intro`)} sense="modify">
                <form onSubmit={save}>
                    <Rows>
                        <ResourceFields kind={resource.kind} form={form} values={values} setValues={setValues} editing />
                        <SubmitRow><FormButton id="rx-save" label={t("fedService.save")} isReplaced={changed && resourceReady(values, resource.kind)} disabled={Boolean(Ruleset.hasFeedbacks(form))} pending={pending} /></SubmitRow>
                    </Rows>
                </form>
            </Section>
            <GuideSection guides={EXTERNAL_GUIDES} />
        </SettingsPage>
    );
}
