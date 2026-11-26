import React from "react";
import { useTranslation } from "react-i18next";
import { Route, Routes } from "react-router-dom";
import { Loading } from "../../bricks/Loading.jsx";
import { Note } from "../../bricks/Note.jsx";
import { Section } from "../../bricks/Section.jsx";
import { DesktopPage, DesktopState, useDesktop } from "./shared.jsx";
import Desktop from "./Desktop.jsx";
import Port from "./Port.jsx";
import Location from "./Location.jsx";
import Account from "./Account.jsx";
import Invite from "./Invite.jsx";
import Join from "./Join.jsx";
import JoinedServer from "./join/Server.jsx";
import Disks from "./Disks.jsx";
import DiskAdd from "./disks/Add.jsx";
import Disk from "./disks/Disk.jsx";
import Update from "./Update.jsx";

// LA CONSOLE DU PC (chantier 70, `#803` ; le porteur, 2026-11-24 : « Fait exactement comme l'Apk. Sauf qu'il y a des
// configurations de plus pour le serveur comme port ») : l'écran d'accueil de l'APK (ConsolePanel.java), en pages
// d'edge, servi par le lanceur du PC à la machine seule (cmd/desktop/console.go). Ni compte ni session : elle vaut un
// écran posé sur le PC. LA TABLE DE SES ROUTES, chacune son fichier à son chemin (chantier 81 : « une route = une
// arborescence/fichier ») ; l'état se lit ici une fois, et passe aux pages (DesktopState).
const ROUTES = [
    { path: "port", page: Port },
    { path: "location", page: Location },
    { path: "account", page: Account },
    { path: "invite", page: Invite },
    { path: "join", page: Join },
    { path: "join/:origin", page: JoinedServer },
    { path: "disks", page: Disks },
    { path: "disks/add", page: DiskAdd },
    { path: "disks/:name", page: Disk },
    { path: "update", page: Update },
];

export default function DesktopConsole() {
    const { t } = useTranslation();
    const desktop = useDesktop();
    const { state } = desktop;

    if (!state) {
        return <DesktopPage><Section><Loading /></Section></DesktopPage>;
    }
    if (state.status === 403 || state.status === 0) {
        return <DesktopPage><Section><Note className="mt-0">{t("desktop.localOnly")}</Note></Section></DesktopPage>;
    }

    return (
        <DesktopState.Provider value={desktop}>
            <Routes>
                <Route index element={<Desktop />} />
                {ROUTES.map(({ path, page: Page }) => <Route key={path} path={path} element={<Page />} />)}
            </Routes>
        </DesktopState.Provider>
    );
}
