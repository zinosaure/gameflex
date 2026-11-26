import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAuth } from "../../libs/data.jsx";
import { focusElement, useScreenInput } from "../../libs/input.js";
import { Form, formContext, FormButton } from "../../ui/Forms.jsx";
import MfaStep from "./MfaStep.jsx";

// LA CONNEXION DU LAUNCHER : une autre origine que la webapp, donc son propre
// jeton (`POST /api/v1/auth/login`, comme profile/auth/AuthContext.jsx). Ses deux champs sont ceux
// de Skeleton, comme la connexion de la webapp (profile/auth/Login.jsx : sans libellé, l'indication
// dans le champ). Son bouton est celui de tout formulaire (FormButton) : « ••• » tant qu'un des deux champs manque.

export default function Login() {
    const { t } = useTranslation();
    const { login, adopt } = useAuth();
    const [values, setValues] = useState({ username: "", password: "" });
    const form = formContext(values, setValues);
    const { username, password } = values;
    const [error, setError] = useState("");
    const [pending, setPending] = useState(false);
    // LA DOUBLE AUTHENTIFICATION (chantier 80) : le bon mot de passe d'un compte qui l'a, ou à qui elle est imposée, rend
    // un second temps au lieu de la session (MfaStep.jsx).
    const [challenge, setChallenge] = useState(null);

    useEffect(() => {
        focusElement(document.querySelector('[data-key="login-user"]'), true);
    }, []);

    useScreenInput({});

    const submit = async (event) => {
        event.preventDefault();
        setPending(true);
        setError("");
        const result = await login(username, password);
        setPending(false);

        if (result.ok && result.item?.mfa) {
            setChallenge(result.item);
        } else if (!result.ok) {
            setError(result.status === 0 ? t("login.unreachable") : t("login.refused"));
        }
    };

    if (challenge) {
        return (
            <main className="login">
                <MfaStep challenge={challenge} onSession={adopt} onCancel={(reason) => {
                    setChallenge(null);
                    setValues({ ...values, password: "" });
                    setError(reason);
                }} />
            </main>
        );
    }

    return (
        <main className="login">
            <form className="d-flex flex-column gap-3" onSubmit={submit}>
                <h1 className="mb-0 fw-bold">GameFlex</h1>
                <p className="text-body-secondary mb-2">{t("login.help")}</p>
                <Form.Text name="username" context={form} props={{ "data-nav": true, "data-key": "login-user", autoComplete: "username", placeholder: t("login.username"), "aria-label": t("login.username") }} />
                <Form.Password name="password" context={form} props={{ "data-nav": true, "data-key": "login-password", autoComplete: "current-password", placeholder: t("login.password"), "aria-label": t("login.password") }} />
                {/* Le refus, en une ligne de la couleur d'une erreur, sans encadré (plus d'alerte, le porteur, 2026-11-16). */}
                {error && <p className="small text-danger mb-0" role="alert">{error}</p>}
                <FormButton id="login-submit" label={t("login.submit")} isReplaced={Boolean(username || password)} pending={pending} disabled={!username || !password} />
            </form>
        </main>
    );
}
