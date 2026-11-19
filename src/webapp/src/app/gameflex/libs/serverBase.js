import { capabilities } from "./capabilities.js";
// D'où vient le serveur GameFlex — et pourquoi ce n'est pas toujours « ici ».
//
// Dans un navigateur, la page ET l'API viennent de la même origine : un chemin
// relatif suffit, et c'est ce que tout le frontend fait depuis le début.
//
// Dans l'application Android, la page est servie DEPUIS L'APPLICATION (des
// assets embarqués, sur une origine locale) tandis que le serveur est ailleurs,
// sur le réseau. « /api/v1/sessions » y désignerait l'application elle-même,
// qui n'a pas d'API. Il faut donc une origine explicite — mais UNIQUEMENT là.
//
// D'où la règle : par défaut, rien ne change. `serverOrigin()` rend une chaîne
// vide, les URL restent relatives, le comportement du web est intact. L'hôte
// n'est utilisé que si quelqu'un l'a posé, et personne ne le fait dans un
// navigateur.
//
// Pourquoi une variable globale plutôt qu'une variable de build : la même APK
// doit pouvoir viser un autre serveur sans être recompilée, et c'est le pont
// natif qui la renseigne au chargement de la page.

/**
 * L'origine du serveur : "" en usage normal (même origine), ou
 * "http://192.168.1.20:10200" quand l'application l'a déclarée.
 */
export function serverOrigin() {
    if (typeof window === "undefined") {
        return "";
    }

    // Une session chez un AUTRE appareil (C7-P4) : tant que la page de
    // streaming la montre, tout — API, gateway, façade v4 — vise l'hôte,
    // pas le kernel local. La surcharge vit le temps de la page, posée par
    // Streaming.jsx et retirée à son démontage.
    if (streamBase) {
        return streamBase;
    }

    const declared = window.GAMEFLEX_SERVER;

    // Une origine vide, absente ou égale à l'origine courante revient au même :
    // le chemin relatif. Autant ne pas allonger toutes les URL pour rien.
    if (!declared || declared === window.location.origin) {
        return "";
    }

    return String(declared).replace(/\/+$/, "");
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
 * d'AUTRE ? », et c'est une question différente — voir buildIceServers, où
 * s'être trompé de question coûtait toute la vidéo.
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

/**
 * CHEZ NOUS, quoi que cette page regarde.
 *
 * `serverOrigin()` suit la surcharge posée par une page qui montre la partie
 * de quelqu'un d'autre. C'est juste pour CETTE page — et faux pour tout ce qui
 * lui survit.
 *
 * Le défaut qui l'a fait naître (2026-09-07) : l'abonnement app-wide aux
 * sessions construit son URL AU MOMENT DE SE CONNECTER, et il se reconnecte
 * tout seul quand le lien tombe. Une reconnexion survenue pendant qu'une page
 * de streaming fédérée était ouverte le rebranchait sur l'hôte — avec NOTRE
 * jeton, que celui-ci refuse — et il y restait après la fermeture de la page.
 * Plus de « Live Now », plus de « Mes sessions », donc plus aucun moyen de
 * rejoindre quoi que ce soit, y compris ses propres parties locales.
 *
 * La règle : ce qui vit plus longtemps qu'une page ne suit pas la surcharge
 * d'une page.
 */
export function homeOrigin() {
    if (typeof window === "undefined") {
        return "";
    }

    const declared = window.GAMEFLEX_SERVER;
    if (!declared || declared === window.location.origin) {
        return "";
    }

    return String(declared).replace(/\/+$/, "");
}

/** Une URL de WebSocket vers NOTRE serveur — voir `homeOrigin`. */
export function homeWsUrl(path) {
    const origin = homeOrigin();
    if (!origin) {
        const protocol = window.location.protocol === "https:" ? "wss:" : "ws:";

        return `${protocol}//${window.location.host}${path}`;
    }

    return `${origin.replace(/^http/, "ws")}${path}`;
}

/** Une URL d'API, relative comme avant si aucun serveur n'a été déclaré. */
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
