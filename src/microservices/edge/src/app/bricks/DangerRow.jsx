import React from "react";
import NavRow from "./NavRow.jsx";
import { useConfirm } from "./useConfirm.jsx";

// UN GESTE QUI DÉTRUIT : une ligne rouge (une NavRow) de la section « Danger », qui ouvre la fenêtre de confirmation
// de Skeleton — « ? » à son bord droit (`opens="confirm"`) ; grisée quand le geste n'est pas disponible, sa raison en
// `help`. `question` : le texte de la
// fenêtre, son titre étant le geste lui-même (`label`).
// `pending` : le geste confirmé est en cours — la pastille devient la roue, comme toute NavRow.
export function DangerRow({ id, icon = "bi-trash", label, help = null, question, confirmLabel = null, disabled = false, pending = false, onConfirm }) {
    const confirm = useConfirm();

    return (
        <NavRow id={`${id}-ask`} icon={icon} tone={disabled ? "slate" : "rose"} label={label} help={help} danger={!disabled} disabled={disabled} pending={pending} opens="confirm"
            onOpen={() => confirm({ title: label, message: question, confirmLabel: confirmLabel || label, onConfirm })} />
    );
}
