import React from "react";
import { MUTED } from "../libs/ui.js";

// Une ligne de texte atténuée, au bord gauche : le nombre trouvé, un renseignement — et ce qui n'a pas pu se lire (une
// erreur se dit là, sans encadré : plus d'`alert` dans le launcher, le porteur, 2026-11-16 : « retirer les alerts »).
export function Note({ className = "mt-3", children }) {
    return <p className={`${MUTED} gf-note mb-0 ${className}`}>{children}</p>;
}
