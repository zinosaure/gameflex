// Route : /settings/federation/new-pairing — le code d'appairage du serveur.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { cancelPairingCode, createPairingCode } from "../../libs/api.js";
import { DiscoveryModeSection, PairingReveal, usePairing } from "../../screens/access/Pairing.jsx";
import { toast } from "../../shell/Shell.jsx";
import { Form, FormButton, formContext } from "../../ui/Forms.jsx";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { FEDERATION_GUIDES } from "../profile/Federation.jsx";
import { ACCESSES, AccessName, RemoteRightsRows, scopesFor, useLevels } from "../Federation.jsx";

// LE CODE D'APPAIRAGE D'UN ACCÈS (chantier 60) : le mode serveur en tête, puis le nom de l’accès (AccessName), ses droits à distance (RemoteRightsRows) et
// « Créer le code », grisé tant que le mode serveur est fermé. Le code créé prend la page (PairingReveal) ; servi, la
// page revient à la liste des accès, où le nouvel accès apparaît.
export default function NewPairing() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const levels = useLevels();
    const pairing = usePairing();
    const { back } = useSettingsNavigation();
    const [values, setValues] = useState({ label: "", providers: false, remoteAdmin: false, relayAllowed: false });
    const [pending, setPending] = useState(false);
    const [created, setCreated] = useState(null);
    const form = formContext(values, setValues);
    const label = (values.label || "").trim();
    const open = Boolean(pairing?.server);

    const create = async () => {
        setPending(true);
        const answer = await createPairingCode({ kind: "federation", label, scopes: scopesFor(values), remoteAdmin: values.remoteAdmin, relayAllowed: values.relayAllowed });
        setPending(false);

        if (!answer.ok || !answer.item) {
            feedback.failed(answer.detail || t("settings.actionFailed"));

            return;
        }
        setCreated(answer.item);
    };
    const finish = (text) => {
        toast(text);
        setCreated(null);
        back(ACCESSES);
    };

    return (
        <SettingsPage title={t("pairing.code")} parent={ACCESSES} levels={levels}>
            {created ? (
                <PairingReveal created={created} pairing={pairing} onDone={finish} onCancel={() => cancelPairingCode(created.id)} />
            ) : (
                <>
                    <DiscoveryModeSection scope="server" pairing={pairing} />
                    <Form props={{ onSubmit: (event) => {
                        event.preventDefault();

                        if (label && open && !pending) {
                            create();
                        }
                    } }}>
                        <Section title={t("fed.accessTitle")} help={open ? t("fed.accessHelp") : t("pairing.createNeedsMode")} sense="add">
                            <Rows>
                                <AccessName form={form} />
                            </Rows>
                        </Section>
                        <Section title={t("fed.remoteRights")} help={t("fed.remoteRightsHelp")} sense="modify">
                            <Rows>
                                <RemoteRightsRows values={values} set={(patch) => setValues((previous) => ({ ...previous, ...patch }))} />
                                <SubmitRow>
                                    <FormButton id="fd-pairing-create" label={t("pairing.create")} isReplaced={Boolean(label)} disabled={!open} pending={pending} />
                                </SubmitRow>
                            </Rows>
                        </Section>
                    </Form>
                    <GuideSection guides={FEDERATION_GUIDES} />
                </>
            )}
        </SettingsPage>
    );
}
