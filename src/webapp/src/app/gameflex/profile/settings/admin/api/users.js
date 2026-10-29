// Thin wrapper around the admin-only /api/v1/users routes
// (python/src/app/users/routes.py's init_admin_users) — same
// envelope-normalizing approach as auth/AuthContext.jsx's own api().
import { authApi, authHeaders } from "../../../auth/AuthContext.jsx";

const jsonHeaders = { "Content-Type": "application/json" };

export function listUsers(token, { page = 1, itemsPerPage = 10, search = "" } = {}) {
    const params = new URLSearchParams({ page, items_per_page: itemsPerPage });
    if (search) params.set("search", search);
    return authApi(`/api/v1/users?${params.toString()}`, { headers: authHeaders(token) });
}

export function createUser(token, data) {
    return authApi("/api/v1/users", {
        method: "POST",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify(data),
    });
}

export function updateUser(token, id, patch) {
    return authApi(`/api/v1/users/${id}`, {
        method: "PATCH",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify(patch),
    });
}

export function deleteUser(token, id) {
    return authApi(`/api/v1/users/${id}`, {
        method: "DELETE",
        headers: authHeaders(token),
    });
}

// LE JETON D'UN COMPTE (chantier 20) — créé avec lui, désactivé. L'activer
// demande une durée (`1d`, `1w`, `1m`, `3m`, `6m`, `1y`, `2y`) ; le désactiver
// efface l'échéance. La réponse est le jeton à jour.
export function updateUserToken(token, id, { enabled, duration = "" }) {
    return authApi(`/api/v1/users/${id}/token`, {
        method: "PATCH",
        headers: { ...jsonHeaders, ...authHeaders(token) },
        body: JSON.stringify({ enabled, duration }),
    });
}

// Régénérer : l'ancien jeton cesse de valoir à l'instant, et les serveurs qui
// l'avaient enregistré doivent coller le nouveau.
export function regenerateUserToken(token, id) {
    return authApi(`/api/v1/users/${id}/token/regenerate`, {
        method: "POST",
        headers: authHeaders(token),
    });
}
