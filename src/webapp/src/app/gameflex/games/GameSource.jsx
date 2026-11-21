import React from "react";
import { isRemoteSource } from "../libs/localLibrary.js";
import { ORIGIN_ICON, ORIGIN_LABEL } from "./helpers.js";

export function GameSource({ rom, t }) {
    const kind = rom.kind || "local";

    return (
        <div className="d-flex align-items-center gap-2">
            <i className={ORIGIN_ICON[kind] || "bi bi-hdd"}></i>
            <span>{t(ORIGIN_LABEL[kind] || "retrogaming.origin.unknown")}</span>
            {isRemoteSource(kind) && rom.source && (
                <span className="text-muted text-truncate">· {rom.source}</span>
            )}
            {/* Un jeu d'un disque interne est un jeu d'ici (chantier 23) : il
                dit seulement sur quel disque il est. */}
            {!isRemoteSource(kind) && rom.disk && (
                <span className="text-muted text-truncate">· {t("retrogaming.origin.disk", { name: rom.disk })}</span>
            )}
        </div>
    );
}
