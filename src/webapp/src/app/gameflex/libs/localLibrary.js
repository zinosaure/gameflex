// La bibliothèque du téléphone — chantier 6, C6-P7.
//
// Un adaptateur, et rien d'autre : il traduit ce que le pont natif rend vers
// la forme que l'interface consomme DÉJÀ pour les jeux du serveur. Aucun
// écran ne connaît deux formes de jeu ; c'est ici, et seulement ici, que les
// deux se rejoignent.
//
// Pourquoi ça compte : sans cet adaptateur, chaque liste, chaque fiche et
// chaque bouton Jouer devrait demander « d'où vient ce jeu ? » et se
// brancher en conséquence. Un jour l'un d'eux oublierait.
//
// Le scanner lui-même n'est pas ici : c'est le MÊME code Go que le serveur,
// compilé pour Android (shared/storage). Ce fichier ne fait que le lire.
import { can } from "./capabilities.js";
import { onNativeLibraryChanged } from "./deviceStream.js";
import { isRemoteSource, sourceOf } from "./sources.js";
import { apiUrl } from "./serverBase.js";

/** La bibliothèque vient du kernel (HTTP) — le pont n'existe plus (C7-P3). */
function gatewayLibrary() {
    return can("webrtcGateway");
}

/**
 * Y a-t-il une bibliothèque locale à lire ?
 *
 * UNE SEULE QUESTION, ET C'EST L'HÔTE QUI Y RÉPOND (chantier 8, C8-P5).
 *
 * Cette fonction demandait aussi « suis-je sur un téléphone ? » et « ai-je un
 * gateway ? ». Deux conjonctions historiques, du temps où une application à
 * moitié migrée pouvait déclarer une bibliothèque sans savoir la servir.
 *
 * Elles coûtaient cher : cinq écrans partagés lisaient cette fonction, donc
 * cinq écrans posaient un test de plateforme sans le savoir. Un hôte qui
 * déclare `localLibrary` a une bibliothèque locale — c'est toute la question,
 * et la déclaration est ce qui fait autorité.
 */
export function hasLocalLibrary() {
    return can("localLibrary");
}

// LES BIOS, retenus : leurs consommateurs (`localBios`…) sont
// SYNCHRONES — le pont l'était —, donc ils se chargent une fois au démarrage
// (index.js), puis à chaque bibliothèque relue.
//
// LES JEUX NE SE RETIENNENT PLUS ICI (chantier 15) : la liste et la fiche
// d'un jeu les lisent dans le listing de l'appareil (`listRoms`), comme sur le
// serveur. `/api/v1/library/games`, qui reparcourait le disque à chaque
// appel, est supprimée, et avec elle le scan que ce fichier gardait.
let cachedBios = null;
// La lecture des BIOS en cours : `rescanLocalLibrary` l'attend avant de dire
// « relue ».
let biosPreload = null;

export function preloadLocalLibrary() {
    if (!gatewayLibrary()) {
        return;
    }

    biosPreload = fetch(apiUrl("/api/v1/library/bios"), { cache: "no-store" })
        .then((response) => response.json())
        .then((payload) => {
            cachedBios = Array.isArray(payload) ? payload : [];
        })
        .catch(() => {
            cachedBios = [];
        });
}

// LES VOLUMES, ET CHANGER DE RACINE, ne sont plus ici : l'emplacement de la
// bibliothèque d'un téléphone se choisit dans son application (le porteur,
// 2026-10-15), et le kernel ne sert plus `/api/v1/library/volumes` ni
// `/api/v1/library/root`.

// LES BIOS QU'UNE CONSOLE EXIGE NE SE DÉCLARENT PLUS ICI (#162).
//
// Ce fichier portait deux tables — `REQUIRED_BIOS` et `OPTIONAL_BIOS` — et
// `missingBiosFor`. L'utilisateur a tranché le 2026-09-17 : « require bios et
// bios optional fait déjà le job. Sauf que c'est pas dans registry.go, c'est
// ça ? » Oui. `#161` les a mises au registre (`Bios` = au moins un est requis,
// `BiosOptional` = améliore sans exiger), d'où elles voyagent déjà jusqu'à la
// page par `GET /api/v1/consoles`.
//
// LA RÈGLE VIT MAINTENANT DANS `launch-mode.js` (`missingRequiredBios`,
// `missingOptionalBios`), avec les autres règles PURES : depuis que la
// présence lui arrive en argument, elle ne dépend plus du cache de l'appareil
// — et un module sans dépendances s'éprouve dans node, ce que celui-ci ne
// permet pas. Seul `deviceBiosNames` reste ici, parce qu'il lit ce cache.
//
// CE QUE LA MIGRATION A CORRIGÉ, mesuré le 2026-09-18 : la table exigeait un
// `saturn_bios.bin` que le registre ne connaissait pas (ajouté, il est sur le
// disque) ; trois noms pour `ps2` dont DEUX n'existent nulle part — ni sur la
// machine, ni ailleurs dans le dépôt — alors que le cœur LRPS2 veut un DOSSIER
// et balaie ses 71 dumps ; et une clé `segacd` désignant une console absente du
// registre ET de la bibliothèque, donc une exigence que rien ne pouvait
// interroger. Le raisonnement qui vivait dans ces commentaires (« un
// gba_bios.bin ne fait pas démarrer une PlayStation », le HLE silencieux de
// `pcsx_rearmed`, la Switch qui veut `prod.keys` et non « prod.keys OU
// title.keys ») est passé côté registre, où il décide pour les deux hôtes.

/** Les noms de BIOS présents sur l'APPAREIL, pour les règles de `launch-mode.js`. */
export function deviceBiosNames(consoleId) {
    return localBiosNames(consoleId);
}

/**
 * Les noms des BIOS installés — toutes consoles, ou celles d'une console.
 *
 * Retirée un temps par une passe de code mort qui ne comptait que les
 * appelants EXTERNES, alors qu'un appelant interne l'utilisait (2026-09-09).
 * Privée, donc, et non morte — c'est `deviceBiosNames` qui l'expose
 * aujourd'hui, depuis que la règle « quels BIOS réclame cette console » vit
 * dans le registre (#162) et non plus dans ce fichier.
 */
function localBiosNames(consoleId = null) {
    return localBios()
        .filter((entry) => !consoleId || entry.consoleId === consoleId)
        .map((entry) => (typeof entry === "string" ? entry : entry.name));
}

/**
 * Relire POUR DE BON : les BIOS, puis le signal « relue » — la liste et la
 * fiche d'un jeu relisent alors le listing de l'appareil, que le kernel a
 * refait AVANT d'annoncer que la bibliothèque a changé (chantier 15).
 *
 * Rend une promesse : l'appelant sait quand redessiner, au lieu de
 * redessiner trop tôt sur un cache encore vide.
 */
export async function rescanLocalLibrary() {
    preloadLocalLibrary();
    if (biosPreload) {
        await biosPreload.catch(() => { });
    }
    announceReloaded();
}

// ── Le signal « la bibliothèque a été RELUE » ───────────────────────────
//
// Distinct de « elle a changé » : entre les deux il y a une lecture, et un
// abonné prévenu trop tôt relit l'ancien cache. C'est ce qui arrivait au menu
// des consoles — il se rafraîchissait sur le signal de changement, avant que
// la nouvelle liste ne soit là, et gardait donc les six consoles du démarrage
// alors que la ressource distante en apportait soixante (2026-09-02).
const reloadListeners = new Set();

export function onLocalLibraryReloaded(callback) {
    reloadListeners.add(callback);

    return () => reloadListeners.delete(callback);
}

function announceReloaded() {
    for (const callback of reloadListeners) {
        try {
            callback();
        } catch {
            // Un abonné qui échoue ne doit pas priver les autres.
        }
    }
}

// C8-P2 : `deviceConsoles` et `deviceRoms` ont été SUPPRIMÉES.
//
// Elles recomposaient dans le navigateur une liste que le kernel du
// téléphone compose déjà — lui seul connaît les serveurs enregistrés et leur
// jeton depuis que l'appareil a des comptes. La page en tirait donc une
// SECONDE liste, qui ne pouvait que dériver de la première, et
// `remoteCatalog()` y répondait déjà par un tableau vide.
//
// La liste vient maintenant de `listRoms`/`listConsoles`, les mêmes que sur
// le serveur.

// ── Quand plusieurs endroits ont le même jeu ────────────────────────────
//
// Un même titre peut exister à trois endroits à la fois : sur l'appareil, sur
// un GameFlex du réseau, et sur un partage monté. Il ne doit apparaître QU'UNE
// fois dans la liste — et c'est la copie la plus sûre à lancer qui doit
// gagner, pas celle qui est arrivée en premier dans le tableau.
//
// L'ordre est une DÉCISION (demandée le 2026-08-31), pas une déduction :
//
//   local        il est là ; instantané, et il marche serveur éteint
//   server       la fédération : un autre GameFlex du réseau
//   smb, webdav  un partage monté, donc suspendu à la machine qui le sert
//
// Le nommer ici, en un seul endroit, évite qu'un tri implicite le décide
// ailleurs à notre place. Signalé le 2026-08-27 avant que la règle existe :
// un jeu PSP téléchargé se relançait quand même à distance, les deux entrées
// coexistant sans que rien ne tranche.
// La règle d'origine vit dans libs/sources.js — voir ce fichier pour la
// raison. IMPORTÉE puis réexportée : une réexportation seule ne met pas le
// nom dans la portée de CE module. Webpack compile sans broncher ; seul le
// navigateur le dit (2026-09-02).
export { isRemoteSource, sourceOf };

// `preferClosestSource` a été SUPPRIMÉE le 2026-09-18, et `variants` avec
// elle. Elle repliait les copies d'un même titre en UNE entrée portant les
// autres, pour que la liste ne montre pas trois fois le même jeu ; la fiche
// les rétalait ensuite en sélecteur. Le sélecteur de source, en tête de la
// liste, répond mieux à la même question : on ne voit qu'une origine à la
// fois, les doublons ne se croisent donc plus, et le choix revient à la
// personne au lieu d'être tranché par un classement qu'elle n'a pas demandé.
// Le champ `variants` n'a jamais existé côté Go : il naissait ici et mourait
// dans la fiche.

/** Les BIOS présents sur l'appareil. */
function localBios() {
    if (gatewayLibrary()) {
        return cachedBios || [];
    }

    return [];
}

// C8-P2 : `deviceBiosDirs`, `deviceLibrarySection` et `deviceUserSaves` ont
// été SUPPRIMÉES — le kernel sert `/api/v1/retrogaming/library/bios-dirs`,
// `/api/v1/retrogaming/library/{section}` et `/api/v1/retrogaming/savestate/user`,
// aux noms et à la forme du serveur. `api/retrogaming.js` les appelle sans
// savoir lequel des deux hôtes lui répond.

// `deviceRemoveResource` a été SUPPRIMÉE le 2026-10-02 : l'écran de stockage
// passe depuis #146 par `deleteInstalled` (storage-admin.js), aux routes du
// serveur, que le téléphone sert désormais aussi.

// C8-P2 : `nativeConsoles` a été SUPPRIMÉE. Le registre des consoles vient
// de `/api/v1/consoles`, que les deux hôtes servent (ConsolesContext.jsx).

// ── Où GameFlex cherche les jeux ────────────────────────────────────────
//
// Demandé le 2026-08-27 : « j'ai des jeux sur ma carte mémoire ; au lieu de
// les copier dans GameFlex je préfère que ce soit GameFlex qui aille dans le
// répertoire. » C'est la bonne façon de le prendre — une bibliothèque de
// plusieurs dizaines de gigaoctets ne se déplace pas pour faire plaisir à une
// application.

// ── Importer un jeu distant dans la bibliothèque (C7-P4) — SUPPRIMÉ ─────
//
// `importRemoteGame` faisait ranger un jeu distant dans la LUDOTHÈQUE par le
// kernel (`POST /api/v1/retrogaming/import`) : « le kernel le télécharge là où
// le scanner le voit, et la prochaine partie ne télécharge plus rien ».
//
// Retirée le 2026-09-18 avec le bouton « ajouter à la bibliothèque » de la
// fiche de jeu, son unique appelante. La ludothèque est PARTAGÉE et un rôle
// `user` n'a rien à y écrire : « un user normal peut télécharger sans problème
// mais ça ne va pas dans la bibliothèque ». Un téléchargement va désormais chez
// la personne ; c'est un administrateur qui ajoute le jeu à la ludothèque, par
// Réglages › Jeux (chantier 14).
//
// Un administrateur ajoute un jeu depuis l'écran du stockage, par les routes
// du serveur (`/api/v1/servers/{id}/storage/roms`, `/api/v1/mounts/{id}/import`),
// que le téléphone sert depuis le 2026-10-10 (#286). La route à sa forme à
// lui, `/retrogaming/import`, n'avait plus d'appelant et est partie.

/**
 * ANNULER le téléchargement de jeu en cours sur l'appareil — lancé par erreur,
 * il ne s'arrêtait pas (signalé le 2026-10-11). La copie s'arrête au bloc
 * suivant et nettoie d'elle-même ; la progression s'efface par le flux
 * d'événements, comme à la fin d'un téléchargement.
 *
 * Rend un message d'erreur, ou null si la demande est passée.
 */
export async function deviceCancelDownload() {
    try {
        const response = await fetch(apiUrl("/api/v1/retrogaming/download/cancel"), { method: "POST" });
        const answer = await response.json().catch(() => null);

        return answer?.error || (response.ok ? null : `HTTP ${response.status}`);
    } catch {
        return "unavailable";
    }
}

// ── Sauvegardes d'état, sur l'appareil ──────────────────────────────────
//
// C7-P3 : le kernel relaie les sauvegardes au host (POST /api/v1/retrogaming/save
// et /load, GET /api/v1/retrogaming/states). Les fichiers vont dans la
// bibliothèque, à côté des jeux — pas dans le stockage privé de l'application.

async function callKernel(path, body) {
    try {
        const response = await fetch(apiUrl(path), {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(body || {}),
        });
        const answer = await response.json().catch(() => null);

        return answer?.error || null;
    } catch {
        return "unavailable";
    }
}

// `deviceImportBios` a été SUPPRIMÉE le 2026-09-18, avec le bouton
// « récupérer depuis un serveur » de la fiche de jeu qui était son seul
// appelant. Ce qui manque se gère à l'administration, qui le sait déjà ; et le
// bouton se trompait de surcroît (son message de réussite lisait un `found`
// jamais déclaré, donc partait en ReferenceError et s'affichait en échec).

/** Retirer un jeu de la bibliothèque de l'appareil — de la place, tout de
 * suite, sans brancher le téléphone à un ordinateur. `disk` : la copie de CE
 * disque interne (chantier 27) ; sans lui, celle de la bibliothèque. */
export function deviceRemoveGame(reference, disk = null) {
    return callKernel("/api/v1/retrogaming/remove", disk ? { reference, disk } : { reference });
}

// C8-P2 : `deviceStateSlots` et `deviceRemoveSave` ont été SUPPRIMÉES. Les
// créneaux de sauvegarde passent par `/api/v1/retrogaming/savestate/slots` et
// leur suppression par `DELETE /api/v1/retrogaming/savestate`, comme sur le
// serveur — c'est le kernel qui traduit vers la clé de fichier de
// l'appareil, et plus la page.

// Un jeu qui vient d'entrer dans la bibliothèque doit s'y voir TOUT DE SUITE.
//
// Sans cet abonnement, un jeu importé restait invisible jusqu'au prochain
// démarrage (signalé le 2026-08-27 sur un jeu PSP). Le kernel a déjà refait
// le listing de l'appareil quand l'annonce arrive (chantier 15) : relire les
// BIOS, puis dire « relue », suffit.
if (typeof window !== "undefined") {
    onNativeLibraryChanged(() => {
        rescanLocalLibrary();
    });
}
