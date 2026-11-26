import React from "react";
import { useTranslation } from "react-i18next";
import { COFFEE_URL, EXTERNAL, REPOSITORY_URL } from "../libs/links.js";
import { Note } from "./Note.jsx";

// La version de cette construction (le fichier VERSION ; webpack.config.js).
const VERSION = process.env.GAMEFLEX_VERSION;

// LE PIED DE PAGE, SUR TOUTES LES PAGES (le porteur, 2026-11-14 : « dans le footer, tu mets gameflex, le lien GitHub et
// le lien Buymeacoffee » ; 2026-11-25, chantier 81 : « section guide et footer sur toutes les pages ») : « GameFlex
// (version) · GitHub · Buy me a coffee », en petit, de la couleur secondaire ; deux liens de texte, hors de
// l'application, à la croix comme tout lien. LA VERSION (« n'oublie pas d'afficher la version. GameFlex ({version}) ») :
// celle du fichier VERSION. SettingsPage le pose sous chaque page : aucune page ne l'oublie.
export function Footer() {
    const { t } = useTranslation();

    return (
        <Note className="text-center mt-4 mb-2">
            {VERSION ? `GameFlex (${VERSION})` : "GameFlex"} · <a href={REPOSITORY_URL} {...EXTERNAL} className="text-reset" data-nav data-key="s-footer-github">{t("settings.footerGithub")}</a>
            {" · "}<a href={COFFEE_URL} {...EXTERNAL} className="text-reset" data-nav data-key="s-footer-coffee">{t("settings.footerCoffee")}</a>
        </Note>
    );
}
