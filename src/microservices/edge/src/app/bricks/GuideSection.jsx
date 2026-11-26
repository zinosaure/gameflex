import React from "react";
import { useTranslation } from "react-i18next";
import { guideUrl } from "../libs/links.js";
import { TextSection } from "./TextSection.jsx";

// LA SECTION « GUIDE » (le porteur, 2026-11-16 : « Rajouter des sections guide avec des liens vers le docs sur le repos »
// ; « Toujours proposer des liens vers la documentation si possible ») : une page qui explique quelque chose renvoie au
// guide du dépôt public qui en parle (libs/links.js, GUIDES), au titre qui en parle quand il y en a un. Sa place : la
// DERNIÈRE de la page, après « Danger » (le porteur, 2026-11-16 : « toujours en dernier. Même après danger. »). `guides` : [[clé, ancre], …], l'ancre facultative.
// UNE SECTION DE TEXTE, PAS UN GROUPE DE LIGNES (TextSection ; le porteur, 2026-11-16 : « C'est pour faire une
// différence. Montrer que c'est un lien de guide. », puis « Enleve l'icône link et enleve les puces. Juste titre et texte
// aussi ») : le titre du groupe avec une icône de documents ; chaque guide, son nom en lien (un autre onglet, EXTERNAL ;
// dans l'application Android, le navigateur du téléphone), son aide dessous (ceux de la page « GameFlex »). La croix
// atteint chaque lien (`data-nav`), A le suit, la lumière se pose sur lui (`is-focus`).
export function GuideSection({ guides }) {
    const { t } = useTranslation();

    return (
        <TextSection title={t("guide.title")} icon="bi-journal-text" help={t("guide.help")}
            items={guides.map(([key, anchor = null]) => ({
                key: `guide-${key}${anchor ? `-${anchor}` : ""}`, title: t(`project.${key}`), text: t(`project.${key}Help`), href: guideUrl(key, anchor),
            }))} />
    );
}
