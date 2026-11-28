// Ce que CETTE installation sait faire — et ce que l'interface doit taire.
//
// GameFlex tourne à deux endroits qui n'ont pas les mêmes moyens : un serveur,
// avec son annuaire d'utilisateurs et son stockage ; et une application
// Android, seule sur un appareil, sans compte ni annuaire. La même interface
// sert les deux, et certains écrans n'ont simplement pas de sens dans le
// second : administrer des membres, se connecter, inviter un compte.
//
// La règle, et c'est elle qui compte : **l'hôte DÉCLARE, l'interface obéit**.
// Jamais un test de plateforme. Deviner « suis-je sur Android ? » fait
// diverger deux chemins, et une fonctionnalité finit toujours par réapparaître
// là où elle n'a plus de sens.
//
// Par défaut — donc dans un navigateur — tout est disponible : c'est le
// comportement d'aujourd'hui, et rien ne bouge.

export const WEB_DEFAULTS = {
    // Y a-t-il un annuaire d'utilisateurs ? Sans lui : pas d'administration
    // des membres, pas de réglages de compte, pas de connexion.
    accounts: true,
    // "accounts" : on invite quelqu'un qui a un compte.
    // "lan" : on partage un lien, et qui l'ouvre choisit son nom en arrivant.
    invites: "accounts",
    // OÙ S'AJOUTE UN DISQUE INTERNE : « compose » sur un serveur (un volume
    // `<chemin>:/mnt/<nom>`, chantier 23), « device » sur le téléphone (son
    // écran d'accueil, chantier 27). Réglages › Ressources internes le dit.
    internalDisks: "compose",
    // Y a-t-il un SERVEUR à administrer ? Le stockage, lui, se gère des deux
    // côtés — un téléphone a des dossiers comme un serveur en a. Ce drapeau
    // ne couvre que ce qui n'existe QUE sur un serveur : la configuration
    // réseau. Distinct de `accounts` parce que ce sont deux
    // absences différentes : l'une est un annuaire, l'autre une machine.
    serverAdmin: true,
    // Cet hôte peut-il OUVRIR une session ailleurs (un conteneur streamex,
    // une console physique en jeu à distance) ? Un serveur, oui. Un
    // téléphone, non — et ça ne se déduit pas de la présence de comptes,
    // qu'il a désormais aussi.
    spawnsSessions: true,

    // Cet hôte peut-il APPAIRER une console de son propre réseau ?
    //
    // Séparée de `spawnsSessions`, et elle a dû l'être : la page d'ajout
    // d'appareil s'en déduisait, ce qui était vrai par accident tant qu'un
    // téléphone ne savait pas parler à une PS5. Depuis qu'il sert lui-même
    // les routes `/api/v1/remoteplay/*` (C9-P6), la déduction est devenue
    // fausse — et l'utilisateur voyait une page qui promettait « ou ajoute
    // un nouvel appareil » sans le moindre bouton (2026-09-09).
    //
    // C'est exactement la règle que `mobilekernel.go` énonce à côté de sa
    // propre déclaration : une capacité doit dire ce qu'elle EST, pas se
    // déduire d'une autre. `localEmulators` avait déjà dû naître pour la
    // même raison.
    pairsConsoles: true,

    // Cet hôte ÉMET-il des jetons de catalogue — ouvre-t-il ses jeux à un
    // autre GameFlex ? Un serveur, et un téléphone aussi depuis `#756` (le
    // porteur, 2026-11-17 : « le téléphone fait tout ce que fait un serveur ») ;
    // faux seulement tant que le service des comptes de l'hôte n'est pas ouvert.
    issuesCatalogTokens: true,

    // SEUL UN ADMINISTRATEUR LANCE ? Sur un serveur, non : tout compte y lance
    // une partie, une à la fois. Sur un téléphone, oui (décidé le 2026-10-08,
    // redit le 2026-10-09) : ce n'est pas un serveur qui fait tourner
    // plusieurs processus, et son propriétaire — seul administrateur
    // d'ordinaire — veut qu'il reste peu sollicité. C'est aussi le contrôle
    // parental : le parent lance, prête la manette à l'enfant, la reprend ou
    // arrête la partie quand il veut. Les comptes, eux, sont les
    // mêmes partout — root, administrateurs, membres. Lu par `playsHere`
    // (AuthContext), jamais recopié.
    onlyAdminsLaunch: false,

    // ── CE QUE L'HÔTE PEUT MONTRER OU FAIRE LUI-MÊME ──────────────────
    //
    // Huit faits, chacun remplaçant un « suis-je sur un téléphone ? » qui
    // vivait dans du code partagé (chantier 8, C8-P5). Ils sont séparés et
    // non regroupés en un « je suis l'application » parce qu'ils sont
    // VRAIMENT indépendants : un navigateur de télévision diffuse sans
    // pouvoir passer en plein écran, et le jour où l'un d'eux bouge, il
    // doit pouvoir bouger seul.

    // La page peut-elle occuper tout l'écran ? Dans l'application, elle
    // l'occupe déjà : l'hôte le dit faux à sa WebView (2026-10-29).
    fullscreen: true,
    // Diffuser vers un téléviseur.
    castToTv: true,
    // Par les fenêtres de l'application plutôt que par le SDK Cast de Chrome, qu'elles n'ont pas (#124) : la page leur
    // parle par le lien réservé `gameflex://cast` (cast.js) — la WebView du téléphone caste par Android, la fenêtre du
    // PC ouvre Chrome. Déclaré par requête, chez elles comme chez un serveur rejoint — comme `appHomeLink`.
    castLink: false,
    // La page rend-elle le son, et peut-elle donc le couper ? Sur un
    // appareil, c'est le haut-parleur qui joue la partie — couper la page ne
    // couperait rien.
    pageAudio: true,
    // Le rembobinage demande au cœur de tenir un historique d'états. Le
    // runtime embarqué n'en tient pas ; l'avance rapide, elle, existe
    // partout.
    rewind: true,
    // L'hôte a-t-il une feuille de partage système ? Un WebView n'implémente
    // pas `navigator.share` — le bouton disparaissait là où partager est le
    // geste le plus naturel (2026-09-02).
    nativeShare: false,
    // L'hôte fait-il pivoter l'écran LUI-MÊME ? Une activité Android tourne
    // pour de bon ; à défaut on retombe sur une rotation CSS qui ne tourne
    // que la vidéo, laissant le menu droit et le cadre hors de l'écran.
    screenOrientation: false,
    // L'hôte donne-t-il la batterie et la chaleur du TÉLÉPHONE QU'ON TIENT
    // (sujet `power/…`) ? Le kernel du téléphone, à sa propre WebView
    // seulement (2026-10-29). C'est aussi « sommes-nous dans l'application » :
    // sans elle, ni pastilles ni lignes de chaleur et de batterie dans le
    // panneau de performances.
    devicePower: false,
    // Cet hôte distribue-t-il l'application ? Proposer de la télécharger
    // depuis elle-même n'a pas de sens.
    appDownload: true,
    // UN BANC D'ESSAI (GAMEFLEX_STAGING, 2026-10-26) : « staging » à côté du
    // nom, et l'application se télécharge chez lui plutôt que sur GitHub.
    staging: false,
    // LE NOM DE CET HÔTE pour ses serveurs fédérés (Réglages › Configuration du serveur, chantier 81) : la fenêtre d'un code
    // d'appairage le montre. Vide : son adresse seule.
    serverName: "",
    // SA CONFIGURATION SE RÈGLE-T-ELLE DE LA PAGE (Réglages › Configuration du serveur, chantier 81) ? Un serveur seulement.
    serverConfig: false,
    // LES GUIDES PROPRES À CET HÔTE (chantier 81, la vérification des capacités) : `{ sujet: [guide, ancre] }` — le
    // téléphone a les siens (ses disques, inviter quand il est le serveur) ; ailleurs, la page garde son guide général.
    // Déclarés par l'hôte, jamais déduits d'une autre capacité (le PC disait « disques de l'appareil » comme le téléphone,
    // et recevait le guide du téléphone).
    hostGuides: {},
    // Cette page a-t-elle un ÉCRAN D'ACCUEIL où revenir — la console de
    // l'application Android ? Seule sa WebView en a un, et le kernel ne le
    // déclare qu'à elle (pas à Chrome sur le même téléphone). Un bouton de
    // Réglages y ramène, sans déconnecter (2026-10-19).
    appHome: false,
    // Ou bien cette page est-elle dans l'application d'un INVITÉ, qui a
    // rejoint cet hôte (chantier 32) ? Elle revient alors à l'accueil de SON
    // application par le lien réservé `gameflex://home`, que sa WebView
    // intercepte. Déclaré par requête, comme `appHome`.
    appHomeLink: false,
    // Peut-on prévisualiser ICI la forme d'un autre hôte ? Sur l'appareil on
    // y est déjà.
    // L'hôte va-t-il chercher les images des jeux chez IGDB et SteamGridDB
    // (chantier 49) ? Un serveur le déclare ; le téléphone non :
    // il n'appelle aucun service externe, il prend les images chez son
    // serveur.
    metadataServices: false,
    // ── CE QUI N'EST PAS PRÊT ─────────────────────────────────────────
    //
    // Caché « pour l'instant » — décidé le 2026-09-15, avant la 0.3.0. Faux
    // partout, le téléphone compris : l'hôte qui saura les servir les
    // déclarera.

    // Les notifications push : remises le 2026-10-15 — l'écran s'abonne pour
    // de bon (`subscribeToPush`). C'est le SERVEUR qui les déclare : faux par
    // défaut, donc pour un hôte qui ne dit rien (le téléphone, dont la page
    // ne sert pas de service worker à un abonné).
    pushNotifications: false,

    // Les paquets d'émulateurs que cet hôte fait tourner : « server » ou « phone » (la fiche d'une console).
    emulatorPackage: "server",

    platform: "web",

    // CE QUI S'ÉCRIT DANS LES JOURNAUX se règle de la page (Réglages › Journaux) : le serveur le déclare ; le téléphone
    // non, son journal a son interrupteur sur la console d'accueil.
    logSettings: false,
    // LES SIGNALEMENTS S'ÉCRIVENT : l'écran d'erreur d'un lancement propose « Signaler l'erreur ». Le serveur le déclare
    // selon Réglages › Journaux ; le téléphone ne le déclare pas, les siens s'écrivent toujours.
    reports: true,
    // Un téléchargement confié au navigateur du téléphone (handToBrowser, libs/api.js).
    nativeBrowse: false,
};


let cached = null;
let bootPromise = null;

/**
 * Les capacités de l'hôte. Lues UNE fois : elles décrivent l'installation, pas
 * un état qui change en cours de route.
 */
export function capabilities() {
    if (cached) {
        return cached;
    }
    cached = { ...WEB_DEFAULTS };

    return cached;
}

/**
 * Déclaration HTTP de l'hôte, lue AVANT le premier rendu (voir index.js).
 *
 * Avec mobilekernel (chantier 7), le PWA est servi par le serveur du
 * téléphone, sans pont : les capacités viennent de `GET /api/v1/capabilities`,
 * selon la règle « l'hôte DÉCLARE, l'interface obéit ». Un hôte qui n'a pas
 * cet endpoint (le web d'aujourd'hui) rend simplement les défauts — le
 * fetch échoue vite et ne retarde rien de visible.
 */
export async function loadHostCapabilities() {
    if (!bootPromise) {
        bootPromise = refreshCapabilities();
    }

    return bootPromise;
}

// RELIRE CE QUE L'HÔTE DÉCLARE (chantier 85) — la seule lecture de `GET /api/v1/capabilities` : au démarrage
// (loadHostCapabilities), et quand le launcher recharge ce qu'il sait (data.jsx). Le moteur (`can`) et les écrans
// (`can(capabilities, …)` de data.jsx) lisent donc la même déclaration, complétée par la même table.
export async function refreshCapabilities() {
    try {
        const response = await fetch("/api/v1/capabilities", { cache: "no-store" });
        const declared = response.ok ? (await response.json())?.capabilities : null;

        if (declared && typeof declared === "object") {
            cached = { ...WEB_DEFAULTS, ...declared };
        }
    } catch {
        // L'hôte ne dit rien : la déclaration d'avant, ou les défauts.
    }

    return capabilities();
}

/** Le guide d'un sujet propre à cet hôte (`hostGuides`), sinon `fallback` (null : aucun). */
export function hostGuide(topic, fallback = null) {
    return capabilities().hostGuides?.[topic] || fallback;
}

/** Raccourci lisible : `can("accounts")`. */
export function can(name) {
    return !!capabilities()[name];
}

// C8-P5 : `isNativeHost()` a été SUPPRIMÉE.
//
// Elle portait déjà son propre avertissement — « à n'utiliser que pour DIRE,
// pas pour décider » — et douze endroits s'en servaient pour décider. C'est
// la fin prévisible d'une échappatoire laissée ouverte : elle répond toujours,
// elle ne demande rien à personne, et elle est plus courte à écrire que la
// capacité qu'il aurait fallu déclarer.
//
// Ce qu'elle permettait de DIRE est couvert par `hostname`, que l'hôte déclare
// (`deviceDisplayName()`, qui le lisait pour l'invité anonyme d'un autre
// appareil, est partie avec lui, `#756`). Ce qu'elle permettait de décider est maintenant déclaré :
// fullscreen, castToTv, pageAudio, rewind, nativeShare, screenOrientation,
// l'afficher, pas pour s'y brancher.
