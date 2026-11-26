import React from "react";
import { useInListing } from "../globals/listing.js";

// UNE LIGNE FANTÔME, le temps qu'une liste se lise (Listing) : la forme d'une ligne — la pastille grise, le libellé,
// une aide plus courte, le chevron —, en `placeholder` de Bootstrap dans une ligne `placeholder-glow`, comme la Table de
// Skeleton dessine les siennes (skeleton/libs/Table.jsx) : la hauteur ne saute pas quand la liste arrive. `lead` : la
// tête de la vraie ligne — "chip" (une pastille), "cover" (une jaquette), "thumb" (une vignette d'état), "avatar", ou
// null ; `help` : la ligne d'aide ; `chevron` : false pour une ligne de renseignement (InfoRow). RIEN NE S'Y TOUCHE : ni
// bouton, ni `data-nav` — la lumière de la croix ne s'y pose pas.
export function PlaceholderRow({ lead = "chip", help = true, chevron = true }) {
    // Dans un listing (globals/listing.js), ni pastille ni avatar : seules les images gardent leur place.
    const listed = useInListing();

    return (
        <div className="list-group-item gf-row gf-row-placeholder placeholder-glow" aria-hidden="true">
            {lead === "cover" ? <span className="gf-cover rounded placeholder"></span>
                : lead === "thumb" ? <span className="thumb ratio ratio-4x3 rounded placeholder"></span>
                    : listed ? null
                        : lead === "avatar" ? <span className="gf-avatar placeholder"></span>
                        : lead ? <span className="gf-chip tone-slate placeholder"></span> : null}
            <span className="gf-row-text">
                <span className="gf-row-label"><span className="placeholder col-7"></span></span>
                {help && <small className="gf-row-help"><span className="placeholder col-4"></span></small>}
            </span>
            {chevron && <i className="bi bi-chevron-right gf-row-chevron" aria-hidden="true"></i>}
        </div>
    );
}
