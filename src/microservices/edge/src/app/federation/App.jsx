import React, { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import * as api from "./api.js";
import { ButtonPlace, MUTED } from "../libs/ui.js";
import { qrImage } from "../libs/share-link.js";
import { focusElement, useScreenInput } from "../libs/input.js";
import { toast } from "../shell/Shell.jsx";
import NavRow, { CopyField, DangerRow, EmptyRow, InfoRow, Listing, Note, PageHeader, Rows, Section, SelectRow, SubmitRow, SwitchRow, TextRow } from "../ui/Bricks.jsx";
import { formatSize } from "../libs/format.js";
import { Form, formContext, FormButton } from "../ui/Forms.jsx";
import { METADATA_SERVICES, MetadataKeysForm } from "../settings/server/Metadata.jsx";
import { FederationRoutes } from "../settings/people/Federation.jsx";
import { FederationHost, GAMEFLEX_HOST } from "../settings/people/federationHost.js";

// L'INTERFACE DE gameflex-federation (chantier 61 ; le porteur, 2026-11-18 : « tu reprends la même design "settings" et
// "login" de edge […] dans l'interface on voit les connexions rajoutées. Et dans section connexion on verra les liens
// pour créer des tokens/connexion avec le serveur. Et ensuite la section more pour rajouter un Samba, WebDAV, kDrive,
// FTP, SFTP ») : les briques, la connexion et le cadre des Réglages d'edge, autour des routes du service.
//
//   /                       l'accueil : ce service, les connexions, les jetons émis, « Plus » (rejoindre, ressources,
//                           fournisseurs, journaux), se déconnecter
//   /connections/<id>       une connexion : son état, la relire, la modifier, la retirer
//   /tokens/<id>            un jeton émis (un appareil relié) : quand il s'en est servi, le révoquer
//   /disks/<nom>            une ressource interne : son état, la relire
//   /join                   rejoindre le service : jeton, invitation et QR, code d'appairage
//   /resources              les ressources internes, et ajouter un Samba, WebDAV, kDrive, FTP, SFTP
//   /link/token             un jeton à coller dans un appareil
//   /link/invitation        un lien d'invitation et son QR code
//   /link/pairing           le mode découverte et le code d'appairage
//   /more/<genre>           ajouter un partage Samba, WebDAV, kDrive, FTP ou SFTP
//   /metadata               les fournisseurs de fiches : les comptes avec lesquels le service fait les fiches (P3)
//   /metadata/<service>     les clés d'un fournisseur, et l'oublier
//   /logs                   les journaux : ce qui s'écrit, et la liste
//   /logs/view?path=        un journal : ses dernières lignes, le télécharger, le supprimer
//   /invite/<code>          un lien d'invitation ouvert dans un navigateur : il se colle dans GameFlex

// Une page : le cadre des Réglages d'edge (une boîte de 600 px, son en-tête, ses sections).
function Page({ title, back = "/", children }) {
    const navigate = useNavigate();
    const box = useRef(null);
    const goBack = back ? () => navigate(back) : null;

    useEffect(() => {
        focusElement(box.current?.querySelector('[data-key="sp-back"]') || box.current?.querySelector("[data-nav]"), true);
    }, [title]);
    useScreenInput({ back: goBack || (() => {}) });

    return (
        <main className="screen tabbed" style={{ "--top": "var(--safe-y)" }} data-nav-layer>
            <div className="gbody d-flex flex-column">
                <div className="lcol px-0 flex-grow-1" data-scroller>
                    <ButtonPlace.Provider value="settings">
                        <div ref={box} className="settings-box w-100 mx-auto">
                            {goBack ? <PageHeader title={title} onBack={goBack} /> : <header className="gf-page-header"><h1 className="ltitle m-0 text-break">{title}</h1></header>}
                            {children}
                        </div>
                    </ButtonPlace.Provider>
                </div>
            </div>
        </main>
    );
}

// Les données qui se relisent quand le service le dit (le flux d'événements), jamais à intervalle.
function useLive(load) {
    const [data, setData] = useState(undefined);
    const reload = useCallback(() => load().then((result) => setData(result.ok ? result.data : null)), [load]);

    useEffect(() => {
        reload();

        return api.subscribe(reload);
    }, [reload]);

    return [data, reload];
}

const when = (value, language) => (value && !value.startsWith("0001") ? new Date(value).toLocaleString(language) : "");

// Le débit mesuré d'une source, lisible.
const speedText = (t, state) => (state?.speedBytesPerSecond ? t("fedService.connections.speedValue", { speed: formatSize(state.speedBytesPerSecond) }) : t("fedService.connections.speedNone"));

// L'ÉPINGLE ET LE DÉBIT d'une source (le porteur, 2026-11-18 : « Oui bonne idée pour épingler une source ») : l'ordre de
// ses copies se lit à sa disponibilité, à son genre selon l'élément et à son débit ; l'épingle la met en tête de tout.
function SourceOrder({ id, pinned, state, onPin }) {
    const { t } = useTranslation();

    return (
        <Section title={t("fedService.connections.order")} help={t("fedService.connections.orderHelp")}>
            <Rows>
                <InfoRow icon="bi-speedometer2" tone="sky" label={t("fedService.connections.speed")} value={speedText(t, state)} />
                <SwitchRow id={`${id}-pin`} label={t("fedService.connections.pin")} help={t("fedService.connections.pinHelp")} checked={Boolean(pinned)} onChange={onPin} />
            </Rows>
        </Section>
    );
}

// ── La connexion ───────────────────────────────────────────────────────────

function Login() {
    const { t } = useTranslation();
    const [values, setValues] = useState({ username: "", password: "" });
    const form = formContext(values, setValues);
    const [error, setError] = useState("");
    const [pending, setPending] = useState(false);

    useEffect(() => {
        focusElement(document.querySelector('[data-key="login-user"]'), true);
    }, []);
    useScreenInput({});

    const submit = async (event) => {
        event.preventDefault();
        setPending(true);
        setError("");
        const result = await api.login(values.username, values.password);
        setPending(false);

        if (!result.ok) {
            setError(result.status === 0 ? t("login.unreachable") : t("login.refused"));
        }
    };

    return (
        <main className="login">
            <form className="d-flex flex-column gap-3" onSubmit={submit}>
                <h1 className="mb-0 fw-bold">{t("fedService.brand")}</h1>
                <p className="text-body-secondary mb-2">{t("fedService.login.help")}</p>
                <Form.Text name="username" context={form} props={{ "data-nav": true, "data-key": "login-user", autoComplete: "username", placeholder: t("login.username"), "aria-label": t("login.username") }} />
                <Form.Password name="password" context={form} props={{ "data-nav": true, "data-key": "login-password", autoComplete: "current-password", placeholder: t("login.password"), "aria-label": t("login.password") }} />
                {error && <p className="small text-danger mb-0" role="alert">{error}</p>}
                <FormButton id="login-submit" label={t("login.submit")} isReplaced={Boolean(values.username || values.password)} pending={pending} disabled={!values.username || !values.password} />
            </form>
        </main>
    );
}

// ── L'accueil ──────────────────────────────────────────────────────────────

const stateText = (t, i18n, state) => {
    if (state?.scanning) {
        return t("fedService.connections.scanning");
    }
    if (state?.error) {
        return t("fedService.connections.failed", { error: state.error });
    }
    if (!state?.scannedAt || state.scannedAt.startsWith("0001")) {
        return t("fedService.connections.never");
    }

    return t("fedService.connections.games", { count: state.games, when: when(state.scannedAt, i18n.language) });
};

// L'ACCUEIL DU SERVICE EST CELUI DE RÉGLAGES › FÉDÉRATION D'EDGE (chantier 61, option B ; le porteur, 2026-11-18 :
// « prendre la même interface et fonctionnalités que dans /settings/federation […] la seule chose qui différencie c'est
// le fait de pouvoir rajouter des ressources externes ») : ses pages (accès délivrés, invitations, jeton, code
// d'appairage, mode découverte) viennent d'edge (FederationRoutes), servies par l'hôte du service (SERVICE_HOST). Le
// service y ajoute, avant : son nom (« Ce service ») ; après : ses ressources internes (disques et partages), « Plus »
// (Ressources, Fournisseurs de métadonnées, Journaux), se déconnecter. Rien pour rejoindre un serveur : le service ne
// fait que prêter.
function ServiceName() {
    const { t } = useTranslation();
    const [settings, reloadSettings] = useLive(api.fetchSettings);
    const [name, setName] = useState({ "fs-name": "" });
    const [saving, setSaving] = useState(false);

    useEffect(() => {
        if (settings) {
            setName({ "fs-name": settings.serverName });
        }
    }, [settings]);

    const saveName = async (event) => {
        event.preventDefault();
        setSaving(true);
        const result = await api.saveSettings({ serverName: name["fs-name"] });
        setSaving(false);
        toast(result.ok ? t("fedService.server.saved") : result.errorMessage || t("fedService.failed"));
        reloadSettings();
    };

    return (
        <Section title={t("fedService.server.title")}>
            <form onSubmit={saveName}>
                <Rows>
                    <TextRow id="fs-name" label={t("fedService.server.name")} help={t("fedService.server.nameHelp")} context={formContext(name, setName)} />
                    <SubmitRow><FormButton id="fs-save" label={t("fedService.save")} isReplaced={Boolean(settings && name["fs-name"] && name["fs-name"] !== settings.serverName)} pending={saving} /></SubmitRow>
                </Rows>
            </form>
        </Section>
    );
}

function ServiceResources() {
    const { t, i18n } = useTranslation();
    const navigate = useNavigate();
    const [resources] = useLive(api.listResources);
    const [disks] = useLive(api.listDisks);
    const present = (disks || []).filter((disk) => disk.present);
    const loading = resources === undefined || disks === undefined;

    return (
        <>
            <Section title={t("fedService.resources.internal")} help={t("fedService.connections.help")}>
                <Rows>
                    {loading && <EmptyRow icon="bi-hourglass" text={t("fedService.loading")} />}
                    {!loading && resources.length === 0 && present.length === 0 && <EmptyRow icon="bi-plug" text={t("fedService.connections.none")} />}
                    {present.map((disk) => (
                        <NavRow key={`disk-${disk.name}`} id={`fd-${disk.name}`} icon={api.DISK.icon} tone={disk.state?.error ? "rose" : api.DISK.tone}
                            label={disk.name} help={`${t("fedService.kinds.disk.label")} · ${stateText(t, i18n, disk.state)}`} pending={Boolean(disk.state?.scanning)}
                            onOpen={() => navigate(`/disks/${encodeURIComponent(disk.name)}`)} />
                    ))}
                    {(resources || []).map((resource) => {
                        const kind = api.kindOf(resource.kind);

                        return (
                            <NavRow key={resource.id} id={`fc-${resource.id}`} icon={kind.icon} tone={resource.state?.error ? "rose" : kind.tone}
                                label={resource.name} help={`${t(`fedService.kinds.${resource.kind}.label`)} · ${stateText(t, i18n, resource.state)}`}
                                pending={Boolean(resource.state?.scanning)} onOpen={() => navigate(`/connections/${resource.id}`)} />
                        );
                    })}
                </Rows>
            </Section>
            <Section title={t("fedService.more.title")}>
                <Rows>
                    <NavRow id="fm-resources" icon="bi-hdd-stack" tone="teal" label={t("fedService.resources.title")} help={t("fedService.resources.help")} onOpen={() => navigate("/resources")} />
                    <NavRow id="fm-metadata" icon="bi-card-text" tone="amber" label={t("fedService.metadata.title")} help={t("fedService.metadata.help")} onOpen={() => navigate("/metadata")} />
                    <NavRow id="fg-logs" icon="bi-journal-text" tone="slate" label={t("fedService.logs.title")} help={t("fedService.logs.help")} onOpen={() => navigate("/logs")} />
                </Rows>
            </Section>
            <Section>
                <Rows>
                    <NavRow id="fs-logout" icon="bi-box-arrow-right" tone="slate" label={t("fedService.logout")} opens="action" onOpen={() => api.logout()} />
                </Rows>
            </Section>
        </>
    );
}

// LE SUJET `pairing` DU SERVICE : son état, relu quand le flux d'événements le dit (jamais à intervalle).
function useServicePairing() {
    const [state] = useLive(api.fetchPairing);

    return state || null;
}

function useServiceName() {
    const [settings] = useLive(api.fetchSettings);

    return settings?.serverName || "";
}

// Le QR d'un lien, dessiné ici (le service n'a pas la route du kernel) : un Blob SVG.
const serviceQR = (text) => fetch(qrImage(text)).then((response) => response.blob()).catch(() => null);

// Les appels du service, aux formes d'edge (federationHost.js) : {ok, status, item | items, detail}.
const answer = (result, shape = (data) => ({ item: data })) => ({ ok: result.ok, status: result.status, detail: result.errorMessage, ...(result.ok ? shape(result.data) : {}) });
const used = (value) => (value && !String(value).startsWith("0001") ? value : null);
const tokenView = (token) => ({ id: token.id, label: token.label, scopes: (token.scopes || []).join(","), last_used_at: used(token.lastUsedAt), created_at: token.createdAt });

// L'HÔTE DES PAGES DE LA FÉDÉRATION, côté service : ses jetons — le catalogue, et, au choix, ses fournisseurs de
// métadonnées (« Partager aussi mes fournisseurs de métadonnées », comme GameFlex) ; pas de consoles —, ses invitations,
// son mode découverte et ses codes.
const SERVICE_HOST = {
    ...GAMEFLEX_HOST,
    base: "/",
    Page: ({ title, parent, children }) => <Page title={title} back={parent === null ? null : parent || "/"}>{children}</Page>,
    joins: false,
    offers: { devices: false, providers: true },
    useIssues: () => true,
    useServerName: useServiceName,
    useOrigin: () => location.origin,
    usePairing: useServicePairing,
    useJoinedCount: () => 0,
    useInApp: () => false,
    invitationQR: serviceQR,
    useHomeTitle: () => useServiceName() || null,
    HomeBefore: ServiceName,
    HomeAfter: ServiceResources,
    api: {
        listTokens: async () => answer(await api.listTokens(), (data) => ({ items: (data || []).map(tokenView) })),
        createToken: async (label, scopes) => answer(await api.issueToken(label, scopes), (data) => ({ item: { ...tokenView(data.token), token: data.value } })),
        updateTokenScopes: async (id, scopes) => answer(await api.updateTokenScopes(id, scopes), (data) => ({ item: tokenView(data) })),
        revokeToken: async (id) => answer(await api.revokeToken(id)),
        listInvitations: async () => answer(await api.listInvitations(), (data) => ({
            items: (data || []).filter((entry) => !used(entry.usedAt) && Date.parse(entry.expiresAt) > Date.now())
                .map((entry) => ({ id: entry.id, label: entry.label, kind: "federation", scopes: (entry.scopes || ["catalog"]).join(","), expires_at: entry.expiresAt })),
        })),
        createInvitation: async ({ label, scopes }) => answer(await api.createInvitation(label, scopes), (data) => ({
            item: { id: data.invitation.id, code: data.code, label: data.invitation.label, kind: "federation", expires_at: data.invitation.expiresAt },
        })),
        revokeInvitation: async (id) => answer(await api.revokeInvitation(id)),
        createPairingCode: async ({ label, scopes }) => answer(await api.createPairingCode(label, scopes)),
        cancelPairingCode: async (id) => answer(await api.cancelPairingCode(id)),
        openPairing: async () => answer(await api.setDiscovery(true)),
        closePairing: async () => answer(await api.setDiscovery(false)),
    },
};

// LES RESSOURCES : les disques montés dans le conteneur, puis les partages à ajouter (l'ancienne section « Plus »).
function ResourcesPage() {
    const { t, i18n } = useTranslation();
    const navigate = useNavigate();
    const [disks] = useLive(api.listDisks);

    return (
        <Page title={t("fedService.resources.title")}>
            <Section title={t("fedService.resources.internal")} help={t("fedService.resources.internalHelp")}>
                <Rows>
                    {disks === undefined && <EmptyRow icon="bi-hourglass" text={t("fedService.loading")} />}
                    {disks?.length === 0 && <EmptyRow icon="bi-device-hdd" text={t("fedService.resources.noDisk")} />}
                    {(disks || []).map((disk) => (
                        <NavRow key={disk.name} id={`fr-disk-${disk.name}`} icon={api.DISK.icon} tone={disk.present ? api.DISK.tone : "slate"} label={disk.name}
                            help={disk.present ? stateText(t, i18n, disk.state) : t("fedService.resources.diskEmpty")} disabled={!disk.present}
                            onOpen={() => navigate(`/disks/${encodeURIComponent(disk.name)}`)} />
                    ))}
                </Rows>
            </Section>
            <Section title={t("fedService.more.title")} help={t("fedService.more.help")}>
                <Rows>
                    {api.KINDS.map(({ kind, icon, tone }) => (
                        <NavRow key={kind} id={`fm-${kind}`} icon={icon} tone={tone} label={t(`fedService.kinds.${kind}.label`)} help={t(`fedService.kinds.${kind}.help`)}
                            onOpen={() => navigate(`/more/${kind}`)} />
                    ))}
                </Rows>
            </Section>
        </Page>
    );
}

// UNE RESSOURCE INTERNE : son état, et la relire. Elle se déclare dans le compose, pas ici : ni modifier ni retirer.
function DiskPage() {
    const { t, i18n } = useTranslation();
    const { name } = useParams();
    const [disks] = useLive(api.listDisks);
    const disk = (disks || []).find((entry) => entry.name === name);

    if (disks === undefined) {
        return <Page title={t("fedService.loading")}><Note>{t("fedService.loading")}</Note></Page>;
    }
    if (!disk) {
        return <Navigate to="/resources" replace />;
    }
    const state = disk.state || {};

    return (
        <Page title={disk.name} back="/resources">
            <Section help={t("fedService.resources.diskHelp")}>
                <Rows>
                    <InfoRow icon={api.DISK.icon} tone={api.DISK.tone} label={t("fedService.fields.kind")} value={t("fedService.kinds.disk.label")} />
                    <InfoRow label={t("fedService.resources.mount")} value={`/mnt/${disk.name}`} mono />
                    <InfoRow label={t("fedService.connections.gamesFound")} value={state.scannedAt && !state.scannedAt.startsWith("0001") ? String(state.games) : ""} />
                    <InfoRow label={t("fedService.connections.lastScan")} value={when(state.scannedAt, i18n.language)} />
                    <InfoRow icon="bi-exclamation-triangle" tone="rose" label={t("fedService.connections.error")} value={state.error || ""} />
                </Rows>
            </Section>
            <Section>
                <Rows>
                    <NavRow id="fd-rescan" icon="bi-arrow-clockwise" tone="teal" label={t("fedService.connections.rescan")} help={t("fedService.resources.rescanDiskHelp")}
                        pending={Boolean(state.scanning)} opens="action" onOpen={() => api.rescanDisk(disk.name)} />
                </Rows>
            </Section>
            <SourceOrder id="fd" pinned={disk.pinned} state={state} onPin={(on) => api.pinDisk(disk.name, on)} />
        </Page>
    );
}

// ── Une connexion ──────────────────────────────────────────────────────────

const EMPTY_RESOURCE = { name: "", address: "", folder: "", username: "", password: "" };

// Les champs d'une connexion, selon son genre : l'adresse, le dossier et le compte n'y disent pas la même chose.
function ResourceFields({ kind, form, editing = false }) {
    const { t } = useTranslation();
    const key = (field) => `fedService.kinds.${kind}.${field}`;

    return (
        <>
            <TextRow id="fr-name" label={t("fedService.fields.name")} help={t("fedService.fields.nameHelp")} context={form} />
            <TextRow id="fr-address" label={t(key("address"))} help={t(key("addressHelp"))} context={form} />
            <TextRow id="fr-folder" label={t("fedService.fields.folder")} help={t("fedService.fields.folderHelp")} context={form} />
            {kind !== "kdrive" && <TextRow id="fr-username" label={t("fedService.fields.username")} help={t(key("usernameHelp"))} context={form} />}
            <TextRow id="fr-password" kind="password" label={t(kind === "kdrive" ? "fedService.kinds.kdrive.token" : "fedService.fields.password")}
                help={editing ? t("fedService.fields.passwordKeep") : t(key("passwordHelp"))} context={form} />
        </>
    );
}

// Les champs d'un formulaire de connexion portent le nom de leur clé : TextRow prend l'identifiant pour nom.
const resourceForm = (values, setValues) => {
    const base = formContext(values, setValues);
    const names = { "fr-name": "name", "fr-address": "address", "fr-folder": "folder", "fr-username": "username", "fr-password": "password" };

    return {
        ...base,
        values: Object.fromEntries(Object.entries(names).map(([id, field]) => [id, values[field]])),
        setValues: (patch) => setValues((previous) => {
            const next = { ...previous };
            for (const [id, value] of Object.entries(patch)) {
                next[names[id] || id] = value;
            }

            return next;
        }),
    };
};

function AddResource() {
    const { t } = useTranslation();
    const { kind } = useParams();
    const navigate = useNavigate();
    const [values, setValues] = useState(EMPTY_RESOURCE);
    const [pending, setPending] = useState(false);

    if (!api.KINDS.some((entry) => entry.kind === kind)) {
        return <Navigate to="/resources" replace />;
    }
    const ready = values.name.trim() && values.address.trim() && (kind !== "sftp" || values.username.trim());

    const submit = async (event) => {
        event.preventDefault();
        setPending(true);
        const result = await api.addResource({ ...values, kind });
        setPending(false);

        if (!result.ok) {
            toast(result.errorMessage || t("fedService.failed"));

            return;
        }
        toast(t("fedService.connections.added"));
        navigate(`/connections/${result.data.id}`, { replace: true });
    };

    return (
        <Page title={t(`fedService.kinds.${kind}.add`)} back="/resources">
            <Section help={t(`fedService.kinds.${kind}.intro`)}>
                <form onSubmit={submit}>
                    <Rows>
                        <ResourceFields kind={kind} form={resourceForm(values, setValues)} />
                        <SubmitRow><FormButton id="fr-add" label={t("fedService.connections.add")} isReplaced={Boolean(ready)} pending={pending} /></SubmitRow>
                    </Rows>
                </form>
            </Section>
        </Page>
    );
}

function ResourcePage() {
    const { t, i18n } = useTranslation();
    const { id } = useParams();
    const navigate = useNavigate();
    const [resources] = useLive(api.listResources);
    const resource = (resources || []).find((entry) => String(entry.id) === id);
    const [values, setValues] = useState(EMPTY_RESOURCE);
    const [pending, setPending] = useState(false);

    useEffect(() => {
        if (resource) {
            setValues({ name: resource.name, address: resource.address, folder: resource.folder, username: resource.username, password: "" });
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [resource?.id, resource?.name, resource?.address, resource?.folder, resource?.username]);

    if (resources === undefined) {
        return <Page title={t("fedService.loading")}><Note>{t("fedService.loading")}</Note></Page>;
    }
    if (!resource) {
        return <Navigate to="/" replace />;
    }
    const kind = api.kindOf(resource.kind);
    const state = resource.state || {};
    const changed = values.name !== resource.name || values.address !== resource.address || values.folder !== resource.folder
        || values.username !== resource.username || values.password !== "";

    const save = async (event) => {
        event.preventDefault();
        setPending(true);
        const result = await api.updateResource(resource.id, { ...values, kind: resource.kind });
        setPending(false);
        toast(result.ok ? t("fedService.connections.saved") : result.errorMessage || t("fedService.failed"));
    };

    return (
        <Page title={resource.name}>
            <Section>
                <Rows>
                    <InfoRow icon={kind.icon} tone={kind.tone} label={t("fedService.fields.kind")} value={t(`fedService.kinds.${resource.kind}.label`)} />
                    <InfoRow label={t(`fedService.kinds.${resource.kind}.address`)} value={resource.address} mono />
                    <InfoRow label={t("fedService.fields.folder")} value={resource.folder} mono />
                    <InfoRow label={t("fedService.connections.gamesFound")} value={state.scannedAt && !state.scannedAt.startsWith("0001") ? String(state.games) : ""} />
                    <InfoRow label={t("fedService.connections.lastScan")} value={when(state.scannedAt, i18n.language)} />
                    <InfoRow icon="bi-exclamation-triangle" tone="rose" label={t("fedService.connections.error")} value={state.error || ""} />
                </Rows>
            </Section>
            <Section>
                <Rows>
                    <NavRow id="fr-rescan" icon="bi-arrow-clockwise" tone="teal" label={t("fedService.connections.rescan")} help={t("fedService.connections.rescanHelp")}
                        pending={Boolean(state.scanning)} opens="action" onOpen={() => api.rescanResource(resource.id)} />
                </Rows>
            </Section>
            <SourceOrder id="fr" pinned={resource.pinned} state={state} onPin={(on) => api.pinResource(resource.id, on)} />
            <Section title={t("fedService.connections.edit")}>
                <form onSubmit={save}>
                    <Rows>
                        <ResourceFields kind={resource.kind} form={resourceForm(values, setValues)} editing />
                        <SubmitRow><FormButton id="fr-save" label={t("fedService.save")} isReplaced={changed} pending={pending} /></SubmitRow>
                    </Rows>
                </form>
            </Section>
            <Section title={t("danger")}>
                <Rows>
                    <DangerRow id="fr-remove" label={t("fedService.connections.remove")} help={t("fedService.connections.removeHelp")}
                        question={t("fedService.connections.removeQuestion", { name: resource.name })}
                        onConfirm={async () => {
                            const result = await api.removeResource(resource.id);
                            toast(result.ok ? t("fedService.connections.removed") : result.errorMessage || t("fedService.failed"));
                            if (result.ok) {
                                navigate("/", { replace: true });
                            }
                        }} />
                </Rows>
            </Section>
        </Page>
    );
}

// ── Les fournisseurs de fiches (P3) ────────────────────────────────────────
//
// LES FICHES DES JEUX QUI N'EN ONT PAS (question 2 A) : un jeu d'un disque qui porte sa fiche la prête telle quelle ; les
// autres — un jeu d'un partage, un jeu de disque sans fiche — ont celle que le service fait avec SES comptes. Les mêmes
// fournisseurs et le même formulaire que Réglages › Fournisseur de métadonnées d'edge ; les valeurs entrent et ne
// ressortent jamais.
const ACCOUNT_SERVICES = METADATA_SERVICES.filter((service) => service.fields);

function MetadataPage() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const [statuses] = useLive(api.listMetadataServices);

    return (
        <Page title={t("fedService.metadata.title")}>
            <Section title={t("settings.metadataServices")} help={t("fedService.metadata.listHelp")}>
                <Rows>
                    {statuses === undefined && <EmptyRow icon="bi-hourglass" text={t("fedService.loading")} />}
                    {statuses !== undefined && ACCOUNT_SERVICES.map((service) => {
                        const status = (statuses || []).find((entry) => entry.service === service.key);

                        return (
                            <NavRow key={service.key} id={`fmd-${service.key}`} icon={service.icon} tone={service.tone} label={t(`settings.metadata.${service.key}.label`)}
                                help={t(`settings.metadata.${service.key}.summary`)} value={status ? t(status.configured ? "settings.configured" : "settings.notConfigured") : null}
                                onOpen={() => navigate(`/metadata/${service.key}`)} />
                        );
                    })}
                </Rows>
            </Section>
        </Page>
    );
}

function MetadataServicePage() {
    const { t } = useTranslation();
    const { service: key } = useParams();
    const navigate = useNavigate();
    const [statuses, reload] = useLive(api.listMetadataServices);
    const service = ACCOUNT_SERVICES.find((entry) => entry.key === key);
    const status = (statuses || []).find((entry) => entry.service === key) || null;
    const name = service ? t(`settings.metadata.${service.key}.label`) : key;
    const stored = (status?.fields || []).length > 0;
    const save = async (serviceKey, fields) => {
        const result = await api.saveMetadataService(serviceKey, fields);

        return { ok: result.ok, items: result.data };
    };

    if (!service) {
        return <Navigate to="/metadata" replace />;
    }

    return (
        <Page title={name} back="/metadata">
            {statuses === undefined ? <Section><Note className="mt-0">{t("fedService.loading")}</Note></Section> : (
                <>
                    <MetadataKeysForm service={service} status={status} save={save} onSaved={() => reload()} />
                    <Section title={t("danger")}>
                        <Rows>
                            <DangerRow id="fmd-remove" label={t("settings.metadataRemove")} help={t(stored ? "settings.metadataRemoveHelp" : "settings.metadataNothingStored", { name })}
                                question={t("settings.metadataRemoveConfirm", { name })} disabled={!stored}
                                onConfirm={async () => {
                                    const result = await api.removeMetadataService(service.key);
                                    toast(result.ok ? t("settings.metadataRemoved", { name }) : result.errorMessage || t("fedService.failed"));
                                    if (result.ok) {
                                        navigate("/metadata", { replace: true });
                                    }
                                }} />
                        </Rows>
                    </Section>
                </>
            )}
        </Page>
    );
}

// ── Relier un appareil ─────────────────────────────────────────────────────

const LEVELS = ["debug", "info", "warn", "error"];

function LogsPage() {
    const { t, i18n } = useTranslation();
    const navigate = useNavigate();
    const [settings, setSettings] = useState(null);
    const [logs, setLogs] = useState(undefined);

    useEffect(() => {
        api.fetchLogSettings().then((result) => setSettings(result.ok ? result.data : null));
        api.listLogs().then((result) => setLogs(result.ok ? result.data || [] : []));
    }, []);

    const change = async (patch) => {
        const next = { ...settings, ...patch };
        setSettings(next);
        const result = await api.saveLogSettings(next);
        if (result.ok) {
            setSettings(result.data);
        } else {
            toast(result.errorMessage || t("fedService.failed"));
        }
    };
    const toggleLevel = (level, on) => change({ levels: on ? [...settings.levels, level] : settings.levels.filter((item) => item !== level) });
    const sorted = [...(logs || [])].sort((a, b) => (a.path < b.path ? 1 : -1));

    return (
        <Page title={t("fedService.logs.title")}>
            {settings && (
                <Section title={t("fedService.logs.writing")} help={t("fedService.logs.writingHelp")}>
                    <Rows>
                        <SwitchRow id="fg-on" label={t("fedService.logs.enabled")} help={t("fedService.logs.enabledHelp")} checked={settings.enabled} onChange={(on) => change({ enabled: on })} />
                        {LEVELS.map((level) => (
                            <SwitchRow key={level} id={`fg-level-${level}`} label={t(`fedService.logs.levels.${level}`)} checked={settings.levels.includes(level)}
                                disabled={!settings.enabled} onChange={(on) => toggleLevel(level, on)} />
                        ))}
                        <SwitchRow id="fg-http" label={t("fedService.logs.http")} help={t("fedService.logs.httpHelp")} checked={settings.http}
                            disabled={!settings.enabled} onChange={(on) => change({ http: on })} />
                    </Rows>
                </Section>
            )}
            <Section title={t("fedService.logs.files")}>
                <Rows>
                    {logs === undefined && <EmptyRow icon="bi-hourglass" text={t("fedService.loading")} />}
                    {logs?.length === 0 && <EmptyRow icon="bi-journal" text={t("fedService.logs.none")} />}
                    {sorted.map((entry) => (
                        <NavRow key={entry.path} id={`fg-${entry.path}`} icon="bi-journal-text" tone="slate" label={entry.path}
                            help={t("fedService.logs.size", { size: formatSize(entry.sizeBytes), when: when(entry.modifiedAt, i18n.language) })}
                            onOpen={() => navigate(`/logs/view?path=${encodeURIComponent(entry.path)}`)} />
                    ))}
                </Rows>
            </Section>
        </Page>
    );
}

const LEVEL_TONES = { DEBUG: "text-body-secondary", INFO: "text-info", WARN: "text-warning", ERROR: "text-danger" };

function LogView() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const path = new URLSearchParams(useLocation().search).get("path") || "";
    const [lines, setLines] = useState(undefined);
    const [values, setValues] = useState({ "fg-filter": "" });
    const filter = values["fg-filter"];

    useEffect(() => {
        api.tailLog(path).then((result) => setLines(result.ok ? result.data || [] : null));
    }, [path]);

    const shown = (lines || []).filter((line) => !filter || (line.level || "").toLowerCase() === filter);

    return (
        <Page title={path} back="/logs">
            <Section>
                <Rows>
                    <SelectRow id="fg-filter" label={t("fedService.logs.filter")} value={filter}
                        options={[{ value: "", label: t("fedService.logs.allLevels") }, ...LEVELS.map((level) => ({ value: level, label: t(`fedService.logs.levels.${level}`) }))]}
                        onChange={(value) => setValues({ "fg-filter": value })} />
                </Rows>
            </Section>
            <Section title={t("fedService.logs.lines")} help={t("fedService.logs.linesHelp")}>
                <Rows>
                    {lines === undefined && <EmptyRow icon="bi-hourglass" text={t("fedService.loading")} />}
                    {lines === null && <EmptyRow icon="bi-exclamation-triangle" text={t("fedService.failed")} />}
                    {lines && shown.length === 0 && <EmptyRow icon="bi-journal" text={t("fedService.logs.empty")} />}
                    {shown.slice().reverse().map((line, index) => (
                        <div key={index} className="list-group-item gf-row">
                            <span className="gf-row-text">
                                <span className="gf-row-label text-break">
                                    <span className="font-monospace small me-2 text-body-secondary">{line.clock}</span>
                                    {line.level && <span className={`small fw-semibold me-2 ${LEVEL_TONES[line.level] || ""}`}>{line.level}</span>}
                                    {line.message || line.raw}
                                </span>
                                {line.details && <small className="gf-row-help font-monospace text-break">{line.details}</small>}
                            </span>
                        </div>
                    ))}
                </Rows>
            </Section>
            <Section title={t("fedService.more.title")}>
                <Rows>
                    <NavRow id="fg-download" icon="bi-download" tone="indigo" label={t("fedService.logs.download")} help={t("fedService.logs.downloadHelp")} opens="action"
                        onOpen={() => { window.location.href = api.logDownloadUrl(path); }} />
                </Rows>
            </Section>
            <Section title={t("danger")}>
                <Rows>
                    <DangerRow id="fg-remove" label={t("fedService.logs.remove")} question={t("fedService.logs.removeQuestion", { path })}
                        onConfirm={async () => {
                            const result = await api.removeLog(path);
                            toast(result.ok ? t("fedService.logs.removed") : result.errorMessage || t("fedService.failed"));
                            if (result.ok) {
                                navigate("/logs", { replace: true });
                            }
                        }} />
                </Rows>
            </Section>
        </Page>
    );
}

// Un lien d'invitation ouvert dans un navigateur : il se colle dans GameFlex, il ne s'ouvre pas ici.
function InviteInfo() {
    const { t } = useTranslation();

    return (
        <main className="login">
            <div className="d-flex flex-column gap-3">
                <h1 className="mb-0 fw-bold">{t("fedService.brand")}</h1>
                <p className={MUTED}>{t("fedService.invitation.openedHere")}</p>
            </div>
        </main>
    );
}

export default function App() {
    const [signedIn, setSignedIn] = useState(api.hasSession());

    useEffect(() => api.onSessionChange(setSignedIn), []);

    return (
        <FederationHost.Provider value={SERVICE_HOST}>
        <Routes>
            <Route path="/invite/:code" element={<InviteInfo />} />
            {!signedIn ? <Route path="*" element={<Login />} /> : (
                <>
                    <Route path="/connections/:id" element={<ResourcePage />} />
                    <Route path="/disks/:name" element={<DiskPage />} />
                    <Route path="/resources" element={<ResourcesPage />} />
                    <Route path="/more/:kind" element={<AddResource />} />
                    <Route path="/metadata" element={<MetadataPage />} />
                    <Route path="/metadata/:service" element={<MetadataServicePage />} />
                    <Route path="/logs" element={<LogsPage />} />
                    <Route path="/logs/view" element={<LogView />} />
                    <Route path="/*" element={<FederationRoutes />} />
                </>
            )}
        </Routes>
        </FederationHost.Provider>
    );
}
