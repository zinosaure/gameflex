import React from "react";
import { useTranslation } from "react-i18next";
import * as bs from "react-bootstrap";
import { useAuth } from "./profile/auth/AuthContext.jsx";

// L'ACCUEIL DE LA WEBAPP.
//
// Il montrait les parties en cours — la sienne à reprendre, celles des autres à regarder ou rejoindre —,
// toutes ouvertes dans la page de jeu de la webapp. Cette page n'existe plus (chantier 52, P8, décision du
// porteur du 2026-11-13) : on joue, on regarde et on rejoint depuis le launcher. L'accueil le dit, au lieu
// de proposer des parties qu'on ne pourrait pas ouvrir.
export default function Dashboard() {
    const { t } = useTranslation();
    const { user } = useAuth();

    return (
        <bs.Container className="pt-3">
            <div className="text-center mb-5">
                <div style={{ fontSize: "5rem" }} className="mb-2">
                    <i className="bi bi-controller"></i>
                </div>
                <h1 className="fw-bold mb-2">
                    {t("dashboard.greeting", { name: user?.display_name || user?.username })}
                </h1>
                <p className="text-muted mb-0">{t("dashboard.playInLauncher")}</p>
            </div>
        </bs.Container>
    );
}
