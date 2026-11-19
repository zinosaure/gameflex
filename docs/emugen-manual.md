> Historique : EmuGen est désormais retiré du parcours actif. Voir [Imports et sources](imports-et-sources.md).

# Vérifier EmuGen dans la fenêtre du Pi

Dans la fiche du jeu, ouvrez **Sources de téléchargement**, puis **Vérifier EmuGen**. Ce bouton est proposé immédiatement, sans attendre une tentative headless.
La fenêtre affichée dans GameFlex est Chromium sur le Pi. Elle utilise une session
éphémère dédiée, indépendante du navigateur externe de votre téléphone.

1. Dans cette fenêtre, terminez vous-même la vérification Cloudflare si elle apparaît.
2. Cliquez vous-même sur **Magic Cookie 🍪** sur la fiche EmuGen.
3. Cliquez sur **Reprendre la recherche** dans GameFlex. Le serveur lit la section
   Téléchargements dans **la même page et le même contexte**, sans navigation,
   sans nouveau clic et sans copier les cookies.
4. Revenez à la fiche pour ouvrir les liens EmuGen, affichés avant les autres sources.

Sur iPhone, un toucher clique dans la fenêtre distante. Le mode paysage, **Grand
écran**, **Zoom + / −** et les flèches de défilement facilitent les petits contrôles.
Fermer/réduire la fenêtre conserve la session ; **Terminer la session** la détruit.
Après vingt minutes, choisissez **Relancer la vérification**. Une seule session
interactive est autorisée sur le Pi, avec au plus deux connexions de visualisation
pour son propriétaire. Un autre compte ou un autre jeu ne peut pas la récupérer.

## Résultats

- `links` : au moins un lien final visible dans la section. Les URL d’hébergeurs
  et pages intermédiaires restent celles fournies par EmuGen.
- `empty` : uniquement après le message explicite d’absence de téléchargements.
- `challenge_required` : vérification ou Magic Cookie toujours en attente.
- `session_expired` : session détruite ou expirée ; relance proposée.
- `error` : chargement/lecture impossible ou section toujours en cours de chargement.
- `not_found` : fiche introuvable, distincte d’une fiche sans liens.

La fiche externe et les autres fournisseurs restent disponibles. La recherche,
les métadonnées et les jaquettes publiques utilisent toujours HTTP et leur cache.
Les corrections manuelles et le repli champ par champ sont conservés. Le navigateur
headless inactif est fermé avant l’ouverture interactive pour limiter la mémoire.
Les résultats de reprise restent liés à la session en mémoire ; une erreur réseau
n’est jamais enregistrée comme une absence de téléchargement.

## Isolation et accès

Tout est installé **dans l’image ARM64 `gameflex-experience`** : Chromium Debian
natif, Xvfb et x11vnc. Aucune installation sur DietPi. Xvfb n’écoute pas TCP ; VNC
écoute seulement `127.0.0.1:5999` **dans ce conteneur**. Aucun port VNC, CDP ou de
débogage n’est publié. Playwright contrôle Chromium par son tube local.

Le client [noVNC 1.7](https://github.com/novnc/noVNC), sous MPL-2.0 (licences
conservées dans le paquet npm), reçoit seulement l’affichage distant. Son
[API RFB](https://novnc.com/noVNC/docs/API.html) accepte le WebSocket déjà authentifié.
Les cookies et jetons du site restent côté serveur. Aucune saisie, cookie ou jeton
n’est journalisé. Téléchargements, fenêtres supplémentaires, sélection de fichiers,
presse-papiers VNC et connexions WebSocket des pages web sont désactivés.

GameFlex utilise sa route privée sur son origine LAN existante :
`/experience-api/v1/emugen/verification/<id>/socket`. Le serveur vérifie l’origine
HTTP et le JWT GameFlex auprès du service credential, puis le propriétaire de la
session **avant toute connexion VNC**. Le JWT circule dans le premier message,
jamais dans l’URL. Il est revérifié toutes les trente secondes ; expiration et
fermeture interrompent la connexion. Aucun identifiant de session ne constitue
à lui seul une autorisation. Les commandes HTTP exigent aussi le compte connecté.
Le lien `/retrogaming/<console>?emugenGame=<slug>&emugenVerification=<id>` peut
rouvrir la fenêtre depuis un autre appareil connecté au même compte.

La navigation principale et ses redirections restent sur les domaines EmuGen
validés, sur la fiche exacte. Les seules ressources réseau supplémentaires admises
viennent des domaines EmuGen et de `challenges.cloudflare.com`, avec adresses DNS
publiques épinglées. Les liens de téléchargement sont affichés, sans être suivis.
Le parcours n’automatise ni Cloudflare ni le clic Magic Cookie.

Utiliser l’adresse LAN existante ou votre accès Tailscale déjà configuré. Aucun
port de routeur, pare-feu ou accès Internet supplémentaire n’est créé.

## Déploiement et tests sur ARM64

Les commandes de construction et de remplacement de l’interface figurent dans
[EmuGen](emugen.md#déployer-sur-ce-pi). Elles reconstruisent également Chromium,
Xvfb et x11vnc. Le service conserve les limites mémoire, processus, privilèges et
système de fichiers du déploiement Pi. Le cache web passe à la version 37.

```bash
docker run --rm --init --user 1000:1000 --network none --read-only \
  --cap-drop ALL --security-opt no-new-privileges:true --memory 1536m \
  --pids-limit 512 --shm-size 128m --tmpfs /tmp:rw,size=192m,mode=1777 \
  --mount type=bind,src="$PWD",dst=/app -w /app \
  gameflex-experience:pi-local \
  python3 -m unittest discover -s tests/pi-tv -p 'test_*.py'
```

`test_emugen_manual.py` utilise uniquement la fixture HTML locale : intervention
simulée, cookie créé par cette fixture, reprise sans changer de page/contexte,
liens, absence explicite, section qui ne termine pas son chargement, expiration,
nettoyage, isolation entre comptes, capacité et authentification du WebSocket.
`manual-viewer.jsx` et `manual-preview.py` servent de banc d’essai tactile avec
un faux compte de fixture, jamais des identifiants réels.

Le test tactile lance aussi un navigateur client émulant un iPhone dans le même
conteneur : la commande de test lui réserve 1536 Mio et 512 tâches. Le service
déployé conserve 1024 Mio et 256 tâches, le client réel étant sur le téléphone.
La validation tactile automatisée utilise Chromium mobile émulé ; elle ne remplace
pas un essai matériel Safari sur iPhone.

## Résultat sur ce Pi

Déployé sur `http://192.168.1.63:2000`, images ARM64 locales, cache web 38.
Les 98 tests Python passent, dont le parcours tactile complet noVNC avec client
Chromium mobile émulé. Les quatre tests de session ont aussi été relancés après
le dernier ajustement de classification des erreurs : ils passent avec les
limites réelles du service (1024 Mio, 256 tâches, système de fichiers en lecture
seule). La compilation React et le test de rendu de fiche passent ; le lint ciblé
ne signale ni erreur ni avertissement. Le lint global garde ses 47 erreurs
préexistantes et 43 avertissements hors de ce parcours.

La fiche réelle **Prince of Persia : Les Sables Oubliés — Wii** a été ouverte
dans le Chromium interactif du Pi, pour le compte `unknow`. La connexion WebSocket
authentifiée a été validée **à travers nginx sur le port 2000**, jusqu’à la
bannière VNC. Son écoute reste limitée au loopback dans le conteneur. La jaquette
publique et les 391 caractères de description restent récupérables indépendamment.
La reprise réelle renvoie `challenge_required` : aucune vérification Cloudflare
ni activation Magic Cookie n’a été effectuée automatiquement. Le résultat réel
« Aucun téléchargement disponible » attend donc l’intervention de l’utilisateur.
Aucun fichier de jeu n’a été transféré. Le navigateur interactif reste prêt pour
cette intervention pendant vingt minutes ; ensuite la fiche permet de relancer.

## Ouverture sans attente HTTP prolongée

La création du navigateur retourne désormais `202` avec `status: starting` et
l’identifiant propriétaire de la session. Le modal s’affiche immédiatement et
interroge son état jusqu’à `ready` ; la validation des métadonnées et le démarrage
Chromium continuent sur le worker unique. Un nouvel appel pour le même jeu et le
même compte récupère la session existante. L’utilisateur peut annuler pendant la
préparation. Une erreur de préparation est affichée distinctement de l’expiration.

Le frontend ne lance plus automatiquement le résolveur headless à l’ouverture des
sources. L’endpoint ancien reste disponible pour compatibilité. Avant le navigateur
interactif, son éventuel navigateur inactif est libéré sur son worker ; un échec de
cette libération ne déclenche plus le lancement concurrent d’un second navigateur.

Validation de cette correction : quatre tests ciblés passent (création asynchrone,
idempotence, annulation, endpoint `202` et modal visible pendant `starting`), puis
les six tests de régression des sessions et du contrôle tactile noVNC passent dans
l’image ARM64. Aucun clic Cloudflare/Magic Cookie réel ni transfert de jeu n’est
effectué par ces tests ; les clics automatisés concernent uniquement la fixture.

Essai réel après correction sur la fiche GBA déjà ouverte par l’utilisateur :
création acceptée en 0,039 seconde avec `starting`, puis passage à `ready`.
La connexion WebSocket via nginx est authentifiée et VNC reste sur le loopback
du conteneur. La reprise attend encore le clic humain (`challenge_required`).
Aucune vérification ni activation Magic Cookie n’a été automatisée ; aucun jeu
n’a été téléchargé.


## Magic Cookie en dehors de la zone visible

Le bouton « Trouver Magic Cookie » de la fenêtre privée ouvre la recherche du
Chromium distant, recherche le texte et ferme la barre de recherche en conservant
la position trouvée. Cela fait défiler la fiche sans cliquer sur le bouton, créer
un cookie ou résoudre Cloudflare. Valider ensuite la vérification affichée, cliquer
manuellement sur Magic Cookie dans la fenêtre puis « Reprendre la recherche ».
Le raccourci est disponible lorsque la connexion noVNC est établie.

Le test tactile couvre maintenant un bouton situé 1 800 pixels plus bas : recherche,
bouton ramené dans la zone visible, absence de cookie avant le clic humain simulé,
puis reprise avec la même page et le même contexte Chromium. Cette modification
est uniquement frontale ; l’API et son navigateur privé ne sont pas redémarrés.

## Affichage derrière un proxy sans WebSocket

Le client essaie le WebSocket authentifié, puis bascule automatiquement en HTTPS
si le proxy refuse la connexion ou ne répond pas sous cinq secondes. Le transport
`POST /experience-api/v1/emugen/verification/<session>/transport` conserve la même
session Chromium et accepte uniquement les opérations ouvrir/lire/écrire/fermer.
Chaque requête valide le compte, l’origine et l’appartenance de la session. Le
jeton reste dans l’en-tête Authorization, jamais dans l’URL. Le serveur ne permet
que deux connexions HTTP, ferme les connexions inactives après 30 secondes et
contrôle l’expiration de la session. VNC reste sur le loopback privé du conteneur.
Aucun clic Cloudflare ou Magic Cookie n’est automatisé.

Validation : `test_vnc_http.py` couvre l’isolation des comptes et sessions,
l’expiration, l’authentification, l’origine et le transport binaire.
`test_emugen_manual_http.py` bloque volontairement le WebSocket et reprend le
parcours tactile complet dans le même contexte Chromium, avec une fixture libre.
Le frontend utilise le service worker version 42 pour actualiser le cache.

Vérification sur le Pi, via HTTPS public : canevas distant visible en viewport tactile, 34 réponses de transport HTTPS réussies, aucune erreur JavaScript. Contrôle Cloudflare et Magic Cookie laissés manuels. Rapport : `output/emugen-http-validation.json`.
