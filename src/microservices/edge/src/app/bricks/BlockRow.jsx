import React from "react";

// UNE LIGNE LIBRE dans un groupe (une scène, un calendrier) : le contenu sur toute la largeur, sans pastille.
export function BlockRow({ children }) {
    return <div className="list-group-item gf-row gf-row-stack">{children}</div>;
}
