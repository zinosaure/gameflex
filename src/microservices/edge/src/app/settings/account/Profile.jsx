import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes } from "react-router-dom";
import { updateMe } from "../../libs/api.js";
import { useAuth } from "../../libs/data.jsx";
import { toast } from "../../shell/Shell.jsx";
import { Form, formContext, FormButton, Ruleset } from "../../ui/Forms.jsx";
import { MyFavoritesRoutes } from "./Favorites.jsx";
import { PasswordPage } from "./Password.jsx";
import { MyFederationRoutes } from "./MyFederation.jsx";
import { MySavesRoutes } from "./Saves.jsx";
import SettingsPage, { PROFILE, usePageText, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, { Rows, Section, SubmitRow, TextRow } from "../../ui/Bricks.jsx";

// PROFIL ET SES PAGES ENFANTS (Settings.jsx en fait l'entrée et la route) : le mot de passe (Password.jsx), la
// fédération — mon jeton (Token.jsx) et mes comptes fédérés (MyFederation.jsx) —, Mes sauvegardes (Saves.jsx), Mes
// favoris (Favorites.jsx). Les pages du compte sont dans settings/account/ ; Se déconnecter aussi (Logout.jsx).
// Ce qui manque encore dans une page le dit dans la page même ; aucun faux contrôle. En direction C
// (Bricks.jsx) : des groupes titrés, leurs lignes, le bouton d'un formulaire dans l'encadré de son groupe (SubmitRow),
// un geste seul en ligne.

// MON PROFIL : mon identifiant (fixe) et mon nom affiché (3 à 25 caractères, comme la webapp).
// PROFIL ET SES PAGES ENFANTS (le porteur, 2026-11-13 : « mot de passe, jeton et mes sauvegardes vont DANS profil.
// Là c'est en dehors. ») : le profil, puis « Le compte » — Mot de passe, Fédération, Mes sauvegardes, Mes favoris (le
// porteur, 2026-11-16 : « dans profile il faut le lien pour : mes favoris ») —, chacun sa page
// sous /settings/profile/ (PROFILE, SettingsPage.jsx), que Retour et le fil d'Ariane ramènent à Profil. « Jeton » est
// devenu « Fédération » (le porteur, 2026-11-16, 2.1) : mon jeton et mes comptes fédérés (MyFederation.jsx).

export function ProfileRoutes() {
    return (
        <Routes>
            <Route index element={<ProfilePage />} />
            <Route path="password" element={<PasswordPage />} />
            <Route path="federation/*" element={<MyFederationRoutes />} />
            <Route path="saves/*" element={<MySavesRoutes />} />
            <Route path="favorites/*" element={<MyFavoritesRoutes />} />
            <Route path="*" element={<Navigate to={PROFILE} replace />} />
        </Routes>
    );
}

function ProfilePage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const page = usePageText("profile");
    const { user, refreshMe } = useAuth();
    const [values, setValues] = useState({ username: user?.username || "", display_name: user?.display_name || "" });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues, {
        display_name: (data) => {
            const name = (data.display_name || "").trim();

            if (name.length < 3) {
                return Ruleset.isInvalid(t("settings.profileMin"));
            }

            return name.length > 25 ? Ruleset.isInvalid(t("settings.profileMax")) : null;
        },
    });
    // Rien de changé n'est pas une erreur (CODE_CONVENTIONS.md, `isReplaced`).
    const isReplaced = (values.display_name || "").trim() !== (user?.display_name || "");

    const save = async () => {
        setPending(true);
        const result = await updateMe({ display_name: values.display_name.trim() });
        setPending(false);

        if (result.ok) {
            await refreshMe();
            toast(t("settings.saved"));
        } else {
            toast(result.detail || t("settings.saveFailed"));
        }
    };

    return (
        <SettingsPage title={page.title}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (isReplaced && !Ruleset.hasFeedbacks(form) && !pending) {
                    save();
                }
            } }}>
                <Section title={t("settings.profileFormTitle")}>
                    <Rows>
                        <TextRow id="username" label={t("settings.username")} help={t("settings.usernameHelp")} context={form} props={{ disabled: true, autoComplete: "username", "data-key": "pr-username" }} />
                        <TextRow id="display_name" label={t("settings.displayName")} help={t("settings.displayNameHelp")} context={form} props={{ maxLength: 40, "data-key": "pr-display-name" }} />
                        <SubmitRow>
                            <FormButton id="sc-save" label={t("settings.save")} isReplaced={isReplaced} pending={pending} disabled={Boolean(Ruleset.hasFeedbacks(form))} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
            <Section title={t("settings.profileAccountTitle")}>
                <Rows>
                    {[["password", "password", "bi-lock", "slate"], ["federation", "myFederation", "bi-arrow-left-right", "amber"], ["saves", "mySaves", "bi-shield-check", "teal"], ["favorites", "myFavorites", "bi-star", "amber"]].map(([path, key, icon, tone]) => (
                        <NavRow key={path} id={`pr-${path}`} icon={icon} tone={tone} label={t(`settings.items.${key}.label`)} help={t(`settings.items.${key}.help`)}
                            onOpen={() => open(`${PROFILE}/${path}`, `pr-${path}`)} />
                    ))}
                </Rows>
            </Section>
        </SettingsPage>
    );
}
