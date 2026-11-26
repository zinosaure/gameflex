import React from "react";
import { Form } from "../ui/Forms.jsx";
import { StackRow } from "./StackRow.jsx";

// UNE ADRESSE RÉSEAU : [protocole | serveur] [port] sur une ligne (le porteur, 2026-11-20 : « des input group. Et flex les
// 2 inputs sur la même ligne »), chaque champ avec sa règle (Ruleset du contexte, par son nom). Le protocole est une
// étiquette de l'input-group de Bootstrap, ou un sélecteur quand il y en a plusieurs (`schemes`) ; le port, vide, prend
// celui du protocole (`portPlaceholder`). Le champ de Skeleton est enveloppé (Form.Feedback) : .gf-input-group range
// l'enveloppe comme un enfant direct de l'input-group (launcher.css).
export function AddressRow({ id, portId, label, help = null, context, schemes, scheme, onScheme, portLabel, portPlaceholder = "" }) {
    return (
        <StackRow label={label} help={help}>
            <div className="d-flex gap-2 align-items-start">
                <div className="input-group flex-nowrap gf-input-group flex-grow-1 min-w-0">
                    {schemes.length > 1 ? (
                        <select className="form-select flex-grow-0 flex-shrink-0 w-auto" value={scheme} aria-label={label} data-nav data-key={`${id}-scheme`}
                            onChange={(event) => onScheme(event.target.value)}>
                            {schemes.map((option) => <option key={option} value={option}>{`${option}://`}</option>)}
                        </select>
                    ) : <span className="input-group-text">{`${scheme}://`}</span>}
                    <Form.Text name={id} context={context} props={{ "data-nav": true, "data-key": id, autoComplete: "off", autoCapitalize: "off", spellCheck: false, "aria-label": label }} />
                </div>
                <div className="flex-shrink-0 gf-port">
                    <Form.Text name={portId} context={context} props={{ "data-nav": true, "data-key": portId, inputMode: "numeric", autoComplete: "off", placeholder: portPlaceholder, "aria-label": portLabel }} />
                </div>
            </div>
        </StackRow>
    );
}
