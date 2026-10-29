import React, { useEffect, useRef, useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../skeleton/skeleton.js";
import { SectionPanel, FormButton } from "./templates/Essentials.jsx";
import { CopyableValue } from "./CopyableValue.jsx";
import { shareLink } from "./utils/share-link.js";
import { can } from "../libs/capabilities.js";
import { invitationQR, readInvitationImage } from "../api/invitations.js";

const Form = Skeleton.Form;

// LES DEUX BOUTS D'UNE INVITATION (chantier 33) : celui qui l'envoie (son QR,
// son lien, de quoi les faire partir) et celui qui la reçoit (où coller le
// lien, où déposer l'image du QR). Partagés par la fédération, le profil et
// « Jouer chez… » : une seule façon de montrer une invitation, une seule de
// l'accepter.

// hh:mm, le jour même ou le lendemain : une invitation dure 12 heures.
const formatTime = (iso) => {
    const date = new Date(iso);
    const pad = (n) => String(n).padStart(2, "0");

    return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}`;
};

/**
 * UNE INVITATION À ENVOYER : son QR, son lien, Télécharger le QR et Partager.
 * Valable 12 heures, une seule fois — l'écran le dit, et que le jeton, lui, ne
 * voyage pas.
 *
 * Dans l'application — la sienne (`appHome`) ou celle d'un invité qui a
 * rejoint cet hôte (`appHomeLink`) —, une WebView ne télécharge rien : le
 * bouton n'y est pas, et Partager y envoie le lien, qui suffit à l'autre.
 */
export function InvitationReveal({ link, expiresAt, title, t, context, onDone }) {
    const [image, setImage] = useState(null);
    const inAppView = can("appHome") || can("appHomeLink");

    useEffect(() => {
        let url = null;
        let cancelled = false;
        invitationQR(link).then((blob) => {
            if (cancelled || !blob) {
                return;
            }
            url = URL.createObjectURL(blob);
            setImage({ url, blob });
        });

        return () => {
            cancelled = true;
            if (url) {
                URL.revokeObjectURL(url);
            }
        };
    }, [link]);

    const download = () => {
        const anchor = document.createElement("a");
        anchor.href = image.url;
        anchor.download = "gameflex-invitation.png";
        document.body.appendChild(anchor);
        anchor.click();
        anchor.remove();
    };

    // Le QR ET le lien quand le navigateur sait partager une image ; le lien
    // seul sinon — il suffit à l'autre.
    const share = async () => {
        const file = image && new File([image.blob], "gameflex-invitation.png", { type: "image/png" });
        if (file && !inAppView && navigator.canShare?.({ files: [file] })) {
            try {
                await navigator.share({ files: [file], text: link, title });
            } catch {
                // La feuille de partage fermée : rien à dire.
            }

            return;
        }
        if (!(await shareLink({ url: link, title, text: title }))) {
            context.toast.open(t("invitations.shareUnavailable"), { variant: "warning" });
        }
    };

    return (
        <SectionPanel label={title} helpText={t("invitations.revealHelp")}>
            <div className="text-center mb-3">
                {image ? (
                    <img
                        src={image.url}
                        alt={t("invitations.qrAlt")}
                        className="rounded"
                        style={{ width: 240, maxWidth: "100%", aspectRatio: "1 / 1" }} />
                ) : (
                    <div className="d-inline-flex align-items-center justify-content-center" style={{ width: 240, height: 240 }}>
                        <bs.Spinner animation="border" />
                    </div>
                )}
            </div>
            <bs.Alert variant="warning" className="d-flex gap-2 align-items-start">
                <i className="bi bi-hourglass-split mt-1"></i>
                <div>{t("invitations.validUntil", { at: formatTime(expiresAt) })}</div>
            </bs.Alert>
            <CopyableValue label={t("invitations.linkLabel")} value={link} t={t} context={context} />
            <div className="d-flex flex-wrap gap-2 justify-content-end">
                {!inAppView && (
                    <bs.Button variant="secondary" disabled={!image} onClick={download}>
                        <i className="bi bi-download me-2"></i>
                        {t("invitations.download")}
                    </bs.Button>
                )}
                <bs.Button variant="primary" onClick={share}>
                    <i className="bi bi-share me-2"></i>
                    {t("invitations.share")}
                </bs.Button>
            </div>
            {onDone && <FormButton callback={onDone} label={t("invitations.done")} />}
        </SectionPanel>
    );
}

/**
 * OÙ COLLER UNE INVITATION : son lien, ou l'image de son QR — une capture reçue
 * par une messagerie. Le lien lu dans l'image remplit le champ `name` du
 * formulaire.
 */
export function InvitationField({ name = "invitation", formContext, label, helpText, t, context }) {
    const [picked, setPicked] = useState({ qr: [] });
    const [isReading, setReading] = useState(false);
    const picker = useRef(null);

    const read = async (file) => {
        setReading(true);
        try {
            const result = await readInvitationImage(file);
            if (!result.ok) {
                context.toast.open(t(`common.errorCodes.${result.code || "noQR"}`, {
                    defaultValue: t("common.errorCodes.noQR"),
                }), { variant: "error" });

                return;
            }
            formContext.setValues({ ...formContext.values, [name]: result.data.text });
            context.toast.open(t("invitations.imageRead"), { variant: "success" });
        } finally {
            setReading(false);
            // LA MÊME IMAGE, UNE SECONDE FOIS : le champ de fichier garde son
            // choix, et un choix identique ne déclenche aucun changement — le
            // second essai ne faisait rien (vu dans l'application, 2026-10-26).
            picker.current?.querySelectorAll("input[type=file]").forEach((input) => {
                input.value = "";
            });
        }
    };

    // Form.File range l'image dans `files` ; elle se lit aussitôt, et la
    // liste se vide : c'est le lien qui compte, pas le fichier.
    const pickContext = {
        values: { qr: [] },
        setValues: () => { },
        files: picked,
        setFiles: (next) => {
            const file = next.qr?.[0];
            setPicked({ qr: [] });
            if (file) {
                read(file);
            }
        },
        rulesets: isReading ? { qr: () => Skeleton.Ruleset.isDisabled(t("invitations.imageReading")) } : {},
    };

    return (
        <>
            <Form.Group label={label} helpText={helpText}>
                <Form.Input
                    name={name}
                    context={formContext}
                    props={{ placeholder: "https://…/invite/…", autoComplete: "off" }} />
            </Form.Group>
            <Form.Group label={t("invitations.imageLabel")} helpText={t("invitations.imageHelp")}>
                <div ref={picker}>
                    <Form.File name="qr" context={pickContext} accept="image/*" />
                </div>
            </Form.Group>
        </>
    );
}
