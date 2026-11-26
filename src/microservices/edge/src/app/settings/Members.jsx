// Route : /settings/members (la liste, `Members`) ; ce fichier tient aussi ce que les pages de settings/members/
// partagent. LES MEMBRES (admin/Members.jsx et api/users.js de la webapp ; 24-membres et 25-membre des modèles) : la
// liste paginée par le serveur, cherchée par nom, filtrée dans sa fenêtre (/settings/members, `?q=&admins=&players=&
// token=&page=`) ; un membre s'ouvre sur sa page (/settings/members/<id>) — son compte (identifiant, nom affiché, rôle,
// mot de passe, dans un seul groupe), puis « Plus » : ses consoles (…/<id>/consoles) et ses sauvegardes
// (…/<id>/saves) — et le supprimer, dans « Danger » ; on en crée un (/settings/members/new), en bas de la liste, dans
// « Plus ». SON MOT DE PASSE, IL LE CHOISIT (chantier 55) : l'administrateur reçoit un lien à lui envoyer, à la
// création, puis « Nouveau lien » tant qu'il attend, « Réinitialiser le mot de passe » une fois qu'il a rejoint. Une ligne de la liste n'a pas de geste : elle ouvre la page (le porteur, 2026-11-13). SON JETON N'EST PLUS
// ICI (le porteur, 2026-11-20 : « L'admin peut contrôler les accès aux consoles. C'est pour cette raison que ça ne sert à
// rien l'accès au token ») : chacun active le sien dans Réglages › Profil › Fédération (profile/federation/Token.jsx). Root et son propre compte ne se gèrent pas ici : leurs pages s'ouvrent grisées, et disent
// pourquoi.

import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation, useParams } from "react-router-dom";
import { AccountName } from "../bricks/AccountName.jsx";
import { GuideSection } from "../bricks/GuideSection.jsx";
import { Listing } from "../bricks/Listing.jsx";
import { Loading } from "../bricks/Loading.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { PlaceholderRow } from "../bricks/PlaceholderRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { SaveSection } from "../bricks/SaveSection.jsx";
import { Section } from "../bricks/Section.jsx";
import { SwitchRow } from "../bricks/SwitchRow.jsx";
import { atUsername, isAdministrator } from "../globals/accounts.js";
import { fetchMfaPolicy, listUsers, setMfaPolicy } from "../libs/api.js";
import { useAuth, useData } from "../libs/data.jsx";
import { dateTime } from "../libs/format.js";
import { toast } from "../shell/Shell.jsx";
import SettingsPage, { NotFound, useQueryState, useSettingsNavigation } from "./SettingsPage.jsx";
import { pill } from "../bricks/Pill.jsx";

export const ROLE_ROOT = 0;
export const ROLE_ADMIN = 1;
export const ROLE_USER = 2;
export const PAGE_SIZE = 10;
export const GRANT_ALL = "*";
export const GRANT_RETRO = "retro";
export const MEMBERS = "/settings/members";
// Le guide des membres, en bas de chacune de leurs pages : créer un compte, et le garder sûr.
export const MEMBERS_GUIDES = [["install", "6-sign-in"], ["remoteAccess", "keep-it-safe"]];

export const day = (iso, language) => (iso ? new Date(iso).toLocaleDateString(language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium" }) : "—");

// LE MOT DE PASSE D'UN MEMBRE, CE QU'IL EN EST (chantier 55) : l'aide de sa ligne « Mot de passe », la même sur les deux
// pages — un nouveau membre le choisira par son lien ; un membre l'a choisi tel jour, ou l'attend, avec son lien (jusqu'à
// quand, créé par qui) ou sans lien valable.
export const awaiting = (member) => !member.password_set_at;
// Son jeton ouvre-t-il encore une session ici : activé, et pas échu.
export const tokenLive = (token) => Boolean(token?.enabled && token.expires_at && new Date(token.expires_at) > new Date());

// LA LIGNE D'UN MEMBRE (le porteur, 2026-11-20 : « @username • signup (in)completed • token (de)activated ») : son
// identifiant, où en est son inscription, et si son jeton vaut.
// « @identifiant • inscription en attente », sinon « @identifiant • jeton activé / désactivé » (un listing : « a • b »).
export const memberSummary = (t, member) => [atUsername(member.username), awaiting(member) ? pill(t("members.passwordPending"), "warning")
    : pill(t(tokenLive(member.token) ? "members.tokenOnShort" : "members.tokenOffShort"), tokenLive(member.token) ? "success" : "secondary")];
export const passwordState = (t, language, member) => {
    if (!member) {
        return t("members.passwordByLink");
    }

    if (!awaiting(member)) {
        return t("members.passwordChosen", { at: day(member.password_set_at, language) });
    }
    const link = member.invitation;

    return link
        ? [t("members.passwordPending"), t("members.passwordLink", { at: dateTime(link.expires_at, language), by: link.created_by ? atUsername(link.created_by) : "—" })]
        : [t("members.passwordPending"), t("members.passwordNoLink")];
};

// La réponse d'une création, d'un nouveau lien ou d'une réinitialisation, sans le code : il ne se montre qu'une fois.
export const withoutCode = (member) => (member?.invitation ? { ...member, invitation: { ...member.invitation, code: undefined } } : member);

// Les droits du compte en cases du formulaire, et retour (« * » quand tout est coché).
export const grantsToForm = (grants, streamexIds, remoteplayIds) => {
    const granted = grants || [];
    const all = granted.includes(GRANT_ALL);

    return {
        retro: all || granted.includes(GRANT_RETRO),
        streamex: all ? streamexIds : streamexIds.filter((id) => granted.includes(id)),
        remoteplay: all ? remoteplayIds : remoteplayIds.filter((id) => granted.includes(id)),
    };
};
export const formToGrants = (values, streamexIds, remoteplayIds) => (values.retro && streamexIds.every((id) => values.streamex.includes(id)) && remoteplayIds.every((id) => values.remoteplay.includes(id))
    ? [GRANT_ALL]
    : [...(values.retro ? [GRANT_RETRO] : []), ...streamexIds.filter((id) => values.streamex.includes(id)), ...remoteplayIds.filter((id) => values.remoteplay.includes(id))]);

// UN MEMBRE PAR SON IDENTIFIANT, pour une page ouverte par un lien direct ou relue : la route des comptes ne se
// cherche pas par identifiant (username, display_name, role_level seulement) ; ses pages vont dans l'ordre
// des identifiants, 100 par 100 — on s'arrête à la page qui le dépasse.
export async function findMember(id) {
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
export const memberName = (member) => member.display_name || member.username;

// Pourquoi ce compte ne se gère pas d'ici (root, son propre compte), ou rien. LA RAISON EST L'AIDE DU PREMIER GROUPE
// de la page grisée (le compte, le détail du jeton, le rétro) — plus une alerte en tête (le porteur, 2026-11-16 :
// « retirer les alerts »).
export const lockReason = (t, member, user) => (member.role_level === ROLE_ROOT ? t("members.rootLocked") : member.id === user?.id ? t("members.ownLocked") : null);

// UN MEMBRE D'UNE PAGE (la sienne, celles de son jeton, de ses consoles, de ses sauvegardes) : celui que la page d'avant a
// passé, aussitôt, puis relu — la page de son jeton, ouverte de la sienne, a pu le changer, et le retour ne rapporte que
// l'état d'avant ; un lien direct ou un rechargement n'a que la relecture. `undefined` tant qu'il se lit, `false`
// introuvable.
export function useMember(memberId) {
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
export function MemberPending({ member, title, parent }) {
    const { t } = useTranslation();

    return (
        <SettingsPage title={title} parent={parent} guides={MEMBERS_GUIDES}>
            {member === undefined ? <Section><Loading /></Section> : <NotFound />}
        </SettingsPage>
    );
}

// Une page sous celle d'un membre (son jeton, ses consoles, ses sauvegardes) : B ramène à sa page, et le fil d'Ariane le
// nomme, même ouvert par un lien direct (sans être passé par sa page). `render(member, locked)` : son contenu.
export function MemberSubPage({ title, render, needs = () => true }) {
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
            <GuideSection guides={MEMBERS_GUIDES} />
        </SettingsPage>
    );
}

// LES INTERRUPTEURS DES CONSOLES — tout le rétro, chaque console streamex, chaque console Remote Play —, écrits UNE fois
// pour un membre et pour un accès délivré (chantier 66 ; le porteur, 2026-11-19 : « un interrupteur pour tous les rétros,
// chaque console streamex et remoteplay PS4 et PS5 (comme dans Settings/membres/accès aux consoles) »).
// UN BOUTON PAR SECTION (le porteur, 2026-11-25 : « Ça devrait aussi être un bouton par section au lieu de "onChange
// save" […] Avec bouton ça permet de savoir si il y a un problème ou pas pendant la sauvegarde vu qu'il y aura des
// feedbacks » ; remplace l'enregistrement au changement du 2026-11-16) : chaque section est une SaveSection, et
// n'enregistre que ses consoles — les autres partent telles qu'elles sont enregistrées (`save(grants)` → {ok, detail}),
// la réponse en fenêtre. `all` : tout allumé et grisé (un administrateur) ; `locked` : la raison d'une page
// grisée.
export function ConsoleGrantsSections({ id, grants, all = false, locked = null, help, save }) {
    const { t } = useTranslation();
    const { consoles } = useData();
    const streamex = (consoles || []).filter((entry) => entry.method === "streamex");
    const remoteplay = (consoles || []).filter((entry) => entry.method === "remoteplay");
    const streamexIds = streamex.map((entry) => entry.id);
    const remoteplayIds = remoteplay.map((entry) => entry.id);
    const [savedGrants, setSavedGrants] = useState(grants);
    const saved = grantsToForm(savedGrants, streamexIds, remoteplayIds);
    const greyed = all || Boolean(locked) || consoles === null;
    // Une console par clé, allumée ou non : l'ordre des gestes ne compte pas.
    const switches = (kind, entries) => Object.fromEntries(entries.map((entry) => [entry.id, all || saved[kind].includes(entry.id)]));

    const saveSection = async (patch) => {
        const asked = formToGrants({ ...saved, ...patch }, streamexIds, remoteplayIds);
        const answer = await save(asked);

        if (answer.ok) {
            setSavedGrants(asked);
        }

        return answer;
    };
    const consoleSection = (kind, entries, title, sectionHelp) => entries.length > 0 && (
        <SaveSection id={`${id}-${kind}`} title={title} help={sectionHelp} saved={switches(kind, entries)} disabled={greyed}
            onSave={(values) => saveSection({ [kind]: entries.filter((entry) => values[entry.id]).map((entry) => entry.id) })}>
            {(values, set) => entries.map((entry) => (
                <SwitchRow key={entry.id} id={`${id}-${kind}-${entry.id}`} label={entry.name} texts="allowed" checked={values[entry.id]} disabled={greyed}
                    onChange={(on) => set({ [entry.id]: on })} />
            ))}
        </SaveSection>
    );

    return (
        <>
            <SaveSection id={`${id}-retro`} title={t("tab.retrogaming")} help={[locked, help].filter(Boolean).join(" ")} saved={{ retro: all || saved.retro }}
                disabled={greyed} onSave={({ retro }) => saveSection({ retro })}>
                {(values, set) => (
                    <SwitchRow id={`${id}-retro-on`} label={t("members.consolesRetro")} texts="allowed" help={t("members.consolesRetroHelp")} checked={values.retro} disabled={greyed}
                        onChange={(retro) => set({ retro })} />
                )}
            </SaveSection>
            {consoleSection("streamex", streamex, t("tab.streamex"), t("members.consolesStreamexHelp"))}
            {consoleSection("remoteplay", remoteplay, t("tab.remoteplay"), t("members.consolesRemoteplayHelp"))}
        </>
    );
}

// Route : /settings/members — LA LISTE (24-membres, 41-membres des modèles) : « Les membres » — Listing (bricks/), que le SERVEUR feuillette
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
export default function Members() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    // IMPOSÉE À TOUT LE SERVEUR (chantier 80) : un serveur seulement — le téléphone répond 404, et la section n'existe
    // pas chez lui (une capacité de l'hôte, la seule exception à « désactivé plutôt que caché »).
    const [policy, setPolicy] = useState(null);
    useEffect(() => {
        fetchMfaPolicy().then((answer) => setPolicy(answer.ok ? Boolean(answer.item?.required_all) : null));
    }, []);
    const changePolicy = async ({ all }) => {
        const answer = await setMfaPolicy(all);

        if (answer.ok) {
            setPolicy(Boolean(answer.item?.required_all));
        }

        return answer;
    };
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
            <Section title={t("members.listTitle")} help={t("members.listHelp")} sense="info">
                {/* Chaque membre ouvre sa page — root et son propre compte aussi, grisés là-bas. */}
                <Listing items={items} total={count} loading={loading} pageSize={PAGE_SIZE} page={page} onPage={setPage}
                    search={search} onSearch={(q) => update({ q: q || null, page: null })} placeholder={t("members.searchPlaceholder")} searchKey="mb-search"
                    filterCount={active} filters={[
                        {
                            key: "roles", title: t("members.filterRoles"), rows: (
                                <>
                                    <SwitchRow id="mf-admins" label={t("members.filterAdmins")} help={t("members.filterAdminsHelp")} checked={filters.admins} onChange={toggle("admins")} />
                                    <SwitchRow id="mf-players" label={t("members.filterPlayers")} help={t("members.filterPlayersHelp")} checked={filters.players} onChange={toggle("players")} />
                                </>
                            ),
                        },
                        {
                            key: "account", title: t("members.accountTitle"), rows: (
                                <>
                                    <SwitchRow id="mf-token" label={t("members.filterToken")} help={t("members.filterTokenHelp")} checked={filters.token} onChange={toggle("token")} />
                                    <SwitchRow id="mf-invited" label={t("members.filterInvited")} help={t("members.filterInvitedHelp")} checked={filters.invited} onChange={toggle("invited")} />
                                </>
                            ),
                        },
                    ]}
                    found={t("members.found", { count })} empty={t(active > 0 ? "members.noMatch" : "members.empty")} noMatch={t("members.noMatch")} placeholderRow={() => <PlaceholderRow />}
                    render={(member) => (
                        <NavRow key={member.id} id={`mb-edit-${member.id}`} label={<AccountName name={memberName(member)} admin={isAdministrator(member.role_level)} />}
                            help={memberSummary(t, member)} onOpen={() => open(`${MEMBERS}/${member.id}`, `mb-edit-${member.id}`, { member })} />
                    )} />
            </Section>
            {policy !== null && (
                <SaveSection id="mb-mfa" title={t("mfa.serverTitle")} help={t("mfa.serverHelp")} saved={{ all: policy }} onSave={changePolicy}
                    success={({ all }) => t(all ? "mfa.allRequiredToast" : "mfa.allFreedToast")}>
                    {(values, set) => <SwitchRow id="mb-mfa-all" label={t("mfa.requireAll")} help={t("mfa.requireAllHelp")} checked={values.all} onChange={(all) => set({ all })} />}
                </SaveSection>
            )}
            <Section title={t("more")} help={t("members.createSectionHelp")} sense="add">
                <Rows>
                    <NavRow id="mb-create" icon="bi-person-plus" label={t("members.create")} help={t("members.createHelp")} onOpen={() => open(`${MEMBERS}/new`, "mb-create")} />
                </Rows>
            </Section>
            <GuideSection guides={MEMBERS_GUIDES} />
        </SettingsPage>
    );
}
