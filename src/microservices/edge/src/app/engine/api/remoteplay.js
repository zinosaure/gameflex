import { apiUrl, streamAuthToken } from "../../libs/serverBase.js";
import { getStoredToken } from "../../libs/sharedHost.js";
import { federatedServerName } from "../../libs/federatedHost.js";
import { invalidResponse, serverUnreachable } from "../../libs/error-code-translator.js";
// Thin wrapper around remoteplay-bin — the PlayStation consoles and the PSN
// account, under /api/v1/playstation. The stream of a running game is
// another domain, /api/v1/remoteplay, served by the kernel (see
// libs/api-client.js).
// Envelope is {success, data, message} / {success:false, errorMessage} —
// different from the Python backend's {state, item, exception} used
// elsewhere in gameflex/ (see skeleton/examples/backendinteraction).

const PLAYSTATION = "/api/v1/playstation";

async function api(path, options = {}) {
    // L'HÔTE VISÉ, quand la console n'est pas d'ici (chantier 9, C9-P4).
    // Adresse et jeton voyagent ensemble, passés à l'appel — voir la note du
    // client de streaming, qui suit exactement la même règle.
    const { host, ...rest } = options;
    let res;
    // Le jeton, attaché ICI aussi.
    //
    // Ce module et `libs/api-client.js` appellent le
    // MÊME service ; un chantier de sécurité précédent avait ajouté
    // l'en-tête dans l'autre, et celui-ci est resté en clair. Personne ne
    // s'en est aperçu pour une raison qui vaut d'être retenue : le service
    // n'exigeait rien, donc l'oubli ne se voyait pas — c'est en fermant le
    // service (2026-08-27) que la page a cessé de charger ses appareils.
    //
    // getStoredToken plutôt qu'un paramètre : ce module n'est pas un
    // composant et n'a pas de contexte. Il rend null sur un appareil sans
    // compte, où ces appels DOIVENT échouer (voir chantier 6, C6-P11).
    // Le jeton de l'hôte visé quand la page en vise un (C9-P4c), le nôtre
    // sinon. Même ordre que dans le client de streaming, et pour la même
    // raison : deux façons de choisir finiraient par ne plus dire pareil.
    const token = host ? host.token : (streamAuthToken() || getStoredToken());
    try {
        res = await fetch(host?.origin ? `${host.origin}${path}` : apiUrl(path), {
            headers: {
                "Content-Type": "application/json",
                ...(token ? { Authorization: `Bearer ${token}` } : {}),
                ...(rest.headers || {}),
            },
            ...rest,
        });
    } catch (err) {
        return serverUnreachable(host?.origin ? federatedServerName(host.origin) : "");
    }
    const json = await res.json().catch(() => invalidResponse());
    return { ...json, ok: res.ok && json.success !== false };
}

export function registerDevice({ name, ip, identifier, pin, hostType = "PS5" }) {
    return api(`${PLAYSTATION}/bind`, {
        method: "POST",
        body: JSON.stringify({
            deviceName: name,
            hostIp: ip,
            accountId: identifier,
            pin,
            hostType,
        }),
    });
}

export function deleteDevice(deviceId) {
    return api(`${PLAYSTATION}/devices/${encodeURIComponent(deviceId)}`, { method: "DELETE" });
}
