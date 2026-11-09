# Dix interfaces, dix agencements

Cette refonte reprend la planche de dix propositions fournie par la personne, puis ajoute des modes de navigation de frontends d’émulation. Les identifiants sauvegardés restent identiques ; aucune préférence personnelle n’est réinitialisée.

| Choix | Structure | Navigation et interactions |
|---|---|---|
| 01 Minimal | Liste numérotée, petites vignettes, séparateurs fins | Barre d’outils horizontale, lecture dense |
| 02 Jaquettes | Mur d’affiches sur papier clair | Rail vertical fin, affiches sans cartes de tableau de bord |
| 03 Consoles | Carrousel de systèmes, grande photo matérielle, nom de constructeur | Ruban horizontal, sélection d’une machine puis exploration ; étagère de cartouches dans sa bibliothèque |
| 04 Favoris | Une affiche mise en vitrine, mosaïque épinglée séparée | Rail d’icônes, épingles pour les jeux de la bibliothèque |
| 05 Reprise rapide | Axe XMB, liste verticale et aperçu latéral | Catégories en haut ; la sélection et le focus changent l’aperçu |
| 06 Collections | Rayonnages horizontaux, groupes par console ou région | Ruban supérieur, défilement des rayons ; collections personnelles conservées |
| 07 Fiche de jeu | Affiche, grand titre, fond de la jaquette et pellicule de sélection | Navigation de cinéma, sélection séparée de l’action Jouer/Fiche |
| 08 Archives | Arborescence des systèmes, liste de fichiers et inspecteur | Barre d’explorateur, filtres de console et d’état |
| 09 Salon multijoueur | Marquise de borne, sélections numérotées, arêtes orange | Navigation de borne, vignettes et compteurs issus des données réelles |
| 10 Tableau de bord | Tuiles Metro asymétriques, raccourcis et statistiques | Rail compact, regroupement des actions principales |

Les pages console ne reviennent plus à une grille commune : `GameBrowser` choisit un arbre de composants différent pour chacun des dix styles. Chaque console conserve sa photo, « Mes jeux », le catalogue externe, le filtre A–Z et les jaquettes du cache. Les favoris, collections et lancement des jeux locaux gardent leurs callbacks et autorisations. Une carte externe ouvre uniquement sa fiche ; elle n’est pas un jeu installé et ne déclenche aucun téléchargement de ROM.

Le dock mobile remplace la longue barre défilante : cinq cibles tactiles de 44 px dans une barre flottante de 54 px, icônes SVG d’une même famille, libellé pour la destination active. « Plus » donne accès aux autres rubriques, aux paramètres et au sélecteur d’interface. La navigation d’une rubrique à l’autre ne change jamais l’interface sauvegardée. Le tiroir et le dialogue de lancement gèrent le focus et Échap ; les flèches parcourent les éléments réellement affichés.

Le sélecteur présente dix schémas d’agencement distincts, pas dix nuanciers. Les consoles utilisent les photos déjà approuvées ; les jeux utilisent le cache de jaquettes existant. Aucun jeu fictif n’est ajouté à la bibliothèque réelle. Aucun changement système, de réseau ou de TV n’est requis par cette refonte. Pas de fork ni de push.

## Vérification

- Rendu des dix interfaces avec données et sans données ; dix arbres de composants distincts sur la même console, même après suppression des classes et marqueurs de disposition.
- Navigation mobile : cinq boutons SVG, dock mesuré à 54 px, aucune largeur débordante à 430 px, tiroir accessible, navigation vers Archives sans changement de mode.
- Dialogue de lancement testé au clavier dans le navigateur d’aperçu : ouverture, sélection et fermeture par Échap ; aucun lancement réel de session pendant l’aperçu.
- Les pages d’aperçu utilisent le composant React réel, des métadonnées Planetemu et les images déjà en cache. La seule démo locale présentée est Palette libre ; ces aperçus n’accèdent pas au compte de la personne.
- Aucune connexion à la TV. Validation matérielle Android TV à effectuer ultérieurement lorsque la personne la rendra disponible.
