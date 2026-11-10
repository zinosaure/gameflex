# GameFlex TV 0.3.1

Les sources de l'application TV existante ont été copiées depuis le travail local
sans modifier les originaux. L'identité de signature doit être celle de l'installation
existante (`GAMEFLEX_TV_SIGNING_DIR`), sinon la mise à jour est refusée par Android.
Aucune clé, aucun mot de passe et aucun jeton ne doivent être ajoutés au dépôt.

Après l'association ou la restauration de sa session, la TV ouvre désormais
l'interface commune du serveur en mode TV (`/?tv=1`). Elle bénéficie donc des
consoles, des icônes et de la manette téléphone de la version web, sans garder une
seconde bibliothèque différente dans l'APK. L'ancienne page locale reste disponible
pour l'association initiale et les erreurs de connexion.

Le pont JavaScript traduit seulement la télécommande vers les commandes de l'interface.
Il n'expose aucune commande système. Le contrôle de l'origine et la vérification TLS
restent actifs. Le lecteur et les entrées de jeu existants sont conservés.
Le retour de la partie utilise la navigation de l'application commune et ses réglages
habituels de session/sauvegarde.

`build.py` utilise le SDK déjà présent, sans téléchargement ni installation système.
`device.py install --tv-free` exige l'identité physique épinglée, sauvegarde l'APK
installée et vérifie la signature avant une mise à jour `pm install -r` (données conservées).
Ne lancer l'application qu'à la demande de l'utilisateur et si aucune lecture n'est en cours.
