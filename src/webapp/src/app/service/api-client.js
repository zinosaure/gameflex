import { invalidateAuth, isAuthErrorMessage } from "@/gameflex/profile/auth/auth-invalidation";
import { getTranslatedErrorMessage } from "@/gameflex/shared/utils/error-code-translator";
import { getStoredToken } from "@/gameflex/profile/auth/AuthContext.jsx";
import { apiUrl, streamAuthToken, streamHostOrigin } from "@/gameflex/libs/serverBase.js";

// API base configuration
// Remote play parle à DEUX domaines, et chaque chemin passé à apiRequest
// porte le sien : `/playstation/…` (les consoles et le compte PSN —
// remoteplay-bin) et `/remoteplay/…` (le flux d'une partie — le kernel, qui
// le relaie au conteneur).
// No `process` global is polyfilled in this webpack build (unlike the
// source Vite app), so `process.env.*` would throw at module-eval time —
// always use the fixed default.
export const API_BASE_URL = "/api/v1";

// API response types
// Backend format:
// Success: { success: true, data: object, message: "" }
// Failure: { success: false, errorMessage: "", errorCode?: number }

/**
 * Generic API request function
 * @param endpoint API endpoint path
 * @param options request options
 * @returns Promise<ApiResponse<T>>
 */
export async function apiRequest(endpoint, options = {}) {
    // L'HÔTE VISÉ, quand ce n'est pas le nôtre (chantier 9, C9-P4).
    //
    // Une console fédérée vit sur la machine de quelqu'un d'autre : l'adresse
    // ET le jeton changent ensemble, et ils voyagent donc ensemble, passés à
    // l'appel plutôt que posés à côté. C'est ce qui distingue ce chemin de la
    // première tentative du chantier, qui posait une cible globale que TOUS
    // les appels lisaient — un onglet en contaminait un autre, et une session
    // d'ici partait chercher son état chez le voisin.
    //
    // `host` absent — le cas normal — ne change rien : même URL relative,
    // même jeton ambiant qu'avant.
    const { host, ...rest } = options;
    const url = host?.origin
        ? `${host.origin}${API_BASE_URL}${endpoint}`
        : apiUrl(`${API_BASE_URL}${endpoint}`);

    // /api/v1/remoteplay/* now requires a valid JWT (kernel's remoteplayRouter,
    // previously the one API surface with no auth check at all) — this was
    // never sent before, since every call here went through fetch() with no
    // Authorization header. Every remote-play call goes through this one
    // function, so attaching it here covers all of them at once instead of
    // threading a token through each of viewer.service.js/streaming.service.js's
    // ~20 call sites.
    // Le jeton de CE serveur-là, jamais le nôtre : le sien ne veut rien dire
    // chez lui, et l'y envoyer le ferait fuiter pour rien.
    //
    // Trois sources, de la plus explicite à la plus ambiante : l'hôte passé à
    // l'appel, puis celui que la page de streaming vise pour sa durée de vie
    // (C9-P4c), puis le nôtre. L'ordre compte — le plus précis gagne, et
    // `getStoredToken` reste le repli de tout le reste de l'application.
    const token = host ? host.token : (streamAuthToken() || getStoredToken());
    // UN REFUS VENU D'UN AUTRE HÔTE NE DIT RIEN DE NOTRE SESSION — qu'on l'ait
    // visé à l'appel (`host`) ou que la page de jeu le vise (`setStreamBase`).
    // Seul le second cas manquait au garde : un 401 de l'hôte fédéré nous
    // déconnectait d'ici (2026-10-10).
    const foreign = Boolean(host) || Boolean(streamHostOrigin());
    // `headers` must be computed AFTER `...options`, not before — `options`
    // (POST calls especially: webrtc/offer, viewer/offer, ...) almost always
    // carries its own `headers` key, and spreading `...options` after a
    // `headers:` entry at the same object level REPLACES that entry
    // wholesale rather than merging into it. The previous order silently
    // discarded the Authorization header on every call that passed its own
    // headers — confirmed live (2026-08-22): GET calls with no explicit
    // headers (my-devices, health) kept working, every POST (the actual
    // WebRTC negotiation calls) lost the token.
    const defaultOptions = {
        ...rest,
        headers: {
            "Content-Type": "application/json",
            ...(token ? { Authorization: `Bearer ${token}` } : {}),
            ...options.headers,
        },
    };

    try {
        const response = await fetch(url, defaultOptions);
        // 401 SEUL : « qui es-tu ? ». Un 403 dit « pas le droit » — une route
        // d'administrateur, la session de quelqu'un d'autre (« not your
        // session ») — et ne dit rien de la validité du jeton. Le traiter en
        // session expirée déconnectait quelqu'un pour un geste refusé
        // (relevé le 2026-10-09).
        const unauthorizedStatus = response.status === 401;
        // L'ERREUR PORTE SON STATUT : « ce serveur nous connaît-il ? » se lit
        // dans `error.status`, jamais en fouillant le texte du message.
        const failure = (message) => Object.assign(new Error(message), { status: response.status });

        // Skip OPTIONS preflight requests (204 status)
        if (response.status === 204 && defaultOptions.method === "OPTIONS") {
            // This is a CORS preflight request; the browser handles it automatically, no action needed here
            return {
                success: true,
                message: "Preflight request successful",
            };
        }

        // Check if response has content
        const contentType = response.headers.get("content-type");

        // If no content type or empty response
        if (!contentType || response.status === 204) {
            if (response.ok) {
                return {
                    success: true,
                    message: "Request successful",
                };
            } else {
                throw failure(`Request failed: ${response.status} ${response.statusText}`);
            }
        }

        // Parse JSON response. Le corps se lit HORS du try de la lecture JSON :
        // le « Session expired » levé ici pour un refus au corps vide était
        // rattrapé par ce même catch et réécrit en « Invalid response format ».
        const text = await response.text();
        let data;
        try {
            data = text && text.trim().length > 0 ? JSON.parse(text) : undefined;
        } catch (parseError) {
            // Un refus qui n'est pas du JSON — un proxy, un service qui répond
            // en texte — reste un refus : il est traité juste en dessous.
            if (!unauthorizedStatus) {
                console.error("JSON parse failed:", parseError);
                throw failure("Invalid response format, cannot parse JSON data");
            }
        }
        if (data === undefined) {
            if (unauthorizedStatus) {
                // PAS NOTRE SESSION QUI EXPIRE quand c'est un AUTRE serveur qui
                // refuse. Sans cette garde, un mot de passe périmé chez un ami
                // déconnectait de sa propre machine (C9-P4).
                if (!foreign) {
                    invalidateAuth(`Endpoint ${endpoint} returned unauthorized (no JSON body)`);
                }
                throw failure("Session expired, please reload");
            }
            throw failure("Empty response");
        }

        // Handle new response format
        if (!response.ok) {
            // On failure, prefer error code translation, otherwise use errorMessage field
            const errorResponse = data;
            const errorMsg = getTranslatedErrorMessage({
                errorCode: errorResponse?.errorCode,
                errorMessage:
                    errorResponse?.errorMessage ||
                    errorResponse?.message ||
                    `Request failed: ${response.status}`,
            });

            if (unauthorizedStatus) {
                // PAS NOTRE SESSION QUI EXPIRE quand c'est un AUTRE serveur qui
                // refuse. Sans cette garde, un mot de passe périmé chez un ami
                // déconnectait de sa propre machine (C9-P4).
                if (!foreign) {
                    invalidateAuth(errorMsg || `Endpoint ${endpoint} returned unauthorized`);
                }
            }

            throw failure(errorMsg);
        }

        // Unified handling of new response format
        // Success: { success: true, data: T, message?: string }
        // Failure: { success: false, errorMessage: string }
        if (data && typeof data === "object") {
            const responseData = data;

            // If response contains data field, map to result field for backward compatibility
            if ("data" in responseData) {
                return {
                    success: responseData.success ?? true,
                    data: responseData.data,
                    message: responseData.message,
                    errorMessage: responseData.errorMessage,
                    result: responseData.data, // backward compatibility
                };
            }

            // If response contains errorMessage, it is a failure response
            if ("errorMessage" in responseData && !responseData.success) {
                const translatedError = getTranslatedErrorMessage({
                    errorCode: responseData.errorCode,
                    errorMessage: responseData.errorMessage,
                });

                if (isAuthErrorMessage(translatedError)) {
                    // PAS NOTRE SESSION QUI EXPIRE quand c'est un AUTRE serveur qui
                    // refuse. Sans cette garde, un mot de passe périmé chez un ami
                    // déconnectait de sa propre machine (C9-P4).
                    if (!foreign) {
                        invalidateAuth(translatedError || `Endpoint ${endpoint} returned auth failure`);
                    }
                }
                return {
                    success: false,
                    errorMessage: translatedError,
                    errorCode: responseData.errorCode,
                    result: undefined,
                };
            }

            // If result field already exists (old format compatibility), keep as-is
            if ("result" in responseData) {
                return {
                    ...responseData,
                    data: responseData.result, // also provide data field
                };
            }
        }

        return data;
    } catch (error) {
        console.error("API request error:", error);
        if (error instanceof Error) {
            throw error;
        }
        throw new Error("Network error, please try again later");
    }
}
