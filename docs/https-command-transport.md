# Commandes et signalisation derrière un proxy HTTPS

Le frontal conserve les WebSockets natifs lorsqu’ils fonctionnent. Si leur ouverture échoue (ou dépasse quatre secondes), `GameSocket` utilise le relais HTTPS authentifié du service `gameflex-experience` : `/experience-api/v1/socket-bridge`.

Le relais conserve la connexion WebSocket vers les services GameFlex sur le réseau Docker. Les messages JSON conservent leurs identifiants, autorisations et réponses. Les commandes déjà envoyées ne sont jamais rejouées après une rupture. La fermeture attend les envois en attente, notamment la réponse SDP et les candidats ICE. Le flux audio/vidéo et les commandes en partie restent sur WebRTC ; ils ne transitent pas par ce relais HTTP.

Chaque opération vérifie le compte auprès du service d’authentification. Les canaux appartiennent à leur utilisateur, expirent après 35 secondes d’inactivité et sont limités à six par compte et seize au total. Les messages et files d’attente sont bornés. Les destinations sont limitées aux chemins GameFlex explicitement autorisés, sur `gameflex-web:8080` ; aucune URL externe n’est acceptée. Les anciens services qui exigent un jeton en query le reçoivent uniquement sur ce trajet interne. Les requêtes publiques utilisent l’en-tête Authorization. Ne jamais journaliser les jetons ou messages.

## Vérification

```sh
node tests/pi-tv/game-socket.cjs
docker run --rm --read-only --network none -v "$PWD:/app" gameflex-experience:pi-local python /app/tests/pi-tv/test_socket_bridge.py
npm --prefix src/webapp/src run build
```

Après compilation, reconstruire le frontal avec `deploy/pi-tv/LiveFrontend.Dockerfile`. Le service experience utilise les fichiers Python montés depuis le projet ; son redémarrage recharge le relais. Vérifier qu’aucun import ni autre session de vérification n’est en cours avant de redémarrer ces services.

Cette correction contourne un refus de WebSocket par un proxy ; elle n’ouvre aucun port et n’ajoute aucun serveur TURN. Le déploiement actuel du gateway est limité au LAN. Jouer depuis un réseau mobile nécessite une connectivité WebRTC adaptée, indépendamment du bon fonctionnement du canal de commande HTTPS.

## Validation du 9 novembre 2026 sur le Pi ARM64

- Compilation de production réussie (trois avertissements webpack de taille/performance).
- Tests Python du relais : trois tests réussis (destinations autorisées, isolation des comptes/expiration, véritable WebSocket avec messages fragmentés).
- Tests JavaScript réussis : chemin natif, secours HTTP, messages ordonnés, fermeture après les envois et absence de rejeu après rupture.
- Sur la route publique HTTPS : ouverture authentifiée, abonnement et réception de deux messages de façade ; présence authentifiée également ouverte.
- Dans l’application sur le LAN, avec tous les WebSockets natifs volontairement refusés : bibliothèque réelle → fiche de la démo CC0 → Jouer → vidéo 256×240 (plus de 200 images décodées), aucune erreur JavaScript. Canaux child, rétro et gateway ouverts par le relais HTTP. Capture `output/demo-https-corrigee.webp` et rapport `output/https-command-validation.json`.
- Vérification séparée du transport multimédia : vidéo et paquets audio reçus en WebRTC.
- Session de démonstration arrêtée par la commande authentifiée `session.stop` (200), aucun conteneur de jeu restant après le test.
- La lecture depuis un réseau mobile extérieur n’est pas validée ; aucun réglage réseau, proxy partagé ou TV n’a été modifié.
