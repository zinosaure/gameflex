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

const WEB_DEFAULTS = {
    // Y a-t-il un annuaire d'utilisateurs ? Sans lui : pas d'administration
    // des membres, pas de réglages de compte, pas de connexion.
    accounts: true,
    // "accounts" : on invite quelqu'un qui a un compte.
    // "lan" : on partage un lien, et qui l'ouvre choisit son nom en arrivant.
    invites: "accounts",
    // Une bibliothèque lue sur l'appareil lui-même (scanner natif).
    localLibrary: false,
    // OÙ S'AJOUTE UN DISQUE INTERNE : « compose » sur un serveur (un volume
    // `<chemin>:/mnt/<nom>`, chantier 23), « device » sur le téléphone (son
    // écran d'accueil, chantier 27). Réglages › Ressources internes le dit.
    internalDisks: "compose",
    // Y a-t-il un SERVEUR à administrer ? Le stockage, lui, se gère des deux
    // côtés — un téléphone a des dossiers comme un serveur en a. Ce drapeau
    // ne couvre que ce qui n'existe QUE sur un serveur : le relais TURN, la
    // configuration réseau. Distinct de `accounts` parce que ce sont deux
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
    // autre GameFlex ? Un serveur, oui. Un téléphone, non : il ne fait
    // qu'enregistrer des serveurs, et sa surface fédération ne vérifie aucun
    // jeton. Sans elle, l'écran « Fédérations » offrait sur le téléphone un
    // onglet « Jetons » qui n'ouvrait rien (2026-09-15).
    issuesCatalogTokens: true,

    // Les CŒURS D'UN PAIR peuvent-ils se poser ici ? Sur un serveur, oui : le
    // pair est une machine du même genre. Sur un téléphone, non — un cœur de
    // serveur est une bibliothèque Linux qu'Android ne charge pas ; ses BIOS
    // et ses jeux, eux, viennent bien des pairs (#286). L'écran des cœurs ne
    // propose donc les pairs que là où l'hôte les accepte.
    peerCores: true,

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
    // L'hôte sait-il RÉGLER le statut de ses jeux (chantier 30) ? Le serveur,
    // oui ; le téléphone pas encore (#471) : il ne propose pas un panneau qui
    // échouerait.
    gameStatus: false,
    // LE MIROIR (chantier 29) : un lien externe où l'administrateur
    // télécharge lui-même émulateurs, firmwares, correctifs et paramètres, déclaré par le
    // serveur (GAMEFLEX_MIRROR_URL). Vide — et c'est le cas du téléphone, qui
    // prend ses émulateurs chez son serveur (chantier 28) : aucun écran ne le
    // propose.
    mirrorUrl: "",
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
    // L'hôte distribue-t-il une configuration TURN ? Un serveur, oui (son
    // relais). Un téléphone, non : sa signalisation reste sur le réseau
    // local. Demander quand même donnait un 404 à CHAQUE négociation vidéo —
    // sans conséquence (le repli STUN existe) mais assez bruyant pour cacher
    // une vraie panne dans la console (2026-09-05).
    turnConfig: true,
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
    // Le bouton « Test » des manettes : il lance une session keytest, une
    // console qui ne part pas dans la 0.3.0.
    gamepadTest: false,

    platform: "web",
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
    if (bootPromise) {
        return bootPromise;
    }

    bootPromise = (async () => {
        if (cached) {
            return cached;
        }

        try {
            const response = await fetch("/api/v1/capabilities", { cache: "no-store" });
            if (response.ok) {
                const payload = await response.json();
                const declared = payload?.capabilities;
                if (declared && typeof declared === "object") {
                    cached = { ...WEB_DEFAULTS, ...declared };

                    return cached;
                }
            }
        } catch {
            // L'hôte ne déclare rien : les défauts web s'appliquent, comme
            // avant ce changement.
        }

        return capabilities();
    })();

    return bootPromise;
}

/** Raccourci lisible : `can("accounts")`. */
export function can(name) {
    return !!capabilities()[name];
}

/** L'hôte est-il l'application native ? À n'utiliser que pour DIRE, pas pour décider. */
/**
 * Le nom que CET appareil se donne — « Pixel 7 GameFlex ».
 *
 * Il est déclaré par l'hôte, jamais deviné : le navigateur ne connaît pas le
 * modèle du téléphone, et un « Android » générique ne distinguerait pas deux
 * invités sur le même réseau. C'est ce nom qu'un invité présente à l'hôte en
 * demandant la manette.
 */
export function deviceDisplayName() {
    return capabilities().hostname || "GameFlex";
}

// C8-P5 : `isNativeHost()` a été SUPPRIMÉE.
//
// Elle portait déjà son propre avertissement — « à n'utiliser que pour DIRE,
// pas pour décider » — et douze endroits s'en servaient pour décider. C'est
// la fin prévisible d'une échappatoire laissée ouverte : elle répond toujours,
// elle ne demande rien à personne, et elle est plus courte à écrire que la
// capacité qu'il aurait fallu déclarer.
//
// Ce qu'elle permettait de DIRE est couvert par `deviceDisplayName()`, qui
// nomme l'appareil. Ce qu'elle permettait de décider est maintenant déclaré :
// fullscreen, castToTv, pageAudio, rewind, nativeShare, screenOrientation,
// l'afficher, pas pour s'y brancher.
