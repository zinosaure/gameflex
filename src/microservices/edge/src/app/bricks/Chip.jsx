import React from "react";

// La pastille d'une ligne : l'icône dans un carré de la couleur du thème.
export function Chip({ icon, tone = "slate" }) {
    return <span className={`gf-chip tone-${tone}`} aria-hidden="true"><i className={`bi ${icon}`}></i></span>;
}
