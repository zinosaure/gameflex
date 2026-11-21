import React, { useState } from "react";
import * as bs from "react-bootstrap";
import { useTranslation } from "react-i18next";
import { copyTextToClipboard } from "../../shared/utils/share-link.js";

// LE LIEN D'UNE INVITATION OUVERT DANS UN NAVIGATEUR (chantier 33) — par
// l'appareil photo qui a lu le QR, ou d'un appui dans une messagerie. Il
// s'adresse à un serveur GameFlex : cette page dit où le coller, et ne
// consomme rien — seul l'échange de serveur à serveur le fait.
//
// Rendue par index.js à la place de tout le reste, connecté ou non : celui qui
// l'ouvre n'a en général aucun compte ici.
export default function InvitePage() {
    const { t } = useTranslation();
    const [isCopied, setCopied] = useState(false);
    const link = window.location.href;

    return (
        <div className="d-flex align-items-center justify-content-center bg-body-tertiary px-3" style={{ minHeight: "var(--gameflex-visible-height)" }}>
            <bs.Card style={{ maxWidth: 440 }} className="shadow-sm w-100">
                <bs.Card.Body className="p-4">
                    <h4 className="fw-bold mb-3 text-center">
                        <i className="bi bi-qr-code me-2"></i>
                        {t("invitations.pageTitle")}
                    </h4>
                    <p className="text-muted">{t("invitations.pageHelp")}</p>
                    <ul className="ps-3">
                        <li className="mb-2">{t("invitations.pageServer")}</li>
                        <li className="mb-2">{t("invitations.pageAccount")}</li>
                    </ul>
                    <p className="small text-muted">{t("invitations.pageValid")}</p>
                    <div
                        className="p-2 mb-3 rounded bg-body font-monospace small user-select-all"
                        style={{ wordBreak: "break-all" }}>
                        {link}
                    </div>
                    <bs.Button
                        variant={isCopied ? "success" : "primary"}
                        className="w-100"
                        onClick={async () => setCopied(await copyTextToClipboard(link))}>
                        <i className={`bi ${isCopied ? "bi-check-lg" : "bi-clipboard"} me-2`}></i>
                        {t("invitations.pageCopy")}
                    </bs.Button>
                </bs.Card.Body>
            </bs.Card>
        </div>
    );
}
