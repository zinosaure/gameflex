import React, { useEffect, useRef, useState } from "react";
import * as bs from "react-bootstrap";
import { useNavigate } from "react-router-dom";
import Skeleton from "../../../skeleton/skeleton.js";
import { useGamepad } from "../../hooks/use-gamepad.jsx";
import { CONTROLLER_PRESETS } from "../../controllers/presets.js";
import { can } from "../../libs/capabilities.js";
import ButtonPositioningPanel from "./gamepad/ButtonPositioning.jsx";
import {
    getKeyboardDeviceId,
    loadKeyMapping,
    subscribeKeyMapping,
    remapKey,
    captureGamepadChordForIndex,
    captureKeyboardChord,
    applyAutoMapping,
    gamepadDeviceId,
} from "../../libs/keyMapping.js";
import { deleteKeyMapping } from "../api.js";
import {
    SectionPanel,
    FormButton,
} from "../../shared/templates/Essentials.jsx";
import {
    setHapticFeedback,
    setRumbleEnabled,
    useHapticFeedback,
    useRumbleEnabled,
} from "../../libs/gamepadPreferences.js";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// Whether to offer the keyboard as a mappable device. "(pointer: coarse)" is
// the browser saying touch is this device's PRIMARY pointer — i.e. a phone or
// tablet, where there's no keyboard to map. Deliberately not a touch-support
// or viewport-width test: a laptop with a touchscreen still has a keyboard,
// and so does one with a narrow window.
const hasKeyboard = () => !window.matchMedia?.("(pointer: coarse)").matches;

// ─── Mapping ────────────────────────────────────────────────────────────

// One row per virtual-controller button — click to capture physical inputs
// (keyboard keys or gamepad buttons) and bind them to that button.
// LEFT_STICK/RIGHT_STICK are excluded (see KeyMappingTab's own doc): those
// stay on the existing fixed analog-axis convention (pad.axes[0]/[1]), not
// remapped here.
//
// Capture never validates on the first press: every button pressed while
// listening ACCUMULATES into a chord ("ArrowDown+ArrowRight"), shown live,
// and the user validates it with the tick or discards it with the trash —
// chords are what make a diagonal (DPAD_DOWN_RIGHT) mappable at all.
// A capture is discarded automatically if the row unmounts mid-listen
// (AbortController + the capture functions' own cleanup).
function KeyMappingCaptureRow({ label, currentInput, deviceType, gamepadIndex, isDisabled, onCapture, onCaptureStart, onCaptureEnd, t }) {
    const [listening, setListening] = useState(false);
    const [chord, setChord] = useState("");
    const cleanupRef = useRef(null);
    const abortRef = useRef(null);

    useEffect(
        () => () => {
            abortRef.current?.abort();
            onCaptureEnd?.();
        },
        [],
    );

    const startCapture = () => {
        // The parent owns the AbortController: starting THIS capture aborts
        // whatever other row was listening — one capture at a time.
        const controller = onCaptureStart();
        abortRef.current = controller;
        controller.signal.addEventListener("abort", () => {
            cleanupRef.current?.();
            cleanupRef.current = null;
            setListening(false);
            setChord("");
        });
        setListening(true);
        setChord("");
        cleanupRef.current =
            deviceType === "keyboard"
                ? captureKeyboardChord({ signal: controller.signal, onChange: setChord })
                : captureGamepadChordForIndex(gamepadIndex, { signal: controller.signal, onChange: setChord });
    };

    const stopListening = () => {
        onCaptureEnd();
    };

    const validate = async () => {
        if (!chord) return;
        onCaptureEnd();
        await onCapture(chord);
    };

    return (
        <div className="d-flex align-items-center justify-content-between gap-2 py-1">
            <span className={`small${isDisabled ? " text-muted" : ""}`}>{label}</span>
            {listening ? (
                <div className="d-flex align-items-center gap-2">
                    <span className="badge bg-primary fs-6 py-2 px-3">
                        {chord ? chord.replace(/\+/g, " + ") : t("profile.gamepad.listening")}
                    </span>
                    <bs.Button
                        size="sm"
                        variant="primary"
                        title={t("profile.gamepad.chordValidate")}
                        disabled={!chord}
                        onClick={validate}>
                        <i className="bi bi-check-lg"></i>
                    </bs.Button>
                    <bs.Button
                        size="sm"
                        variant="secondary"
                        title={t("profile.gamepad.chordDiscard")}
                        onClick={stopListening}>
                        <i className="bi bi-trash"></i>
                    </bs.Button>
                </div>
            ) : (
                <bs.Button
                    size="sm"
                    variant="primary"
                    disabled={isDisabled}
                    onClick={startCapture}
                    style={{ minWidth: 120 }}>
                    {currentInput ?? "—"}
                </bs.Button>
            )}
        </div>
    );
}

// Binds ONE physical device (a specific connected gamepad, identified by its
// gamepad.id, or "the keyboard on this browser", see gameflex/libs/
// keyMapping.js's device-id doc) to ONE virtual controller preset
// (CONTROLLER_PRESETS). Every session that renders through that same virtual
// controller — several consoles at once for retro-gaming
// (RETRO_CONTROLLER_PRESET_MAP), always "dualsense" for remote-play — then
// applies this mapping automatically, without reconfiguring per console. A
// device/controller pair with no saved mapping falls back to the runtime's
// own per-console defaults (gamepadMapping.js/keyboardMapping.js,
// remote-play's PS5_BUTTON_MAP): this is an opt-in override, not a required
// setup step.
export default function Gamepad({ context, token, t }) {
    const navigate = useNavigate();
    const { connectedGamepads } = useGamepad();
    const [keyboardId] = useState(() => getKeyboardDeviceId());
    // Guard: only ONE capture can listen at a time (see the screenshot of
    // three rows all showing "Press a button…") — starting a capture on one
    // row discards whatever another row was listening for.
    const [capturingButton, setCapturingButton] = useState(null);
    const captureControllerRef = useRef(null);
    const startCaptureFor = (name) => {
        captureControllerRef.current?.abort();
        const controller = new AbortController();
        captureControllerRef.current = controller;
        setCapturingButton(name);
        return controller;
    };
    const endCaptureFor = (name) => {
        if (capturingButton !== name) return; // a stale row must not kill the current one
        captureControllerRef.current?.abort();
        captureControllerRef.current = null;
        setCapturingButton(null);
    };

    // Virtual controller first, then the device it's bound to — that's the
    // order the choice is actually made in.
    const [controllerName, setControllerName] = useState("dualshock");
    const [selectedDeviceId, setSelectedDeviceId] = useState(null);
    const [mapping, setMapping] = useState({});
    const [isAutoMapping, setIsAutoMapping] = useState(false);

    // Device-level toggles (localStorage, see gamepadPreferences.js) — the
    // hooks re-render this tab when either switch is flipped, so the
    // checkbox state never drifts from the stored value.
    const hapticFeedback = useHapticFeedback();
    const rumbleEnabled = useRumbleEnabled();

    const devices = [
        ...(hasKeyboard() ? [{ id: keyboardId, type: "keyboard", label: t("profile.gamepad.keyboard") }] : []),
        ...connectedGamepads.map((g) => {
            const id = gamepadDeviceId(g);
            // LA DISPOSITION EST DITE, pas devinée.
            //
            // `pad.mapping` vaut "standard" quand le navigateur a pu ramener
            // la manette à la disposition normalisée, et une chaîne vide
            // quand les index sont ceux du pilote. Dans le second cas
            // l'auto-mapping ne s'applique pas (il produirait une table
            // fausse), et il faut le DIRE — sinon on se retrouve devant un
            // écran vide sans savoir pourquoi.
            const isStandard = g.mapping === "standard";

            return {
                id,
                type: "gamepad",
                label: isStandard ? id : `${id} — ${t("profile.gamepad.nonStandardLayout")}`,
                gamepadIndex: g.index,
                isStandard,
            };
        }),
    ];
    const selectedDevice = devices.find((d) => d.id === selectedDeviceId) || devices[0] || null;
    const hasDevice = !!selectedDevice;

    // Keeps the selection valid as controllers are plugged in or unplugged.
    useEffect(() => {
        if (selectedDevice && selectedDevice.id !== selectedDeviceId) {
            setSelectedDeviceId(selectedDevice.id);
        }

        if (!selectedDevice && selectedDeviceId) {
            setSelectedDeviceId(null);
        }
    }, [selectedDevice?.id]);

    useEffect(() => {
        if (!hasDevice) {
            return;
        }

        let cancelled = false;
        loadKeyMapping(token, selectedDevice.id, controllerName).then((m) => {
            if (!cancelled) setMapping(m);
        });

        return () => { cancelled = true; };
    }, [token, selectedDevice?.id, controllerName, hasDevice]);

    useEffect(() => {
        if (!hasDevice) {
            return;
        }

        return subscribeKeyMapping(selectedDevice.id, controllerName, setMapping);
    }, [selectedDevice?.id, controllerName, hasDevice]);

    const formContext = {
        values: {
            controllerName,
            deviceId: selectedDevice?.id || "",
            hapticFeedback,
            rumble: rumbleEnabled,
        },
        setValues: (v) => {
            if (v.controllerName !== controllerName) {
                setControllerName(v.controllerName);
            } else {
                setSelectedDeviceId(v.deviceId);
            }

            if (typeof v.hapticFeedback === "boolean") {
                setHapticFeedback(v.hapticFeedback);
            }

            if (typeof v.rumble === "boolean") {
                setRumbleEnabled(v.rumble);
            }
        },
        files: {},
        setFiles: () => { },
        rulesets: {
            // Nothing to bind an input to without a device, so the picker
            // says so rather than sitting there empty and inert.
            deviceId: () => (hasDevice ? null : Ruleset.isDisabled(t("profile.gamepad.noDevice"))),
            // Presets without a rumble motor (NES, PSP, …) get an inert
            // switch — the option physically can't apply to that pad.
            rumble: () =>
                CONTROLLER_PRESETS[controllerName]?.rumble === false
                    ? Ruleset.isDisabled(t("profile.gamepad.rumbleUnsupported"))
                    : null,
        },
    };

    const preset = CONTROLLER_PRESETS[controllerName];
    const buttonNames = [...new Set((preset?.buttons || []).map((b) => b.name))]
        .filter((name) => name !== "LEFT_STICK" && name !== "RIGHT_STICK");

    const handleCapture = async (buttonName, input) => {
        const result = await remapKey(token, selectedDevice.id, selectedDevice.type, controllerName, buttonName, input);
        if (!result.ok) {
            context.toast.open(result.exception?.detail || t("profile.gamepad.saveFailed"), { variant: "error" });
        }
    };

    // Applies the default mapping to EVERY virtual controller for the
    // selected device, overwriting what's saved — the user can then re-map
    // anything. Only gamepads have a default table (the keyboard has none).
    const handleAutoMapping = async () => {
        if (!selectedDevice || selectedDevice.type !== "gamepad") {
            return;
        }

        setIsAutoMapping(true);
        const applied = await applyAutoMapping(token, selectedDevice.id, "gamepad");
        setIsAutoMapping(false);

        if (applied === 0) {
            context.toast.open(t("profile.gamepad.saveFailed"), { variant: "error" });
            return;
        }

        const fresh = await loadKeyMapping(token, selectedDevice.id, controllerName);
        setMapping(fresh);
        context.toast.open(t("profile.gamepad.autoMappingDone"), { variant: "normal" });
    };

    const handleReset = (event) => {
        event.preventDefault();
        context.modal.open(t("profile.gamepad.resetConfirm"), {
            variant: "confirm",
            title: t("profile.gamepad.resetTitle"),
            confirmLabel: t("profile.gamepad.resetAction"),
            cancelLabel: t("app.cancel"),
            onConfirm: async () => {
                setMapping({});
                const result = await deleteKeyMapping(token, selectedDevice.id, controllerName);

                if (!result.ok) {
                    return context.toast.open(result.exception?.detail || t("profile.gamepad.saveFailed"), { variant: "error" });
                }

                return context.toast.open(t("profile.gamepad.resetDone"), { variant: "success" });
            },
            onCancel: () => null,
        });
    };

    // size: null is Skeleton's own "full viewport" (see skeleton.js's
    // offcanvas.open) — button positions are only meaningful at the size
    // they'll really be played at.
    //
    // No `title` on purpose: the panel renders OffCanvas, which brings
    // its own header (and the footer arrow that closes it) — same as every
    // other panel in the app. Passing one here would stack two titles.
    const openEditor = () => context.offcanvas.open(
        <ButtonPositioningPanel controllerName={controllerName} />,
        { size: null }
    );

    // Full-page route (not the offcanvas): the stream must survive a
    // reload, and the offcanvas's own close fought the Escape blocking.
    const openLayout = () => {
        context.offcanvas.close();
        navigate(`/keytest/${controllerName}`);
    };

    return (
        <>
            <SectionPanel>
                <Form.Group
                    label={t("profile.gamepad.virtualController")}
                    helpText={t("profile.gamepad.virtualControllerHelp")}>
                    <div className="d-flex gap-2 align-items-start">
                        <div className={can("gamepadTest") ? "w-75" : "w-100"}>
                            <Form.Select
                                name="controllerName"
                                context={formContext}
                                options={Object.entries(CONTROLLER_PRESETS).map(([key, p]) => ({
                                    value: key,
                                    label: p.label || key,
                                }))} />
                        </div>
                        {/* Caché pour l'instant : voir `gamepadTest` dans capabilities.js. */}
                        {can("gamepadTest") && (
                            <div className="w-25">
                                <bs.Button
                                    className="w-100"
                                    variant="secondary"
                                    onClick={openLayout}>
                                    Test
                                </bs.Button>
                            </div>
                        )}
                    </div>
                </Form.Group>
                <Form.Group>
                    <Form.SkeletonSwitch
                        name="hapticFeedback"
                        reverse={true}
                        context={formContext}
                        label={t("profile.gamepad.hapticFeedback")}
                        helpText={t("profile.gamepad.hapticFeedbackHelp")} />
                    <Form.SkeletonSwitch
                        name="rumble"
                        reverse={true}
                        context={formContext}
                        label={t("profile.gamepad.rumble")}
                        helpText={t("profile.gamepad.rumbleHelp")} />
                </Form.Group>

                <FormButton
                    callback={openEditor}
                    label={t("settings.editLayout")}
                    isDisabled={false}
                    isPending={false} />
            </SectionPanel>

            <SectionPanel
                label={selectedDevice?.label
                    ? t("profile.gamepad.mappingTitle", {
                        device: selectedDevice.label,
                        controller: CONTROLLER_PRESETS[controllerName]?.label,
                    })
                    : t("profile.gamepad.noDeviceTitle")}
                helpText={t("profile.gamepad.mappingDescription")}>
                <Form.Group>
                    <div className="d-flex gap-2 align-items-start">
                        <div className="w-75">
                            <Form.Select
                                name="deviceId"
                                context={formContext}
                                options={devices.map((d) => ({ value: d.id, label: d.label }))} />
                        </div>
                        <bs.Button
                            className="w-25"
                            variant="secondary"
                            disabled={!hasDevice || selectedDevice?.type !== "gamepad" || isAutoMapping}
                            onClick={handleAutoMapping}>
                            {isAutoMapping
                                ? <bs.Spinner size="sm" animation="border" />
                                : t("profile.gamepad.autoMapping")}
                        </bs.Button>
                    </div>
                </Form.Group>

                <div className="d-flex flex-column">
                    {buttonNames.map((name) => {
                        const boundInput = Object.entries(mapping).find(([, v]) => v === name)?.[0];
                        return (
                            <KeyMappingCaptureRow
                                key={`${selectedDevice?.id || "none"}:${controllerName}:${name}`}
                                label={name.replace(/_/g, " ")}
                                currentInput={boundInput}
                                deviceType={selectedDevice?.type}
                                gamepadIndex={selectedDevice?.gamepadIndex}
                                isDisabled={!hasDevice}
                                onCapture={(input) => handleCapture(name, input)}
                                onCaptureStart={() => startCaptureFor(name)}
                                onCaptureEnd={() => endCaptureFor(name)}
                                t={t} />
                        );
                    })}
                </div>

                <FormButton
                    callback={handleReset}
                    label={t("profile.gamepad.reset")}
                    isDisabled={!hasDevice}
                    isPending={false}
                    variant="danger" />
            </SectionPanel>
        </>
    );
}