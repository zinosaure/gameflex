import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { castErrorKey, castSender } from "../../engine/stream/onscreen/cast.js";
import { focusElement, useScreenInput } from "../../libs/input.js";
import { useButton } from "../../libs/ui.js";

// ENVOYER UNE PARTIE AU TÉLÉVISEUR DEPUIS CHROME (#124 ; le porteur, 2026-11-26 : « un autre chemin si tu peux. Comme
// ouvrir un navigateur Chrome et faire le cast ») — /cast/<partie>?application=<récepteur>, PUBLIQUE. La fenêtre du PC
// (Electron) n'a aucun envoyeur Cast : son lien `gameflex://cast` ouvre cette page dans le navigateur du système
// (window/main.js), où le SDK de Chrome caste. Le sélecteur de TV demande un geste : un bouton, rien d'autre.
//
// LA MISE EN PAGE EST CELLE DU LIEN D'UNE INVITATION (Invite.jsx, `.login`) : « GameFlex », ce qu'est la page, puis son
// bouton.
export default function Cast() {
    const { t } = useTranslation();
    const button = useButton();
    const { sessionId } = useParams();
    const application = new URLSearchParams(window.location.search).get("application") || "";
    const sender = castSender(false);
    const [casting, setCasting] = useState(sender.casting());
    const [failed, setFailed] = useState("");

    useEffect(() => {
        focusElement(document.querySelector('[data-key="cast-choose"]'), true);
    }, []);

    useScreenInput({});

    const choose = async () => {
        setFailed("");
        try {
            await sender.start(application, sessionId);
            setCasting(true);
        } catch (error) {
            setFailed(error.cancelled ? "" : t(castErrorKey(error)));
        }
    };

    const stop = () => {
        sender.stop();
        setCasting(false);
    };

    return (
        <main className="login">
            <div className="d-flex flex-column gap-3">
                <h1 className="mb-0 fw-bold">GameFlex</h1>
                <div>
                    <h2 className="h4 mb-1">{t("castPage.title")}</h2>
                    <p className="text-body-secondary mb-0">{t(casting ? "castPage.casting" : "castPage.help")}</p>
                </div>
                {failed && <p className="small text-danger mb-0">{failed}</p>}
                <button type="button" className={button("main")} data-nav data-key="cast-choose" onClick={choose}>{t("castPage.choose")}</button>
                {casting && <button type="button" className={button("other")} data-nav data-key="cast-stop" onClick={stop}>{t("castPage.stop")}</button>}
            </div>
        </main>
    );
}
