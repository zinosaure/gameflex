import React, { useState } from "react";
import * as bs from "react-bootstrap";
import { useTranslation } from "react-i18next";
import Skeleton from "../../../skeleton/skeleton.js";
import { useAuth } from "./AuthContext.jsx";

const Form = Skeleton.Form;

// Rendu par App() DANS index.js à la place de <Skeleton> entier quand on
// n'est pas connecté.
//
// PAS D.INSCRIPTION, NULLE PART. Un compte se crée par un administrateur —
// sur le serveur comme sur le téléphone (décidé le 2026-09-08). Le téléphone
// a porté l'inscription libre quelque temps, pour la soirée où trois
// personnes veulent jouer sans administrateur sous la main ; l'arbitrage a
// changé, et deux façons de créer un compte étaient une de trop.
//
// Cette page n'a donc qu'un nom et un mot de passe.
export default function Login() {
    const { t } = useTranslation();
    const { login } = useAuth();
    const [values, setValuesState] = useState({ username: "", password: "" });
    const [error, setError] = useState(null);
    const [isSubmitting, setIsSubmitting] = useState(false);

    const formContext = {
        values,
        setValues: (v) => setValuesState((prev) => ({ ...prev, ...v })),
        files: {}, setFiles: () => {}, rulesets: {},
    };

    const handleSubmit = async (e) => {
        e.preventDefault();
        setError(null);
        setIsSubmitting(true);

        // LE BOUTON REVIENT TOUJOURS, ET UN ÉCHEC SE DIT.
        //
        // Ni cette fonction ni `login()` n'attrapaient quoi que ce soit : la
        // moindre exception sur le chemin de connexion laissait `isSubmitting`
        // à vrai. Le bouton tournait alors indéfiniment, sans message et sans
        // retour possible — il fallait fermer complètement l'application pour
        // retrouver un écran utilisable (signalé le 2026-09-25).
        //
        // Le `finally` rend la main quoi qu'il arrive ; le `catch` n'est que
        // le filet, la cause précise étant désormais nommée par `api()`
        // lui-même (voir AuthContext.jsx).
        try {
            const result = await login(values.username, values.password);

            if (!result.ok) {
                setError(result.exception?.detail || t("auth.loginFailed"));
            }
        } catch (failure) {
            console.error("échec de connexion", failure);
            setError(t("app.error"));
        } finally {
            setIsSubmitting(false);
        }
    };

    return (
        <div className="d-flex align-items-center justify-content-center bg-body-tertiary" style={{ minHeight: "var(--gameflex-visible-height)" }}>
            <bs.Card style={{ width: 360 }} className="shadow-sm">
                <bs.Card.Body className="p-4">
                    <h4 className="fw-bold mb-4 text-center">
                        {t("auth.login")}
                    </h4>
                    <form onSubmit={handleSubmit}>
                        <div className="mb-3">
                            <Form.Text
                                name="username"
                                context={formContext}
                                props={{ autoComplete: "username", placeholder: t("auth.username"), autoFocus: true }}
                            />
                        </div>
                        <div className="mb-3">
                            <Form.Password
                                name="password"
                                context={formContext}
                                props={{
                                    autoComplete: "current-password",
                                    placeholder: t("auth.password"),
                                }}
                            />
                        </div>
                        {error && <bs.Alert variant="danger" className="py-2 small">{error}</bs.Alert>}
                        <bs.Button
                            type="submit"
                            variant="primary"
                            className="w-100"
                            disabled={isSubmitting || !values.username || !values.password}
                        >
                            {isSubmitting
                                ? <bs.Spinner animation="border" size="sm" />
                                : t("auth.login")}
                        </bs.Button>
                    </form>
                </bs.Card.Body>
            </bs.Card>
        </div>
    );
}
