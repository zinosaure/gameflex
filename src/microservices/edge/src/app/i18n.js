import i18n from "i18next";
import { initReactI18next } from "react-i18next";
// LES TEXTES D'EDGE, et ceux de la webapp que lit son code rapatrié (le moteur de la page de jeu, la manette à
// l'écran, les erreurs de connexion, les étapes de lancement) : versés ici au chantier 56 — l'espace `webapp` qu'ils
// formaient, lu seulement quand edge n'avait pas la clé, n'existe plus.
import fr from "./locales/fr.json";
import en from "./locales/en.json";

// La langue de la page : `?lang=fr|en`, sinon celle choisie dans les réglages (gardée
// dans CE navigateur, rien n'est écrit sur le serveur), sinon celle du navigateur.
const LANGUAGE_KEY = "gameflex-edge-language";

const initialLanguage = () => {
    const forced = new URLSearchParams(location.search).get("lang");

    if (forced === "fr" || forced === "en") {
        return forced;
    }

    try {
        const saved = localStorage.getItem(LANGUAGE_KEY);

        if (saved === "fr" || saved === "en") {
            return saved;
        }
    } catch {
        // Sans stockage, la langue du navigateur.
    }

    return (navigator.language || "fr").toLowerCase().startsWith("fr") ? "fr" : "en";
};

// UNE CLÉ ABSENTE DES DEUX LANGUES, au serveur de développement : dite dans la console, avec son nom — les clés
// calculées (un gabarit, `play.${mode}`) échappent à l'audit statique (tools/audit/i18n.js), et à l'écran une clé absente
// s'affiche telle quelle. La construction de production n'en dit rien.
const reportMissing = process.env.NODE_ENV !== "production";

i18n.use(initReactI18next).init({
    resources: { fr: { translation: fr }, en: { translation: en } },
    lng: initialLanguage(),
    fallbackLng: "en",
    interpolation: { escapeValue: false },
    saveMissing: reportMissing,
    missingKeyHandler: (languages, namespace, key) => console.warn(`[i18n] clé absente : ${key}`),
});

document.documentElement.lang = i18n.language;

export const setLanguage = (language) => {
    i18n.changeLanguage(language);
    document.documentElement.lang = language;

    try {
        localStorage.setItem(LANGUAGE_KEY, language);
    } catch {
        // Sans stockage, le choix ne vaut que pour cette page.
    }
};

export default i18n;
