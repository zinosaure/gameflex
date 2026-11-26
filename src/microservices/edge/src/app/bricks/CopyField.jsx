import React from "react";
import { useTranslation } from "react-i18next";
import { copyTextToClipboard } from "../libs/share-link.js";
import { toast } from "../shell/Shell.jsx";
import { IconAddon } from "./IconAddon.jsx";
import { TextRow } from "./TextRow.jsx";

// UNE VALEUR À RECOPIER : le champ en lecture seule, en chasse fixe, et l'icône qui copie. EN HTTP AUSSI : sur une adresse
// du réseau local (http://192.168.…), le navigateur n'a pas de presse-papiers (`navigator.clipboard` n'existe qu'en
// contexte sûr) ; `copyTextToClipboard` (libs/share-link.js) prend alors l'ancien chemin. Si rien n'a copié, le dire :
// le champ reste là, sélectionné au toucher, pour copier à la main.
export function CopyField({ id, label = null, help = null, value }) {
    const { t } = useTranslation();
    const copy = async () => toast(t((await copyTextToClipboard(value)) ? "settings.copied" : "settings.copyFailed"));

    return (
        <TextRow id={id} label={label} help={help} value={value} props={{ readOnly: true, onFocus: (event) => event.target.select() }}
            addon={<IconAddon id={`${id}-copy`} icon="bi-clipboard" label={t("settings.copy")} onClick={copy} />} />
    );
}
