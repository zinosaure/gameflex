// Route : /settings/federation (l'index, `Federation`) ; ce fichier tient aussi ce que les pages de settings/federation/
// partagent.
// LA FÉDÉRATION, CÔTÉ ADMINISTRATEUR : CRÉER LA CONNEXION ENTRE SERVEURS, ET RIEN D'AUTRE (le porteur, 2026-11-16 :
// « Dans Settings/fédération il faut qu'en premier lieu on voit les 2 liens: delivered access et les joined servers » ;
// « dans fédération (admin) c'est pour créer la connexion seulement » — jouer chez un serveur fédéré, avec son compte de
// là-bas, est l'affaire de chaque utilisateur, dans Réglages › Profil › Fédération, MyFederation.jsx) :
//
//   /settings/federation                  deux lignes — « Accès délivrés », « Serveurs rejoints » — et le guide ;
//   …/access                              les accès délivrés (les jetons de catalogue émis ici, si l'hôte en émet,
//                                         `issuesCatalogTokens`) ; « Plus » : « Créer un nouvel accès » (…/new-token :
//                                         une invitation, ou un jeton montré une seule fois), « Code d'appairage »
//                                         (…/new-pairing : le mode serveur et un code de six chiffres, chantier 60)
//                                         et « Accès délivrés en attente » (…/pending : les invitations pas encore
//                                         échangées) ;
//   …/tokens/<id>                         un accès : ce qu'il ouvre, partager aussi les consoles ou les fournisseurs de
//                                         métadonnées, le révoquer ;
//   …/invitations/<id>                    une invitation en attente : la révoquer ;
//   …/servers                             les serveurs rejoints ; « Plus » : « Rejoindre un serveur » (…/add : son
//                                         invitation, lien ou QR, son code d'appairage, ou son lien et son jeton,
//                                         vérifiés avant d'être enregistrés) ;
//   …/servers/<id>                        un serveur, en sections d'informations comme l'engrenage d'un jeu (2.8) : le
//                                         serveur, sa machine, ce que son jeton nous ouvre ; le retirer.
//
// UNE LIGNE DE LISTE N'A PAS DE GESTE (le porteur, 2026-11-13) : elle ouvre la page de ce qu'elle montre,
// où sont ses gestes, chacun dans sa section ; retirer ou révoquer est une ligne de « Danger », en bas de la
// page. Rejoindre, donner accès, « J'ai copié » : des lignes du groupe « Plus », pas des boutons
// (« Les boutons orphelins […] ne doivent pas être des boutons mais des liens » ; 45 et 46 des modèles) ; le
// bouton d'un formulaire est dans l'encadré de son dernier groupe. Les pages se composent des briques de la
// direction C (bricks/). Une page d'ajout revient à la liste une fois son travail fait, une page de détail
// aussi après un retrait ; la liste se relit en s'ouvrant. Une page ouverte par un lien direct relit la liste
// et y cherche son objet.
//
// CE QU'EST UN SERVEUR FÉDÉRÉ SE DIT DANS LES AIDES DES SECTIONS ET DANS « GUIDE » (le porteur, 2026-11-16, 3.1 ; « Il
// faut aussi enlever l'alert dans fédération ») — plus d'encadré `alert` : un serveur fédéré, c'est l'administrateur
// qui l'ajoute, ou qui donne accès au sien. Le compte fédéré, et ce qu'il faut pour jouer, se disent dans le Profil.
//
// LES LISTES LUES EN ASYNCHRONE se feuillettent (Listing ; le porteur, 2026-11-14 : « Rajouter des pagination pour les
// listes asynchrone ») : les serveurs, les jetons, et les invitations en attente.

import { useFeedback } from "../bricks/useFeedback.jsx";
import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useLocation } from "react-router-dom";
import { DangerSection } from "../bricks/DangerSection.jsx";
import { GuideSection } from "../bricks/GuideSection.jsx";
import { InfoRow } from "../bricks/InfoRow.jsx";
import { Loading } from "../bricks/Loading.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { SaveSection } from "../bricks/SaveSection.jsx";
import { Section } from "../bricks/Section.jsx";
import { SwitchRow } from "../bricks/SwitchRow.jsx";
import { TextRow } from "../bricks/TextRow.jsx";
import { listCatalogTokens, listInvitations, listServerRegistry } from "../libs/api.js";
import { can, useData } from "../libs/data.jsx";
import { counted, fact } from "../libs/format.js";
import { useRemoteAdmin } from "../libs/remoteAdmin.js";
import { DiscoveryModeSection, usePairing } from "../screens/access/Pairing.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "./SettingsPage.jsx";
import { FEDERATION_GUIDES } from "./profile/Federation.jsx";
import { CONSOLE_ICON } from "../globals/icons.js";

// Les chemins des pages de la fédération.
export const FEDERATION = "/settings/federation";
export const ACCESSES = `${FEDERATION}/access`;
export const PENDING = `${FEDERATION}/pending`;
export const JOINED = `${FEDERATION}/servers`;
export const at = (sub) => `${FEDERATION}/${sub}`;

// Les niveaux sous la Fédération, nommés pour le fil d'Ariane même par un lien direct (SettingsPage, `levels`).
export function useLevels() {
    const { t } = useTranslation();

    return { [ACCESSES]: t("fed.accessesTitle"), [PENDING]: t("fed.pendingTitle"), [JOINED]: t("fed.joinedTitle") };
}

export const dateTime = (iso, language) => (iso ? new Date(iso).toLocaleString(language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "");

// Ce que le listing d'un serveur dit de lui : sa date, sa version, sa machine, ses consoles.
export function describeListing(listing, t, language) {
    if (!listing) {
        return t("fed.noListing");
    }
    const gigabytes = (bytes) => Math.round(bytes / 2 ** 30);
    const machine = listing.machine || {};

    return [
        t("fed.listedAt", { at: dateTime(listing.updatedAt, language) }),
        listing.server?.version && `GameFlex ${listing.server.version}`,
        machine.device, machine.cpu,
        machine.ramBytes && t("fed.ram", { gb: gigabytes(machine.ramBytes) }),
        ...(machine.gpus || []).map((gpu) => (gpu.vramBytes ? t("fed.vram", { name: gpu.name, gb: gigabytes(gpu.vramBytes) }) : gpu.name)),
    ].filter(Boolean).join(" · ");
}

// Les droits d'un jeton émis ici, et leurs noms.
export const scopesOf = (item) => String(item.scopes || "").split(",").map((scope) => scope.trim()).filter(Boolean);
// CE QU'UN JETON PARTAGE EN PLUS DES JEUX (le catalogue, toujours) : l'option du chantier 57, `#719` — les fournisseurs de
// métadonnées (`metadata-providers`) : avec elle, la recherche des fiches d'ici interroge aussi nos comptes IGDB et
// SteamGridDB pour son porteur ; nos clés ne sortent jamais. Coupée par défaut. Les consoles ne sont plus une portée
// (chantier 66) : chaque accès a sa liste, comme un membre (« Accès aux consoles »).
export const PROVIDERS = "metadata-providers";
export const scopesFor = ({ providers }) => ["catalog", ...(providers ? [PROVIDERS] : [])];
// CE QU'UN ACCÈS LAISSE LANCER, en un mot (chantier 66) : toutes, aucune, ou le nombre de ses droits (« tout le rétro »
// en est un). Nul : non dit.
export const consolesSummary = (granted, t) => {
    if (!Array.isArray(granted)) {
        return null;
    }

    return granted.includes("*") ? t("fed.consolesAll") : granted.length === 0 ? t("fed.consolesNone") : t("fed.consolesSome");
};
// Ce qu'ouvre une invitation (« Jeux · Consoles »), sous elle dans la liste des invitations en attente. Retiré par erreur
// avec les sous-titres des accès (f786ab1d) : la liste des invitations s'effondrait dès qu'une attendait.
export const grantsOf = (item, t) => scopesOf(item).map((scope) => t(`fed.scope.${scope}`, { defaultValue: scope }));
// Le sous-titre d'un accès dans la liste : sa date de dernière utilisation, seule (le porteur, 2026-11-16 : « les
// sous-titres des serveurs fédérés ou accès délivrés sont trop verbeux. Mets juste la date ça suffit »).
export const lastUseOf = (item, t, language) => (item.last_used_at ? dateTime(item.last_used_at, language) : t("fed.neverUsed"));

// L'OBJET D'UNE PAGE DE DÉTAIL : celui que la liste a passé (`state[name]`), sinon relu par `load` (un lien
// direct, un rechargement) ; `false` quand il n'existe plus.
export function usePassed(name, id, load) {
    const location = useLocation();
    const passed = location.state?.[name];
    const [item, setItem] = useState(() => (passed && String(passed.id) === id ? passed : undefined));

    useEffect(() => {
        let cancelled = false;

        if (item === undefined) {
            load().then((items) => !cancelled && setItem(items.find((entry) => String(entry.id) === id) || false));
        }

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [id]);

    return item;
}

// Une page de détail qui attend son objet, ou ne le trouve plus.
export function Waiting({ item, title, parent }) {
    const { t } = useTranslation();
    const levels = useLevels();

    return (
        <SettingsPage title={title} parent={parent} levels={levels} guides={FEDERATION_GUIDES}>
            {item === false ? <NotFound /> : <Section><Loading /></Section>}
        </SettingsPage>
    );
}

// LES SERVEURS FÉDÉRÉS : la liste, et la ligne d'un serveur — son lien et ce que son listing dit de lui.
export const serverRow = (item, listing, t, language) => ({ id: item.id, label: item.hostname || item.link, link: item.link, listing: describeListing(listing, t, language), facts: listing || null });

export const loadServers = (refreshServers, t, language) => Promise.all([listServerRegistry(), refreshServers()]).then(([registry, known]) => {
    const listings = new Map((known || []).map((server) => [server.id, server.listing]));

    return (registry.ok ? registry.items || [] : []).map((item) => serverRow(item, listings.get(item.id), t, language));
});

// CE QU'OUVRE UN ACCÈS, écrit une fois pour « Créer un nouvel accès » et « Code d'appairage » (chantier 60) : pour qui, et
// partager aussi les fournisseurs de métadonnées. Ses consoles se restreignent ensuite sur sa page (chantier 66) : il naît
// avec toutes.
export function AccessName({ form }) {
    const { t } = useTranslation();

    return <TextRow id="label" context={form} label={t("fed.forWho")} help={t("fed.forWhoHelp")} props={{ placeholder: t("fed.forWhoPlaceholder"), "data-key": "fd-label" }} />;
}

// LES DROITS À DISTANCE D'UN ACCÈS NEUF (le porteur, 2026-11-20 : « pareil, avec une section pour le "Remote rights" ») —
// les mêmes lignes que sur la page de l'accès (IssuedTokenSections) : les fournisseurs de métadonnées, et l'administration
// à distance (chantier 73), coupées par défaut. Jamais depuis une page distante : le service des comptes la refuse.
export function RemoteRightsRows({ values, setValues }) {
    const { t } = useTranslation();
    const remote = useRemoteAdmin();

    return (
        <>
            <SwitchRow id="fd-providers" label={t("fed.providersLabel")} texts="allowed" help={t("fed.providersHelp")} checked={values.providers}
                onChange={(providers) => setValues((previous) => ({ ...previous, providers }))} />
            {!remote && <SwitchRow id="fd-remote-admin" label={t("remoteAdmin.title")} texts="allowed" help={t("remoteAdmin.switchHelp")} checked={values.remoteAdmin}
                onChange={(remoteAdmin) => setValues((previous) => ({ ...previous, remoteAdmin }))} />}
        </>
    );
}

// LES JETONS ÉMIS ICI, et les invitations en attente : une ligne chacun, qui ouvre sa page.
export const loadTokens = () => listCatalogTokens().then((answer) => (answer.ok ? answer.items || [] : []));
export const loadInvitations = () => listInvitations().then((answer) => (answer.ok ? answer.items || [] : []));
export const invitationKind = (item, t) => (item.kind === "account" ? t("invitations.kindAccount") : grantsOf(item, t));

// UN JETON ÉMIS : ce qu'il ouvre, sa dernière utilisation ; ses consoles (« Accès aux consoles », chantier 66, une page à
// part, `onConsoles`) ; partager aussi les fournisseurs de métadonnées (par le bouton de sa section) ; le révoquer.
// `onScopes(scopes)` → {ok, item, detail} ; `onConsoles()` ouvre la page de ses consoles ;
// `onRevoke()` → {ok, detail} ; `onRevoked`, après. Le jeton : `{id, label, scopes: "a,b", consoles: [...], last_used_at}`.
export function IssuedTokenSections({ token, onScopes = null, onConsoles = null, onRemoteAdmin = null, onRevoke, onRevoked }) {
    const { t, i18n } = useTranslation();
    const feedback = useFeedback();
    const remote = useRemoteAdmin();
    const [current, setCurrent] = useState(null);
    const shown = current || token;
    const shared = { providers: scopesOf(shown).includes(PROVIDERS) };

    // LES DROITS À DISTANCE, PAR LE BOUTON DE LEUR SECTION (le porteur, 2026-11-25 : « Faudra rajouter des boutons pour
    // tous les formulaires qui n'en ont pas ») : ce qui a changé part — les fournisseurs (ses portées), l'administration à
    // distance —, la réponse en fenêtre (SaveSection) ; le premier refus s'arrête là.
    const saveRights = async (values) => {
        let next = shown;

        if (onScopes && values.providers !== shared.providers) {
            const scopes = scopesFor({ ...shared, providers: values.providers });
            const answer = await onScopes(scopes);

            if (!answer.ok) {
                return answer;
            }
            next = { ...next, scopes: answer.item?.scopes || scopes.join(",") };
        }
        if (onRemoteAdmin && !remote && values.remoteAdmin !== Boolean(shown.remote_admin)) {
            const answer = await onRemoteAdmin(values.remoteAdmin);

            if (!answer.ok) {
                setCurrent(next);

                return answer;
            }
            next = { ...next, remote_admin: values.remoteAdmin };
        }
        setCurrent(next);

        return { ok: true };
    };
    const rows = [["catalog", "bi-collection", t("fed.scope.catalog")], ["catalog", "bi-card-text", t("fed.scopeMetadata")],
        [PROVIDERS, "bi-images", t("fed.scopeProviders")]];

    return (
        <>
            <Section title={t("details")} help={t("fed.tokenDetailsHelp")} sense="info">
                <Rows>
                    {/* CE QU'IL OUVRE, UNE LIGNE PAR PORTÉE (le porteur, 2026-11-16 : « change la ligne "ce qu'il ouvre" pour le
                        faire passer en 3 lignes : jeux, console et métadonnées. Comme valeur ce serait : autorisé ou pas
                        autorisé ») : elles suivent les interrupteurs plus bas dès qu'ils basculent (`shown`). Les jeux et leurs
                        métadonnées vont toujours ensemble, par `catalog` (« La métadonnée vient toujours du serveur. C'est
                        toujours autorisé comme les jeux. C'est l'utilisation des fournisseurs metadata du serveur qui doit être
                        autorisée ») : seuls les consoles et les fournisseurs se règlent. */}
                    {rows.map(([scope, icon, label]) => {
                        const allowed = scopesOf(shown).includes(scope);

                        return <InfoRow key={`${scope}-${icon}`} icon={icon} label={label} value={t(allowed ? "fed.allowed" : "fed.notAllowed")} pill={allowed ? "success" : "danger"} />;
                    })}
                    {/* Le libellé dit déjà « Dernière utilisation » : la valeur n'est que la date (chantier 59, P1 : le libellé et la valeur
                        disaient la même chose) ; la ligne de la liste garde sa phrase entière (`lastUseOf`). */}
                    <InfoRow icon="bi-clock-history" label={t("fed.lastUseLabel")}
                        value={shown.last_used_at ? dateTime(shown.last_used_at, i18n.language) : t("members.tokenNever")} />
                </Rows>
            </Section>
            {/* SES CONSOLES (chantier 66) : la même page que celle d'un membre, une ligne qui l'ouvre et dit ce qu'il laisse lancer. */}
            {onConsoles && (
                <Section title={t("fed.consolesSection")} help={t("fed.consolesSectionHelp")} sense="modify">
                    <Rows>
                        <NavRow id={`fd-consoles-${shown.id}`} icon={CONSOLE_ICON} label={t("members.consolesTitle")} help={t("fed.consolesRowHelp")}
                            value={consolesSummary(shown.consoles, t)} onOpen={onConsoles} />
                    </Rows>
                </Section>
            )}
            {/* LES DROITS À DISTANCE (le porteur, 2026-11-20) : les fournisseurs de métadonnées, et l'administration de ce
                serveur depuis celui qui tient l'accès (chantier 73) — accordée ACCÈS PAR ACCÈS, coupée par défaut : « je ne
                veux pas que ma smart TV puisse avoir la possibilité de faire du remote admin ». Jamais changée depuis une
                page distante : le service des comptes la refuse à une session venue d'ailleurs. */}
            {(onScopes || (onRemoteAdmin && !remote)) && (
                <SaveSection id={`fd-rights-${shown.id}`} title={t("fed.remoteRights")} help={t("fed.remoteRightsHelp")}
                    saved={{ providers: shared.providers, remoteAdmin: Boolean(shown.remote_admin) }} onSave={saveRights}>
                    {(values, set) => (
                        <>
                            {onScopes && (
                                <SwitchRow id={`fd-prov-${shown.id}`} label={t("fed.providersLabel")} texts="allowed" help={t("fed.providersHelp")} checked={values.providers}
                                    onChange={(providers) => set({ providers })} />
                            )}
                            {onRemoteAdmin && !remote && (
                                <SwitchRow id={`fd-admin-${shown.id}`} label={t("remoteAdmin.title")} texts="allowed" help={t("remoteAdmin.switchHelp")} checked={values.remoteAdmin}
                                    onChange={(remoteAdmin) => set({ remoteAdmin })} />
                            )}
                        </>
                    )}
                </SaveSection>
            )}
            <DangerSection id={`fd-rev-${shown.id}`} help={t("fed.revokeHelp")} label={t("fed.revokeTitle")} confirmLabel={t("fed.revoke")} question={`${t("fed.revokeConfirm")} ${shown.label}`}
                onConfirm={async () => {
                    const answer = await onRevoke();
                    feedback.result(answer.ok, t("fed.revoked", { name: shown.label }), answer.detail || t("settings.actionFailed"));

                    if (answer.ok) {
                        onRevoked();
                    }
                }} />
        </>
    );
}

// Les consoles qu'une page d'accès vient d'enregistrer (chantier 66) : la page de l'accès, rouverte avec le jeton qu'on
// lui avait passé, les montre telles qu'elles sont maintenant.
export const savedConsoles = new Map();

// Route : /settings/federation — les deux liens, et le guide.
// /settings/federation : LES DEUX BOUTS DE LA CONNEXION, deux lignes d'abord (le porteur, 2026-11-16 : « en premier lieu
// on voit les 2 liens » ; « on voit les amis avec qui on a partagé nos jeux et consoles et on voit aussi les amis qui
// nous ont partagé leurs jeux et consoles ») — les accès délivrés (si l'hôte en émet : un serveur, un téléphone aussi depuis `#756`), les
// serveurs rejoints —, chacune avec son nombre ; puis le guide. Rien sur le jeu chez un ami : c'est le Profil.
export default function Federation() {
    const { t } = useTranslation();
    const { open } = useSettingsNavigation();
    const { capabilities, servers } = useData();
    const issues = can(capabilities, "issuesCatalogTokens");
    const joined = (servers || []).length;
    const [delivered, setDelivered] = useState(null);
    const pairing = usePairing();

    useEffect(() => {
        let cancelled = false;

        if (issues) {
            loadTokens().then((tokens) => !cancelled && setDelivered(tokens.length));
        }

        return () => {
            cancelled = true;
        };
    }, [issues]);

    return (
        <SettingsPage title={t("settings.items.federation.label")}>
            <Section title={t("fed.connectionTitle")} help={t(issues ? "fed.connectionHelp" : "fed.connectionHelpJoinOnly")} sense="modify">
                <Rows>
                    {issues && <NavRow id="fd-accesses" icon="bi-key" label={t("fed.accessesTitle")} help={t("fed.accessesHelp")} value={fact(t("facts.accesses"), counted(t, "accesses", delivered))}
                        onOpen={() => open(ACCESSES, "fd-accesses")} />}
                    <NavRow id="fd-joined" icon="bi-lightning-charge" label={t("fed.joinedTitle")} help={t("fed.joinedHelp")} value={fact(t("facts.servers"), counted(t, "servers", joined))}
                        onOpen={() => open(JOINED, "fd-joined")} />
                </Rows>
            </Section>
            {/* CRÉER UN ACCÈS, SANS CONDITION (le porteur, 2026-11-25 : « créer un accès classique n'a pas besoin de discovery
                mode ») : une invitation ou un jeton, à part du mode découverte. */}
            {issues && (
                <Section title={t("fed.newAccess")} help={t("fed.accessMoreHelp")} sense="add">
                    <Rows>
                        <NavRow id="fd-mode-generate" icon="bi-key" label={t("fed.newAccess")} help={t("fed.newAccessHelp")}
                            onOpen={() => open(`${FEDERATION}/new-token`, "fd-mode-generate")} />
                    </Rows>
                </Section>
            )}
            {/* LE MODE SERVEUR (chantier 60) : un hôte qui émet des accès, serveur ou téléphone (`#756`). */}
            {issues && (
                <DiscoveryModeSection scope="server" pairing={pairing}>
                    <NavRow id="fd-mode-pairing" icon="bi-123" label={t("pairing.code")} help={t("pairing.codeRowHelp")}
                        onOpen={() => open(`${FEDERATION}/new-pairing`, "fd-mode-pairing")} />
                </DiscoveryModeSection>
            )}
            {/* JOUER SUR LES CONSOLES D'UN AMI N'EST PAS ICI (le porteur, 2026-11-16 : « Dans fédération (admin) retire la
                section plus. On n'a pas besoin ça ici. C'est dans profile qu'il le faut ») : ni ligne, ni formulaire de
                compte côté administrateur — Réglages › Profil › Fédération (MyFederation.jsx). */}
            <GuideSection guides={FEDERATION_GUIDES} />
        </SettingsPage>
    );
}
