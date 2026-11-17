import React, { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { CONTROLLER_PRESETS } from "../../engine/controllers/presets.js";
import ControllerLayout, { PadLookContext } from "../../engine/stream/onscreen/Controllers.jsx";
import { PAD_LOOK } from "../../libs/padLook.jsx";
import { credential } from "../../libs/api.js";
import { toast } from "../../shell/Shell.jsx";
import { fieldContext, FormButton } from "../../ui/Forms.jsx";
import SettingsPage, { NotFound } from "../SettingsPage.jsx";
import { BlockRow, DangerSection, GuideSection, PadOpacityRow, Rows, SelectRow, Section, SubmitRow, SwitchRow, TextRow } from "../../ui/Bricks.jsx";

// LA DISPOSITION DE LA MANETTE À L'ÉCRAN (profile/settings/gamepad/ButtonPositioning.jsx de la webapp) :
// cinq dispositions par manette virtuelle (`/key-positioning`), dont une en jeu ; l'opacité ; la
// position et la taille d'un bouton ou d'un groupe — en portrait et en paysage, deux jeux de valeurs,
// celui de l'orientation affichée. La manette dessinée est CELLE DES PARTIES (engine/stream/onscreen, venue
// de la webapp) : on la touche et on la glisse ; à la manette, on choisit un bouton
// ou un groupe dans la liste et on règle ses valeurs. Sa page : /settings/gamepads/layout/<manette>
// (11-disposition des modèles), à deux écarts près, pour que la croix atteigne tout : supprimer la disposition
// est en bas de la page, dans « Danger » (accolé au sélecteur, la croix ne l'atteint pas : GAUCHE et DROITE y
// changent d'option) ; l'opacité est sous la position (juste sous l'interrupteur, BAS passait l'un des deux).
// La manette dessinée est un contrôle du groupe de la position : elle est dans son encadré, et Enregistrer après
// elle (le porteur, 2026-11-13 : « le bouton doit être dans la section card avec les autres form controls »).

const DEFAULT_BUTTON_SIZE = 50;
const SLOTS = [1, 2, 3, 4, 5];

const positioningPath = (params) => `/key-positioning?${new URLSearchParams(params)}`;
const listPositioning = (controllerName) => credential(positioningPath({ controller_name: controllerName }));
const savePositioning = (controllerName, slotIndex, layout, isDefault, opacity) => credential("/key-positioning", {
    method: "POST", headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ controller_name: controllerName, slot_index: slotIndex, layout, is_default: isDefault, opacity }),
});
const deletePositioning = (controllerName, slotIndex) => credential(positioningPath({ controller_name: controllerName, slot_index: String(slotIndex) }), { method: "DELETE" });

// La position d'un bouton dans l'orientation affichée (le paysage a ses propres valeurs, `xl`).
function readGeometry(button, landscape) {
    const source = landscape ? { ...button, ...(button.xl || {}) } : button;

    return { x: Number(source.x ?? 0), y: Number(source.y ?? 0), size: Number(source.size ?? DEFAULT_BUTTON_SIZE) };
}

const writeGeometry = (button, landscape, patch) => (landscape ? { ...button, xl: { ...readGeometry(button, true), ...patch } } : { ...button, ...patch });
const round = (value) => Math.round(value * 10000) / 10000;

function groupGeometry(members, landscape) {
    const positions = members.map((button) => readGeometry(button, landscape));
    const sizes = new Set(positions.map((position) => position.size));

    return { x: Math.min(...positions.map((p) => p.x)), y: Math.min(...positions.map((p) => p.y)), size: sizes.size === 1 ? [...sizes][0] : null };
}

// Déplacer un groupe : chaque bouton d'autant, sans qu'aucun ne sorte de la zone.
function moveGroup(members, landscape, patch, current) {
    let dx = patch.x === undefined ? 0 : patch.x - current.x;
    let dy = patch.y === undefined ? 0 : patch.y - current.y;

    for (const member of members) {
        const at = readGeometry(member, landscape);
        dx = Math.min(Math.max(dx, -at.x), 1 - at.x);
        dy = Math.min(Math.max(dy, -at.y), 1 - at.y);
    }

    return members.map((member) => {
        const at = readGeometry(member, landscape);

        return writeGeometry(member, landscape, { x: round(at.x + dx), y: round(at.y + dy), ...(patch.size !== undefined ? { size: patch.size } : {}) });
    });
}

function useLandscape() {
    const [landscape, setLandscape] = useState(window.innerWidth > window.innerHeight);

    useEffect(() => {
        const update = () => setLandscape(window.innerWidth > window.innerHeight);
        window.addEventListener("resize", update);

        return () => window.removeEventListener("resize", update);
    }, []);

    return landscape;
}

const presetButtons = (controllerName) => CONTROLLER_PRESETS[controllerName].buttons.map((button) => ({ ...button }));

// Une disposition en texte, chaque objet à clés triées : deux dispositions égales s'écrivent pareil.
const canonical = (value) => JSON.stringify(value, (key, item) => (item && typeof item === "object" && !Array.isArray(item)
    ? Object.fromEntries(Object.entries(item).sort(([left], [right]) => left.localeCompare(right)))
    : item));

function LayoutEditor({ controllerName }) {
    const { t } = useTranslation();
    const landscape = useLandscape();
    const [slotIndex, setSlotIndex] = useState(1);
    const [slots, setSlots] = useState([]);
    const [layout, setLayout] = useState(() => presetButtons(controllerName));
    const [selectedName, setSelectedName] = useState("");
    const [selectedGroup, setSelectedGroup] = useState("");
    const [isDefault, setIsDefault] = useState(false);
    const [opacity, setOpacity] = useState(1);
    const [pending, setPending] = useState(false);
    // CE QU'ON TAPE dans X, Y ou la taille, tel quel : « 0. » s'écrit avant « 0.5 » ; la valeur ne
    // s'applique qu'une fois lisible, et le brouillon tombe quand la sélection change ou qu'on glisse.
    const [drafts, setDrafts] = useState({});
    const selectedButton = layout.find((button) => button.name === selectedName) || null;
    const members = selectedGroup ? layout.filter((button) => button.group === selectedGroup) : [];
    const geometry = selectedGroup ? groupGeometry(members, landscape) : selectedButton ? readGeometry(selectedButton, landscape) : null;
    const groups = [...new Set(layout.map((button) => button.group).filter(Boolean))];
    const groupName = (group) => t(`layout.group.${group}`, { defaultValue: group.replace(/_/g, " ") });
    // CE QUI EST ENREGISTRÉ DANS CET EMPLACEMENT, comparé à ce qu'on voit (CODE_CONVENTIONS.md, `isReplaced`) ; un
    // emplacement vide s'enregistre toujours — c'est le créer. Les boutons se comparent clés triées : le serveur ne les
    // rend pas dans l'ordre du modèle.
    const saved = slots.find((slot) => slot.slot_index === slotIndex);
    const isReplaced = !saved || isDefault !== Boolean(saved.is_default) || opacity !== (saved.opacity ?? 1) || canonical(layout) !== canonical(saved.layout?.length ? saved.layout : presetButtons(controllerName));

    useEffect(() => {
        let cancelled = false;
        listPositioning(controllerName).then((answer) => {
            if (cancelled) {
                return;
            }
            const items = answer.ok ? answer.items || [] : [];
            const existing = items.find((slot) => slot.slot_index === slotIndex);
            setSlots(items);
            // Une disposition vide (créée par la glissière d'opacité du menu d'une partie, padOpacity.js) est celle par défaut.
            setLayout(existing?.layout?.length ? existing.layout.map((button) => ({ ...button })) : presetButtons(controllerName));
            setIsDefault(existing ? existing.is_default : false);
            setOpacity(existing ? existing.opacity ?? 1 : 1);
            setSelectedName("");
            setSelectedGroup("");
        });

        return () => {
            cancelled = true;
        };
    }, [controllerName, slotIndex]);

    useEffect(() => setDrafts({}), [selectedName, selectedGroup, landscape]);

    const patchSelected = (patch) => {
        if (selectedGroup) {
            setLayout((current) => {
                const moved = new Map(moveGroup(current.filter((button) => button.group === selectedGroup), landscape, patch, geometry).map((button) => [button.name, button]));

                return current.map((button) => moved.get(button.name) || button);
            });

            return;
        }
        setLayout((current) => current.map((button) => (button.name === selectedName ? writeGeometry(button, landscape, patch) : button)));
    };

    // Enregistrer : sans autre disposition en jeu, celle-ci le devient.
    const save = async () => {
        setPending(true);
        const effective = isDefault || !slots.some((slot) => slot.is_default && slot.slot_index !== slotIndex);
        const answer = await savePositioning(controllerName, slotIndex, layout, effective, opacity);
        setPending(false);

        if (!answer.ok) {
            toast(answer.detail || t("layout.saveFailed"));

            return;
        }
        setIsDefault(effective);
        toast(t("layout.saved"));
        setSlots((previous) => [...previous.filter((slot) => slot.slot_index !== slotIndex).map((slot) => (effective ? { ...slot, is_default: false } : slot)), ...(answer.item ? [answer.item] : [])].sort((a, b) => a.slot_index - b.slot_index));
    };

    // X, Y et la taille : trois champs de nombre, chacun sa ligne.
    const axis = (name, label, help, min, max, step) => {
        const shown = geometry ? String(name === "size" ? geometry.size ?? DEFAULT_BUTTON_SIZE : geometry[name]) : "";
        const context = fieldContext(`ly-${name}`, drafts[name] ?? shown, (typed) => {
            setDrafts((current) => ({ ...current, [name]: typed }));

            if (typed !== "" && typed !== null && !String(typed).endsWith(".") && Number.isFinite(Number(typed))) {
                patchSelected({ [name]: Number(typed) });
            }
        }, !geometry);

        return <TextRow id={`ly-${name}`} kind="number" label={label} help={help} context={context} min={min} max={max} step={step} />;
    };

    const remove = async () => {
        const answer = await deletePositioning(controllerName, slotIndex);

        if (!answer.ok) {
            toast(answer.detail || t("settings.actionFailed"));

            return;
        }
        setSlots((previous) => previous.filter((slot) => slot.slot_index !== slotIndex));
        setLayout(presetButtons(controllerName));
        setIsDefault(false);
        setOpacity(1);
    };

    return (
        <>
            <Section title={t("layout.slotTitle")}>
                <Rows>
                    <SelectRow id="ly-slot" label={t("layout.slotLabel")} help={t("layout.slotHelp")} value={String(slotIndex)} onChange={(value) => setSlotIndex(Number(value))}
                        options={SLOTS.map((index) => {
                            const saved = slots.find((slot) => slot.slot_index === index);

                            return { value: String(index), label: `${t("layout.slot", { index })}${saved ? (saved.is_default ? " ★" : "") : ` (${t("layout.emptySlot")})`}` };
                        })} />
                    <SwitchRow id="ly-default" label={t("layout.isDefault")} help={t("layout.isDefaultHelp")} checked={isDefault} onChange={setIsDefault} />
                </Rows>
            </Section>
            <Section title={t("layout.editorTitle")} help={selectedGroup ? t("layout.selectedGroupHelp") : t(landscape ? "layout.selectedHelpLandscape" : "layout.selectedHelpPortrait")}>
                <Rows>
                    <SelectRow id="ly-button" label={t("layout.button")} help={t("layout.buttonHelp")} value={selectedGroup ? "" : selectedName} onChange={(value) => {
                        setSelectedGroup("");
                        setSelectedName(value || "");
                    }} options={[{ value: "", label: "—" }, ...layout.map((button) => ({ value: button.name, label: button.name.replace(/_/g, " ") }))]} />
                    {groups.length > 0 && (
                        <SelectRow id="ly-group" label={t("layout.groups")} help={t("layout.groupsHelp")} value={selectedGroup} onChange={(value) => {
                            setSelectedName("");
                            setSelectedGroup(value || "");
                        }} options={[{ value: "", label: "—" }, ...groups.map((group) => ({ value: group, label: groupName(group) }))]} />
                    )}
                    {axis("x", t("layout.axisX"), t("layout.axisXHelp"), 0, 1, 0.01)}
                    {axis("y", t("layout.axisY"), t("layout.axisYHelp"), 0, 1, 0.01)}
                    {axis("size", t("layout.size"), t("layout.sizeHelp"), 16, 140, 1)}
                    <PadOpacityRow id="ly-opacity" value={opacity} onChange={setOpacity} />
                    {/* La manette dessinée, dans une ligne libre du groupe (BlockRow). */}
                    <BlockRow>
                        <div className="layout-stage position-relative border rounded overflow-hidden">
                            <PadLookContext.Provider value={PAD_LOOK}>
                                <ControllerLayout name={controllerName} callbacks={{}} opacity={opacity} editable layoutOverride={layout} onLayoutChange={(next) => {
                                    setDrafts({});
                                    setLayout(next);
                                }}
                                    selectedName={selectedGroup ? null : selectedName || null} selectedNames={selectedGroup ? new Set(members.map((button) => button.name)) : null}
                                    dragGroup={Boolean(selectedGroup)} onSelectButton={(name) => {
                                        setSelectedGroup("");
                                        setSelectedName(name || "");
                                    }} />
                            </PadLookContext.Provider>
                        </div>
                    </BlockRow>
                    <SubmitRow>
                        <FormButton id="ly-save" type="button" label={t("settings.save")} isReplaced={isReplaced} pending={pending} onClick={save} />
                    </SubmitRow>
                </Rows>
            </Section>
            <DangerSection id="ly-delete" label={t("layout.delete")} help={t("layout.deleteHelp")} question={t("layout.deleteConfirm")} disabled={pending} onConfirm={remove} />
            <GuideSection guides={[["controllers", "the-on-screen-controller"]]} />
        </>
    );
}

// /settings/gamepads/layout/<manette> : B ramène aux Manettes, sur cette manette virtuelle.
export function LayoutPage() {
    const { t } = useTranslation();
    const { controller } = useParams();
    const preset = CONTROLLER_PRESETS[controller];

    return (
        <SettingsPage title={preset ? preset.label || controller : t("gamepads.editLayout")} parent={`/settings/gamepads?controller=${encodeURIComponent(controller || "")}`}>
            {preset ? <LayoutEditor key={controller} controllerName={controller} /> : <NotFound />}
        </SettingsPage>
    );
}
