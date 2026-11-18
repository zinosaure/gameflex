import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useParams } from "react-router-dom";
import {
    addServerRegistry, checkFederationServer, listServerRegistry, redeemServerCode, redeemServerInvitation, removeServerRegistry, rescanServer,
} from "../../libs/api.js";
import { useData } from "../../libs/data.jsx";
import { invitationLink, InvitationField, InvitationReveal } from "../../screens/access/Invitation.jsx";
import { DiscoveryModeSection, LocalServersSection, PairingReveal, pairingRefusal } from "../../screens/access/Pairing.jsx";
import { FEDERATION_GUIDES } from "../account/MyFederation.jsx";
import { openExternal } from "../../libs/links.js";
import { toast } from "../../shell/Shell.jsx";
import { Form, formContext, FormButton } from "../../ui/Forms.jsx";
import { NotFound, useQueryState, useSettingsNavigation } from "../SettingsPage.jsx";
import { useFederationHost, useFederationPaths } from "./federationHost.js";
import NavRow, { ChoiceRow, CopyField, DangerSection, GuideSection, InfoRow, Listing, Note, Rows, Section, SubmitRow, SwitchRow, TextRow } from "../../ui/Bricks.jsx";

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
// direction C (Bricks.jsx). Une page d'ajout revient à la liste une fois son travail fait, une page de détail
// aussi après un retrait ; la liste se relit en s'ouvrant. Une page ouverte par un lien direct relit la liste
// et y cherche son objet.
//
// CE QU'EST UN SERVEUR FÉDÉRÉ SE DIT DANS LES AIDES DES SECTIONS ET DANS « GUIDE » (le porteur, 2026-11-16, 3.1 ; « Il
// faut aussi enlever l'alert dans fédération ») — plus d'encadré `alert` : un serveur fédéré, c'est l'administrateur
// qui l'ajoute, ou qui donne accès au sien. Le compte fédéré, et ce qu'il faut pour jouer, se disent dans le Profil.
//
// LES LISTES LUES EN ASYNCHRONE se feuillettent (Listing ; le porteur, 2026-11-14 : « Rajouter des pagination pour les
// listes asynchrone ») : les serveurs, les jetons, et les invitations en attente.

// LE CADRE D'UNE PAGE : celui de l'hôte (federationHost.js) — SettingsPage chez GameFlex, la page du service chez
// gameflex-federation.
function Frame(props) {
    const { Page } = useFederationHost();

    return <Page {...props} />;
}

// Les niveaux sous la Fédération, nommés pour le fil d'Ariane même par un lien direct (SettingsPage, `levels`).
function useLevels() {
    const { t } = useTranslation();
    const { ACCESSES, PENDING, JOINED } = useFederationPaths();

    return { [ACCESSES]: t("fed.accessesTitle"), [PENDING]: t("fed.pendingTitle"), [JOINED]: t("fed.joinedTitle") };
}

const dateTime = (iso, language) => (iso ? new Date(iso).toLocaleString(language === "fr" ? "fr-FR" : "en-GB", { dateStyle: "medium", timeStyle: "short" }) : "");

// Ce que le listing d'un serveur dit de lui : sa date, sa version, sa machine, ses consoles.
function describeListing(listing, t, language) {
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
        (listing.scopes || []).includes("devices") ? t("fed.consolesGranted") : t("fed.consolesRefused"),
    ].filter(Boolean).join(" · ");
}

// Les droits d'un jeton émis ici, et leurs noms.
const scopesOf = (item) => String(item.scopes || "").split(",").map((scope) => scope.trim()).filter(Boolean);
// CE QU'UN JETON PARTAGE EN PLUS DES JEUX (le catalogue, toujours) : les consoles à distance (`devices`), et — l'option du
// chantier 57, `#719` — les fournisseurs de métadonnées (`metadata-providers`) : avec elle, la recherche des fiches
// d'ici interroge aussi nos comptes IGDB et SteamGridDB pour son porteur ; nos clés ne sortent jamais. Coupées par défaut.
const PROVIDERS = "metadata-providers";
const scopesFor = ({ devices, providers }) => ["catalog", ...(devices ? ["devices"] : []), ...(providers ? [PROVIDERS] : [])];
// Ce qu'ouvre une invitation (« Jeux · Consoles »), sous elle dans la liste des invitations en attente. Retiré par erreur
// avec les sous-titres des accès (f786ab1d) : la liste des invitations s'effondrait dès qu'une attendait.
const grantsOf = (item, t) => scopesOf(item).map((scope) => t(`fed.scope.${scope}`, { defaultValue: scope })).join(" · ");
// Le sous-titre d'un accès dans la liste : sa date de dernière utilisation, seule (le porteur, 2026-11-16 : « les
// sous-titres des serveurs fédérés ou accès délivrés sont trop verbeux. Mets juste la date ça suffit »).
const lastUseOf = (item, t, language) => (item.last_used_at ? dateTime(item.last_used_at, language) : t("fed.neverUsed"));

// L'OBJET D'UNE PAGE DE DÉTAIL : celui que la liste a passé (`state[name]`), sinon relu par `load` (un lien
// direct, un rechargement) ; `false` quand il n'existe plus.
function usePassed(name, id, load) {
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
function Waiting({ item, title, parent }) {
    const { t } = useTranslation();
    const levels = useLevels();

    return (
        <Frame title={title} parent={parent} levels={levels}>
            {item === false ? <NotFound /> : <Section><Note className="mt-0">{t("settings.loading")}</Note></Section>}
        </Frame>
    );
}

// REJOINDRE UN SERVEUR : son invitation (et le nom sous lequel il apparaîtra), son code d'appairage (chantier 60 : les
// serveurs du réseau local en mode découverte, et l'adresse, toujours tapable), ou son lien et son jeton — le choix de
// la méthode est un choix segmenté (ChoiceRow), plus des onglets.
function AddServerPage() {
    const { JOINED } = useFederationPaths();
    const { t } = useTranslation();
    const levels = useLevels();
    const { refreshServers } = useData();
    const { back } = useSettingsNavigation();
    const [values, setValues] = useState({ mode: "invitation", invitation: "", name: "", url: "", token: "", address: "", code: "" });
    const [pending, setPending] = useState(false);
    const form = formContext(values, setValues);
    const set = (patch) => setValues((previous) => ({ ...previous, ...patch }));

    // Le serveur désigné : l'invitation échangée chez lui, ou le lien et le jeton vérifiés.
    const serverOf = async () => {
        if (values.mode === "invitation") {
            const redeemed = await redeemServerInvitation(values.invitation);

            if (!redeemed.ok) {
                toast(t(`invitations.errors.${redeemed.errorCode || "serverUnreachable"}`, { server: "", defaultValue: redeemed.errorMessage || t("fed.unreachable") }));

                return null;
            }

            return { link: redeemed.data.link, serverToken: redeemed.data.token, seen: redeemed.data };
        }

        if (values.mode === "code") {
            const redeemed = await redeemServerCode(values.address, values.code);

            if (!redeemed.ok) {
                toast(pairingRefusal(t, redeemed.errorCode));

                return null;
            }

            return { link: redeemed.data.link, serverToken: redeemed.data.token, seen: redeemed.data };
        }
        const probe = await checkFederationServer(values.url.trim(), values.token.trim());

        if (!probe.ok) {
            toast(probe.errorMessage || t("fed.unreachable"));

            return null;
        }

        return { link: values.url.trim(), serverToken: values.token.trim(), seen: probe.data || {} };
    };

    const save = async () => {
        setPending(true);
        const found = await serverOf();

        if (!found) {
            setPending(false);

            return;
        }
        const answer = await addServerRegistry({ link: found.link, serverToken: found.serverToken, hostname: (values.name || "").trim() || found.seen.hostname || found.link, identity: found.seen.identity || "" });

        if (!answer.ok) {
            setPending(false);
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }

        if (answer.item?.id) {
            await rescanServer(answer.item.id);
        }
        await refreshServers();
        setPending(false);
        toast(t("fed.added"));
        back(JOINED);
    };

    const field = (name, label, help, placeholder) => (
        <TextRow id={name} context={form} label={label} help={help} props={{ placeholder, "data-key": `fd-${name}` }} />
    );
    const typed = { invitation: ["invitation", "name"], code: ["address", "code", "name"], token: ["name", "url", "token"] }[values.mode];
    const ready = values.mode === "invitation" ? (values.invitation || "").trim()
        : values.mode === "code" ? (values.address || "").trim() && /^\d{3} ?\d{3}$/.test((values.code || "").trim())
            : (values.url || "").trim() && (values.token || "").trim();
    // Rien de tapé dans le mode choisi, rien de changé (CODE_CONVENTIONS.md, `isReplaced`).
    const isReplaced = typed.some((name) => (values[name] || "").trim());

    // Le bouton du formulaire, dans l'encadré de son dernier groupe (le porteur, 2026-11-13 : « le bouton doit être
    // dans la section card avec les autres form controls »).
    const submit = (
        <SubmitRow>
            <FormButton id="fd-save" label={t("settings.save")} isReplaced={isReplaced} pending={pending} disabled={!ready} />
        </SubmitRow>
    );

    // La méthode, puis l'invitation (son champ, Invitation.jsx) et le nom ; ou le nom, le lien et le jeton.
    return (
        <Frame title={t("fed.join")} parent={JOINED} levels={levels}>
            <Form props={{ onSubmit: (event) => {
                event.preventDefault();

                if (ready && !pending) {
                    save();
                }
            } }}>
                <Section title={t("fed.joinMethodTitle")}>
                    <Rows>
                        <ChoiceRow id="fd-mode" label={t("fed.joinMethod")} help={t("fed.joinMethodHelp")} value={values.mode} onChange={(mode) => set({ mode })}
                            options={[["invitation", t("invitations.tab")], ["code", t("pairing.code")], ["token", t("invitations.tokenTab")]]} />
                    </Rows>
                </Section>
                {values.mode === "code" ? (
                    <>
                        {/* La liste du sujet `discovery`, le temps où cette option est choisie : la page ne s'y abonne qu'ici. */}
                        <LocalServersSection onChoose={(server) => set({ address: server.link, name: values.name || server.name })} />
                        <Section title={t("fed.serverTitle")}>
                            <Rows>
                                {field("address", t("pairing.urlLabel"), t("pairing.urlHelp"), "http://192.168.1.10:10200")}
                                <TextRow id="code" context={form} label={t("pairing.codeLabel")} help={t("pairing.codeHelp")}
                                    props={{ placeholder: "000 000", "data-key": "fd-code", inputMode: "numeric", autoComplete: "one-time-code", maxLength: 7 }} />
                                {field("name", t("fed.nameLabel"), t("invitations.nameHelp"), t("fed.namePlaceholder"))}
                                {submit}
                            </Rows>
                        </Section>
                    </>
                ) : values.mode === "invitation" ? (
                    <>
                        <Section title={t("invitations.detailTitle")}>
                            <InvitationField id="fd-invitation" label={t("invitations.fieldLabel")} help={t("invitations.serverFieldHelp")} value={values.invitation} onChange={(invitation) => set({ invitation })} />
                        </Section>
                        <Section title={t("fed.serverTitle")}>
                            <Rows>
                                {field("name", t("fed.nameLabel"), t("invitations.nameHelp"), t("fed.namePlaceholder"))}
                                {submit}
                            </Rows>
                        </Section>
                    </>
                ) : (
                    <Section title={t("fed.serverTitle")}>
                        <Rows>
                            {field("name", t("fed.nameLabel"), t("fed.nameHelp"), t("fed.namePlaceholder"))}
                            {field("url", t("fed.urlLabel"), t("fed.gameflexUrlHelp"), "https://gameflex.com")}
                            {field("token", t("fed.tokenLabel"), t("fed.tokenHelp"), t("fed.tokenLabel"))}
                            {submit}
                        </Rows>
                    </Section>
                )}
            </Form>
        </Frame>
    );
}

// LES SERVEURS FÉDÉRÉS : la liste, et la ligne d'un serveur — son lien et ce que son listing dit de lui.
const serverRow = (item, listing, t, language) => ({ id: item.id, label: item.hostname || item.link, link: item.link, listing: describeListing(listing, t, language), facts: listing || null });

const loadServers = (refreshServers, t, language) => Promise.all([listServerRegistry(), refreshServers()]).then(([registry, known]) => {
    const listings = new Map((known || []).map((server) => [server.id, server.listing]));

    return (registry.ok ? registry.items || [] : []).map((item) => serverRow(item, listings.get(item.id), t, language));
});

// /settings/federation/servers : LES SERVEURS REJOINTS — ceux qui nous ont donné un accès —, une ligne chacun : son nom
// et son adresse, rien d'autre (le porteur, 2026-11-16 : « Pas besoin des longs sous-titre. Juste l'adresse suffira. Dans
// la fiche on aura toutes les info ») ; puis « Plus » : en rejoindre un ; puis le guide.
function JoinedPage() {
    const { FEDERATION, JOINED } = useFederationPaths();
    const { t, i18n } = useTranslation();
    const { refreshServers } = useData();
    const { open } = useSettingsNavigation();
    const [items, setItems] = useState([]);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState("");

    useEffect(() => {
        loadServers(refreshServers, t, i18n.language).then((rows) => {
            setItems(rows);
            setLoading(false);
        });
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    const needle = search.trim().toLowerCase();
    const filtered = needle ? items.filter((item) => `${item.label} ${item.link} ${item.listing}`.toLowerCase().includes(needle)) : items;

    return (
        <Frame title={t("fed.joinedTitle")} parent={FEDERATION}>
            <Section title={t("fed.serversTitle")} help={t("fed.serversHelp")}>
                <Listing items={filtered} loading={loading} search={search} onSearch={setSearch} placeholder={t("fed.serversSearch")} searchKey="fd-srv-search"
                    found={t("fed.serversFound", { count: filtered.length })} empty={t("fed.serversEmpty")}
                    render={(item) => (
                        <NavRow key={item.id} id={`fd-srv-${item.id}`} icon="bi-lightning-charge" tone="amber" label={item.label} help={item.link}
                            onOpen={() => open(`${JOINED}/${item.id}`, `fd-srv-${item.id}`, { server: item })} />
                    )} />
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="fd-add" icon="bi-box-arrow-in-right" tone="amber" label={t("fed.join")} help={t("fed.joinHelp")} onOpen={() => open(`${FEDERATION}/add`, "fd-add")} />
                </Rows>
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </Frame>
    );
}

// UN SERVEUR FÉDÉRÉ, EN SECTIONS D'INFORMATIONS COMME L'ENGRENAGE D'UN JEU (le porteur, 2026-11-16, 2.8 : « Le listing des
// informations du serveur fédéré en section comme les fiches des jeux ») : ce que la copie de son listing dit de lui —
// le serveur (son lien, sa version, la date de son listing, les jeux qu'il prête), sa machine (système, processeur,
// mémoire, cartes graphiques), ce que son jeton nous ouvre (les jeux, ses consoles à distance, ses fournisseurs de
// métadonnées). Pas de ligne par console : jouer demande toujours un compte chez lui, l'affaire de chaque utilisateur
// dans Réglages › Profil › Fédération (le porteur, 2026-11-16). Le retirer (« Danger »), puis le guide, en dernier.
function ServerPage() {
    const { JOINED } = useFederationPaths();
    const { t, i18n } = useTranslation();
    const { serverId } = useParams();
    const { refreshServers } = useData();
    const { back } = useSettingsNavigation();
    const levels = useLevels();
    const server = usePassed("server", serverId, () => loadServers(refreshServers, t, i18n.language));

    if (!server) {
        return <Waiting item={server} title={t("fed.joinedTitle")} parent={JOINED} />;
    }
    const facts = server.facts || {};
    const machine = facts.machine || {};
    const scopes = facts.scopes || [];
    const gigabytes = (bytes) => (bytes ? t("fed.gigabytes", { gb: Math.round(bytes / 2 ** 30) }) : null);
    const lent = Object.values(facts.consoles || {}).reduce((total, count) => total + count, 0);
    // Les mêmes mots que la page d'un accès délivré (« Autorisé », « Pas autorisé ») : une portée, deux pages.
    const opened = (scope) => t(scopes.includes(scope) ? "fed.allowed" : "fed.notAllowed");

    return (
        <Frame title={server.label} parent={JOINED} levels={levels}>
            <Section title={t("fed.serverTitle")} help={t("fed.serverHelp")}>
                <Rows>
                    {/* SON ADRESSE S'OUVRE dans un autre onglet (le porteur, 2026-11-16 : « rend le lien cliquable pour ouvrir
                        dans un nouveau tab la page ») ; dans l'application Android, au navigateur du téléphone (EXTERNAL). */}
                    <NavRow id="fd-server-link" icon="bi-link-45deg" tone="amber" label={t("fed.linkLabel")} help={server.link} opens="link" onOpen={() => openExternal(server.link)} />
                    <InfoRow icon="bi-tag" label={t("fed.versionLabel")} value={facts.server?.version ? `GameFlex ${facts.server.version}` : null} />
                    <InfoRow icon="bi-clock-history" label={t("fed.listedLabel")} value={facts.updatedAt ? dateTime(facts.updatedAt, i18n.language) : t("fed.noListing")} />
                    <InfoRow icon="bi-collection" label={t("fed.gamesLabel")} value={server.facts ? t("fed.gamesCount", { count: lent, total: lent.toLocaleString(i18n.language) }) : null} />
                </Rows>
            </Section>
            {server.facts && (
                <Section title={t("fed.machineTitle")}>
                    <Rows>
                        <InfoRow icon="bi-phone" label={t("fed.deviceLabel")} value={machine.device} />
                        <InfoRow icon="bi-terminal" label={t("fed.osLabel")} value={machine.os} />
                        <InfoRow icon="bi-cpu" label={t("fed.cpuLabel")} value={machine.cpu} />
                        <InfoRow icon="bi-diagram-3" label={t("fed.coresLabel")} value={machine.cores ? String(machine.cores) : null} />
                        <InfoRow icon="bi-memory" label={t("fed.memoryLabel")} value={gigabytes(machine.ramBytes)} />
                        {(machine.gpus || []).map((gpu, index) => (
                            <InfoRow key={`${gpu.name}-${index}`} icon="bi-gpu-card" label={t("fed.gpuLabel")} value={[gpu.name, gigabytes(gpu.vramBytes)].filter(Boolean).join(" · ")} />
                        ))}
                    </Rows>
                </Section>
            )}
            {server.facts && (
                <Section title={t("fed.grantsTitle")} help={t("fed.grantsHelp")}>
                    <Rows>
                        <InfoRow icon="bi-collection" tone="green" label={t("fed.scope.catalog")} help={t("fed.catalogHelp")} value={opened("catalog")} />
                        {/* LES MÉTADONNÉES VIENNENT AVEC LES JEUX (le porteur, 2026-11-16 : « La métadonnée vient toujours du serveur.
                            C'est toujours autorisé comme les jeux. C'est l'utilisation des fournisseurs metadata du serveur qui doit
                            être autorisée ») : la même portée que les jeux, `catalog`. */}
                        <InfoRow icon="bi-card-text" tone="green" label={t("fed.scopeMetadata")} help={t("fed.metadataHelp")} value={opened("catalog")} />
                        <InfoRow icon="bi-playstation" tone={scopes.includes("devices") ? "green" : "slate"} label={t("fed.scope.devices")} help={t("fed.devicesGrantHelp")} value={opened("devices")} />
                        <InfoRow icon="bi-images" tone={scopes.includes("metadata-providers") ? "green" : "slate"} label={t("fed.scopeProviders")} help={t("fed.providersGrantHelp")}
                            value={opened("metadata-providers")} />
                    </Rows>
                </Section>
            )}
            <DangerSection id={`fd-del-${server.id}`} help={t("fed.removeHelp")} label={t("fed.remove")} question={`${t("fed.removeConfirm")} ${server.label}`}
                onConfirm={async () => {
                    const answer = await removeServerRegistry(server.id);
                    toast(answer.ok ? t("fed.removed", { name: server.label }) : answer.detail || t("settings.actionFailed"));

                    if (answer.ok) {
                        await refreshServers();
                        back(JOINED);
                    }
                }} />
            <GuideSection guides={FEDERATION_GUIDES} />
        </Frame>
    );
}

// CE QU'OUVRE UN ACCÈS, écrit une fois pour « Créer un nouvel accès » et « Code d'appairage » (chantier 60) : pour qui,
// partager aussi les consoles, partager aussi les fournisseurs de métadonnées.
function AccessFields({ form, values, setValues }) {
    const { t } = useTranslation();
    const { offers } = useFederationHost();

    return (
        <>
            <TextRow id="label" context={form} label={t("fed.forWho")} help={t("fed.forWhoHelp")} props={{ placeholder: t("fed.forWhoPlaceholder"), "data-key": "fd-label" }} />
            {offers.devices && <SwitchRow id="fd-devices" label={t("fed.devicesLabel")} help={t("fed.devicesHelp")} checked={values.devices}
                onChange={(devices) => setValues((previous) => ({ ...previous, devices }))} />}
            {offers.providers && <SwitchRow id="fd-providers" label={t("fed.providersLabel")} help={t("fed.providersHelp")} checked={values.providers}
                onChange={(providers) => setValues((previous) => ({ ...previous, providers }))} />}
        </>
    );
}

// CRÉER UN ACCÈS : pour qui, partager aussi les consoles, partager aussi les fournisseurs de métadonnées (coupé par
// défaut, `#719`) ; une invitation (le chemin proposé), ou — l'interrupteur
// « Plutôt le lien et le jeton » allumé (le porteur, 2026-11-14 : « Il est préférable de mettre en option (switch)
// l'action "Plutôt lien et jeton". Au lieu d'avoir de bouton. ») — le lien et un jeton montré une seule fois, qui
// restent sur la page jusqu'à « J'ai copié ». UN bouton, dont le libellé suit l'interrupteur (« Créer l'invitation »,
// « Créer le jeton ») : il dit ce qu'on obtiendra.
function NewTokenPage() {
    const { ACCESSES } = useFederationPaths();
    const { t } = useTranslation();
    const levels = useLevels();
    const host = useFederationHost();
    const origin = host.useOrigin();
    const { back } = useSettingsNavigation();
    const [values, setValues] = useState({ label: "", devices: false, providers: false, tokenInstead: false });
    const [pending, setPending] = useState(false);
    const [created, setCreated] = useState(null);
    const [invited, setInvited] = useState(null);
    const form = formContext(values, setValues);
    const label = (values.label || "").trim();
    const scopes = () => scopesFor(values);
    const done = () => back(ACCESSES);

    const invite = async () => {
        setPending(true);
        const answer = await host.api.createInvitation({ kind: "federation", label, scopes: scopes() });
        setPending(false);

        if (!answer.ok || !answer.item) {
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }
        setInvited(answer.item);
    };

    const create = async () => {
        setPending(true);
        const answer = await host.api.createToken(label, scopes());
        setPending(false);

        if (!answer.ok || !answer.item) {
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }
        setCreated(answer.item);
    };

    return (
        <Frame title={t("fed.newAccess")} parent={ACCESSES} levels={levels}>
            {invited ? (
                <InvitationReveal link={invitationLink(origin, invited.code)} expiresAt={invited.expires_at} title={t("invitations.federationTitle", { label: invited.label })} onDone={done} />
            ) : created ? (
                <>
                    {/* Montré une seule fois : l'aide du groupe le dit, plus un encadré `alert` (le porteur, 2026-11-16). */}
                    <Section title={t("fed.createdTitle", { label: created.label })} help={`${t("fed.createdHelp")} ${t("fed.onceOnly")}`}>
                        <Rows>
                            <CopyField id="fd-created-link" label={t("fed.linkLabel")} help={t("fed.linkHelp")} value={origin} />
                            <CopyField id="fd-created-token" label={t("fed.tokenLabel")} help={t("fed.tokenOnceHelp")} value={created.token || ""} />
                        </Rows>
                    </Section>
                    {/* « J'ai copié » referme la page : une ligne de « Plus », comme « Terminé » d'une invitation (47 des modèles). */}
                    <Section title={t("more")}>
                        <Rows>
                            <NavRow id="fd-created-done" icon="bi-check2" tone="slate" label={t("fed.done")} opens="action" onOpen={done} />
                        </Rows>
                    </Section>
                </>
            ) : (
                <Form props={{ onSubmit: (event) => {
                    event.preventDefault();

                    if (label && !pending) {
                        (values.tokenInstead ? create : invite)();
                    }
                } }}>
                    <Section title={t("fed.accessTitle")}>
                        <Rows>
                            <AccessFields form={form} values={values} setValues={setValues} />
                            <SwitchRow id="fd-token-instead" label={t("invitations.tokenInstead")} help={t("fed.tokenInsteadHelp")} checked={values.tokenInstead}
                                onChange={(tokenInstead) => setValues((previous) => ({ ...previous, tokenInstead }))} />
                            <SubmitRow>
                                <FormButton id="fd-create" label={t(values.tokenInstead ? "fed.createToken" : "invitations.create")} isReplaced={Boolean(label)} pending={pending} />
                            </SubmitRow>
                        </Rows>
                    </Section>
                </Form>
            )}
        </Frame>
    );
}

// LE CODE D'APPAIRAGE D'UN ACCÈS (chantier 60) : le mode serveur en tête, puis ce qu'ouvre l'accès (AccessFields) et
// « Créer le code », grisé tant que le mode serveur est fermé. Le code créé prend la page (PairingReveal) ; servi, la
// page revient à la liste des accès, où le nouvel accès apparaît.
function NewPairingPage() {
    const { ACCESSES } = useFederationPaths();
    const { t } = useTranslation();
    const levels = useLevels();
    const host = useFederationHost();
    const pairing = host.usePairing();
    const { back } = useSettingsNavigation();
    const [values, setValues] = useState({ label: "", devices: false, providers: false });
    const [pending, setPending] = useState(false);
    const [created, setCreated] = useState(null);
    const form = formContext(values, setValues);
    const label = (values.label || "").trim();
    const open = Boolean(pairing?.server);

    const create = async () => {
        setPending(true);
        const answer = await host.api.createPairingCode({ kind: "federation", label, scopes: scopesFor(values) });
        setPending(false);

        if (!answer.ok || !answer.item) {
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }
        setCreated(answer.item);
    };
    const finish = (text) => {
        toast(text);
        setCreated(null);
        back(ACCESSES);
    };

    return (
        <Frame title={t("pairing.code")} parent={ACCESSES} levels={levels}>
            {created ? (
                <PairingReveal created={created} pairing={pairing} onDone={finish} onCancel={() => host.api.cancelPairingCode(created.id)} />
            ) : (
                <>
                    <DiscoveryModeSection scope="server" pairing={pairing} />
                    <Form props={{ onSubmit: (event) => {
                        event.preventDefault();

                        if (label && open && !pending) {
                            create();
                        }
                    } }}>
                        <Section title={t("fed.accessTitle")} help={open ? null : t("pairing.createNeedsMode")}>
                            <Rows>
                                <AccessFields form={form} values={values} setValues={setValues} />
                                <SubmitRow>
                                    <FormButton id="fd-pairing-create" label={t("pairing.create")} isReplaced={Boolean(label)} disabled={!open} pending={pending} />
                                </SubmitRow>
                            </Rows>
                        </Section>
                    </Form>
                    <GuideSection guides={FEDERATION_GUIDES} />
                </>
            )}
        </Frame>
    );
}

// LES JETONS ÉMIS ICI, et les invitations en attente : une ligne chacun, qui ouvre sa page.
const loadTokens = (api) => api.listTokens().then((answer) => (answer.ok ? answer.items || [] : []));
const loadInvitations = (api) => api.listInvitations().then((answer) => (answer.ok ? answer.items || [] : []));
const invitationKind = (item, t) => (item.kind === "account" ? t("invitations.kindAccount") : grantsOf(item, t));

// /settings/federation/access : LES ACCÈS DÉLIVRÉS — les jetons émis ici —, puis « Plus » : en créer un, et ceux qui
// attendent encore d'être échangés ; puis le guide.
function AccessesPage() {
    const { FEDERATION, PENDING } = useFederationPaths();
    const { api } = useFederationHost();
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const [items, setItems] = useState([]);
    const [pending, setPending] = useState(null);
    const [loading, setLoading] = useState(true);
    const [search, setSearch] = useState("");

    useEffect(() => {
        Promise.all([loadTokens(api), loadInvitations(api)]).then(([tokens, invitations]) => {
            setItems(tokens);
            setPending(invitations.length);
            setLoading(false);
        });
    }, []);

    const needle = search.trim().toLowerCase();
    const filtered = needle ? items.filter((item) => String(item.label || "").toLowerCase().includes(needle)) : items;

    return (
        <Frame title={t("fed.accessesTitle")} parent={FEDERATION}>
            <Section title={t("fed.tokensTitle")} help={t("fed.tokensHelp")}>
                <Listing items={filtered} loading={loading} search={search} onSearch={setSearch} placeholder={t("fed.search")} searchKey="fd-tok-search"
                    found={t("fed.found", { count: filtered.length })} empty={t("fed.empty")}
                    render={(item) => (
                        <NavRow key={item.id} id={`fd-tok-${item.id}`} icon="bi-key" tone="amber" label={item.label} help={lastUseOf(item, t, i18n.language)}
                            onOpen={() => open(`${FEDERATION}/tokens/${item.id}`, `fd-tok-${item.id}`, { token: item })} />
                    )} />
            </Section>
            <Section title={t("more")}>
                <Rows>
                    <NavRow id="fd-generate" icon="bi-key" tone="amber" label={t("fed.newAccess")} help={t("fed.newAccessHelp")} onOpen={() => open(`${FEDERATION}/new-token`, "fd-generate")} />
                    <NavRow id="fd-pairing" icon="bi-123" tone="amber" label={t("pairing.code")} help={t("pairing.codeRowHelp")} onOpen={() => open(`${FEDERATION}/new-pairing`, "fd-pairing")} />
                    <NavRow id="fd-pending" icon="bi-qr-code" tone="indigo" label={t("fed.pendingTitle")} help={t("fed.pendingHelp")} value={pending}
                        onOpen={() => open(PENDING, "fd-pending")} />
                </Rows>
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </Frame>
    );
}

// /settings/federation/pending : LES ACCÈS DÉLIVRÉS EN ATTENTE — les invitations créées ici et pas encore échangées
// (valables 12 heures, une seule fois) ; une ligne ouvre sa page.
function PendingPage() {
    const { FEDERATION, ACCESSES } = useFederationPaths();
    const { api } = useFederationHost();
    const { t, i18n } = useTranslation();
    const { open } = useSettingsNavigation();
    const [read, update] = useQueryState({ page: "1" });
    const [items, setItems] = useState([]);
    const [loading, setLoading] = useState(true);
    const levels = useLevels();

    useEffect(() => {
        loadInvitations(api).then((invitations) => {
            setItems(invitations);
            setLoading(false);
        });
    }, []);

    return (
        <Frame title={t("fed.pendingTitle")} parent={ACCESSES} levels={levels}>
            <Section title={t("invitations.pendingTitle")} help={t("invitations.pendingHelp")}>
                <Listing items={items} loading={loading} empty={t("fed.pendingEmpty")} page={read("page")} onPage={(page) => update({ page })}
                    render={(item) => (
                        <NavRow key={item.id} id={`fd-inv-${item.id}`} icon="bi-qr-code" tone="indigo" label={item.label || t("invitations.kindAccount")}
                            help={`${invitationKind(item, t)} · ${t("invitations.until", { at: dateTime(item.expires_at, i18n.language) })}`}
                            onOpen={() => open(`${FEDERATION}/invitations/${item.id}`, `fd-inv-${item.id}`, { invitation: item })} />
                    )} />
            </Section>
            <GuideSection guides={FEDERATION_GUIDES} />
        </Frame>
    );
}

// UNE INVITATION EN ATTENTE : pour qui, ce qu'elle ouvre, jusqu'à quand ; la révoquer.
function InvitationPage() {
    const { PENDING } = useFederationPaths();
    const { t, i18n } = useTranslation();
    const { invitationId } = useParams();
    const { back } = useSettingsNavigation();
    const { api } = useFederationHost();
    const invitation = usePassed("invitation", invitationId, () => loadInvitations(api));
    const levels = useLevels();

    if (!invitation) {
        return <Waiting item={invitation} title={t("fed.pendingTitle")} parent={PENDING} />;
    }
    const label = invitation.label || t("invitations.kindAccount");

    return (
        <Frame title={label} parent={PENDING} levels={levels}>
            <Section title={t("invitations.detailTitle")}>
                <Rows>
                    <InfoRow icon="bi-person" tone="indigo" label={t("fed.forWho")} value={invitation.label} />
                    <InfoRow icon="bi-unlock" tone="indigo" label={t("invitations.kindLabel")} value={invitationKind(invitation, t)} />
                    <InfoRow icon="bi-hourglass-split" label={t("invitations.expiresLabel")} value={dateTime(invitation.expires_at, i18n.language)} />
                </Rows>
            </Section>
            <DangerSection id={`fd-inv-revoke-${invitation.id}`} help={t("invitations.revokeHelp")} label={t("invitations.revokeTitle")} confirmLabel={t("fed.revoke")}
                question={`${t("invitations.revokeConfirm")} ${invitation.label || ""}`} onConfirm={async () => {
                    const answer = await api.revokeInvitation(invitation.id);
                    toast(answer.ok ? t("invitations.revoked", { name: invitation.label }) : answer.detail || t("settings.actionFailed"));

                    if (answer.ok) {
                        back(PENDING);
                    }
                }} />
        </Frame>
    );
}

// UN JETON ÉMIS : ce qu'il ouvre, sa dernière utilisation ; partager aussi les consoles, partager aussi les fournisseurs
// de métadonnées (chacun appliqué à l'instant, comme avant) ; le révoquer. LA MÊME PAGE POUR gameflex-federation (le
// porteur, 2026-11-18 : « Utilise la même page de Issued token […] car il manque des infos ») : ses sections, sans son
// cadre. `offers` : ce que l'hôte peut prêter en plus de ses jeux — le service de fédération n'a ni consoles ni prêt de
// ses fournisseurs : ni leurs lignes ni leurs interrupteurs. `onScopes(scopes)` → {ok, item, detail} ; `onRevoke()` →
// {ok, detail} ; `onRevoked`, après. Le jeton : `{id, label, scopes: "a,b", last_used_at}`.
export function IssuedTokenSections({ token, offers = { devices: true, providers: true }, onScopes = null, onRevoke, onRevoked }) {
    const { t, i18n } = useTranslation();
    const [current, setCurrent] = useState(null);
    const [pending, setPending] = useState(false);
    const shown = current || token;
    const shared = { devices: scopesOf(shown).includes("devices"), providers: scopesOf(shown).includes(PROVIDERS) };

    // Un partage de plus ou de moins (`name` : devices, providers) ; les autres restent ce qu'ils sont.
    const setShared = async (name, next) => {
        const scopes = scopesFor({ ...shared, [name]: next });
        setPending(true);
        const answer = await onScopes(scopes);
        setPending(false);

        if (!answer.ok) {
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }
        toast(t(`fed.${name}${next ? "Shared" : "Unshared"}`, { label: shown.label }));
        setCurrent({ ...shown, scopes: answer.item?.scopes || scopes.join(",") });
    };
    const rows = [["catalog", "bi-collection", t("fed.scope.catalog")], ["catalog", "bi-card-text", t("fed.scopeMetadata")],
        ...(offers.devices ? [["devices", "bi-playstation", t("fed.scope.devices")]] : []),
        ...(offers.providers ? [[PROVIDERS, "bi-images", t("fed.scopeProviders")]] : [])];

    return (
        <>
            <Section title={t("fed.tokenTitle")}>
                <Rows>
                    {/* CE QU'IL OUVRE, UNE LIGNE PAR PORTÉE (le porteur, 2026-11-16 : « change la ligne "ce qu'il ouvre" pour le
                        faire passer en 3 lignes : jeux, console et métadonnées. Comme valeur ce serait : autorisé ou pas
                        autorisé ») : elles suivent les interrupteurs plus bas dès qu'ils basculent (`shown`). Les jeux et leurs
                        métadonnées vont toujours ensemble, par `catalog` (« La métadonnée vient toujours du serveur. C'est
                        toujours autorisé comme les jeux. C'est l'utilisation des fournisseurs metadata du serveur qui doit être
                        autorisée ») : seuls les consoles et les fournisseurs se règlent. */}
                    {rows.map(([scope, icon, label]) => {
                        const allowed = scopesOf(shown).includes(scope);

                        return <InfoRow key={`${scope}-${icon}`} icon={icon} tone={allowed ? "green" : "slate"} label={label} value={t(allowed ? "fed.allowed" : "fed.notAllowed")} />;
                    })}
                    {/* Le libellé dit déjà « Dernière utilisation » : la valeur n'est que la date (chantier 59, P1 : le libellé et la valeur
                        disaient la même chose) ; la ligne de la liste garde sa phrase entière (`lastUseOf`). */}
                    <InfoRow icon="bi-clock-history" label={t("fed.lastUseLabel")}
                        value={shown.last_used_at ? dateTime(shown.last_used_at, i18n.language) : t("members.tokenNever")} />
                </Rows>
            </Section>
            {offers.devices && onScopes && (
                <Section title={t("fed.scope.devices")}>
                    <Rows>
                        <SwitchRow id={`fd-dev-${shown.id}`} label={t("fed.devicesLabel")} help={t("fed.devicesHelp")} checked={shared.devices} disabled={pending}
                            onChange={(next) => setShared("devices", next)} />
                    </Rows>
                </Section>
            )}
            {offers.providers && onScopes && (
                <Section title={t(`fed.scope.${PROVIDERS}`)}>
                    <Rows>
                        <SwitchRow id={`fd-prov-${shown.id}`} label={t("fed.providersLabel")} help={t("fed.providersHelp")} checked={shared.providers} disabled={pending}
                            onChange={(next) => setShared("providers", next)} />
                    </Rows>
                </Section>
            )}
            <DangerSection id={`fd-rev-${shown.id}`} help={t("fed.revokeHelp")} label={t("fed.revokeTitle")} confirmLabel={t("fed.revoke")} question={`${t("fed.revokeConfirm")} ${shown.label}`}
                onConfirm={async () => {
                    const answer = await onRevoke();
                    toast(answer.ok ? t("fed.revoked", { name: shown.label }) : answer.detail || t("settings.actionFailed"));

                    if (answer.ok) {
                        onRevoked();
                    }
                }} />
        </>
    );
}

function TokenPage() {
    const { ACCESSES } = useFederationPaths();
    const { t } = useTranslation();
    const { tokenId } = useParams();
    const { back } = useSettingsNavigation();
    const { api, offers } = useFederationHost();
    const passed = usePassed("token", tokenId, () => loadTokens(api));
    const levels = useLevels();

    if (!passed) {
        return <Waiting item={passed} title={t("fed.accessesTitle")} parent={ACCESSES} />;
    }

    return (
        <Frame title={passed.label} parent={ACCESSES} levels={levels}>
            <IssuedTokenSections token={passed} offers={offers} onScopes={(scopes) => api.updateTokenScopes(passed.id, scopes)}
                onRevoke={() => api.revokeToken(passed.id)} onRevoked={() => back(ACCESSES)} />
        </Frame>
    );
}

// /settings/federation : LES DEUX BOUTS DE LA CONNEXION, deux lignes d'abord (le porteur, 2026-11-16 : « en premier lieu
// on voit les 2 liens » ; « on voit les amis avec qui on a partagé nos jeux et consoles et on voit aussi les amis qui
// nous ont partagé leurs jeux et consoles ») — les accès délivrés (si l'hôte en émet : un serveur, un téléphone aussi depuis `#756`), les
// serveurs rejoints —, chacune avec son nombre ; puis le guide. Rien sur le jeu chez un ami : c'est le Profil.
function FederationPage() {
    const { ACCESSES, JOINED } = useFederationPaths();
    const { t } = useTranslation();
    const host = useFederationHost();
    const { open } = useSettingsNavigation();
    const issues = host.useIssues();
    const joined = host.useJoinedCount();
    const [delivered, setDelivered] = useState(null);
    const pairing = host.usePairing();

    useEffect(() => {
        let cancelled = false;

        if (issues) {
            loadTokens(host.api).then((tokens) => !cancelled && setDelivered(tokens.length));
        }

        return () => {
            cancelled = true;
        };
    }, [issues]);

    return (
        <Frame title={host.useHomeTitle() || t("settings.items.federation.label")} parent={host.joins ? undefined : null}>
            {host.HomeBefore && <host.HomeBefore />}
            <Section title={t("fed.connectionTitle")} help={t(!issues ? "fed.connectionHelpJoinOnly" : host.joins ? "fed.connectionHelp" : "fed.connectionHelpServeOnly")}>
                <Rows>
                    {issues && <NavRow id="fd-accesses" icon="bi-key" tone="amber" label={t("fed.accessesTitle")} help={t("fed.accessesHelp")} value={delivered}
                        onOpen={() => open(ACCESSES, "fd-accesses")} />}
                    {/* REJOINDRE, SEULEMENT CHEZ UN HÔTE QUI REJOINT : gameflex-federation ne fait que prêter (le porteur,
                        2026-11-18 : « On fait que du output et non du input par fédération »). */}
                    {host.joins && <NavRow id="fd-joined" icon="bi-lightning-charge" tone="amber" label={t("fed.joinedTitle")} help={t("fed.joinedHelp")} value={joined}
                        onOpen={() => open(JOINED, "fd-joined")} />}
                </Rows>
            </Section>
            {/* LE MODE SERVEUR (chantier 60) : un hôte qui émet des accès, serveur ou téléphone (`#756`). */}
            {issues && <DiscoveryModeSection scope="server" pairing={pairing} />}
            {/* JOUER SUR LES CONSOLES D'UN AMI N'EST PAS ICI (le porteur, 2026-11-16 : « Dans fédération (admin) retire la
                section plus. On n'a pas besoin ça ici. C'est dans profile qu'il le faut ») : ni ligne, ni formulaire de
                compte côté administrateur — Réglages › Profil › Fédération (MyFederation.jsx). */}
            {host.HomeAfter && <host.HomeAfter />}
            <GuideSection guides={FEDERATION_GUIDES} />
        </Frame>
    );
}

export function FederationRoutes() {
    const { FEDERATION } = useFederationPaths();
    const { joins } = useFederationHost();

    return (
        <Routes>
            <Route index element={<FederationPage />} />
            <Route path="access" element={<AccessesPage />} />
            <Route path="pending" element={<PendingPage />} />
            <Route path="new-token" element={<NewTokenPage />} />
            <Route path="new-pairing" element={<NewPairingPage />} />
            {joins && <Route path="add" element={<AddServerPage />} />}
            {joins && <Route path="servers" element={<JoinedPage />} />}
            {joins && <Route path="servers/:serverId" element={<ServerPage />} />}
            <Route path="invitations/:invitationId" element={<InvitationPage />} />
            <Route path="tokens/:tokenId" element={<TokenPage />} />
            <Route path="*" element={<Navigate to={FEDERATION} replace />} />
        </Routes>
    );
}
