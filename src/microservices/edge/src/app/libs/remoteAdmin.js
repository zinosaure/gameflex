import { createContext, useContext, useEffect, useState } from "react";
import { peerHost } from "./federation.js";
import { CONSOLE_ICON } from "../globals/icons.js";

// ADMINISTRER UN SERVEUR FÉDÉRÉ DEPUIS LE SIEN (chantier 73) : les mêmes pages des Réglages, sous
// `/settings/remote/<serveur>/…`, dont chaque appel part chez ce serveur (libs/api.js, `setRemoteTarget`) avec la session
// de mon compte de là-bas — administrateur seulement si je l'y ai autorisé (son interrupteur, coupé par défaut).
// `RemoteAdminContext` : le serveur visé (`{serverId, name}`), null en local ; les pages ne le lisent pas, seuls la
// navigation (`toRemote`) et le titre (la pastille orange) le font.
export const RemoteAdminContext = createContext(null);

export const useRemoteAdmin = () => useContext(RemoteAdminContext);

// La racine des pages distantes d'un serveur.
export const remoteRoot = (serverId) => `/settings/remote/${encodeURIComponent(serverId)}`;

// UNE ADRESSE DES RÉGLAGES, RAMENÉE CHEZ LE SERVEUR VISÉ : `/settings/members/3` devient
// `/settings/remote/<serveur>/members/3` quand on y est ; le reste ne change pas. La liste des Réglages (`/settings`)
// devient la page du serveur rejoint, d'où l'on est venu.
export function toRemote(path, remote) {
    if (!remote || typeof path !== "string" || !path.startsWith("/settings") || path.startsWith("/settings/remote/")) {
        return path;
    }
    const rest = path.slice("/settings".length);

    return rest === "" || rest === "/" ? remote.home : `${remoteRoot(remote.serverId)}${rest}`;
}

// CE QU'ON ADMINISTRE D'UN SERVEUR FÉDÉRÉ (le porteur, 2026-11-19 : « membres, fédération, consoles, remoteplay, logs et
// rescan dans un premier temps ») : la clé de sa page des Réglages, son icône et sa teinte (celles de la liste).
// Leurs données sont les siennes : un second fournisseur de données, lu chez lui (DataProvider `remote`, P3).
export const REMOTE_PAGES = [
    { key: "members", path: "members", icon: "bi-person-badge", tone: "green" },
    { key: "federation", path: "federation", icon: "bi-arrow-left-right", tone: "green" },
    { key: "consoles", path: "consoles", icon: CONSOLE_ICON, tone: "indigo" },
    { key: "remotePlay", path: "remote-play", icon: "bi-playstation", tone: "sky" },
    { key: "logs", path: "logs", icon: "bi-journal-text", tone: "slate" },
    { key: "rescan", path: "rescan", icon: "bi-arrow-repeat", tone: "slate" },
];

// SUIS-JE ADMINISTRATEUR CHEZ CE SERVEUR, D'ICI ? La session de mon compte de là-bas, puis son `/auth/me` : vrai seulement
// si j'y ai autorisé l'administration à distance (son rang n'est alors pas rabattu). `server` : `{id, link}`.
export async function adminThere(server) {
    const opened = await peerHost(server);

    if (!opened.ok) {
        return { ok: false, code: opened.code };
    }
    const me = await fetch(`${opened.host.origin}/api/v1/auth/me`, { headers: { Authorization: `Bearer ${opened.host.token}` } })
        .then((answer) => (answer.ok ? answer.json() : null)).catch(() => null);
    const role = me?.item?.role_level ?? me?.role_level;

    return { ok: typeof role === "number" && role <= 1, host: opened.host, code: "notAdmin" };
}

export function useAdminThere(server) {
    const [admin, setAdmin] = useState(false);

    useEffect(() => {
        if (!server?.id) {
            return undefined;
        }
        let cancelled = false;
        adminThere(server).then((answer) => !cancelled && setAdmin(answer.ok));

        return () => {
            cancelled = true;
        };
    }, [server?.id]);

    return admin;
}
