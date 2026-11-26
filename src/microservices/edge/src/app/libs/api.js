// LES APPELS AU BACKEND — les mêmes routes que la webapp, rien de plus.
//
// Deux enveloppes, comme dans src/webapp : le kernel répond
// `{success, data, errorMessage}` (service/api-client.js), le service des
// comptes `{state, item, exception}` (profile/auth/AuthContext.jsx). Le jeton
// part en `Authorization: Bearer`.
//
// LE LAUNCHER EST UNE AUTRE ORIGINE (un autre port) : il ne voit pas le
// stockage de la webapp, et garde donc son propre jeton, sous sa propre clé.

import i18n from "../i18n.js";
import { capabilities } from "./capabilities.js";

const TOKEN_KEY = "gameflex-edge-token";
const AUTH_LOST = "gameflex-edge:auth-lost";

export const readToken = () => {
    try {
        return localStorage.getItem(TOKEN_KEY);
    } catch {
        return null;
    }
};

export const writeToken = (token) => {
    try {
        if (token) {
            localStorage.setItem(TOKEN_KEY, token);
        } else {
            localStorage.removeItem(TOKEN_KEY);
        }
    } catch {
        // Stockage refusé (navigation privée) : le jeton ne vit que le temps de la page.
    }
};

// Un 401 ferme la session ; un 403 est un geste refusé, il ne dit rien du jeton
// (la même règle que la webapp, relevée le 2026-10-09).
export const onAuthLost = (listener) => {
    window.addEventListener(AUTH_LOST, listener);

    return () => window.removeEventListener(AUTH_LOST, listener);
};

const headers = (extra = {}) => {
    const token = readToken();

    return { ...(token ? { Authorization: `Bearer ${token}` } : {}), ...extra };
};

// UN JETON NEUF EN ROUTE (chantier 55) : « Mon mot de passe » et « Déconnecter les autres appareils » coupent les
// sessions du compte, celle-ci comprise, puis rendent un jeton neuf à CET appareil. Pendant ce temps, les refus de
// l'ancien jeton arrivent (le kernel ferme les canaux en 1008) : ils attendent le neuf avant de juger quoi que ce soit.
let renewal = null;

// renewSession — `call()` rend `{ok, item: {access_token}}` ; le jeton neuf est gardé avant que les refus de
// l'ancien ne soient jugés.
export async function renewSession(call) {
    let finish = () => {};
    renewal = new Promise((resolve) => {
        finish = resolve;
    });

    try {
        const answer = await call();

        if (answer.ok && answer.item?.access_token) {
            writeToken(answer.item.access_token);
        }

        return answer;
    } finally {
        finish();
        renewal = null;
    }
}

// UN 401 NE FERME LA SESSION QUE S'IL RÉPOND AU JETON COURANT (chantier 55) : `used` est le jeton de la requête —
// celui d'avant un jeton neuf (renewSession) ne déconnecte pas l'appareil qui l'a reçu.
const lose = async (status, used = readToken()) => {
    if (status !== 401 || !used) {
        return;
    }
    await renewal;

    if (used === readToken()) {
        writeToken(null);
        window.dispatchEvent(new Event(AUTH_LOST));
    }
};

// Le jeton refusé ailleurs (le canal du kernel de la page de jeu, libs/sharedHost.js).
export const loseAuth = () => lose(401);

// UN CANAL FERMÉ EN 1008, OU REFUSÉ À SA POIGNÉE DE MAIN (chantier 55) : la session est-elle finie ? `/auth/me` avec le
// jeton courant le dit — un 401 la ferme (lose), sinon le canal se reconnecte. La règle du moteur de la page de jeu
// (kernelChannel.js, `diagnose`), écrite ici une fois pour les canaux du launcher : le centre de messages
// (subscribeTopic) et la présence (subscribeInvites). Un seul examen à la fois, quel que soit le nombre de canaux
// fermés ensemble.
let examining = null;

export function examineSession() {
    if (!examining) {
        examining = (async () => {
            await renewal;
            const used = readToken();

            if (!used) {
                return;
            }

            try {
                const response = await fetch("/api/v1/auth/me", { headers: { Authorization: `Bearer ${used}` }, cache: "no-store" });
                await lose(response.status, used);
            } catch {
                // Le serveur ne répond pas : rien ne dit que la session est finie, le canal se reconnecte.
            }
        })().finally(() => {
            examining = null;
        });
    }

    return examining;
}

// SESSION_ENDED — le code d'une trame de fermeture qui dit « session finie » (facade.SessionEnded, la présence).
const SESSION_ENDED = 1008;

// UN REFUS DU KERNEL, DIT DANS LA LANGUE DE LA PAGE (chantier 59, P5 — le porteur : « Un code oui ») : chaque refus porte
// un code (writeErrorCode), traduit ici une fois (`kernelErrors.<code>`, puis `common.errorCodes.<code>`), et chaque avis
// qui montre `errorMessage` dit la cause sans que chaque page la traduise. Son texte anglais ne s'affiche JAMAIS : sans
// traduction, `errorMessage` est vide et la page dit son propre échec traduit (`answer.errorMessage || t("…")`) ; le
// texte brut va à la console, pour qui cherche.
export function explained(code, message) {
    for (const key of [`kernelErrors.${code}`, `common.errorCodes.${code}`]) {
        if (code && i18n.exists(key)) {
            return i18n.t(key);
        }
    }
    if (message) {
        console.warn(`[kernel] ${code || "sans code"} : ${message}`);
    }

    return "";
}

// UNE ROUTE QUE L'HÔTE NE SERT PAS (chantier 56, P8) — le téléphone, dont le kernel n'a pas encore tout ce que sert le
// serveur (le chantier 54 : la commande `download`, `/defaults`, la disponibilité, le correctif d'un jeu, ses
// métadonnées et ses images, les fonds d'une console, la SRAM). Elle répond le 404 JSON du servant de la page,
// `no such route: …` (shared/webpage) ; une commande du canal qu'il ne connaît pas, `unknown message type: …`
// (shared/facade). Reconnue ICI, une fois, pour `kernel` et `kernelCommand`, et dite comme telle : « pas encore sur
// cet appareil » (`kernelErrors.not_served`), au lieu d'un échec brut. Son statut devient 501 : dans une page, un 404
// veut dire « cette chose n'existe pas » (une partie finie, un jeu inconnu) — une route absente n'en dit rien.
const NOT_SERVED = "not_served";
const notServed = (status, message) => (status === 404 && String(message ?? "").startsWith("no such route"))
    || (status === 400 && String(message ?? "").startsWith("unknown message type:"));

// La réponse du kernel, d'une route ou d'une commande du canal : `{ok, status, data, errorMessage, errorCode}`.
function kernelAnswer(status, body) {
    const absent = notServed(status, body?.errorMessage);
    const errorCode = absent ? NOT_SERVED : body?.errorCode || "";

    return {
        ok: status >= 200 && status < 300 && body?.success !== false,
        status: absent ? 501 : status,
        data: body && "data" in body ? body.data : body,
        errorMessage: explained(errorCode, body?.errorMessage),
        errorCode,
    };
}

// Le kernel : `{ok, status, data, errorMessage, errorCode}`. `host` — un serveur fédéré (`{origin, token}`,
// libs/federation.js) : la même route chez lui, sous notre compte de là-bas ; un 401 de sa part
// ne ferme pas la session d'ici.
// LE SERVEUR VISÉ PAR LES RÉGLAGES (chantier 73) : sous `/settings/remote/<serveur>/…`, chaque appel au kernel et au
// service des comptes part chez lui, avec la session de mon compte de là-bas (`{origin, token}`) — posé par la page
// distante (settings/RemoteAdmin.jsx), retiré en la quittant. Un seul endroit le décide, jamais page par page.
let remoteTarget = null;
export const setRemoteTarget = (target) => {
    remoteTarget = target;
};
// La cible du moment, pour qui ouvre son propre canal (`kernelTarget`) : null en local.
export const remoteTargetNow = () => remoteTarget;

export async function kernel(path, options = {}, given = null) {
    const host = given || remoteTarget;
    const used = readToken();
    let response;

    try {
        response = await fetch(host ? `${host.origin}/api/v1${path}` : `/api/v1${path}`, {
            ...options,
            headers: host ? { Authorization: `Bearer ${host.token}`, ...(options.headers || {}) } : headers(options.headers),
        });
    } catch {
        return { ok: false, status: 0, data: null, errorCode: "serverUnreachable", errorMessage: explained("serverUnreachable") };
    }

    if (!host) {
        lose(response.status, used);
    }

    let body = null;

    try {
        body = await response.json();
    } catch {
        body = null;
    }

    return kernelAnswer(response.status, body);
}

// Le service des comptes : `{ok, status, item, detail, codes}` — `codes` : un refus que la page traduit (le mot de
// passe d'une personne, libs/password.js).
//
// UN 401 FERME LA SESSION, comme au kernel (chantier 55 : un compte réinitialisé ou supprimé revient à la connexion à son
// premier appel) — sauf celui de la connexion elle-même, qui dit « mauvais mot de passe ». Le lien d'un compte répond
// 410, jamais 401 : ouvert par quelqu'un de connecté, il ne le déconnecte pas.
export async function credential(path, options = {}) {
    const used = readToken();
    const target = remoteTarget;
    let response;

    try {
        response = await fetch(target ? `${target.origin}/api/v1${path}` : `/api/v1${path}`, {
            ...options,
            headers: target ? { Authorization: `Bearer ${target.token}`, ...(options.headers || {}) } : headers(options.headers),
        });
    } catch {
        return { ok: false, status: 0, item: null, detail: explained("serverUnreachable") };
    }

    // Un 401 du serveur visé ne ferme pas la session d'ici.
    // La connexion et son second temps (chantier 80) disent « mauvais mot de passe » ou « mauvais code ».
    if (!path.startsWith("/auth/login") && !target) {
        lose(response.status, used);
    }

    let body = null;

    try {
        body = await response.json();
    } catch {
        body = null;
    }

    return {
        ok: response.ok && body?.state !== "failed",
        status: response.status,
        item: body?.item ?? null,
        items: body?.items ?? null,
        pagination: body?.pagination ?? null,
        // Le refus du service des comptes, traduit par son code (chantier 59, P5) ; son anglais ne s'affiche jamais.
        detail: explained(body?.exception?.code || "", typeof body?.exception?.detail === "string" ? body.exception.detail : ""),
        codes: body?.exception?.codes || [],
        // Le code du refus lui-même (`mfa_expired`…), pour une page qui doit réagir à un refus précis.
        refusal: body?.exception?.code || "",
        retryAfter: Number(response.headers.get("Retry-After")) || 0,
    };
}

// LE NOM SOUS LEQUEL L'APPLICATION D'AUTHENTIFICATION RANGE LE CODE (chantier 81) : celui du serveur (sa capacité
// `serverName`, Réglages › Configuration du serveur), pour les appels qui peuvent commencer une double authentification.
const issued = (path) => `${path}?issuer=${encodeURIComponent(capabilities().serverName || "")}`;

export const login = (username, password) => credential(issued("/auth/login"), {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ username, password }),
});

// LE SECOND TEMPS DE LA CONNEXION (chantier 80) : le jeton d'étape que la connexion a rendu (`mfa: "code"` ou "enroll") et
// un code de l'application d'authentification, ou un code de secours. Rend la session comme la connexion.
export const loginMfa = (token, code) => credential("/auth/login/mfa", jsonBody("POST", { mfa_token: token, code }));

// MA DOUBLE AUTHENTIFICATION (chantier 80) : son état (`enabled`, `required`, `required_by` : "server" ou "member",
// `recovery_left`), la commencer (le mot de passe, puis le secret et son adresse otpauth), l'activer (un premier code ;
// rend les codes de secours), la désactiver ou refaire les codes de secours (le mot de passe et un code).
export const fetchMfa = () => credential("/auth/mfa");
export const startMfa = (password) => credential(issued("/auth/mfa/setup"), jsonBody("POST", { password }));
export const enableMfa = (code) => credential("/auth/mfa/enable", jsonBody("POST", { code }));
export const disableMfa = (password, code) => credential("/auth/mfa/disable", jsonBody("POST", { password, code }));
export const renewRecoveryCodes = (password, code) => credential("/auth/mfa/recovery", jsonBody("POST", { password, code }));
// L'ADMINISTRATEUR : l'imposer à tout le serveur (un serveur seulement), effacer celle d'un membre.
export const fetchMfaPolicy = () => credential("/auth/mfa/policy");
export const setMfaPolicy = (requiredAll) => credential("/auth/mfa/policy", jsonBody("PUT", { required_all: requiredAll }));
export const resetMemberMfa = (id) => credential(`/users/${id}/mfa/reset`, { method: "POST" });

export const fetchMe = () => credential("/auth/me");

// LES NOMS AFFICHÉS DE COMPTES, par leur identifiant (`POST /users/resolve`, comme le pool des parties de la webapp) :
// `item` est `{ identifiant: nom }`.
export const resolveUsers = (usernames) => credential("/users/resolve", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ usernames }) });

// Mon compte : changer mon nom (`display_name`) — la même route que Profile.jsx de la webapp.
export const updateMe = (patch) => credential("/auth/me", {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(patch),
});

// MON MOT DE PASSE (chantier 55) : la même route ; le serveur coupe les sessions du compte et rend un jeton neuf à cet
// appareil (renewSession) — les autres reviennent à la connexion.
export const changePassword = (current, next) => renewSession(() => updateMe({ current_password: current, new_password: next }));

// SE DÉCONNECTER DES AUTRES APPAREILS (cet appareil reçoit un jeton neuf), OU DE TOUS (chantier 55, la génération des
// sessions) : deux routes, deux intentions.
export const logoutOtherDevices = () => renewSession(() => credential("/auth/logout/others", { method: "POST" }));
export const logoutAllDevices = () => credential("/auth/logout/all", { method: "POST" });

// LE LIEN D'UN COMPTE (chantier 55, la page /join/<code>) : à qui il est, puis le mot de passe choisi — le code dans le
// corps, jamais dans une adresse. Un lien mort répond 410, trop d'essais 429 (`retryAfter`).
export const checkJoin = (code) => credential("/auth/join/check", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ code }) });
export const joinWithLink = (code, password) => credential(issued("/auth/join"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ code, password }) });

// Mon jeton (chantier 20) : `{token: {enabled, expires_at, token, use_count, last_used_at}, consoles}`.
export const fetchMyToken = () => credential("/auth/me/token");
// MON JETON, ET MOI SEUL (le porteur, 2026-11-20 : « Le membre l'active lui-même ») : l'activer pour une durée, le
// désactiver, le régénérer — sous ma session, jamais celle qu'il a ouverte ailleurs.
export const updateMyToken = ({ enabled, duration = "" }) => credential("/auth/me/token", jsonBody("PATCH", { enabled, duration }));
export const regenerateMyToken = () => credential("/auth/me/token/regenerate", { method: "POST" });

// MES FAVORIS (le porteur, 2026-11-16 ; le service des comptes, favorites.go) : ceux du compte connecté, sur tous ses
// appareils — `items` : `[{id, console_id, reference, server_id, title, created_at}]`. Un jeu d'un serveur fédéré
// (`srv<id>:<référence>`) porte aussi son serveur (`server_id`), le même que sa référence ; reposer un favori rend le
// même ; retirer un favori absent n'est pas une erreur.
export const listFavorites = () => credential("/auth/me/favorites");
export const addFavorite = ({ consoleId, reference, serverId = null, title = "" }) => credential("/auth/me/favorites",
    jsonBody("POST", { console_id: consoleId, reference, ...(serverId != null ? { server_id: Number(serverId) } : {}), ...(title ? { title } : {}) }));
export const removeFavorite = (consoleId, reference) => credential(`/auth/me/favorites?${new URLSearchParams({ console_id: consoleId, reference })}`, { method: "DELETE" });

// LES SERVEURS FÉDÉRÉS DONT CE COMPTE NE VOIT PAS LES JEUX (le porteur, 2026-11-16 ; le service des comptes,
// hidden_servers.go) : seulement les éteints — `items` : `[{server_id}]` ; un serveur se montre par défaut. L'éteindre
// (`PUT`), le rallumer (`DELETE`) : chacun s'enregistre à son changement, et suit le compte d'un appareil à l'autre.
export const listHiddenServers = () => credential("/auth/me/hidden-servers");
export const setServerHidden = (serverId, hidden) => credential(`/auth/me/hidden-servers/${encodeURIComponent(serverId)}`, { method: hidden ? "PUT" : "DELETE" });

// LES MEMBRES (admin/api/users.js) : la liste paginée côté serveur (`search` : des groupes de
// filtres en JSON ; `tokenEnabled` : ceux dont le jeton est activé, que le serveur joint à ses
// comptes), créer, modifier, supprimer ; le jeton d'un compte — l'activer pour une durée,
// le prolonger, le désactiver, le régénérer.
export function listUsers({ page = 1, itemsPerPage = 10, search = "", tokenEnabled = false } = {}) {
    const params = new URLSearchParams({ page, items_per_page: itemsPerPage });

    if (search) {
        params.set("search", search);
    }

    if (tokenEnabled) {
        params.set("token_enabled", "1");
    }

    return credential(`/users?${params}`);
}
const jsonBody = (method, body) => ({ method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
export const createUser = (data) => credential("/users", jsonBody("POST", data));
export const updateUser = (id, patch) => credential(`/users/${id}`, jsonBody("PATCH", patch));
export const deleteUser = (id) => credential(`/users/${id}`, { method: "DELETE" });
// Le lien d'un membre (chantier 55) : un nouveau pour celui qui attend ; la réinitialisation de celui qui a rejoint. La
// réponse est le membre, son lien et — cette fois seulement — son code (`invitation.code`).
export const newMemberLink = (id) => credential(`/users/${id}/invitation`, { method: "POST" });
export const resetMember = (id) => credential(`/users/${id}/reset`, { method: "POST" });

// LA FÉDÉRATION (Federations.jsx, Federation.jsx, api/invitations.js) : les serveurs enregistrés ici
// (`/server-registry`, le service des comptes, qui garde leurs jetons), ce que le kernel en sait
// (`/servers`, son listing) ; les jetons émis ici (`/catalog-tokens`) ; les invitations (un lien ou
// un QR que l'autre serveur échange contre un jeton, `/token-invitations`).
export const listServerRegistry = () => credential("/server-registry");
export const addServerRegistry = ({ link, serverToken, hostname, identity }) => credential("/server-registry", jsonBody("POST", { link, token: serverToken, hostname, identity }));
export const removeServerRegistry = (id) => credential(`/server-registry/${id}`, { method: "DELETE" });
export const checkFederationServer = (link, serverToken) => kernel("/servers/check", jsonBody("POST", { link, token: serverToken }));
export const redeemServerInvitation = (invitation) => kernel("/servers/redeem", jsonBody("POST", { invitation: invitation.trim() }));
// Rejoindre par un code d'appairage (chantier 60) : l'adresse tapée ou choisie, et les six chiffres.
export const redeemServerCode = (link, code) => kernel("/servers/redeem", jsonBody("POST", { link: link.trim(), code: code.trim() }));
export const fetchServer = (id) => kernel(`/servers/${encodeURIComponent(id)}`);
export const rescanServer = (id) => kernel(`/servers/${encodeURIComponent(id)}/rescan`, { method: "POST" });
export const listCatalogTokens = () => credential("/catalog-tokens");
// `remoteAdmin` : l'administration à distance par cet accès, choisie dès sa création (chantier 73) ; elle suit aussi
// l'invitation et le code d'appairage jusqu'à l'accès qu'ils font naître.
export const createCatalogToken = (label, scopes, remoteAdmin = false) => credential("/catalog-tokens", jsonBody("POST", { label, scopes, remote_admin: remoteAdmin }));
export const updateCatalogTokenScopes = (id, scopes) => credential(`/catalog-tokens/${id}`, jsonBody("PATCH", { scopes }));
// Les consoles d'un accès délivré (chantier 66) : ce que les comptes venus par lui peuvent lancer, comme ceux d'un membre.
export const updateCatalogTokenConsoles = (id, consoles) => credential(`/catalog-tokens/${id}`, jsonBody("PATCH", { consoles }));
// L'ADMINISTRATION À DISTANCE PAR CET ACCÈS (chantier 73) : coupée par défaut, accordée accès par accès.
export const updateCatalogTokenRemoteAdmin = (id, allowed) => credential(`/catalog-tokens/${id}`, jsonBody("PATCH", { remote_admin: allowed }));
// LE RELAIS DE LA FÉDÉRATION (chantier 83) : le porteur de cet accès peut-il nous relayer à ses appareils, et les
// serveurs fédérés qu'on lui relaie.
export const updateCatalogTokenRelayAllowed = (id, allowed) => credential(`/catalog-tokens/${id}`, jsonBody("PATCH", { relay_allowed: allowed }));
export const updateCatalogTokenRelayed = (id, servers) => credential(`/catalog-tokens/${id}`, jsonBody("PATCH", { relayed_servers: servers }));
// Les consoles coupées pour tout le serveur (chantier 66) : couper ou rallumer une console.
export const setConsoleSwitch = (id, enabled) => credential(`/console-switches/${encodeURIComponent(id)}`, jsonBody("PUT", { enabled }));
export const revokeCatalogToken = (id) => credential(`/catalog-tokens/${id}`, { method: "DELETE" });
export const createInvitation = ({ kind, label = "", scopes = [], remoteAdmin = false }) => credential("/token-invitations",
    jsonBody("POST", kind === "account" ? { kind } : { kind, label, scopes, remote_admin: remoteAdmin }));
export const listInvitations = () => credential("/token-invitations");
// LE CODE D'APPAIRAGE (chantier 60) : ouvrir ou prolonger un mode découverte (`server`, `account`), le fermer ; créer un
// code (rendu cette fois-là seulement), l'annuler. Son état revient par le sujet `pairing` (screens/access/Pairing.jsx).
export const openPairing = (scope) => credential(`/token-invitations/pairing/${scope}/open`, { method: "POST" });
export const closePairing = (scope) => credential(`/token-invitations/pairing/${scope}/close`, { method: "POST" });
export const createPairingCode = ({ kind, label = "", scopes = [], remoteAdmin = false }) => credential("/token-invitations/pairing/codes",
    jsonBody("POST", kind === "account" ? { kind } : { kind, label, scopes, remote_admin: remoteAdmin }));
export const cancelPairingCode = (id) => credential(`/token-invitations/pairing/codes/${id}`, { method: "DELETE" });
export const revokeInvitation = (id) => credential(`/token-invitations/${id}`, { method: "DELETE" });

// LE QR D'UNE INVITATION : son image (le kernel la dessine), et le lien lu dans une image reçue.
export async function invitationQR(text, size = 512) {
    try {
        const response = await fetch("/api/v1/qr", { method: "POST", headers: headers({ "Content-Type": "application/json" }), body: JSON.stringify({ text, size }) });

        return response.ok ? await response.blob() : null;
    } catch {
        return null;
    }
}
export const readInvitationImage = (file) => kernel("/qr/read", { method: "POST", headers: { "Content-Type": file.type || "application/octet-stream" }, body: file });

// Les types de notification de ce compte (`notification_type`, `enabled`) ; tous rendus, activés
// par défaut (Notifications.jsx de la webapp).
export const listNotificationPreferences = () => credential("/notification-preferences");

// ─── L'administration (Réglages), les mêmes routes que la webapp ─────────────
// storage-admin.js et profile/api.js de la webapp.

// Le listing de cette machine (sa date), et TOUT RESCANNER : jeux, BIOS, cœurs, correctifs, paramètres.
export const fetchListing = () => kernel("/retrogaming/listing");
export const rescanListing = () => kernel("/retrogaming/listing/rescan", { method: "POST" });

// Le cache du dossier (#448) : ce que le premier lancement d'un jeu installe, par console.
export const fetchCache = () => kernel("/retrogaming/storage/cache");
export const deleteCacheEntry = (consoleId, name) => kernel(`/retrogaming/storage/cache?consoleId=${encodeURIComponent(consoleId)}&name=${encodeURIComponent(name)}`, { method: "DELETE" });

// Les sauvegardes, par compte : la place qu'elles prennent, les emporter, les reprendre.
// Les journaux de cette machine (storage/Logs.jsx) : la liste, les 100 dernières lignes d'un
// journal, son fichier (par une adresse, le jeton en paramètre), et sa suppression.
export const fetchLogs = () => kernel("/retrogaming/storage/logs");
export const fetchLogTail = (path) => kernel(`/retrogaming/storage/logs/tail?path=${encodeURIComponent(path)}`);
export const deleteLog = (path) => kernel(`/retrogaming/storage/logs?path=${encodeURIComponent(path)}`, { method: "DELETE" });
export const logFileUrl = (path) => `/api/v1/retrogaming/storage/logs/file?path=${encodeURIComponent(path)}&token=${encodeURIComponent(readToken() || "")}`;
// CE QUI S'ÉCRIT DANS LES JOURNAUX (le porteur, 2026-11-14) : `{kernel, levels, http, sessions}`, lu et changé par un
// administrateur ; le kernel le suit dès sa ligne suivante. Changer n'envoie que ce qui change.
export const fetchLogSettings = () => kernel("/retrogaming/storage/logs/settings");
export const saveLogSettings = (patch) => kernel("/retrogaming/storage/logs/settings", jsonBody("PUT", patch));
// LA CONFIGURATION DU SERVEUR (chantier 81, server_config.go) : ses réglages et, à côté (`fixed`), ce que fixe le compose.
export const fetchServerConfig = () => kernel("/retrogaming/storage/server-config");
export const saveServerConfig = (values) => kernel("/retrogaming/storage/server-config", jsonBody("PUT", values));

export const fetchUserSaves = () => kernel("/retrogaming/storage/saves");
export const deleteUserSaves = (username) => kernel(`/retrogaming/storage/saves?user=${encodeURIComponent(username)}`, { method: "DELETE" });
export const userSavesArchiveUrl = (username) => `/api/v1/retrogaming/storage/saves/archive?user=${encodeURIComponent(username)}&token=${encodeURIComponent(readToken() || "")}`;

// Les comptes des services de métadonnées (chantier 49) : la liste ne rend que les champs remplis,
// jamais leurs valeurs ; un champ vide envoyé garde la valeur enregistrée.
export const listMetadataServices = () => credential("/metadata-services");
export const saveMetadataService = (service, fields) => credential(`/metadata-services/${encodeURIComponent(service)}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ fields }),
});
export const removeMetadataService = (service) => credential(`/metadata-services/${encodeURIComponent(service)}`, { method: "DELETE" });
// ÉCRIRE LES MÉTADONNÉES SUR LE DISQUE DU JEU (le porteur, 2026-11-14) : `{write_on_game_disk}`, lu par tout compte,
// changé par un administrateur ; le kernel en tire où vivent les métadonnées d'un jeu d'un disque interne.
export const fetchMetadataSettings = () => credential("/metadata-services/settings");
export const saveMetadataSettings = (settings) => credential("/metadata-services/settings", jsonBody("PUT", settings));
// GOOGLE CAST (cast.go du service des comptes) : l'identifiant du récepteur — lu par tout compte, réglé par un administrateur.
export const fetchCastSettings = () => credential("/cast");
export const saveCastSettings = (applicationId) => credential("/cast", jsonBody("PUT", { application_id: applicationId }));

// Revenir à l'accueil de l'application Android (Settings.jsx de la webapp, returnToAppHome).
export async function returnToAppHome(capabilities) {
    if (capabilities?.appHomeLink) {
        window.location.href = "gameflex://home";

        return true;
    }

    try {
        const response = await fetch("/api/v1/device/home", { method: "POST" });

        return response.ok;
    } catch {
        return false;
    }
}

// Le registre des consoles : sans jeton, comme dans ConsolesContext.jsx.
export async function fetchConsoles() {
    try {
        const response = await fetch("/api/v1/consoles");
        const body = await response.json();
        const items = Array.isArray(body) ? body : body?.data;

        return Array.isArray(items) ? items : [];
    } catch {
        return [];
    }
}

export async function fetchCapabilities() {
    try {
        const response = await fetch("/api/v1/capabilities", { cache: "no-store" });
        const body = await response.json();

        return body?.capabilities || {};
    } catch {
        return {};
    }
}

// Les jeux d'une console, d'UNE source (Games.jsx) : la bibliothèque, un
// disque interne (`disk`), ou un serveur fédéré (`server`).
export function listRoms(consoleId, { query = "", disk = null, server = null, resource = null } = {}) {
    const params = new URLSearchParams();

    if (query) {
        params.set("q", query);
    }

    if (disk) {
        params.set("disk", disk);
    }

    if (server != null) {
        params.set("server", String(server));
    }

    // Une ressource externe (`#808`) : ses jeux, à importer (Réglages › Consoles › Jeux ; administrateur).
    if (resource != null) {
        params.set("resource", String(resource));
    }
    const search = params.toString();

    return kernel(`/retrogaming/${encodeURIComponent(consoleId)}/roms${search ? `?${search}` : ""}`);
}

export const listDisks = () => kernel("/retrogaming/disks");
export const rescanDisk = (name) => kernel(`/retrogaming/disks/${encodeURIComponent(name)}/rescan`, { method: "POST" });

// LES RESSOURCES EXTERNES (`#808`, kernel : resources_api.go) — Samba, WebDAV, kDrive, FTP, SFTP ; administrateur. Leur
// liste vit aussi sur le sujet `resources` du centre de messages (Resources.jsx), qui la repousse à chaque parcours.
const asJSON = (method, body) => ({ method, headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });
export const listResources = () => kernel("/retrogaming/resources");
// Ce que tout compte voit des ressources (le Catalogue) : leur nom, leur genre, leurs jeux par console.
export const listLentResources = () => kernel("/retrogaming/resources/catalog");
export const addResource = (body) => kernel("/retrogaming/resources", asJSON("POST", body));
export const updateResource = (id, body) => kernel(`/retrogaming/resources/${encodeURIComponent(id)}`, asJSON("PATCH", body));
export const removeResource = (id) => kernel(`/retrogaming/resources/${encodeURIComponent(id)}`, { method: "DELETE" });
export const rescanResource = (id) => kernel(`/retrogaming/resources/${encodeURIComponent(id)}/rescan`, { method: "POST" });

// LE NOMBRE DE JEUX D'ICI PAR CONSOLE (`handleRetroConsoles`) : la bibliothèque et les
// disques internes réunis, une même référence comptée une fois — ce que la bibliothèque
// d'une console montre. Lu dans le listing de ce serveur et le catalogue gardé des disques,
// sans parcours du disque. Une console sans jeu n'y figure pas : 0.
export const listConsoleCounts = () => kernel("/retrogaming/consoles");

// CE QUE LA FICHE D'UN JEU DE LA WEBAPP LIT (GameDetailPanel.jsx) : les cœurs
// de la console (installés ou non), les BIOS présents, et le ticket qui ouvre
// le téléchargement d'un jeu (consommé sans en-tête, par une adresse).
export const listCores = (consoleId) => kernel(`/retrogaming/${encodeURIComponent(consoleId)}/cores`);
// Le défaut d'une console (chantier 53) : l'émulateur en service et le BIOS par défaut — `stored`, ce qui est
// enregistré ; `inUse`, ce qu'une partie lancée sans choix prendrait. L'enregistrer (un administrateur) :
// `{core?, emulatorFile?, bios?}`, un champ absent garde sa valeur, "" l'efface ; la réponse est la même vue.
export const consoleDefaults = (consoleId) => kernel(`/retrogaming/${encodeURIComponent(consoleId)}/defaults`);
export const setConsoleDefaults = (consoleId, patch) => kernel(`/retrogaming/${encodeURIComponent(consoleId)}/defaults`, {
    method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(patch),
});
export const listBios = (consoleId) => kernel(`/retrogaming/${encodeURIComponent(consoleId)}/bios`);
export const downloadTicket = (consoleId, reference) => kernel(`/retrogaming/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}/ticket`, { method: "POST" });
export const ticketDownloadUrl = (ticket) => `/api/v1/retrogaming/downloads/${encodeURIComponent(ticket)}`;
// L'ARCHIVE DES MÉTADONNÉES D'UN JEU (le Catalogue ; metadata_archive.go) : son dossier `metadata/<console>/<gameid>/`
// en .zip, par le même mécanisme — le ticket (404 `no_metadata` pour un jeu qui n'en a pas encore ; celle d'un jeu
// fédéré se lit chez son serveur), puis l'adresse que le navigateur suit, une fois.
export const metadataArchiveTicket = (consoleId, reference) => kernel(`/retrogaming/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}/metadata/ticket`, { method: "POST" });
export const metadataArchiveUrl = (ticket) => `/api/v1/retrogaming/downloads/metadata/${encodeURIComponent(ticket)}`;

// LES SAUVEGARDES DE CE COMPTE (en lecture) : les états du rétro (tous les jeux,
// comme « Mes sauvegardes »), la SRAM d'un jeu (game_saves.go), et les jeux des
// consoles récentes qui ont une sauvegarde (par identifiant de jeu).
// UNE CONSOLE : les fonds de sa bande, en huit emplacements (chantier 53, P9 ; le porteur, 2026-11-14, remarque e) —
// `{slots: [{slot, name}], extra: [noms]}` : `name` vide, un emplacement libre ; `extra`, les images du dossier en trop,
// que la bande ne montre pas et que rien n'efface.
const backgroundsPath = (consoleId) => `/retrogaming/consoles/${encodeURIComponent(consoleId)}/backgrounds`;
export const listConsoleBackgrounds = (consoleId) => kernel(backgroundsPath(consoleId));
// Un administrateur : les images que les fournisseurs proposent pour un titre (`{query, kind, items, sources}`) ;
// l'une d'elles, choisie (`{source, url}`), dans un emplacement de 1 à 8 — le serveur la récupère chez l'hôte d'images de
// sa source, et seulement là ; les images par défaut redemandées (à la visite suivante, dans les emplacements libres).
// Un emplacement ne se vide pas : une image se remplace (le porteur, 2026-11-14).
export const searchConsoleBackgrounds = (consoleId, query) => kernel(`${backgroundsPath(consoleId)}/search?${new URLSearchParams({ q: query })}`);
export const setConsoleBackground = (consoleId, slot, { source, url }) => kernel(`${backgroundsPath(consoleId)}/slots/${slot}`, jsonBody("PUT", { source, url }));
export const resetConsoleBackgrounds = (consoleId) => kernel(`${backgroundsPath(consoleId)}/reset`, { method: "POST" });
// UN emplacement qui reprend son image par défaut (l'image de fond d'une console Remote Play, chantier 53, P12).
export const resetConsoleBackgroundSlot = (consoleId, slot) => kernel(`${backgroundsPath(consoleId)}/slots/${slot}/reset`, { method: "POST" });
export const listUserStates = (source = "", host = null) => kernel(`/retrogaming/savestate/user${source ? `?source=${encodeURIComponent(source)}` : ""}`, {}, host);
export const saveRAMOf = (consoleId, reference) => kernel(`/retrogaming/savestate/sram?consoleId=${encodeURIComponent(consoleId)}&reference=${encodeURIComponent(reference)}`);
export const listStreamexSaves = (source = "", host = null) => kernel(`/saves/streamex${source ? `?source=${encodeURIComponent(source)}` : ""}`, {}, host);
export const stateThumbnailPath = (consoleId, reference, slot, source = "") => `/api/v1/retrogaming/savestate/screenshot?${new URLSearchParams({ consoleId, reference, slot, ...(source ? { source } : {}) })}`;

// MES SAUVEGARDES PAR SOURCE (chantier 38, SavedStates.jsx de la webapp) : le principal, que les
// parties jouent ; des copies (`local` : les backups ; une par serveur fédéré) ; et celles d'un
// serveur fédéré, sous notre compte de là-bas (`host`). Rien de ce qui voyage n'écrase une
// sauvegarde : un envoi arrive dans une copie d'en face, un téléchargement dans une copie d'ici.
const withSource = (params, source) => new URLSearchParams({ ...params, ...(source ? { source } : {}) });
const postJson = (body) => ({ method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(body) });

export const listSaveSources = () => kernel("/saves/sources");
export const listSaveSlots = (consoleId, reference, source = "", host = null) => kernel(`/retrogaming/savestate/slots?${withSource({ consoleId, reference }, source)}`, {}, host);
export const deleteSaveSlot = (consoleId, reference, slot, source = "") => kernel(`/retrogaming/savestate?${withSource({ consoleId, reference, slot }, source)}`, { method: "DELETE" });
export const deleteGameSaves = (consoleId, reference, source = "") => kernel(`/retrogaming/savestate/game?${withSource({ consoleId, reference }, source)}`, { method: "DELETE" });
// Un backup emporte sa note (`note`, facultative : pourquoi on le fait), posée à côté de chaque sauvegarde copiée.
export const backupStates = (items, note = "") => kernel("/saves/states/backup", postJson({ items, note }));
export const restoreStates = (source, items) => kernel("/saves/states/restore", postJson({ source, items }));
export const backupStreamexSaves = (console, ids, note = "") => kernel("/saves/streamex/backup", postJson({ console, ids, note }));
export const restoreStreamexSaves = (source, console, ids) => kernel("/saves/streamex/restore", postJson({ source, console, ids }));
export const deleteStreamexSave = (console, id, source) => kernel(`/saves/streamex/game?${withSource({ console, id }, source)}`, { method: "DELETE" });
// LES NOTES DES BACKUPS (saves/notes.go) : toutes celles du compte (`{notes: [{kind, console, reference, slot | id, note,
// date, account}]}`) ; une, écrite, corrigée ou vidée (`note` vide) — `item` : `{kind: "states", console, reference,
// slot}` ou `{kind: "streamex", console, id}` ; la réponse rend `{item}`, nul quand il n'y en a plus.
export const listSaveNotes = () => kernel("/saves/notes");
export const writeSaveNote = (item, note) => kernel("/saves/notes", jsonBody("PUT", { ...item, note }));

// Les octets d'une sauvegarde (un état, sa vignette, l'archive d'un jeu récent), ici ou chez `host`.
// `onProgress(reçus, total)` : la lecture par morceaux, pour une ligne de téléchargement (DownloadRow) ; `signal` : son
// annulation (AbortController) — `aborted` le dit.
export async function fetchBytes(path, host = null, { onProgress = null, signal = undefined } = {}) {
    const token = host ? host.token : readToken();

    try {
        const response = await fetch(host ? `${host.origin}/api/v1${path}` : `/api/v1${path}`, { headers: token ? { Authorization: `Bearer ${token}` } : {}, signal });

        if (!response.ok) {
            return { ok: false, status: response.status };
        }
        if (!onProgress || !response.body) {
            return { ok: true, blob: await response.blob() };
        }
        const total = Number(response.headers.get("Content-Length")) || 0;
        const reader = response.body.getReader();
        const chunks = [];
        let received = 0;

        for (;;) {
            const { done, value } = await reader.read();
            if (done) {
                break;
            }
            chunks.push(value);
            received += value.length;
            onProgress(received, total);
        }

        return { ok: true, blob: new Blob(chunks, { type: response.headers.get("Content-Type") || "" }) };
    } catch {
        return { ok: false, status: 0, aborted: Boolean(signal?.aborted) };
    }
}

export const fetchStateFile = (consoleId, reference, slot, source, host, screenshot = false, options = {}) => fetchBytes(`/retrogaming/savestate/${screenshot ? "screenshot" : "file"}?${withSource({ consoleId, reference, slot }, source)}`, host, options);
export const fetchStreamexSave = (console, id, source, host, options = {}) => fetchBytes(`/saves/streamex/game?${withSource({ console, id }, source)}`, host, options);

// ÉCRIRE dans une COPIE (`source` : le nom que la fédération donne à celui qui envoie). Le corps
// est les octets ; `timestamp` garde la date de l'original, `title` le titre du jeu.
export function uploadStateFile(consoleId, reference, slot, blob, { host = null, source, timestamp = null, title = "", screenshot = false }) {
    const params = new URLSearchParams({ consoleId, reference, slot, source });
    const millis = timestamp == null ? NaN : new Date(timestamp).getTime();

    if (millis > 0) {
        params.set("timestamp", String(millis));
    }

    if (title) {
        params.set("title", title);
    }

    if (screenshot) {
        params.set("screenshot", "1");
    }

    return kernel(`/retrogaming/savestate/file?${params}`, { method: "POST", body: blob }, host);
}

// `game` : ce que la liste d'où vient la sauvegarde dit de son jeu (`title`, `reference`, `path`) — la copie le garde, et
// la sauvegarde garde son nom chez un serveur qui n'a pas le jeu (chantier 76, titles.json).
export function uploadStreamexSave(console, id, blob, source, host = null, game = {}) {
    const params = withSource({ console, id }, source);

    for (const [key, value] of [["title", game.title], ["gameId", game.reference], ["path", game.path]]) {
        if (value) {
            params.set(key, value);
        }
    }

    return kernel(`/saves/streamex/game?${params}`, { method: "POST", body: blob }, host);
}
export const listEmulatorParams = () => kernel("/retrogaming/storage/params");

// LE STOCKAGE (storage-admin.js) : les émulateurs (cœurs), les BIOS, les paramètres, ici ou chez une
// source — un disque interne (`disk:<nom>`), un serveur fédéré (`peer:<id>`) —, et retirer d'ici. Un émulateur, un
// BIOS, un correctif ou un jeu se TÉLÉCHARGE par la commande `download` du canal du kernel, qui dit sa progression
// (`runDownload`, libs/downloads.js). Les paramètres d'une source ne s'importent plus (chantier 53) : ce qu'elle prête
// est un préréglage du formulaire d'ici (`fetchRequired("params", source, { console, emulator })`, chaque prêt avec
// ses valeurs), qui n'écrit qu'à « Enregistrer ».
const STORAGE = "/retrogaming/storage";
const parseSource = (descriptor) => {
    const [kind, id] = String(descriptor || "local").split(":");

    return { kind, id: id || null };
};
const sourceBase = (descriptor) => {
    const { kind, id } = parseSource(descriptor);

    return kind === "peer" ? `/servers/${encodeURIComponent(id)}/storage` : kind === "disk" ? `/retrogaming/disks/${encodeURIComponent(id)}/storage` : STORAGE;
};
export const fetchRequired = (section, descriptor = "local", query = null) => kernel(`${sourceBase(descriptor)}/${section}${query ? `?${new URLSearchParams(query)}` : ""}`);
export const deleteInstalled = (section, consoleId, name) => kernel(`${STORAGE}/${section}?${new URLSearchParams({ consoleId, [section === "cores" ? "core" : "name"]: name })}`, { method: "DELETE" });
// QUI L'A, DEMANDÉ À CHAQUE SOURCE (chantier 53, P5) : la source officielle (un cœur chez le buildbot, un émulateur
// de console récente chez son éditeur), puis chaque serveur fédéré — `{sources: [{source, serverId, name,
// available, reason}]}`, avec la raison de celle qui ne l'a pas.
export const fetchAvailability = (consoleId, section, name) => kernel(`${STORAGE}/availability?${new URLSearchParams({ console: consoleId, section, name })}`);
// LA SOURCE D'UN JEU, SUR CHAQUE APPEL QUI LE VISE (ses images, sa fiche, ses corrections) : un jeu d'un
// disque interne porte `disk=<nom>` — ses métadonnées vivent sur ce disque (cedfcf79), et sans lui le
// kernel prend la copie de la bibliothèque quand le jeu est aux deux endroits ; un jeu d'un serveur
// fédéré porte `server=<id>` — comme la liste des jeux et « Jouer ». `romSource(rom)` la tire d'un jeu
// listé, `gameQuery(source)` l'écrit dans une adresse ; aucun appel ne s'en passe.
export const romSource = (rom) => (rom?.peer != null ? { server: String(rom.peer) } : rom?.disk ? { disk: rom.disk } : {});
export const gameQuery = (source = {}) => {
    const params = new URLSearchParams();

    if (source.server != null && source.server !== "") {
        params.set("server", String(source.server));
    } else if (source.disk) {
        params.set("disk", source.disk);
    }

    return params;
};
// Une adresse et sa source : `?…` ou `&…` selon qu'elle en a déjà.
const withGameSource = (path, source) => {
    const query = gameQuery(source).toString();

    return query ? `${path}${path.includes("?") ? "&" : "?"}${query}` : path;
};

// LA FICHE D'UN JEU, CORRIGÉE (un administrateur) : le texte à la main (un champ absent ne change pas ;
// null, "" ou [] l'efface), une image à la main par sorte, les candidats d'un service, un jeu désigné
// (ou la recherche refaite), une source oubliée. Un serveur fédéré est un service de plus (`server:<id>`, chantier 57) :
// ses candidats, et le jeu désigné chez lui, dont la fiche se copie ici.
const gameMeta = (consoleId, reference) => `/retrogaming/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}/metadata`;
// `game` — la source du jeu (romSource) ; `service` — le service des métadonnées (igdb, steamgriddb…).
export const gameMetadataPath = (consoleId, reference, game, suffix = "") => withGameSource(`/api/v1${gameMeta(consoleId, reference)}${suffix}`, game);
const gameMetaCall = (consoleId, reference, game, suffix = "") => withGameSource(`${gameMeta(consoleId, reference)}${suffix}`, game);
export const saveManualMetadata = (consoleId, reference, game, patch) => kernel(gameMetaCall(consoleId, reference, game, "/manual"), { method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify(patch) });
// UNE IMAGE SE CHOISIT (chantier 53, P9 ; le porteur, 2026-11-14, remarque d) : les images d'une sorte que proposent
// les fournisseurs, puis mes serveurs fédérés (chantier 57 ; `query` vide : le titre du jeu ; `{query, kind, items,
// sources}`), puis l'une d'elles — `{source, url}`, que le serveur récupère chez l'hôte d'images de sa source ; celle
// d'un serveur fédéré, `{source: "server", server, reference}`, qu'il récupère chez lui, jamais par une adresse. Une
// image seule ne se supprime pas, elle se remplace ; « Réinitialiser la fiche » (`dropMetadataSource(…, "manual")`)
// remet tout, textes et images.
export const searchGameImages = (consoleId, reference, game, kind, query = "") => kernel(gameMetaCall(consoleId, reference, game, `/images/search?${new URLSearchParams({ kind, ...(query ? { q: query } : {}) })}`));
export const putManualImage = (consoleId, reference, game, kind, { source, url, server, reference: lent }) => kernel(gameMetaCall(consoleId, reference, game, `/manual/images/${encodeURIComponent(kind)}`),
    jsonBody("PUT", source === "server" ? { source, server, reference: lent } : { source, url }));
export const metadataCandidates = (consoleId, reference, game, service, query = "") => kernel(gameMetaCall(consoleId, reference, game, `/candidates?${new URLSearchParams({ source: service, ...(query ? { q: query } : {}) })}`));
export const fetchMetadataSource = (consoleId, reference, game, service, externalId = "") => kernel(gameMetaCall(consoleId, reference, game, "/fetch"), { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify(externalId ? { source: service, externalId } : { source: service }) });
export const dropMetadataSource = (consoleId, reference, game, service) => kernel(gameMetaCall(consoleId, reference, game, `?source=${encodeURIComponent(service)}`), { method: "DELETE" });
// LA FICHE D'UN JEU FÉDÉRÉ, COPIÉE DANS MES MÉTADONNÉES SANS LE JEU (le porteur, 2026-11-16, 2.4.1 ;
// federation_metadata.go, handleFederatedSheetCopy) : `{gameId, source}` ; 409 `already_here` pour un jeu déjà ici.
export const copyFederatedSheet = (consoleId, reference, game) => kernel(gameMetaCall(consoleId, reference, game, "/copy"), { method: "POST" });
// LES MÉTADONNÉES SANS JEU d'une console (chantier 53, metadata_orphans.go) : `{orphans: [{gameId, disk, title,
// cover}]}` — les dossiers qu'aucun jeu accessible ne porte —, la jaquette que l'un garde, et le retirer (`disk` : le
// disque qui le porte, vide pour le dossier du serveur ; le kernel revérifie qu'il est toujours orphelin).
const orphanCall = (consoleId, gameId = null, disk = "", suffix = "") => `/retrogaming/${encodeURIComponent(consoleId)}/metadata/orphans${gameId ? `/${encodeURIComponent(gameId)}${suffix}` : ""}${disk ? `?disk=${encodeURIComponent(disk)}` : ""}`;
export const listMetadataOrphans = (consoleId) => kernel(orphanCall(consoleId));
export const deleteMetadataOrphan = (consoleId, gameId, disk) => kernel(orphanCall(consoleId, gameId, disk), { method: "DELETE" });
export const metadataOrphanCoverPath = (consoleId, gameId, disk) => `/api/v1${orphanCall(consoleId, gameId, disk, "/cover")}`;
// L'adresse d'une image d'un jeu (libs/images.js), avec sa source.
export const gameImagePath = (consoleId, reference, kind, width, game) => withGameSource(`/api/v1/retrogaming/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}/images/${kind}?w=${width}`, game);

// LE STATUT D'UN JEU D'ICI (chantier 30 ; un administrateur ; les routes de la webapp, api/retrogaming.js) : `{view,
// console, reports, emulators}` — sa vue (le statut de chaque émulateur de la console, celle de la liste des jeux), ses
// signalements, et les émulateurs de la console, installés ou non, avec leur version. Un signalement : `{status,
// emulators, note}`, `emulators` vide pour tous ; il répond la même chose. Le statut vit dans le dossier du serveur, sous
// l'identifiant du jeu : aucune source à passer.
const statusPath = (consoleId, reference) => `/retrogaming/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}/status`;
export const fetchGameStatus = (consoleId, reference) => kernel(statusPath(consoleId, reference));
export const reportGameStatus = (consoleId, reference, { status, emulators = [], note = "" }) => kernel(statusPath(consoleId, reference), postJson({ status, emulators, note }));
// Retirer un signalement de l'historique, par son instant (chantier 68) : la réponse est le statut recalculé.
export const removeGameStatus = (consoleId, reference, at) => kernel(`${statusPath(consoleId, reference)}?at=${encodeURIComponent(at)}`, { method: "DELETE" });
// LE STATUT DE TOUTE UNE CONSOLE (chantier 30 ; chantier 56, A2) : `{status, by, at, default}` — celui que prend chaque
// jeu sans statut à lui, y compris un jeu ajouté plus tard ; `status` vide quand l'administrateur n'en a pas posé (le
// défaut de la console, `default`, vaut alors).
export const fetchConsoleStatus = (consoleId) => kernel(`/retrogaming/${encodeURIComponent(consoleId)}/status`);
export const setConsoleStatus = (consoleId, status) => kernel(`/retrogaming/${encodeURIComponent(consoleId)}/status`, postJson({ status }));

// Un jeu de la bibliothèque (ou d'un disque interne) retiré.
export const deleteRom = (consoleId, reference, disk = null) => kernel(`/retrogaming/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}${disk ? `?disk=${encodeURIComponent(disk)}` : ""}`, { method: "DELETE" });
// LE CORRECTIF D'UN JEU (chantier 53, P6 ; un administrateur) : `{gameId, folder, installed, sources}` — son
// identifiant, son dossier `patches/<console>/<id>/`, celui installé ici (ou null), et chaque source qui l'a (un disque
// interne, un serveur fédéré), avec la raison de celle qui ne répond pas. `game` : la source du jeu (romSource).
export const gamePatch = (consoleId, reference, game) => kernel(withGameSource(`/retrogaming/${encodeURIComponent(consoleId)}/roms/${encodeURIComponent(reference)}/patch`, game));
const emulatorPath = (consoleId, emulator) => `${STORAGE}/params/${encodeURIComponent(consoleId)}/${encodeURIComponent(emulator)}`;
export const writeEmulatorParams = (consoleId, emulator, { enabled, values, preset = "", engines }) => kernel(emulatorPath(consoleId, emulator), {
    method: "PUT", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ enabled, values, preset, ...(engines ? { engines } : {}) }),
});
export const deleteEmulatorPreset = (consoleId, emulator, presetId) => kernel(`${emulatorPath(consoleId, emulator)}/presets/${encodeURIComponent(presetId)}`, { method: "DELETE" });
export const listServers = () => kernel("/servers");
export const listDevices = () => kernel("/playstation/my-devices");
export const listFederatedDevices = () => kernel("/servers/remoteplay/devices");
// L'ÉTAT DES CONSOLES D'UN SERVEUR FÉDÉRÉ, POUSSÉ (`#750`) : le sujet `servers/<id>/devices` de notre kernel, qui relaie
// celles que le serveur pousse, sous SES identifiants ; une liste vide quand il est perdu. Un refus (un serveur absent du
// registre) ferme l'abonnement.
export const subscribeFederatedDevices = (serverId, onList) => subscribeTopic(`servers/${serverId}/devices`, (data) => onList(Array.isArray(data) ? data : []), () => {});
// Allumer une console Remote Play (un paquet de réveil) : ici, ou chez un serveur fédéré (`host`), sous
// l'identifiant que CET hôte lui connaît.
export const wakeDevice = (deviceId, host = null) => kernel(`/playstation/wake/${encodeURIComponent(deviceId)}`, { method: "POST" }, host);

// Le canal du kernel (`/api/v1/facade/ws`, libs/kernelChannel.js de la
// webapp) : on s'abonne au sujet `sessions`, la liste vivante des parties.
// Pas de sondage (règle du projet) ; une reconnexion qui s'espace si le
// kernel se tait.
// Les parties en cours, en direct (le sujet `sessions`).
export const subscribeSessions = (onList) => subscribeTopic("sessions", (data) => onList(Array.isArray(data) ? data : []));

// L'adresse du canal du kernel, le jeton en paramètre (une poignée de main WebSocket ne porte pas d'en-tête).
// LE CANAL DU KERNEL VISÉ : le nôtre, ou celui du serveur fédéré qu'on administre d'ici (chantier 73) — une commande
// (un téléchargement) partie de ses réglages se fait chez lui, pas sur cet appareil.
const facadeAddress = () => {
    if (remoteTarget) {
        const origin = new URL(remoteTarget.origin);

        return `${origin.protocol === "https:" ? "wss:" : "ws:"}//${origin.host}/api/v1/facade/ws?token=${encodeURIComponent(remoteTarget.token)}`;
    }
    const token = readToken();

    return `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}/api/v1/facade/ws${token ? `?token=${encodeURIComponent(token)}` : ""}`;
};

// UN SUJET DU CANAL DU KERNEL (`/facade/ws`) : `sessions`, `download/<id>` (un téléchargement)… ; reconnecté de lui-même. `onRefused(raison)` : le kernel
// refuse l'abonnement (un téléchargement qu'il ne connaît plus) — on ne se réabonne pas. Rend de quoi se
// désabonner.
export function subscribeTopic(topic, onData, onRefused = null) {
    let socket = null;
    let closed = false;
    let retryMs = 1000;
    let timer = null;

    const connect = () => {
        if (closed) {
            return;
        }
        let opened = false;
        socket = new WebSocket(facadeAddress());
        socket.onopen = () => {
            opened = true;
            retryMs = 1000;
            socket.send(JSON.stringify({ type: "subscribe", id: "subscribe", data: { topic } }));
        };
        socket.onmessage = (event) => {
            try {
                const frame = JSON.parse(event.data);

                if (frame.topic === topic) {
                    onData(frame.data);
                } else if (frame.type === "reply" && frame.id === "subscribe" && frame.status >= 400 && onRefused) {
                    closed = true;
                    socket.close(1000);
                    onRefused(frame.data?.errorMessage || "");
                }
            } catch {
                // Une trame illisible ne casse pas la liste.
            }
        };
        // Fermé en 1008 (la session est finie), ou refusé à sa poignée de main (un jeton que le kernel ne prend plus,
        // un appareil qui sort de veille) : la session s'examine d'abord (examineSession), puis le canal se reconnecte
        // — sauf si elle est fermée entre-temps.
        socket.onclose = (event) => {
            if (closed) {
                return;
            }
            const wait = event.code === SESSION_ENDED || !opened ? examineSession() : Promise.resolve();
            wait.then(() => {
                if (!closed) {
                    timer = setTimeout(connect, retryMs);
                    retryMs = Math.min(retryMs * 2, 10000);
                }
            });
        };
    };
    connect();

    return () => {
        closed = true;
        clearTimeout(timer);
        socket?.close(1000);
    };
}

// UNE COMMANDE DU CANAL DU KERNEL (`{type, id, data}` → la réponse `reply` du même `id`, CODE_CONVENTIONS.md, « le
// centre de messages ») : le temps de sa réponse, puis le canal se referme. Rend la forme de `kernel` — le corps de
// la réponse est celui qu'aurait rendu la route. `download` en est une (libs/downloads.js).
export function kernelCommand(type, data) {
    return new Promise((resolve) => {
        let socket;

        try {
            socket = new WebSocket(facadeAddress());
        } catch {
            resolve({ ok: false, status: 0, data: null, errorCode: "serverUnreachable", errorMessage: explained("serverUnreachable") });

            return;
        }
        let answered = false;
        const answer = (result) => {
            if (!answered) {
                answered = true;
                socket.close(1000);
                resolve(result);
            }
        };
        socket.onopen = () => socket.send(JSON.stringify({ type, id: "command", data }));
        socket.onmessage = (event) => {
            try {
                const frame = JSON.parse(event.data);

                if (frame.type === "reply" && frame.id === "command") {
                    answer(kernelAnswer(frame.status, frame.data));
                }
            } catch {
                // Une trame illisible n'est pas la réponse.
            }
        };
        socket.onclose = () => answer({ ok: false, status: 0, data: null, errorCode: "serverUnreachable", errorMessage: explained("serverUnreachable") });
    });
}

// LE CANAL DE PRÉSENCE (`/api/v1/presence/ws`, PresenceContext.jsx de la
// webapp) : on y lit les invitations (« X t'invite à regarder ») ; comme dans la
// webapp, y être connecté annonce le compte en ligne, et la page de jeu y dit à
// quoi l'on joue (`reportPresence`).
let presenceSocket = null;

// « Je joue à… » / « je ne joue plus » (reportPlaying, reportIdle de la webapp) : envoyé seulement
// quand le canal est ouvert, comme chez elle — il se redit à la prochaine partie.
export function reportPresence(message) {
    if (presenceSocket?.readyState === WebSocket.OPEN) {
        presenceSocket.send(JSON.stringify(message));
    }
}

export function subscribeInvites(onInvite) {
    let socket = null;
    let closed = false;
    let timer = null;

    const connect = () => {
        if (closed) {
            return;
        }
        const token = readToken();

        if (!token) {
            return;
        }
        const base = `${location.protocol === "https:" ? "wss:" : "ws:"}//${location.host}`;
        let opened = false;
        socket = new WebSocket(`${base}/api/v1/presence/ws?token=${encodeURIComponent(token)}`);
        socket.onopen = () => {
            opened = true;
        };
        presenceSocket = socket;
        socket.onmessage = (event) => {
            try {
                const message = JSON.parse(event.data);

                if (message.type === "invite") {
                    onInvite({ ...message, id: `${Date.now()}-${Math.random()}` });
                }
            } catch {
                // Une trame illisible ne casse rien.
            }
        };
        // La présence ferme en 1008 un jeton refusé, et les connexions d'un compte qui a coupé ses sessions (chantier
        // 55) : la session s'examine, comme pour le centre de messages.
        socket.onclose = (event) => {
            if (closed) {
                return;
            }
            const wait = event.code === SESSION_ENDED || !opened ? examineSession() : Promise.resolve();
            wait.then(() => {
                if (!closed) {
                    timer = setTimeout(connect, 3000);
                }
            });
        };
    };
    connect();

    return () => {
        closed = true;
        clearTimeout(timer);
        socket?.close(1000);
    };
}
