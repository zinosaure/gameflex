import React from "react";

// L'AVATAR d'un compte, à la place de la pastille d'une ligne : ses initiales dans un rond.
export function Avatar({ name }) {
    const initials = (name || "").split(/\s+/).filter(Boolean).slice(0, 2).map((word) => word[0].toUpperCase()).join("") || "?";

    return <span className="gf-avatar" aria-hidden="true">{initials}</span>;
}
