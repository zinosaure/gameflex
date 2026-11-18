// LE CLIENT DE L'ADMINISTRATION DE gameflex-federation (chantier 61) : les routes `/api/v1/admin/…` du service, sous la
// session ouverte par la page de connexion (le compte que donne l'environnement du conteneur). La session vit dans CE
// navigateur (localStorage) ; une réponse 401 la retire et ramène à la connexion.
const SESSION_KEY = "gameflex-federation-session";

const read = () => {
    try {
        return localStorage.getItem(SESSION_KEY) || "";
    } catch {
        return "";
    }
};

const write = (value) => {
    try {
        if (value) {
            localStorage.setItem(SESSION_KEY, value);
        } else {
            localStorage.removeItem(SESSION_KEY);
        }
    } catch {
        // Sans stockage, la session ne dure que la page.
    }
};

let session = read();
const listeners = new Set();

export const hasSession = () => Boolean(session);

// Ceux qui veulent savoir que la session est partie (l'application, pour revenir à la connexion).
export const onSessionChange = (listener) => {
    listeners.add(listener);

    return () => listeners.delete(listener);
};

const setSession = (value) => {
    session = value;
    write(value);
    listeners.forEach((listener) => listener(Boolean(value)));
};

// call — { ok, status, data, errorMessage } ; jamais d'exception : une page lit `ok`.
async function call(method, path, body = undefined) {
    try {
        const response = await fetch(path, {
            method,
            headers: { "Content-Type": "application/json", ...(session ? { Authorization: `Bearer ${session}` } : {}) },
            body: body === undefined ? undefined : JSON.stringify(body),
        });
        const answer = await response.json().catch(() => ({}));

        if (response.status === 401 && path !== "/api/v1/admin/login") {
            setSession("");
        }

        return { ok: response.ok && answer.success !== false, status: response.status, data: answer.data, errorMessage: answer.errorMessage || "" };
    } catch {
        return { ok: false, status: 0, data: null, errorMessage: "" };
    }
}

export async function login(username, password) {
    const result = await call("POST", "/api/v1/admin/login", { username, password });

    if (result.ok) {
        setSession(result.data.token);
    }

    return result;
}

export async function logout() {
    await call("POST", "/api/v1/admin/logout");
    setSession("");
}

export const fetchSettings = () => call("GET", "/api/v1/admin/settings");
export const saveSettings = (settings) => call("PUT", "/api/v1/admin/settings", settings);

export const listResources = () => call("GET", "/api/v1/admin/resources");
export const addResource = (resource) => call("POST", "/api/v1/admin/resources", resource);
export const updateResource = (id, resource) => call("PUT", `/api/v1/admin/resources/${id}`, resource);
export const removeResource = (id) => call("DELETE", `/api/v1/admin/resources/${id}`);
export const rescanResource = (id) => call("POST", `/api/v1/admin/resources/${id}/rescan`);

export const listDisks = () => call("GET", "/api/v1/admin/disks");
export const rescanDisk = (name) => call("POST", `/api/v1/admin/disks/${encodeURIComponent(name)}/rescan`);

export const pinResource = (id, pinned) => call("PUT", `/api/v1/admin/resources/${id}/pin`, { pinned });
export const pinDisk = (name, pinned) => call("PUT", `/api/v1/admin/disks/${encodeURIComponent(name)}/pin`, { pinned });

export const listTokens = () => call("GET", "/api/v1/admin/tokens");
export const issueToken = (label) => call("POST", "/api/v1/admin/tokens", { label });
export const revokeToken = (id) => call("DELETE", `/api/v1/admin/tokens/${id}`);

export const listInvitations = () => call("GET", "/api/v1/admin/invitations");
export const createInvitation = (label) => call("POST", "/api/v1/admin/invitations", { label });
export const revokeInvitation = (id) => call("DELETE", `/api/v1/admin/invitations/${id}`);

export const fetchPairing = () => call("GET", "/api/v1/admin/pairing");
export const setDiscovery = (open) => call("POST", "/api/v1/admin/pairing/discovery", { open, link: location.origin });
export const createPairingCode = (label) => call("POST", "/api/v1/admin/pairing/code", { label });

export const fetchLogSettings = () => call("GET", "/api/v1/admin/logs/settings");
export const saveLogSettings = (settings) => call("PUT", "/api/v1/admin/logs/settings", settings);
export const listLogs = () => call("GET", "/api/v1/admin/logs");
export const tailLog = (path) => call("GET", `/api/v1/admin/logs/tail?path=${encodeURIComponent(path)}&lines=300`);
export const removeLog = (path) => call("DELETE", `/api/v1/admin/logs/file?path=${encodeURIComponent(path)}`);
// Un lien ne porte pas d'en-tête : la session voyage sous le nom `token`, que le journal d'accès masque.
export const logDownloadUrl = (path) => `/api/v1/admin/logs/download?path=${encodeURIComponent(path)}&token=${encodeURIComponent(session)}`;

// LE FLUX D'ÉVÉNEMENTS (jamais de sondage) : `onChange` à chaque changement du service — un parcours commencé ou
// fini, une ressource, le mode découverte. Rend de quoi le fermer.
export function subscribe(onChange) {
    if (!session) {
        return () => {};
    }
    const source = new EventSource(`/api/v1/admin/events?token=${encodeURIComponent(session)}`);
    source.addEventListener("changed", () => onChange());

    return () => source.close();
}

// Les genres de connexion, dans l'ordre de « Plus » : leur icône et leur teinte (celles des lignes d'edge).
export const KINDS = [
    { kind: "smb", icon: "bi-hdd-network", tone: "indigo" },
    { kind: "webdav", icon: "bi-cloud", tone: "sky" },
    { kind: "kdrive", icon: "bi-cloud-fill", tone: "teal" },
    { kind: "ftp", icon: "bi-folder-symlink", tone: "amber" },
    { kind: "sftp", icon: "bi-shield-lock", tone: "violet" },
];

// Une ressource interne : un disque monté dans le conteneur.
export const DISK = { kind: "disk", icon: "bi-device-hdd", tone: "green" };

export const kindOf = (kind) => (kind === "disk" ? DISK : null) || KINDS.find((entry) => entry.kind === kind) || { kind, icon: "bi-folder", tone: "slate" };
