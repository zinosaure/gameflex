import React from "react";
import { useTranslation } from "react-i18next";
import { AdminMark } from "./AdminMark.jsx";

// LE NOM D'UN COMPTE, ET SON RÔLE QUAND IL ADMINISTRE (le porteur, 2026-11-14 : « quand quelqu'un est administrateur (ou
// root) mettre un icône vert de shield à côté de son nom » ; « Pour les administrateurs on rajoute le shield dans son
// nom ») : le badge vert, défini ICI une fois — la liste des membres, la carte « Mon compte », partout où un nom de
// compte s'affiche. `admin` : son rôle est root ou administrateur (`role_level` 0 ou 1, comme le serveur le juge). Son
// identifiant va dans le sous-titre, seul, un @ devant (`atUsername`).
//
// LE BADGE EST LA COCHE DENTELÉE DE BOOTSTRAP ICONS (le porteur, 2026-11-14, chantier 53, P13 : « Remplace le bouclier
// par un check avec patch-check-fill de bootstrap ») : `bi-patch-check-fill`, vert (`text-success`), la police d'icônes
// du launcher — plus de bouclier.
export function AccountName({ name, admin = false }) {
    const { t } = useTranslation();

    return (
        <>
            {name}
            {admin && <AdminMark label={t("members.roleAdmin")} className="ms-2" />}
        </>
    );
}
