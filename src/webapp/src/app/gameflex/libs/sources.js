// D'OÙ VIENT UN JEU — la règle, et rien d'autre.
//
// Dans son propre fichier parce que deux modules qui s'importent l'un l'autre
// finissent par se lire à moitié initialisés : `localLibrary` a besoin de
// `deviceStream` pour savoir quand la bibliothèque change, et `deviceStream` a
// besoin de cette règle-ci pour savoir s'il faut récupérer un jeu avant de le
// lancer. Un prédicat de deux lignes ne justifie pas ce nœud — ni sa copie.

// `SOURCE_PRIORITY` et `sourceRank` ont été SUPPRIMÉS le 2026-09-18, avec
// `preferClosestSource` qui était leur seule cliente. Classer les origines
// servait à élire une copie GAGNANTE quand la liste repliait les doublons
// d'un même titre. La source se choisit désormais explicitement, en tête de
// la liste : il n'y a plus de gagnant à désigner, donc plus de rang à tenir.

/** Un jeu vient-il d'ailleurs ? Le local est l'ordinaire, et l'ordinaire ne se signale pas. */
export function isRemoteSource(kind) {
    return Boolean(kind) && kind !== "local";
}

/**
 * D'OÙ vient ce jeu, et LEQUEL — le couple `(source, id)` demandé le
 * 2026-09-17 pour qu'une fiche de jeu sache de quoi elle parle :
 * `local` [id: 0], `federation` [id: serverId], `distance` [id: mountId].
 *
 * POURQUOI UN COUPLE ET PAS `kind` SEUL : `kind` sait dire « ça vient d'un
 * kDrive », jamais DUQUEL. Deux kDrive enregistrés étaient indistinguables, et
 * un jeu distant ne pouvait donc pas être rouvert à sa propre source (#163).
 *
 * LES DEUX IDENTIFIANTS SONT DES CHAMPS, jamais une chaîne à découper : le
 * serveur les pose (`serverId` dans federation_catalog.go, `mountId` dans
 * mounts_client.go) et le téléphone aussi (mobilekernel.go). La référence
 * reste un localisateur opaque — `mnt3:jeux/Mario.zip` sert à ALLER lire le
 * fichier, pas à raconter d'où il vient.
 *
 * C'EST `kind` QUI DÉCIDE DE LA NATURE, le champ ne fournit que le numéro.
 * L'inverse (déduire la nature de la présence d'un identifiant) ferait passer
 * pour local un jeu distant dont l'identifiant manquerait — un mensonge, là où
 * un `id: 0` sur une origine distante se remarque.
 */
export function sourceOf(rom) {
    const kind = rom?.kind || "local";

    if (kind === "server") {
        return { source: "federation", id: Number(rom?.serverId) || 0 };
    }
    if (isRemoteSource(kind)) {
        return { source: "distance", id: Number(rom?.mountId) || 0 };
    }

    return { source: "local", id: 0 };
}
