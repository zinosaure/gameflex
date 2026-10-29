import React from "react";
import { isRemoteSource } from "../libs/localLibrary.js";
import { ListElement } from "../shared/templates/Essentials.jsx";
import { GameStatusBadge } from "./GameStatus.jsx";
import { formatSize, ORIGIN_ICON } from "./helpers.js";

export function GameCard({ rom, t, onClick }) {
    // PLUS DE BARRE DE TÉLÉCHARGEMENT SUR LA CARTE (chantier 14) : un jeu ne
    // descend plus d'ici pour être joué, et un téléchargement passe par le
    // navigateur, qui montre sa propre barre.

    // On n'annonce un compte de fichiers que si on en a compté.
    //
    // Deux sources rendent une liste vide sans que ce soit une erreur : un
    // serveur distant (il assemble un jeu multi-fichiers en un seul envoi)
    // et la bibliothèque du téléphone (le scanner rend un jeu, pas ses
    // pièces). « 0 fichier » y serait faux autant qu'inutile — et c'est la
    // même correction pour les deux, donc une seule règle plutôt qu'un cas
    // par origine.
    // « Local » plutôt qu'un compte de fichiers pour ce qui est ICI.
    //
    // « file » ne disait rien à personne : c'est le nombre de morceaux du
    // jeu, une information de rangement, et elle occupait la place de la
    // seule qu'on cherche des yeux dans une liste mêlée — d'où vient ce
    // jeu (demandé le 2026-09-01).
    const parts = [];
    parts.push(formatSize(rom.sizeBytes));

    // (rom.files || []) : a folder game with no files serializes as a null
    // list — never crash the whole list on one odd entry.
    if (isRemoteSource(rom.kind) && (rom.files || []).length > 0) {
        parts.push(t("retrogaming.files", { count: rom.files.length }));
    }

    // PLUS DE « N SOURCES » : une ligne EST une copie.
    //
    // Ce compteur répondait à « ce jeu existe-t-il ailleurs si celui d'ici ne
    // démarre pas ? » (2026-09-07), du temps où la liste repliait les copies
    // d'un même titre. La source se choisit désormais en haut de la liste :
    // chaque ligne vient d'un seul endroit, et l'icône le dit déjà.
    const details = parts.join(" · ");

    return (
        <ListElement
            onClick={onClick}
            // Une icône différente pour ce qui n'est pas là : la première
            // chose qu'on regarde dans une liste est l'icône, pas le texte
            // sous le titre.
            icon={isRemoteSource(rom.kind)
                ? (ORIGIN_ICON[rom.kind] || "bi bi-hdd")
                : ((rom.files || []).length > 1 ? "bi bi-collection-play" : "bi bi-disc")}
            // LE STATUT, à côté du titre (chantier 30) : ce qu'on cherche des
            // yeux dans une liste, « est-ce que ça joue ? ».
            label={<>{rom.title}<GameStatusBadge status={rom.status} t={t} /></>}
            helpText={details}
        />
    );
}

