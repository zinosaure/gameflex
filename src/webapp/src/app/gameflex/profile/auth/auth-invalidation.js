const AUTH_INVALIDATED_EVENT = "auth:invalidated";

let invalidationInProgress = false;

export function isAuthErrorMessage(message) {
    if (!message) return false;
    const normalizedMessage = message.toLowerCase();
    return (
        normalizedMessage.includes("not authenticated") ||
        normalizedMessage.includes("unauthorized") ||
        normalizedMessage.includes("401") ||
        normalizedMessage.includes("forbidden") ||
        normalizedMessage.includes("token") ||
        normalizedMessage.includes("auth")
    );
}

/**
 * NOTRE JETON N'EST PLUS RECONNU — on le dit, au lieu de continuer.
 *
 * Ce module existait sans un seul appelant (constaté le 2026-09-08). Un jeton
 * expiré, ou valable sur une autre origine, restait donc en place : l'écran
 * continuait de s'afficher — le kernel du téléphone sert la plupart des
 * lectures sans identité — pendant que chaque appel authentifié échouait en
 * silence. On voyait ses jeux et on ne pouvait rien lancer, sans qu'aucun
 * message ne dise pourquoi.
 *
 * L'événement est ce qui permet à AuthContext de refermer la session sans que
 * ce module ait à le connaître : il est appelé depuis la couche réseau, qui
 * n'a rien à savoir de React.
 */
export function invalidateAuth(reason) {
    if (typeof window === "undefined") return;
    if (invalidationInProgress) return;
    invalidationInProgress = true;
    // Le jeton se range sous `retrogaming-users-token` : AuthContext l'efface
    // en recevant l'événement. (Ce module effaçait aussi `auth_token` et
    // `user_data`, deux clés d'un gabarit que rien n'a jamais écrites —
    // retirées le 2026-10-15.)
    console.warn("Auth invalidated:", reason);
    window.dispatchEvent(new CustomEvent(AUTH_INVALIDATED_EVENT, { detail: { reason } }));
    invalidationInProgress = false;
}

export { AUTH_INVALIDATED_EVENT };
