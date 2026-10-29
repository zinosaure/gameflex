import React, { createContext, useCallback, useContext, useEffect, useState } from "react";
import { AUTH_INVALIDATED_EVENT } from "./auth-invalidation.js";
import { can } from "../../libs/capabilities.js";
import { apiUrl } from "../../libs/serverBase.js";
import { connectionLost, invalidResponse, serverUnreachable } from "../../shared/utils/error-code-translator.js";

// Deliberately a SEPARATE context from Skeleton.context — skeleton.js is
// vendored (règle du dépôt : "on n'y touche plus, ou
// le moins possible" once adopted) and its own `app` context object is a
// closed literal with no extension point. This wraps <Skeleton> in index.js
// the same way <BrowserRouter> already does, so any page can
// useContext(AuthContext) alongside useContext(Skeleton.context).
const TOKEN_STORAGE_KEY = "retrogaming-users-token";
const AuthContext = createContext(null);

// Backend envelope is {at, code, state, reference, item|items+pagination|exception}
// (python/src/app/libs/skeleton.py StandardResponse) — same shape as the
// gameflex/api/retrogaming.js wrapper, reimplemented here rather than shared
// since the two API surfaces (Go vs Python) intentionally have different
// envelopes and there's no shared fetch helper today.
async function api(path, options = {}) {
    let res;
    try {
        // apiUrl : chemin relatif dans un navigateur (rien ne change), origine
        // explicite quand l'application Android a déclaré son serveur.
        res = await fetch(apiUrl(path), options);
    } catch {
        return { ok: false, state: "failed", exception: { detail: serverUnreachable().errorMessage } };
    }
    // An empty body is NOT a failure: several routes answer 204 No Content
    // (removing a member, a key mapping, a positioning slot, and
    // skeleton.py's generic DELETE). Parsing straight to JSON threw on all of
    // them and the catch below reported "réponse invalide", so every one of
    // those deletions looked like an error to its caller — the row vanished
    // server-side but the list never refreshed.
    // LIRE LE CORPS PEUT ÉCHOUER APRÈS L'EN-TÊTE.
    //
    // `fetch` a rendu la main — donc le `try` ci-dessus est refermé — mais le
    // corps arrive encore : une connexion coupée en cours de réponse, un
    // service arrêté pendant l'envoi, et `res.text()` REJETTE. Hors d'un
    // `try`, ce rejet ne s'arrêtait nulle part : il traversait `login()`, qui
    // ne rend alors jamais la main, et le bouton de connexion tournait
    // indéfiniment sans afficher quoi que ce soit — il fallait fermer
    // complètement l'application (signalé le 2026-09-25).
    //
    // Une réponse illisible est un échec comme un autre. Elle se DIT, comme
    // les deux autres juste à côté ; elle ne remonte pas en exception.
    let text;

    try {
        text = await res.text();
    } catch {
        return { ok: false, state: "failed", exception: { detail: connectionLost().errorMessage } };
    }

    if (!text) {
        return { ok: res.ok, state: res.ok ? "success" : "failed" };
    }

    let json;
    try {
        json = JSON.parse(text);
    } catch {
        return { ok: false, state: "failed", exception: { detail: invalidResponse().errorMessage } };
    }
    // `json?.state` et non `json.state` : un corps valant littéralement
    // `null` est du JSON parfaitement valide, et le déréférencer jetait ici —
    // au même endroit, avec la même conséquence, que le rejet ci-dessus.
    return { ...json, ok: res.ok && json?.state !== "failed" };
}

export function authHeaders(token) {
    return token ? { Authorization: `Bearer ${token}` } : {};
}

// getStoredToken — the current token straight from storage, for callers that
// are NOT React components and cannot reach the context: the retro-gaming API
// module (gameflex/api/retrogaming.js), whose fifteen functions would
// otherwise all need a token parameter threaded through every call site, and
// the retro streaming WebSocket, which carries it in the query string because
// a browser cannot set a header on a WS handshake.
//
// Reads the same key this provider writes, so there is still one source of
// truth for where the token lives. Prefer `const { token } = useAuth()` inside
// a component — this exists for the places that genuinely cannot.
export function getStoredToken() {
    // Même règle que dans le provider, et il FAUT qu'elle soit ici aussi :
    // cette fonction sert les appelants qui ne sont pas des composants (l'API
    // retrogaming, le WebSocket de streaming). Ne la corriger qu'à un seul
    // endroit laisserait exactement les chemins les plus discrets porter le
    // jeton qu'on croit avoir retiré.
    if (!can("accounts")) {
        return null;
    }
    try {
        return localStorage.getItem(TOKEN_STORAGE_KEY);
    } catch {
        return null;
    }
}

// getStoredUsername — the username claim of the stored token, read
// SYNCHRONOUSLY on the very first render. `user` from the context only
// arrives once /api/v1/auth/me answers, and anything that must be decided
// before that round-trip flickers: the streaming page bakes the viewer's
// identity and its input DataChannel into a WebRTC offer, so a first render
// that believed itself anonymous produced a throwaway viewer and a second
// negotiation ~50ms later (seen in the gateway log, 2026-08-24).
//
// This is NOT an authentication check: the payload is unverified, the server
// validates the signature on every request. It is only good enough to answer
// "who does this browser think it is" without waiting.
export function getStoredUsername() {
    const token = getStoredToken();

    if (!token) {
        return null;
    }

    const segments = token.split(".");

    if (segments.length !== 3) {
        return null;
    }

    try {
        return JSON.parse(atob(segments[1].replace(/-/g, "+").replace(/_/g, "/"))).username || null;
    } catch {
        return null;
    }
}

// Exported so other modules (history/presence API calls, later
// phases) can reuse the same envelope-normalizing fetch wrapper + attach
// the current token without re-deriving it from context every time.
export { api as authApi };

// Le porteur de l'appareil, quand il n'y a pas d'annuaire.
//
// Sur une installation sans comptes (`accounts: false` — un téléphone), il
// n'y a personne à qui prouver son identité : la machine est à celui qui la
// tient. Demander un mot de passe au démarrage y est un pur rite, et un rite
// qui BLOQUE — l'application ouvre sur un écran de connexion vers un serveur
// qu'elle n'a même pas besoin d'avoir.
//
// Ce n'est pas un contournement d'authentification : il n'y a rien à
// contourner. Ce que ce faux utilisateur donne, c'est un nom pour l'interface
// (« Ravis de vous revoir »), un rôle d'administrateur local — sur son propre
// appareil, on l'est — et surtout PAS de jeton. Toute la surface qui exige un
// jeton reste donc fermée par construction : le porteur ne peut rien faire
// sur un serveur distant sans en avoir reçu l'accès explicitement.
const DEVICE_OWNER = {
    username: "device",
    display_name: "GameFlex",
    role_level: 0,
    // Tout, comme un administrateur (chantier 20) : il n'y a personne d'autre.
    consoles: ["*"],
};

export function AuthProvider({ children }) {
    // deviceMode est lu UNE fois, comme les capacités elles-mêmes : c'est une
    // propriété de l'installation, pas un état qui change en cours de route.
    const [deviceMode] = useState(() => !can("accounts"));
    // Sur un appareil, l'application n'a PAS de session sur un serveur — et
    // ne doit surtout pas en garder une.
    //
    // Faille trouvée le 2026-08-27 : une version précédente demandait un
    // login au démarrage, le jeton restait dans le stockage du navigateur, et
    // `deviceMode` continuait de le distribuer à chaque appel. L'application
    // gardait donc TOUS les droits du compte — y compris l'accès à la PS5.
    // Signalé par l'utilisateur, exactement dans ces termes : « si je donne
    // l'app à un ami, les gens auront accès à ma PS5 ».
    //
    // On ne se contente pas de l'ignorer : on l'EFFACE. Un secret qu'on
    // choisit de ne plus lire reste un secret posé sur l'appareil, que la
    // prochaine version — ou un bout de code oublié — relira.
    const [token, setToken] = useState(() => {
        if (!can("accounts")) {
            try {
                localStorage.removeItem(TOKEN_STORAGE_KEY);
            } catch {
                // Stockage refusé : rien à effacer, rien à lire non plus.
            }

            return null;
        }

        return localStorage.getItem(TOKEN_STORAGE_KEY);
    });
    const [user, setUser] = useState(() => (can("accounts") ? null : DEVICE_OWNER));
    // True until the initial /api/v1/auth/me check (existing token) resolves —
    // distinct from `!isAuthenticated`, which would otherwise flash the
    // Login page for a split second even when a valid token is already
    // stored, before that check has had a chance to run.
    const [isLoading, setIsLoading] = useState(true);

    useEffect(() => {
        let cancelled = false;
        // Sans annuaire, il n'y a pas de /api/v1/auth/me à interroger — et
        // surtout pas de serveur dont l'absence devrait retarder l'écran.
        if (deviceMode) {
            setIsLoading(false);
            return;
        }
        if (!token) {
            // PLUS D'OUVERTURE AUTOMATIQUE, MÊME SUR L'APPAREIL.
            //
            // Le téléphone frappait son propre jeton et ouvrait la page en
            // root, sans rien demander. C'était juste tant qu'il n'y avait
            // qu'un compte et qu'un porteur ; ça ne l'est plus. Avec de vrais
            // comptes, cette ouverture-là écrasait tout : se déconnecter
            // reconnectait aussitôt en root, et il devenait impossible
            // d'utiliser l'application sous SON compte à soi (signalé le
            // 2026-09-04).
            //
            // Le téléphone se comporte donc comme le serveur : première
            // utilisation, root et son mot de passe (voir la console de
            // l'application, carte COMPTES), puis on crée les comptes depuis
            // Gestion des membres. Le jeton vaut un an — on ne se reconnecte
            // pas tous les jours.
            setUser(null);
            setIsLoading(false);
            return;
        }
        api("/api/v1/auth/me", { headers: authHeaders(token) }).then((result) => {
            if (cancelled) return;
            if (result.ok) {
                setUser(result.item);
            } else {
                // Expired/invalid token — drop it rather than getting stuck
                // retrying it on every render.
                localStorage.removeItem(TOKEN_STORAGE_KEY);
                setToken(null);
                setUser(null);
            }
            setIsLoading(false);
        });
        return () => { cancelled = true; };
    }, [token, deviceMode]);

    const login = useCallback(async (username, password) => {
        const result = await api("/api/v1/auth/login", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ username, password }),
        });
        if (result.ok) {
            localStorage.setItem(TOKEN_STORAGE_KEY, result.item.access_token);
            setToken(result.item.access_token);
            setUser(result.item.user);
        }
        return result;
    }, []);


    const logout = useCallback(() => {
        localStorage.removeItem(TOKEN_STORAGE_KEY);
        setToken(null);
        setUser(null);
    }, []);

    // LE JETON A ÉTÉ REFUSÉ AILLEURS — on referme ici.
    //
    // La couche réseau ne connaît pas React : elle efface le jeton et le DIT
    // (voir auth-invalidation.js). Sans cet écouteur, elle l'effaçait dans le
    // stockage pendant que ce contexte gardait le sien en mémoire — donc
    // l'écran restait celui d'une personne connectée, et chaque appel
    // continuait d'échouer.
    useEffect(() => {
        const closeSession = () => {
            setToken(null);
            setUser(null);
        };
        window.addEventListener(AUTH_INVALIDATED_EVENT, closeSession);

        return () => window.removeEventListener(AUTH_INVALIDATED_EVENT, closeSession);
    }, []);

    const value = {
        user,
        token,
        // Vrai quand il n'y a pas d'annuaire : ce que l'interface doit
        // savoir pour ne pas proposer de se déconnecter de nulle part.
        deviceMode,
        login,
        logout,
        isLoading,
        isAuthenticated: !!user,
        // role_level: 0=root, 1=admin, 2=user (see python/src/app/users/models.py) —
        // lower is more privileged, admin gating checks <= ROLE_ADMIN(1).
        isAdmin: !!user && user.role_level <= 1,
        isRoot: !!user && user.role_level === 0,
        // JOUER ICI : lancer une partie, et écrire sur la machine qui la tient.
        //
        // Sur un serveur, tout compte joue. Sur un téléphone
        // (`onlyAdminsLaunch`), seul un administrateur — le propriétaire,
        // d'ordinaire — y lance une partie, y range ou y retire un jeu : il ne
        // tient qu'une partie. Un membre y regarde, rejoint, demande la
        // manette. Les comptes, eux, sont ceux d'un serveur. Défini ICI, une
        // fois, pour que chaque écran pose la même question au lieu de
        // recopier la règle (décidé le 2026-10-08, comptes précisés le
        // 2026-10-09).
        playsHere: !!user && (!can("onlyAdminsLaunch") || user.role_level <= 1),
    };

    return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth() {
    return useContext(AuthContext);
}
