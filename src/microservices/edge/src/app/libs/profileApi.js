// Thin wrapper around the Python user-management backend's history/
// account-settings routes (python/src/app/profile/routes.py +
// users/routes.py's PATCH /api/v1/auth/me) — same envelope-normalizing
// approach as auth/AuthContext.jsx's own `api()`, reused here rather than
// imported since every call site also needs to attach the current token,
// which only a component with useAuth() in scope knows.
import { authApi, authHeaders } from "./sharedHost.js";

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

// Retro-gaming keyboard/gamepad remap, per (user, console) — see
// python/src/app/profile/models.py's ControllerMapping. kind is "keyboard" or
// "gamepad". Server-side counterpart of engine/libs/keyboardMapping.js/
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

// KeyMapping (Phase 8) — physical device (gamepad.id, or a client-generated
// keyboard UUID, see engine/libs/keyMapping.js) -> virtual controller
// (CONTROLLER_PRESETS key, e.g. "dualshock"). Replaces ControllerMapping
// above's per-console scope: one mapping covers every console/session that
// renders through the same virtual controller. See
// python/src/app/profile/models.py's KeyMapping doc.
export function getKeyMapping(token, physicalDeviceId, controllerName) {
    const params = new URLSearchParams({ physical_device_id: physicalDeviceId, controller_name: controllerName });
    return authApi(`/api/v1/key-mapping?${params.toString()}`, { headers: authHeaders(token) });
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

// Écrire un emplacement — l'opacité réglée depuis le menu d'une partie (engine/stream/onscreen/padOpacity.js) : au même
// serveur, avec le même jeton, que la lecture ci-dessus.
export function saveButtonPositioning(token, controllerName, slotIndex, layout, isDefault, opacity) {
    return authApi("/api/v1/key-positioning", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({ controller_name: controllerName, slot_index: slotIndex, layout, is_default: isDefault, opacity }),
    });
}

