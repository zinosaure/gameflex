// La « session » d'un jeu qui tourne sur ce téléphone — C6-P10.
//
// Il n'y en a pas, à proprement parler : rien n'est enregistré nulle part,
// personne d'autre ne la voit, et il n'y a pas de file de joueurs. Ce que la
// page de streaming demande à une session, en revanche, tient en cinq
// champs — alors on les lui donne, depuis l'adresse.
//
// Pourquoi pas une entrée dans le pool de sessions : elle apparaîtrait dans
// « en direct », et quelqu'un essaierait de la rejoindre par un chemin qui
// n'existe pas encore. Une session locale est locale, et le dire ici évite de
// le corriger partout ailleurs.


// CE FICHIER N'INVENTE PLUS DE SESSION.
//
// Il en fabriquait deux : celle du téléphone lui-même, depuis les paramètres
// de l'adresse et depuis `/api/v1/retrogaming/current`. Rôle, propriétaire,
// places et joueurs étaient donc écrits par la page — et quatre pannes du
// 2026-09-04 en sont sorties : un invité se voyait propriétaire de la partie
// d'un autre, « 1/1 » s'affichait sur un jeu à quatre manettes, l'hôte
// perdait sa session dès qu'un spectateur arrivait, et plus personne ne
// pouvait l'arrêter.
//
//   C8-P1  `deviceCurrentSession` supprimée — le kernel sert la façade.
//   C8-P3  `deviceSessionFrom` supprimée — la partie du téléphone a un
//          identifiant comme les autres, et le pool la résout.
//
// Ce qui reste ci-dessous est le seul cas qui ne PEUT pas se résoudre ici :
// la partie d'un AUTRE appareil, qui vit dans le pool de son hôte.

/**
 * La session d'un AUTRE appareil, rejointe par un invité (C7-P4).
 *
 * L'invité est un appareil GameFlex comme un autre : sa page vit sur son
 * propre kernel, mais cette partie-ci se joue CHEZ L'HÔTE. Même forme que la
 * session du téléphone — donc même chemin dans la page de streaming — mais
 * rien à lancer ni à arrêter : le jeu tourne déjà là-bas.
 */
export function remoteSessionFrom(searchParams) {
    const host = searchParams.get("host");
    if (searchParams.get("guest") !== "1" || !host) {
        return null;
    }

    return {
        id: `remote:${host}`,
        platform: "device",
        childSessionId: "device-session",
        guest: true,
        host,
        title: searchParams.get("name") || host,
        // Ni console ni fichier : il n'y a rien à lancer. Les champs que la
        // page lit existent, vides — le jeu se présente par les frames.
        consoleId: "",
        reference: "",
        core: "",
        // ON ARRIVE EN SPECTATEUR.
        //
        // Un lien se partage largement, et qui l'ouvre n'a rien demandé de
        // plus que de regarder. Entrer directement comme joueur prenait une
        // place dans la file sans que personne ne l'ait accordée — et sur le
        // téléphone, où il n'y a qu'un porteur, ça lui retirait sa manette.
        //
        // Demander la manette reste possible depuis le menu : c'est un geste,
        // et le détenteur voit qui demande.
        me: { role: "spectator", slot: null },
        players: [],
        openSlots: 0,
        connectionPool: [],
        width: 640,
        height: 480,
    };
}
