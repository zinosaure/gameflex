// CE QUI VA SE PASSER SI ON APPUIE SUR JOUER — la décision, et rien d'autre.
//
// DES MODES DE LANCEMENT, ET DES REFUS QUI SE NOMMENT. Vocabulaire corrigé
// par l'utilisateur le 2026-09-18, en trois temps :
//
//   « Pour lancer un jeu il n'y a pas 5 modes à proprement parler, il n'y en a
//     que 3. Et les 2 autres sont des noLaunch. »
//   « Missing est divisé en 2 : core missing ou bios missing, du coup c'est des
//     noLaunch pour full et hybride. »
//   « Un autre état d'erreur qu'on peut avoir : server offline (si le serveur
//     federated ou remote content est offline). »
//
// Puis le chantier 14 (2026-10-12) a retiré le troisième : le mode `hybrid` —
// un jeu d'ailleurs copié dans un cache pour être joué ici — n'existe plus.
// « Mettre en cache dans gameflex/cache/roms, c'est exactement comme mettre dans
// gameflex/roms » (le porteur) : un jeu d'ailleurs entre dans la bibliothèque
// par Réglages › Jeux, ou se joue chez son serveur.
//
// Ce n'est pas qu'une affaire de mots. Pour le JOUEUR il n'y a que deux façons
// de lancer un jeu ; les autres cas ne sont pas des modes mais des
// empêchements, et un empêchement doit dire CE QUI MANQUE — « il manque le
// cœur », « il manque le BIOS » et « le serveur ne répond pas » ne se corrigent
// pas du tout au même endroit.
//
//   Modes (le jeu part) :
//     1. `full`        jeu, cœur et BIOS sont tous ICI ;
//     2. `federated`   tout est chez le pair, et on y joue sous SON compte de
//                      chez lui : le jeton qu'on y a reçu, collé ici une fois
//                      à la place de l'identifiant et du mot de passe
//                      d'autrefois (chantier 20).
//
//   Refus (rien ne part) :
//     - `server-offline`  le pair ne répond pas. Refus de `federated` ;
//     - `federation-refused` le pair répond, et refuse le jeton du LIEN — il
//                         l'a révoqué. Ses jeux ne se lisent plus d'ici, pour
//                         personne (chantier 20). Refus de `federated` ;
//     - `unauthorised`    on n'a pas collé le jeton de son compte chez lui :
//                         ses jeux se voient et se téléchargent, ils ne s'y
//                         lancent pas. Refus de `federated` ;
//     - `account-refused` il refuse le jeton qu'on a collé — désactivé, échu,
//                         régénéré, compte supprimé. Refus de `federated` ;
//     - `remote-launch-refused` notre compte chez lui ne lance pas cette
//                         console (ses consoles, chantier 20). Refus de
//                         `federated` ;
//     - `console-not-granted` ce compte-ci ne lance pas cette console ICI
//                         (chantier 20). Refus de `full` : chez un pair, ce
//                         sont ses droits à lui qui comptent ;
//     - `owner-only`      ici, seul un administrateur lance une partie — le
//                         propriétaire d'un téléphone, d'ordinaire
//                         (`onlyAdminsLaunch`, 2026-10-08). Refus de `full` ;
//                         jamais de `federated`, qui suit les règles du pair ;
//     - `not-in-library`  le jeu n'est pas dans la bibliothèque d'ici : il vient
//                         d'un serveur fédéré. Refus de `full` — on l'ajoute par
//                         Réglages › Jeux, ou on le joue chez son serveur
//                         (chantier 14) ;
//     - `game-missing`    le pair visé n'a pas ce jeu. Confirmé par
//                         l'utilisateur, et pour une raison qui vaut d'être
//                         écrite : en fédéré, « on ne sait jamais si le jeu est
//                         toujours sur le serveur ou pas avant le lancement ».
//                         La bibliothèque du pair change sans nous prévenir ;
//     - `core-missing`    le cœur choisi n'est pas là ;
//     - `bios-missing`    le BIOS requis n'est pas là.
//
// L'ORDRE DES REFUS EST UNE DÉCISION, PAS UN HASARD : joignabilité, puis
// authentification, puis droits, puis inventaire. On ne sait rien de ce qu'un
// serveur contient tant qu'on ne l'atteint pas, et rien de ce qu'il nous laisse
// lancer tant qu'on n'y est pas entré. Dire « colle ton jeton » d'un serveur
// éteint serait un mauvais conseil ; dire « il manque un cœur » à qui n'a pas
// le droit de jouer serait une devinette.
//
// `server-offline` NE CONCERNE JAMAIS `full` : un jeu d'ici, avec son cœur
// d'ici, ne dépend d'aucun serveur. D'où la valeur par défaut « en ligne » —
// une information absente ne doit pas bloquer un lancement purement local.
//
// LE MODE RESTE RENSEIGNÉ MÊME QUAND C'EST REFUSÉ, et c'est tout l'intérêt du
// couple : c'est lui qui permet de dire « ce jeu se lancerait ici, mais il
// manque le cœur ». Une chaîne unique ne savait dire que « missing ».
//
// CHEZ LE PAIR, UN CŒUR OU UN BIOS ABSENT SE DIT DANS LE SÉLECTEUR, pas ici :
// « le jeu a comme possibilité 3 bios mais le serveur n'en a que 2, alors le
// 3ème est indisponible (disabled) ». Cette fonction reste néanmoins TOTALE —
// si un appelant annonce quand même une indisponibilité pour une cible
// distante, elle la nomme au lieu de prétendre que tout va bien.
//
// REMOTE PLAY N'EST PAS CONCERNÉ, et il ne faut pas essayer de le faire passer
// par ici : il lance une CONSOLE, pas un jeu. Il n'a donc ni cœur, ni BIOS, ni
// jeu à trouver — sa question est « la console répond-elle, et ai-je le droit
// de m'y attacher », qui n'est pas celle-ci.
//
// POURQUOI UNE FONCTION PURE, DANS SON PROPRE FICHIER. La décision vivait
// dispersée dans le JSX — un empilement de conditions sur un `isDisabled`
// (`Games.jsx`), qui savait dire « le bouton est mort » sans jamais dire
// POURQUOI. On ne pouvait donc ni l'expliquer à l'écran, ni l'éprouver. C'est
// la leçon de `zipPolicyFor`, que j'avais écrite à l'envers faute de l'avoir
// nommée : une règle qui n'a pas de nom n'a pas de test.
//
// DEUX VERROUS MORTS QUE CE FICHIER REMPLACE, et c'est la mesure qui les a
// trouvés (2026-09-17) :
//
//   - `const noCore = false;` — une constante en dur au milieu du garde-fou.
//     La page n'avait AUCUNE notion de la présence d'un cœur, donc le refus
//     `core-missing` n'était pas « difficile à calculer », il était désactivé.
//   - `biosMissing` n'était évalué que `onDevice` (sur le téléphone), donc
//     toujours faux sur le serveur — même conclusion.
//
// La donnée, elle, existait déjà : `GET /api/v1/retrogaming/{console}/cores`
// rend un `installed` par cœur, et `/bios` son équivalent. C'est l'appelant
// qui les lit et les passe ici ; cette fonction ne va rien chercher.

/** Les deux modes de lancement — ce que fait le jeu quand il part. */
const LAUNCH_MODES = Object.freeze({
    full: "full",
    federated: "federated",
});

/** Les empêchements — chacun nomme ce qui manque, jamais un « missing » global. */
export const NO_LAUNCH = Object.freeze({
    serverOffline: "server-offline",
    // Le pair répond, et refuse le jeton du LIEN (chantier 20) : il l'a
    // révoqué. Un administrateur d'ici rajoute le serveur avec le nouveau.
    federationRefused: "federation-refused",
    // Pas de jeton de notre compte chez ce pair : la fiche montre où le
    // coller. Ses jeux se voient et se téléchargent.
    unauthorised: "unauthorised",
    // Le pair refuse le jeton qu'on a collé (chantier 20) : désactivé, échu,
    // régénéré, ou son compte supprimé. On en colle un nouveau.
    accountRefused: "account-refused",
    // Ce compte-ci ne lance pas cette console ICI (chantier 20) : ses consoles
    // se règlent dans Réglages › Membres. Il en voit et télécharge les jeux.
    consoleNotGranted: "console-not-granted",
    // Notre compte chez ce pair ne lance pas cette console chez lui (chantier
    // 20) : ses jeux s'y voient et se téléchargent, ils ne s'y lancent pas.
    remoteLaunchRefused: "remote-launch-refused",
    ownerOnly: "owner-only",
    notInLibrary: "not-in-library",
    // Le même fichier (même UUID) EST dans la bibliothèque d'ici, mais la fiche
    // montre celui d'un serveur fédéré : il se lance depuis la source « Ici »,
    // pas d'ici — un jeu de la liste d'un ami ne part que chez lui.
    alsoInLibrary: "also-in-library",
    gameMissing: "game-missing",
    coreMissing: "core-missing",
    biosMissing: "bios-missing",
});

/**
 * La décision de lancement, pour UNE cible choisie.
 *
 * @param {object} decision
 * @param {"local"|"federation"|"distance"} decision.source d'où vient le jeu
 *        (le couple rendu par `sourceOf`, libs/sources.js). Tout ce qui n'est
 *        pas `local` n'est pas dans la bibliothèque d'ici.
 * @param {"here"|"peer"} decision.target où l'émulateur tournera.
 * @param {boolean} [decision.sourceOnline=true] la source distante répond-elle.
 *        Vrai par défaut : un lancement purement local n'en dépend pas, et une
 *        information absente ne doit pas le bloquer.
 * @param {boolean} [decision.tokenRefused=false] le pair refuse-t-il le jeton
 *        du LIEN — il l'a révoqué (`refused`, `GET /api/v1/servers/{id}`).
 * @param {boolean} [decision.signedIn=false] a-t-on une session chez le pair :
 *        le jeton de notre compte chez lui, collé ici (chantier 20).
 * @param {boolean} [decision.accountRefused=false] le pair refuse-t-il ce
 *        jeton-là (`accountRefused`, `GET /api/v1/servers/{id}/session`).
 * @param {boolean} [decision.remoteLaunch=true] notre compte chez le pair
 *        lance-t-il cette console chez lui : les consoles de sa session.
 * @param {boolean} [decision.consoleGranted=true] ce compte-ci lance-t-il cette
 *        console ICI (`user.consoles`, chantier 20). Chez un pair, ce sont ses
 *        droits à lui qui comptent.
 * @param {boolean} decision.coreAvailable le cœur choisi est-il présent —
 *        **sur la cible**, pas ici.
 * @param {boolean} decision.biosAvailable de même pour les BIOS. Une console
 *        qui n'en exige aucun est « satisfaite » : c'est à l'appelant de le
 *        dire (voir BiosRequiredFor côté registre, #161).
 * @param {boolean} [decision.gameAvailable=true] la cible a-t-elle ce jeu. Un
 *        émulateur fédéré ne lit que les jeux de SON serveur.
 * @param {boolean} [decision.playsHere=true] ce compte peut-il lancer une
 *        partie sur CET hôte (`playsHere`, AuthContext). Faux pour un membre
 *        sur un téléphone, où seul le propriétaire lance.
 * @param {boolean} [decision.inLibrary=false] pour un jeu d'ailleurs : le même
 *        fichier (même UUID) est-il AUSSI dans la bibliothèque d'ici ? Il ne
 *        part pas d'ici pour autant — le refus le dit seulement juste.
 * @returns {{mode: string, noLaunch: string|null, canLaunch: boolean}} le mode
 *        qu'emprunterait ce lancement, et ce qui l'empêche s'il y a lieu.
 */
export function launchModeFor({
    source,
    target = "here",
    sourceOnline = true,
    tokenRefused = false,
    signedIn = false,
    accountRefused = false,
    remoteLaunch = true,
    consoleGranted = true,
    coreAvailable = false,
    biosAvailable = false,
    gameAvailable = true,
    playsHere = true,
    inLibrary = false,
}) {
    if (target === "peer") {
        // JOIGNABILITÉ D'ABORD : un pair éteint ne peut pas non plus nous
        // ouvrir de session, et « colle ton jeton » serait un mauvais conseil.
        if (!sourceOnline) {
            return decided(LAUNCH_MODES.federated, NO_LAUNCH.serverOffline);
        }
        // IL RÉPOND, ET A RÉVOQUÉ NOTRE LIEN : ses jeux ne se lisent plus d'ici.
        if (tokenRefused) {
            return decided(LAUNCH_MODES.federated, NO_LAUNCH.federationRefused);
        }
        // PUIS QUI JOUE CHEZ LUI (chantier 20) : le jeton de notre compte de
        // chez lui — pas collé, ou refusé —, puis ce que ce compte y lance.
        if (accountRefused) {
            return decided(LAUNCH_MODES.federated, NO_LAUNCH.accountRefused);
        }
        if (!signedIn) {
            return decided(LAUNCH_MODES.federated, NO_LAUNCH.unauthorised);
        }
        if (!remoteLaunch) {
            return decided(LAUNCH_MODES.federated, NO_LAUNCH.remoteLaunchRefused);
        }

        return decided(LAUNCH_MODES.federated, whatIsMissing({
            gameAvailable, coreAvailable, biosAvailable,
        }));
    }

    const mode = LAUNCH_MODES.full;

    // À QUI EST L'HÔTE, d'abord : un membre d'un téléphone n'y lance rien, et
    // lui nommer un cœur absent serait lui suggérer une réparation qu'il ne
    // peut pas faire.
    if (!playsHere) {
        return decided(mode, NO_LAUNCH.ownerOnly);
    }
    // PUIS CE QUE CE COMPTE LANCE (chantier 20) : sans la console, il en voit
    // et télécharge les jeux, sans y jouer.
    if (!consoleGranted) {
        return decided(mode, NO_LAUNCH.consoleNotGranted);
    }

    // UN JEU D'AILLEURS NE PART PAS D'ICI (chantier 14). Le porteur, le
    // 2026-10-13 : un jeu de la liste d'un serveur fédéré se lance chez lui,
    // si on y est autorisé ; sinon l'administrateur l'ajoute à la bibliothèque
    // par Réglages › Jeux, et il se lance depuis la source « Ici ». Même quand
    // le même fichier y est déjà : c'est alors la copie d'ici qu'on lance, de
    // là. Avant l'inventaire — nommer un cœur absent pour un jeu qui ne
    // partira de toute façon pas d'ici serait un mauvais conseil.
    if (source !== "local") {
        return decided(mode, inLibrary ? NO_LAUNCH.alsoInLibrary : NO_LAUNCH.notInLibrary);
    }

    // ICI, LE CŒUR ET LES BIOS SONT OBLIGATOIRES : plus rien ne se télécharge
    // au lancement (#143), donc un cœur absent n'est plus une attente, c'est un
    // refus.
    return decided(mode, whatIsMissing({ gameAvailable, coreAvailable, biosAvailable }));
}

/**
 * Ce qui manque, dans l'ordre où ça se répare — ou null si rien ne manque.
 *
 * LE CŒUR AVANT LE BIOS : c'est lui qui fait tourner le jeu, et un BIOS sans
 * émulateur ne sert à rien. Quand les deux manquent, l'écran nomme donc le
 * premier geste à faire, pas les deux à la fois.
 */
function whatIsMissing({ gameAvailable, coreAvailable, biosAvailable }) {
    if (!gameAvailable) {
        return NO_LAUNCH.gameMissing;
    }
    if (!coreAvailable) {
        return NO_LAUNCH.coreMissing;
    }
    if (!biosAvailable) {
        return NO_LAUNCH.biosMissing;
    }

    return null;
}

/**
 * Le cœur de cette cible est-il ABSENT, à notre connaissance ?
 *
 * Sert à montrer une option d'émulateur désactivée plutôt qu'à la taire
 * (#163) : lire le nom de ce qui manque fait partie de ce qu'on est venu voir.
 *
 * ELLE VIT ICI, ET NON EN CONDITION DANS LE JSX, pour la raison qui a donné ce
 * fichier : une règle écrite dans un attribut ne s'éprouve pas. Sur la machine
 * de développement, TOUS les cœurs déclarés sont installés (mesuré le
 * 2026-09-18, 33 consoles, zéro absent) — l'écran ne peut donc rien prouver,
 * et seuls des cas fabriqués le peuvent.
 *
 * @param {{core: string, server: object|null}} target la cible choisie.
 * @param {{name: string, installed: boolean}[]} cores ce que `/cores` a rendu
 *        pour cette console : les cœurs DÉCLARÉS, chacun avec sa présence.
 * @returns {boolean} vrai seulement quand on SAIT qu'il manque.
 */
export function coreMissingOn(target, cores) {
    // UNE CIBLE CHEZ UN PAIR NE SE JUGE PAS D'ICI : son inventaire ne nous est
    // pas servi (`fetchRequiredCores` est une route d'administration). On ne
    // désactive donc pas son option — s'il y a un refus, il viendra de lui.
    if (!target || target.server) {
        return false;
    }

    // RIEN DE CONNU = RIEN À AFFIRMER. Tant que la liste n'est pas arrivée, ou
    // qu'elle est vide (l'appareil ne range pas ses cœurs là), aucune option
    // n'est condamnée. C'est `noCore = false` à l'envers : ne pas mentir dans
    // l'autre sens non plus.
    if (!Array.isArray(cores) || cores.length === 0) {
        return false;
    }

    return !cores.some((core) => core.installed && core.name === target.core);
}

/**
 * La cible d'une fiche tant qu'on n'en a pas choisi.
 *
 * UN JEU D'UN SERVEUR FÉDÉRÉ S'OUVRE SUR SON SERVEUR : ici ne le joue plus
 * (chantier 14), il n'y a donc rien d'autre à proposer d'abord. Jusque-là, il
 * ne s'y ouvrait que quand ici ne pouvait pas le jouer — trop gros pour notre
 * cache, ou cœur absent (#292). Même quand il est AUSSI dans la bibliothèque
 * d'ici (le même UUID) : un jeu de la liste d'un ami ne part que chez lui (le
 * porteur, 2026-10-13).
 *
 * UN JEU D'ICI s'ouvre sur la première cible d'ici dont le cœur est là, sinon
 * sur la première d'ici. Une liste de cœurs inconnue ne condamne rien
 * (`coreMissingOn`) : tant qu'elle n'est pas arrivée, c'est la première.
 *
 * @param {Array<{id: string, core: string, server: object|null}>} targets
 * @param {object} context
 * @param {number|string|null} [context.gameServerId] le serveur d'où vient le
 *        jeu, s'il vient d'un pair.
 * @param {Array} [context.cores] les cœurs d'ici, avec leur `installed`.
 */
export function defaultLaunchTarget(targets, { gameServerId = null, cores = [] } = {}) {
    if (gameServerId != null) {
        return targets.find((target) => String(target.server?.id) === String(gameServerId))
            || targets[0]
            || null;
    }

    return targets.find((target) => !target.server && !coreMissingOn(target, cores))
        || targets.find((target) => !target.server)
        || targets[0]
        || null;
}

// ── QUELS BIOS MANQUENT (#162) ──────────────────────────────────────────
//
// Ces deux règles vivaient dans `localLibrary.js`, sur ses propres tables
// `REQUIRED_BIOS` / `OPTIONAL_BIOS`. Elles sont ici pour deux raisons : le
// registre est désormais la source unique (`bios` / `biosOptional`, #161), et
// surtout elles sont devenues PURES — la présence leur arrive en argument —
// donc éprouvables dans node, ce que `localLibrary.js` ne permet pas (il
// dépend de `sources.js`, `deviceStream.js`, `capabilities`…).
//
// `biosAvailable` de `launchModeFor` se calcule avec la première : c'est la
// même question, posée une fois.

/** L'entrée de registre d'une console, ou null. */
function consoleEntry(consoles, consoleId) {
    if (!Array.isArray(consoles)) {
        return null;
    }

    return consoles.find((entry) => entry && entry.id === consoleId) || null;
}

/**
 * Les noms présents, en minuscules — les dumps circulent en `SCPH5501.BIN`
 * autant qu'en `scph5501.bin`.
 *
 * Tolère les deux formes : une chaîne, ou un objet `{name}` — c'est ce que
 * `/bios` rend (avec sa taille) et ce que l'appareil rend parfois nu.
 */
function loweredNames(names) {
    return new Set((Array.isArray(names) ? names : [])
        .map((entry) => String(entry && entry.name ? entry.name : entry).toLowerCase()));
}

/**
 * Les BIOS REQUIS qui manquent — ou une liste vide.
 *
 * « AU MOINS UN SUFFIT » : ce sont des révisions du même BIOS, et c'est la
 * sémantique que le registre porte (`BiosRequiredFor`, côté Go). Tant que l'un
 * des noms est là, rien ne manque ; sinon on rend la liste entière, parce que
 * l'écran propose de récupérer n'importe lequel.
 *
 * LA PRÉSENCE ARRIVE EN ARGUMENT, une règle pour deux hôtes : l'appareil range
 * ses BIOS dans sa bibliothèque, le serveur les sert par
 * `GET /api/v1/retrogaming/{console}/bios`. Lire l'un des deux ici aurait
 * déclaré tout manquant sur l'autre.
 */
export function missingRequiredBios(consoles, consoleId, presentNames) {
    const wanted = consoleEntry(consoles, consoleId)?.bios || [];
    if (wanted.length === 0) {
        return [];
    }

    const present = loweredNames(presentNames);

    return wanted.some((name) => present.has(String(name).toLowerCase())) ? [] : wanted;
}

/**
 * Ce que cette console ATTEND, requis et facultatifs séparés.
 *
 * La règle ci-dessus rend ce qui MANQUE ; un sélecteur, lui, doit
 * proposer l'attendu — sinon il n'offre que ce qui est déjà là, et une option
 * ne peut jamais s'afficher désactivée. C'est précisément ce que `/bios`
 * empêchait : il ne liste que le disque (`ListBiosEntries`), donc tout ce
 * qu'il rend est présent par construction (#162).
 *
 * Rend deux tableaux vides pour une console qui ne déclare rien, ou pour un
 * registre pas encore chargé : l'appelant boucle dessus, il ne teste pas.
 */
export function expectedBiosFor(consoles, consoleId) {
    const entry = consoleEntry(consoles, consoleId);

    return {
        required: entry?.bios || [],
        optional: entry?.biosOptional || [],
    };
}

/**
 * Un nom est-il présent dans cet inventaire ?
 *
 * Exposée parce que le sélecteur en a besoin option par option — la même
 * comparaison que les deux règles, donc au même endroit plutôt qu'une seconde
 * écriture qui oublierait la casse ou la forme objet.
 */
export function biosIsPresent(presentNames, name) {
    return loweredNames(presentNames).has(String(name).toLowerCase());
}

/** Le couple, sous une seule forme — pour que `canLaunch` ne se calcule pas deux fois. */
function decided(mode, noLaunch) {
    return { mode, noLaunch, canLaunch: noLaunch === null };
}
