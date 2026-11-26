import React from "react";

// L'INSIGNE DE L'ADMINISTRATION, défini ICI une fois : à côté du nom d'un administrateur (AccountName), et à côté du
// titre d'un groupe des Réglages qu'un administrateur seul voit — « Le serveur », « Les personnes et le partage » (le
// porteur, 2026-11-16 : « rajouter Icône patch fill à côté du texte le serveur et personnes et partage pour faire savoir
// que c'est de l'admin »). `label` : ce qu'il dit à un lecteur d'écran, et dans sa bulle.
export function AdminMark({ label, className = "" }) {
    return <i className={`bi bi-patch-check-fill text-success ${className}`} role="img" aria-label={label} title={label}></i>;
}
