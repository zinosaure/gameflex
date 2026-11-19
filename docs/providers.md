> État actuel : EmuGen est retiré. Les autres fournisseurs servent uniquement aux catalogues et aux jaquettes. Voir [Imports et sources](imports-et-sources.md).

# Fournisseurs de GameFlex

Dans **Paramètres → Fournisseurs**, EmuGen figure en premier avec le badge
**Prioritaire**. Les interrupteurs activent/désactivent chaque fournisseur.
Le réglage est commun au serveur et sa modification est réservée à
l’administrateur ; les autres comptes peuvent le consulter.

Par défaut, seul **EmuGen** est actif. Planetemu, FreeROMs, ROMsPure et le catalogue
GameFlex de démonstration restent disponibles dans les paramètres, en sourdine.
Les jeux déjà installés restent dans la bibliothèque, quelle que soit leur source.

Les changements sont enregistrés immédiatement dans
`.local/pi-tv/provider-settings.json` (volume `/data` du service Docker). Le
catalogue est actualisé après un changement ; les autres onglets/appareils
relisent le réglage au retour au premier plan ou toutes les trente secondes.

Les fournisseurs désactivés ne sont pas interrogés pour la recherche, le repli de
métadonnées ni un nouveau téléchargement. Les essais de repli déjà en file sont
contrôlés à nouveau avant leur transfert. Une désactivation ne supprime ni les
ROMs personnelles ni les corrections de métadonnées. Les jaquettes déjà en cache
restent affichables ; une image manquante n’entraîne pas de requête vers un
fournisseur en sourdine. Désactiver EmuGen ferme aussi sa session interactive.
Un transfert déjà commencé n’est pas annulé rétroactivement.

Avec EmuGen seul, une recherche ne lance plus les vingt-sept pages alphabétiques
des autres fournisseurs. La recherche publique EmuGen reste en HTTP ; Chromium
sert uniquement au parcours de sources/vérification déjà documenté dans
[EmuGen](emugen-manual.md).

API authentifiée :

- `GET /experience-api/v1/game-sources` : fournisseurs ordonnés, `enabledIds`,
  `enabled` pour chacun et `canConfigure` pour le compte courant.
- `PUT /experience-api/v1/game-sources`, corps `{"enabledIds":["emugen"]}` :
  administrateur uniquement ; liste vide acceptée, identifiants inconnus et
  doublons refusés. EmuGen garde toujours la première position.

Les téléchargements de jeux commerciaux ne sont pas effectués pendant cette
configuration. Utiliser **Mes jeux → Ajouter une ROM** pour importer ses fichiers
personnels et les classer dans le dossier de bibliothèque configuré.

Validation sur le Pi ARM64 : 107 tests Python passent, ainsi que les tests de
recherche et de fiche React. Compilation réussie et lint ciblé sans erreur.
Après déploiement sur le port 2000, l’API authentifiée confirme EmuGen seul actif,
les autres catalogues désactivés et le réglage persistant. La recherche publique
EmuGen « Pokemon » sur GBA renvoie 13 fiches. Aucun fichier de jeu téléchargé.

## Filtre alphabétique

Sans lettre sélectionnée, la console montre tous ses jeux disponibles, triés
alphabétiquement après fusion des sources. Un clic sur une lettre applique le
filtre ; un deuxième clic sur cette lettre l’enlève. Le bouton `0–9` suit le même
comportement. Entrer dans une autre console rétablit la vue sans filtre.
La recherche textuelle continue de porter sur toute la console.

Le catalogue EmuGen est chargé progressivement, par pages de métadonnées de
100 titres (30 pour une recherche textuelle), jusqu’à sa fin. L’API accepte
`letter=` et `page=1`, puis renvoie `nextPage`. Cela enlève la limite implicite
ancienne aux trois premières pages. Les pages déjà lues sont réutilisées en cache.
Le signal de fin explicite WordPress est distingué d’une panne réseau ; voir
[le contrôleur REST officiel](https://developer.wordpress.org/reference/classes/wp_rest_posts_controller/get_items/).
