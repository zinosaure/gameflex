import React, { useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import { FormButton, ListElement, SectionPanel } from "../../../shared/templates/Essentials.jsx";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// Cheats panel (menu redesign) — its own component for the same frozen-
// snapshot reason as SaveStatesPanel: the entry list, the form fields and the
// editing index all live HERE, and every mutation pushes the full list to
// the server immediately (setCheats is whole-list by contract, see
// stream_session.go's handleSetCheats).
//
// Persisted server-side now (saves/{username}/cheats/{reference}/*.txt,
// written by handleSetCheats on every push) — initialCheats seeds the list
// from what the "offer" message already carried (see
// use-retro-streaming-connection.js), so a returning player sees their
// cheats again instead of an empty list every session.
export function CheatsPanel({ setCheats, initialCheats = [], context, t }) {
    const [entries, setEntries] = useState(initialCheats);
    const [nameInput, setNameInput] = useState("");
    const [codeInput, setCodeInput] = useState("");
    const [editingIndex, setEditingIndex] = useState(null);
    const [isPending, setIsPending] = useState(false);

    const pushEntries = (next, onDone) => {
        setIsPending(true);
        setCheats(next, (error) => {
            setIsPending(false);
            if (error) {
                context.toast.open(error, { variant: "error" });
            }
            onDone?.();
        });
    };

    const handleValidate = () => {
        const code = codeInput.trim();
        if (!code) return;
        const entry = { code, enabled: true, description: nameInput.trim() };
        const next =
            editingIndex === null
                ? [...entries, entry]
                : entries.map((c, i) => (i === editingIndex ? { ...c, code } : c));
        setEntries(next);
        pushEntries(next, () => {
            setNameInput("");
            setCodeInput("");
            setEditingIndex(null);
        });
    };

    const handleToggle = (index) => {
        const next = entries.map((c, i) =>
            i === index ? { ...c, enabled: !c.enabled } : c,
        );
        setEntries(next);
        pushEntries(next);
    };

    const handleDelete = (index) => {
        context.modal.open(t("retrogaming.streaming.cheatDeleteConfirm"), {
            variant: "confirm",
            title: t("retrogaming.streaming.menuCheats"),
            confirmLabel: t("app.delete"),
            cancelLabel: t("app.cancel"),
            onConfirm: () => {
                const next = entries.filter((_, i) => i !== index);
                setEntries(next);
                pushEntries(next);
            },
        });
    };

    const handleEdit = (index) => {
        const entry = entries[index];
        setNameInput(entry.description || "");
        setCodeInput(entry.code);
        setEditingIndex(index);
    };

    const isFormValid = codeInput.trim().length > 0;

    return (
        <>
            <SectionPanel>
                {/* Form — Skeleton fields + FormButton, same conventions as the
            admin pages (AGENTS.md). Name locked while editing: the list has
            no per-entry identity beyond its index, and the wire only
            carries description+code, so renaming an existing entry is
            deliberately out of scope. */}
                <Form.Group label={t("retrogaming.streaming.cheatNameLabel")} helpText={t("retrogaming.streaming.cheatNameHelp")}>
                    <Form.Text
                        name="cheatName"
                        context={{
                            values: { cheatName: nameInput },
                            setValues: (v) => setNameInput(v.cheatName ?? ""),
                            files: {},
                            setFiles: () => { },
                            rulesets: {
                                cheatName: () =>
                                    editingIndex !== null ? Ruleset.isDisabled("") : null,
                            },
                        }}
                        props={{
                            placeholder: t("retrogaming.streaming.cheatNamePlaceholder"),
                        }}
                    />
                </Form.Group>
                <Form.Group label={t("retrogaming.streaming.cheatCodeLabel")} helpText={t("retrogaming.streaming.cheatCodeHelp")}>
                    <Form.Longtext
                        name="cheatCode"
                        context={{
                            values: { cheatCode: codeInput },
                            setValues: (v) => setCodeInput(v.cheatCode ?? ""),
                            files: {},
                            setFiles: () => { },
                            rulesets: {},
                        }}
                        props={{
                            placeholder: t("retrogaming.streaming.cheatCodePlaceholder"),
                        }}
                    />
                </Form.Group>
                <FormButton
                    callback={handleValidate}
                    label={t(
                        editingIndex === null
                            ? "retrogaming.streaming.cheatAdd"
                            : "retrogaming.streaming.cheatUpdate",
                    )}
                    isDisabled={!isFormValid || isPending}
                    isPending={isPending}
                />
            </SectionPanel>

            <SectionPanel
                label={t("retrogaming.streaming.cheatListTitle")}
                helpText={t("retrogaming.streaming.cheatListHelp")}
            >
                {/* List — same row skeleton as every other list in the app
            (Games/RemotePlay/InvitationsTab, see ListElement's own doc):
            name + enable switch + edit/delete, no chevron (not a nav row). */}
                {entries.length > 0 ? (
                    <div className="d-flex flex-column gap-2">
                        {entries.map((cheat, index) => (
                            <ListElement
                                key={index}
                                icon="bi bi-braces"
                                onClick={() => {
                                    if (editingIndex === index) {
                                        setEditingIndex(null);
                                        setNameInput("");
                                        setCodeInput("");
                                    } else {
                                        handleEdit(index);
                                    }
                                }}
                                label={cheat.description || cheat.code}
                                helpText={cheat.description ? cheat.code : null}
                                next={
                                    <Form.SkeletonSwitch
                                        reverse
                                        name={`cheatEnabled${index}`}
                                        context={{
                                            values: { [`cheatEnabled${index}`]: cheat.enabled },
                                            setValues: () => handleToggle(index),
                                            files: {},
                                            setFiles: () => { },
                                            rulesets: {},
                                        }}
                                    />
                                }
                            >
                                {editingIndex === index && (
                                    <div className="pt-2 mt-2 border-top">
                                        <bs.Button
                                            variant="danger"
                                            className="w-100"
                                            title={t("app.delete")}
                                            onClick={() => handleDelete(index)}
                                        >
                                            <i className="bi bi-trash"></i>
                                        </bs.Button>
                                    </div>

                                )}
                            </ListElement>
                        ))}
                    </div>
                ) : (
                    <p className="text-muted small mb-0">
                        {t("retrogaming.streaming.cheatListEmpty")}
                    </p>
                )}
            </SectionPanel>
        </>
    );
}