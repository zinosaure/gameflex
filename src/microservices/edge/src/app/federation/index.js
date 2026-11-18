import React from "react";
import { createRoot } from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
// LA MÊME APPARENCE QU'EDGE (chantier 61) : son Bootstrap compilé, ses icônes, launcher.css, ses textes.
import "../styles/bootstrap.scss";
import "bootstrap-icons/font/bootstrap-icons.css";
import "../styles/launcher.css";
import "../i18n.js";
import { installInput } from "../libs/input.js";
import { applyLayout } from "../libs/layout.js";
import { Toast } from "../shell/Shell.jsx";
import Skeleton from "../skeleton/skeleton.js";
import App from "./App.jsx";

// L'INTERFACE DE gameflex-federation, construite par la même chaîne qu'edge (`npm run build:federation`) et servie par
// le service lui-même : ses briques, sa connexion et son cadre des Réglages, autour des routes du service.
installInput();
// La mise en page d'edge (portrait, paysage, télévision, et son échelle), suivie au redimensionnement.
applyLayout();
addEventListener("resize", applyLayout);

createRoot(document.getElementById("root")).render(
    <BrowserRouter>
        <Skeleton>
            <App />
            <Toast />
        </Skeleton>
    </BrowserRouter>,
);
