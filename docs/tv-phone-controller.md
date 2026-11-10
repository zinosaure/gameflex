# Télévision et manette téléphone

## Utilisation

1. Ouvrir GameFlex sur le navigateur de la TV et se connecter au compte habituel.
2. Dans le centre de contrôle ou les paramètres, activer **Mode TV**. Ce choix est enregistré sur cet appareil. `?tv=1` permet aussi d’ouvrir directement la présentation TV. Les marges sont de 5 %, les textes et commandes sont agrandis, le focus est visible à la télécommande. Un bouton plein écran est proposé quand le navigateur le permet.
3. Ouvrir **Manette téléphone**, puis **Afficher un code**.
4. Sur le téléphone, ouvrir l’adresse affichée (`/gameflex/controller` sur l’instance publique). Saisir le code à huit chiffres et un nom de téléphone. Aucun compte n’est nécessaire sur cette page.
5. Autoriser explicitement le téléphone sur la TV. Fermer le panneau avec **Continuer sur la TV**, puis sélectionner une console et lancer un jeu.

La partie reste affichée sur la télévision ; le téléphone affiche uniquement la manette. Ce parcours ne fait pas de recopie vidéo du téléphone. Croix et A naviguent dans la bibliothèque ; B revient à l’accueil/ferme le panneau. En jeu : croix, A/B/X/Y, L1/L2/R1/R2, L3/R3, Select/Start et deux sticks. **Menu TV** rouvre l’association et suspend les entrées pendant son affichage. Une seule manette téléphone est associée à une TV ; elle utilise le joueur/slot déjà détenu par la TV et ne crée pas une place multijoueur supplémentaire.

L’intégration des entrées utilise les canaux existants de l’émulation rétro (libretro) et des consoles Streamex avec profil disponible. Le Remote Play natif n’est pas intégré à cette première version. La manette physique, les comptes, les sessions et les sauvegardes ne sont pas remplacés.

## Connexion et limites

Le code expire au bout de 120 secondes et n’est utilisable qu’une fois. Un code saisi ne suffit pas : la TV doit confirmer le téléphone. Les commandes passent par un WebSocket du même serveur, sous le préfixe existant ; aucun port supplémentaire, service de découverte ou serveur tiers n’est requis. Un Wi-Fi commun réduit la latence ; une connexion mobile distante ajoute la latence de son réseau. Le navigateur doit supporter WebSocket et Pointer Events. Les modes portrait et paysage sont prévus ; le plein écran et le maintien de l’écran éveillé dépendent des API du navigateur.

À la libération d’un doigt, une annulation du toucher, un changement de visibilité ou une déconnexion, les boutons et sticks sont remis au neutre. Une absence de mises à jour relâche aussi les entrées après environ une seconde. Une page téléphone suspendue peut reprendre sa connexion ; si l’association a été révoquée/expirée, elle revient à la saisie d’un code. Le changement d’écran invalide les anciennes commandes en attente.

Le composant ne modifie ni ne contourne le garde d’autorité du moteur : le TV doit déjà détenir le contrôle de sa session. Les menus suspendent les entrées de jeu via le garde partagé existant. Les commandes de navigation ne sont jamais présentées comme des frappes de clavier fiables au moteur.

## Association

- Création et approbation : utilisateur authentifié de la TV + capacité hôte.
- Téléphone : capacité aléatoire limitée à cette manette ; jamais de JWT du compte transmis au téléphone.
- Capacités absentes des URL, des journaux et des captures ; le registre du serveur ne conserve que leurs empreintes. Le code illustré dans les captures de test est révoqué à la fin des essais.
- Quatre associations actives maximum sur le serveur (deux par compte). Appairages et entrées bornés en mémoire, codes/associations expirables, contrôle d’origine WebSocket et limites de tentatives/débit.
- Les messages autorisés sont uniquement des instantanés de boutons/sticks, des informations de contexte hôte et des battements de présence. Le téléphone ne dispose d’aucune commande serveur arbitraire. Sa navigation est bloquée sur les réglages du compte ; seul le retour à l’accueil reste accessible. Les actions du compte restent à la télécommande de la TV.
- Une déconnexion TV limite la reprise à une minute ; un redémarrage de l’API impose une nouvelle association.

API : `POST /v1/phone-pad/create`, `/join`, `/{id}/approve`, `/{id}/revoke` et `WS /v1/phone-pad/{id}/ws`. Le proxy web conserve Host/Origin et transmet la mise à niveau WebSocket. Les autres routes et services restent inchangés.

## Validation

Tests unitaires/HTTP : `PYTHONPATH=tools/pi-tv python3 -m unittest discover -s tests/pi-tv -p 'test_phone_pad*.py'` (wsproto requis, déjà présent dans l’image API).

La prévisualisation dédiée (`tests/pi-tv/serve-tv-phone-preview.py`) utilise le vrai relais HTTP/WebSocket, mais exclusivement une identité et un jeu simulés. Ne jamais utiliser ce serveur de test pour la production. `tests/pi-tv/tv-phone-browser.py` ouvre deux processus Chromium séparés : TV 1920 × 1080, téléphone 844 × 390 et 390 × 844. Il vérifie l’approbation, la navigation, le multitouch, les sticks, la suspension brutale du JavaScript, le relâchement et la révocation. Les parcours de bibliothèque restent couverts par `consoles-first-browser.py`.

Les essais navigateur ne valent pas un essai sur un téléviseur physique ou une validation de l’APK Android TV. Le mode livré ici est celui de l’application web. Aucun APK ni firmware TV n’est installé par cette modification.

Références techniques : [Pointer Events et capture des doigts](https://developer.mozilla.org/en-US/docs/Web/API/Pointer_events), [annulation d’un toucher](https://developer.mozilla.org/en-US/docs/Web/API/Element/pointercancel_event), [protocole WebSocket wsproto](https://python-hyper.org/projects/wsproto/en/stable/api.html).
