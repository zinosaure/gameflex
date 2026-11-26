import React, { useEffect, useRef } from "react";
import { helpLines, SUBTITLES } from "../globals/subtitles.jsx";
import { fieldContext, Form, PadReach } from "../ui/Forms.jsx";
import { StackRow } from "./StackRow.jsx";

// Un choix dont les options se lisent (une phrase, une aide sous chacune) : LES CARTES D'OPTION DE SKELETON
// (Form.SkeletonRadio — le porteur, 2026-11-16 : « dans choix des flux, change le radio button en skeleton.SkeletonRadio »),
// l'une sous l'autre, chacune son icône, son libellé et son aide. Skeleton tient l'option choisie par son libellé : la
// valeur s'y traduit ici, une fois. La croix les parcourt (PadReach), A en choisit une ; chaque carte reçoit sa clé
// (`<id>-<rang>`), pour que la lumière arrive sur celle qui est choisie (SidePanel, `firstKey`). `options` : [{ value,
// label, help, icon }].
export function RadioRow({ id, label = null, help = null, value, options, onChange, disabled = false, separator = SUBTITLES.separator, noKey = SUBTITLES.noKey }) {
    const box = useRef(null);
    const chosen = options.find((option) => option.value === value);
    const pick = (title) => {
        const picked = options.find((option) => option.label === title);

        if (picked) {
            onChange(picked.value);
        }
    };

    useEffect(() => {
        box.current?.querySelectorAll("button").forEach((card, index) => card.setAttribute("data-key", `${id}-${index}`));
    });

    return (
        <StackRow label={label} help={help}>
            <PadReach>
                <div ref={box}>
                    <Form.SkeletonRadio name={id} context={fieldContext(id, chosen ? chosen.label : null, pick, disabled)}
                        options={options.map((option) => ({ icon: `bi ${option.icon || "bi-circle"}`, title: option.label, subtitle: helpLines(option.help, Infinity, separator, noKey) || "" }))} />
                </div>
            </PadReach>
        </StackRow>
    );
}
