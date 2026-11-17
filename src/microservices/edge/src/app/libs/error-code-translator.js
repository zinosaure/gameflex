import i18n from "../i18n.js";
import { explained } from "./api.js";

// Le refus dans la langue de la page, par son code (chantier 59, P5) : la même règle qu'`explained` (libs/api.js) —
// `kernelErrors.<code>`, puis `common.errorCodes.<code>` ; sans traduction, rien : l'anglais du serveur ne s'affiche pas.
export function getTranslatedErrorMessage(errorResponse) {
    return explained(errorResponse?.errorCode ?? "", errorResponse?.errorMessage || "");
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
