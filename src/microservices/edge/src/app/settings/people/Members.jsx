import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import {
    createUser, deleteUser, deleteUserSaves, fetchUserSaves, listUsers, newMemberLink, regenerateUserToken, resetMember, updateUser, updateUserToken,
    userSavesArchiveUrl,
} from "../../libs/api.js";
import { useAuth, useData } from "../../libs/data.jsx";
import { formatSize } from "../../libs/format.js";
import { toast } from "../../shell/Shell.jsx";
import { InvitationReveal, useMemberLink, useShareOrigin } from "../../screens/access/Invitation.jsx";
import { Form, formContext, FormButton, Ruleset } from "../../ui/Forms.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import NavRow, {
    AccountName, atUsername, Avatar, CopyField, DangerRow, DangerSection, GuideSection, InfoRow, isAdministrator, Listing, Note, PlaceholderRow, Rows, Section, SelectRow, SubmitRow,
    SwitchRow, TextRow,
} from "../../ui/Bricks.jsx";

// LES MEMBRES (admin/Members.jsx et api/users.js de la webapp ; 24-membres et 25-membre des modèles) : la
// liste paginée par le serveur, cherchée par nom, filtrée dans sa fenêtre (/settings/members, `?q=&admins=&players=&
// token=&page=`) ; un membre s'ouvre sur sa page (/settings/members/<id>) — son compte (identifiant, nom affiché, rôle,
// mot de passe, dans un seul groupe), son jeton, puis « Plus » : ses consoles (…/<id>/consoles) et ses sauvegardes
// (…/<id>/saves) — et le supprimer, dans « Danger » ; on en crée un (/settings/members/new), en bas de la liste, dans
// « Plus ». SON MOT DE PASSE, IL LE CHOISIT (chantier 55) : l'administrateur reçoit un lien à lui envoyer, à la
// création, puis « Nouveau lien » tant qu'il attend, « Réinitialiser le mot de passe » une fois qu'il a rejoint. Une ligne de la liste n'a pas de geste : elle ouvre la page (le porteur, 2026-11-13). SON JETON a sa page
// (/settings/members/<id>/token) : sa vraie échéance, l'activer pour une durée, le prolonger ; le désactiver et le
// régénérer dans « Danger ». Root et son propre compte ne se gèrent pas ici : leurs pages s'ouvrent grisées, et disent
// pourquoi. Le bloc du jeton sert aussi à Réglages › Jeton pour un administrateur (son propre jeton, comme Profile.jsx de
// la webapp).

const ROLE_ROOT = 0;
const ROLE_ADMIN = 1;
const ROLE_USER = 2;
const PAGE_SIZE = 10;
const GRANT_ALL = "*";
const GRANT_RETRO = "retro";
const TOKEN_DURATIONS = ["1d", "1w", "1m", "3m", "6m", "1y", "2y"];
const MEMBERS = "/settings/members";

export const dateTime = (iso, language) => (iso ? new Date(iso).toLocaleString(language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "—");
const day = (iso, language) => (iso ? new Date(iso).toLocaleDateString(language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium" }) : "—");

// LE MOT DE PASSE D'UN MEMBRE, CE QU'IL EN EST (chantier 55) : l'aide de sa ligne « Mot de passe », la même sur les deux
// pages — un nouveau membre le choisira par son lien ; un membre l'a choisi tel jour, ou l'attend, avec son lien (jusqu'à
// quand, créé par qui) ou sans lien valable.
const awaiting = (member) => !member.password_set_at;
const passwordState = (t, language, member) => {
    if (!member) {
        return t("members.passwordByLink");
    }

    if (!awaiting(member)) {
        return t("members.passwordChosen", { at: day(member.password_set_at, language) });
    }
    const link = member.invitation;

    return link
        ? t("members.passwordPendingLink", { at: dateTime(link.expires_at, language), by: link.created_by ? atUsername(link.created_by) : "—" })
        : t("members.passwordPendingNoLink");
};

// La réponse d'une création, d'un nouveau lien ou d'une réinitialisation, sans le code : il ne se montre qu'une fois.
const withoutCode = (member) => (member?.invitation ? { ...member, invitation: { ...member.invitation, code: undefined } } : member);

// Les droits du compte en cases du formulaire, et retour (« * » quand tout est coché).
const grantsToForm = (grants, streamexIds, remoteplayIds) => {
    const granted = grants || [];
    const all = granted.includes(GRANT_ALL);

    return {
        retro: all || granted.includes(GRANT_RETRO),
        streamex: all ? streamexIds : streamexIds.filter((id) => granted.includes(id)),
        remoteplay: all ? remoteplayIds : remoteplayIds.filter((id) => granted.includes(id)),
    };
};
const formToGrants = (values, streamexIds, remoteplayIds) => (values.retro && streamexIds.every((id) => values.streamex.includes(id)) && remoteplayIds.every((id) => values.remoteplay.includes(id))
    ? [GRANT_ALL]
    : [...(values.retro ? [GRANT_RETRO] : []), ...streamexIds.filter((id) => values.streamex.includes(id)), ...remoteplayIds.filter((id) => values.remoteplay.includes(id))]);

// UN MEMBRE PAR SON IDENTIFIANT, pour une page ouverte par un lien direct ou relue : la route des comptes ne se
// cherche pas par identifiant (username, display_name, role_level seulement) ; ses pages vont dans l'ordre
// des identifiants, 100 par 100 — on s'arrête à la page qui le dépasse.
async function findMember(id) {
    const wanted = Number(id);

    for (let page = 1; page < 1000; page += 1) {
        const answer = await listUsers({ page, itemsPerPage: 100 });
        const items = answer.ok ? answer.items || [] : [];
        const found = items.find((member) => member.id === wanted);

        if (found || !answer.ok || items.length < 100 || items[items.length - 1].id > wanted) {
            return found || null;
        }
    }

    return null;
}

// Le nom d'un membre, partout où il se dit (son titre, le fil d'Ariane, une confirmation).
const memberName = (member) => member.display_name || member.username;

// LE GUIDE D'UN JETON : il sert à jouer d'un autre serveur, sous ce compte (docs/federation.md).
export const TOKEN_GUIDES = [["federation", "play-on-a-friends-server"]];

// L'ÉTAT D'UN JETON ET SES USAGES : la ligne qui ouvre sa page.
const isLive = (token) => Boolean(token.enabled && token.expires_at && new Date(token.expires_at) > new Date());
const tokenStatus = (t, language, token) => ({
    status: !token.enabled ? t("members.tokenDisabled") : t(isLive(token) ? "members.tokenEnabledUntil" : "members.tokenExpired", { at: dateTime(token.expires_at, language) }),
    usage: token.last_used_at ? t("members.tokenUsage", { count: token.use_count || 0, at: dateTime(token.last_used_at, language) }) : t("members.tokenNeverUsed"),
});

// Pourquoi ce compte ne se gère pas d'ici (root, son propre compte), ou rien. LA RAISON EST L'AIDE DU PREMIER GROUPE
// de la page grisée (le compte, le détail du jeton, le rétro) — plus une alerte en tête (le porteur, 2026-11-16 :
// « retirer les alerts »).
const lockReason = (t, member, user) => (member.role_level === ROLE_ROOT ? t("members.rootLocked") : member.id === user?.id ? t("members.ownLocked") : null);

// LE DÉTAIL D'UN JETON (le porteur, 2026-11-14 : « quand on extend un token et si on refresh la page, on ne sait plus la
// durée car le selectbox montre 1 mois par défaut ») : son état, SA VRAIE ÉCHÉANCE — celle que le serveur a enregistrée
// (`expires_at`), relue à chaque ouverture —, ses sessions et son dernier usage. Le choix d'une durée, lui, ne dit plus
// rien tant qu'on n'en a pas choisi une (MemberToken). Partagé par Réglages › Profil › Jeton (settings/account/Token.jsx) et la
// page du jeton d'un membre. `children` : ce qui suit ses lignes (ce que fait un joueur dont le jeton est éteint) ;
// `help` : l'aide du groupe (pourquoi ce jeton ne se gère pas d'ici).
export function TokenDetails({ token, help = null, children = null }) {
    const { t, i18n } = useTranslation();
    const live = isLive(token);

    return (
        <Section title={t("details")} help={help}>
            <Rows>
                <InfoRow icon="bi-key" tone={live ? "green" : "slate"} label={t("members.tokenState")}
                    value={t(!token.enabled ? "members.tokenDisabled" : live ? "members.tokenLive" : "members.tokenOver")} />
                {token.enabled && <InfoRow icon="bi-calendar-event" label={t(live ? "members.tokenExpiresAt" : "members.tokenExpiredAt")} value={token.expires_at ? dateTime(token.expires_at, i18n.language) : null} />}
                <InfoRow icon="bi-box-arrow-in-right" label={t("members.tokenSessions")} value={String(token.use_count || 0)} />
                <InfoRow icon="bi-clock-history" label={t("members.tokenLastUsed")} value={token.last_used_at ? dateTime(token.last_used_at, i18n.language) : t("members.tokenNever")} />
            </Rows>
            {children}
        </Section>
    );
}

// LE JETON D'UN COMPTE : son détail (TokenDetails) ; l'activer (ou le prolonger) pour la durée choisie — AVANT le jeton
// (le porteur, 2026-11-18 : « mettre la section extend avant la section token ») ; actif, le lien et le jeton à copier — AUCUNE n'est choisie d'avance (« ••• » tant qu'on n'en a pas pris une), le bouton
// dans l'encadré de la durée ; `more` : les lignes de « Plus » (« Créer mon QR » dans Réglages › Jeton) ; puis
// « Danger » — le désactiver (le porteur, 2026-11-14 : « "disable my token" dans danger avec une confirmation »), le
// régénérer (l'ancien cesse de valoir). `locked` : pourquoi ce compte ne se gère pas d'ici — tout est grisé, la raison
// dans l'aide du détail. Avant « Danger », le guide : jouer chez un autre serveur avec ce jeton (TOKEN_GUIDES).
export function MemberToken({ member, onChanged, locked = null, more = null }) {
    const { t } = useTranslation();
    const origin = useShareOrigin();
    const [state, setState] = useState(member.token || null);
    const [duration, setDuration] = useState("");
    const [pending, setPending] = useState(false);
    const disabled = Boolean(locked);
    const busy = pending || disabled;
    const name = memberName(member);

    if (!state) {
        return null;
    }

    const apply = async (call, done) => {
        setPending(true);
        const answer = await call();
        setPending(false);

        if (!answer.ok) {
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }
        setState(answer.item);
        setDuration("");
        onChanged?.({ ...member, token: answer.item });
        toast(done);
    };

    const enable = state.enabled ? "extend" : "enable";

    return (
        <>
            <TokenDetails token={state} help={locked} />
            <Section title={t(state.enabled ? "members.tokenExtend" : "members.tokenEnable")} help={t(state.enabled ? "members.tokenExtendHelp" : "members.tokenEnableHelp")}>
                <Rows>
                    <SelectRow id="mt-duration" label={t("members.tokenDuration")} help={t("members.tokenDurationHelp")} value={duration} onChange={(value) => setDuration(value || "")} disabled={disabled}
                        options={[{ value: "", label: t("members.tokenDurationPick") }, ...TOKEN_DURATIONS.map((value) => ({ value, label: t(`members.tokenDurations.${value}`) }))]} />
                    <SubmitRow>
                        <FormButton id={`mt-${enable}`} type="button" label={t(state.enabled ? "members.tokenExtend" : "members.tokenEnable")} isReplaced={duration !== ""} pending={pending} disabled={disabled}
                            onClick={() => apply(() => updateUserToken(member.id, { enabled: true, duration }), t("members.tokenEnabledToast", { name }))} />
                    </SubmitRow>
                </Rows>
            </Section>
            {isLive(state) && (
                <Section title={t("members.tokenValue")}>
                    <Rows>
                        <CopyField id="mt-link" label={t("members.tokenLink")} help={t("members.tokenLinkHelp")} value={origin} />
                        <CopyField id="mt-token" label={t("members.tokenValue")} help={t("members.tokenValueHelp")} value={state.token || ""} />
                    </Rows>
                </Section>
            )}
            {more && (
                <Section title={t("settings.tokenAlternative")}>
                    <Rows>{more}</Rows>
                </Section>
            )}
            <DangerSection>
                {state.enabled && (
                    <DangerRow id="mt-disable" icon="bi-toggle-off" label={t("members.tokenDisable")} help={t("members.tokenDisableHelp")} disabled={busy}
                        question={t("members.tokenDisableConfirm", { name })}
                        onConfirm={() => apply(() => updateUserToken(member.id, { enabled: false }), t("members.tokenDisabledToast", { name }))} />
                )}
                <DangerRow id="mt-regenerate" icon="bi-arrow-repeat" label={t("members.tokenRegenerate")} help={t("members.tokenRegenerateHelp")} disabled={busy}
                    question={t("members.tokenRegenerateConfirm", { name })}
                    onConfirm={() => apply(() => regenerateUserToken(member.id), t("members.tokenRegeneratedToast", { name }))} />
            </DangerSection>
            <GuideSection guides={TOKEN_GUIDES} />
        </>
    );
}

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
function MemberForm({ member: given, locked = null }) {
    const { t, i18n } = useTranslation();
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

    const problems = {
        username: editing ? "" : values.username.trim().length < 3 ? t("members.usernameMinChars", { min: 3 })
            : !/^[A-Za-z0-9._-]{3,32}$/.test(values.username.trim()) ? t("members.usernameChars", { max: 32 }) : "",
        display_name: values.display_name.length < 3 ? t("members.displayNameMinChars", { min: 3 }) : values.display_name.length > 25 ? t("members.displayNameMaxChars", { max: 25 }) : "",
    };
    // L'erreur d'un champ se montre une fois qu'on y a écrit (un champ vide n'est pas faux, il manque).
    const shownProblem = (name) => (data) => (data[name] && problems[name] ? Ruleset.isInvalid(problems[name]) : null);
    const greyed = () => Ruleset.isDisabled(null);
    const form = formContext(values, setValues, {
        username: editing ? greyed : shownProblem("username"),
        display_name: locked ? greyed : shownProblem("display_name"),
        password: greyed,
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
            toast(answer.detail || t("members.saveFailed"));

            return;
        }
        toast(t(editing ? "members.updated" : "members.created", { name: values.display_name }));

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
            toast(answer.detail || t("members.deleteFailed"));

            return;
        }
        toast(t("members.deleted", { name: memberName(member) }));
        back(MEMBERS);
    };

    // Un nouveau lien, ou la réinitialisation : le membre tel que le serveur le rend, et son lien à montrer.
    const link = async (call, done) => {
        setLinking(true);
        const answer = await call(member.id);
        setLinking(false);

        if (!answer.ok || !answer.item?.invitation?.code) {
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }
        setUpdated(withoutCode(answer.item));
        setRevealed(answer.item.invitation);
        toast(t(done, { name: memberName(answer.item) }));
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
            <SettingsPage title={memberName(member)} parent={MEMBERS}>
                <InvitationReveal link={memberLink(revealed.code)} expiresAt={revealed.expires_at} title={t("members.invitationTitle", { name: memberName(member) })} canShare={false}
                    help={t(revealed.kind === "reset" ? "members.invitationResetHelp" : "members.invitationHelp")} validity="members.invitationValidUntil" onDone={done} />
            </SettingsPage>
        );
    }

    const token = editing && member.token ? tokenStatus(t, i18n.language, member.token) : null;
    const page = (path, key) => () => open(`${MEMBERS}/${member.id}/${path}`, key, { member });

    return (
        <SettingsPage title={editing ? memberName(member) : t("members.create")} parent={MEMBERS}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (changed && !invalid && !pending && !locked) {
                    save();
                }
            } }}>
                <Section title={t("members.accountTitle")} help={locked}>
                    <Rows>
                        <TextRow id="username" label={t("members.username")} help={t(editing ? "members.usernameFixedHelp" : "members.usernameHelp")} context={form}
                            props={{ "data-key": "mb-username", autoComplete: "off" }} />
                        <TextRow id="display_name" label={t("members.displayName")} help={t("members.displayNameHelp")} context={form} props={{ "data-key": "mb-display_name" }} />
                        <SwitchRow id="mb-admin" label={t("members.roleAdmin")} help={t("members.roleHelp")} checked={admin} disabled={Boolean(locked)}
                            onChange={(on) => setValues((previous) => ({ ...previous, role_level: String(on ? ROLE_ADMIN : ROLE_USER) }))} />
                        <TextRow id="password" kind="password" label={t("members.password")} help={passwordState(t, i18n.language, editing ? member : null)} context={form}
                            props={{ autoComplete: "off", "data-key": "mb-password" }} />
                        <SubmitRow>
                            <FormButton id="mb-save" label={t(editing ? "settings.save" : "members.createSubmit")} isReplaced={Boolean(changed)} pending={pending} disabled={Boolean(locked) || Boolean(invalid)} />
                        </SubmitRow>
                    </Rows>
                </Section>
            </Form>
            {token && (
                <Section title={t("settings.items.federation.label")}>
                    <Rows>
                        <NavRow id="mb-token" icon="bi-key" tone="amber" label={t("members.tokenTitle")} help={`${token.status} · ${token.usage}`} onOpen={page("token", "mb-token")} />
                    </Rows>
                </Section>
            )}
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="mb-consoles" icon="bi-joystick" tone={editing ? "indigo" : "slate"} label={t("members.consolesTitle")}
                        help={!editing ? t("members.afterCreate") : isAdministrator(member.role_level) ? t("members.consolesAdmin") : t("members.consolesRowHelp")}
                        disabled={!editing} onOpen={editing ? page("consoles", "mb-consoles") : undefined} />
                    <NavRow id="mb-saves" icon="bi-shield-check" tone={editing ? "teal" : "slate"} label={t("members.saves")} help={t(editing ? "members.savesRowHelp" : "members.afterCreateSaves")}
                        disabled={!editing} onOpen={editing ? page("saves", "mb-saves") : undefined} />
                    {editing && awaiting(member) && (
                        <NavRow id="mb-new-link" icon="bi-link-45deg" tone="green" label={t("members.newLink")} help={t("members.newLinkHelp")} opens="action" pending={linking}
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
                    <DangerRow id={`mb-del-${member.id}`} label={t("members.deleteTitle")} help={locked || t("members.deleteHelp")} disabled={Boolean(locked)}
                        question={t("members.deleteConfirm", { name: memberName(member) })} onConfirm={remove} />
                </DangerSection>
            )}
        </SettingsPage>
    );
}

// UN MEMBRE D'UNE PAGE (la sienne, celles de son jeton, de ses consoles, de ses sauvegardes) : celui que la page d'avant a
// passé, aussitôt, puis relu — la page de son jeton, ouverte de la sienne, a pu le changer, et le retour ne rapporte que
// l'état d'avant ; un lien direct ou un rechargement n'a que la relecture. `undefined` tant qu'il se lit, `false`
// introuvable.
function useMember(memberId) {
    const location = useLocation();
    const passed = location.state?.member;
    const [member, setMember] = useState(() => (passed && String(passed.id) === memberId ? passed : undefined));

    useEffect(() => {
        let cancelled = false;
        findMember(memberId).then((found) => !cancelled && setMember((current) => found || (current === undefined ? false : current)));

        return () => {
            cancelled = true;
        };
    }, [memberId]);

    return member;
}

// Ce qu'une page d'un membre montre tant qu'il se lit (un mot seul, sans titre : rien encore n'en mérite un), ou
// quand il n'existe plus.
function MemberPending({ member, title, parent }) {
    const { t } = useTranslation();

    return (
        <SettingsPage title={title} parent={parent}>
            {member === undefined ? <Section><Note className="mt-0">{t("settings.loading")}</Note></Section> : <NotFound />}
        </SettingsPage>
    );
}

// /settings/members/<id> : la page d'un membre. Root et son propre compte s'ouvrent grisés, et disent pourquoi.
function MemberPage() {
    const { t } = useTranslation();
    const { memberId } = useParams();
    const { user } = useAuth();
    const member = useMember(memberId);

    if (!member) {
        return <MemberPending member={member} title={t("settings.items.members.label")} parent={MEMBERS} />;
    }

    return <MemberForm member={member} locked={lockReason(t, member, user)} />;
}

// Une page sous celle d'un membre (son jeton, ses consoles, ses sauvegardes) : B ramène à sa page, et le fil d'Ariane le
// nomme, même ouvert par un lien direct (sans être passé par sa page). `render(member, locked)` : son contenu.
function MemberSubPage({ title, render, needs = () => true }) {
    const { t } = useTranslation();
    const { memberId } = useParams();
    const { user } = useAuth();
    const member = useMember(memberId);
    const parent = `${MEMBERS}/${memberId}`;

    if (!member || !needs(member)) {
        return <MemberPending member={member} title={title} parent={parent} />;
    }

    return (
        <SettingsPage title={title} parent={parent} levels={{ [parent]: memberName(member) }}>
            {render(member, lockReason(t, member, user))}
        </SettingsPage>
    );
}

// /settings/members/<id>/token : le jeton d'un membre.
function MemberTokenPage() {
    const { t } = useTranslation();

    return (
        <MemberSubPage title={t("members.tokenTitle")} needs={(member) => Boolean(member.token)} render={(member, locked) => <MemberToken member={member} locked={locked} />} />
    );
}

// /settings/members/<id>/consoles : CE QU'IL PEUT LANCER (le porteur, 2026-11-14 : « All rétro, streamex et remoteplay
// doit être un lien dans more qui ouvre la page des "Console access" ») — tout le rétro, puis chaque console récente,
// puis chaque console Remote Play, un interrupteur chacune ; « * » quand tout est coché (formToGrants), ce que lit le
// kernel. Un administrateur a toutes les consoles : ses interrupteurs se montrent tous allumés, et grisés.
//
// CHAQUE INTERRUPTEUR S'ENREGISTRE À SON CHANGEMENT (le porteur, 2026-11-16 : « Dans membres/accès aux consoles, retirer
// le bouton de save et faire en sorte qu'on sauvegarde au changement des switches ») — le modèle de Réglages › Journaux :
// plus de bouton Enregistrer ; ce qu'on touche part aussitôt (tous les droits du compte, `PATCH /auth/users/{id}`), et
// revient en arrière, avec un avis, si le serveur refuse. Les envois partent l'un après l'autre (`queue`), dans l'ordre des
// gestes : le dernier dit l'état final ; un refus rend celui que le serveur a gardé (`saved`).
function MemberConsoles({ member, locked }) {
    const { t } = useTranslation();
    const { consoles } = useData();
    const streamex = (consoles || []).filter((entry) => entry.method === "streamex");
    const remoteplay = (consoles || []).filter((entry) => entry.method === "remoteplay");
    const streamexIds = streamex.map((entry) => entry.id);
    const remoteplayIds = remoteplay.map((entry) => entry.id);
    const admin = isAdministrator(member.role_level);
    // Le registre des consoles peut arriver après la page : tant que rien n'est touché, les cases suivent le compte.
    const [touched, setTouched] = useState(null);
    const saved = useRef(member.consoles);
    const queue = useRef(Promise.resolve());
    const values = touched || grantsToForm(member.consoles, streamexIds, remoteplayIds);
    const shown = admin ? { retro: true, streamex: streamexIds, remoteplay: remoteplayIds } : values;
    const greyed = admin || Boolean(locked) || consoles === null;

    const change = (patch) => {
        const next = { ...values, ...patch };
        const grants = formToGrants(next, streamexIds, remoteplayIds);
        setTouched(next);
        queue.current = queue.current.then(async () => {
            const answer = await updateUser(member.id, { consoles: grants });

            if (answer.ok) {
                saved.current = grants;

                return;
            }
            setTouched(grantsToForm(saved.current, streamexIds, remoteplayIds));
            toast(answer.detail || t("members.saveFailed"));
        });
    };
    const toggle = (kind, id) => (on) => change({ [kind]: on ? [...values[kind], id] : values[kind].filter((entry) => entry !== id) });

    const consoleRows = (kind, entries) => entries.map((entry) => (
        <SwitchRow key={entry.id} id={`mb-${kind}-${entry.id}`} label={entry.name} checked={shown[kind].includes(entry.id)} disabled={greyed} onChange={toggle(kind, entry.id)} />
    ));

    return (
        <>
            <Section title={t("tab.retrogaming")} help={[locked, t(admin ? "members.consolesAdmin" : "members.consolesHelp")].filter(Boolean).join(" ")}>
                <Rows>
                    <SwitchRow id="mb-retro" label={t("members.consolesRetro")} help={t("members.consolesRetroHelp")} checked={shown.retro} disabled={greyed}
                        onChange={(retro) => change({ retro })} />
                </Rows>
            </Section>
            {streamex.length > 0 && (
                <Section title={t("tab.streamex")} help={t("members.consolesStreamexHelp")}>
                    <Rows>{consoleRows("streamex", streamex)}</Rows>
                </Section>
            )}
            {remoteplay.length > 0 && (
                <Section title={t("tab.remoteplay")} help={t("members.consolesRemoteplayHelp")}>
                    <Rows>{consoleRows("remoteplay", remoteplay)}</Rows>
                </Section>
            )}
        </>
    );
}

function MemberConsolesPage() {
    const { t } = useTranslation();

    return <MemberSubPage title={t("members.consolesTitle")} render={(member, locked) => <MemberConsoles member={member} locked={locked} />} />;
}

// UN TÉLÉCHARGEMENT, comme le lien qu'il était (un onglet à part, que le serveur sert en pièce jointe) : une ligne
// n'est pas un lien, elle en ouvre un (le même geste que GameSettings.jsx).
const openDownload = (href) => {
    const link = document.createElement("a");
    link.href = href;
    link.target = "_blank";
    link.rel = "noreferrer";
    document.body.appendChild(link);
    link.click();
    link.remove();
};

// /settings/members/<id>/saves : SES SAUVEGARDES (storage/Saves.jsx de la webapp ; le porteur, 2026-11-14 : « rajoute
// aussi un lien pour voir les sauvegardes de l'utilisateur (ce qui fait qu'on enlève "user's saves" dans settings) ») —
// de loin, volontairement : la place qu'occupe son dossier (`saves/<identifiant>`), et deux gestes — l'emporter
// (l'archive), ou la reprendre (tout supprimer ; le serveur efface en fond, d'où « lancée »). Elles ne dépendent pas du
// compte : la page s'ouvre aussi pour root et pour soi.
function MemberSaves({ member }) {
    const { t } = useTranslation();
    const { back } = useSettingsNavigation();
    const [saves, setSaves] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        fetchUserSaves().then((answer) => {
            if (cancelled) {
                return;
            }

            if (!answer.ok) {
                toast(answer.errorMessage || t("settings.loadFailed"));
            }
            setSaves((answer.data?.users || []).find((entry) => entry.username === member.username) || null);
        });

        return () => {
            cancelled = true;
        };
    }, [member.username, t]);

    if (saves === undefined) {
        return <Section><Note className="mt-0">{t("settings.loading")}</Note></Section>;
    }
    const none = !saves || saves.files === 0;
    const unavailable = none ? t("members.savesNone") : null;

    return (
        <>
            <Section title={t("details")}>
                <Rows>
                    <InfoRow icon="bi-files" label={t("members.savesFiles")} value={String(saves?.files || 0)} />
                    <InfoRow icon="bi-hdd" label={t("members.savesSize")} value={formatSize(saves?.sizeBytes || 0)} />
                    {saves?.deleting && <InfoRow icon="bi-hourglass-split" label={t("members.savesState")} value={t("settings.savesDeleting")} />}
                </Rows>
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="saves-download" icon="bi-download" tone={none ? "slate" : "indigo"} label={t("settings.download")} help={unavailable || t("settings.savesDownloadHelp")} opens="action"
                        disabled={none} onOpen={() => openDownload(userSavesArchiveUrl(member.username))} />
                </Rows>
            </Section>
            {/* Rien à supprimer, ou un effacement en cours : grisée, et pourquoi. */}
            <DangerSection id="saves-delete" help={saves?.deleting ? t("settings.savesDeleting") : unavailable || t("settings.deleteSavesHelp")} label={t("settings.deleteAll")}
                question={t("settings.deleteSavesConfirm")} disabled={none || Boolean(saves?.deleting)} onConfirm={async () => {
                    const answer = await deleteUserSaves(member.username);

                    if (!answer.ok) {
                        toast(answer.errorMessage || t("settings.actionFailed"));

                        return;
                    }
                    toast(t(answer.data?.finished ? "settings.savesDeleted" : "settings.savesDeleteStarted", { user: member.username }));
                    back(`${MEMBERS}/${member.id}`);
                }} />
        </>
    );
}

function MemberSavesPage() {
    const { t } = useTranslation();

    return <MemberSubPage title={t("members.saves")} render={(member) => <MemberSaves member={member} />} />;
}

// LA LISTE (24-membres, 41-membres des modèles) : « Les membres » — Listing (Bricks.jsx), que le SERVEUR feuillette
// (`total`) : la recherche et son filtre accolé, le nombre trouvé, les membres de la page, leur pagination ; le temps
// d'une lecture, autant de lignes fantômes que de membres montrés (une page à la première) ; puis « Plus » : « Nouveau
// membre » (le porteur, 2026-11-14 : « le bouton "new member" doit être dans section more, plus bas. Et enlever la section
// filtre. »). Une ligne : son nom, le badge vert d'un administrateur, « @identifiant » dessous — sans le rôle.
//
// SES FILTRES, dans le panneau du bouton accolé à la recherche (Listing ; le porteur : « Le filtre doit être comme sur
// Webapp, un bouton en input-group […] avec les deux rôles en switches. Puis on doit avoir la possibilité de trier par
// token enabled et par utilisateur qui ont validé leur invitation. », puis « Remplace le modal filtre par un offcanvas »)
// : les deux rôles (les administrateurs, root compris ; les joueurs), le jeton activé — chacun s'applique aussitôt et va
// dans l'adresse ; comme les cases de la webapp, rien de coché veut dire tous, des rôles cochés s'additionnent, le jeton
// restreint encore, et « Invitation validée » aussi : ceux qui ont choisi leur mot de passe par leur lien (chantier 55,
// `password_set_at`, que le serveur cherche).
function MembersPage() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState();
    const search = read("q") || "";
    const filters = { admins: read("admins") === "1", players: read("players") === "1", token: read("token") === "1", invited: read("invited") === "1" };
    const page = Math.max(1, Number(read("page")) || 1);
    const [items, setItems] = useState([]);
    const [count, setCount] = useState(0);
    const [loading, setLoading] = useState(true);
    const filterKey = `${search}|${filters.admins}|${filters.players}|${filters.token}|${filters.invited}`;
    const lastFilter = useRef(null);
    const request = useRef(0);
    const setPage = (next) => update({ page: next > 1 ? next : null });

    // La recherche du serveur : un groupe par champ cherché, chacun avec le filtre de rôle et celui de l'invitation
    // (`password_set_at` connu) ; le jeton à part (`tokenEnabled`), le serveur le joint à ses comptes.
    const fetchPage = async (target) => {
        const ticket = ++request.current;
        setLoading(true);
        const needle = search.trim();
        const levels = [...(filters.admins ? [ROLE_ROOT, ROLE_ADMIN] : []), ...(filters.players ? [ROLE_USER] : [])];
        const level = levels.length > 0 && levels.length < 3 ? { role_level: { in: levels } } : null;
        const restrict = level || filters.invited ? { ...level, ...(filters.invited ? { password_set_at: { not_null: true } } : null) } : null;
        const groups = needle ? [{ username: { like: needle }, ...restrict }, { display_name: { like: needle }, ...restrict }] : restrict ? [restrict] : [];
        const answer = await listUsers({ page: target, itemsPerPage: PAGE_SIZE, search: groups.length ? JSON.stringify(groups) : "", tokenEnabled: filters.token });

        if (ticket !== request.current) {
            return;
        }
        setLoading(false);

        if (answer.ok) {
            const total = answer.pagination?.count ?? (answer.items || []).length;
            setItems(answer.items || []);
            setCount(total);

            // Une page de l'adresse au-delà de la dernière (un membre supprimé depuis) : la dernière.
            if (!(answer.items || []).length && target > 1 && total > 0) {
                setPage(Math.ceil(total / PAGE_SIZE));
            }
        } else {
            toast(answer.detail || t("settings.loadFailed"));
        }
    };

    // Une lettre tapée ou un filtre posé relit un instant après ; une page, aussitôt.
    useEffect(() => {
        const delay = lastFilter.current === null || lastFilter.current === filterKey ? 0 : 300;
        lastFilter.current = filterKey;
        const timer = setTimeout(() => fetchPage(page), delay);

        return () => clearTimeout(timer);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [filterKey, page]);

    const active = Object.values(filters).filter(Boolean).length;
    const toggle = (name) => (on) => update({ [name]: on ? 1 : null, page: null });

    return (
        <SettingsPage title={t("settings.items.members.label")}>
            <Section title={t("members.listTitle")}>
                {/* Chaque membre ouvre sa page — root et son propre compte aussi, grisés là-bas. */}
                <Listing items={items} total={count} loading={loading} pageSize={PAGE_SIZE} page={page} onPage={setPage}
                    search={search} onSearch={(q) => update({ q: q || null, page: null })} placeholder={t("members.searchPlaceholder")} searchKey="mb-search"
                    filterCount={active} filterTitle={t("members.filterTitle")} filterHelp={t("members.filterHelp")} filters={(
                        <>
                            <SwitchRow id="mf-admins" label={t("members.filterAdmins")} help={t("members.filterAdminsHelp")} checked={filters.admins} onChange={toggle("admins")} />
                            <SwitchRow id="mf-players" label={t("members.filterPlayers")} help={t("members.filterPlayersHelp")} checked={filters.players} onChange={toggle("players")} />
                            <SwitchRow id="mf-token" label={t("members.filterToken")} help={t("members.filterTokenHelp")} checked={filters.token} onChange={toggle("token")} />
                            <SwitchRow id="mf-invited" label={t("members.filterInvited")} help={t("members.filterInvitedHelp")} checked={filters.invited} onChange={toggle("invited")} />
                        </>
                    )}
                    found={t("members.found", { count })} empty={t(active > 0 ? "members.noMatch" : "members.empty")} noMatch={t("members.noMatch")} placeholderRow={() => <PlaceholderRow lead="avatar" />}
                    render={(member) => (
                        <NavRow key={member.id} id={`mb-edit-${member.id}`} lead={<Avatar name={memberName(member)} />} label={<AccountName name={memberName(member)} admin={isAdministrator(member.role_level)} />}
                            help={atUsername(member.username)} onOpen={() => open(`${MEMBERS}/${member.id}`, `mb-edit-${member.id}`, { member })} />
                    )} />
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="mb-create" icon="bi-person-plus" tone="green" label={t("members.create")} help={t("members.createHelp")} onOpen={() => open(`${MEMBERS}/new`, "mb-create")} />
                </Rows>
            </Section>
        </SettingsPage>
    );
}

export function MembersRoutes() {
    return (
        <Routes>
            <Route index element={<MembersPage />} />
            <Route path="new" element={<MemberForm member={null} />} />
            <Route path=":memberId" element={<MemberPage />} />
            <Route path=":memberId/token" element={<MemberTokenPage />} />
            <Route path=":memberId/consoles" element={<MemberConsolesPage />} />
            <Route path=":memberId/saves" element={<MemberSavesPage />} />
            <Route path="*" element={<Navigate to={MEMBERS} replace />} />
        </Routes>
    );
}
