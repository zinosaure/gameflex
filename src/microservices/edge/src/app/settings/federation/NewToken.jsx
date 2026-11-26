// Route : /settings/federation/new-token — créer un nouvel accès.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { ChoiceRow } from "../../bricks/ChoiceRow.jsx";
import { CopyField } from "../../bricks/CopyField.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { createCatalogToken, createInvitation } from "../../libs/api.js";
import { invitationLink, InvitationReveal, useShareOrigin } from "../../screens/access/Invitation.jsx";
import { Form, FormButton, formContext } from "../../ui/Forms.jsx";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../profile/Federation.jsx";
import { ACCESSES, AccessName, RemoteRightsRows, scopesFor, useLevels } from "../Federation.jsx";

// CRÉER UN ACCÈS : comment le partager — « Partager par » : « Jeton classique » (le lien et un jeton montré une seule
// fois, qui restent sur la page jusqu'à « J'ai copié ») ou « QR code » (une invitation, le chemin proposé) ; un choix
// segmenté comme celui de Remote Play › Ajouter une console, en tête du formulaire (le porteur, 2026-11-20 ; il remplace
// l'interrupteur « Plutôt le lien et le jeton »). Puis pour qui, et ses droits à distance (RemoteRightsRows), coupés par
// défaut. UN bouton, dont le libellé suit le choix (« Créer l'invitation », « Créer le jeton ») : il dit ce qu'on obtiendra.
export default function NewToken() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const levels = useLevels();
    const origin = useShareOrigin();
    const { back } = useSettingsNavigation();
    const [values, setValues] = useState({ label: "", providers: false, remoteAdmin: false, share: "qr" });
    const [pending, setPending] = useState(false);
    const [created, setCreated] = useState(null);
    const [invited, setInvited] = useState(null);
    const form = formContext(values, setValues);
    const label = (values.label || "").trim();
    const scopes = () => scopesFor(values);
    const done = () => back(ACCESSES);

    const invite = async () => {
        setPending(true);
        const answer = await createInvitation({ kind: "federation", label, scopes: scopes(), remoteAdmin: values.remoteAdmin });
        setPending(false);

        if (!answer.ok || !answer.item) {
            feedback.failed(answer.detail || t("settings.actionFailed"));

            return;
        }
        setInvited(answer.item);
    };

    const create = async () => {
        setPending(true);
        const answer = await createCatalogToken(label, scopes(), values.remoteAdmin);
        setPending(false);

        if (!answer.ok || !answer.item) {
            feedback.failed(answer.detail || t("settings.actionFailed"));

            return;
        }
        setCreated(answer.item);
    };

    return (
        <SettingsPage title={t("fed.newAccess")} parent={ACCESSES} levels={levels} guides={FEDERATION_GUIDES}>
            {invited ? (
                <InvitationReveal link={invitationLink(origin, invited.code)} expiresAt={invited.expires_at} title={t("invitations.federationTitle", { label: invited.label })} onDone={done} />
            ) : created ? (
                <>
                    {/* Montré une seule fois : l'aide du groupe le dit, plus un encadré `alert` (le porteur, 2026-11-16). */}
                    <Section title={t("fed.createdTitle", { label: created.label })} help={`${t("fed.createdHelp")} ${t("fed.onceOnly")}`} sense="modify">
                        <Rows>
                            <CopyField id="fd-created-link" label={t("fed.linkLabel")} help={t("fed.linkHelp")} value={origin} />
                            <CopyField id="fd-created-token" label={t("fed.tokenLabel")} help={t("fed.tokenOnceHelp")} value={created.token || ""} />
                        </Rows>
                    </Section>
                    {/* « J'ai copié » referme la page : une ligne de « Plus », comme « Terminé » d'une invitation (47 des modèles). */}
                    <Section title={t("more")} help={t("fed.createdMoreHelp")} sense="optional">
                        <Rows>
                            <NavRow id="fd-created-done" icon="bi-check2" label={t("fed.done")} opens="action" onOpen={done} />
                        </Rows>
                    </Section>
                </>
            ) : (
                <Form props={{ onSubmit: (event) => {
                    event.preventDefault();

                    if (label && !pending) {
                        (values.share === "token" ? create : invite)();
                    }
                } }}>
                    <Section title={t("fed.accessTitle")} help={t("fed.accessHelp")} sense="add">
                        <Rows>
                            {/* JETON CLASSIQUE OU QR CODE (le porteur, 2026-11-20 : « remplacer le switch par un bouton radio group
                                comme dans remote play/add console ») — le QR code, l'invitation, reste le chemin proposé. */}
                            <ChoiceRow id="fd-share" label={t("fed.shareAs")} help={t("fed.shareAsHelp")} value={values.share}
                                options={[["token", t("fed.shareToken")], ["qr", t("fed.shareQr")]]} onChange={(share) => setValues((previous) => ({ ...previous, share }))} />
                            <AccessName form={form} />
                        </Rows>
                    </Section>
                    <Section title={t("fed.remoteRights")} help={t("fed.remoteRightsHelp")} sense="modify">
                        <Rows>
                            <RemoteRightsRows values={values} setValues={setValues} />
                            <SubmitRow>
                                <FormButton id="fd-create" label={t(values.share === "token" ? "fed.createToken" : "invitations.create")} isReplaced={Boolean(label)} pending={pending} />
                            </SubmitRow>
                        </Rows>
                    </Section>
                </Form>
            )}
        </SettingsPage>
    );
}
