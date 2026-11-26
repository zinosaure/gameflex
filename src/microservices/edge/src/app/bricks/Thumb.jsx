import React from "react";

// UNE VIGNETTE EN TÊTE D'UNE LIGNE, à la place de la pastille (une image d'une console ou d'un jeu, une image proposée
// par une recherche ; chantier 53, P9) : 56 px au format 4:3, la vignette d'un état (`.thumb`, Mes sauvegardes) ; sans
// adresse, l'icône d'une image. `fit` : `cover` pour un fond, `contain` pour une jaquette ou un logo. Une image d'un
// fournisseur se charge chez lui, sans dire d'où vient la page (`referrerPolicy`).
export function Thumb({ url = null, fit = "contain" }) {
    return (
        <span className="thumb ratio ratio-4x3 rounded overflow-hidden bg-body-secondary flex-shrink-0">
            {url ? <img className={`object-fit-${fit}`} src={url} alt="" loading="lazy" referrerPolicy="no-referrer" />
                : <span className="d-flex align-items-center justify-content-center text-body-tertiary"><i className="bi bi-image" aria-hidden="true"></i></span>}
        </span>
    );
}
