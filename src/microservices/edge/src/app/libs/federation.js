import { useEffect, useState } from "react";
import { kernel } from "./api.js";

// NOTRE COMPTE CHEZ UN SERVEUR FÉDÉRÉ (chantier 20) : notre kernel garde le jeton qu'on a collé une fois et ouvre une
// session courte chez l'ami (`GET /servers/{id}/session` → `{token, expiresAt, username, displayName}`). La page la garde
// en mémoire, jamais dans le navigateur, et la redemande une heure avant son expiration — la même marge que notre kernel,
// qui la ré-échange alors : une partie lancée avec elle ne doit pas tomber au bout de quelques minutes.
//
// CE N'EST PAS L'IDENTITÉ DE LA PAGE (api.js) : celle-là est une, celle de la personne devant l'écran ; ici on tient des
// sessions chez D'AUTRES, une par serveur fédéré — rangées par SERVEUR et non par console : deux PS5 du même salon se
// jouent avec la même session. Le jeton collé, lui, ne revient jamais à la page.
//
// UNE SEULE MÉMOIRE (#748) : les Réglages, la bibliothèque, la page de jeu, « En direct » et le canal d'un kernel la
// lisent tous ici. Il y en avait deux (celle-ci et libs/federationSession.js, venue de la webapp) : un jeton collé ou
// oublié dans les Réglages n'était vu des parties qu'au prochain retour sur leur écran, et une session refusée par l'ami
// restait servie aux Réglages et aux lancements jusqu'à son expiration.

// LA DONNÉE RÉSIDUELLE DU FORMULAIRE SUPPRIMÉ (identifiant et mot de passe chez l'ami, avant le chantier 20) : les
// sessions qu'il rangeait dans ce navigateur ne servent plus à rien, et ne doivent pas y traîner.
try {
    localStorage.removeItem("gameflex.federationSessions");
} catch {
    // Stockage refusé (navigation privée) : il n'y a rien à effacer.
}

const MARGIN_MS = 60 * 60 * 1000;
const held = new Map();
const listeners = new Set();

// Une session obtenue, collée, oubliée ou refusée : ceux qui en dépendent la redemandent. Un abonné qui échoue
// n'empêche pas les autres d'être prévenus.
const announce = () => listeners.forEach((listener) => {
    try {
        listener();
    } catch {
        // Rien : les autres abonnés passent quand même.
    }
});

export function onFederationSessionChanged(listener) {
    listeners.add(listener);

    return () => listeners.delete(listener);
}

// Une invitation (le lien créé chez l'ami, Réglages › Profil, « Mon QR ») plutôt qu'un jeton.
const INVITATION = /^https?:\/\/[^\s]+\/invite\/[A-Za-z0-9_-]{22}\/?$/;
export const isInvitationLink = (text) => INVITATION.test(String(text || "").trim());

// La session chez ce serveur, au nom de NOTRE compte de chez lui : `{ok: true, session}`, ou `{ok: false, code}`, que
// la page traduit — `noFederatedAccount` (aucun jeton collé : ses jeux se voient et se téléchargent), `accountRefused`
// (il refuse ce jeton : désactivé, échu, régénéré, compte supprimé), `serverUnreachable` (il ne répond pas).
export async function federationSession(serverId) {
    const kept = held.get(String(serverId));

    if (kept && Date.parse(kept.expiresAt) - Date.now() > MARGIN_MS) {
        return { ok: true, session: kept };
    }
    const answer = await kernel(`/servers/${encodeURIComponent(serverId)}/session`);

    if (!answer.ok || !answer.data?.token) {
        return { ok: false, code: answer.errorCode || "serverUnreachable" };
    }
    held.set(String(serverId), answer.data);

    return { ok: true, session: answer.data };
}

// Coller le jeton de notre compte de là-bas, ou son invitation (notre kernel l'échange chez lui).
export const pasteFederationToken = (serverId, pasted) => sendFederationAccount(serverId,
    isInvitationLink(pasted) ? { invitation: pasted.trim() } : { token: pasted.trim() });

// Ou taper le code d'appairage de notre compte de là-bas (chantier 60) : notre kernel l'échange chez lui.
export const typeFederationCode = (serverId, code) => sendFederationAccount(serverId, { code: code.trim() });

async function sendFederationAccount(serverId, body) {
    const answer = await kernel(`/servers/${encodeURIComponent(serverId)}/account`, {
        method: "PUT",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
    });

    if (!answer.ok || !answer.data?.token) {
        return { ok: false, code: answer.errorCode || "serverUnreachable" };
    }
    held.set(String(serverId), answer.data);
    announce();

    return { ok: true, session: answer.data };
}

export async function forgetFederationAccount(serverId) {
    const answer = await kernel(`/servers/${encodeURIComponent(serverId)}/account`, { method: "DELETE" });
    held.delete(String(serverId));
    announce();

    return answer.ok || answer.status === 404;
}

// Oublier cette session, chez quelque serveur qu'elle ait été prise : son hôte vient de la refuser (kernelChannel le
// lui a demandé). Ceux qui jouaient chez lui la redemandent ; notre kernel la ré-échange s'il le peut.
export function forgetFederationToken(token) {
    let forgotten = false;

    for (const [serverId, session] of held) {
        if (session.token === token) {
            held.delete(serverId);
            forgotten = true;
        }
    }

    if (forgotten) {
        announce();
    }
}

// Un serveur fédéré comme destination ou comme source : son adresse et la session de notre compte.
export async function peerHost(server) {
    const opened = await federationSession(server.id);

    return opened.ok ? { ok: true, host: { origin: server.link, token: opened.session.token } } : { ok: false, code: opened.code };
}

// La session d'un serveur, suivie : `{session, code, loading}` (null : aucun serveur choisi).
export function useFederationSession(serverId) {
    const [state, setState] = useState({ session: null, code: null, loading: Boolean(serverId) });

    useEffect(() => {
        if (!serverId) {
            setState({ session: null, code: null, loading: false });

            return undefined;
        }
        let cancelled = false;
        const ask = () => {
            setState((current) => ({ ...current, loading: true }));
            federationSession(serverId).then((result) => {
                if (!cancelled) {
                    setState({ session: result.ok ? result.session : null, code: result.ok ? null : result.code, loading: false });
                }
            });
        };
        ask();
        const stopListening = onFederationSessionChanged(ask);

        return () => {
            cancelled = true;
            stopListening();
        };
    }, [serverId]);

    return state;
}
