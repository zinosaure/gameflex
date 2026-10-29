import i18n from "@/i18n/index.js";

/**
 * Get the error message for a given error code in the current language
 * @param errorCode error code (number or string)
 * @param fallbackMessage default message to return if no translation is found
 * @returns translated error message
 */
function translateErrorCode(errorCode, fallbackMessage) {
    // No error code — return fallback or empty string
    if (errorCode === null || errorCode === undefined) {
        return fallbackMessage || "";
    }

    const codeStr = String(errorCode);

    // Look up translation by error code
    const translationKey = `common.errorCodes.${codeStr}`;
    const translated = i18n.t(translationKey);

    // If the result equals the key itself, no translation was found
    if (translated === translationKey) {
        return fallbackMessage || `Error ${codeStr}`;
    }

    return translated;
}

/**
 * Extract and translate the error message from an API error response
 * @param errorResponse API error response object
 * @returns translated error message
 */
export function getTranslatedErrorMessage(errorResponse) {
    // Prefer error code translation when available
    if (errorResponse.errorCode !== null && errorResponse.errorCode !== undefined) {
        const translated = translateErrorCode(errorResponse.errorCode, errorResponse.errorMessage);
        return translated;
    }

    // No error code — return the raw error message
    return errorResponse.errorMessage || "";
}

// UN ÉCHEC DIT CE QUI EST ARRIVÉ, ET CHEZ QUI (le porteur, 2026-10-21 :
// « enlever les messages d'erreur génériques comme "backend injoignable" »).
//
// Les clients d'API rendaient « Backend injoignable » en dur, en français,
// quelle que soit la cause — un serveur arrêté, l'adresse d'un ami muette, ou
// un jeton refusé, ce qui était le cas du lancement chez « Gameflex Private »
// depuis le S25. Chacun dit désormais laquelle, d'un seul endroit. `server` :
// le nom d'un serveur fédéré ; vide, c'est le nôtre.
function failure(errorCode, errorMessage) {
    return { success: false, ok: false, status: 0, errorCode, errorMessage };
}

/** Pas de réponse du tout : le serveur est arrêté, ou l'adresse est muette. */
export function serverUnreachable(server = "") {
    return failure("unreachable", server
        ? i18n.t("common.errorCodes.peerUnreachable", { server })
        : i18n.t("common.errorCodes.serverUnreachable"));
}

/** Le serveur répond, et refuse le jeton : la connexion est à refaire. */
export function sessionRefused(server = "") {
    return failure("sessionRefused", server
        ? i18n.t("common.errorCodes.peerSessionRefused", { server })
        : i18n.t("common.errorCodes.sessionRefused"));
}

/** Le serveur répond, accepte le jeton, mais le canal de commandes ne passe pas. */
export function channelRefused(server = "") {
    return failure("channelRefused", server
        ? i18n.t("common.errorCodes.peerChannelRefused", { server })
        : i18n.t("common.errorCodes.channelRefused"));
}

/** Le canal s'est coupé pendant une commande : elle a pu aboutir quand même. */
export function connectionLost() {
    return failure("connectionLost", i18n.t("common.errorCodes.connectionLost"));
}

/** Une réponse est arrivée, mais elle ne se lit pas. */
export function invalidResponse() {
    return failure("invalidResponse", i18n.t("common.errorCodes.invalidResponse"));
}
