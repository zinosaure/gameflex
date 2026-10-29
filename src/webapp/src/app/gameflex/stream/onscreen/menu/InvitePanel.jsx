// gameflex/shared/InvitePanel.jsx — member list for the "invite someone"
// menu button (all platforms). Members come from auth's
// GET /api/v1/users/members (every member, most recent connection first —
// offline included); the green badge overlays live presence
// (PresenceContext). Clicking a member opens a confirm modal, then
// sendInvite. Whether the invitation becomes "join" or "watch" is decided
// at ACCEPT time (slot availability), never here — see the plan's social
// flow doc.
//
// List markup mirrors Games.jsx's GameCard rows (icon + bold title + muted
// subtitle + chevron) so the panel looks like the rest of the app. `self`
// is the CURRENT user's username — you don't invite yourself.
import React, { useEffect, useState } from "react";
import * as bs from "react-bootstrap";
import { listMembers, sendInvite } from "../../../profile/api.js";
import { usePresence } from "../../../contexts/PresenceContext.jsx";
import { ListElement, SectionPanel } from "../../../shared/templates/Essentials.jsx";

export function InvitePanel({ session, token, self = null, context, t }) {
    const { users: onlineUsers } = usePresence();
    const [members, setMembers] = useState(null);
    const [feedback, setFeedback] = useState(null);

    useEffect(() => {
        let cancelled = false;

        listMembers(token).then((result) => {
            if (cancelled) return;
            if (result.ok) {
                setMembers(result.item?.members || []);
            } else {
                setFeedback({
                    variant: "danger",
                    message: t("streaming.invite.failed"),
                });
            }
        });

        return () => {
            cancelled = true;
        };
    }, [token, t]);

    const onlineSet = new Set(onlineUsers.map((entry) => entry.username));

    const handleInvite = (member) => {
        const displayName = member.display_name || member.username;
        context.modal.open(
            t("streaming.invite.confirmMessage", { username: displayName }),
            {
                variant: "confirm",
                title: t("streaming.invite.title"),
                confirmLabel: t("streaming.invite.send"),
                cancelLabel: t("app.cancel"),
                onConfirm: async () => {
                    setFeedback(null);
                    const result = await sendInvite(
                        token,
                        member.username,
                        session.platform,
                        session.consoleId,
                        session.reference,
                        session.id,
                    );
                    setFeedback(
                        result.ok ? {
                            variant: "success",
                            message: t("streaming.invite.sent", { username: displayName }),
                        } : { variant: "danger", message: t("streaming.invite.failed") },
                    );
                },
            },
        );
    };

    const visibleMembers = (members || []).filter(
        (member) => self == null || member.username !== self,
    );

    return (
        <SectionPanel>
            {feedback && (
                <p className={`small mb-0 text-${feedback.variant}`}>
                    {feedback.message}
                </p>
            )}
            {/* The shareable link used to be duplicated here. It now lives one
                level up, in FloatingPanel's own "Share" section — one tap
                instead of two, and one implementation instead of two (this one
                rebuilt the URL by hand instead of calling buildStreamingUrl,
                and its clipboard call failed silently on a plain-HTTP origin).
                This panel is now only about inviting a MEMBER by notification. */}
            {members === null ? (
                <bs.Spinner animation="border" size="sm" className="align-self-center" />
            ) : visibleMembers.length === 0 ? (
                <p className="text-muted small mb-0">{t("streaming.invite.none")}</p>
            ) : (
                <div className="d-flex flex-column gap-2 mt-3">
                    {visibleMembers.map((member) => {
                        const isOnline = onlineSet.has(member.username);

                        return (
                            <div key={member.id}>
                                {/* Plain div stack, NOT Row/Col — the Row gutter's negative
                                    margins overflowed the offcanvas body sideways (the
                                    chevron stuck out of the panel).
                                    Le clic est sur la LIGNE, pas sur la boîte : elle seule
                                    prend le focus (2026-10-21). */}
                                <ListElement
                                    onClick={() => {
                                        context.offcanvas.close();
                                        handleInvite(member);
                                    }}
                                    icon="bi bi-person"
                                    label={member.display_name || member.username}
                                    helpText={isOnline ? <span className="text-success">{t("streaming.invite.online")}</span> : t("streaming.invite.offline")}
                                    next={null}
                                />
                            </div>
                        );
                    })}
                </div>
            )}
        </SectionPanel>
    );
}
