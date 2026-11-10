# Pack de consoles fourni pour GameFlex

Source : `gameflex-74-icones-consoles.zip`, fourni par le propriétaire le 10 novembre 2026.
Les 74 PNG individuels sont utilisés, sans découpe des planches ni modification du dessin.
Les WebP optimisés gardent le format original et le nom intégré. Le manifeste conserve les
empreintes des PNG sources et des WebP livrés ; le ZIP original reste dans les pièces jointes.
Les illustrations sont servies localement sous `/images/console-icons/`, avec un nom de fichier
contenant leur empreinte pour éviter de réutiliser une ancienne image en cache.

`console-icons.js` relie les identifiants exacts de GameFlex aux images du pack. Les 74 noms
canoniques du pack sont aussi disponibles. Les systèmes absents du pack gardent leur visuel
antérieur et ses crédits : par exemple Commodore 64, Amstrad CPC, Capcom CPS et Steam PC.
Les modèles distincts ne sont pas assimilés (NGP monochrome / NGP Color, WonderSwan / Color,
Amstrad CPC / GX4000, Steam / Steam Deck). Les identifiants existants ne changent pas.

Les cartes remplacent les photos dans le composant partagé des consoles de l'interface active.
Les jaquettes des jeux, le logo de l'application et les 22 styles archivés restent inchangés.
Le crédit ne prétend pas que ces illustrations ont été créées par Evan-Amos ou Libretro.
Les marques et noms restent la propriété de leurs titulaires ; aucune licence n'est présumée.

Vérifications : `node tests/pi-tv/run-playstation-tests.cjs` et
`tests/pi-tv/console-icons-browser.py` sur la prévisualisation isolée.
