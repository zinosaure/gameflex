import React from "react";

// LE BOUTON D'UN GROUPE, dans son encadré, après son dernier contrôle (le porteur, 2026-11-13) : la dernière ligne de
// `Rows`, qui porte le FormButton du groupe — et fait de ce groupe un formulaire (Rows).
export function SubmitRow({ children }) {
    return <div className="list-group-item gf-row gf-row-submit">{children}</div>;
}
