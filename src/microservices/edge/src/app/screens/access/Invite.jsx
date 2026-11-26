import React, { useEffect } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "react-router-dom";
import { focusElement, useScreenInput } from "../../libs/input.js";
import { useButton } from "../../libs/ui.js";
import { toast } from "../../shell/Shell.jsx";

// LE LIEN D'UNE INVITATION DE FÉDÉRATION OUVERT DANS UN NAVIGATEUR (chantier 33 ; chantier 56, A3 — le porteur,
// 2026-11-14 : « Oui ») — /invite/<code>, PUBLIQUE, comme InvitePage.jsx de la webapp. Par l'appareil photo qui a lu le
// QR, ou d'un appui dans une messagerie. Le lien s'adresse à un serveur GameFlex : cette page dit où le coller, et ne
// consomme rien — seul l'échange de serveur à serveur le fait. Celui qui l'ouvre n'a en général aucun compte ici :
// elle s'affiche avant la porte de connexion (index.js), connecté ou non.
//
// LA MISE EN PAGE EST CELLE DU LIEN D'UN COMPTE (Join.jsx, `.login`) : « GameFlex », puis ce qu'est le lien, les deux
// endroits où le coller (un serveur : Réglages › Fédération › Rejoindre un serveur ; un compte : « Jouer chez … » dans
// le panneau d'un de ses jeux), sa durée, le lien lui-même et son bouton, et la ligne vers la connexion.
export default function Invite() {
    const { t } = useTranslation();
    const navigate = useNavigate();
    const button = useButton();
    const link = window.location.href;

    // À la manette : la lumière sur « Copier le lien ».
    useEffect(() => {
        focusElement(document.querySelector('[data-key="invite-copy"]'), true);
    }, []);

    useScreenInput({});

    const copy = async () => {
        try {
            await navigator.clipboard.writeText(link);
            toast(t("settings.copied"));
        } catch {
            toast(t("settings.copyFailed"));
        }
    };

    return (
        <main className="login">
            <div className="d-flex flex-column gap-3">
                <h1 className="mb-0 fw-bold">GameFlex</h1>
                <div>
                    <h2 className="h4 mb-1">{t("invitations.pageTitle")}</h2>
                    <p className="text-body-secondary mb-0">{t("invitations.pageHelp")}</p>
                </div>
                <ul className="ps-3 mb-0">
                    <li className="mb-2">{t("invitations.pageServer")}</li>
                    <li>{t("invitations.pageAccount")}</li>
                </ul>
                <p className="small text-body-secondary mb-0">{t("invitations.pageValid")}</p>
                <input type="text" className="form-control" data-nav data-key="invite-link" aria-label={t("invitations.pageLink")} value={link} readOnly
                    onFocus={(event) => event.target.select()} />
                <button type="button" className={button("main")} data-nav data-key="invite-copy" onClick={copy}>{t("invitations.pageCopy")}</button>
                <button type="button" className={button("other")} data-nav data-key="invite-login" onClick={() => navigate("/", { replace: true })}>{t("invitations.pageOpen")}</button>
            </div>
        </main>
    );
}
