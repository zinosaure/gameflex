import React, { useContext, useEffect, useState } from "react";

// Same default the presets use, so an unset size reads as what is drawn.
const DEFAULT_BUTTON_SIZE = 50;
import { useTranslation } from "react-i18next";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import { useAuth } from "../../auth/AuthContext.jsx";
import ControllerLayout from "../../../stream/onscreen/Controllers.jsx";
import { CONTROLLER_PRESETS } from "../../../controllers/presets.js";
import { listButtonPositioning, saveButtonPositioning, deleteButtonPositioning } from "../../api.js";
import {
    OffCanvas,
    SectionPanel
} from "../../../shared/templates/Essentials.jsx";
const Form = Skeleton.Form;

// Screen orientation, mirrored from ControllerLayout's own internal
// isLandscape check (window.innerWidth > window.innerHeight) — purely for
// the toolbar's read-only indicator below, so it's obvious which coordinate
// set (button.x/y for portrait, button.xl.x/xl.y for landscape) a drag is
// about to write to (see ControllerLayout's own handleButtonDrag
// doc). Rotate the device/resize the window to edit the other orientation —
// there's no in-page toggle, since what's being edited must always match
// what's actually rendered right now (same reasoning as that component
// picking real vs. mocked-up rendering).
// Portrait and landscape are TWO stored positions, not one scaled by the
// screen (button.x/y versus button.xl.*). So the form always reads and writes
// the set that matches what is on screen right now — editing coordinates you
// cannot see would be worse than not offering the field at all.
//
// `xl` is optional in the presets: a button that never got a landscape
// position falls back to its portrait one, which is exactly what the renderer
// does, so the number shown is the number in effect.
function readGeometry(button, isLandscape) {
    const source = isLandscape ? { ...button, ...(button.xl || {}) } : button;

    return {
        x: Number(source.x ?? 0),
        y: Number(source.y ?? 0),
        size: Number(source.size ?? DEFAULT_BUTTON_SIZE),
    };
}

function writeGeometry(button, isLandscape, patch) {
    if (!isLandscape) {
        return { ...button, ...patch };
    }

    // Merged onto the CURRENT effective values, so nudging x on a button with
    // no landscape entry does not leave y and size undefined behind it.
    return { ...button, xl: { ...readGeometry(button, true), ...patch } };
}

// A GROUP's geometry is derived, never stored. presets.js gives each button a
// `group` (the d-pad's eight, the face buttons, the shoulders); the group
// itself has no row anywhere and never will. What the form shows is a handle:
// the bounding box's origin, which is stable, and the shared size when every
// member agrees on one.
//
// Origin rather than centre on purpose — a centre drifts when a single member
// is nudged, so the number under the cursor would change without anyone having
// touched the group.
function groupGeometry(members, isLandscape) {
    const positions = members.map((button) => readGeometry(button, isLandscape));
    const sizes = new Set(positions.map((p) => p.size));

    return {
        x: Math.min(...positions.map((p) => p.x)),
        y: Math.min(...positions.map((p) => p.y)),
        // null when they differ: showing one member's size as the group's
        // would be a number that lies about the other seven.
        size: sizes.size === 1 ? [...sizes][0] : null,
    };
}

// Moving a group keeps its SHAPE: one delta for everyone, clamped so no member
// leaves the play area (the same rule the drag follows — see Controllers.jsx).
// Coordinates are RELATIVE (0..1) and reached by adding deltas, so they drift:
// 0.45 moved by -0.35 lands on 0.10000000000000003, which is what the field
// then shows and what the next delta compounds. Four decimals is far finer
// than the 0.01 step anyone edits with, and far coarser than the noise.
const round = (value) => Math.round(value * 10000) / 10000;

function moveGroup(members, isLandscape, patch, current) {
    let dx = patch.x === undefined ? 0 : patch.x - current.x;
    let dy = patch.y === undefined ? 0 : patch.y - current.y;

    for (const member of members) {
        const at = readGeometry(member, isLandscape);
        dx = Math.min(Math.max(dx, -at.x), 1 - at.x);
        dy = Math.min(Math.max(dy, -at.y), 1 - at.y);
    }

    return members.map((member) => {
        const at = readGeometry(member, isLandscape);
        const next = { x: round(at.x + dx), y: round(at.y + dy) };
        // Size, unlike position, applies as an ABSOLUTE to every member: the
        // point of setting a group's size is to make them agree.
        if (patch.size !== undefined) {
            next.size = patch.size;
        }

        return writeGeometry(member, isLandscape, next);
    });
}

function useIsLandscape() {
    const [isLandscape, setIsLandscape] = useState(window.innerWidth > window.innerHeight);
    useEffect(() => {
        const handleResize = () => setIsLandscape(window.innerWidth > window.innerHeight);
        window.addEventListener("resize", handleResize);
        return () => window.removeEventListener("resize", handleResize);
    }, []);
    return isLandscape;
}

// Button-layout editor, opened as a full-viewport offcanvas from the
// Positioning tab (profile/GamepadParameters.jsx) — it used to be a
// standalone full-screen route, which meant leaving the settings page
// entirely to reposition a button and navigating back afterwards.
//
// Dragging repositions buttons through ControllerLayout's own
// `editable` mode, not a separate implementation, so what's edited here IS
// exactly what a real session renders; save/slot controls are wired to the
// ButtonPositioning backend rather than localStorage. The button SET itself is
// NOT editable — deliberately, not an oversight: RETRO_BUTTON_INDEX/
// RETRO_GAMEPAD_INDEX (Controllers.jsx) are fixed tables keyed by the
// preset's own button names, so an arbitrary added button would have no
// index to actually send to the server. The number/identity of buttons stays
// exactly what CONTROLLER_PRESETS[controllerName] defines; only x/y is ever
// edited.
//
// controllerName is a prop: the tab that opens this panel has already asked
// which controller, so asking again here would be a second source of truth
// for the same choice.
const SLOTS = [1, 2, 3, 4, 5];

export default function ButtonPositioningPanel({ controllerName = "dualshock" }) {
    const { t } = useTranslation();
    const context = useContext(Skeleton.context);
    const { token } = useAuth();
    const isLandscape = useIsLandscape();

    const [slotIndex, setSlotIndex] = useState(1);
    const [slots, setSlots] = useState([]);
    const [layout, setLayout] = useState(() => CONTROLLER_PRESETS.dualshock.buttons.map((b) => ({ ...b })));
    const [selectedName, setSelectedName] = useState(null);
    const [isDefault, setIsDefault] = useState(false);
    const [opacity, setOpacity] = useState(1);
    const [isSaving, setIsSaving] = useState(false);
    // What is selected, not how selection behaves. A mode switch meant the
    // same tap did two different things depending on a control elsewhere on
    // the page — and nothing on screen said which. Here the groups are listed:
    // tap one and it is selected, tap a button and that button is.
    const [selectedGroup, setSelectedGroup] = useState(null);

    // The button being edited, and its live geometry. Derived from `layout`
    // rather than copied into state: a drag writes to `layout`, so the numbers
    // follow the drag instead of drifting out of step with it.
    const selectedButton = layout.find((button) => button.name === selectedName) || null;
    // In group mode the selection is everything sharing the clicked button's
    // group. A button with no group stays alone, so the switch never leaves
    // someone unable to select what they clicked.
    const groupMembers = selectedGroup
        ? layout.filter((button) => button.group === selectedGroup)
        : [];
    const selectedNames = selectedGroup
        ? new Set(groupMembers.map((button) => button.name))
        : null;

    const geometry = selectedGroup
        ? groupGeometry(groupMembers, isLandscape)
        : (selectedButton ? readGeometry(selectedButton, isLandscape) : null);

    // The two selections are exclusive: picking one clears the other, so what
    // moves is always what is highlighted.
    const selectButton = (name) => {
        setSelectedGroup(null);
        setSelectedName(name);
    };

    // Every group present in this layout, in the order its buttons appear —
    // built from the layout itself, so a controller without a d-pad simply
    // does not offer one.
    const groups = [...new Set(layout.map((button) => button.group).filter(Boolean))];
    // The same name in the chip and in the form heading — a heading that said
    // "dpad" beside a chip that said "D-pad" reads as two different things.
    const groupName = (group) => t(`profile.positioning.group.${group}`, {
        defaultValue: group.replace(/_/g, " "),
    });

    const patchSelected = (patch) => {
        if (selectedGroup) {
            setLayout((current) => {
                const members = current.filter((button) => button.group === selectedGroup);
                const moved = new Map(
                    moveGroup(members, isLandscape, patch, geometry).map((button) => [button.name, button]),
                );

                return current.map((button) => moved.get(button.name) || button);
            });
            return;
        }

        setLayout((current) => current.map((button) => (
            button.name === selectedName ? writeGeometry(button, isLandscape, patch) : button
        )));
    };

    const geometryContext = {
        values: geometry
            ? { x: geometry.x, y: geometry.y, size: geometry.size ?? DEFAULT_BUTTON_SIZE }
            : { x: 0, y: 0, size: DEFAULT_BUTTON_SIZE },
        setValues: (v) => {
            // GEOMETRY, not selectedButton: with a group selected there is no
            // single button, and testing for one silently swallowed every edit
            // — the fields showed the group's numbers and refused to change
            // them, which is the worst of both (reported live 2026-08-25).
            if (!geometry) {
                return;
            }
            const patch = {};
            if (Number(v.x) !== geometry.x) patch.x = Number(v.x);
            if (Number(v.y) !== geometry.y) patch.y = Number(v.y);
            // geometry.size is null when a group's members disagree, and any
            // number differs from null — which is what we want: typing a size
            // on a mixed group is how you make them agree.
            if (Number(v.size) !== geometry.size) patch.size = Number(v.size);
            if (Object.keys(patch).length > 0) {
                patchSelected(patch);
            }
        },
        files: {}, setFiles: () => { }, rulesets: {},
    };

    // One context for the whole toolbar — slot, default switch and opacity
    // are three views of the same editing state, so they share it rather
    // than each carrying a hand-rolled onChange.
    const formContext = {
        values: { slotIndex: String(slotIndex), isDefault, opacity },
        setValues: (v) => {
            if (v.slotIndex !== String(slotIndex)) setSlotIndex(Number(v.slotIndex));
            if (v.isDefault !== isDefault) setIsDefault(!!v.isDefault);
            if (v.opacity !== opacity) setOpacity(Number(v.opacity));
        },
        files: {}, setFiles: () => { }, rulesets: {},
    };

    useEffect(() => {
        let cancelled = false;
        listButtonPositioning(token, controllerName).then((result) => {
            if (cancelled) return;
            const items = result.ok ? (result.items || []) : [];
            setSlots(items);
            const existing = items.find((s) => s.slot_index === slotIndex);
            if (existing) {
                setLayout(existing.layout.map((b) => ({ ...b })));
                setIsDefault(existing.is_default);
                setOpacity(existing.opacity ?? 1);
            } else {
                setLayout(CONTROLLER_PRESETS[controllerName].buttons.map((b) => ({ ...b })));
                setIsDefault(false);
                setOpacity(1);
            }
            setSelectedName(null);
        });
        return () => { cancelled = true; };
    }, [token, controllerName, slotIndex]);

    const handleSave = async () => {
        setIsSaving(true);
        // Real gameplay (ControllerLayout's own fetch, non-edit mode)
        // only ever applies a saved layout when SOME slot for this
        // controller is marked is_default — without this, saving a
        // positioning without ever touching the "Par défaut" switch would
        // silently do nothing in Streaming, which is exactly the confusing
        // "my changes aren't applied" experience this guards against: if no
        // OTHER slot already holds the default, this save always becomes it
        // (even if the switch is off), so there's always a default the
        // moment at least one slot exists. Explicitly checking the switch
        // still works normally to move the default to a different slot.
        const hasOtherDefault = slots.some((s) => s.is_default && s.slot_index !== slotIndex);
        const effectiveIsDefault = isDefault || !hasOtherDefault;

        // `isSaving` ne commande pas que le bouton « Enregistrer » : il grise
        // aussi la corbeille ET la flèche qui referme le panneau. Resté à vrai
        // sur un rejet, il enfermait donc l'écran sans aucune sortie — il
        // fallait quitter l'application (même sinistre que Login.jsx, 2026-09-25).
        try {
            const result = await saveButtonPositioning(token, controllerName, slotIndex, layout, effectiveIsDefault, opacity);

            if (!result.ok) {
                context.toast.open(result.exception?.detail || t("profile.positioning.saveFailed"), { variant: "error" });

                return;
            }

            setIsDefault(effectiveIsDefault);
            context.toast.open(t("profile.positioning.saved"), { variant: "success" });
            setSlots((prev) => {
                const rest = prev.filter((s) => s.slot_index !== slotIndex).map((s) => (
                    effectiveIsDefault ? { ...s, is_default: false } : s
                ));
                return [...rest, result.item].sort((a, b) => a.slot_index - b.slot_index);
            });
        } catch (failure) {
            console.error("échec de l'enregistrement de la disposition", failure);
            context.toast.open(t("app.error"), { variant: "error" });
        } finally {
            setIsSaving(false);
        }
    };

    const removeLayout = async () => {
        setIsSaving(true);

        try {
            const result = await deleteButtonPositioning(token, controllerName, slotIndex);

            if (!result.ok) {
                context.toast.open(result.exception?.detail || t("app.error"), { variant: "error" });

                return;
            }

            setSlots((prev) => prev.filter((s) => s.slot_index !== slotIndex));
            setLayout(CONTROLLER_PRESETS[controllerName].buttons.map((b) => ({ ...b })));
            setIsDefault(false);
            setOpacity(1);
        } catch (failure) {
            console.error("échec de la suppression de la disposition", failure);
            context.toast.open(t("app.error"), { variant: "error" });
        } finally {
            setIsSaving(false);
        }
    };

    // UNE CONFIRMATION, COMME PARTOUT AILLEURS (#193).
    //
    // C'était le SEUL geste destructeur de l'application à s'exécuter au premier
    // clic, alors qu'il efface une disposition placée bouton par bouton — et
    // que la corbeille est juste à côté du sélecteur qu'on vient d'utiliser.
    const handleDelete = (event) => {
        event.preventDefault();
        context.modal.open(
            <div className="text-center">{t("profile.positioning.deleteConfirm")}</div>,
            {
                variant: "confirm",
                onConfirm: removeLayout,
                confirmLabel: t("app.delete"),
                cancelLabel: t("app.cancel"),
            },
        );
    };

    const slotOptions = SLOTS.map((s) => {
        const saved = slots.find((sl) => sl.slot_index === s);
        const suffix = saved ? (saved.is_default ? " ★" : "") : ` (${t("profile.positioning.emptySlot")})`;
        return { value: String(s), label: `${t("profile.positioning.slot", { index: s })}${suffix}` };
    });

    return (
        // OffCanvas, like every other panel in the app: it carries the
        // standard header (icon/title/help) and the sticky footer whose
        // arrow closes the panel — this had no way back at all before, since
        // the old standalone page's own Back button went away with the route.
        // Structure d'offcanvas : en-tête + onglets + pied collant (CODE_CONVENTIONS.md).
        //
        // No hardcoded background/text colours either: this renders inside
        // the offcanvas, which already follows the active theme — the old
        // bg-black + text-white were only right in dark mode.
        <OffCanvas
            icon="bi bi-arrows-move"
            label={CONTROLLER_PRESETS[controllerName]?.label || controllerName}
            helpText={t("profile.positioning.description")}
            callback={handleSave}
            buttonLabel={t("app.save")}
            isDisabled={isSaving}
            isPending={isSaving}>
            <SectionPanel
                label={t("profile.positioning.editorTitle")}
                helpText={(
                    <>
                        <i className={`bi ${isLandscape ? "bi-phone-landscape" : "bi-phone"} me-1`}></i>
                        {isLandscape ? t("profile.positioning.landscape") : t("profile.positioning.portrait")}
                    </>
                )}
                collapsible={true}>
                <Form.Group
                    label={t("profile.positioning.slotLabel")}
                    helpText={t("profile.positioning.slotHelp")}>
                    <div className="d-flex gap-2 align-items-center">
                        <div className="w-75">
                            <Form.Select
                                name="slotIndex"
                                context={formContext}
                                options={slotOptions}
                            />
                        </div>
                        <div className="w-25">
                            <bs.Button
                                variant="danger"
                                className="w-100"
                                onClick={handleDelete}
                                disabled={isSaving}>
                                <i className="bi bi-trash me-1"></i>
                            </bs.Button>
                        </div>
                    </div>
                </Form.Group>
                <Form.Group
                    label={t("profile.positioning.opacity")}
                    helpText={t("profile.positioning.opacityHelp")}>
                    <Form.Range name="opacity" context={formContext} min={0.25} max={1} step={0.05} variant="lg" />
                </Form.Group>
                <Form.Group props={{ style: { marginBottom: 0 } }}>
                    <Form.SkeletonSwitch
                        reverse={true}
                        name="isDefault"
                        context={formContext}
                        label={t("profile.positioning.isDefault")}
                        helpText={t("profile.positioning.isDefaultHelp")} />
                </Form.Group>
            </SectionPanel>

            <div className="border-top border-bottom py-3 mt-3">
                {groups.length > 0 && (
                    <Form.Group
                        label={t("profile.positioning.groups")}
                        helpText={t("profile.positioning.groupsHelp")}>
                        <Form.Select
                            name="selectedGroup"
                            context={{
                                values: { selectedGroup },
                                setValues: (v) => setSelectedGroup(v.selectedGroup),
                            }}
                            options={[
                                {
                                    value: "",
                                    label: "---"
                                },
                                ...groups.map((g) => ({ value: g, label: groupName(g) }))
                            ]} />
                    </Form.Group>
                )}

                {(selectedButton || selectedGroup) && (
                    <Form.Group
                        label={selectedGroup
                            ? t("profile.positioning.selectedGroup", { name: groupName(selectedGroup), count: groupMembers.length })
                            : t("profile.positioning.selected", { name: selectedButton.name })}
                        helpText={selectedGroup
                            ? t("profile.positioning.selectedGroupHelp")
                            : t(isLandscape
                                ? "profile.positioning.selectedHelpLandscape"
                                : "profile.positioning.selectedHelpPortrait")}>
                        <div className="d-flex gap-2">
                            <div className="flex-fill">
                                <label className="form-label small text-muted mb-1">{t("profile.positioning.axisX")}</label>
                                <Form.IntFloat name="x" context={geometryContext} min={0} max={1} step={0.01} />
                            </div>
                            <div className="flex-fill">
                                <label className="form-label small text-muted mb-1">{t("profile.positioning.axisY")}</label>
                                <Form.IntFloat name="y" context={geometryContext} min={0} max={1} step={0.01} />
                            </div>
                            <div className="flex-fill">
                                <label className="form-label small text-muted mb-1">{t("profile.positioning.size")}</label>
                                <Form.IntFloat name="size" context={geometryContext} min={16} max={140} step={1} />
                            </div>
                        </div>
                    </Form.Group>
                )}
            </div>


            {/* The editor itself takes whatever height is left — button
                positions are relative to the play area, so it has to be as
                large as the offcanvas allows. */}
            <div className="flex-grow-1" style={{ position: "relative", bottom: 0, minHeight: 470 }}>
                <ControllerLayout
                    name={controllerName}
                    callbacks={{}}
                    opacity={opacity}
                    editable
                    layoutOverride={layout}
                    onLayoutChange={setLayout}
                    selectedName={selectedName}
                    selectedNames={selectedNames}
                    dragGroup={!!selectedGroup}
                    onSelectButton={selectButton}
                />
            </div>
        </OffCanvas>
    );
}
