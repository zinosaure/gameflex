import React, { useState } from "react";
import Skeleton from "../../../skeleton/skeleton.js";
import { updateMe } from "../api.js";
import {
    SectionPanel,
    FormButton,
} from "../../../gameflex/shared/templates/Essentials.jsx";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// The password change form is a separate section because it maps to a
// different backend concern (current_password+new_password, see
// users/routes.py's update_me) and there's no reason changing your name
// should require also re-typing your password. Logout lives in the left
// menu now (Profil section), not here — see index.js.
export default function Password({ context, token, t }) {
    const [values, setValues] = useState({
        current_password: "",
        new_password: "",
        confirm_password: "",
    });
    const [isPending, setPending] = useState(false);
    const contextForm = {
        values,
        setValues: (value) => setValues((prev) => ({ ...prev, ...value })),
        files: {},
        setFiles: () => { },
        rulesets: {
            new_password: (data) => {
                if (!data.new_password || data.new_password.length < 8) {
                    return Ruleset.isInvalid(t("profile.newPasswordMinChars", { min: 8 }));
                }

                return null;
            },
            confirm_password: (data) => {
                if (data.confirm_password !== data.new_password) {
                    return Ruleset.isInvalid(t("profile.passwordMismatch"));
                }

                return null;
            },
        },
    };

    return (
        <SectionPanel>
            <Form.Group label={t("profile.currentPassword")} helpText={t("profile.currentPasswordHelp")}>
                <Form.Password
                    name="current_password"
                    context={contextForm}
                    props={{
                        autoComplete: "current-password",
                        placeholder: t("profile.currentPassword")
                    }}
                />
            </Form.Group>
            <Form.Group label={t("profile.newPassword")} helpText={t("profile.newPasswordHelp")}>
                <Form.Password
                    name="new_password"
                    context={contextForm}
                    withStrengthEvaluation
                    props={{
                        autoComplete: "new-password",
                        placeholder: t("profile.newPassword")
                    }}
                />
            </Form.Group>
            <Form.Group label={t("profile.confirmPassword")} helpText={t("profile.confirmPasswordHelp")}>
                <Form.Password
                    name="confirm_password"
                    context={contextForm}
                    props={{
                        autoComplete: "new-password",
                        placeholder: t("profile.confirmPassword")
                    }}
                />
            </Form.Group>
            <FormButton
                callback={async (e) => {
                    e.preventDefault();
                    setPending(true);

                    try {
                        const result = await updateMe(token, {
                            current_password: values.current_password,
                            new_password: values.new_password,
                        });

                        if (result.ok) {
                            context.toast.open(t("profile.passwordChangeSuccess"), { variant: "success" });
                        } else {
                            context.toast.open(result.exception?.detail || t("profile.saveFailed"), { variant: "danger" });
                        }
                    } catch (failure) {
                        console.error("échec du changement de mot de passe", failure);
                        context.toast.open(t("app.error"), { variant: "danger" });
                    } finally {
                        // Même raison que l'écran de connexion : sans ce
                        // `finally`, un appel qui jette laisse `isPending` à
                        // vrai, et le bouton d'enregistrement reste figé.
                        setPending(false);
                    }
                }}
                label={t("app.save")}
                isDisabled={isPending || Ruleset.hasFeedbacks(contextForm)}
                isPending={isPending} />
        </SectionPanel>
    );
}