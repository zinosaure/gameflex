import React, { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Navigate, Route, Routes, useLocation, useNavigate, useParams } from "react-router-dom";
import * as api from "./api.js";
import { ButtonPlace, MUTED } from "../libs/ui.js";
import { qrImage } from "../libs/share-link.js";
import { focusElement, useScreenInput } from "../libs/input.js";
import { toast } from "../shell/Shell.jsx";
import NavRow, { CopyField, DangerRow, EmptyRow, InfoRow, Note, PageHeader, Rows, Section, SelectRow, SubmitRow, SwitchRow, TextRow } from "../ui/Bricks.jsx";
import { formatSize } from "../libs/format.js";
import { Form, formContext, FormButton } from "../ui/Forms.jsx";

// L'INTERFACE DE gameflex-federation (chantier 61 ; le porteur, 2026-11-18 : « tu reprends la même design "settings" et
// "login" de edge […] dans l'interface on voit les connexions rajoutées. Et dans section connexion on verra les liens
// pour créer des tokens/connexion avec le serveur. Et ensuite la section more pour rajouter un Samba, WebDAV, kDrive,
// FTP, SFTP ») : les briques, la connexion et le cadre des Réglages d'edge, autour des routes du service.
//
//   /                       l'accueil : ce service, les connexions, « Plus » (rejoindre, ressources, journaux), se déconnecter
//   /connections/<id>       une connexion : son état, la relire, la modifier, la retirer
//   /disks/<nom>            une ressource interne : son état, la relire
//   /join                   rejoindre le service : jeton, invitation et QR, code d'appairage
//   /resources              les ressources internes, et ajouter un Samba, WebDAV, kDrive, FTP, SFTP
//   /link/token             un jeton à coller dans un appareil
//   /link/invitation        un lien d'invitation et son QR code
//   /link/pairing           le mode découverte et le code d'appairage
//   /more/<genre>           ajouter un partage Samba, WebDAV, kDrive, FTP ou SFTP
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

function Home() {
    const { t, i18n } = useTranslation();
    const navigate = useNavigate();
    const [resources] = useLive(api.listResources);
    const [disks] = useLive(api.listDisks);
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
    const present = (disks || []).filter((disk) => disk.present);
    const loading = resources === undefined || disks === undefined;

    // L'ORDRE (le porteur, 2026-11-18) : « 1. Ce service 2. Connexions 3. Section "Plus" avec : rejoindre le service, les
    // ressources, logs 4. Déconnexion ».
    return (
        <Page title={settings?.serverName || t("fedService.brand")} back={null}>
            <Section title={t("fedService.server.title")}>
                <form onSubmit={saveName}>
                    <Rows>
                        <TextRow id="fs-name" label={t("fedService.server.name")} help={t("fedService.server.nameHelp")} context={formContext(name, setName)} />
                        <SubmitRow><FormButton id="fs-save" label={t("fedService.save")} isReplaced={Boolean(settings && name["fs-name"] && name["fs-name"] !== settings.serverName)} pending={saving} /></SubmitRow>
                    </Rows>
                </form>
            </Section>
            <Section title={t("fedService.connections.title")} help={t("fedService.connections.help")}>
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
                    <NavRow id="fm-join" icon="bi-link-45deg" tone="indigo" label={t("fedService.join.title")} help={t("fedService.join.help")} onOpen={() => navigate("/join")} />
                    <NavRow id="fm-resources" icon="bi-hdd-stack" tone="teal" label={t("fedService.resources.title")} help={t("fedService.resources.help")} onOpen={() => navigate("/resources")} />
                    <NavRow id="fg-logs" icon="bi-journal-text" tone="slate" label={t("fedService.logs.title")} help={t("fedService.logs.help")} onOpen={() => navigate("/logs")} />
                </Rows>
            </Section>
            <Section>
                <Rows>
                    <NavRow id="fs-logout" icon="bi-box-arrow-right" tone="slate" label={t("fedService.logout")} opens="action" onOpen={() => api.logout()} />
                </Rows>
            </Section>
        </Page>
    );
}

// REJOINDRE LE SERVICE : les trois façons de relier un appareil (l'ancienne section « Connexion »).
function JoinPage() {
    const { t } = useTranslation();
    const navigate = useNavigate();

    return (
        <Page title={t("fedService.join.title")}>
            <Section help={t("fedService.link.help")}>
                <Rows>
                    <NavRow id="fl-token" icon="bi-key" tone="indigo" label={t("fedService.link.token")} help={t("fedService.link.tokenHelp")} onOpen={() => navigate("/link/token")} />
                    <NavRow id="fl-invite" icon="bi-qr-code" tone="teal" label={t("fedService.link.invitation")} help={t("fedService.link.invitationHelp")} onOpen={() => navigate("/link/invitation")} />
                    <NavRow id="fl-pairing" icon="bi-123" tone="violet" label={t("fedService.link.pairing")} help={t("fedService.link.pairingHelp")} onOpen={() => navigate("/link/pairing")} />
                </Rows>
            </Section>
        </Page>
    );
}

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

// ── Relier un appareil ─────────────────────────────────────────────────────

// Le nom de l'appareil, et le bouton qui crée ce qui le reliera.
function LabelForm({ id, button, onCreate }) {
    const { t } = useTranslation();
    const [values, setValues] = useState({ [id]: "" });
    const [pending, setPending] = useState(false);

    const submit = async (event) => {
        event.preventDefault();
        setPending(true);
        const done = await onCreate(values[id].trim());
        setPending(false);

        if (done) {
            setValues({ [id]: "" });
        }
    };

    return (
        <form onSubmit={submit}>
            <Rows>
                <TextRow id={id} label={t("fedService.link.device")} help={t("fedService.link.deviceHelp")} context={formContext(values, setValues)} />
                <SubmitRow><FormButton id={`${id}-create`} label={button} isReplaced={Boolean(values[id].trim())} pending={pending} /></SubmitRow>
            </Rows>
        </form>
    );
}

function TokenPage() {
    const { t, i18n } = useTranslation();
    const [tokens, reload] = useLive(api.listTokens);
    const [issued, setIssued] = useState(null);

    const create = async (label) => {
        const result = await api.issueToken(label);
        if (!result.ok) {
            toast(result.errorMessage || t("fedService.failed"));

            return false;
        }
        setIssued(result.data);
        reload();

        return true;
    };

    return (
        <Page title={t("fedService.link.token")} back="/join">
            <Section help={t("fedService.token.intro")}>
                <LabelForm id="ft-label" button={t("fedService.token.create")} onCreate={create} />
            </Section>
            {issued && (
                <Section title={t("fedService.token.issued", { name: issued.token.label })} help={t("fedService.token.once")}>
                    <Rows>
                        <CopyField id="ft-link" label={t("fedService.link.address")} help={t("fedService.link.addressHelp")} value={location.origin} />
                        <CopyField id="ft-value" label={t("fedService.link.token")} value={issued.value} />
                    </Rows>
                </Section>
            )}
            <Section title={t("fedService.token.list")}>
                <Rows>
                    {tokens?.length === 0 && <EmptyRow icon="bi-key" text={t("fedService.token.none")} />}
                    {(tokens || []).map((token) => (
                        <InfoRow key={token.id} icon="bi-key" tone="indigo" label={token.label}
                            value={token.lastUsedAt && !token.lastUsedAt.startsWith("0001") ? t("fedService.token.used", { when: when(token.lastUsedAt, i18n.language) }) : t("fedService.token.unused")}
                            help={t("fedService.token.created", { when: when(token.createdAt, i18n.language) })} />
                    ))}
                </Rows>
            </Section>
            {tokens?.length > 0 && (
                <Section title={t("danger")}>
                    <Rows>
                        {tokens.map((token) => (
                            <DangerRow key={token.id} id={`ft-revoke-${token.id}`} icon="bi-x-circle" label={t("fedService.token.revoke", { name: token.label })}
                                help={t("fedService.token.revokeHelp")} question={t("fedService.token.revokeQuestion", { name: token.label })}
                                onConfirm={async () => {
                                    const result = await api.revokeToken(token.id);
                                    toast(result.ok ? t("fedService.token.revoked") : result.errorMessage || t("fedService.failed"));
                                    reload();
                                }} />
                        ))}
                    </Rows>
                </Section>
            )}
        </Page>
    );
}

function InvitationPage() {
    const { t, i18n } = useTranslation();
    const [invitations, reload] = useLive(api.listInvitations);
    const [created, setCreated] = useState(null);

    const create = async (label) => {
        const result = await api.createInvitation(label);
        if (!result.ok) {
            toast(result.errorMessage || t("fedService.failed"));

            return false;
        }
        setCreated({ ...result.data, link: `${location.origin}/invite/${result.data.code}` });
        reload();

        return true;
    };
    const pending = (invitations || []).filter((item) => (!item.usedAt || item.usedAt.startsWith("0001")) && new Date(item.expiresAt) > new Date());

    return (
        <Page title={t("fedService.link.invitation")} back="/join">
            <Section help={t("fedService.invitation.intro")}>
                <LabelForm id="fi-label" button={t("fedService.invitation.create")} onCreate={create} />
            </Section>
            {created && (
                <Section title={t("fedService.invitation.created", { name: created.invitation.label })}
                    help={t("fedService.invitation.until", { when: when(created.invitation.expiresAt, i18n.language) })}>
                    <div className="gf-qr mx-auto mb-3" style={{ maxWidth: 240 }}>
                        <img className="d-block w-100 rounded bg-white p-2" src={qrImage(created.link)} alt={t("fedService.invitation.qrAlt")} />
                    </div>
                    <Rows>
                        <CopyField id="fi-link" label={t("fedService.invitation.link")} value={created.link} />
                    </Rows>
                </Section>
            )}
            <Section title={t("fedService.invitation.list")}>
                <Rows>
                    {pending.length === 0 && <EmptyRow icon="bi-qr-code" text={t("fedService.invitation.none")} />}
                    {pending.map((item) => (
                        <InfoRow key={item.id} icon="bi-qr-code" tone="teal" label={item.label} value={t("fedService.invitation.until", { when: when(item.expiresAt, i18n.language) })} />
                    ))}
                </Rows>
            </Section>
            {pending.length > 0 && (
                <Section title={t("danger")}>
                    <Rows>
                        {pending.map((item) => (
                            <DangerRow key={item.id} id={`fi-revoke-${item.id}`} icon="bi-x-circle" label={t("fedService.invitation.revoke", { name: item.label })}
                                question={t("fedService.invitation.revokeQuestion", { name: item.label })}
                                onConfirm={async () => {
                                    const result = await api.revokeInvitation(item.id);
                                    toast(result.ok ? t("fedService.invitation.revoked") : result.errorMessage || t("fedService.failed"));
                                    reload();
                                }} />
                        ))}
                    </Rows>
                </Section>
            )}
        </Page>
    );
}

function PairingPage() {
    const { t, i18n } = useTranslation();
    const [pairing] = useLive(api.fetchPairing);
    const [code, setCode] = useState(null);
    const open = Boolean(pairing?.open);

    useEffect(() => {
        if (!open) {
            setCode(null);
        }
    }, [open]);

    const create = async (label) => {
        const result = await api.createPairingCode(label);
        if (!result.ok) {
            toast(result.status === 423 ? t("fedService.pairing.closedFirst") : result.errorMessage || t("fedService.failed"));

            return false;
        }
        setCode(result.data);

        return true;
    };

    return (
        <Page title={t("fedService.link.pairing")} back="/join">
            <Section help={t("fedService.pairing.intro")}>
                <Rows>
                    <SwitchRow id="fp-discovery" label={t("fedService.pairing.discovery")}
                        help={open ? t("fedService.pairing.openUntil", { when: when(pairing.until, i18n.language) }) : t("fedService.pairing.discoveryHelp")}
                        checked={open} onChange={(checked) => api.setDiscovery(checked)} />
                </Rows>
                {open && !pairing?.announced && <Note>{t("fedService.pairing.notAnnounced")}</Note>}
            </Section>
            {open && (
                <Section title={t("fedService.pairing.code")}>
                    <LabelForm id="fp-label" button={t("fedService.pairing.create")} onCreate={create} />
                </Section>
            )}
            {open && code && (
                <Section title={t("fedService.pairing.yourCode")} help={t("fedService.pairing.codeUntil", { when: when(code.expiresAt, i18n.language) })}>
                    <p className="display-5 fw-bold text-center font-monospace my-3" aria-live="polite">{`${code.code.slice(0, 3)} ${code.code.slice(3)}`}</p>
                </Section>
            )}
        </Page>
    );
}

// ── Les journaux ───────────────────────────────────────────────────────────

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
        <Routes>
            <Route path="/invite/:code" element={<InviteInfo />} />
            {!signedIn ? <Route path="*" element={<Login />} /> : (
                <>
                    <Route path="/" element={<Home />} />
                    <Route path="/connections/:id" element={<ResourcePage />} />
                    <Route path="/disks/:name" element={<DiskPage />} />
                    <Route path="/join" element={<JoinPage />} />
                    <Route path="/resources" element={<ResourcesPage />} />
                    <Route path="/more/:kind" element={<AddResource />} />
                    <Route path="/link/token" element={<TokenPage />} />
                    <Route path="/link/invitation" element={<InvitationPage />} />
                    <Route path="/link/pairing" element={<PairingPage />} />
                    <Route path="/logs" element={<LogsPage />} />
                    <Route path="/logs/view" element={<LogView />} />
                    <Route path="*" element={<Navigate to="/" replace />} />
                </>
            )}
        </Routes>
    );
}
