// Route : /settings/profile/mfa — ma double authentification (chantier 80, `#809` ; le porteur, 2026-11-24 : « Le joueur décide
// s'il active le MFA ou pas mais l'admin dans membres il peut aussi l'imposer »).
//   - Détails : activée ou non, imposée (par ce serveur, par un administrateur) ou non, les codes de secours restants ;
//   - désactivée : l'activer — le mot de passe, puis le code QR et la clé, puis un premier code ; les codes de secours,
//     montrés une fois ;
//   - activée : refaire les codes de secours, ou la désactiver (le mot de passe et un code) — désactiver reste visible et
//     désactivé quand elle est imposée (son Ruleset), sa raison à la place de l'aide de ses champs.
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { disableMfa, enableMfa, fetchMfa, renewRecoveryCodes, startMfa } from "../../libs/api.js";
import { Form, formContext, FormButton, Ruleset } from "../../ui/Forms.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import { Loading } from "../../bricks/Loading.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { MfaEnrollment, RecoveryCodes } from "../../ui/Mfa.jsx";
import SettingsPage, { PROFILE, usePageText } from "../SettingsPage.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";

// Les champs des trois formulaires de la page, chacun les siens.
const EMPTY = { password: "", code: "", renewPassword: "", renewCode: "", offPassword: "", offCode: "" };

export default function Mfa() {
    const { t } = useTranslation();
    const page = usePageText("mfa");
    const feedback = useFeedback();
    const [state, setState] = useState(null);
    const [setup, setSetup] = useState(null);
    const [codes, setCodes] = useState(null);
    const [values, setValues] = useState(EMPTY);
    const [pending, setPending] = useState("");

    const reload = async () => {
        const result = await fetchMfa();
        setState(result.ok ? result.item : false);
    };

    useEffect(() => {
        reload();
    }, []);

    // Un geste du formulaire : le champ vidé, l'état relu ; le refus dit tel que le service le traduit.
    const act = async (name, call, onDone) => {
        setPending(name);
        const result = await call();
        setPending("");

        if (!result.ok) {
            feedback.failed(result.detail || t("settings.saveFailed"));

            return;
        }
        setValues(EMPTY);
        onDone(result.item);
        reload();
    };
    const submitOn = (handler) => ({ onSubmit: (event) => {
        event.preventDefault();
        handler();
    } });

    const required = state?.required;
    const requiredBy = state?.required_by === "server" ? t("mfa.byServer") : state?.required_by === "member" ? t("mfa.byAdmin") : t("mfa.notRequired");
    // Imposée, la désactiver reste là, désactivé par son Ruleset, et dit pourquoi à la place de l'aide de ses champs.
    const imposed = () => (required ? Ruleset.isDisabled(t("mfa.cannotDisable", { by: requiredBy })) : null);
    const form = formContext(values, setValues, { offPassword: imposed, offCode: imposed });

    return (
        <SettingsPage title={page.title} parent={PROFILE}>
            {state === null ? <Section title={t("details")} help={t("mfa.detailsHelp")} sense="info"><Loading /></Section> : (
                <Section title={t("details")} help={t("mfa.detailsHelp")} sense="info">
                    <Rows>
                        <InfoRow icon="bi-shield-lock" label={t("mfa.state")} value={t(state?.enabled ? "mfa.on" : "mfa.off")} />
                        <InfoRow icon="bi-person-lock" label={t("mfa.required")} value={requiredBy} />
                        <InfoRow icon="bi-key" label={t("mfa.recoveryLeft")} value={state?.enabled ? String(state.recovery_left) : "-"} />
                    </Rows>
                </Section>
            )}

            {codes && (
                <Section title={t("mfa.codesTitle")} help={t("mfa.codesHelp")} sense="info">
                    <RecoveryCodes codes={codes} onDone={() => setCodes(null)} />
                </Section>
            )}

            {state && !state.enabled && !codes && (setup ? (
                <Form props={submitOn(() => act("enable", () => enableMfa(values.code.trim()), (item) => {
                    setSetup(null);
                    setCodes(item.recovery_codes);
                    feedback.success(t("mfa.enabled"));
                }))}>
                    <Section title={t("mfa.enrollTitle")} help={t("mfa.enrollSettingsHelp")} sense="add">
                        <MfaEnrollment uri={setup.uri} secret={setup.secret} />
                        <Rows className="mt-3">
                            <TextRow id="code" label={t("mfa.firstCode")} help={t("mfa.firstCodeHelp")} context={form} props={{ autoComplete: "one-time-code", inputMode: "numeric", "data-key": "mfa-first-code" }} />
                            <SubmitRow>
                                <FormButton id="mfa-enable" label={t("mfa.enable")} isReplaced={Boolean(values.code.trim())} pending={pending === "enable"} />
                            </SubmitRow>
                        </Rows>
                    </Section>
                </Form>
            ) : (
                <Form props={submitOn(() => act("setup", () => startMfa(values.password), setSetup))}>
                    <Section title={t("mfa.enrollTitle")} help={t("mfa.startHelp")} sense="add">
                        <Rows>
                            <TextRow id="password" kind="password" label={t("settings.currentPassword")} help={t("mfa.passwordHelp")} context={form} props={{ autoComplete: "current-password", "data-key": "mfa-password" }} />
                            <SubmitRow>
                                <FormButton id="mfa-start" label={t("mfa.start")} isReplaced={Boolean(values.password)} pending={pending === "setup"} />
                            </SubmitRow>
                        </Rows>
                    </Section>
                </Form>
            ))}

            {state?.enabled && !codes && (
                <>
                    <Form props={submitOn(() => act("renew", () => renewRecoveryCodes(values.renewPassword, values.renewCode.trim()), (item) => setCodes(item.recovery_codes)))}>
                        <Section title={t("mfa.renewTitle")} help={t("mfa.renewHelp")} sense="modify">
                            <Rows>
                                <TextRow id="renewPassword" kind="password" label={t("settings.currentPassword")} help={t("mfa.passwordHelp")} context={form} props={{ autoComplete: "current-password", "data-key": "mfa-renew-password" }} />
                                <TextRow id="renewCode" label={t("mfa.code")} help={t("mfa.codeHelp")} context={form} props={{ autoComplete: "one-time-code", "data-key": "mfa-renew-code" }} />
                                <SubmitRow>
                                    <FormButton id="mfa-renew" label={t("mfa.renew")} isReplaced={Boolean(values.renewPassword && values.renewCode.trim())} pending={pending === "renew"} />
                                </SubmitRow>
                            </Rows>
                        </Section>
                    </Form>
                    {/* DÉSACTIVER : visible et désactivé quand elle est imposée, la raison à la place de l'aide de ses champs. */}
                    <Form props={submitOn(() => !required && act("disable", () => disableMfa(values.offPassword, values.offCode.trim()), () => feedback.success(t("mfa.disabled"))))}>
                        <Section title={t("danger")} help={t("mfa.disableHelp")} sense="danger">
                            <Rows>
                                <TextRow id="offPassword" kind="password" label={t("settings.currentPassword")} help={t("mfa.passwordHelp")} context={form} props={{ autoComplete: "current-password", "data-key": "mfa-off-password" }} />
                                <TextRow id="offCode" label={t("mfa.code")} help={t("mfa.codeHelp")} context={form} props={{ autoComplete: "one-time-code", "data-key": "mfa-off-code" }} />
                                <SubmitRow>
                                    <FormButton id="mfa-disable" kind="danger" label={t("mfa.disable")} isReplaced={Boolean(values.offPassword && values.offCode.trim())} disabled={required} pending={pending === "disable"} />
                                </SubmitRow>
                            </Rows>
                        </Section>
                    </Form>
                </>
            )}
            <GuideSection guides={[["remoteAccess", "keep-it-safe"]]} />
        </SettingsPage>
    );
}
