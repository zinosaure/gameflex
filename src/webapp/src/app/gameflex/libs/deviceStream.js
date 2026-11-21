// Le jeu du téléphone, vu comme n'importe quelle autre source — C6-P10,
// puis C7-P3.
//
// C7-P2 faisait la même négociation que le gateway par le pont natif ; C7-P3
// a supprimé le pont : il ne reste qu'UN transport — le gateway in-process du
// kernel, que GatewayWebRTC joint comme celui d'un serveur. Ce fichier porte
// le magasin de progression des téléchargements, qui survit tant que la page
// l'affiche.
import { useSyncExternalStore } from "react";
import { can, loadHostCapabilities } from "./capabilities.js";
import { apiUrl } from "./serverBase.js";


// Ce que l'application raconte pendant qu'elle travaille : la progression
// d'un téléchargement, et le fait qu'un jeu vienne d'entrer dans la
// bibliothèque.
//
// Ces deux-là arrivent HORS de toute négociation — pendant un import, par
// exemple, alors qu'aucune page de streaming n'est ouverte. Ils ont donc
// leurs propres abonnés, indépendants du transport de signalisation.
const progressListeners = new Set();
const libraryListeners = new Set();
// Les échecs d'un téléchargement OU d'un lancement sur l'appareil. Ils
// arrivent APRÈS coup — le natif a déjà répondu « accepté » — donc ils ont
// leur propre canal, comme la progression : sans lui, un échec ne quittait
// jamais le logcat. Le bouton restait sur « téléchargement… » pour toujours
// (2026-09-01), et la page de jeu devant un écran noir quand le cœur refusait
// le jeu (2026-10-02).
const failureListeners = new Set();

// LE DERNIER ÉCHEC, RETENU — avec son jeu et son heure. La page de jeu
// s'abonne parfois un instant APRÈS l'échec qui la concerne (19 ms mesurées
// le 2026-10-02 : un jeu que le cœur refuse échoue aussitôt lancé) ; sans
// cette mémoire, l'annonce était perdue et la page attendait l'image
// indéfiniment. Un jeu qui démarre (« ready ») l'efface.
let lastFailure = null;

// `callback` reçoit `{ reason, game }` — `game` est le chemin du jeu
// (celui de `device:<chemin>`), vide quand l'échec ne vise aucune partie.
export function onDeviceFailed(callback) {
    failureListeners.add(callback);

    return () => failureListeners.delete(callback);
}

// L'échec récent de CE jeu, s'il y en a un.
export function recentDeviceFailure(game, withinMs = 30000) {
    if (!lastFailure || !game || lastFailure.game !== game) {
        return null;
    }

    return Date.now() - lastFailure.at < withinMs ? lastFailure : null;
}

// The LAST download in flight, as a tiny external store. Components render
// from it with useDownloadProgress; the event channel below is what feeds
// it. `null` means nothing is moving — a download that reached 100% is not
// a download any more.
let currentProgress = null;

function onDownloadProgress(callback) {
    progressListeners.add(callback);

    if (currentProgress) {
        // A late subscriber (a panel opened mid-download) must see where
        // things stand right away, not wait for the next percent.
        callback(currentProgress);
    }

    return () => progressListeners.delete(callback);
}

export function useDownloadProgress() {
    return useSyncExternalStore(
        (callback) => onDownloadProgress(callback),
        () => currentProgress,
        () => null,
    );
}

export function onNativeLibraryChanged(callback) {
    libraryListeners.add(callback);

    return () => libraryListeners.delete(callback);
}

// UNE PARTIE VIENT DE DÉMARRER SUR CET APPAREIL.
//
// On reste abonné, et on est prévenu CHAQUE fois. (Le signal à usage unique
// « est-ce déjà parti ? », `markReady`, n'avait plus d'abonné : retiré le
// 2026-10-15.)
//
// À quoi ça sert : quelqu'un qui ouvre le lien d'invitation avant que l'hôte
// n'appuie sur Jouer n'a pas de compte, donc pas de canal façade — celui-ci
// exige un jeton. Sa page demandait sa session UNE fois, ne la trouvait pas
// (elle n'existait pas encore), et n'en reparlait plus jamais : « connexion… »
// pour toujours, même une fois la partie lancée (reproduit le 2026-09-08 sur
// l'émulateur). Ce signal-ci lui dit quand redemander — c'est une poussée, pas
// un sondage, et c'est la différence qui compte.
const startListeners = new Set();

export function onDeviceGameStarted(callback) {
    startListeners.add(callback);

    return () => startListeners.delete(callback);
}

function announce(listeners, payload) {
    for (const callback of listeners) {
        try {
            callback(payload);
        } catch {
            // Un abonné qui échoue ne doit pas priver les autres.
        }
    }
}

function setProgress(payload) {
    currentProgress = payload && payload.percent < 100 ? payload : null;
    announce(progressListeners, currentProgress);
}

// DEUX SOURCES, UN SEUL MAGASIN.
//
// Le canal natif n'est plus le seul à faire descendre des fichiers : depuis
// #154, un administrateur rapatrie un cœur, un BIOS ou un jeu depuis un
// serveur fédéré, et c'est le SERVEUR qui va le chercher. Ce transfert-là doit
// se voir dans la cloche comme les autres — c'est exactement ce qu'elle existe
// pour dire.
//
// Pourquoi pas un second magasin : `useDownloadProgress` a cinq
// consommateurs (la cloche, deux endroits de Games.jsx, Streaming.jsx,
// Essentials.jsx). Un magasin parallèle devrait être fusionné chez chacun, et
// le premier qui oublierait la fusion afficherait un écran qui mente. La
// fusion légitime restante est celle de Games.jsx, qui télécharge DANS la page
// et connaît donc ses propres octets (`webProgress || downloadProgress`).
//
// `percent: -1` dit « en cours, quantité inconnue » : la page ne compte aucun
// octet quand c'est le serveur qui transfère, et l'anneau tourne au lieu de
// mentir à 0 %. Publier `null` met fin au transfert.
export function publishDownloadProgress(payload) {
    setProgress(payload);
}

// LE CANAL DE NOUVELLES — servi par le kernel, plus poussé dans la page.
//
// Ces quatre nouvelles arrivaient en appelant du JavaScript DANS la page
// depuis Java (`web.evaluateJavascript("GameFlexNativeEvents.onReady()")`).
// Ce chemin n'existe QUE dans une WebView : ouvrir la même application dans
// Chrome — sur l'appareil, ou depuis un autre téléphone du réseau — donnait
// une page qui attend « l'émulateur tourne » pour toujours, donc aucune
// vidéo, et sans le moindre message (constaté le 2026-09-03). Ce n'était pas
// une limite du navigateur, c'étaient quatre fonctions qui n'existaient pas
// chez lui.
//
// `EventSource` se reconnecte tout seul — c'est dans la spécification, pas
// dans notre code — ce qui couvre le kernel qui redémarre et le téléphone qui
// se réveille, sans machine à états de notre côté.
//
// Le canal s'ouvre DÈS LE CHARGEMENT, pas seulement quand une page de
// streaming s'ouvre : un import se déclenche depuis la fiche d'un jeu, et
// c'est là qu'on veut voir la progression.
let eventStream = null;

function openEventStream() {
    if (typeof window === "undefined" || typeof EventSource === "undefined") {
        return;
    }
    // Un hôte qui ne sert pas ce flux (un serveur GameFlex ordinaire) n'a rien
    // à raconter de cette nature : inutile d'ouvrir une connexion qui
    // échouera en boucle. La déclaration de capacités le dit déjà.
    if (!can("webrtcGateway")) {
        return;
    }
    if (eventStream) {
        return;
    }

    eventStream = new EventSource(apiUrl("/api/v1/events"));
    // CHAQUE (RE)CONNEXION REPART DE L'ÉTAT RÉEL. Le kernel rejoue, dès
    // l'abonnement, la progression en cours s'il y en a une (`replayable`,
    // events.go) — et rien sinon. Une page déconnectée au moment du « 100 % »
    // (onglet en arrière-plan, serveur qui redémarre) gardait donc ses 97 %
    // pour toujours : rien ne venait les démentir (signalé le 2026-10-11).
    // On oublie à l'ouverture ; le rejeu qui suit dit ce qui descend encore.
    eventStream.addEventListener("open", () => {
        setProgress(null);
    });
    eventStream.addEventListener("progress", (event) => {
        try {
            setProgress(JSON.parse(event.data));
        } catch {
            // Une progression illisible se saute.
        }
    });
    eventStream.addEventListener("library", () => {
        // LA BIBLIOTHÈQUE A CHANGÉ : le fichier est arrivé, donc il ne
        // descend plus. Rien ne signalait la FIN — la dernière progression
        // restait affichée indéfiniment, et une entrée figée à 87 % laisse
        // croire qu'on attend encore (signalé le 2026-09-08). L'échec avait
        // déjà son signal (« failed ») ; le succès n'en avait aucun.
        setProgress(null);
        announce(libraryListeners, null);
    });
    eventStream.addEventListener("failed", (event) => {
        // Une progression en cours ne l'est plus : le fil qui la publiait
        // vient de s'arrêter.
        setProgress(null);
        let failure = { reason: null, game: "" };
        try {
            const data = JSON.parse(event.data) || {};
            failure = { reason: data.reason || null, game: data.game || "" };
        } catch {
            // Sans motif lisible, on signale quand même l'échec.
        }
        lastFailure = { ...failure, at: Date.now() };
        announce(failureListeners, failure);
    });
    eventStream.addEventListener("ready", () => {
        lastFailure = null;
        announce(startListeners, null);
    });
    // Pas de `onerror` qui referme : EventSource retente de lui-même, et
    // fermer ici transformerait une coupure passagère en silence définitif.
}

// APRÈS la déclaration de l'hôte, jamais avant.
//
// `can()` construit et RETIENT les capacités au premier appel : l'appeler au
// chargement du module, avant que `GET /api/v1/capabilities` ait répondu,
// figerait les valeurs du web pour toute la session — et pas seulement pour
// ce flux. On attend donc la même promesse que le démarrage utilise déjà
// (elle est mémoïsée, ça ne coûte pas un second appel).
loadHostCapabilities().then(openEventStream).catch(() => {
    // Un hôte qui ne déclare rien n'a pas ce flux : il n'y a rien à ouvrir.
});

// C8-P5 : `canStreamLocally` a été SUPPRIMÉE. Son unique appelant décidait
// si le téléphone pouvait diffuser avant de lancer un jeu — une question que
// C8-P3 a rendue sans objet, puisque le lancement passe désormais par le
// serveur, qui répond ou non.

// La session fixe du téléphone — la seule que le gateway du kernel connaît.
// C8-P4 : LA SIGNALISATION ET LES VERBES DE JEU ONT ÉTÉ SUPPRIMÉS.
//
// Ce fichier portait un transport complet — `gatewaySignal`, `deviceSignal`,
// `guestSignal` — plus les verbes du menu en HTTP : disques, pause, arrêt,
// type de manette. Un second chemin, à côté de celui du serveur, pour les
// mêmes gestes.
//
//   la vidéo    `SessionStream` demande son viewer au gateway sous
//               `session.childSessionId`, le nom que le child a donné au
//               flux. Le chemin PAR DÉFAUT sert donc déjà le téléphone.
//   le contrôle le kernel sert `/api/v1/retrogaming/stream/ws`, le canal du
//               serveur, et `use-retrogaming-input.js` le pilote.
//   l'entrée    elle passait déjà par le canal de données du gateway, des
//               deux côtés, avec les mêmes identifiants libretro.
//
// Ce qui reste ici n'a pas d'équivalent côté serveur et n'en aura pas : les
// nouvelles que l'appareil pousse pendant qu'il travaille (téléchargement en
// cours, bibliothèque relue, échec), lues par le flux SSE. Un serveur n'a
// personne à qui les raconter.
