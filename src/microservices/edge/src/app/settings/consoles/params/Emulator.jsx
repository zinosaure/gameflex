// Route : /settings/consoles/<console>/params/<émulateur> — les paramètres d'un émulateur.
import React, { useEffect, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { DangerSection } from "../../../bricks/DangerSection.jsx";
import { GuideSection } from "../../../bricks/GuideSection.jsx";
import { InfoRow } from "../../../bricks/InfoRow.jsx";
import { Loading } from "../../../bricks/Loading.jsx";
import { Note } from "../../../bricks/Note.jsx";
import { Rows } from "../../../bricks/Rows.jsx";
import { Section } from "../../../bricks/Section.jsx";
import { SelectRow } from "../../../bricks/SelectRow.jsx";
import { SubmitRow } from "../../../bricks/SubmitRow.jsx";
import { SwitchRow } from "../../../bricks/SwitchRow.jsx";
import { TextRow } from "../../../bricks/TextRow.jsx";
import { useFeedback } from "../../../bricks/useFeedback.jsx";
import { deleteEmulatorPreset, fetchRequired, listEmulatorParams, writeEmulatorParams } from "../../../libs/api.js";
import { useData } from "../../../libs/data.jsx";
import { fact } from "../../../libs/format.js";
import { toast } from "../../../shell/Shell.jsx";
import { fieldContext, Form, FormButton, Ruleset } from "../../../ui/Forms.jsx";
import SettingsPage, { NotFound } from "../../SettingsPage.jsx";
import { consolePath, paramsLabel } from "../../Consoles.jsx";
import { reasonOf } from "../Required.jsx";

// Les textes d'un émulateur dans la langue de la page (son fichier les porte en anglais, et ses traductions).
const inLanguage = (emulator, language) => {
    const own = (item) => item?.translations?.[language] || {};

    return {
        ...emulator,
        description: own(emulator).description || emulator.description,
        params: (emulator.params || []).map((param) => ({
            ...param,
            label: own(param).label || param.label,
            description: own(param).description || param.description,
            options: param.options && param.options.map((option) => ({ ...option, label: own(param).options?.[option.value] || option.label })),
        })),
        presets: (emulator.presets || []).map((preset) => ({ ...preset, name: own(preset).name || preset.name, description: own(preset).description || preset.description })),
    };
};
const valuesOf = (emulator) => Object.fromEntries((emulator.params || []).map((param) => [param.id, param.value]));

// CE QUE PRÊTENT LES AUTRES SOURCES pour cet émulateur (le porteur, 2026-11-14 : « rajouter les paramètres venant des
// serveurs fédérés dans presets. On rajoute des option group si possible. ») : chaque disque interne, puis chaque
// serveur fédéré, lus à part — ses valeurs (« Ses valeurs ») et ses préréglages perso, chacun avec ses valeurs, déjà
// passées par les paramètres d'ici (storage_params_api.go, lentPreset). `items` : undefined en lecture, null quand la
// source n'a pas répondu. Les disques d'abord, comme les sources des jeux d'une console (Games.jsx).
function useLentPresets(consoleId, emulator) {
    const { t } = useTranslation();
    const { disks, servers } = useData();
    const sources = useMemo(() => [
        ...(disks || []).map((disk) => ({ key: `disk:${disk.name}`, name: disk.name })),
        ...(servers || []).map((server) => ({ key: `peer:${server.id}`, name: server.hostname || server.link || t("consoles.serverNamed", { id: server.id }) })),
    ], [disks, servers, t]);
    const [lent, setLent] = useState({});

    useEffect(() => {
        let cancelled = false;
        setLent({});

        for (const source of sources) {
            fetchRequired("params", source.key, { console: consoleId, emulator }).then((answer) => !cancelled && setLent((current) => ({
                ...current,
                [source.key]: answer.ok ? (answer.data?.consoles || []).flatMap((found) => found.items || []).filter((item) => item.emulator === emulator) : null,
            })));
        }

        return () => {
            cancelled = true;
        };
    }, [consoleId, emulator, sources]);

    return useMemo(() => sources.map((source) => ({ ...source, items: lent[source.key] })), [sources, lent]);
}

// Un prêt dans le sélecteur : sa valeur (sa source et son préréglage — un préréglage d'ici n'a ni « : » ni « / »), son
// nom (« Ses valeurs », ou celui de son préréglage).
const lentId = (source, item) => `${source.key}/${item.preset || ""}`;
const lentName = (item, t) => (item.preset ? item.name : t("consoles.paramsTheirValues"));
const enginesOf = (emulator) => (emulator.engines || []).map(({ engine, name, preset, enabled, error }) => ({ engine, name, preset, enabled, error }));
const textInvalid = (value) => String(value ?? "").trim() === "" || /[\r\n]/.test(String(value ?? ""));

// LE FORMULAIRE D'UN ÉMULATEUR (EmulatorParams de la webapp), construit depuis son fichier : sa présentation, son
// fichier (une ligne) et son interrupteur, son préréglage, ses paramètres, ses moteurs, l'enregistrer comme
// préréglage — un groupe chacun, Enregistrer dans l'encadré du dernier (le formulaire couvre tous ses groupes) ;
// supprimer le préréglage choisi (un des siens), dans « Danger ». Le préréglage se choisit parmi ceux de ce serveur,
// puis ce que prête chaque disque interne et chaque serveur fédéré (useLentPresets), un groupe d'options chacun ;
// choisi, il remplit les champs, et « Enregistrer » écrit ICI.
function EmulatorForm({ initial }) {
    const { t, i18n } = useTranslation();
    const feedback = useFeedback();
    const language = (i18n.language || "en").split("-")[0];
    const [emulator, setEmulator] = useState(() => inLanguage(initial, language));
    const [values, setValues] = useState(() => valuesOf(initial));
    const [enabled, setEnabled] = useState(initial.exists ? initial.enabled : true);
    const [engines, setEngines] = useState(() => enginesOf(initial));
    const [preset, setPreset] = useState("");
    const [saveAs, setSaveAs] = useState({ on: false, name: "" });
    const [pending, setPending] = useState(false);
    const lent = useLentPresets(initial.consoleId, initial.emulator);
    const lentById = useMemo(() => new Map(lent.flatMap((source) => (source.items || []).filter((item) => !item.error).map((item) => [lentId(source, item), { source, item }]))), [lent]);
    const borrowed = lentById.get(preset) || null;
    const saved = valuesOf(emulator);
    const savedEngines = enginesOf(emulator);
    const changed = !emulator.exists || Boolean(emulator.error) || enabled !== emulator.enabled
        || emulator.params.some((param) => values[param.id] !== saved[param.id])
        || engines.some((setting, index) => setting.preset !== savedEngines[index]?.preset || setting.enabled !== savedEngines[index]?.enabled);
    const selected = emulator.presets.find((item) => item.id === preset) || null;
    const readOnlyNames = new Set(emulator.presets.filter((item) => !item.override).map((item) => String(item.name).trim().toLowerCase()));
    const textParams = emulator.params.filter((param) => param.type === "text");
    const anyTextInvalid = textParams.some((param) => textInvalid(values[param.id]));
    const presetNameProblem = !saveAs.on ? "" : !saveAs.name.trim() ? t("consoles.paramsPresetNameInvalid") : readOnlyNames.has(saveAs.name.trim().toLowerCase()) ? t("consoles.paramsPresetNameReadOnly") : "";

    const apply = (answer) => {
        const next = inLanguage(answer.data, language);
        setEmulator(next);
        setValues(valuesOf(next));
        setEnabled(next.enabled);
        setEngines(enginesOf(next));
    };

    // Un préréglage ne fait que REMPLIR les champs ; rien n'est écrit avant « Enregistrer ». Celui d'une autre source
    // n'apporte que ses valeurs (le porteur, 2026-11-14 : « on charge que les données de celui-ci ») : rien ne le
    // garde ici, sauf « Enregistrer comme préréglage », sous le nom qu'on lui donne.
    const choose = (id) => {
        setPreset(id);
        const from = lentById.get(id);

        if (from) {
            setSaveAs({ on: false, name: "" });
            setValues((current) => ({ ...current, ...from.item.values }));
            toast([t("consoles.presetFilledFrom", { name: lentName(from.item, t), source: from.source.name }),
                from.item.ignored?.length ? t("consoles.presetIgnored", { list: from.item.ignored.join(", ") }) : null].filter(Boolean).join(" "));

            return;
        }
        const chosen = emulator.presets.find((item) => item.id === id);
        setSaveAs(chosen && chosen.override && !chosen.error ? { on: true, name: chosen.name } : { on: false, name: "" });

        if (chosen && !chosen.error) {
            setValues((current) => ({ ...current, ...chosen.values }));
            toast([t("consoles.presetFilled", { name: chosen.name }), chosen.ignored?.length ? t("consoles.presetIgnored", { list: chosen.ignored.join(", ") }) : null].filter(Boolean).join(" "));
        }
    };

    const save = async () => {
        setPending(true);
        const presetName = saveAs.on ? saveAs.name.trim() : "";
        const answer = await writeEmulatorParams(emulator.consoleId, emulator.emulator, {
            enabled, values, preset: presetName,
            ...(engines.length ? { engines: engines.map(({ engine, preset: chosen, enabled: on }) => ({ engine, preset: chosen, enabled: on })) } : {}),
        });
        setPending(false);
        feedback.result(answer.ok, answer.data?.savedPreset ? t("consoles.paramsSavedWithPreset", { name: presetName }) : t("consoles.paramsSaved"), answer.errorMessage || t("consoles.paramsFailed"));

        // La réponse est l'émulateur relu (son fichier tel qu'enregistré).
        if (answer.ok && answer.data?.params) {
            apply(answer);
            setPreset(answer.data.savedPreset || "");

            if (!answer.data.savedPreset) {
                setSaveAs({ on: false, name: "" });
            }
        }
    };

    const setParam = (id, value) => setValues((current) => ({ ...current, [id]: value }));
    const setEngine = (engine, patch) => setEngines((current) => current.map((setting) => (setting.engine === engine ? { ...setting, ...patch } : setting)));

    // LES PARAMÈTRES TEXTE, un contexte de Skeleton qui porte leur règle (une ligne, non vide) : leurs noms
    // préfixés (`pm-<id>`), pour que leurs identifiants dans la page ne croisent ceux d'aucun autre champ.
    const textContext = {
        values: Object.fromEntries(textParams.map((param) => [`pm-${param.id}`, values[param.id] ?? ""])),
        setValues: (next) => setValues((current) => ({
            ...current,
            ...Object.fromEntries(textParams.filter((param) => `pm-${param.id}` in next).map((param) => [param.id, next[`pm-${param.id}`]])),
        })),
        files: {},
        setFiles: () => {},
        rulesets: Object.fromEntries(textParams.map((param) => [`pm-${param.id}`, (data) => (textInvalid(data[`pm-${param.id}`]) ? Ruleset.isInvalid(t("consoles.paramTextInvalid")) : null)])),
    };
    const presetNameContext = {
        ...fieldContext("pm-preset-name", saveAs.name, (name) => setSaveAs((current) => ({ ...current, name }))),
        // TOUJOURS LÀ (la règle : jamais caché, désactivé avec sa raison) : sans « Enregistrer comme préréglage », grisé.
        rulesets: { "pm-preset-name": () => (!saveAs.on ? Ruleset.isDisabled(t("consoles.paramsPresetNameOff"))
            : presetNameProblem ? Ruleset.isInvalid(presetNameProblem) : null) },
    };

    // Un paramètre, sa ligne : un interrupteur, un sélecteur (des chaînes, comme le fichier), un champ.
    const row = (param) => {
        const id = `pm-${param.id}`;
        const help = [param.description, param.config];

        return param.type === "switch" ? <SwitchRow key={id} id={id} label={param.label} texts="enabled" help={help} checked={Boolean(values[param.id])} onChange={(on) => setParam(param.id, on)} />
            : param.type === "select" ? <SelectRow key={id} id={id} label={param.label} help={help} value={String(values[param.id] ?? "")} onChange={(value) => setParam(param.id, value)}
                options={(param.options || []).map((option) => ({ value: String(option.value), label: option.label }))} />
                : <TextRow key={id} id={id} label={param.label} help={help} context={textContext} props={{ autoCapitalize: "off" }} />;
    };
    const intro = Boolean(emulator.description || emulator.requirements?.length);
    // LE SÉLECTEUR DES PRÉRÉGLAGES, un groupe par source (`<optgroup>`, Form.Select) : ce serveur, puis chaque disque
    // interne et chaque serveur fédéré — sa lecture, sa raison quand il ne répond pas, ou qu'il ne prête rien.
    const unavailable = (source, label) => [{ value: `${source.key}/`, label, attributes: { disabled: true } }];
    const presetOptions = [
        { value: "", label: t("consoles.paramsPresetNone") },
        { label: t("consoles.presetsHere"), options: emulator.presets.length ? emulator.presets.map((item) => ({ value: item.id, label: item.error ? `${item.name} — ${t("consoles.presetBroken")}` : item.name }))
            : [{ value: "here/", label: t("consoles.presetsNone"), attributes: { disabled: true } }] },
        ...lent.map((source) => ({
            label: source.name,
            options: source.items === undefined ? unavailable(source, t("settings.loading"))
                : source.items === null ? unavailable(source, reasonOf("unreachable", t))
                    : !source.items.length ? unavailable(source, t("consoles.presetsLendsNothing"))
                        : source.items.map((item) => ({ value: lentId(source, item), label: item.error ? `${lentName(item, t)} — ${reasonOf(item.error, t)}` : lentName(item, t), attributes: item.error ? { disabled: true } : undefined })),
        })),
    ];
    const presetHelp = borrowed ? t("consoles.paramsPresetLentHelp", { source: borrowed.source.name }) : selected && !selected.override ? t("consoles.paramsPresetReadOnly") : t("consoles.paramsPresetHelp");

    return (
        <Form props={{ onSubmit: (event) => {
            event.preventDefault();

            if (changed && !anyTextInvalid && !presetNameProblem && !pending) {
                save();
            }
        } }}>
            {/* LE FICHIER EST UNE LIGNE (le porteur, 2026-11-14, remarque b), comme « Le dossier » de l'Apparence : son
                icône, son chemin en chasse fixe — plus collé dans le paragraphe. */}
            <Section title={t("consoles.paramsFileTitle")} sense="modify">
                {intro && (
                    <Note className="mt-0">
                        {emulator.description && <span className="d-block">{emulator.description}</span>}
                        {emulator.requirements?.length > 0 && <span className="d-block">{t("consoles.paramsRequirements", { list: emulator.requirements.join(", ") })}</span>}
                    </Note>
                )}
                {/* LE FICHIER ET SON ÉTAT, des renseignements (InfoRow) : son chemin, puis, s'il manque ou ne se lit pas, ce
                    qu'il en est et ce que fera l'enregistrement — plus des alertes (le porteur, 2026-11-16 : « retirer les
                    alerts »). */}
                {(emulator.file || emulator.error || !emulator.exists) && (
                    <Rows className={intro ? "mt-3" : ""}>
                        {emulator.file && <InfoRow icon="bi-file-earmark-code" label={t("consoles.paramsFileLabel")} value={emulator.file} mono />}
                        {emulator.error && <InfoRow icon="bi-exclamation-triangle" label={t("consoles.paramsFileState")} value={t("consoles.paramsFileUnreadable")} pill="danger"
                            help={t("consoles.paramsBroken", { error: emulator.error })} />}
                        {!emulator.exists && <InfoRow icon="bi-file-earmark" label={t("consoles.paramsFileState")} value={t("consoles.paramsFileAbsent")} pill="danger" help={t("consoles.paramsMissing")} />}
                    </Rows>
                )}
                <Rows className={intro || emulator.file || emulator.error || !emulator.exists ? "mt-3" : ""}>
                    <SwitchRow id="pm-enabled" label={t("consoles.paramsEnabled")} help={t("consoles.paramsEnabledHelp")} checked={Boolean(enabled)} onChange={setEnabled} />
                </Rows>
            </Section>
            {(emulator.presets.length > 0 || lent.length > 0) && (
                <Section title={t("consoles.paramsPresetsTitle")} sense="modify">
                    <Rows>
                        <SelectRow id="pm-preset" label={t("consoles.paramsPreset")} help={presetHelp} value={preset} onChange={choose} options={presetOptions} />
                    </Rows>
                </Section>
            )}
            {emulator.params.length > 0 && <Section title={t("consoles.tabParams")} sense="modify"><Rows>{emulator.params.map(row)}</Rows></Section>}
            {engines.length > 0 && (
                <Section title={t("consoles.paramsEngines")} help={t("consoles.paramsEnginesHelp")} sense="modify">
                    <Rows>
                        {engines.map((setting) => (
                            <React.Fragment key={setting.engine}>
                                <SelectRow id={`pm-engine-${setting.engine}`} label={setting.name || setting.engine} value={setting.preset || ""} onChange={(chosen) => setEngine(setting.engine, { preset: chosen })}
                                    help={setting.error ? t("consoles.paramsEngineBroken", { error: setting.error }) : t("consoles.paramsEngineHelp")}
                                    options={[{ value: "", label: t("consoles.paramsEngineNone") }, ...emulator.presets.filter((item) => !item.error).map((item) => ({ value: item.id, label: item.name }))]} />
                                <SwitchRow id={`pm-engine-on-${setting.engine}`} label={t("consoles.paramsEngineEnabled")} help={fact(t("gear.engine"), setting.name || setting.engine)}
                                    checked={Boolean(setting.enabled)} onChange={(on) => setEngine(setting.engine, { enabled: on })} />
                            </React.Fragment>
                        ))}
                    </Rows>
                </Section>
            )}
            <Section title={t("consoles.paramsSaveTitle")} sense="modify">
                <Rows>
                    <SwitchRow id="pm-save-as" label={t("consoles.paramsSaveAsPreset")} help={t("consoles.paramsSaveAsPresetHelp")} checked={saveAs.on}
                        onChange={(on) => setSaveAs((current) => ({ ...current, on }))} />
                    <TextRow id="pm-preset-name" label={t("consoles.paramsPresetName")} help={t("consoles.paramsPresetNameHelp")} context={presetNameContext} />
                    <SubmitRow>
                        <FormButton id="pm-save" label={t("settings.save")} isReplaced={Boolean(changed)} pending={pending} disabled={anyTextInvalid || Boolean(presetNameProblem)} />
                    </SubmitRow>
                </Rows>
            </Section>
            {selected?.override && (
                <DangerSection id="pm-preset-del" label={t("consoles.paramsPresetDelete")} question={t("consoles.paramsPresetDeleteConfirm", { name: selected.name })} onConfirm={async () => {
                    const answer = await deleteEmulatorPreset(emulator.consoleId, emulator.emulator, selected.id);
                    feedback.result(answer.ok, t("consoles.paramsPresetDeleted", { name: selected.name }), answer.errorMessage || t("consoles.paramsFailed"));

                    if (answer.ok && answer.data?.params) {
                        setEmulator(inLanguage(answer.data, language));
                        setPreset("");
                        setSaveAs({ on: false, name: "" });
                    }
                }} />
            )}
            <GuideSection guides={[["emulatorSettings", "change-an-emulators-settings"]]} />
        </Form>
    );
}

// /settings/consoles/<console>/params/<émulateur> (« Paramètres d'Eden », une ligne du « Plus » de la console) :
// l'émulateur relu à chaque ouverture dans la liste des paramètres de ce serveur — jamais pris dans l'historique,
// qui garderait, après un rechargement, le fichier d'avant le dernier enregistrement.
export default function Params() {
    const { t } = useTranslation();
    const { consoleId, emulator: emulatorId } = useParams();
    const [emulator, setEmulator] = useState(undefined);

    useEffect(() => {
        let cancelled = false;
        listEmulatorParams().then((answer) => {
            if (!cancelled) {
                setEmulator((answer.ok ? (answer.data?.emulators || []) : []).find((entry) => entry.consoleId === consoleId && entry.emulator === emulatorId) || null);
            }
        });

        return () => {
            cancelled = true;
        };
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [consoleId, emulatorId]);

    return (
        <SettingsPage title={emulator ? paramsLabel(emulator, t) : emulatorId} parent={consolePath(consoleId)}>
            {emulator === undefined ? <Section><Loading /></Section>
                : emulator === null ? <NotFound /> : <EmulatorForm key={`${emulator.consoleId}/${emulator.emulator}`} initial={emulator} />}
        </SettingsPage>
    );
}

// L'APPARENCE D'UNE CONSOLE (chantier 53, P9, #678 ; la décision 11 et le modèle 30, puis les remarques d et e du
// porteur, 2026-11-14) — /settings/consoles/<console>/appearance :
//   - ses informations (le nom, le constructeur, l'année), le dossier de ses images ;
//   - SES HUIT EMPLACEMENTS (« gardons maximum 8 images par console. On peut donc remplacer des images dans les 1 à 8
//     slots seulement ») : la bande de la console les montre dans cet ordre. Un emplacement pris ouvre sa page ; un
//     libre, la recherche qui le remplit ;
//   - les images du dossier EN TROP, quand il y en a : signalées, ni ouvertes ni effacées — ce sont ses fichiers ;
//   - « Plus » : « Ajouter une image » (le premier emplacement libre ; grisée quand les huit sont pris) et
//     « Réinitialiser les images par défaut » — qui ne détruit rien : le serveur redemande les images par défaut à la
//     visite suivante, dans les emplacements libres (console_backgrounds.go). C'est pourquoi elle est dans « Plus » et
//     non dans « Danger » (la règle des boutons : « Danger » pour ce qui détruit).
// …/appearance/<n>          un emplacement pris : l'image, la remplacer (« Plus ») — une image ne se supprime pas
//                           seule (le porteur, 2026-11-14 : « Il est préférable de ne pas laisser le choix de supprimer
//                           une image. On ne peut que la remplacer. ») ;
// …/appearance/search?slot= la recherche qui remplit ou remplace l'emplacement (ImageSearch, ImageSearch.jsx),
//                           l'image actuelle au-dessus quand il est pris.
// L'envoi d'un fichier (Form.File) et l'ancienne grille des « Fonds » sont partis (AGENTS.md : aucune rétro
// compatibilité).
