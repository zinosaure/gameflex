import React from "react";
import { EmptyRow } from "./EmptyRow.jsx";
import { Rows } from "./Rows.jsx";

// RIEN À MONTRER, dans une liste qui n'est pas une Listing (la liste des sauvegardes d'un jeu, les personnes d'une
// partie) : sa boîte et sa ligne grisée (EmptyRow). Le geste qui y remédie est une ligne de son groupe, pas un bouton.
export function EmptyState({ icon = "bi-inbox", text }) {
    return <Rows><EmptyRow icon={icon} text={text} /></Rows>;
}
