// Jouer CHEZ UN AMI — chantier 9, C9-P3, refait au chantier 20.
//
// Ce n'est pas AuthContext, et la distinction porte tout le fichier. Celui-ci
// tient l'identité de la personne devant l'écran : une, indivisible, celle qui
// ouvre CETTE installation. Ici on tient des sessions chez D'AUTRES — une par
// serveur fédéré où l'on joue.
//
// UN JETON, ET C'EST BON (chantier 20, le porteur : « au lieu de voir l'ancien
// formulaire login, c'est un formulaire de token […] on met le token valide et
// c'est bon »). Quelqu'un qui a un compte chez un ami recopie le jeton de ce
// compte (Réglages › Profil, chez l'ami) et le colle ici, UNE fois
// (`pasteFederationToken`) : notre kernel l'essaie chez l'ami, puis le garde
// avec son compte d'ici — sur tous ses appareils. Pour jouer, la page a besoin
// d'une SESSION de ce compte : notre kernel échange le jeton contre elle
// (`GET /api/v1/servers/{id}/session`) — le jeton, lui, ne revient jamais à la
// page. Le formulaire qui demandait un identifiant et un mot de passe chez
// l'ami (`FederatedSignIn`) n'existe plus, ni les sessions qu'il rangeait dans
// ce navigateur.
//
// LE LIEN, LUI, N'Y EST POUR RIEN : le jeton de catalogue qu'un administrateur
// a enregistré fait lister et télécharger les jeux de l'ami, à tous nos
// membres. Il n'ouvre aucune session.
//
// Rangé par SERVEUR et non par console : deux PS5 du même salon se jouent avec
// la même session.
import { useEffect, useState } from "react";
import { apiUrl } from "./serverBase.js";
import { getStoredToken } from "../profile/auth/AuthContext.jsx";
import { isInvitationLink } from "../api/invitations.js";

// LA DONNÉE RÉSIDUELLE DU FORMULAIRE SUPPRIMÉ : les sessions qu'il rangeait
// dans ce navigateur ne servent plus à rien, et ne doivent pas y traîner.
try {
    localStorage.removeItem("gameflex.federationSessions");
} catch {
    // Stockage refusé (navigation privée) : il n'y a rien à effacer.
}

// Une session qui finit avant ce délai se redemande — la même marge que notre
// kernel, qui la ré-échange alors : une partie lancée avec elle ne doit pas
// tomber au bout de quelques minutes.
const MARGIN_MS = 60 * 60 * 1000;

// Les sessions déjà obtenues, par serveur : en mémoire seulement. Notre kernel
// les garde aussi, et vérifie chez l'ami qu'elles valent encore.
const held = new Map();

// Ce qui écoute : les écrans qui jouent chez un ami, prévenus quand une
// session vient d'être refusée — ils la redemandent.
const listeners = new Set();

export function onFederationSessionChanged(callback) {
    listeners.add(callback);

    return () => listeners.delete(callback);
}

function announceChange() {
    for (const callback of listeners) {
        try {
            callback();
        } catch {
            // Un abonné qui échoue ne doit pas empêcher les autres d'être
            // prévenus.
        }
    }
}

/**
 * La session chez ce serveur fédéré, au nom de NOTRE compte de chez lui —
 * `{ok: true, session}`, où `session` est `{token, username, displayName,
 * consoles, expiresAt}` ; ou `{ok: false, code}`, que la page traduit
 * (`common.errorCodes.<code>`) : `noFederatedAccount` (on n'a pas collé de
 * jeton pour lui : ses jeux se voient et se téléchargent), `accountRefused`
 * (il refuse ce jeton : désactivé, échu, régénéré, compte supprimé),
 * `serverUnreachable` (il ne répond pas).
 */
export async function federationSession(serverId) {
    const key = String(serverId);
    const kept = held.get(key);
    if (kept && Date.parse(kept.expiresAt) - Date.now() > MARGIN_MS) {
        return { ok: true, session: kept };
    }

    const token = getStoredToken();
    let response;
    try {
        response = await fetch(apiUrl(`/api/v1/servers/${encodeURIComponent(serverId)}/session`), {
            headers: token ? { Authorization: `Bearer ${token}` } : {},
        });
    } catch {
        return { ok: false, code: "serverUnreachable" };
    }
    let payload = null;
    try {
        payload = await response.json();
    } catch {
        // Une réponse illisible se traite comme un serveur qui ne répond pas.
    }
    if (!response.ok || !payload?.data?.token) {
        return { ok: false, code: payload?.errorCode || "serverUnreachable" };
    }
    held.set(key, payload.data);

    return { ok: true, session: payload.data };
}

/**
 * Coller le jeton de NOTRE compte chez ce serveur — une fois. Notre kernel
 * l'essaie chez lui avant de le garder : `{ok: true, session}`, ou `{ok: false,
 * code}` (`accountRefused`, `serverUnreachable`). Les écrans qui montraient le
 * formulaire montrent aussitôt sous quel nom on y joue.
 *
 * OU SON INVITATION (chantier 33) : un lien `…/invite/<code>` que notre kernel
 * échange chez lui contre le jeton — `invitationRefused`, `invitationTokenOff`…
 */
export async function pasteFederationToken(serverId, accountToken) {
    const token = getStoredToken();
    let response;
    try {
        response = await fetch(apiUrl(`/api/v1/servers/${encodeURIComponent(serverId)}/account`), {
            method: "PUT",
            headers: {
                "Content-Type": "application/json",
                ...(token ? { Authorization: `Bearer ${token}` } : {}),
            },
            body: JSON.stringify(isInvitationLink(accountToken) ? { invitation: accountToken } : { token: accountToken }),
        });
    } catch {
        return { ok: false, code: "serverUnreachable" };
    }
    let payload = null;
    try {
        payload = await response.json();
    } catch {
        // Une réponse illisible se traite comme un serveur qui ne répond pas.
    }
    if (!response.ok || !payload?.data?.token) {
        return { ok: false, code: payload?.errorCode || "serverUnreachable" };
    }
    held.set(String(serverId), payload.data);
    announceChange();

    return { ok: true, session: payload.data };
}

/**
 * Oublier le jeton de notre compte chez ce serveur : ses jeux restent à voir et
 * à télécharger, ils ne s'y lancent plus. Rend vrai si notre kernel l'a oublié.
 */
export async function forgetFederationAccount(serverId) {
    const token = getStoredToken();
    let response;
    try {
        response = await fetch(apiUrl(`/api/v1/servers/${encodeURIComponent(serverId)}/account`), {
            method: "DELETE",
            headers: token ? { Authorization: `Bearer ${token}` } : {},
        });
    } catch {
        return false;
    }
    held.delete(String(serverId));
    announceChange();

    return response.ok || response.status === 404;
}

/**
 * Oublier cette session, chez quelque serveur qu'elle ait été prise : son hôte
 * vient de la refuser (kernelChannel le lui a demandé). Les écrans qui jouaient
 * chez lui la redemandent ; notre kernel la ré-échange s'il le peut.
 */
export function forgetFederationToken(token) {
    let forgotten = false;
    for (const [serverId, session] of held) {
        if (session.token === token) {
            held.delete(serverId);
            forgotten = true;
        }
    }
    if (forgotten) {
        announceChange();
    }
}

/**
 * La session chez ce serveur, pour un écran qui joue chez lui — `{session,
 * code, isLoading}`. Redemandée quand elle est refusée ; rien sans serveur.
 */
export function useFederationSession(serverId) {
    const [state, setState] = useState({ session: null, code: null, isLoading: Boolean(serverId) });

    useEffect(() => {
        if (!serverId) {
            setState({ session: null, code: null, isLoading: false });

            return undefined;
        }
        let cancelled = false;
        const ask = () => {
            setState((current) => ({ ...current, isLoading: true }));
            federationSession(serverId).then((result) => {
                if (!cancelled) {
                    setState({
                        session: result.ok ? result.session : null,
                        code: result.ok ? null : result.code,
                        isLoading: false,
                    });
                }
            });
        };
        ask();
        const unsubscribe = onFederationSessionChanged(ask);

        return () => {
            cancelled = true;
            unsubscribe();
        };
    }, [serverId]);

    return state;
}
