import React, { useState } from "react";
import * as bs from "react-bootstrap";
import { copyTextToClipboard } from "./utils/share-link.js";

/**
 * UNE VALEUR À RECOPIER AILLEURS — le lien d'un serveur, le jeton d'un compte.
 *
 * Née dans l'onglet « Jetons » de Fédérations (2026-09-02), qui montre un
 * jeton de catalogue une seule fois ; partagée depuis le chantier 20 avec la
 * fiche d'un membre et le Profil, où le jeton d'un compte se recopie pour le
 * coller sur un autre serveur.
 */
export function CopyableValue({ label, value, t, context }) {
    const [isCopied, setCopied] = useState(false);

    const copy = async () => {
        if (await copyTextToClipboard(value)) {
            setCopied(true);

            return;
        }
        // Jamais en silence : sur une origine en clair, l'API du
        // presse-papiers n'existe pas, et un bouton qui ne fait rien se lit
        // comme une application cassée.
        context.toast.open(t("common.copyFailed"), { variant: "warning" });
    };

    return (
        <div className="mb-3">
            <div className="small text-muted mb-1">{label}</div>
            <div className="d-flex gap-2 align-items-start">
                {/* Sélectionnable et qui revient à la ligne : soixante-quatre
                    caractères tronqués ne se recopient pas à la main. */}
                <div
                    className="p-2 rounded bg-body-tertiary font-monospace small user-select-all flex-grow-1"
                    style={{ wordBreak: "break-all" }}
                >
                    {value}
                </div>
                <bs.Button variant={isCopied ? "success" : "secondary"} onClick={copy} title={label}>
                    <i className={`bi ${isCopied ? "bi-check-lg" : "bi-clipboard"}`}></i>
                </bs.Button>
            </div>
        </div>
    );
}
