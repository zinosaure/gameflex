import React from "react";
import * as bs from "react-bootstrap";
import RequiredList from "./RequiredList.jsx";
import MirrorNotice from "./MirrorNotice.jsx";
import { fetchRequiredBios } from "../api/storage-admin.js";

// LES FIRMWARES — listing seul, et c'est un choix, pas un manque.
//
// GameFlex n'en fournit aucun : ils s'extraient de la console qu'on possède.
// Un bouton « télécharger » ne mènerait nulle part, et l'écran dit donc ce qui
// manque et où le poser. Ce qu'on partage, c'est hors du projet (le porteur,
// 2026-10-24 : « dans le repo on a 0 BIOS fourni, mais en fédération on est
// libre de partager ce qu'on veut ») : un serveur fédéré, ou le miroir que
// l'installation déclare.
export default function Bios({ t }) {
    return (
        <>
            <bs.Alert variant="secondary" className="small">
                {t("admin.storage.biosNotice")}
            </bs.Alert>
            <MirrorNotice t={t} section="bios" />
            <RequiredList
                t={t}
                load={fetchRequiredBios}
                section="bios"
                // Matrice #154 : pas de source officielle pour un BIOS —
                // personne ne les distribue légitimement. Ils viennent d'un
                // disque interne (chantier 24), d'un pair fédéré, ou de la
                // console qu'on possède.
                sources={["local", "disks", "peers"]}
                texts={{
                    search: t("admin.storage.searchBios"),
                    found: t("admin.storage.biosFound"),
                    empty: t("admin.storage.noBios"),
                }}
            />
        </>
    );
}
