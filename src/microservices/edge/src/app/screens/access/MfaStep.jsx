import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { loginMfa } from "../../libs/api.js";
import { focusElement } from "../../libs/input.js";
import { Form, formContext, FormButton, FormButtons } from "../../ui/Forms.jsx";
import { MfaEnrollment, RecoveryCodes } from "../../ui/Mfa.jsx";

// LE SECOND TEMPS DE LA CONNEXION (chantier 80, `#809`) : après le bon mot de passe (Login.jsx) ou le lien d'un compte
// (Join.jsx), quand le service des comptes demande un code plutôt que d'ouvrir la session. `challenge` : sa réponse —
// `mfa: "code"` (le code de l'application, ou un code de secours), ou `mfa: "enroll"` (la double authentification est
// imposée et pas encore activée : le code QR, la clé, puis un premier code). Une activation finie montre les codes de
// secours UNE fois, avant d'entrer. `onSession` : la session ouverte ; `onCancel` : revenir au mot de passe.
export default function MfaStep({ challenge, onSession, onCancel }) {
    const { t } = useTranslation();
    const [values, setValues] = useState({ code: "" });
    const form = formContext(values, setValues);
    const [error, setError] = useState("");
    const [pending, setPending] = useState(false);
    const [opened, setOpened] = useState(null);
    const enroll = challenge.mfa === "enroll";

    useEffect(() => {
        focusElement(document.querySelector('[data-key="mfa-code"]'), true);
    }, []);

    const submit = async (event) => {
        event.preventDefault();
        setPending(true);
        setError("");
        const result = await loginMfa(challenge.mfa_token, values.code.trim());
        setPending(false);

        if (result.ok && result.item?.access_token) {
            if (result.item.recovery_codes?.length) {
                setOpened(result.item);
            } else {
                onSession(result.item);
            }

            return;
        }
        if (result.status === 0) {
            setError(t("login.unreachable"));
        } else if (result.status === 429) {
            setError(t("mfa.tooMany"));
        } else if (result.refusal === "mfa_expired") {
            onCancel(t("mfa.expired"));
        } else {
            setError(t("mfa.refused"));
            setValues({ code: "" });
        }
    };

    if (opened) {
        return (
            <div className="d-flex flex-column gap-3">
                <h1 className="mb-0 fw-bold">{t("mfa.codesTitle")}</h1>
                <p className="text-body-secondary mb-2">{t("mfa.codesHelp")}</p>
                <RecoveryCodes codes={opened.recovery_codes} onDone={() => onSession(opened)} />
            </div>
        );
    }

    return (
        <form className="d-flex flex-column gap-3" onSubmit={submit}>
            <h1 className="mb-0 fw-bold">{t(enroll ? "mfa.enrollTitle" : "mfa.title")}</h1>
            <p className="text-body-secondary mb-2">{t(enroll ? "mfa.enrollHelp" : "mfa.codeHelp")}</p>
            {enroll && <MfaEnrollment uri={challenge.uri} secret={challenge.secret} />}
            <Form.Text name="code" context={form} props={{ "data-nav": true, "data-key": "mfa-code", autoComplete: "one-time-code", inputMode: enroll ? "numeric" : "text",
                placeholder: t(enroll ? "mfa.firstCode" : "mfa.code"), "aria-label": t(enroll ? "mfa.firstCode" : "mfa.code") }} />
            {error && <p className="small text-danger mb-0" role="alert">{error}</p>}
            <FormButtons split="25/75">
                <FormButton bare id="mfa-back" type="button" kind="other" label={t("mfa.back")} isReplaced onClick={() => onCancel("")} />
                <FormButton bare id="mfa-submit" label={t(enroll ? "mfa.enable" : "mfa.verify")} isReplaced={Boolean(values.code.trim())} pending={pending} disabled={!values.code.trim()} />
            </FormButtons>
        </form>
    );
}
