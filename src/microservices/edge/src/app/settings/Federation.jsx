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
import { Listing } from "../bricks/Listing.jsx";
import { Loading } from "../bricks/Loading.jsx";
import NavRow from "../bricks/NavRow.jsx";
import { Rows } from "../bricks/Rows.jsx";
import { SaveSection } from "../bricks/SaveSection.jsx";
import { Section } from "../bricks/Section.jsx";
import { SwitchRow } from "../bricks/SwitchRow.jsx";
import { TextRow } from "../bricks/TextRow.jsx";
import { fetchServer, listCatalogTokens, listInvitations, listServerRegistry } from "../libs/api.js";
import { can, useData } from "../libs/data.jsx";
import { counted, dateTime, fact } from "../libs/format.js";
import { useRemoteAdmin } from "../libs/remoteAdmin.js";
import { DiscoveryModeSection, usePairing } from "../screens/access/Pairing.jsx";
import SettingsPage, { NotFound, useSettingsNavigation } from "./SettingsPage.jsx";
import { FEDERATION_GUIDES } from "./profile/Federation.jsx";
import { CONSOLE_ICON, RELAY_ICON } from "../globals/icons.js";

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

// La date et l'heure, celles de toute la page (libs/format.js).
export { dateTime };

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

// LA FICHE D'UN ACCÈS, ÉCRITE UNE FOIS (le porteur, 2026-11-26 : « Tu n'as pas fait une fiche commune et un formulaire
// commun […] Le but de la refonte était justement d'éviter de modifier plusieurs endroits juste pour une modification ») :
// ce qu'un accès ouvre, les mêmes lignes, dans le même ordre, avec les mêmes valeurs, vu des deux bouts — `side`
// "issued" (un accès délivré ici : ce qu'il ouvre chez nous) ou "received" (un serveur rejoint : ce que son accès nous
// ouvre) ; seuls les textes d'aide changent de point de vue (`fed.grant.<ligne>.<side>`). Une information de plus
// s'ajoute ICI, et les deux pages l'ont.
// `grants` : `{scopes: [...], consoles, relayAllowed, relayed: [noms]}` ; `onConsoles` : la ligne des consoles ouvre leur
// page (l'accès délivré, chantier 66). Les jeux et leurs métadonnées vont ensemble, par `catalog` (le porteur,
// 2026-11-16 : « La métadonnée vient toujours du serveur ») ; le relais, chantier 83.
export function AccessGrantRows({ side, grants, onConsoles = null, onRelayed = null }) {
    const { t } = useTranslation();
    const yesNo = (row, icon, allowed) => (
        <InfoRow key={row} icon={icon} label={grantLabel(t, row, side)} help={grantHelp(t, row, side)} value={t(allowed ? "fed.allowed" : "fed.notAllowed")}
            pill={allowed ? "success" : "danger"} />
    );
    const scopes = grants.scopes || [];
    const consoles = consolesSummary(grants.consoles, t);
    const relayed = grants.relayed || [];

    return (
        <>
            {yesNo("catalog", "bi-collection", scopes.includes("catalog"))}
            {yesNo("metadata", "bi-card-text", scopes.includes("catalog"))}
            {onConsoles
                ? <NavRow id="fd-grant-consoles" icon={CONSOLE_ICON} label={grantLabel(t, "consoles", side)} help={grantHelp(t, "consoles", side)} value={consoles} onOpen={onConsoles} />
                : <InfoRow icon={CONSOLE_ICON} label={grantLabel(t, "consoles", side)} help={grantHelp(t, "consoles", side)} value={consoles} />}
            {yesNo("providers", REMOTE_RIGHTS.providers, scopes.includes(PROVIDERS))}
            {yesNo("remoteAdmin", REMOTE_RIGHTS.remoteAdmin, Boolean(grants.remoteAdmin))}
            {yesNo("relayAllowed", REMOTE_RIGHTS.relayAllowed, Boolean(grants.relayAllowed))}
            {/* LEUR NOMBRE, PAS LEURS NOMS (le porteur, 2026-11-26 : « serveurs relayés doit être un listing car on peut en avoir
                un nombre infini ») : la ligne ouvre leur liste, sur sa route (un seul listing par route) — `onRelayed` ; sur
                un accès délivré, son formulaire les liste. */}
            {onRelayed
                ? <NavRow id="fd-grant-relayed" icon={RELAY_ICON} label={grantLabel(t, "relayed", side)} help={grantHelp(t, "relayed", side)}
                    value={relayed.length ? counted(t, "servers", relayed.length) : t("fed.relayedNone")} onOpen={onRelayed} />
                : <InfoRow icon={RELAY_ICON} label={grantLabel(t, "relayed", side)} help={grantHelp(t, "relayed", side)}
                    value={relayed.length ? counted(t, "servers", relayed.length) : t("fed.relayedNone")} />}
        </>
    );
}

// LES SERVEURS QU'UN SERVEUR REJOINT NOUS RELAIE (chantier 83) : leur identifiant de pair virtuel porte le sien
// (remote.RelayID, -(via × 10⁶ + id)). `[{id, name}]`.
export const relayedThrough = (servers, viaId) => (servers || []).filter((entry) => entry.relayedBy && Math.floor(-entry.id / 1e6) === Number(viaId))
    .map((entry) => ({ id: entry.id, name: entry.hostname || entry.link }));

// LES SERVEURS RELAYÉS, EN LISTE (le porteur, 2026-11-26 : « on peut en avoir un nombre infini ») : cherchés, une page à la
// fois au-delà de dix (Listing). `servers` : `[{id, name}]`. Ceux qu'un serveur rejoint nous relaie, sur leur route
// (settings/federation/servers/Relayed.jsx) ; sur la page d'un accès délivré, son formulaire les liste déjà.
export function RelayedServersSection({ side, servers }) {
    const { t } = useTranslation();
    const [search, setSearch] = useState("");
    const needle = search.trim().toLowerCase();
    const shown = servers.filter((server) => !needle || server.name.toLowerCase().includes(needle));

    return (
        <Section title={grantLabel(t, "relayed", side)} help={grantHelp(t, "relayed", side)} sense="info">
            <Listing items={shown} overall={servers.length} search={search} onSearch={setSearch} placeholder={t("fed.searchServer")} searchKey="fd-relayed-search"
                empty={t("fed.relayedNone")}
                render={(server) => <InfoRow key={server.id} icon={RELAY_ICON} label={server.name} />} />
        </Section>
    );
}

// LES TEXTES D'UNE LIGNE DE LA FICHE, une seule source pour la fiche ET le formulaire : `fed.grant.<ligne>.label`, puis
// `.labelIssued` / `.labelReceived` quand le libellé change de point de vue (le relais), et l'aide `.issued` / `.received`.
const grantLabel = (t, row, side) => t(`fed.grant.${row}.label${side === "issued" ? "Issued" : "Received"}`, { defaultValue: t(`fed.grant.${row}.label`) });
const grantHelp = (t, row, side) => t(`fed.grant.${row}.${side}`);
// Les droits à distance, et leur icône : les lignes que le formulaire règle, dans l'ordre de la fiche.
const REMOTE_RIGHTS = { providers: "bi-images", remoteAdmin: "bi-person-gear", relayAllowed: RELAY_ICON };

// LE FORMULAIRE DES DROITS À DISTANCE, ÉCRIT UNE FOIS : à la création d'un accès (jeton, invitation, code d'appairage) et
// sur sa page (le bouton de sa section) — les mêmes libellés et les mêmes aides que la fiche (`grantLabel`, `grantHelp`,
// côté "issued"). Coupés par défaut. L'administration et le relais jamais depuis une page distante : le service des
// comptes les refuse. `set(patch)` ; `only` : les lignes que la page sait enregistrer (toutes par défaut).
export function RemoteRightsRows({ values, set, only = Object.keys(REMOTE_RIGHTS) }) {
    const { t } = useTranslation();
    const remote = useRemoteAdmin();

    return Object.keys(REMOTE_RIGHTS).filter((row) => only.includes(row) && (row === "providers" || !remote)).map((row) => (
        <SwitchRow key={row} id={`fd-right-${row}`} label={grantLabel(t, row, "issued")} texts="allowed" help={grantHelp(t, row, "issued")} checked={values[row]}
            onChange={(on) => set({ [row]: on })} />
    ));
}

// LES JETONS ÉMIS ICI, et les invitations en attente : une ligne chacun, qui ouvre sa page.
export const loadTokens = () => listCatalogTokens().then((answer) => (answer.ok ? answer.items || [] : []));
export const loadInvitations = () => listInvitations().then((answer) => (answer.ok ? answer.items || [] : []));
export const invitationKind = (item, t) => (item.kind === "account" ? t("invitations.kindAccount") : grantsOf(item, t));

// UN JETON ÉMIS : ce qu'il ouvre, sa dernière utilisation ; ses consoles (« Accès aux consoles », chantier 66, une page à
// part, `onConsoles`) ; partager aussi les fournisseurs de métadonnées (par le bouton de sa section) ; le révoquer.
// `onScopes(scopes)` → {ok, item, detail} ; `onConsoles()` ouvre la page de ses consoles ;
// `onRevoke()` → {ok, detail} ; `onRevoked`, après. Le jeton : `{id, label, scopes: "a,b", consoles: [...], last_used_at}`.
export function IssuedTokenSections({ token, onScopes = null, onConsoles = null, onRemoteAdmin = null, onRelayAllowed = null, onRelayed = null, onRevoke, onRevoked }) {
    const { t, i18n } = useTranslation();
    const feedback = useFeedback();
    const remote = useRemoteAdmin();
    const { servers, refreshServers } = useData();
    // LE RELAIS (chantier 83) : les serveurs fédérés directs, que cet accès peut relayer — un ami qui nous relaie un
    // serveur ne se relaie pas plus loin.
    const direct = (servers || []).filter((server) => !server.relayedBy);
    const [current, setCurrent] = useState(null);
    const shown = current || token;
    const relayedNames = direct.filter((server) => (shown.relayed_servers || []).includes(server.id)).map((server) => server.hostname || server.link);
    const shared = { providers: scopesOf(shown).includes(PROVIDERS) };

    // CE QUE CHAQUE SERVEUR PERMET, RELU À L'OUVERTURE (chantier 83) : « Peut me relayer » se règle chez lui, et notre copie
    // de son listing ne le sait qu'à sa prochaine lecture. Une fois, à l'ouverture de la page — jamais en boucle.
    useEffect(() => {
        if (!onRelayed || remote) {
            return;
        }
        Promise.all(direct.map((server) => fetchServer(server.id))).then(() => refreshServers());
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [token.id]);

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
        if (onRelayAllowed && !remote && values.relayAllowed !== Boolean(shown.relay_allowed)) {
            const answer = await onRelayAllowed(values.relayAllowed);

            if (!answer.ok) {
                setCurrent(next);

                return answer;
            }
            next = { ...next, relay_allowed: values.relayAllowed };
        }
        setCurrent(next);

        return { ok: true };
    };
    return (
        <>
            {/* LA FICHE COMMUNE (AccessGrantRows), qui suit les interrupteurs plus bas dès qu'ils basculent (`shown`), puis sa
                dernière utilisation. */}
            <Section title={t("details")} help={t("fed.tokenDetailsHelp")} sense="info">
                <Rows>
                    <AccessGrantRows side="issued" onConsoles={onConsoles}
                        grants={{ scopes: scopesOf(shown), consoles: shown.consoles, remoteAdmin: shown.remote_admin, relayAllowed: shown.relay_allowed, relayed: relayedNames }} />
                    {/* Le libellé dit déjà « Dernière utilisation » : la valeur n'est que la date (chantier 59, P1). */}
                    <InfoRow icon="bi-clock-history" label={t("fed.lastUseLabel")}
                        value={shown.last_used_at ? dateTime(shown.last_used_at, i18n.language) : t("members.tokenNever")} />
                </Rows>
            </Section>
            {/* LES DROITS À DISTANCE (le porteur, 2026-11-20) : les fournisseurs de métadonnées, et l'administration de ce
                serveur depuis celui qui tient l'accès (chantier 73) — accordée ACCÈS PAR ACCÈS, coupée par défaut : « je ne
                veux pas que ma smart TV puisse avoir la possibilité de faire du remote admin ». Jamais changée depuis une
                page distante : le service des comptes la refuse à une session venue d'ailleurs. */}
            {(onScopes || ((onRemoteAdmin || onRelayAllowed) && !remote)) && (
                <SaveSection id={`fd-rights-${shown.id}`} title={t("fed.remoteRights")} help={t("fed.remoteRightsHelp")}
                    saved={{ providers: shared.providers, remoteAdmin: Boolean(shown.remote_admin), relayAllowed: Boolean(shown.relay_allowed) }} onSave={saveRights}>
                    {(values, set) => (
                        <RemoteRightsRows values={values} set={set}
                            only={[onScopes && "providers", onRemoteAdmin && "remoteAdmin", onRelayAllowed && "relayAllowed"].filter(Boolean)} />
                    )}
                </SaveSection>
            )}
            {/* LES SERVEURS RELAYÉS (chantier 83 ; le porteur, 2026-11-26 : « Le mieux ce serait par serveur et non tout
                d'un coup ») : un interrupteur par serveur fédéré direct ; grisé quand ce serveur ne nous permet pas de le
                relayer (« Peut me relayer », sur l'accès qu'il nous a donné). */}
            {onRelayed && !remote && (
                <SaveSection id={`fd-relayed-${shown.id}`} title={t("fed.grant.relayed.label")} help={t("fed.grant.relayed.issued")}
                    saved={{ relayed: (shown.relayed_servers || []).map(String) }}
                    onSave={async (values) => {
                        const ids = values.relayed.map(Number);
                        const answer = await onRelayed(ids);

                        if (answer.ok) {
                            setCurrent({ ...shown, relayed_servers: ids });
                        }

                        return answer;
                    }}
                    list={{
                        items: direct, textOf: (server) => server.hostname || server.link || "", placeholder: t("fed.searchServer"),
                        render: (server, values, set) => {
                            const id = String(server.id);
                            const allowed = Boolean(server.listing?.relayAllowed);

                            return (
                                <SwitchRow key={id} id={`fd-relayed-${shown.id}-${id}`} label={server.hostname || server.link}
                                    help={allowed ? fact(t("fed.urlLabel"), server.link) : t("fed.relayNotAllowed")} checked={values.relayed.includes(id)} disabled={!allowed}
                                    onChange={(on) => set({ relayed: on ? [...values.relayed, id] : values.relayed.filter((one) => one !== id) })} />
                            );
                        },
                    }} />
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
