# GameFlex sur loutrenas.myqnapcloud.com:2000

Destination demandée : `https://loutrenas.myqnapcloud.com:2000`.

## État vérifié

Le nom DNS se résout et le port public 2000 répond actuellement en HTTP, pas en
TLS. Le port 443 dispose d’un certificat valide incluant ce nom, mais son site
actuel est NASFLIX. Aucun réglage NASFLIX, reverse proxy existant ou routeur n’a été
modifié. Le gestionnaire de reverse proxy présent sur NasPi n’a pas d’accès QNAP
configuré ; aucune clé privée du certificat n’a été lue ni copiée.

## Règle prête à appliquer sur le QNAP

La description vérifiable est `deploy/pi-tv/qnap-gameflex-route.json` :

- Source **HTTPS**, nom `loutrenas.myqnapcloud.com`, port **2000**.
- Cible **HTTP**, `192.168.1.63`, port **2000**.
- Utiliser le certificat existant de ce domaine.
- Conserver l’en-tête Host avec le port et autoriser les WebSockets, nécessaires
  au navigateur privé EmuGen et aux canaux GameFlex.
- Ne pas limiter le corps des requêtes d’import ; délai d’inactivité adapté aux
  gros transferts (3 600 secondes côté proxy).
- Conserver toutes les autres règles et le HOME TV.

Vérifier d’abord si la redirection publique du port 2000 arrive sur le QNAP ou
sur NasPi. Si elle arrive directement sur NasPi, il faudra la faire arriver sur
l’entrée HTTPS du QNAP ; cette modification de routeur exige un accord explicite.
Ne pas diriger une règle proxy vers elle-même et ne pas remplacer NASFLIX sur 443.
L’accès LAN HTTP `http://192.168.1.63:2000` reste disponible pour la TV.

## Validation après application

Vérifier avec validation TLS normale (aucune option ignorant le certificat) :

1. Chargement de GameFlex à l’adresse HTTPS demandée.
2. Connexion au compte existant et bibliothèque.
3. WebSocket authentifié et fenêtre privée EmuGen, sans exécuter la vérification
   Cloudflare ni cliquer Magic Cookie à la place de l’utilisateur.
4. Import personnel supérieur à 64 Mio et fin de progression confirmée.

Les imports GameFlex ne portent plus de plafond de 64 Mio. Le Pi reçoit le flux
par blocs de 1 Mio, contrôle l’espace libre, vérifie les en-têtes et l’empreinte,
publie le fichier atomiquement et supprime les fichiers partiels en cas d’échec.
Les protections de nom, format, permissions et absence d’écrasement sont conservées.
L’accès HTTPS à l’interface ne constitue pas une validation du streaming WebRTC
hors du LAN ; aucun port du moteur ou du système n’est publié par cette règle.
