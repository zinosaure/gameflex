import React, { useEffect, useState } from "react";
import * as bs from "react-bootstrap";
import { authHeaders, getStoredToken } from "../../../profile/auth/AuthContext.jsx";
import { screenshotUrl, screenshotPath, listSaveSlots, deleteSaveSlot } from "../../../api/retrogaming.js";
import { streamAuthToken } from "../../../libs/serverBase.js";
import { ListElement, SectionPanel } from "../../../shared/templates/Essentials.jsx";
import { hasLocalLibrary } from "../../../libs/localLibrary.js";


const THUMBNAIL_ASPECT_RATIO = { width: "100px", aspectRatio: "16/9", objectFit: "cover" };

// SaveStateThumbnail — the slot's screenshot, fetched rather than pointed at
// with a plain <img src>. The screenshot route sits behind the JWT
// middleware like the rest of /api/v1/retrogaming, and an <img> cannot carry an
// Authorization header — so the bytes are fetched with one and handed to the
// element as a blob URL. Revoked on unmount: a blob URL pins its data in
// memory until it is.
//
// `source` et `host` — une copie, ou les sauvegardes d'un serveur fédéré
// (chantier 38) : la vignette vit avec la sauvegarde, et se lit avec la
// session qu'on a là-bas.
function SaveStateThumbnail({ consoleId, reference, slot, version, alt = "", source = "", host = null }) {
    const [objectUrl, setObjectUrl] = useState(null);

    useEffect(() => {
        let cancelled = false;
        let created = null;

        // Le jeton de l'hôte que la page vise, s'il en vise un : la capture
        // vit chez lui, comme la sauvegarde (voir `api`, retrogaming.js).
        const url = host
            ? `${host.origin}${screenshotPath(consoleId, reference, slot, source)}`
            : screenshotUrl(consoleId, reference, slot, source);
        fetch(`${url}&v=${version}`, {
            headers: authHeaders(host ? host.token : (streamAuthToken() || getStoredToken())),
        })
            .then((response) => (response.ok ? response.blob() : null))
            .then((blob) => {
                if (cancelled || !blob) return;
                created = URL.createObjectURL(blob);
                setObjectUrl(created);
            })
            .catch(() => { });

        return () => {
            cancelled = true;
            if (created) URL.revokeObjectURL(created);
        };
    }, [consoleId, reference, slot, version, source, host]);

    return (
        <img
            src={objectUrl || undefined}
            alt={alt}
            className="rounded"
            style={{ ...THUMBNAIL_ASPECT_RATIO }}
        />
    );
}

// Save/load-state slot picker (0-9) — its own component, NOT inlined
// straight into the menu content. The whole JSX tree is handed to
// context.offcanvas.open() exactly once, at the moment the menu button is
// clicked (same "frozen snapshot" constraint Games.jsx's GameDetailPanel
// already worked around): state owned by the PARENT that changes AFTER the
// menu opens never makes it back into that already-open panel. Owning
// slots/selectedSlot as THIS component's own local state sidesteps the
// issue — a component's re-renders in response to ITS OWN setState calls
// happen normally, freezing only affects props pushed down from an
// ancestor that never re-renders.
//
// One flex row per slot — #<slot>, thumbnail + timestamp, trash icon —
// then, below the selected slot's own divider, the action area: a single
// full-width Save for an empty slot, or side-by-side Replace/Load for a
// filled one. Slot 0 stays read-only (auto-save slot).
// `savedOnesOnly` — Réglages › Mes sauvegardes : les emplacements remplis
// seulement, et sous celui qu'on touche, les gestes que l'appelant donne
// (`slotActions(slot, info, refresh)`, chantier 38 : backup, envoyer,
// restaurer, télécharger…), ou la seule suppression sans eux. `source` et
// `host` disent quel rangement on regarde : le principal, une copie, ou les
// sauvegardes d'un serveur fédéré.
// UN REFUS DU CŒUR, DIT DANS LA LANGUE DE LA PAGE (#396). Le porteur a lu
// « le cœur a refusé cette sauvegarde : unserialize failed » sans rien pour
// comprendre (2026-10-17) : une sauvegarde PSP écrite par une autre version de
// PPSSPP. Les deux hôtes disent maintenant qui refuse, et qui avait écrit
// quand la sauvegarde le dit ; le reste des erreurs passe tel quel.
function refusalOf(answer, t) {
    if (answer?.code !== "state-refused") {
        return null;
    }

    return answer.stateCore && answer.stateCore !== answer.core
        ? t("retrogaming.streaming.stateRefusedOther", { core: answer.core, stateCore: answer.stateCore })
        : t("retrogaming.streaming.stateRefused", { core: answer.core });
}

export function SaveStatesPanel({ session, username, saveState, loadState, context, t, savedOnesOnly = false, source = "", host = null, slotActions = null }) {
    // LES HOOKS D'ABORD, la condition ENSUITE.
    //
    // Le garde « pas de compte » était un `return` placé AVANT les useState :
    // le composant appelait donc un nombre de hooks différent selon la
    // présence d'un nom, ce que React interdit. Sur un appareil — où il n'y
    // a jamais de compte — ouvrir les sauvegardes faisait tomber toute la
    // page de jeu (signalé le 2026-09-01).
    //
    // Et le message lui-même était faux ici : sur l'appareil, les
    // sauvegardes sont locales et le porteur du téléphone est chez lui. Il
    // n'y a personne à qui se connecter, donc rien à réclamer — c'est
    // l'appelant qui décide en fournissant, ou non, `saveState`/`loadState`.
    const [slots, setSlots] = useState([]);
    const [selectedSlot, setSelectedSlot] = useState(1);
    const isDeviceOwner = hasLocalLibrary();

    const refreshSlots = () => {
        listSaveSlots(session.consoleId, session.reference, host, source).then(
            (result) => {
                if (result.ok) setSlots(result.data || []);
            },
        );
    };
    useEffect(() => {
        refreshSlots();
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, []);

    // Sur le web, sans compte, il n'y a pas de sauvegardes à montrer — le
    // serveur les range PAR utilisateur. Sur l'appareil, il n'y a pas de
    // compte du tout et c'est normal : les fichiers sont là, à côté des jeux.
    if (!username && !isDeviceOwner) {
        return (
            <SectionPanel
                label={t("streaming.settings.saveStates.title")}
                helpText={t("streaming.settings.saveStates.help")}
            >
                <div className="text-muted">{t("streaming.settings.saveStates.loginRequired")}</div>
            </SectionPanel>
        );
    }

    const handleSaveState = (slot) => {
        context.offcanvas.close();
        saveState(slot, (error) => {
            context.toast.open(error || t("retrogaming.streaming.stateSaved"), {
                variant: error ? "error" : "normal",
            });
            if (!error) refreshSlots();
        });
    };

    const handleLoadState = (slot) => {
        context.offcanvas.close();
        loadState(slot, (error, _info, answer) => {
            context.toast.open(error ? (refusalOf(answer, t) || error) : t("retrogaming.streaming.stateLoaded"), {
                variant: error ? "error" : "normal",
            });
        });
    };

    const handleDeleteSlot = (slot) => {
        context.modal.open(t("retrogaming.streaming.deleteStateConfirm"), {
            variant: "confirm",
            title: t("retrogaming.streaming.slotLabel", { slot }),
            confirmLabel: t("app.delete"),
            cancelLabel: t("app.cancel"),
            onConfirm: async () => {
                const result = await deleteSaveSlot(
                    session.consoleId,
                    session.reference,
                    slot,
                    source,
                );
                if (!result.ok) {
                    context.toast.open(t("retrogaming.streaming.stateSaveFailed"), {
                        variant: "error",
                    });
                    return;
                }
                if (selectedSlot === slot) {
                    setSelectedSlot(slot === 1 ? 2 : 1);
                }
                // L'emplacement se vide, SUR PLACE (#295).
                setSlots((current) => current.filter((entry) => entry.slot !== slot));
            },
        });
    };

    const currentSlotInfo = slots.find((s) => s.slot === selectedSlot);
    // Slot 0 stays read-only HERE, exactly as before — it is not a slot the
    // player picks. What changed (2026-08-24) is who writes it: the child no
    // longer auto-saves on every teardown (it filled the store with states
    // for games merely opened for ten seconds), so slot 0 now holds the
    // player's own "save on quit", written only when they ask for it.
    const isQuitSlot = (slot) => slot === 0;

    return (
        <SectionPanel>
            {Array.from({ length: 10 }, (_, slot) => {
                const info = slots.find((s) => s.slot === slot);
                const isSelected = slot === selectedSlot;

                if (savedOnesOnly && !info?.exists) {
                    return null;
                }

                return (
                    <ListElement
                        key={slot}
                        icon={info?.exists ? "bi bi-check-circle-fill" : "bi bi-circle"}
                        onClick={() => setSelectedSlot(slot)}
                        next={info?.exists ? (
                            <SaveStateThumbnail
                                consoleId={session.consoleId}
                                reference={session.reference}
                                slot={slot}
                                version={info.timestamp}
                                source={source}
                                host={host}
                            />
                        ) : (
                            <div
                                className="rounded bg-body-tertiary d-flex align-items-center justify-content-center text-muted small"
                                style={{ ...THUMBNAIL_ASPECT_RATIO }}
                            >
                                <i className="bi bi-camera-fill fs-2"></i>
                            </div>
                        )}

                        // No bold "title" here — the slot number + thumbnail
                        // (icon) already carry the row's visual weight, same
                        // as before this used ListElement. The timestamp goes
                        // in subtitle instead, so it keeps its original
                        // muted/small treatment rather than becoming the
                        // row's headline text.
                        label={
                            // LA LARGEUR FIXE CASSAIT LE LIBELLÉ DU SLOT 0 : « (à la
                            // sortie) » ne tient pas dans 2,5 rem et se brisait à chaque
                            // espace — quatre lignes, vu en capture le 2026-09-18. Ça ne
                            // se voyait que sur ce slot, le seul qui porte une mention.
                            // `minWidth` garde l'alignement des numéros — la raison de
                            // cette largeur, dite juste au-dessus — sans enfermer le
                            // texte, et `text-nowrap` tient la mention sur une ligne.
                            <div className="fw-semibold text-nowrap" style={{ minWidth: "2.5rem" }}>
                                #{slot + 1}
                                {isQuitSlot(slot) && (
                                    <span className="small text-muted ms-2">
                                        ({t("retrogaming.streaming.autoSlot")})
                                    </span>
                                )}
                            </div>
                        }
                        helpText={
                            info?.timestamp ? new Date(info.timestamp).toLocaleString() : "—"
                        }
                    >
                        {isSelected && (
                            <div className="pt-2 mt-2 border-top">
                                {savedOnesOnly && slotActions ? (
                                    slotActions(selectedSlot, currentSlotInfo, refreshSlots)
                                ) : savedOnesOnly ? (
                                    <bs.Button
                                        variant="danger"
                                        className="w-100"
                                        onClick={() => handleDeleteSlot(selectedSlot)}
                                    >
                                        {t("app.delete")}
                                    </bs.Button>
                                ) : currentSlotInfo?.exists ? (
                                    <div className="d-flex gap-2">
                                        <bs.Button
                                            variant="warning"
                                            className="w-50"
                                            disabled={isQuitSlot(selectedSlot)}
                                            onClick={() => handleSaveState(selectedSlot)}
                                        >
                                            {t("app.overwrite")}
                                        </bs.Button>
                                        <bs.Button
                                            variant="info"
                                            className="w-50"
                                            onClick={() => handleLoadState(selectedSlot)}
                                        >
                                            {t("app.load")}
                                        </bs.Button>
                                    </div>
                                ) : (
                                    <bs.Button
                                        variant="success"
                                        className="w-100"
                                        disabled={isQuitSlot(selectedSlot)}
                                        onClick={() => handleSaveState(selectedSlot)}
                                    >
                                        {t("app.save")}
                                    </bs.Button>
                                )}
                            </div>
                        )}
                    </ListElement>
                );
            })}
        </SectionPanel>
    );
}
