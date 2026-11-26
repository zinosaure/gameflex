import React from "react";
import { Chip } from "./Chip.jsx";
import { RowText } from "./RowText.jsx";
import { useInListing } from "../globals/listing.js";

// UNE LISTE VIDE GARDE SA BOÎTE (le porteur, 2026-11-14 : « Pour toutes les listes vides, il faut afficher au moins un
// lien (un placeholder) grisé avec le message. Sinon ça fait trop moche comme ça. ») : UNE ligne grisée, la forme d'une
// ligne de la liste — sa pastille, le message —, désactivée : ni bouton, ni `data-nav` (la lumière de la croix ne s'y
// pose pas), rien à son bord droit. « Aucun jeu. », ou ce qu'une recherche n'a pas trouvé. Définie ICI une fois : la
// dernière ligne d'une Listing vide, ou EmptyState pour une liste qui ne se feuillette pas.
export function EmptyRow({ icon = "bi-inbox", text }) {
    return (
        <div className="list-group-item gf-row gf-row-empty" aria-disabled="true">
            {!useInListing() && <Chip icon={icon} tone="slate" />}
            <RowText label={text} />
        </div>
    );
}
