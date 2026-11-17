// LES LIENS DE LA PAGE. L'adresse de ce serveur, dans un lien partagé ou un jeton (Membres, Jeton, Invitation), est
// celle que l'hôte déclare (`shareOrigin`), sinon celle de la page (screens/access/Invitation.jsx, `useShareOrigin`) ;
// l'APK de dev se télécharge sur la même origine (/downloads/gameflex.apk). Il n'y a plus de « page de la webapp » à
// viser sur un autre port : edge EST l'interface (chantier 56, P5).

// LE DÉPÔT PUBLIC DE GAMEFLEX, sur GitHub — jamais le dépôt privé : la page « GameFlex » des Réglages (ses documents,
// settings/about/Project.jsx, ProjectPage), la dernière version publiée de l'application (Application Android) et le pied des
// Réglages (Settings.jsx).
export const REPOSITORY_URL = "https://github.com/zinosaure/gameflex";

// LA PAGE BUY ME A COFFEE DU PROJET, celle de la webapp et des documents du dépôt : « À propos » et le pied des Réglages
// (le porteur, 2026-11-14, chantier 53, P13).
export const COFFEE_URL = "https://buymeacoffee.com/zinosaure";

// Un document du dépôt, tel que GitHub l'affiche (sa branche publiée, `main`).
export const repositoryFile = (path) => `${REPOSITORY_URL}/blob/main/${path}`;

// LES GUIDES DU DÉPÔT PUBLIC (`docs/`), nommés ICI une fois : la page « GameFlex » des Réglages les liste tous
// (settings/about/Project.jsx), et chaque page qui explique quelque chose renvoie au sien dans sa section « Guide »
// (ui/Bricks.jsx, GuideSection ; le porteur, 2026-11-16 : « Toujours proposer des liens vers la documentation si
// possible »). Leur nom et leur aide sont `project.<clé>` et `project.<clé>Help`.
export const GUIDES = {
    install: { icon: "bi-box-arrow-in-down", path: "docs/install.md" },
    update: { icon: "bi-arrow-up-circle", path: "docs/update.md" },
    android: { icon: "bi-android2", path: "docs/android.md" },
    consoles: { icon: "bi-joystick", path: "docs/consoles.md" },
    library: { icon: "bi-collection", path: "docs/library.md" },
    controllers: { icon: "bi-controller", path: "docs/controllers.md" },
    emulatorSettings: { icon: "bi-sliders", path: "docs/emulator-settings.md" },
    federation: { icon: "bi-arrow-left-right", path: "docs/federation.md" },
    remotePlay: { icon: "bi-playstation", path: "docs/remote-play.md" },
    remoteAccess: { icon: "bi-globe", path: "docs/remote-access.md" },
    storage: { icon: "bi-device-hdd", path: "docs/storage.md" },
    troubleshooting: { icon: "bi-tools", path: "docs/troubleshooting.md" },
};

// L'adresse d'un guide, et d'un de ses titres (`anchor` : l'ancre que GitHub donne au titre, « add-your-console »).
export const guideUrl = (key, anchor = null) => `${repositoryFile(GUIDES[key].path)}${anchor ? `#${anchor}` : ""}`;

// Suivre un lien comme si on le touchait (l'APK de ce serveur, une page d'un autre site) : la ligne est un bouton, que
// la croix atteint ; elle ouvre le lien qu'un `<a>` portait.
export const followLink = (href, attributes) => {
    const anchor = Object.assign(document.createElement("a"), { href, ...attributes });
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
};

// LES ATTRIBUTS D'UN LIEN VERS UN AUTRE SITE : un autre onglet, sans rien lui transmettre. Dans l'application Android, la
// WebView n'a qu'une fenêtre : MainActivity (src/mobileapp, `shouldOverrideUrlLoading`) envoie toute adresse qui n'est
// ni la sienne ni celle du serveur rejoint au navigateur du téléphone, et l'application reste sur sa page.
export const EXTERNAL = { target: "_blank", rel: "noopener noreferrer" };

// Ouvrir une adresse d'un autre site (une ligne `opens="link"`).
export const openExternal = (href) => followLink(href, EXTERNAL);
