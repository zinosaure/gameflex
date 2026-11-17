import { explained, readToken, loseAuth } from "./api.js";
import { useAuth as useLauncherAuth, useData } from "./data.jsx";
import { AUTH_INVALIDATED_EVENT } from "./auth-invalidation.js";

// LE COMPTE ET LE REGISTRE DU LAUNCHER, sous la forme où le code venu de la webapp les lit :
//
//   profile/auth/AuthContext.jsx de la webapp — LE JETON DU LAUNCHER : il est une autre origine, et garde le
//     sien sous sa propre clé (api.js) ; `useAuth` rend le compte du launcher et ce jeton. Le moteur de la page
//     de jeu (app/engine/, chantier 52 P8) et les fichiers de la webapp rapatriés dans edge au chantier 56
//     (engine/, libs/) l'importent directement, à la place de l'AuthContext qu'ils importaient ;
//   `useConsoles` — le registre des consoles, celui que le launcher a déjà lu (data.jsx), pour le moteur
//     (l'entrée rétro) et la page de jeu.

export function authHeaders(token) {
    return token ? { Authorization: `Bearer ${token}` } : {};
}

export const getStoredToken = () => readToken();

// Le nom du compte, lu dans le jeton sans attendre `/auth/me` — la même lecture que la webapp : la
// page de jeu le met dans son offre WebRTC dès le premier rendu (getStoredUsername, AuthContext.jsx).
export function getStoredUsername() {
    const segments = (readToken() || "").split(".");

    if (segments.length !== 3) {
        return null;
    }

    try {
        return JSON.parse(atob(segments[1].replace(/-/g, "+").replace(/_/g, "/"))).username || null;
    } catch {
        return null;
    }
}

// L'enveloppe du service des comptes telle que la webapp la lit (`{state, item, items, exception}`
// et `ok`) — libs/profileApi.js l'attend ainsi.
export async function authApi(path, options = {}) {
    let response;

    try {
        response = await fetch(path, options);
    } catch {
        return { ok: false, state: "failed", exception: { code: "serverUnreachable", detail: explained("serverUnreachable") } };
    }

    const text = await response.text().catch(() => "");

    if (!text) {
        return { ok: response.ok, state: response.ok ? "success" : "failed" };
    }

    try {
        const json = JSON.parse(text);

        // Le refus, traduit par son code (chantier 59, P5) : son anglais ne s'affiche jamais.
        const exception = json?.exception ? { ...json.exception, detail: explained(json.exception.code || "", json.exception.detail) } : json?.exception;

        return { ...json, exception, ok: response.ok && json?.state !== "failed" };
    } catch {
        return { ok: false, state: "failed", exception: { code: "invalidResponse", detail: explained("invalidResponse") } };
    }
}

export function useAuth() {
    return { ...useLauncherAuth(), token: readToken() };
}

const NO_CONSOLES = [];

export const useConsoles = () => useData()?.consoles || NO_CONSOLES;

// UN JETON QUE LE KERNEL REFUSE ferme la session du launcher, comme dans la webapp : son canal
// (libs/kernelChannel.js) le signale par cet événement, que son AuthContext écoutait.
window.addEventListener(AUTH_INVALIDATED_EVENT, () => loseAuth());
