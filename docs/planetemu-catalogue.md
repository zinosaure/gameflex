# Catalogue Planetemu dans GameFlex

Chaque fiche console distingue **Mes jeux**, obtenus du scanner local authentifié, du **Catalogue Planetemu**, qui contient uniquement des titres et des liens vers les fiches du site. Les cartes externes ne lancent ni ne téléchargent rien dans GameFlex. Le catalogue n’atteste ni la disponibilité réelle d’un fichier, ni son droit d’utilisation.

Le service lit uniquement `/robots.txt`, les pages publiques `/machine/<nom>` et `/roms/<ensemble>?page=<lettre>` de https://www.planetemu.net/. Aucun accès aux endpoints de téléchargement, archives, recherches, pages `/rom/`, BIOS ou firmwares. La présence d’un titre sur un site ne constitue pas une licence de redistribution.

Les règles robots consultées le 8 novembre 2026 permettent les pages publiques canoniques et excluent notamment les recherches et les utilitaires. Les jaquettes viennent du service de métadonnées **Libretro Thumbnails**, documenté pour le téléchargement de vignettes à la demande : https://docs.libretro.com/guides/roms-playlists-thumbnails/#thumbnails . Aucun screenshot Planetemu n’est aspiré.

Le service lit exclusivement les index `Named_Boxarts` de l’organisation GitHub `libretro-thumbnails`, puis des PNG de ce même répertoire. Aucun jeu, archive, BIOS, firmware ou endpoint de téléchargement n’est accessible par ce fournisseur. Les index sont gardés 7 jours (avec réutilisation de l’ancien en cas d’indisponibilité). Les images restent dans `.local/pi-tv/media`, budget global 256 Mio, révision à 30 jours ; elles ne sont ni incorporées à une image Docker, ni destinées au futur dépôt public. Les jaquettes conservent les droits de leurs éditeurs : il s’agit d’une consultation personnelle sur le LAN, pas d’une licence libre des illustrations. Une diffusion publique ou commerciale doit passer par un fournisseur offrant les droits adaptés, par exemple le contrat IGDB préparé séparément.

Attribution : Libretro Thumbnails et ayants droit dans les cartes ; source exacte et notice conservées dans les crédits. Correspondance par console et titre exact, puis même titre sans qualificatifs de région/révision/bêta ; préférence à la région demandée. Aucune correspondance approximative entre jeux différents. Une édition peut partager la jaquette de l’édition publiée du même titre. En absence de correspondance fiable (par exemple prototype ou borne sans boîte), « Sans image » reste explicite.

Index GitHub : API publique sans identifiant, quota habituel de 60 requêtes/heure par IP, deux requêtes par console lors du renouvellement ; les images utilisent le CDN raw.githubusercontent.com. Documentation : https://docs.github.com/en/rest/using-the-rest-api/rate-limits-for-the-rest-api . Les appels sont sérialisés et espacés d’au moins 350 ms ; aucune aspiration globale des images. Les PNG sont limités à 2 Mio, avec vérification MIME et signature ; URL de fournisseur fixe, redirections refusées, chemins de fichiers issus de l’index validé seulement. L’endpoint ne reçoit jamais d’URL arbitraire ni de clé API.

Correction du filtre : les jeux Famicom Disk System restent associés à leur propre jeu de jaquettes ; même si une source renvoie toute sa liste, le serveur filtre chaque titre selon la lettre choisie, puis trie les résultats. Les anciens caches non filtrés sont écartés (`-v2.json`). Le client ignore également les réponses annulées lors d’un changement rapide de lettre et filtre les cartes à l’affichage.

Cache JSON local dans `.local/pi-tv/planetemu/`, index par console et lettre pendant 24 heures ; index des ensembles pendant 7 jours. Accès authentifié `/experience-api/v1/planetemu/<console>?letter=A` ; lettres A–Z et 0. Pages HTML limitées à 2 Mo, au plus quatre ensembles par console et 1 000 entrées par lettre. Les requêtes sont séquentielles, espacées d’au moins une seconde ; pas d’exploration en arrière-plan de tout le site. Les données expirées peuvent être réutilisées si la source est indisponible. Les erreurs réseau s’affichent avec un lien vers la source.

Game Boy/Color associe ses deux ensembles. Pour les consoles absentes du site ou sans ensemble exact identifié (notamment CPS, Neo Geo, Vectrex, Wii, PS2, PSP, Doom), GameFlex affiche explicitement l’absence de correspondance, sans présenter des titres d’autres machines comme compatibles. Le panneau reste présent. Les formats alternatifs et variantes régionales du site restent identifiés dans les titres.

Attribution visible : « Catalogue Planetemu », avec lien vers la machine et vers chaque fiche. Aucun jeu commercial, BIOS ou firmware n’est téléchargé ou intégré.

Pour associer une jaquette importée et approuvée à une fiche externe : cible `planetemu:/rom/<ensemble>/<fiche>`. Cette association manuelle prend priorité sur la résolution Libretro à la demande.

### Chargement des jaquettes

Les statuts de la source (« Retirée » après la région) sont séparés du titre avant la recherche ; Pac-Man Collection retrouve ainsi sa vraie jaquette GBA. Les cartes e-Reader ne sont pas mélangées aux jeux GBA. Le catalogue v3 conserve ces corrections.

Les images sont décodées puis réduites sans recadrage en WebP (384 × 512 maximum). Quatre téléchargements et deux conversions au maximum peuvent travailler simultanément ; les accès au cache ne patientent plus derrière les téléchargements des autres images. La lecture d’un catalogue ne prépare plus aucune jaquette en arrière-plan : seules les cartes visibles ou proches de l’écran demandent leurs images. Le navigateur affiche 24 titres, puis propose « Afficher la suite ». Les URL à empreinte du cache évitent toute nouvelle résolution et toute redirection. Les titres sans jaquette fiable gardent une indication explicite.

Le conteneur `gameflex-experience:pi-local` doit être construit avec `deploy/pi-tv/Experience.Dockerfile` avant son remplacement ; Pillow est installé uniquement dans cette image, sans modifier Python sur l’hôte.

Les succès sont servis depuis le cache avant toute attente de rafraîchissement d’index. Une absence connue est mémorisée quinze minutes ; un échec réseau trente secondes, pour éviter de répéter les mêmes téléchargements. Le navigateur suit le chargement par URL et vérifie aussi les images déjà complètes en cache, sans remettre cet état à zéro après `onLoad`. Une tentative limitée à huit secondes passe à l’image alternative lorsqu’elle existe, puis à une indication temporaire si les deux sources échouent. Les métadonnées de recherche occupent au plus deux connexions à la fois, avec priorité réseau basse, afin de laisser passer les images.
