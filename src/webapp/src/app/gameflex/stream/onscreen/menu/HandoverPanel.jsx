// HandoverPanel — WHO do you ask for the pad?
//
// The request itself has always existed (FloatingPanel emits `requestslot`,
// the holder answers a prompt, the result comes back). What was missing is the
// choice: with several pads in play, "the current player" is not a single
// person, and the old code silently picked the FIRST one it found. On a
// four-pad session that meant asking whoever happened to be first in the list,
// with no way to reach the other three.
//
// So this panel lists the people actually holding a pad right now, and asking
// one is one tap. It is only worth showing when there IS a choice — with a
// single holder, FloatingPanel skips the panel and asks directly.
//
// The list comes from `players`, the live roster the page already receives
// (the child's own state for a chantier-4 session, the façade otherwise) —
// never from a fetch of its own, which would show a different truth than the
// pads on screen.
//
// It serves BOTH directions of a hand-over (2026-08-25):
//
//   ask  — a watcher asks a holder for the pad. The holder answers a prompt.
//   give — a holder passes the pad to a watcher, unasked. Nobody answers:
//          giving is not a request, and a controller held out across a sofa
//          is not a negotiation.
//
// The give direction is the one that was missing, and it is why hand-over
// existed in the first place: remote-play has ONE pad and JOIN is
// structurally impossible there, so this is the only way it ever moves.
import React from "react";
import { useDisplayNames } from "../../../libs/SessionPool.js";
import { ListElement, SectionPanel } from "../../../shared/templates/Essentials.jsx";

// Who can be asked: someone holding a pad, who is not you. A watcher has
// nothing to hand over, and asking yourself is not a thing.
// Who can be GIVEN the pad: somebody present, holding nothing, who is not
// you. Reported by the child only (`watchers`); on the v1 path it is empty and
// the entry never appears, which is the honest outcome — that model never knew
// who was watching.
export function giveCandidates(watchers, self) {
    return (watchers || [])
        .filter((username) => username && username !== self)
        .map((username) => ({ username }));
}

export function handoverCandidates(players, self) {
    return (players || []).filter((player) => (
        player.username !== self
        && (player.role === "player" || player.role === "owner")
        // A LENT pad is nobody's to hand over: only its owner takes it back
        // (#274). Asking for it would only ever be refused.
        && !player.lentTo
    ));
}

export function HandoverPanel({
    players,
    watchers = [],
    self = null,
    onAsk,
    onGive,
    // "ask" (a watcher wants the pad) or "give" (a holder passes it on).
    direction = "ask",
    // lending — the one giving is the OWNER, who lends rather than gives:
    // they keep their place and take the pad back when they want (#274).
    lending = false,
    context,
    t,
}) {
    const giving = direction === "give";
    const giveKey = (key) => `streaming.handover.${lending ? key.replace("give", "lend") : key}`;
    const candidates = giving
        ? giveCandidates(watchers, self)
        : handoverCandidates(players, self);
    // A username is an ACCOUNT, a display name is what people call each other.
    // Everything addressed to a human names them the way they chose; the raw
    // username stays for keys, comparisons and the wire.
    const displayNames = useDisplayNames();
    const nameOf = (username) => displayNames[username] || username;

    // Confirmed either way, and for the same reason: both hand a controller to
    // somebody else. Giving takes effect immediately once confirmed — the
    // recipient is not asked, because being handed a controller is not a
    // question.
    const act = (player) => {
        context.modal.open(
            t(giving ? giveKey("giveMessage") : "streaming.handover.askMessage", {
                username: nameOf(player.username),
            }),
            {
                variant: "confirm",
                title: t(giving ? giveKey("giveTitle") : "streaming.participate.handover"),
                confirmLabel: t(giving ? giveKey("give") : "streaming.handover.ask"),
                cancelLabel: t("app.cancel"),
                onConfirm: () => (giving ? onGive : onAsk)(player.username),
            },
        );
    };

    return (
        <SectionPanel>
            {candidates.length === 0 ? (
                // Reachable: everyone can leave between the tap that opened
                // this panel and its first render.
                <p className="text-muted small mb-0">
                    {t(giving ? "streaming.handover.noneToGive" : "streaming.handover.noneToAsk")}
                </p>
            ) : (
                <div className="d-flex flex-column gap-2 mt-3">
                    {candidates.map((player) => (
                        <div key={player.username}>
                            {/* Plain div stack, NOT Row/Col — the Row gutter's
                                negative margins overflow the offcanvas body
                                sideways (the chevron sticks out).
                                Le clic est sur la LIGNE, pas sur la boîte : elle
                                seule prend le focus, et la croix ne s'arrêtait
                                pas sur la boîte (2026-10-21). */}
                            <ListElement
                                onClick={() => {
                                    context.offcanvas.close();
                                    act(player);
                                }}
                                icon="bi bi-controller"
                                label={nameOf(player.username)}
                                helpText={giving
                                    ? t("streaming.handover.isWatching")
                                    : player.role === "owner"
                                        ? t("streaming.handover.isOwner", { slot: player.slot ?? 1 })
                                        : t("streaming.handover.isPlayer", { slot: player.slot ?? "?" })}
                                next={null}
                            />
                        </div>
                    ))}
                </div>
            )}
        </SectionPanel>
    );
}
