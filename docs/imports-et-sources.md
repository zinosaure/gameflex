# Imports personnels et sources

EmuGen est retiré du registre actif, des fiches et de la vérification distante.
Planetemu, FreeROMs et ROMsPure alimentent uniquement les listes et les jaquettes.
Leurs interrupteurs restent dans les paramètres. Les anciennes métadonnées,
images en cache et corrections personnelles sont conservées.

Dans **Paramètres › Sources de jeux** :

- **Ajouter un fichier** : choisir la console puis un fichier personnel compatible.
- **Source réseau** : ajouter un partage Samba, WebDAV ou kDrive. La connexion est
  testée avant enregistrement avec les API du projet. Les identifiants restent
  dans le service de comptes existant ; ils ne sont pas stockés dans le navigateur.
- Le dossier local de destination reste configurable via **Dossier de jeux**.

Les imports passent par `POST /experience-api/v1/imports/sessions`, puis des blocs
binaires de 1 Mio confirmés par le Pi, et une finalisation. Chaque bloc porte un
offset contrôlé. La progression correspond aux octets reçus, puis à la vérification
et au rangement. Le fichier est publié atomiquement, sans écrasement. Les imports
apparaissent aussi dans la bibliothèque pendant le transfert. Annuler supprime le
fichier temporaire. Une session inactive expire après 15 minutes ; le redémarrage
marque les imports interrompus comme des échecs.

Il n’y a pas de limite de 64 Mio. L’espace libre doit permettre le fichier temporaire
et son installation. Chaque requête exige le compte administrateur ; les sessions
sont liées au compte. Aucun montage système ou ajout de partage n’est exécuté par
le déploiement. Les adresses et identifiants des sources sont renseignés par l’utilisateur.

Tests : `test_upload_sessions.py`, `personal-upload.mjs`, `test_source_settings.py`
et les tests d’import existants. Les transferts de validation utilisent uniquement
la démo originale CC0 du projet et des octets de remplissage, supprimés ensuite.
