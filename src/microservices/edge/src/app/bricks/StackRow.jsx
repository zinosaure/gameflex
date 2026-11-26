import React from "react";
import { helpLines } from "../globals/subtitles.jsx";
import { Form } from "../ui/Forms.jsx";

// UNE LIGNE EMPILÉE : Form.Group de Skeleton (le libellé, l'aide) puis le contrôle sur toute la largeur, et son
// bouton accolé (`addon`, un IconAddon) à côté. Pas d'icône.
export function StackRow({ label, help, addon = null, children }) {
    return (
        <div className="list-group-item gf-row gf-row-stack">
            <Form.Group label={label} helpText={helpLines(help)} props={{ className: "mb-0 w-100" }}>
                {addon ? <div className="d-flex gap-2 align-items-start"><div className="flex-grow-1 min-w-0">{children}</div>{addon}</div> : children}
            </Form.Group>
        </div>
    );
}
