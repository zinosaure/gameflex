import { capabilities } from "./capabilities.js";
// D'OÙ VIENT LE SERVEUR GAMEFLEX — et pourquoi ce n'est pas toujours « ici ».
//
// La page ET l'API viennent de la même origine : dans un navigateur, et dans l'application Android, qui charge la page
// servie par son propre kernel (chantier 7, MainActivity). Un chemin relatif suffit. L'adresse que l'application
// déclarait autrefois (`window.GAMEFLEX_SERVER`) n'est plus posée par personne : sa lecture est partie avec le
// chantier 85.
//
// SEULE EXCEPTION : la page de jeu d'une partie chez un AUTRE serveur (un serveur fédéré, C9) — tant qu'elle la montre,
// l'API, la passerelle et le canal du kernel visent l'hôte (`setStreamBase`), puis la surcharge part avec la page. Ce qui
// vit plus longtemps qu'une page ne la suit pas : le chemin relatif, chez nous (le défaut du 2026-09-07 — un abonnement
// reconnecté pendant une page fédérée y restait rebranché, avec notre jeton, refusé là-bas).

/** L'origine du serveur visé : celle de l'hôte d'une partie fédérée pendant sa page, sinon "" (la nôtre). */
export function serverOrigin() {
    return streamBase || "";
}

let streamBase = null;
let streamToken = null;
let streamUsername = null;

/**
 * Vise un autre serveur le temps d'une page (session d'un hôte fédéré).
 *
 * `token` est celui de CE serveur-là, quand on y a un compte (chantier 9,
 * C9-P3). Il vient avec l'adresse et repart avec elle, parce que les deux
 * n'ont de sens qu'ensemble : le nôtre ne veut rien dire chez lui, et le sien
 * ne veut rien dire ici.
 *
 * Omis — le cas d'un invité (C7-P4), qui n'a de compte nulle part — la page
 * garde le jeton qu'elle avait déjà : celui que l'hôte lui a frappé.
 */
export function setStreamBase(origin, token = "", username = "") {
    streamBase = origin ? String(origin).replace(/\/+$/, "") : null;
    streamToken = token || null;
    streamUsername = username || null;
}

/** Retire la surcharge — à appeler quand la page qui l'a posée se démonte. */
export function clearStreamBase() {
    streamBase = null;
    streamToken = null;
    streamUsername = null;
}

/**
 * L'hôte que cette page vise, ou null quand elle parle au nôtre.
 *
 * Distinct de `serverOrigin()`, qui rend aussi l'adresse déclarée par
 * l'application : celle-ci répond « est-ce que je parle à quelqu'un
 * d'AUTRE ? », et c'est une question différente.
 */
export function streamHostOrigin() {
    return streamBase;
}

/**
 * Le jeton à présenter à l'hôte visé, ou null quand on parle au nôtre.
 *
 * Lu par les clients d'API qui ne sont pas des composants et n'ont donc pas
 * de contexte — le même besoin qui a fait exister `getStoredToken`, et la
 * même règle : un seul endroit décide, sinon les chemins les plus discrets
 * finissent par porter le mauvais jeton.
 */
export function streamAuthToken() {
    return streamToken;
}

/**
 * Le nom sous lequel on est connu CHEZ L'HÔTE VISÉ.
 *
 * Il voyage avec l'adresse et le jeton pour la même raison : c'est lui qui
 * prendra une place dans la file de joueurs de l'hôte. Y présenter notre nom
 * local ferait entrer quelqu'un qui n'existe pas chez lui — et le décodage de
 * NOTRE jeton, qui sert partout ailleurs, donne précisément ce mauvais nom.
 */
export function streamHostUsername() {
    return streamUsername;
}

/** Une URL d'API : relative, ou chez l'hôte d'une partie fédérée pendant sa page. */
export function apiUrl(path) {
    return `${serverOrigin()}${path}`;
}

/**
 * La même, mais ABSOLUE — pour une adresse qui quitte la page.
 *
 * L'application Android télécharge avec `java.net.URL`, qui exige un
 * protocole : un chemin relatif y échoue par « no protocol », et le
 * téléchargement d'un jeu distant s'arrêtait là (signalé le 2026-09-02).
 *
 * Même raison que `shareOrigin` un peu plus bas, autre destinataire : là
 * c'est quelqu'un d'autre qui ouvrira le lien, ici c'est l'application qui
 * l'ouvrira — et ni l'un ni l'autre ne sait quoi faire d'un chemin sans
 * origine.
 */
export function absoluteApiUrl(path) {
    const declared = serverOrigin();

    return declared ? `${declared}${path}` : `${window.location.origin}${path}`;
}

/**
 * Une URL de WebSocket. Le protocole suit celui du SERVEUR visé, pas celui de
 * la page : une page servie en https par l'application doit tout de même
 * parler ws:// à un serveur en clair sur le réseau local.
 */
export function wsUrl(path) {
    const origin = serverOrigin();
    if (!origin) {
        const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";

        return `${protocol}//${window.location.host}${path}`;
    }

    return `${origin.replace(/^http/, "ws")}${path}`;
}

/**
 * L'origine à mettre dans un lien qu'on PARTAGE.
 *
 * Jamais l'origine locale de l'application : sur un téléphone, le PWA est
 * servi sur 127.0.0.1, et chez celui qui reçoit le lien, 127.0.0.1 désigne
 * SON appareil. Le lien mènerait donc chez lui, ou nulle part.
 *
 * L'hôte déclare l'adresse à laquelle on peut le joindre (`shareOrigin` dans
 * les capacités) — pour un téléphone, son adresse sur le réseau local. Vide
 * quand il n'est sur aucun réseau : il n'y a alors pas d'invitation possible,
 * et l'appelant doit le dire plutôt que de fabriquer un lien mort.
 */
export function shareOrigin() {
    const declared = capabilities().shareOrigin;
    if (declared) {
        return declared;
    }

    return serverOrigin() || window.location.origin;
}

/** Peut-on inviter quelqu'un ? Faux quand l'hôte n'est joignable par personne. */
export function canInvite() {
    return shareOrigin() !== "";
}
