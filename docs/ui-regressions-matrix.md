# Matrice des 22 interfaces

✓ : Paramètres → activation → accueil → Mes jeux/recherche → fiche → retour avec filtre/focus → Archives → Paramètres → rechargement → Compte et bibliothèque. Fermeture du sélecteur sans choix également vérifiée.

Les trois formats ont aussi un audit de palette : comparaison avec les styles d’accueil d’origine, maintien des dégradés, rubrique commune, fiche et paramètres.

| Interface | Identifiant | PC | Mobile tactile | TV navigateur | Blocage du parcours |
|---|---|---|---|---|---|
| Horizon | `station` | ✓ | ✓ | ✓ | Aucun |
| Signal CRT | `crt` | ✓ | ✓ | ✓ | Aucun |
| La collection | `shelf` | ✓ | ✓ | ✓ | Aucun |
| Orbitale | `orbit` | ✓ | ✓ | ✓ | Aucun |
| Borne Arcade | `arcade` | ✓ | ✓ | ✓ | Aucun |
| Minimal | `minimal` | ✓ | ✓ | ✓ | Aucun |
| Jaquettes | `covers` | ✓ | ✓ | ✓ | Aucun |
| Consoles | `consoles` | ✓ | ✓ | ✓ | Aucun |
| Tableau de bord | `dashboard` | ✓ | ✓ | ✓ | Aucun |
| Hub Canapé | `hub-canape` | ✓ | ✓ | ✓ | Aucun |
| Ligne Cinéma | `ligne-cinema` | ✓ | ✓ | ✓ | Aucun |
| Mosaïque Joy | `mosaique-joy` | ✓ | ✓ | ✓ | Aucun |
| Roue Retro | `roue-retro` | ✓ | ✓ | ✓ | Aucun |
| Terminal de poche | `terminal-poche` | ✓ | ✓ | ✓ | Aucun |
| Cartouches | `cartouches` | ✓ | ✓ | ✓ | Aucun |
| Carte des époques | `carte-epoques` | ✓ | ✓ | ✓ | Aucun |
| Double écran | `double-ecran` | ✓ | ✓ | ✓ | Aucun |
| Club Local | `club-local` | ✓ | ✓ | ✓ | Aucun |
| Quai des systèmes | `quai-systemes` | ✓ | ✓ | ✓ | Aucun |
| Magazine rétro | `magazine-retro` | ✓ | ✓ | ✓ | Aucun |
| Bureau du collectionneur | `bureau-collectionneur` | ✓ | ✓ | ✓ | Aucun |
| Grand écran accessible | `grand-ecran-accessible` | ✓ | ✓ | ✓ | Aucun |

PC : 1440 × 1000. Mobile : 390 × 844. TV : 1920 × 1080 avec clavier et navigation directionnelle. Les 22 empreintes de composition d’accueil restent distinctes dans chaque format.

Le premier essai TV a révélé la minuterie de focus de Bureau du collectionneur. Après correction, les parcours PC/mobile de ce mode ont été rejoués et la matrice TV complète a été relancée avec succès.

Limites communes : Philips matérielle non testée (ADB absent) ; lancement de session simulé, cœur GBA réellement exécuté ; preuve de compatibilité des anciens états non exposée par l’API, donc Relancer conservé sans cette preuve. Production non déployée.

Sources : `output/ui-regressions/matrix-*.json`, `palettes-*.json`, `scenarios.json`, `focus-launch.json`, `core-check.json` et `report.json`.
