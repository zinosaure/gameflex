import React from "react";
import { fieldContext, Form } from "../ui/Forms.jsx";
import { StackRow } from "./StackRow.jsx";

// Un sélecteur (Form.Select) : un choix parmi plusieurs, un filtre aussi. À la manette, GAUCHE et DROITE passent à
// l'option suivante. `options` : [{ value, label, attributes }] comme Skeleton les lit.
export function SelectRow({ id, label, help = null, value, options, onChange, disabled = false, addon = null }) {
    return (
        <StackRow label={label} help={help} addon={addon}>
            <Form.Select name={id} context={fieldContext(id, value, onChange, disabled)} options={options}
                props={{ "data-nav": true, "data-key": id, "aria-label": label }} />
        </StackRow>
    );
}
