import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { listNotificationPreferences } from "../../libs/api.js";
import { pushSubscriptionState, secureContext, subscribeToPush, unsubscribeFromPush } from "../../libs/push.js";
import { toast } from "../../shell/Shell.jsx";
import { InfoRow, Note, Rows, Section, SwitchRow } from "../../ui/Bricks.jsx";

// LES NOTIFICATIONS (profile/settings/Notifications.jsx de la webapp), sur la page Application
// (Application.jsx) : l'abonnement de CET appareil (les invitations, même la page fermée) est une tuile en
// tête de la page (usePush) ; cette section dit l'autorisation du navigateur et les types de notification du
// compte — tous grisés, comme dans la webapp : seules les invitations sont envoyées pour l'instant. Sur une
// adresse qui n'est pas sûre (http hors localhost), le navigateur refuse service worker et push : on le dit,
// et la tuile reste grisée plutôt que de faire semblant.

const PERMISSIONS = { granted: "push.permissionGranted", denied: "push.permissionDenied", default: "push.permissionDefault" };
const TYPES = [
    ["general", "push.notifGeneral"],
    ["join_request", "push.notifJoinRequest"],
    ["user_connected", "push.notifUserConnected"],
    ["stream_started", "push.notifStreamStarted"],
    ["maintenance", "push.notifMaintenance"],
];

// L'ABONNEMENT DE CET APPAREIL : son état (`device` : null tant qu'il se lit, true, false, ou « unsupported »),
// et le geste qui l'allume ou l'éteint. `enabled` faux (l'hôte n'a ni comptes ni notifications) : rien ne se lit.
export function usePush(enabled) {
    const { t } = useTranslation();
    const [device, setDevice] = useState(null);
    const permission = typeof Notification !== "undefined" ? Notification.permission : "denied";

    useEffect(() => {
        let cancelled = false;

        if (enabled) {
            pushSubscriptionState().then((state) => !cancelled && setDevice(state)).catch(() => !cancelled && setDevice("unsupported"));
        }

        return () => {
            cancelled = true;
        };
    }, [enabled]);

    const toggle = async (wanted) => {
        setDevice(wanted);

        if (!wanted) {
            await unsubscribeFromPush().catch(() => {});
            toast(t("push.disabledToast"));

            return;
        }
        const result = await subscribeToPush().catch(() => ({ ok: false }));

        if (result.ok) {
            toast(t("push.grantedToast"));

            return;
        }
        setDevice(result.reason === "unsupported" ? "unsupported" : false);
        toast(t({ denied: "push.deniedToast", unsupported: "push.unsupported" }[result.reason] || "push.failedToast"));
    };

    return { device, permission, secure: secureContext(), blocked: device === "unsupported" || permission === "denied", toggle };
}

export function PushSection({ push }) {
    const { t } = useTranslation();
    const [preferences, setPreferences] = useState(null);

    useEffect(() => {
        let cancelled = false;
        listNotificationPreferences().then((answer) => {
            if (!cancelled && answer.ok) {
                setPreferences(Object.fromEntries((answer.items || []).map((item) => [item.notification_type, item.enabled])));
            }
        });

        return () => {
            cancelled = true;
        };
    }, []);

    return (
        <Section title={t("settings.items.push.label")} help={t("settings.items.push.help")}>
            <Rows>
                <InfoRow icon="bi-shield-lock" tone="amber" label={t("push.browserPermission")} value={t(PERMISSIONS[push.permission] || PERMISSIONS.denied)} />
            </Rows>
            {/* Une adresse qui n'est pas sûre : une ligne atténuée, comme les autres raisons de la page (plus d'alerte). */}
            {!push.secure && <Note>{t("push.insecure", { origin: location.origin })}</Note>}
            {push.permission === "denied" && push.secure && <Note>{t("push.permissionDeniedHelp")}</Note>}
            {push.device === "unsupported" && <Note>{t("push.unsupported")}</Note>}
            {preferences && (
                <>
                    <Note>{t("push.notSent")}</Note>
                    <Rows className="mt-3">
                        {TYPES.map(([key, name]) => (
                            <SwitchRow key={key} id={`pu-${key}`} label={t(name)} help={t("push.typeHelp", { type: t(name) })}
                                checked={preferences[key] !== false} disabled onChange={() => {}} />
                        ))}
                    </Rows>
                </>
            )}
        </Section>
    );
}
