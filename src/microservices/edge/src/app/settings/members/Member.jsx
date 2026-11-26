// Route : /settings/members/<id> — la page d'un membre.
import React, { useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useNavigate, useParams } from "react-router-dom";
import { DangerRow } from "../../bricks/DangerRow.jsx";
import { DangerSection } from "../../bricks/DangerSection.jsx";
import { GuideSection } from "../../bricks/GuideSection.jsx";
import { InfoRow } from "../../bricks/InfoRow.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { Rows } from "../../bricks/Rows.jsx";
import { SaveSection } from "../../bricks/SaveSection.jsx";
import { Section } from "../../bricks/Section.jsx";
import { SubmitRow } from "../../bricks/SubmitRow.jsx";
import { SwitchRow } from "../../bricks/SwitchRow.jsx";
import { TextRow } from "../../bricks/TextRow.jsx";
import { useFeedback } from "../../bricks/useFeedback.jsx";
import { isAdministrator } from "../../globals/accounts.js";
import { createUser, deleteUser, newMemberLink, resetMember, resetMemberMfa, updateUser } from "../../libs/api.js";
import { useAuth } from "../../libs/data.jsx";
import { counted, dateTime } from "../../libs/format.js";
import { InvitationReveal, useMemberLink } from "../../screens/access/Invitation.jsx";
import { Form, FormButton, formContext, Ruleset } from "../../ui/Forms.jsx";
import SettingsPage, { useSettingsNavigation } from "../SettingsPage.jsx";
import { awaiting, day, lockReason, memberName, MemberPending, MEMBERS, MEMBERS_GUIDES, passwordState, ROLE_ADMIN, ROLE_USER, tokenLive, useMember, withoutCode } from "../Members.jsx";
import { CONSOLE_ICON } from "../../globals/icons.js";

// UN MEMBRE (nouveau, ou existant) — le porteur, 2026-11-14 : « username, display name, role et password doivent être
// dans la même section » ; « rôle doit être en switch au lieu de selectbox » : son compte en UN groupe, le rôle en
// interrupteur (administrateur ou non), Enregistrer dans son encadré. Un membre existant a son jeton (une ligne qui ouvre
// sa page), puis « Plus » — « All rétro, streamex et remoteplay doit être un lien dans more qui ouvre la page des "Console
// access", et rajoute aussi un lien pour voir les sauvegardes de l'utilisateur » —, puis sa suppression, dans « Danger » :
// ses sauvegardes partent avec lui (le porteur, 2026-11-14 : « Supprimer » ; le kernel les efface), ce que son aide et sa
// fenêtre de confirmation disent. Un nouveau membre a ces deux lignes grisées : elles s'ouvrent une fois le compte créé (il peut d'ici là tout lancer, ce
// que le serveur donne par défaut). `locked` : pourquoi ce compte ne se gère pas d'ici (root, son propre compte) — la page
// s'ouvre quand même, ses champs et ses gestes grisés, la raison dite dans l'aide du compte et sous la ligne de suppression.
//
// LE MOT DE PASSE, LE MEMBRE LE CHOISIT (chantier 55 ; le porteur : « dans new member, password disabled car on va
// changer de méthode, les nouveaux membres vont rejoindre avec un lien d'invitation ») : sa ligne reste à sa place,
// grisée, et son aide dit où il en est (passwordState). Créé, le membre a un lien : la page le montre jusqu'à « Terminé »
// (InvitationReveal, sans « Partager », comme « Mon QR »), puis s'ouvre sur sa page. Tant qu'il attend, « Plus » lui donne
// un nouveau lien (l'ancien ne vaut plus ; sans confirmation : rien ne se perd) ; une fois rejoint, « Danger » réinitialise
// son mot de passe — la fenêtre de Skeleton, puis le lien.
export function MemberForm({ member: given, locked = null }) {
    const { t, i18n } = useTranslation();
    const feedback = useFeedback();
    const { open, back } = useSettingsNavigation();
    const navigate = useNavigate();
    const location = useLocation();
    const memberLink = useMemberLink();
    // La réponse du serveur à un geste sur le lien, qui remplace le membre donné par la page.
    const [updated, setUpdated] = useState(null);
    const member = updated || given;
    const editing = Boolean(member);
    const [values, setValues] = useState(() => ({
        username: member?.username || "",
        display_name: member?.display_name || "",
        role_level: String(member?.role_level ?? ROLE_USER),
    }));
    const [pending, setPending] = useState(false);
    const [linking, setLinking] = useState(false);
    // Le lien qu'on vient de créer, montré une fois : `{code, expires_at, kind}`.
    const [revealed, setRevealed] = useState(null);
    const admin = isAdministrator(values.role_level);

    // L'IDENTIFIANT (le porteur, 2026-11-25 : « pas d'espace, minimum 3 characters et max 20 ») : la règle du service des
    // comptes (users_handlers.go, `usernamePattern`), dite dans l'ordre où on la rencontre en tapant.
    const typed = values.username;
    const problems = {
        username: editing ? "" : /\s/.test(typed) ? t("members.usernameNoSpace") : typed.length < 3 ? t("members.usernameMinChars", { min: 3 })
            : typed.length > 20 ? t("members.usernameMaxChars", { max: 20 }) : !/^[A-Za-z0-9._-]+$/.test(typed) ? t("members.usernameChars") : "",
        display_name: values.display_name.length < 3 ? t("members.displayNameMinChars", { min: 3 }) : values.display_name.length > 25 ? t("members.displayNameMaxChars", { max: 25 }) : "",
    };
    // L'erreur d'un champ se montre une fois qu'on y a écrit (un champ vide n'est pas faux, il manque).
    const shownProblem = (name) => (data) => (data[name] && problems[name] ? Ruleset.isInvalid(problems[name]) : null);
    const greyed = () => Ruleset.isDisabled(null);
    const form = formContext(values, setValues, {
        username: editing ? greyed : shownProblem("username"),
        display_name: locked ? greyed : shownProblem("display_name"),
        // LE MOT DE PASSE, PAR LE LIEN (le porteur, 2026-11-25 : « le champ "son mot de passe comment ?" doit être en switch
        // aussi ») : l'interrupteur « Par un lien d'invitation », allumé et désactivé — le service des comptes refuse un mot
        // de passe donné à la création (chantier 55) —, et le champ désactivé, sa raison à la place de son aide.
        byLink: () => Ruleset.isDisabled(t("members.byLinkOnly")),
        password: () => Ruleset.isDisabled(editing ? passwordState(t, i18n.language, member) : t("members.passwordByLink")),
    });
    const changed = !editing || values.display_name !== member.display_name || values.role_level !== String(member.role_level);
    const invalid = Object.values(problems).some(Boolean);

    const save = async () => {
        setPending(true);
        const payload = { display_name: values.display_name, role_level: parseInt(values.role_level, 10) };

        if (!editing) {
            payload.username = values.username.trim();
        }
        const answer = editing ? await updateUser(member.id, payload) : await createUser(payload);
        setPending(false);

        if (!answer.ok) {
            feedback.failed(answer.detail || t("members.saveFailed"));

            return;
        }
        feedback.success(t(editing ? "members.updated" : "members.created", { name: values.display_name }));

        // Créé : son lien, tout de suite — il ne se reverra pas.
        if (!editing && answer.item?.invitation?.code) {
            setUpdated(withoutCode(answer.item));
            setRevealed(answer.item.invitation);

            return;
        }
        // La liste se relit en s'ouvrant : elle montre le membre tel qu'enregistré.
        back(MEMBERS);
    };

    const remove = async () => {
        const answer = await deleteUser(member.id);

        if (!answer.ok) {
            feedback.failed(answer.detail || t("members.deleteFailed"));

            return;
        }
        feedback.success(t("members.deleted", { name: memberName(member) }));
        back(MEMBERS);
    };

    // SA DOUBLE AUTHENTIFICATION (chantier 80) : l'imposer, ou l'effacer (téléphone et codes de secours perdus) ; le membre
    // tel que le serveur le rend.
    const [securing, setSecuring] = useState(false);
    const secure = async (call, done) => {
        setSecuring(true);
        const answer = await call();
        setSecuring(false);

        if (!answer.ok) {
            feedback.failed(answer.detail || t("settings.actionFailed"));

            return;
        }
        setUpdated(answer.item);
        feedback.success(t(done, { name: memberName(member) }));
    };

    // Un nouveau lien, ou la réinitialisation : le membre tel que le serveur le rend, et son lien à montrer.
    const link = async (call, done) => {
        setLinking(true);
        const answer = await call(member.id);
        setLinking(false);

        if (!answer.ok || !answer.item?.invitation?.code) {
            feedback.failed(answer.detail || t("settings.actionFailed"));

            return;
        }
        setUpdated(withoutCode(answer.item));
        setRevealed(answer.item.invitation);
        feedback.success(t(done, { name: memberName(answer.item) }));
    };

    // « Terminé » : la page du membre — celle d'un nouveau membre remplace « Nouveau membre » dans l'historique.
    const done = () => {
        setRevealed(null);

        if (!given) {
            navigate(`${MEMBERS}/${member.id}`, { replace: true, state: { ...location.state, member } });
        }
    };

    if (revealed) {
        return (
            <SettingsPage title={memberName(member)} parent={MEMBERS} guides={MEMBERS_GUIDES}>
                <InvitationReveal link={memberLink(revealed.code)} expiresAt={revealed.expires_at} title={t("members.invitationTitle", { name: memberName(member) })} canShare={false}
                    help={t(revealed.kind === "reset" ? "members.invitationResetHelp" : "members.invitationHelp")} validity="members.invitationValidUntil" onDone={done} />
            </SettingsPage>
        );
    }

    const page = (path, key) => () => open(`${MEMBERS}/${member.id}/${path}`, key, { member });

    return (
        <SettingsPage title={editing ? memberName(member) : t("members.create")} parent={MEMBERS}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (changed && !invalid && !pending && !locked) {
                    save();
                }
            } }}>
                <Section title={t("members.accountTitle")} help={locked || t("members.accountHelp")} sense="modify">
                    <Rows>
                        <TextRow id="username" label={t("members.username")} help={t(editing ? "members.usernameFixedHelp" : "members.usernameHelp")} context={form}
                            props={{ "data-key": "mb-username", autoComplete: "off" }} />
                        <TextRow id="display_name" label={t("members.displayName")} help={t("members.displayNameHelp")} context={form} props={{ "data-key": "mb-display_name" }} />
                        <SwitchRow id="mb-admin" label={t("members.roleAdmin")} help={t("members.roleHelp")} checked={admin} disabled={Boolean(locked)}
                            onChange={(on) => setValues((previous) => ({ ...previous, role_level: String(on ? ROLE_ADMIN : ROLE_USER) }))} />
                        <SwitchRow id="mb-by-link" label={t("members.byLink")} help={t("members.byLinkOnly")} checked disabled onChange={() => {}} />
                        <TextRow id="password" kind="password" label={t("members.password")} help={t("members.passwordHelp")} context={form}
                            props={{ autoComplete: "off", "data-key": "mb-password" }} />
                        <SubmitRow>
                            <FormButton id="mb-save" label={t(editing ? "settings.save" : "members.createSubmit")} isReplaced={Boolean(changed)} pending={pending} disabled={Boolean(locked) || Boolean(invalid)} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
            {/* INFOS (le porteur, 2026-11-20 : « une section info avant la section plus pour afficher les infos de la
                connexion et du token ») : son inscription, sa dernière connexion ; son jeton — l'état, l'échéance, ses
                sessions, son dernier usage —, en lecture : chacun gère le sien (Profil › Fédération). */}
            {editing && (
                <Section title={t("details")} help={t("members.detailsHelp")} sense="info">
                    <Rows>
                        <InfoRow icon="bi-person-check" label={t("members.signup")}
                            value={awaiting(member) ? t("members.signupPending") : t("members.signupDoneOn", { at: day(member.password_set_at, i18n.language) })} />
                        <InfoRow icon="bi-box-arrow-in-right" label={t("members.lastLogin")} value={member.last_login ? dateTime(member.last_login, i18n.language) : t("members.tokenNever")} />
                        {member.token && (
                            <>
                                <InfoRow icon="bi-key" label={t("members.tokenTitle")}
                                    value={!member.token.enabled ? t("members.tokenDisabled") : tokenLive(member.token)
                                        ? t("members.tokenUntil", { at: dateTime(member.token.expires_at, i18n.language) }) : t("members.tokenOver")} />
                                <InfoRow icon="bi-box-arrow-in-right" label={t("members.tokenSessions")} value={counted(t, "sessions", member.token.use_count || 0)} />
                                <InfoRow icon="bi-clock-history" label={t("members.tokenLastUsed")}
                                    value={member.token.last_used_at ? dateTime(member.token.last_used_at, i18n.language) : t("members.tokenNever")} />
                            </>
                        )}
                    </Rows>
                </Section>
            )}
            {/* LA DOUBLE AUTHENTIFICATION (chantier 80) : activée ou non, l'imposer. Root et son propre compte : grisé, la
                raison en tête de section, comme le reste de la fiche. */}
            {editing && (
                <SaveSection id="mb-mfa-section" title={t("mfa.memberTitle")} help={locked || t("mfa.memberHelp")} saved={{ required: Boolean(member.mfa_required) }}
                    disabled={Boolean(locked)} success={(values) => t(values.required ? "mfa.requiredToast" : "mfa.freedToast", { name: memberName(member) })}
                    onSave={async (values) => {
                        // SON BOUTON (le porteur, 2026-11-26 : « les sections formulaire n'ont pas de FormButton »).
                        const answer = await updateUser(member.id, { mfa_required: values.required });

                        if (answer.ok) {
                            setUpdated(answer.item);
                        }

                        return answer;
                    }}>
                    {(values, set) => (
                        <>
                            <InfoRow icon="bi-shield-lock" label={t("mfa.state")} value={t(member.mfa_enabled ? "mfa.on" : "mfa.off")} />
                            <SwitchRow id="mb-mfa" label={t("mfa.requireMember")} help={t("mfa.requireMemberHelp")} checked={values.required} disabled={Boolean(locked)}
                                onChange={(on) => set({ required: on })} />
                        </>
                    )}
                </SaveSection>
            )}
            <Section title={t("more")} help={t("members.moreHelp")} sense="modify">
                <Rows>
                    <NavRow id="mb-consoles" icon={CONSOLE_ICON} label={t("members.consolesTitle")}
                        help={!editing ? t("members.afterCreate") : isAdministrator(member.role_level) ? t("members.consolesAdmin") : t("members.consolesRowHelp")}
                        disabled={!editing} onOpen={editing ? page("consoles", "mb-consoles") : undefined} />
                    <NavRow id="mb-saves" icon="bi-shield-check" label={t("members.saves")} help={t(editing ? "members.savesRowHelp" : "members.afterCreateSaves")}
                        disabled={!editing} onOpen={editing ? page("saves", "mb-saves") : undefined} />
                    {editing && awaiting(member) && (
                        <NavRow id="mb-new-link" icon="bi-link-45deg" label={t("members.newLink")} help={t("members.newLinkHelp")} opens="action" pending={linking}
                            onOpen={() => link(newMemberLink, "members.newLinkToast")} />
                    )}
                </Rows>
            </Section>
            {editing && (
                <DangerSection>
                    {!awaiting(member) && (
                        <DangerRow id="mb-reset" icon="bi-key" label={t("members.resetTitle")} help={locked || t("members.resetHelp")} disabled={Boolean(locked) || linking}
                            question={t("members.resetConfirm", { name: memberName(member) })} onConfirm={() => link(resetMember, "members.resetToast")} />
                    )}
                    <DangerRow id="mb-mfa-reset" icon="bi-shield-x" label={t("mfa.resetTitle")} help={locked || (member.mfa_enabled ? t("mfa.resetHelp") : t("mfa.resetNothing"))}
                        disabled={Boolean(locked) || !member.mfa_enabled || securing} question={t("mfa.resetConfirm", { name: memberName(member) })}
                        onConfirm={() => secure(() => resetMemberMfa(member.id), "mfa.resetToast")} />
                    <DangerRow id={`mb-del-${member.id}`} label={t("members.deleteTitle")} help={locked || t("members.deleteHelp")} disabled={Boolean(locked)}
                        question={t("members.deleteConfirm", { name: memberName(member) })} onConfirm={remove} />
                </DangerSection>
            )}
            <GuideSection guides={MEMBERS_GUIDES} />
        </SettingsPage>
    );
}

// /settings/members/<id> : la page d'un membre. Root et son propre compte s'ouvrent grisés, et disent pourquoi.
export default function Member() {
    const { t } = useTranslation();
    const { memberId } = useParams();
    const { user } = useAuth();
    const member = useMember(memberId);

    if (!member) {
        return <MemberPending member={member} title={t("settings.items.members.label")} parent={MEMBERS} />;
    }

    return <MemberForm member={member} locked={lockReason(t, member, user)} />;
}
