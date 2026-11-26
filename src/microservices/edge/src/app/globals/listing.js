// UNE LIGNE DE LISTING N'A PAS DE COLONNE D'ICÔNE (chantier 81 ; le porteur, 2026-11-25 : « Dans le listing, enlève la
// colonne de gauche (celle des icônes) excepté pour le listing des jeux les pochettes. En gros les éléments du listing
// deviennent qu'avec 2 colonnes et pour les jeux tu rajoutes dans la colonne de gauche un flex de 2 colonnes. ») —
// défini ICI une fois : Listing (bricks/) pose ce contexte autour de ses lignes, et chaque ligne (NavRow, InfoRow,
// SplitRow, EmptyRow, PlaceholderRow) y laisse sa pastille. UN LISTING EST UNE LISTE QUI SE CHERCHE (le porteur, 2026-11-25,
// sur les fournisseurs de métadonnées : « comment ça se fait que les éléments soient des éléments de listing ? […] C'est
// possible de mettre avec des icônes vu que ce n'est pas des éléments du listing ? ») : une liste feuilletée sans
// recherche (`Listing` sans `onSearch` : les services, les sources, les emplacements) garde ses pastilles. Deux colonnes : le texte, puis ce que la ligne fait. Une
// image (`lead` : une pochette, une vignette) reste, avec le texte, dans la colonne de gauche (`.gf-row-main`).
import { createContext, useContext } from "react";

export const InListing = createContext(false);
export const useInListing = () => useContext(InListing);
