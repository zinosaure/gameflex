// Per-user, per-platform streaming quality preference (resolution/frameRate/
// bitrate), kept in this browser's localStorage rather than only on the
// server. The Go backend has a single settings row per *device*
// (golang/src/internal/api/playstation_handlers.go, resolveDeviceStreamSettings
// — read by ConnectWebRTC's launch_spec, no per-session override exists), so
// several people sharing the same console would otherwise keep overwriting
// each other's preference. Keying by hostType (e.g. "PS4"/"PS5") instead of a
// specific device id means the preference follows the user across any
// device of that type, and each browser/user keeps their own — matching the
// intent of "chacun garde son settings, par rapport à la plateforme".
const STORAGE_PREFIX = "remoteplay-streaming-settings:";

// UNE PRÉFÉRENCE PEUT SURVIVRE À L'OPTION QU'ELLE DÉSIGNE.
//
// Le 2026-09-20, « 480p » a été retiré de l'échelle (voir `StreamSettings.jsx`)
// parce que la PS5 refuse de l'ouvrir. Les navigateurs, eux, gardaient la
// préférence : elle serait revenue dans le formulaire, aurait affiché une
// valeur absente de la liste, et un simple « Enregistrer » l'aurait réécrite
// côté serveur. Une préférence périmée est donc OUBLIÉE — l'appelant retombe
// alors sur son propre défaut, ce qu'il sait déjà faire (`|| { … }`). Même
// chose pour « 360p », retiré le 2026-10-26 (le porteur : « de nos jours 540p
// est assez bien partout »).
//
// Cette liste doit suivre celle de `STREAM_OPTIONS`. Elle est recopiée plutôt
// qu'importée pour qu'un module de stockage ne dépende pas d'un composant React
// — le prix est cette ligne à tenir à jour, et ce commentaire le dit.
const KNOWN_RESOLUTIONS = ["540p", "720p", "1080p"];

export function loadStreamingSettings(hostType) {
    try {
        const raw = localStorage.getItem(STORAGE_PREFIX + hostType);
        const stored = raw ? JSON.parse(raw) : null;
        if (stored && stored.resolution && !KNOWN_RESOLUTIONS.includes(stored.resolution)) {
            return null;
        }
        return stored;
    } catch {
        return null;
    }
}

export function saveStreamingSettings(hostType, values) {
    try {
        localStorage.setItem(STORAGE_PREFIX + hostType, JSON.stringify(values));
    } catch {
        // localStorage unavailable (private browsing, quota, disabled) —
        // non-fatal, the server-side save below is what actually matters
        // for the upcoming session anyway.
    }
}
