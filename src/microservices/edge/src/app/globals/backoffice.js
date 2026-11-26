// LES RÉGLAGES OU LE RESTE (chantier 81 ; le porteur, 2026-11-25 : « côté front office seulement, les sous-titres des
// éléments restent comme avant (hormis status) en format textuel ») — défini ICI une fois : SettingsPage pose ce contexte
// autour de ses pages ; une ligne (RowText, InfoRow) n'y met ses valeurs en pastilles que dans les Réglages. Ailleurs (la
// fiche d'un jeu, le menu d'une partie), une valeur reste du texte ; un état coloré (un statut) garde sa pastille.
import { createContext, useContext } from "react";

export const InSettings = createContext(false);
export const useInSettings = () => useContext(InSettings);
