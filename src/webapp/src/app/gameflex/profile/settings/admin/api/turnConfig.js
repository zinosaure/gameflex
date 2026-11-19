// Thin wrapper around the Go backend's admin TURN config routes
// (golang/src/internal/api/streaming_stubs.go's ReadTurnConfig/
// SaveTurnConfig, exposed under /api/v1/turn-config, see
// cmd/server/main.go). Same {success, data} envelope and admin-only-
// gated-client-side-only reasoning as gameflex/admin/api/storage.js.
//
// Note neither call handles the decryption passphrase: it's attached to
// every /api/v1/turn-config request by the webpack proxy, server-side (see
// webpack.config.js), specifically so it never exists in browser-reachable
// code. There is nothing to pass here.

import { authHeaders, getStoredToken } from "../../../auth/AuthContext.jsx";
import { invalidResponse, serverUnreachable } from "../../../../shared/utils/error-code-translator.js";

const BASE = "/api/v1/turn-config";

async function api(path, options = {}) {
    let res;
    try {
        // Le jeton du membre : ces routes sont sous la garde de remote play
        // depuis le 2026-09-16 (D3), comme /api/v1/playstation.
        res = await fetch(path, {
            ...options,
            headers: { ...authHeaders(getStoredToken()), ...(options.headers || {}) },
        });
    } catch (err) {
        return serverUnreachable();
    }
    const json = await res.json().catch(() => invalidResponse());
    return { ...json, ok: res.ok && json.success !== false };
}

// A POST, despite being a read: the passphrase travels in a header, and a
// GET would put it in proxy/access logs and browser caches. Returns
// {active: {category, genericType}, generic: [coturn, eturnal],
// cloudflare: {...}} with secrets already decrypted.
export function readTurnConfig() {
    return api(`${BASE}/read`, { method: "POST" });
}

export function saveTurnConfig(payload) {
    return api(BASE, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(payload),
    });
}
