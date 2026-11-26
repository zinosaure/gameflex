import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { FormButton, FormButtons } from "../ui/Forms.jsx";
import { EmptyRow } from "./EmptyRow.jsx";
import { RowText } from "./RowText.jsx";
import { Rows } from "./Rows.jsx";

// LES DERNIÈRES NOTIFICATIONS (le porteur, 2026-11-25 : « Ce ne sera pas du listing classique. Ce qu'on va avoir c'est
// les dernières notifications. Par exemple pour l'accueil on va voir les 10 dernières et dans le menu du streaming les 5
// dernières » ; « Je te conseillerais de créer un composant que pour ça au lieu d'utiliser le composant listing ») —
// écrites ICI une fois, pour l'accueil (En direct) et le menu d'une partie :
//   - une notification ENCORE EN COURS (un téléchargement qui tourne, une demande sans réponse) reste là ;
//   - une notification FINIE (le téléchargement « terminé », la demande répondue ou restée sans réponse, quelqu'un qui a
//     rejoint la partie) part d'elle-même 1 min 30 après (« les éléments vont disparaître d'eux-mêmes après 1m30 après
//     leur interaction ») — une minuterie jusqu'à la prochaine échéance, aucune cadence ;
//   - au-delà de la limite (`limit`), ce sont toujours les dernières changées qui restent : ni pagination, ni recherche.
// `notices` : `[{ key, changedAt, endedAt, row }]` — `changedAt`, la dernière modification (ms) ; `endedAt`, la fin de
// son interaction (ms ; `null` tant qu'elle est en cours) ; `row`, sa ligne (InfoRow, DownloadRow, RequestRow).
export const NOTICE_LINGER_MS = 90 * 1000;

export function RecentNotices({ notices, limit, empty = null }) {
    const { t } = useTranslation();
    const [now, setNow] = useState(() => Date.now());
    const alive = notices.filter((notice) => !notice.endedAt || notice.endedAt + NOTICE_LINGER_MS > now);
    const next = Math.min(...alive.filter((notice) => notice.endedAt).map((notice) => notice.endedAt + NOTICE_LINGER_MS));

    // La prochaine qui part : une minuterie jusqu'à son heure, refaite quand la liste change.
    useEffect(() => {
        if (!Number.isFinite(next)) {
            return undefined;
        }
        const timer = setTimeout(() => setNow(Date.now()), Math.max(0, next - Date.now()) + 20);

        return () => clearTimeout(timer);
    }, [next]);

    const shown = [...alive].sort((left, right) => right.changedAt - left.changedAt).slice(0, limit);

    return (
        <Rows>
            {shown.length ? shown.map((notice) => <React.Fragment key={notice.key}>{notice.row}</React.Fragment>)
                : <EmptyRow icon="bi-bell" text={empty || t("notifications.none")} />}
        </Rows>
    );
}

// UNE DEMANDE, RÉPONDUE SUR PLACE (le porteur, 2026-11-25 : « Titre / Sous-titres / -- / [Ignorer (25%)] [D'accord (75%)].
// Les boutons seront affichés directement dans la notification et juste au-dessus un hr. Comme les FormButton ») : son
// titre, son sous-titre, puis les deux boutons côte à côte (FormButtons, 25/75) — ignorer d'abord, accepter ensuite.
export function RequestRow({ id, label, help = null, acceptLabel, ignoreLabel = null, onAccept, onIgnore }) {
    const { t } = useTranslation();

    return (
        <div className="list-group-item gf-row gf-row-stack">
            <RowText label={label} help={help} />
            <FormButtons split="25/75">
                <FormButton bare id={`${id}-ignore`} type="button" kind="other" label={ignoreLabel || t("notifications.ignore")} isReplaced onClick={onIgnore} />
                <FormButton bare id={`${id}-accept`} type="button" label={acceptLabel} isReplaced onClick={onAccept} />
            </FormButtons>
        </div>
    );
}
