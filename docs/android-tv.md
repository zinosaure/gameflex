# GameFlex TV pour Philips Android TV 11

Le module `deploy/pi-tv/android` est un client TV dédié (`org.gameflex.tv`,
version 0.2.0 / code 2), sans bibliothèque native. Il fonctionne sur ARM32 aussi
bien que sur ARM64. La Philips vérifiée expose Android 11 et `armeabi-v7a`.
L’APK signé est dans `output/android-tv/GameFlex-TV-0.2.0.apk` ; ses empreintes,
son certificat et le HEAD source sont dans `output/android-tv/apk.json`.
Aucun commit, fork ou push n’a été créé.

## Architecture et lancement

L’accueil TV est fourni avec l’APK : grandes jaquettes, bibliothèque réelle,
consoles, favoris, récents et fiche. Il réutilise les API et préférences GameFlex,
avec un focus spatial adapté au DPAD. Une enveloppe hybride conserve le lecteur
WebRTC existant au lieu de dupliquer son protocole et ses contrôles d’accès.
Les interfaces web, PC et mobile restent disponibles.

| Mode | Exécution | Affichage TV | Commandes |
| --- | --- | --- | --- |
| Rétrogaming installé | Core libretro sur NasPi | Lecteur GameFlex `/streaming`, vidéo H.264 et audio Opus via WebRTC | Télécommande/clavier et Gamepad API du lecteur → canal WebRTC `input` → moteur sur Pi |
| Console sans core installé | Aucun lancement | Consigne dans la fiche | Installer un core approprié depuis l’administration |
| Moonlight / application Android externe | Pas de chemin intégré vérifié | Aucun lancement externe dans cet APK | Non utilisé ; la présence de `com.limelight` ne prouve pas une intégration |
| Autres plateformes, ports, hôtes distants | Hors périmètre de ce client initial | Non présentés comme jouables | Utiliser le parcours web existant |

Le bouton Jouer appelle le véritable canal authentifié `/api/v1/facade/ws`,
action `launch`, puis ouvre `/streaming?sessionId=…&play=1`.
Retour pendant la partie propose Continuer, Activer le son ou Quitter la partie.
Quitter revient à l’accueil TV et demande `session.stop` pour cette seule session.
Aucun simple démarrage de processus n’est présenté comme une réception vidéo.

## Connexion sans mot de passe à la télécommande

Au premier démarrage, valider l’adresse LAN connue `http://192.168.1.63:2000`.
Le site public `https://gameflex.skino.app` est accepté mais répondait HTTP 403
lors du contrôle ; aucun réglage réseau n’a été changé.

L’écran TV affiche un code de six chiffres. Depuis un téléphone déjà connecté au
même serveur GameFlex, ouvrir :

`http://192.168.1.63:2000/experience-api/v1/tv/link`

Saisir le code puis Autoriser cette TV. Sinon, se connecter au site avec le lien
proposé et revenir sur cette page. Une connexion classique à la télécommande
reste possible. « Compte » crée un nouveau code ; MENU ouvre l’adresse serveur.

L’API réutilise le JWT GameFlex existant ; aucune nouvelle identité ni mot de passe
n’est créé. Le code expire en trois minutes. Le poll nécessite un secret aléatoire
256 bits, dont le serveur conserve seulement l’empreinte. Le JWT est livré une
seule fois après approbation authentifiée et conservé dans le stockage privé de la
WebView. Les associations en attente restent uniquement en mémoire. Créations et
approbations sont limitées en fréquence. Aucun secret n’entre dans une URL ou les
logs. HTTP LAN n’est pas chiffré ; utiliser le domaine HTTPS lorsqu’il est accessible.

## Accès depuis l’accueil Philips existant

L’entrée Android TV de GameFlex est installée sans changer le HOME. L’accueil
existant redirige vers LoutreTV. L’accès réel validé est : accueil → descendre à
« Médiathèque ↓ » → « Toutes les applications » → GameFlex → OK.
GameFlex a été sélectionné au DPAD et ouvert depuis cette liste. Aucun raccourci
existant, ordre d’applications ou paramètre LoutreTV n’a été modifié.

## Navigation

- Flèches / DPAD : focus visible ; OK / A : sélectionner ; Retour / B : revenir.
- La fiche retrouve le jeu sélectionné en revenant à la bibliothèque.
- Bibliothèque, consoles, favoris et récents utilisent les données du compte.
- Pendant le jeu, les événements manette sont laissés au lecteur existant.
- Le jeu original Palette libre permet de contrôler la vidéo animée. Il est
  silencieux et ne réagit pas aux commandes : il ne suffit pas à prouver le son
  audible ou une manette physique.

## Construction locale et installation contrôlée

Le constructeur utilise uniquement le JDK/SDK existant de LoutreTV en lecture
seule, sans toucher à ce projet ni à sa clé de signature. Tous ses fichiers de
travail sont créés dans GameFlex. Aucune installation système n’est nécessaire :

```sh
python3 tools/android-tv/build.py
python3 tools/android-tv/audit.py
```

Une clé GameFlex distincte est conservée sous `.local/android-tv/signing/`
(mode 700, fichiers 600). La sauvegarder de manière privée pour les mises à jour ;
ne jamais la versionner. L’APK est signé v2/v3, non débogable, sans sauvegarde
Android et sans pont JavaScript natif. Les navigations principales restent sur
l’origine choisie ; les erreurs TLS ne sont pas ignorées.

Après confirmation explicite que la TV est libre :

```sh
python3 tools/android-tv/device.py install --tv-free
python3 tools/android-tv/device.py launch --tv-free
```

Ces commandes utilisent directement le transport ADB existant, la clé dédiée et
la seule cible `192.168.1.175:5555`. Elles ne redémarrent aucun serveur ADB.
Le modèle et l’empreinte de série doivent correspondre à la référence ; l’adresse
Wi-Fi est aussi comparée lorsqu’elle est lisible. Une signature incompatible ou
une version déjà plus récente bloque l’installation sans supprimer l’application.
Le transfert est vérifié par SHA-256 avant `pm install`.

Le backend de jumelage est inclus dans l’image Experience. Pour cette installation
locale existante, construire d’abord l’image candidate, puis remplacer uniquement
`gameflex-experience` :

```sh
DOCKER_CONFIG="$PWD/.local/docker" docker build -f deploy/pi-tv/Experience.Dockerfile -t gameflex-experience:tv-candidate .
python3 tools/android-tv/deploy-api.py
```

Le déploiement conserve ses montages et limites. Il refuse
les travaux de téléchargement/métadonnées actifs. Ne pas employer une commande
Compose globale pour ce changement.

## Vérifications et limites

Les tests automatisés sont dans `tests/pi-tv/test_tv_pairing.py`,
`test_tv_shell.py` et `tools/android-tv/test-server-policy.java`.
Ils couvrent expiration, secret invalide, approbation unique, limitation,
navigation réelle dans Chromium, favoris, console, fiche et restauration du focus.
Le rapport de l’audit TV en lecture seule est `.local/android-tv/tv-audit.json`.
Le test `python3 tools/android-tv/verify-live.py` utilise uniquement la démo NES
originale déjà installée, au moyen d’un navigateur temporaire sur le réseau hôte
pour recevoir directement les paquets UDP. Il ne change aucun réglage réseau.
Les résultats de lecture vidéo locale sont séparés dans
`output/android-tv/local-integration.json` ; ils ne remplacent jamais les essais
sur la Philips. Le bilan final des essais sur la Philips est `output/android-tv/validation.md`.
La mise à l’échelle compacte, le scan code des touches Android et la confirmation
de sortie ont été corrigés à partir de ces essais. Les captures `tv-*` viennent
de l’appareil ; les captures `local-*` viennent du navigateur de test sur NasPi.

À essayer physiquement une fois installé : naviguer avec la télécommande,
ouvrir Palette libre, écouter un contenu autorisé avec son et essayer une manette
déjà appairée. Des touches ADB simulées ne prouvent pas une manette Bluetooth.

## Retour arrière limité à GameFlex

L’installation sauvegarde l’état initial et, en cas de mise à jour, les APK
GameFlex existants sous `.local/android-tv/rollback/<date>/`.
`output/android-tv/installation.json` indique cette sauvegarde.

Pour une première installation, quitter la partie dans GameFlex, puis désinstaller
uniquement `org.gameflex.tv` avec ADB sur la cible autorisée : cela supprime aussi
la session et les préférences locales de cette nouvelle application. La bibliothèque,
les sauvegardes et les comptes sur le Pi ne sont pas supprimés. Cette action exige
l’accord de la personne avant de supprimer les données de l’application.

Après avoir accepté la suppression de la seule application TV, exécuter depuis le
répertoire GameFlex (la classe TV vérifie l’identité et fixe la cible autorisée) :

```sh
python3 - <<'PYTHON'
import sys
sys.path.insert(0, 'tools/android-tv')
from tv_device import TV
with TV() as tv:
    print(tv.shell('pm uninstall org.gameflex.tv'))
PYTHON
```

Pour une mise à jour, réinstaller l’ancien APK n’est possible que si Android accepte
sa signature et sa version. Un retour à une version plus basse peut être refusé ;
ne pas forcer un effacement de données pour contourner ce refus. Aucun autre
package, HOME, réglage TV ou service Docker n’entre dans le retour arrière.
