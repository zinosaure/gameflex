import React, { useState } from "react";
import Skeleton from "../../../../skeleton/skeleton.js";
import { FormButton } from "../../../shared/templates/Essentials.jsx";

const Form = Skeleton.Form;

// LE CLAVIER DU TÉLÉPHONE, DANS LE MENU (#420, le porteur, 2026-10-29 : « je
// n'ai pas de clavier qui s'affiche pour entrer le nom depuis l'émulateur »).
//
// Un champ du téléphone, qui ouvre SON clavier, et un bouton qui tape ce
// qu'on y a écrit dans le jeu, puis Entrée — comme un clavier branché. Le
// premier besoin est la fenêtre « Create Profile » de Xenia, qui ne se remplit
// qu'au clavier ; tout autre champ de texte d'un émulateur en profite.
//
// Le menu se ferme d'abord : on voit le texte arriver.
export function KeyboardPanel({ typeText, context, t }) {
    const [text, setText] = useState("");

    const send = () => {
        const typed = text;
        setText("");
        context.offcanvas.close();
        typeText(typed).then(({ missing }) => {
            if (missing.length > 0) {
                context.toast.open(t("streaming.settings.keyboard.missing", { characters: missing.join(" ") }),
                    { variant: "warning" });
            }
        });
    };

    return (
        <div className="pt-2 mt-2 border-top">
            <Form.Group label={t("streaming.settings.keyboard.textLabel")} helpText={t("streaming.settings.keyboard.textHelp")}>
                <Form.Text
                    name="typedText"
                    context={{
                        values: { typedText: text },
                        setValues: (values) => setText(values.typedText ?? ""),
                        files: {},
                        setFiles: () => { },
                        rulesets: {},
                    }}
                    props={{
                        autoFocus: true,
                        autoComplete: "off",
                        autoCapitalize: "off",
                        spellCheck: false,
                        onKeyDown: (event) => event.key === "Enter" && text && send(),
                    }}
                />
            </Form.Group>
            <FormButton callback={send} label={t("streaming.settings.keyboard.send")} isDisabled={!text} />
        </div>
    );
}
