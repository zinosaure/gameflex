import React from "react";
import * as bs from "react-bootstrap";
import { capabilities } from "../../../../libs/capabilities.js";

// LE MIROIR (chantier 29, le porteur, 2026-10-24) — un lien EXTERNE, sans
// rapport avec le projet, que le serveur déclare (GAMEFLEX_MIRROR_URL) et que
// rien ne lit ici : ni liste, ni téléchargement par le serveur. On y va
// soi-même, et ce qu'on y prend se dépose dans le même dossier que là-bas
// (`cores/<console>/`…). Sans lui, rien ne s'affiche.
export default function MirrorNotice({ t, section }) {
    const url = capabilities().mirrorUrl;
    if (!url) {
        return null;
    }

    return (
        <bs.Alert variant="secondary" className="small d-flex align-items-center gap-2">
            <span className="flex-grow-1">{t("admin.storage.mirrorNotice", { section })}</span>
            <bs.Button variant="secondary" size="sm" className="text-nowrap" href={url} target="_blank" rel="noopener noreferrer">
                <i className="bi bi-box-arrow-up-right"></i> {t("admin.storage.mirrorOpen")}
            </bs.Button>
        </bs.Alert>
    );
}
