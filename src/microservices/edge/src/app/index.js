import React, { useEffect } from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter, Navigate, Route, Routes, useMatch } from "react-router-dom";
import { useTranslation } from "react-i18next";
// BOOTSTRAP, la même version que la webapp (5.3.8), compilé depuis ses sources avec le
// thème du launcher (styles/bootstrap.scss) — tout Bootstrap, composants compris : « il faut
// toujours suivre Bootstrap » (le porteur, 2026-11-12). launcher.css vient après, et ne
// garde que ce que Bootstrap ne fournit pas.
import "./styles/bootstrap.scss";
import "bootstrap-icons/font/bootstrap-icons.css";
import "./styles/launcher.css";
import "./i18n.js";
import { AuthProvider, DataProvider, useAuth, useData, usePlatforms } from "./libs/data.jsx";
import { installInput } from "./libs/input.js";
import { registerServiceWorker } from "./libs/push.js";
import { applyLayout } from "./libs/layout.js";
import Login from "./screens/access/Login.jsx";
import Join from "./screens/access/Join.jsx";
import Invite from "./screens/access/Invite.jsx";
import Platforms from "./screens/library/Platforms.jsx";
import Library from "./screens/library/Library.jsx";
import Live from "./screens/live/Live.jsx";
import Settings from "./settings/Settings.jsx";
import Play from "./screens/play/Play.jsx";
import { Toast } from "./shell/Shell.jsx";
import { lastTabPath } from "./libs/tabs.js";
import Skeleton from "./skeleton/skeleton.js";

const CONSOLE_TABS = ["retrogaming", "streamex", "remoteplay"];

// LE LAUNCHER EN BETA (chantier 51, proposition E) : quatre onglets (En direct,
// Retrogaming, Streamex, Remote Play) ; dans un onglet de consoles, une console
// par écran qui défile en vertical, puis sa bibliothèque — ses fiches de jeux,
// une par écran, en vertical aussi. Il tourne À CÔTÉ de la webapp, sur la pile
// de dev, pour essayer la nouvelle interface sur les vraies données. Chaque
// écran est une vraie route : le Retour du navigateur (et celui d'Android)
// remonte naturellement.
//
//   /live                                   En direct (votre partie en cours, puis les autres)
//   /<onglet>/<console>                     l'écran d'une console (onglet : retrogaming, streamex, remoteplay)
//   /<onglet>/<console>/games/<ref>         sa bibliothèque, sur la fiche d'un jeu (?q=, ?source=)
//   /settings                               l'engrenage : la liste des réglages
//   /settings/<entrée>/…                    une page des réglages, et les siennes (Settings.jsx)
//   /play/<session>                         la page d'une partie (Play.jsx, chantier 52 : ?play=1, ?launch=, ?state=) ;
//                                           PUBLIQUE pour la regarder (`#747`, le « watch » grand public : un lien
//                                           partagé, le récepteur Chromecast) — sans connexion, on la regarde seulement
//   /join/<code>                            le lien d'un compte (Join.jsx, chantier 55) : PUBLIQUE, avant la porte de connexion
//   /invite/<code>                          le lien d'une invitation de fédération (Invite.jsx, chantier 56) : PUBLIQUE, avant la porte
//   ailleurs                                le dernier onglet ouvert dans ce navigateur (Retrogaming d'abord)

function Screens() {
    const { t } = useTranslation();
    const { status } = useAuth();
    const { consoles } = useData();
    const plats = usePlatforms();
    const joining = useMatch("/join/:code");
    const inviting = useMatch("/invite/:code");
    const watching = useMatch("/play/:sessionId");

    // Le lien d'un compte s'ouvre connecté ou non (chantier 55) : la personne n'a pas encore de mot de passe.
    if (joining) {
        return <Join code={joining.params.code} />;
    }

    // Le lien d'une invitation de fédération (chantier 56, A3) : il ne se consomme pas ici, la page dit où le coller.
    if (inviting) {
        return <Invite />;
    }

    if (status === "loading") {
        return <div className="splash">{t("games.loading")}</div>;
    }

    // REGARDER UNE PARTIE SANS COMPTE (`#747`) : son identifiant suffit, comme l'ancien /streaming public de la webapp
    // (le récepteur Chromecast l'ouvre ainsi). La page n'a pas de jeton : elle regarde, elle ne joue pas (Play.jsx).
    if (status !== "in" && watching) {
        return (
            <Routes>
                <Route path="/play/:sessionId" element={<Play />} />
            </Routes>
        );
    }

    if (status !== "in") {
        return <Login />;
    }

    if (!plats) {
        return <div className="splash">{t("games.loading")}</div>;
    }

    if (!consoles.length) {
        return <div className="splash">{t("platforms.none")}</div>;
    }

    return (
        <Routes>
            <Route path="/live" element={<Live />} />
            {CONSOLE_TABS.map((tab) => [
                <Route key={tab} path={`/${tab}/:id?`} element={<Platforms key={tab} tab={tab} />} />,
                <Route key={`${tab}-games`} path={`/${tab}/:consoleId/games/:reference?`} element={<Library key={tab} tab={tab} />} />,
            ])}
            <Route path="/settings/*" element={<Settings />} />
            <Route path="/play/:sessionId" element={<Play />} />
            <Route path="*" element={<Navigate to={lastTabPath()} replace />} />
        </Routes>
    );
}

function App() {
    useEffect(() => {
        installInput();
        // Les notifications push : le service worker, là où le navigateur le permet (libs/push.js).
        registerServiceWorker();
        const onResize = () => applyLayout();
        window.addEventListener("resize", onResize);

        return () => window.removeEventListener("resize", onResize);
    }, []);

    // SKELETON (sa copie, app/skeleton/skeleton.js) autour de tout : ses fenêtres de confirmation
    // (`context.modal.open`), partout dans le launcher (chantier 53).
    return (
        <BrowserRouter>
            <Skeleton>
                <AuthProvider>
                    <AuthenticatedData>
                        <div id="app">
                            <Screens />
                            <Toast />
                        </div>
                    </AuthenticatedData>
                </AuthProvider>
            </Skeleton>
        </BrowserRouter>
    );
}

// Les données ne se chargent qu'une fois connecté : la plupart des routes demandent le jeton.
function AuthenticatedData({ children }) {
    const { status, user } = useAuth();

    if (status !== "in") {
        return <DataProvider key="anonymous" idle>{children}</DataProvider>;
    }

    return <DataProvider key={user?.id || "in"}>{children}</DataProvider>;
}

applyLayout();
createRoot(document.getElementById("root")).render(<App />);
