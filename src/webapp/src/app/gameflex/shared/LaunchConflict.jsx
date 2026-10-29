import React, { useContext } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../skeleton/skeleton.js";
import { stopSession } from "../api/spawnSessions.js";
import { buildStreamingUrl } from "../libs/sessionLinks.js";
import { federatedId } from "../libs/federatedHost.js";
import { useDisplayNames } from "../libs/SessionPool.js";

// UN LANCEMENT REFUSÉ PARCE QU'UNE AUTRE PARTIE EST LÀ — et ce qu'on propose.
//
// Deux refus du kernel, une seule réponse de la page, pour toutes les pages
// qui lancent (le catalogue, les consoles remote play) :
//
//   - « tu joues déjà ailleurs » (writeAlreadyPlaying) : arrêter l'autre
//     partie, puis relancer ;
//   - « ta partie continue sans toi » (writeOwnedGameGoesOn, #276) : d'autres y
//     jouent, et relancer la leur coupait en silence. On propose d'y
//     retourner, ou de la terminer pour tout le monde.
//
// Rend true quand le refus était l'un des deux et qu'un choix a été proposé.
export function handleLaunchConflict({ result, context, t, token, host = null, navigate, retryLaunch }) {
    if (!result?.activeSessionId) {
        return false;
    }

    const other = {
        id: result.activeSessionId,
        platform: result.activeSessionPlatform,
        reference: result.activeSessionReference,
    };
    // CHEZ CELUI QUI A REFUSÉ, pas chez nous. Le conflit vient du serveur
    // qu'on vient d'interroger : l'identifiant qu'il rend est de chez lui.
    // L'arrêter ici ne trouverait rien — ou, deux serveurs frappant les mêmes
    // identifiants, trouverait l'homonyme d'ici et arrêterait la mauvaise
    // partie (même raison que le préfixe `srv<id>:`).
    const endAndRelaunch = async () => {
        const stopResult = await stopSession(other.platform, other.id, token, host);
        if (!stopResult.ok) {
            context.toast.open(t("streaming.conflict.switchFailed"), { variant: "error" });

            return;
        }
        retryLaunch();
    };

    if (result.ownedGameGoesOn) {
        context.modal.open((
            <OwnedGameGoesOn
                t={t}
                title={result.activeSessionTitle}
                players={result.activeSessionPlayers || []}
                onReturn={() => navigate(buildStreamingUrl({
                    sessionId: host ? federatedId(host.serverId, other.id) : other.id,
                    play: true,
                }))}
                onEnd={endAndRelaunch}
            />
        ), { variant: "normal", size: 600 });

        return true;
    }

    context.modal.open(t("streaming.conflict.switchMessage"), {
        variant: "confirm",
        title: t("streaming.conflict.switchTitle"),
        confirmLabel: t("streaming.conflict.switchLabel"),
        cancelLabel: t("app.cancel"),
        onConfirm: endAndRelaunch,
    });

    return true;
}

// Les trois issues, dans la même forme que la fenêtre de sortie
// (QuitConfirmModal, Streaming.jsx) : y retourner — sa place l'attend —, la
// terminer pour tout le monde, ou ne rien faire.
function OwnedGameGoesOn({ t, title, players, onReturn, onEnd }) {
    const context = useContext(Skeleton.context);
    const displayNames = useDisplayNames();
    const names = players.map((username) => displayNames[username] || username).join(", ");

    return (
        <div className="text-center">
            <i className="bi bi-people-fill d-block mb-3 text-primary" style={{ fontSize: "4.5rem" }} />
            <h5 className="modal-title fw-bold mb-2">{t("streaming.conflict.ownedTitle")}</h5>
            <p className="mb-0">{t("streaming.conflict.ownedMessage", { title, players: names })}</p>
            <div className="d-flex gap-0 mt-4 pt-1 border-top" style={{ marginLeft: "-1rem", marginRight: "-1rem", marginBottom: "-1rem" }}>
                <bs.Button
                    variant="link"
                    className="text-decoration-none flex-fill rounded-0 m-0 py-3 border-end"
                    onClick={() => context.modal.close()}
                >
                    {t("app.cancel")}
                </bs.Button>
                <bs.Button
                    variant="link"
                    className="text-decoration-none flex-fill rounded-0 m-0 py-3 border-end"
                    onClick={() => {
                        context.modal.close();
                        onEnd();
                    }}
                >
                    {t("streaming.conflict.ownedEnd")}
                </bs.Button>
                <bs.Button
                    variant="link"
                    className="text-decoration-none flex-fill rounded-0 m-0 py-3 fw-semibold"
                    onClick={() => {
                        context.modal.close();
                        onReturn();
                    }}
                >
                    {t("streaming.conflict.ownedReturn")}
                </bs.Button>
            </div>
        </div>
    );
}
