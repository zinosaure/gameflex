import i18n from 'i18next';
import { initReactI18next } from 'react-i18next';

import en from './locales/en.json';
import fr from './locales/fr.json';

const resources = { en: { translation: en }, fr: { translation: fr } };

/**
 * Initialise i18next avec react-i18next.
 *
 * Anglais par défaut — seule une préférence explicitement enregistrée
 * (sélecteur de langue, localStorage "lang") doit changer la langue
 * affichée. La langue du navigateur n'est volontairement PAS utilisée
 * comme signal : un visiteur avec un système en français ne doit pas se
 * retrouver en français sans l'avoir choisi.
 */
const detectedLang = (() => {
    try {
        return localStorage.getItem('lang');
    } catch {
        return null;
    }
})() || 'en';

i18n.use(initReactI18next).init({
    resources,
    lng: ['fr', 'en'].includes(detectedLang) ? detectedLang : 'en',
    fallbackLng: 'en',
    interpolation: { escapeValue: false },
    react: { useSuspense: false },
});

export default i18n;
