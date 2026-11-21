# GameFlex sur le domaine NAS

Adresse : **https://loutrenas.myqnapcloud.com/gameflex/**.
`https://loutrenas.myqnapcloud.com:gameflex` n'est pas une URL valide : après `:`, un port doit être un nombre.

Le certificat HTTPS existant du NAS reste utilisé. Seul `/gameflex/` vise GameFlex ; la racine, NASFLIX et les autres routes restent inchangés. La connexion GameFlex reste obligatoire pour la bibliothèque privée. Les cookies NASFLIX ne sont pas transmis à GameFlex.

## Application

Le même build supporte la racine du serveur LAN et `/gameflex/` : routeur, API, WebSocket, images, chunks, manifeste et service worker. Les caches GameFlex sont isolés des autres applications du domaine. La TV conserve son adresse LAN et sa session.

## Proxy partagé

Le bloc prêt à intégrer est `deploy/pi-tv/gameflex-subpath.nginx.conf`, **dans** le serveur `loutrenas.myqnapcloud.com` de `/home/config/Projects/nginx/default.conf`. Il retire le préfixe pour le serveur existant `http://192.168.1.63:2000/`, préserve les WebSocket et les transferts sans limite de taille Nginx.

Avant application : accord pour ce fichier hors projet, sauvegarde locale dans `.local/subpath/`, comparaison du serveur visé, validation `docker exec reverse-proxy nginx -t`. Appliquer par rechargement gracieux `docker exec reverse-proxy nginx -s reload`, sans redémarrer les autres services. Aucune modification du routeur, de QNAP ni de Tailscale.

## Entrée HTTPS publique appliquée

Le trafic HTTPS public arrive directement sur `nasflix-unified` (port LAN 16002), et ne traverse pas le proxy du port 80. Le bloc `deploy/pi-tv/gameflex-public-ingress.nginx.conf` est donc aussi ajouté dans `/home/config/NASFLIX/unified/nginx.conf`. Il relaie uniquement `/gameflex/`, pour le domaine demandé, vers le port 2000. Aucun contrôle de connexion NASFLIX ne s’applique à cette route. Les autres emplacements restent inchangés. Les deux configurations ont été sauvegardées dans `.local/subpath/`, validées puis rechargées sans redémarrage.

## Retour arrière

Dans chacun des deux fichiers Nginx ci-dessus, retirer uniquement les deux blocs `/gameflex` ajoutés, vérifier `nginx -t`, puis recharger Nginx. Ne pas restaurer un fichier partagé complet si une autre application l'a modifié entre-temps. Le serveur LAN sur le port 2000 continue de fonctionner.

Le frontend précédent est conservé sous `gameflex-web:before-subpath` avant sa mise à jour ; les API et leurs données restent intactes.

## Vérifications

`docker run --rm --network none -v "$PWD:/app:ro" node:20-bookworm-slim node /app/tests/pi-tv/subpath.cjs`

Le test couvre l'API, les WebSocket, les URL absolues, le serveur fédéré et l'isolation des caches. Vérifier également l'accès HTTPS public, un lien profond, les images, une réponse d'API et une connexion GameFlex avant livraison.
