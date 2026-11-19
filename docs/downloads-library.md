# Bibliothèque, sources et téléchargements

Le déploiement Pi utilise par défaut `/home/config/GameFlex/.local/demo-library/roms`, avec un sous-dossier par console (`nes`, `gba`, etc.). Le moteur conserve `.local/demo-library` comme racine technique ; aucun second dossier `roms/roms` n’est ajouté. Le navigateur de dossiers, accessible dans les paramètres, affiche seulement les dossiers montés. Sa configuration reste réservée à l’administrateur. Un autre emplacement exige son montage dans le moteur et dans le service, avec l’accord de la personne avant toute modification hors du projet.

## Sources et catalogue commun

La page d’une console fusionne la bibliothèque locale, le contenu CC0 du projet, Planetemu et les métadonnées publiques FreeROMs. Les provenances sont conservées, les éditions sont regroupées et les jeux installés restent prioritaires. Les cinq éditions principales Pokémon GBA disposent aussi de rapprochements exacts entre noms traduits. Le dialogue conserve chaque langue, région, source, référence de lancement et jaquette ; aucune traduction approximative ne fusionne des jeux différents.

FreeROMs est interrogé uniquement pour la console et la lettre demandées ; les listes sont mises en cache 24 heures et les fiches d’images 7 jours. Les images de la source peuvent compléter les jaquettes manquantes de Libretro. Elles restent dans le cache local de 256 Mio, avec source et attribution ; elles ne deviennent pas libres de droits et ne sont pas incorporées dans le futur fork ni dans les images Docker.

La connexion HTTPS du fournisseur utilise les certificats intermédiaires publics de `deploy/pi-tv/certs/`, vérifiés jusqu’à la racine ISRG Root X2 déjà reconnue par le système. La vérification TLS et celle du nom d’hôte restent actives ; aucun magasin de certificats de l’hôte n’est modifié.

## Déclenchement et résultat

À la demande explicite de la personne, le filtre « contenu libre approuvé uniquement » ne s’applique plus aux demandes FreeROMs. Sélectionner une édition FreeROMs expose son bouton de téléchargement ; un jeu ne comportant qu’une édition téléchargeable peut lancer directement la tâche au clic. Planetemu reste une source de métadonnées, sans connecteur de téléchargement.

La demande tourne en arrière-plan sur le Pi. Le serveur relit la fiche, cherche un lien direct compatible, reçoit et vérifie le format du fichier, puis l’écrit dans `roms/<console>/`. Les états persistent par compte : attente, en cours, réussite ou échec. La notification apparaît en haut à droite ; une réussite actualise la bibliothèque. Une tâche interrompue par un redémarrage devient un échec explicite. Les clics répétés sur une tâche active ne créent pas de double transfert.

**La présence d’une fiche ou d’un bouton publicitaire ne garantit pas un fichier.** Sur la fiche Émeraude examinée, le compte à rebours charge un script publicitaire de notifications et le bouton sponsorisé pointe vers un autre site ; aucun lien direct compatible n’est fourni. Dans ce cas, la tâche échoue avec cette explication. Les scripts, publicités et redirections externes ne sont pas exécutés ou suivis automatiquement.

L’import actuel prend en charge des archives ZIP contenant un seul fichier compatible, ou un fichier de jeu direct, avec un maximum de 64 Mio. Il couvre les formats de consoles anciennes enregistrés dans `freeroms.SYSTEMS`, dont NES, SNES, GB/GBC et GBA. Les archives RAR/7z, multi-fichiers et les jeux volumineux ne sont pas pris en charge par ce connecteur. Le refus d’un format n’est pas un filtre de licence.

## Contrôles conservés

L’authentification est requise. Le navigateur n’envoie qu’un identifiant de source, de console et de fiche : jamais une URL arbitraire. Les hôtes, chemins, schémas HTTPS et destinations réseau sont vérifiés ; les adresses privées et les liens publicitaires sont refusés. Les fichiers HTML ou exécutables hôtes ne sont pas traités comme des ROMs. Les chemins d’archive dangereux, liens symboliques, fichiers chiffrés ou volumes excessifs sont refusés. Les BIOS et firmwares restent exclus du catalogue de jeux.

Les écritures utilisent des descripteurs de dossiers avec refus des liens symboliques, une publication atomique sans écrasement et un contrôle de l’espace disponible. Un fichier personnel différent portant le même nom est conservé. Un seul transfert distant peut utiliser la mémoire à la fois. Les tests de transfert utilisent uniquement la cartouche NES originale CC0 du projet et des archives synthétiques ; aucun jeu commercial n’a été téléchargé pour valider ce changement.

## API locale authentifiée

| Route | Méthode | Usage |
| --- | --- | --- |
| `/experience-api/v1/library` | GET / PUT | Emplacement et choix administrateur |
| `/experience-api/v1/library/folders` | GET | Navigateur de dossiers administrateur |
| `/experience-api/v1/game-sources` | GET | Sources et capacités |
| `/experience-api/v1/freeroms/<console>?letter=P` | GET | Métadonnées de la lettre demandée |
| `/experience-api/v1/downloads` | POST | Demande par `gameId`, y compris `freeroms:<console>:<fiche>` |
| `/experience-api/v1/downloads` | GET | États du compte connecté |

Le service d’images public utilise des identifiants validés (`/experience-api/v1/freeroms-art/<console>/<fiche>`), comme les jaquettes Libretro. Aucune clé ou session utilisateur n’est envoyée aux fournisseurs. Aucun port supplémentaire n’est publié, aucune commande TV, aucun fork ou push n’est effectué.

## Recherche dans une console

Saisir une recherche suspend le filtre A–Z ; vider le champ rétablit la lettre précédemment sélectionnée. La recherche ignore la casse et les accents et prend en compte les noms de toutes les éditions regroupées : « Pokemon » trouve « Pokémon », et « Emerald » trouve la fiche française Émeraude.

La recherche interroge les métadonnées de toute la console, avec résultats déjà en cache puis chargement progressif des autres lettres. Une lettre est traitée à la fois (deux requêtes API), pour laisser la priorité aux jaquettes visibles, avec priorité à l’initiale saisie. Les résultats restent fusionnés et paginés. Une progression indique les lettres traitées ; une source inaccessible ne fait pas disparaître les résultats des autres sources. Changer de recherche, quitter la console ou vider le champ annule les requêtes et la file restantes. Les pages de métadonnées sont réutilisées cinq minutes dans le navigateur, dans un cache borné à 60 pages. Aucun fichier de jeu n’est téléchargé par une recherche. Le filtre Installés utilise seulement les jeux locaux et n’entraîne pas de recherche globale distante.

## Suivi sur les jaquettes

Une demande acceptée n’est jamais annoncée comme une réussite. Les étapes sont séparées : en attente, vérification du lien (0 % sans fichier reçu), transfert, vérification/extraction, rangement, analyse de la bibliothèque, puis résultat terminal. Le transfert publie les octets reçus et la taille annoncée ; si cette taille manque, l’interface affiche un indicateur indéterminé et les octets reçus. Le pourcentage reste inférieur à 100 tant que le fichier n’est pas publié dans le dossier. Les mises à jour sont limitées à quatre écritures par seconde et l’historique conserve les demandes actives.

Le shop et « Mes jeux » utilisent le même état de tâche. Une carte provisoire apparaît immédiatement dans la bibliothèque avec la jaquette et un voile gris portant la progression. Elle n’est pas jouable avant installation. Les échecs restent expliqués dans le suivi mais ne créent pas de faux jeu installé. La fin du transfert déclenche l’analyse avant l’unique notification terminale. Si l’analyse échoue après une copie réussie, une seule notification d’avertissement indique que le fichier est rangé mais que l’analyse est à relancer. Il n’y a plus de notification « Installation lancée » superposée au résultat.

Les tâches et leurs compteurs persistent sur le Pi, séparés par compte. Un incident de suivi ne transforme pas une tâche en échec : les dernières valeurs restent visibles avec une indication de reconnexion. Le texte et la jaquette fournis à la demande servent uniquement à l’affichage ; ils ne permettent jamais de choisir une URL ou un chemin d’écriture arbitraire.

### Importer son propre fichier

Dans **Mes jeux** ou la page d’une console, **Ajouter une ROM** ouvre le choix de
console et de fichier. L’administrateur envoie le fichier depuis son téléphone ou
son ordinateur ; le serveur le range sous `roms/<console>/`. Aucun fichier existant
n’est écrasé. L’envoi affiche son pourcentage et une seule notification confirme
le rangement, puis la bibliothèque est réanalysée.

L’import personnel ne comporte plus de plafond de 64 Mo. Le serveur reçoit les
fichiers par blocs de 1 Mio directement dans un fichier temporaire de la
bibliothèque, vérifie l’espace disponible et supprime les transferts incomplets.
La taille réelle et le SHA-256 complet sont conservés sans charger toute la ROM
en RAM. Les archives sont à décompresser, sauf les ZIP d’arcade conservés tels
quels ; les ensembles multifichiers restent à copier par les moyens habituels.
La route authentifiée `POST /experience-api/v1/imports?console=<id>` reçoit un
corps binaire et un en-tête `X-GameFlex-Filename` encodé avec `encodeURIComponent`.
Elle est réservée au rôle administrateur, refuse les chemins, extensions
incompatibles et exécutables, limite à un import simultané et publie sans écrasement
via les mêmes descripteurs de dossiers sécurisés que les installations.

L’identification conserve le SHA-256 et le nom du fichier. Les titres internes
GB/GBC, GBA et DS sont lus lorsqu’ils sont disponibles ; ils remplacent seulement
un nom générique (`dump`, `game`, etc.). Il n’existe pas ici de base universelle
associant chaque empreinte à un titre : un fichier mal nommé peut rester mal
identifié. Aucune correspondance incertaine n’est présentée comme vérifiée.
Les jaquettes utilisent Libretro Thumbnails avec son cache local, puis les
métadonnées FreeROMs déjà présentes en cache comme solution de repli. Une jaquette
absente reste signalée comme telle. Les crédits restent dans les paramètres et
sur les images ; les noms des fournisseurs ne figurent plus dans les listes ni
dans le choix des éditions.

ROMsPure est ajouté au registre des sources, avec `downloads: false` : sa page
d’accueil répond depuis le Pi mais aucun connecteur de catalogue, d’API ou de
transfert n’a été validé. Cette référence ne prétend pas apporter de nouveaux
jeux au catalogue. Le moteur peut essayer jusqu’à cinq identifiants compatibles
pour une même demande (identifiant principal et quatre alternatives), en gardant
un seul suivi et une seule notification finale. Le client limite les alternatives
à la langue de l’édition choisie. Aucune URL arbitraire n’est acceptée.

### Connecteur ROMsPure (mise à jour du 8 novembre 2026)

ROMsPure est désormais intégré au catalogue fusionné, aux jaquettes et aux essais
de transfert sur demande. Cela remplace la référence désactivée décrite plus haut.
Aucun jeu n’est préchargé avec le catalogue. Le connecteur couvre les catégories
NES, SNES, GB/GBC, GBA, N64, DS, GameCube, Wii, PS1, PS2, PSP, Mega Drive,
Saturn, Dreamcast et Neo Geo observées dans le [répertoire du site](https://romspure.cc/roms/).

Le fournisseur ne propose pas ici d’API officielle documentée : l’adaptateur lit
les cartes HTML, leur pagination et leurs images. Le tri alphabétique permet de
chercher les pages d’une lettre sans aspirer le catalogue complet. La recherche
utilise le filtre public `q`, avec cinq pages maximum par recherche ; une lettre
est limitée à dix pages de résultats par catégorie. Les réponses indiquent
`truncated` si la limite est atteinte. Les autres sources restent disponibles en
cas de panne d’un connecteur. Le navigateur limite à deux requêtes de catalogue
simultanées.

Le protocole public de préparation du lien suit les scripts `function.js` et
`assets/js/download.js` du site : page de sélection, nonce éphémère, action
`app_get_download_link` avec `post_id` et `index`. Les nonces et URL temporaires ne
sont ni exposés au client ni enregistrés dans les notifications. Aucune publicité,
aucun script tiers et aucun CAPTCHA ne sont exécutés ou contournés. Un refus du
site produit un échec explicite ou l’essai d’une autre source compatible, jamais
une réussite anticipée. La limite existante de 64 Mio s’applique aussi ici ; les
jeux plus volumineux nécessitent l’import personnel par les moyens habituels.

Le réseau valide HTTPS et les certificats, refuse les adresses privées et fixe
l’adresse DNS validée à la connexion TCP. Chaque redirection est revalidée.
Les URL de fichiers sont obtenues uniquement de la réponse du fournisseur, jamais
d’un paramètre URL libre du client. Le format, les en-têtes NES/GBA, la taille et
les chemins d’archive sont contrôlés avant publication atomique. Les ZIP Neo Geo
restent des archives, sans extraction sur le serveur.

Le cache conserve les pages et recherches 24 h, les images optimisées WebP et
leurs crédits 7 jours, avec un repli vers les métadonnées locales lors des pannes.
Les échecs sont temporisés une minute. Les fichiers de métadonnées du connecteur
sont élagués à environ 1 200 lorsque leur nombre dépasse 1 500. Les images partagent
le cache local de GameFlex et les crédits des paramètres ; elles restent soumises
aux droits de leurs ayants droit. Les [conditions du site](https://romspure.cc/terms-of-use/)
prévoient un usage personnel et interdisent notamment de surcharger le service.
L’adaptateur espace ses requêtes de métadonnées d’une seconde, et ses images
d’un quart de seconde, avec deux connexions maximum.

Validation : catalogue et API publique de nonce vérifiés en lecture sur le site.
Les transferts, erreurs, replis entre fournisseurs et écritures de fichiers sont
testés avec des métadonnées fictives et la démo originale CC0 de GameFlex.
**Aucun binaire de jeu commercial n’a été téléchargé pour cette validation.**
