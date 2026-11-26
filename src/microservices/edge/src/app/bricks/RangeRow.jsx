import React from "react";
import { fieldContext, Form } from "../ui/Forms.jsx";
import { StackRow } from "./StackRow.jsx";

// Une glissière (Form.Range), sa valeur lue à côté ; GAUCHE et DROITE la déplacent d'un pas.
export function RangeRow({ id, label, help = null, value, min, max, step = 1, onChange, disabled = false }) {
    return (
        <StackRow label={label} help={help}>
            <Form.Range name={id} context={fieldContext(id, value, onChange, disabled)} min={min} max={max} step={step}
                props={{ "data-nav": true, "data-key": id, "aria-label": label }} />
        </StackRow>
    );
}
