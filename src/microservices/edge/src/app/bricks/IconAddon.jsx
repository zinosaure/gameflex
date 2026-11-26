import React from "react";
import { useButton } from "../libs/ui.js";

// UN BOUTON ACCOLÉ À UN CHAMP : l'icône seule, son nom en aria-label et en title (la règle des boutons).
export function IconAddon({ id, icon, label, kind = "other", disabled = false, onClick }) {
    const button = useButton();

    return (
        <button type="button" className={`${button(kind)} flex-shrink-0`} data-nav data-key={id} aria-label={label} title={label} disabled={disabled} onClick={onClick}>
            <i className={`bi ${icon}`}></i>
        </button>
    );
}
