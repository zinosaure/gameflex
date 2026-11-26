import React from "react";
import { Form, PadReach } from "../ui/Forms.jsx";
import { StackRow } from "./StackRow.jsx";

// UN CHOIX DE FICHIER : Form.File de Skeleton (sa copie), dans une ligne empilée de son groupe — sa zone de choix, que
// Skeleton dessine seul. `upload.context` : le contexte de formulaire où il range le fichier (Invitation.jsx, l'image
// d'un code QR). Les images ne s'envoient plus : elles se choisissent dans une recherche (chantier 53, P9).
export function FileRow({ id, label, help = null, upload, accept = "image/*", maxFileSize = 16 * 1024 * 1024 }) {
    return (
        <StackRow label={label} help={help}>
            <PadReach>
                <Form.File name={id} context={upload.context} accept={accept} maxFileSize={maxFileSize} />
            </PadReach>
        </StackRow>
    );
}
