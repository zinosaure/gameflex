import { authApi, authHeaders, getStoredToken } from "../profile/auth/AuthContext.jsx";
import { apiUrl, shareOrigin } from "../libs/serverBase.js";

// LES INVITATIONS (chantier 33) — ce qui voyage par une messagerie à la place
// d'un jeton : un lien `<serveur>/invite/<code>`, à usage unique, valable 12
// heures, que NOTRE serveur échange auprès du sien contre le vrai jeton. Le
// service des comptes les crée et les garde (`/api/v1/token-invitations`) ; le
// kernel les échange, dessine leur QR et le relit dans une image (`/api/v1/qr`).

const jsonHeaders = { "Content-Type": "application/json" };

/** Le code d'une invitation : 16 octets en base64 sans remplissage. */
const INVITATION = /^https?:\/\/[^\s]+\/invite\/[A-Za-z0-9_-]{22}\/?$/;

/** Ce texte est-il le lien d'une invitation ? */
export function isInvitationLink(text) {
    return INVITATION.test(String(text || "").trim());
}

/** Le lien d'une invitation de CE serveur, à son adresse publique. */
export function invitationLink(code) {
    return `${shareOrigin()}/invite/${code}`;
}

/**
 * Une invitation : `federation` (un administrateur — pour qui, et s'il prête
 * ses consoles) ou `account` (le titulaire, pour son propre jeton). Le code ne
 * sort qu'ici, une fois.
 */
export function createInvitation(token, { kind, label = "", scopes = [] }) {
    return authApi("/api/v1/token-invitations", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify(kind === "account" ? { kind } : { kind, label, scopes }),
    });
}

/** Les invitations en attente, des deux sortes — jamais leur code. */
export function listInvitations(token) {
    return authApi("/api/v1/token-invitations", { headers: authHeaders(token) });
}

export function revokeInvitation(token, id) {
    return authApi(`/api/v1/token-invitations/${id}`, {
        method: "DELETE",
        headers: authHeaders(token),
    });
}

// kernel — { ok, data, code } : `code` est celui que la page traduit
// (`common.errorCodes.<code>`).
async function kernel(path, options) {
    let response;
    try {
        response = await fetch(apiUrl(path), {
            ...options,
            headers: { ...(options.headers || {}), ...authHeaders(getStoredToken()) },
        });
    } catch {
        return { ok: false, code: "serverUnreachable" };
    }
    const payload = await response.json().catch(() => null);

    return {
        ok: response.ok && payload?.success !== false,
        data: payload?.data,
        code: payload?.errorCode || "",
        message: payload?.errorMessage || "",
    };
}

/**
 * Le lien d'une invitation de fédération devient le jeton du lien : échangé
 * chez lui, puis sondé. `data` a la forme de la sonde (`link`, `hostname`,
 * `identity`, `scopes`) plus `token`, que la page range comme un jeton collé.
 */
export function redeemServerInvitation(invitation) {
    return kernel("/api/v1/servers/redeem", {
        method: "POST",
        headers: jsonHeaders,
        body: JSON.stringify({ invitation: invitation.trim() }),
    });
}

/** Le QR d'un lien d'invitation, en image PNG ; null si le serveur refuse. */
export async function invitationQR(link, size = 512) {
    try {
        const response = await fetch(apiUrl("/api/v1/qr"), {
            method: "POST",
            headers: { ...jsonHeaders, ...authHeaders(getStoredToken()) },
            body: JSON.stringify({ text: link, size }),
        });

        return response.ok ? await response.blob() : null;
    } catch {
        return null;
    }
}

/**
 * Le lien que porte le QR d'une image déposée — une capture reçue par une
 * messagerie, une photo. `code` : `noQR`, `notInvitation`.
 */
export function readInvitationImage(file) {
    return kernel("/api/v1/qr/read", {
        method: "POST",
        headers: { "Content-Type": file.type || "application/octet-stream" },
        body: file,
    });
}
