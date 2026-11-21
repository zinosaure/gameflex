import { shareOrigin } from "./serverBase.js";
// Builds the deep link used to join/watch a session from anywhere OTHER
// than its own console page — NotificationBell's invite list (index.js)
// needs this, factored out here rather than inlined so any other future
// caller doesn't have to duplicate the URL shape.
//
// One single URL for both joining and watching now — no ?role= (removed:
// a client-supplied role was never anything but a UX hint, and trusting it
// for anything was the exact hole retro_relay.go's own identity check just
// closed) and no separate /watch route (removed: Streaming.jsx decides
// owner/player/spectate entirely server-side, from session.me.role). Sharing
// a session with someone is now genuinely just "send them this one id".
export function buildStreamingUrl({ sessionId, play = false }) {
    // UNE PARTIE DU TÉLÉPHONE SE DÉSIGNE PAR UN IDENTIFIANT, comme les
    // autres (chantier 8, C8-P3).
    //
    // Il y avait ici une branche qui fabriquait `?device=1&path=…&console=…`,
    // du temps où la partie du téléphone n'était enregistrée nulle part et où
    // la page la reconstruisait depuis l'adresse.
    //
    // Elle a un vrai identifiant depuis C8-P3, et le kernel la publie sur la
    // façade. Le lecteur de ces paramètres (`deviceSessionFrom`) a été
    // supprimé avec la branche qui les lisait — mais CELLE-CI, qui les
    // écrivait, est restée. Le lien de reprise partait donc sans le moindre
    // `sessionId` : la page n'avait rien à résoudre et tournait sur
    // « Connecting… » indéfiniment (signalé le 2026-09-04).
    //
    // Un écrivain sans lecteur ne se voit pas à la compilation. Quand on
    // supprime l'un des deux, chercher l'autre.
    const params = new URLSearchParams({ sessionId });

    // play=1 — "I came here to play", set ONLY by the deliberate actions:
    // launching a game, joining from the Dashboard, taking a session back.
    // A shared link never carries it, so it still lands on watch.
    //
    // This is NOT the old ?role=. That one ASSERTED a role the server took at
    // face value. This one only asks the page to attempt an intent, and the
    // intent still goes through the POST and the child's own checks: forging
    // it gets you exactly what clicking the button would, and nothing more.
    if (play) {
        params.set("play", "1");
    }

    return `/streaming?${params.toString()}`;
}

// Same link, absolute — for anything that leaves the app (clipboard, OS share
// sheet, a message to a friend). Separate from buildStreamingUrl because a
// router <Link> must stay relative; only what travels outside needs the origin.
export function buildStreamingShareUrl({ sessionId }) {
    return `${shareOrigin()}${buildStreamingUrl({ sessionId })}`;
}

/**
 * Le lien d'une partie qui tourne SUR CET APPAREIL.
 *
 * Rien à voir avec un identifiant de session : une partie du téléphone n'est
 * enregistrée nulle part, et son « id » n'est que le chemin du fichier. Le
 * lien copié contenait donc `sessionId=device:appareil/psp/[PSP] Crisis
 * Core….iso`, que personne ne pouvait ouvrir — l'invité tombait sur un écran
 * noir et un spinner (signalé le 2026-09-01).
 *
 * Ce qu'il faut envoyer est une INVITATION : l'adresse de l'appareil, que le
 * kernel de celui-ci reconnaît (`guest=1&host=…`, voir remoteSessionFrom).
 * Le jeu tourne là-bas et y reste ; qui ouvre le lien devient spectateur ou
 * joueur, sans rien lancer.
 *
 * Vide quand l'appareil n'est joignable par personne — pas de réseau, donc
 * pas d'invitation possible. L'appelant doit le dire plutôt que de fabriquer
 * un lien mort.
 */
export function buildDeviceShareUrl() {
    const origin = shareOrigin();
    if (!origin) {
        return "";
    }

    return `${origin}/streaming?guest=1&host=${encodeURIComponent(origin)}`;
}

/** Une partie du téléphone, qui se partage autrement qu'une session du serveur. */
export function isDeviceSession(session) {
    return session?.platform === "device" && String(session?.id || "").startsWith("device:");
}

