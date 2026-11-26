import React from "react";
import { fieldContext, Form, helpOrReason, NewPassword } from "../ui/Forms.jsx";
import { StackRow } from "./StackRow.jsx";

// Un champ de formulaire (Form.Text, Form.Password, Form.IntFloat, Form.Longtext) dans le contexte d'un formulaire de
// Skeleton (`context`, formContext) ou seul (`value` + `onChange`). `kind` : text, password, new-password (un mot de passe
// choisi, sa jauge et la règle : NewPassword, chantier 55), number, longtext. Désactivé par son Ruleset, il dit pourquoi à
// la place de son aide (helpOrReason).
export function TextRow({ id, label, help = null, context = null, value = "", onChange = () => {}, kind = "text", props = {}, addon = null, min, max, step }) {
    const own = context || fieldContext(id, value, onChange);
    const field = { "data-nav": true, "data-key": id, autoComplete: "off", "aria-label": label, ...props };

    return (
        <StackRow label={label} help={helpOrReason(own, id, help)} addon={addon}>
            {kind === "password" ? <Form.Password name={id} context={own} props={field} />
                : kind === "new-password" ? <NewPassword name={id} context={own} props={field} />
                    : kind === "number" ? <Form.IntFloat name={id} context={own} min={min} max={max} step={step} props={field} />
                        : kind === "longtext" ? <Form.Longtext name={id} context={own} props={field} />
                            : <Form.Text name={id} context={own} props={field} />}
        </StackRow>
    );
}
