import React, { useState, useEffect } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../skeleton/skeleton.js";
import { listNotificationPreferences, saveNotificationPreference } from "../api.js";
import { pushSubscriptionState, subscribeToPush, unsubscribeFromPush } from "../../contexts/push.js";
import { SectionPanel } from "../../../gameflex/shared/templates/Essentials.jsx";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// The browser permission is READ-ONLY as far as this page is concerned, so
// it's shown as a status badge, not a switch. A page can only ever ASK for
// the permission (default → granted/denied); nothing in JavaScript can
// revoke a granted one — that's the user's call, in the browser's own site
// settings. It used to be a switch, which promised an "off" the browser
// never honoured: it flipped to Off, then snapped back to On on the next
// page load (confirmed live), because "on" was re-derived from the still-
// granted permission every time.
//
// CET APPAREIL — l'abonnement push de ce navigateur, la seule chose qui fait
// arriver une notification quand la page est fermée (remis le 2026-10-15 :
// « push.js : il faut le remettre »). L'interrupteur demande lui-même
// l'autorisation ; le bouton qui ne faisait que la demander, sans jamais
// s'abonner, est parti avec.
//
// The 5 type toggles below are a separate, per-account concern
// (NotificationPreference) — and today the server sends none of them: the
// only push that leaves it is an invitation. They stay listed, disabled and
// saying so, rather than offering switches that change nothing.
const PUSH_PERMISSIONS = {
    granted: { bg: "success", labelKey: "profile.permissionGranted" },
    denied: { bg: "danger", labelKey: "profile.permissionDenied" },
    default: { bg: "secondary", labelKey: "profile.permissionDefault" },
};

export default function Notifications({ context, token, t }) {
    const [values, setValues] = useState({});
    const [prefsLoaded, setPrefsLoaded] = useState(false);
    // null tant qu'on ne sait pas ; "unsupported", true ou false ensuite.
    const [device, setDevice] = useState(null);
    // Read once at render: it can't change under us from this page (only
    // from browser settings, which reloads the tab anyway).
    const permission = typeof Notification !== "undefined" ? Notification.permission : "denied";
    const permissionStatus = PUSH_PERMISSIONS[permission] || PUSH_PERMISSIONS.denied;
    // Order/labels shown in this page's notification-types list — keys must
    // match the auth service's NOTIFICATION_TYPES exactly.
    const NOTIFICATION_TYPES = [
        { key: "general", labelKey: "profile.notifGeneral" },
        { key: "join_request", labelKey: "profile.notifJoinRequest" },
        { key: "user_connected", labelKey: "profile.notifUserConnected" },
        { key: "stream_started", labelKey: "profile.notifStreamStarted" },
        { key: "maintenance", labelKey: "profile.notifMaintenance" },
    ];

    useEffect(() => {
        let cancelled = false;
        pushSubscriptionState()
            .then((state) => !cancelled && setDevice(state))
            .catch(() => !cancelled && setDevice("unsupported"));

        return () => { cancelled = true; };
    }, []);

    const deviceForm = {
        values: { thisDevice: device === true },
        setValues: async (v) => {
            const wanted = Boolean(v.thisDevice);
            if (wanted === (device === true)) {
                return;
            }
            setDevice(wanted);
            if (!wanted) {
                await unsubscribeFromPush(token).catch(() => { });
                context.toast.open(t("profile.pushDisabledToast"), { variant: "success" });

                return;
            }
            const result = await subscribeToPush(token).catch(() => ({ ok: false }));
            if (result.ok) {
                context.toast.open(t("profile.permissionGrantedToast"), { variant: "success" });

                return;
            }
            setDevice(result.reason === "unsupported" ? "unsupported" : false);
            const message = {
                denied: "profile.permissionDeniedToast",
                unsupported: "profile.pushUnsupported",
            }[result.reason] || "profile.pushFailedToast";
            context.toast.open(t(message), { variant: "danger" });
        },
        files: {},
        setFiles: () => { },
        rulesets: {
            thisDevice: () => {
                if (device === "unsupported") {
                    return Ruleset.isDisabled(t("profile.pushUnsupported"));
                }
                if (permission === "denied") {
                    return Ruleset.isDisabled(t("profile.permissionDeniedHelp"));
                }

                return null;
            },
        },
    };

    useEffect(() => {
        let cancelled = false;

        listNotificationPreferences(token).then((result) => {
            if (cancelled || !result.ok) {
                return;
            }

            const next = {};
            for (const item of result.items || []) {
                next[item.notification_type] = item.enabled;
            }

            setValues(next);
            setPrefsLoaded(true);
        }).catch((failure) => {
            // ON NE MARQUE PAS « CHARGÉ » ICI, ET C'EST VOULU.
            //
            // Le bloc d'interrupteurs n'apparaît que sur `prefsLoaded`. Le
            // poser après un échec afficherait cinq positions OFF comme si
            // c'était l'état du compte — alors qu'on n'a rien pu lire, et que
            // l'utilisateur écrirait ensuite en croyant partir d'un état
            // connu. Un écran muet vaut mieux qu'un écran qui ment.
            console.error("échec du chargement des préférences de notification", failure);
        });

        return () => { cancelled = true; };
    }, [token]);

    // Each switch saves immediately on toggle (no separate Save button) —
    // reverts the optimistic flip if the PUT fails, same pattern as
    // Games.jsx's heart-button toggle.
    const contextForm = {
        values,
        // Form.SkeletonSwitch's onChange always calls this with the FULL
        // merged object ({...context.values, [name]: newValue} — see
        // skeleton/libs/Form.jsx), not just the one entry that changed.
        // Object.entries(v)[0] used to grab whichever key happened to be
        // FIRST in insertion order (set by the order listNotificationPreferences'
        // own API response came back in, not NOTIFICATION_TYPES' declared
        // order) — overwriting an EXISTING key doesn't move it, so that was
        // only ever the actually-toggled one when it happened to already be
        // first (confirmed live: exactly one type worked, whichever the API
        // happened to list first, every other switch silently no-opped).
        // Diffing against the current `values` finds the one that actually
        // changed instead, regardless of its position.
        setValues: (v) => {
            const changed = Object.entries(v).find(([key, val]) => values[key] !== val);
            if (!changed) return;
            const [notificationType, enabled] = changed;
            setValues((prev) => ({ ...prev, [notificationType]: enabled }));
            saveNotificationPreference(token, notificationType, enabled).then((result) => {
                if (!result.ok) {
                    setValues((prev) => ({ ...prev, [notificationType]: !enabled }));
                    context.toast.open(t("profile.notificationsSaveFailed"), { variant: "danger" });
                }
            }).catch((failure) => {
                console.error("échec de l'enregistrement d'une préférence de notification", failure);
                setValues((prev) => ({ ...prev, [notificationType]: !enabled }));
                context.toast.open(t("profile.notificationsSaveFailed"), { variant: "danger" });
            });
        },
        files: {},
        setFiles: () => { },
        rulesets: NOTIFICATION_TYPES.reduce((acc, { key }) => {
            acc[key] = () => Ruleset.isDisabled(t("profile.notifTypeNotSent"));

            return acc;
        }, {}),

    };

    return (
        <SectionPanel>
            <div className="mb-3">
                <div className="d-flex align-items-center justify-content-between py-2">
                    <div>
                        <span className="fw-semibold">{t("profile.browserPermission")}</span>
                        <br />
                        <span className="small text-muted">{t("profile.browserPermissionHelp")}</span>
                    </div>
                    <bs.Badge bg={permissionStatus.bg} className="rounded-pill px-3 py-2" style={{ fontSize: "0.75rem" }}>
                        {t(permissionStatus.labelKey)}
                    </bs.Badge>
                </div>
                {permission === "denied" && (
                    <span className="d-block small text-muted">{t("profile.permissionDeniedHelp")}</span>
                )}
            </div>

            {device !== null && (
                <Form.Group>
                    <Form.SkeletonSwitch
                        reverse={true}
                        name="thisDevice"
                        context={deviceForm}
                        label={t("profile.pushThisDevice")}
                        helpText={t("profile.pushThisDeviceHelp")}
                        isDisabled={device === "unsupported" || permission === "denied"} />
                </Form.Group>
            )}

            {prefsLoaded && (
                <Form.Group>
                    {NOTIFICATION_TYPES.map(({ key, labelKey }) => (
                        <Form.SkeletonSwitch
                            key={key}
                            reverse={true}
                            name={key}
                            context={contextForm}
                            label={t(labelKey)}
                            helpText={t("profile.notifTypeHelp", { type: t(labelKey) })}
                            isDisabled={true} />
                    ))}
                </Form.Group>
            )}
        </SectionPanel>
    );
}