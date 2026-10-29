import React from "react";
import RequiredList from "./RequiredList.jsx";
import MirrorNotice from "./MirrorNotice.jsx";
import { fetchRequiredCores, downloadCore } from "../api/storage-admin.js";
import { can } from "../../../../libs/capabilities.js";

// LES ÉMULATEURS — ceux que les consoles réclament.
//
// Le bouton descend le core SUR LE SERVEUR, pas dans le navigateur : le
// fichier doit atterrir sur la machine qui fait tourner l'émulation. C'est
// la même chose que fait le téléphone depuis toujours, et depuis que la
// racine froide est inscriptible (2026-09-09) le serveur peut le faire
// aussi — renvoyer l'administrateur vers le site de libretro pour qu'il
// dépose le fichier à la main était le contournement de ce montage-là.
export default function Cores({ t }) {
    return (
        <>
            <MirrorNotice t={t} section="cores" />
            <RequiredList
                t={t}
                load={fetchRequiredCores}
                onDownload={downloadCore}
                section="cores"
                // Matrice #154 : un cœur se trouve dans notre dossier, sur un
                // disque interne (chantier 24), à la source officielle (buildbot),
                // ou chez un pair fédéré. Jamais sur un montage Samba/WebDAV/kDrive,
                // qui ne porte que des ROMs. Et chez un pair seulement si l'hôte
                // peut charger ses cœurs (`peerCores`).
                sources={["local", "disks", "official", ...(can("peerCores") ? ["peers"] : [])]}
                texts={{
                    search: t("admin.storage.searchCore"),
                    found: t("admin.storage.coresFound"),
                    empty: t("admin.storage.noCores"),
                }}
            />
        </>
    );
}
