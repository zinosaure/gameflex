import React from "react";
import { choiceButton } from "../libs/ui.js";
import { StackRow } from "./StackRow.jsx";

// Un choix de deux à quatre options, toutes visibles : le motif de Bootstrap, `btn-check` et ses étiquettes en
// boutons — la choisie pleine, les autres à contour, partout (`choiceButton`, libs/ui.js). Au-delà de quatre, un
// sélecteur. `options` : [[valeur, libellé], …] ; `isDisabled(valeur)`. La croix s'arrête sur chaque case ; A la
// choisit.
export function ChoiceRow({ id, label, help = null, value, options, onChange, isDisabled = () => false }) {
    return (
        <StackRow label={label} help={help}>
            <div className="btn-group w-100" role="group" aria-label={label}>
                {options.map(([option, text]) => {
                    const key = `${id}-${option || "default"}`;

                    return (
                        <React.Fragment key={option || "default"}>
                            <input type="radio" className="btn-check" name={id} id={key} autoComplete="off" checked={option === value}
                                disabled={isDisabled(option)} onChange={() => onChange(option)} />
                            <label className={`${choiceButton(option === value)} btn-sm`} htmlFor={key} data-nav data-key={key} tabIndex={0}
                                aria-pressed={option === value} onClick={(event) => {
                                    // A à la manette « clique » l'étiquette : la case suit, comme au doigt.
                                    if (event.detail === 0 && !isDisabled(option)) {
                                        event.preventDefault();
                                        onChange(option);
                                    }
                                }}>
                                {text}
                            </label>
                        </React.Fragment>
                    );
                })}
            </div>
        </StackRow>
    );
}
