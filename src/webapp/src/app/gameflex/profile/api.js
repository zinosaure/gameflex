// Thin wrapper around the Python user-management backend's history/
// account-settings routes (python/src/app/profile/routes.py +
// users/routes.py's PATCH /api/v1/auth/me) — same envelope-normalizing
// approach as auth/AuthContext.jsx's own `api()`, reused here rather than
// imported since every call site also needs to attach the current token,
// which only a component with useAuth() in scope knows.
import { authApi, authHeaders } from "./auth/AuthContext.jsx";

const jsonHeaders = { "Content-Type": "application/json" };

// Delivers over both channels — live WebSocket if the recipient is
// connected, Web Push otherwise (see python/src/app/push/routes.py's
// send_invite, which decides that server-side; this call doesn't know or
// care which one actually reached them).
export function sendInvite(token, toUsername, platform, consoleId, reference, sessionId) {
    return authApi("/api/v1/invites", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({
            to_username: toUsername, platform, console_id: consoleId, reference, session_id: sessionId,
        }),
    });
}

export function updateMe(token, patch) {
    return authApi("/api/v1/auth/me", {
        method: "PATCH",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify(patch),
    });
}

// Retro-gaming keyboard/gamepad remap, per (user, console) — see
// python/src/app/profile/models.py's ControllerMapping. kind is "keyboard" or
// "gamepad". Server-side counterpart of gameflex/libs/keyboardMapping.js/
// gamepadMapping.js's localStorage cache (those stay the source of truth for
// instant, offline-friendly reads/writes — these two calls are what syncs
// that cache with the user's account, called from Streaming.jsx's
// KeyboardRemapTab/GamepadRemapTab). result.item.mapping is null when the
// user has never remapped anything for this console yet.
// Resolves usernames -> display_name, for any authenticated user (not
// admin-gated) — see python/src/app/users/routes.py's resolve_users doc.
// Go's session registries only ever store plain usernames, so this is how
// SessionPool.js turns those into an actual display name; usernames with no
// matching account are simply absent from result.item, not an error.
// Members for the invite panel — auth's GET /api/v1/users/members,
// every member sorted by most recent connection (last_login).
export function listMembers(token) {
    return authApi("/api/v1/users/members", { headers: authHeaders(token) });
}

export function resolveUsers(token, usernames) {
    return authApi("/api/v1/users/resolve", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({ usernames }),
    });
}

// Notification type toggles (Phase 4) — see python/src/app/profile/models.py's
// NotificationPreference. list always returns all known types (server fills
// in enabled=true defaults for any type with no saved row yet), so the
// frontend never needs its own "what are the 5 types" fallback list.
export function listNotificationPreferences(token) {
    return authApi("/api/v1/notification-preferences", { headers: authHeaders(token) });
}

export function saveNotificationPreference(token, notificationType, enabled) {
    return authApi("/api/v1/notification-preferences", {
        method: "PUT",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({ notification_type: notificationType, enabled }),
    });
}

// KeyMapping (Phase 8) — physical device (gamepad.id, or a client-generated
// keyboard UUID, see gameflex/libs/keyMapping.js) -> virtual controller
// (CONTROLLER_PRESETS key, e.g. "dualshock"). Replaces ControllerMapping
// above's per-console scope: one mapping covers every console/session that
// renders through the same virtual controller. See
// python/src/app/profile/models.py's KeyMapping doc.
export function getKeyMapping(token, physicalDeviceId, controllerName) {
    const params = new URLSearchParams({ physical_device_id: physicalDeviceId, controller_name: controllerName });
    return authApi(`/api/v1/key-mapping?${params.toString()}`, { headers: authHeaders(token) });
}

export function saveKeyMapping(token, physicalDeviceId, physicalDeviceType, controllerName, mapping) {
    return authApi("/api/v1/key-mapping", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({
            physical_device_id: physicalDeviceId,
            physical_device_type: physicalDeviceType,
            controller_name: controllerName,
            mapping,
        }),
    });
}

export function deleteKeyMapping(token, physicalDeviceId, controllerName) {
    const params = new URLSearchParams({ physical_device_id: physicalDeviceId, controller_name: controllerName });
    return authApi(`/api/v1/key-mapping?${params.toString()}`, {
        method: "DELETE",
        headers: authHeaders(token),
    });
}

// ButtonPositioning (Phase 9) — per-user, per-virtual-controller button layout,
// 5 slots (see python/src/app/profile/models.py's ButtonPositioning doc).
// controllerName omitted lists every controller's slots at once; passed,
// scopes to just that controller's up-to-5 slots (profile/ButtonPositioning.jsx
// always passes it).
export function listButtonPositioning(token, controllerName) {
    const params = controllerName ? `?${new URLSearchParams({ controller_name: controllerName }).toString()}` : "";
    return authApi(`/api/v1/key-positioning${params}`, { headers: authHeaders(token) });
}

export function saveButtonPositioning(token, controllerName, slotIndex, layout, isDefault, opacity) {
    return authApi("/api/v1/key-positioning", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({
            controller_name: controllerName, slot_index: slotIndex, layout, is_default: isDefault, opacity,
        }),
    });
}

export function deleteButtonPositioning(token, controllerName, slotIndex) {
    const params = new URLSearchParams({ controller_name: controllerName, slot_index: String(slotIndex) });
    return authApi(`/api/v1/key-positioning?${params.toString()}`, {
        method: "DELETE",
        headers: authHeaders(token),
    });
}

// ── Jetons de catalogue (chantier 6) ────────────────────────────────────
//
// LE LIEN entre deux serveurs : un jeton donne à un autre GameFlex le droit
// de LIRE le catalogue et de télécharger un jeu ou un BIOS. Il ne s'affiche
// qu'une fois, à la création : la base n'en garde que l'empreinte, donc rien
// ni personne ne peut le retrouver ensuite — pas même l'administrateur qui
// l'a créé. Perdu, il se révoque et se recrée. Jouer chez nous depuis là-bas
// passe par le jeton d'un COMPTE (Mon jeton, plus bas), jamais par lui.

export function listFederationTokens(token) {
    return authApi("/api/v1/catalog-tokens", { headers: authHeaders(token) });
}

// label — le nom de la personne ou de l'appareil à qui on ouvre. Obligatoire,
// et c'est le coeur du modèle : il permet de couper UN accès plutôt que tous,
// et il s'affiche à côté des jeux qu'il fait apparaître chez l'autre.
//
// scopes — ce que ce jeton ouvre. Vide veut dire « le catalogue », qui était
// la seule portée jusqu'au chantier 9 ; le serveur applique ce défaut, la
// page n'a donc rien à supposer.
export function createFederationToken(token, label, scopes = []) {
    return authApi("/api/v1/catalog-tokens", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({ label, scopes }),
    });
}

/**
 * Changer CE QU'UN JETON OUVRE, sans le réémettre.
 *
 * Le secret, lui, ne peut pas l'être — la base n'en a que l'empreinte. Les
 * deux sont dissociés exprès : sans ça, élargir un accès obligerait à
 * retourner coller un nouveau jeton sur chaque appareil qui l'avait, et cette
 * friction pousse à tout accorder d'avance « au cas où ».
 */
export function updateFederationTokenScopes(token, id, scopes) {
    return authApi(`/api/v1/catalog-tokens/${id}`, {
        method: "PATCH",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({ scopes }),
    });
}

export function revokeFederationToken(token, id) {
    return authApi(`/api/v1/catalog-tokens/${id}`, {
        method: "DELETE",
        headers: authHeaders(token),
    });
}

// ── Mon jeton (chantier 20) ──────────────────────────────────────────────
//
// Ce que Réglages › Profil montre : l'état de mon jeton, ses chiffres et mes
// consoles — le jeton lui-même seulement quand il est actif. C'est lui que je
// colle sur un autre serveur pour jouer ici depuis là-bas.
export function myAccountToken(token) {
    return authApi("/api/v1/auth/me/token", { headers: authHeaders(token) });
}

// ── Serveurs distants (chantier 6, l'autre sens) ─────────────────────────
//
// Deux gestes séparés, et c'est voulu : on SONDE, puis on ENREGISTRE.
//
// La sonde passe par le kernel — lui seul parle le protocole fédéré — et ne
// range rien. Elle sert à pouvoir dire « serveur trouvé : nas-du-salon, vous
// y êtes connu comme Marc » AVANT d'écrire quoi que ce soit. Un formulaire
// qui accepte en silence et rend une liste vide trois écrans plus loin est
// une bien pire expérience qu'un aller-retour de plus.

export function checkFederationServer(token, link, serverToken) {
    return authApi("/api/v1/servers/check", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({ link, token: serverToken }),
    });
}

export function listFederationServers(token) {
    return authApi("/api/v1/server-registry", { headers: authHeaders(token) });
}

// hostname/identity viennent de la sonde : on range ce qu'on a VU, pas ce
// qu'on suppose.
export function addFederationServer(token, { link, serverToken, hostname, identity }) {
    return authApi("/api/v1/server-registry", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({ link, token: serverToken, hostname, identity }),
    });
}

export function removeFederationServer(token, id) {
    return authApi(`/api/v1/server-registry/${id}`, {
        method: "DELETE",
        headers: authHeaders(token),
    });
}

// LES PARTAGES MONTÉS (WebDAV, kDrive, SMB) — une autre table, et pas par goût de
// la séparation : un serveur GameFlex a déjà classé ses jeux et rend un
// catalogue, un partage est un arbre de dossiers qu'on scanne. Ni les mêmes
// champs, ni la même façon d'échouer.
export function listRemoteMounts(token) {
    return authApi("/api/v1/mount-registry", { headers: authHeaders(token) });
}

// Le mot de passe entre et ne ressort jamais : c'est un identifiant de compte
// chez un tiers, pas un jeton qu'on peut vouloir relire pour le recopier
// ailleurs. Pour kDrive, `driveId` suffit — l'adresse se construit côté
// serveur, à partir du nombre qui suit /drive/ dans l'URL du kDrive. Pour SMB,
// `host`, `port` et `share` : l'adresse `smb://…` se compose côté serveur, sous
// une forme unique.
export function addRemoteMount(token, {
    name, kind, baseUrl, driveId, host, port, share, folder, username, password,
}) {
    return authApi("/api/v1/mount-registry", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({
            name, kind, base_url: baseUrl, drive_id: driveId, host, port, share,
            folder, username, password,
        }),
    });
}

/**
 * « Est-ce que ça marche ? » — pour un partage qu'on vient de saisir, ou pour
 * un déjà enregistré (par son `id`).
 *
 * La sonde vit dans le service d'identité parce que c'est LUI qui détient les
 * mots de passe : celui d'un montage enregistré ne redescend jamais jusqu'ici,
 * et n'a donc pas à remonter pour être vérifié.
 *
 * Rend `{reachable, folders, files}` ou `{reachable: false, detail}` — la
 * raison, pas un échec muet : « identifiants refusés » et « dossier
 * introuvable » ne se corrigent pas de la même façon.
 */
/**
 * LES JEUX d'une console dans une ressource, du CACHE — lus du seul fichier
 * concerné, parce que personne n'affiche vingt consoles à la fois.
 *
 * Le résumé (quand a-t-on parcouru, quelles consoles, un parcours tourne-t-il)
 * ne se demande plus : c'est le sujet `mount/<id>` du centre de messages,
 * poussé à chaque début et chaque fin de parcours.
 */
export function mountListing(token, id, { console = "" } = {}) {
    const params = new URLSearchParams({ section: "roms" });
    if (console) params.set("console", console);

    return authApi(`/api/v1/mounts/${id}/listing?${params}`, {
        headers: authHeaders(token),
    });
}

/**
 * Relire la ressource. Rend la main TOUT DE SUITE : un drive de dix mille
 * jeux demande un aller-retour par dossier et dépasse les deux minutes. L'état
 * se lit ensuite sur le listing.
 */
export function rescanRemoteMount(token, id) {
    return authApi(`/api/v1/mounts/${id}/rescan`, {
        method: "POST",
        headers: authHeaders(token),
    });
}

/** L'URL d'un élément — à récupérer avec le jeton, jamais par un lien nu. */
export function mountFileUrl(id, path) {
    return `/api/v1/mounts/${id}/file?path=${encodeURIComponent(path)}`;
}

export function checkRemoteMount(token, mount) {
    return authApi("/api/v1/mount-registry/check", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify(mount),
    });
}

export function removeRemoteMount(token, id) {
    return authApi(`/api/v1/mount-registry/${id}`, {
        method: "DELETE",
        headers: authHeaders(token),
    });
}
