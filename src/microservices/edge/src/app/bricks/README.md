# Les briques de l'interface

Une brique par fichier (chantier 81). Les règles qui valaient pour toutes, recopiées de l'en-tête de l'ancien
`ui/Bricks.jsx` ; celles du chantier 81 sont dans `docs/roadmap/chantier-81-refonte-des-pages.md` (§0).

```
LES BRIQUES DU LAUNCHER — la direction C (« tuiles et groupes »), choisie et validée par le porteur le
2026-11-13 (docs/reports/2026-11-13-launcher-modeles/composants.html). Définies UNE fois, ici ; le menu de la
partie, les réglages d'un jeu et chaque page des Réglages s'en composent, et rien d'autre ne dessine un groupe,
une ligne, une tuile ou un geste destructif à sa façon.

LES RÈGLES, mesurées sur les modèles :
  - un seul bord gauche : le rond de Retour, les onglets, la recherche, les tuiles, les intitulés, les groupes,
    les textes et les boutons commencent sur la même verticale ; le titre suit le rond sur sa ligne, le fil
    d'Ariane sous le titre, plus petit (PageHeader) ;
  - `mb-3` sous chaque intitulé de groupe (sous son aide quand il en a une), `mt-4` entre deux groupes ;
  - un groupe est un conteneur arrondi (`Rows`), et il a TOUJOURS un titre quand il en existe un honnête (le
    porteur, 2026-11-13 : « chaque section doit toujours avoir un titre si possible ») ; ses lignes s'ouvrent
    (NavRow ; SplitRow quand un geste se pose à leur bord droit), se règlent sur place (SwitchRow, SelectRow, RangeRow, ChoiceRow, TextRow) ou renseignent (InfoRow : le
    libellé, sa valeur dessous, à gauche — le porteur, 2026-11-16, « Au lieu de les mettre en 2 colonnes ») ;
  - LE BORD DROIT D'UNE LIGNE DIT CE QU'ELLE FAIT (NavRow, `opens`) : le chevron pour une page, « ? » pour une
    question, « i » pour un renseignement, l'icône de son geste ou rien (le porteur, 2026-11-14) ;
  - L'ORDRE D'UNE PAGE : ses listes d'abord, puis « Plus », puis « Danger », puis « Bon à savoir » et « Guide », TOUJOURS
    en dernier (le porteur, 2026-11-14 : « Les sections sont toujours après les listings » ; 2026-11-16 : « la section
    bon à savoir et guide sont toujours en dernier. Même après danger. ») ; « Guide » renvoie aux documents du dépôt public, en section
    de texte (titres en lien, texte dessous) et non en lignes (GuideSection, TextSection) ;
  - PAS D'ALERTE (le porteur, 2026-11-16 : « retirer les alerts ») : un renseignement est une ligne d'une section
    d'informations (InfoRow) ou l'aide d'un groupe (`help`), une erreur une ligne atténuée (Note) ;
  - UN GROUPE QUI SERT DE FORMULAIRE N'A PAS DE TRAITS, ses lignes espacées d'un mb-3, et ses interrupteurs n'y ont
    pas d'icône ; un groupe de liens a ses traits d'un bord à l'autre (le porteur, 2026-11-14 ; Rows) ;
  - LE BOUTON D'UN GROUPE EST DANS SON ENCADRÉ, après son dernier contrôle (SubmitRow ; le porteur, 2026-11-13 :
    « le bouton doit être dans la section card avec les autres form controls ») ;
  - UN GESTE QUI N'APPARTIENT À AUCUN GROUPE EST UNE LIGNE, pas un bouton (« Les boutons orphelins […] ne doivent
    pas être des boutons mais des liens ») : ce qui détruit dans la section « Danger » (DangerSection, DangerRow),
    le reste dans le groupe « Plus » ; grisée quand le geste n'est pas disponible, sa raison en sous-titre ;
  - TOUTE CONFIRMATION EST LA FENÊTRE DE SKELETON (useConfirm ; « utilise skeleton.Feedback pour les fenêtres de
    confirmation partout dans le launcher ») ;
  - LES BOUTONS SONT PLEINS, et leur couleur vient de leur sorte et de leur lieu : primary pour valider et danger
    dans les Réglages ; sur les écrans, success pour le geste principal et les ronds, les autres couleurs gardées
    (secondary, warning, danger) ; un choix segmenté primary et outline-primary partout (libs/ui.js, `useButton()` ;
    le porteur, 2026-11-14) ;
  - LE BOUTON D'UN FORMULAIRE EST FormButton (Forms.jsx) : « ••• » tant que rien n'a changé, une ligne au-dessus ;
  - UN BOUTON PORTE SOIT UN TEXTE, SOIT UNE ICÔNE, JAMAIS LES DEUX (le porteur, 2026-11-13) : les gestes sont
    des boutons de texte ; un bouton accolé à un champ n'est qu'une icône (IconAddon), son nom en aria-label et
    en title. Les lignes et les tuiles, qui ne sont pas des boutons, gardent icône et libellé ;
  - une ligne EMPILÉE (un sélecteur, un champ, une glissière, un choix) n'a pas d'icône : son contrôle prend la
    largeur ;
  - boutons et champs à 6 px (bootstrap.scss, 60fd09c5), tuiles comprises — une tuile est un bouton ; les
    conteneurs de groupe sont arrondis à 16 px (launcher.css, `.gf-list`).
```
