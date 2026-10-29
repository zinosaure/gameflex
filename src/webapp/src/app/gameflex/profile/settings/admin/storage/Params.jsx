import React, { useState, useEffect, useCallback } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../../skeleton/skeleton.js";
import {
    DownloadRing,
    FormButton,
    ListElement,
    ListingElement,
    OffCanvas,
    useLatestContext,
} from "../../../../shared/templates/Essentials.jsx";
import {
    deleteEmulatorPreset,
    fetchEmulatorParams,
    fetchSourceParams,
    importFromSource,
    parseSource,
    writeEmulatorParams,
} from "../api/storage-admin.js";
import SourceSelector, { useDiskSources } from "./SourceSelector.jsx";
import MirrorNotice from "./MirrorNotice.jsx";
import { federatedServers } from "../../../../libs/federatedHost.js";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// LES PARAMÈTRES DES ÉMULATEURS — `params/<console>/<émulateur>.json`
// (2026-10-12).
//
// « Il faut faire d'une façon où on peut changer les réglages sans
// reconstruire l'APK à chaque fois » (le porteur, en réglant RPCS3). Le
// fichier porte la liste des paramètres — type, libellé, description, valeur,
// options — et cet écran en construit le formulaire : rien n'y est écrit en
// dur, un paramètre ajouté au fichier à la main y apparaît.
//
// D'ABORD LA LISTE DES ÉMULATEURS, puis le formulaire de celui qu'on choisit
// (« Liste émulateurs -> formulaire dynamique », le porteur) : un panneau par
// émulateur, comme un journal dans l'écran des journaux.
//
// Les préréglages (`presets/*.json`) ne font que REMPLIR les champs : « en
// tant qu'admin je peux choisir quel preset je veux appliquer sur les
// paramètres, et ensuite sauvegarder ». Rien n'est écrit avant « Enregistrer ».
//
// « SAVE AS PRESET », EN BAS DU FORMULAIRE (le porteur, 2026-10-12) : les
// valeurs enregistrées sont gardées aussi en préréglage, sous le slug de leur
// nom, « pour revenir dans le futur sur un preset custom ». Chaque préréglage
// dit dans son fichier s'il se remplace (`"override": true|false`) : ceux de
// GameFlex sont en lecture seule — on en part pour créer le sien sous un autre
// nom —, ceux qu'on enregistre se remplacent et se suppriment (le bouton à côté
// de la liste).
//
// LES RÉGLAGES PAR MOTEUR DE JEU (chantier 36), pour une console dont les jeux
// ont un moteur reconnu (la PS4, la PS5) : chaque moteur désigne un des
// préréglages ci-dessus, qui s'applique par-dessus les valeurs du fichier à
// chacun de ses jeux — le moteur gagne, son interrupteur le coupe. Enregistrés
// avec le reste.
//
// Les textes du formulaire viennent du fichier, tels quels : ceux que GameFlex
// livre sont en anglais (« il faut seulement que ça soit en anglais par
// défaut — on ne va pas pouvoir les traduire », le porteur).
//
// Le même écran sur le téléphone et sur le serveur — « c'est une feature en
// commun » : les deux kernels servent les mêmes routes depuis
// gameflex/emuparams. Le téléphone pose le fichier de RPCS3 au premier
// démarrage ; le serveur n'en pose aucun, et sans fichier rien n'est appliqué.

// Les valeurs du fichier, telles que le formulaire les tient : un
// interrupteur en booléen, le reste en texte.
const valuesOf = (emulator) => Object.fromEntries(emulator.params.map((param) => [param.id, param.value]));

const titleOf = (emulator) => `${emulator.name} — ${emulator.system}`;

// Les réglages des moteurs, tels que l'enregistrement les envoie.
const enginesOf = (emulator) => (emulator.engines || []).map(({ engine, name, preset, enabled, error }) => ({
    engine, name, preset, enabled, error,
}));

// L'état d'un émulateur, en une ligne de la liste.
const stateOf = (emulator, t) => {
    if (emulator.error) {
        return t("admin.storage.paramsStateBroken");
    }
    if (!emulator.exists) {
        return t("admin.storage.paramsStateMissing");
    }

    return emulator.enabled ? t("admin.storage.paramsStateEnabled") : t("admin.storage.paramsStateDisabled");
};

function ParamField({ param, context }) {
    // L'adresse dans le fichier de l'émulateur, avec ses propres noms : c'est
    // sous ce nom-là qu'on le retrouve dans son journal ou sa documentation.
    const helpText = [param.description, param.config].filter(Boolean).join(" — ");

    if (param.type === "switch") {
        return (
            <Form.Group>
                <Form.SkeletonSwitch
                    name={param.id}
                    context={context}
                    reverse={true}
                    label={param.label}
                    helpText={helpText} />
            </Form.Group>
        );
    }

    return (
        <Form.Group label={param.label} helpText={helpText}>
            {param.type === "select"
                ? <Form.Select name={param.id} context={context} options={param.options || []} />
                : <Form.Text name={param.id} context={context}
                    props={{ autoComplete: "off", autoCapitalize: "off" }} />}
        </Form.Group>
    );
}

// EmulatorParams — le formulaire d'UN émulateur, dans son panneau.
//
// Un panneau est un instantané (voir ListingElement) : il ne re-rend pas avec
// la liste. Le formulaire tient donc lui-même le fichier qu'il montre, et le
// remplace par la réponse d'un enregistrement ; la liste, elle, est prévenue
// par onSaved.
// Rien à garder en préréglage : l'interrupteur décoché, le nom vide.
const NO_PRESET_SAVE = { saveAsPreset: false, presetName: "" };

function EmulatorParams({ initial, onSaved, t }) {
    const [emulator, setEmulator] = useState(initial);
    const [values, setValues] = useState(() => valuesOf(initial));
    const [enabled, setEnabled] = useState(initial.exists ? initial.enabled : true);
    const [engines, setEngines] = useState(() => enginesOf(initial));
    const [preset, setPreset] = useState("");
    const [presetSave, setPresetSave] = useState(NO_PRESET_SAVE);
    const [isSaving, setSaving] = useState(false);
    const [isDeleting, setDeleting] = useState(false);
    const latestContext = useLatestContext();
    const context = latestContext.current;

    const saved = valuesOf(emulator);
    // Sans fichier, ou avec un fichier illisible, enregistrer a toujours un
    // effet : créer le fichier, ou le réparer.
    const savedEngines = enginesOf(emulator);
    const isChanged = !emulator.exists || Boolean(emulator.error)
        || enabled !== emulator.enabled
        || emulator.params.some((param) => values[param.id] !== saved[param.id])
        || engines.some((setting, index) => setting.preset !== savedEngines[index]?.preset
            || setting.enabled !== savedEngines[index]?.enabled);
    const selected = emulator.presets.find((item) => item.id === preset) || null;

    const choosePreset = (id) => {
        setPreset(id);
        const chosen = emulator.presets.find((item) => item.id === id);
        // PARTIR D'UN PRÉRÉGLAGE QUI SE REMPLACE, c'est le modifier :
        // l'interrupteur se coche et son nom est gardé — modifiable, pour ne
        // pas l'écraser. Un préréglage en lecture seule ne se remplace pas : on
        // en part pour créer le sien, sous un nom qu'on choisit.
        setPresetSave(chosen && chosen.override && !chosen.error
            ? { saveAsPreset: true, presetName: chosen.name }
            : NO_PRESET_SAVE);
        if (!chosen || chosen.error) {
            return;
        }
        setValues((current) => ({ ...current, ...chosen.values }));
        const ignored = chosen.ignored || [];
        // Ce qu'un préréglage ne remplit pas se DIT : un champ resté tel quel
        // passerait pour réglé par lui.
        context.toast.open(
            [
                t("admin.storage.presetFilled", { name: chosen.name }),
                ignored.length ? t("admin.storage.presetIgnored", { list: ignored.join(", ") }) : null,
            ].filter(Boolean).join(" "),
            { variant: ignored.length ? "warning" : "success" },
        );
    };

    const save = async () => {
        setSaving(true);
        const presetName = presetSave.saveAsPreset ? presetSave.presetName.trim() : "";
        try {
            const result = await writeEmulatorParams(emulator.consoleId, emulator.emulator, {
                enabled, values, preset: presetName,
                ...(engines.length > 0 ? {
                    engines: engines.map(({ engine, preset: chosen, enabled: on }) => ({ engine, preset: chosen, enabled: on })),
                } : {}),
            });
            context.toast.open(
                result.ok
                    ? (result.savedPreset
                        ? t("admin.storage.paramsSavedWithPreset", { name: presetName })
                        : t("admin.storage.paramsSaved"))
                    : (result.errorMessage || t("admin.storage.paramsFailed")),
                { variant: result.ok ? "success" : "error" },
            );
            if (result.ok) {
                // LA RÉPONSE EST LE NOUVEAU FICHIER : le formulaire repart de
                // ce qu'il dit. Le préréglage qu'on vient d'écrire est celui
                // que les champs montrent : il reste choisi, l'interrupteur
                // coché à son nom.
                setEmulator(result);
                setValues(valuesOf(result));
                setEnabled(result.enabled);
                setEngines(enginesOf(result));
                setPreset(result.savedPreset || "");
                if (!result.savedPreset) {
                    setPresetSave(NO_PRESET_SAVE);
                }
                onSaved(result);
            }
        } catch (failure) {
            console.error("Enregistrement des paramètres", failure);
            context.toast.open(t("admin.storage.paramsFailed"), { variant: "error" });
        } finally {
            setSaving(false);
        }
    };

    // SUPPRIMER UN PRÉRÉGLAGE ne touche ni aux champs ni au fichier de
    // l'émulateur : seulement à la liste des préréglages.
    const deletePreset = async (target) => {
        setDeleting(true);
        try {
            const result = await deleteEmulatorPreset(emulator.consoleId, emulator.emulator, target.id);
            context.toast.open(
                result.ok
                    ? t("admin.storage.paramsPresetDeleted", { name: target.name })
                    : (result.errorMessage || t("admin.storage.paramsFailed")),
                { variant: result.ok ? "success" : "error" },
            );
            if (result.ok) {
                setEmulator(result);
                setPreset("");
                setPresetSave(NO_PRESET_SAVE);
                onSaved(result);
            }
        } catch (failure) {
            console.error("Suppression d'un préréglage", failure);
            context.toast.open(t("admin.storage.paramsFailed"), { variant: "error" });
        } finally {
            setDeleting(false);
        }
    };

    const confirmDelete = () => {
        const target = selected;
        context.modal.open(
            <div className="text-center">
                {t("admin.storage.paramsPresetDeleteConfirm")} <br />
                <strong>{target.name}</strong>
            </div>,
            {
                variant: "confirm",
                onConfirm: () => deletePreset(target),
                confirmLabel: t("app.delete"),
                cancelLabel: t("app.cancel"),
            },
        );
    };

    // Un fichier illisible a peut-être été écrit à la main : le remplacer se
    // demande.
    const confirmSave = () => (emulator.error
        ? context.modal.open(
            <div className="text-center">{t("admin.storage.paramsReplaceConfirm")}</div>,
            {
                variant: "confirm",
                onConfirm: save,
                confirmLabel: t("app.save"),
                cancelLabel: t("app.cancel"),
            },
        )
        : save());

    const formContext = {
        values,
        setValues,
        files: {},
        setFiles: () => { },
        rulesets: Object.fromEntries(emulator.params
            .filter((param) => param.type === "text")
            .map((param) => [param.id, (data) => (String(data[param.id] ?? "").trim() !== ""
                && !/[\r\n]/.test(String(data[param.id] ?? ""))
                ? null
                : Ruleset.isInvalid(t("admin.storage.paramTextInvalid")))])),
    };
    const enabledContext = {
        values: { enabled },
        setValues: (next) => setEnabled(Boolean(next.enabled)),
        files: {},
        setFiles: () => { },
    };
    const presetContext = {
        values: { preset },
        setValues: (next) => choosePreset(next.preset || ""),
        files: {},
        setFiles: () => { },
    };
    const engineContext = (setting) => ({
        values: { preset: setting.preset, enabled: setting.enabled },
        setValues: (next) => setEngines((current) => current.map((item) => (item.engine === setting.engine
            ? { ...item, preset: String(next.preset ?? ""), enabled: Boolean(next.enabled) }
            : item))),
        files: {},
        setFiles: () => { },
    });
    const engineOptions = [
        { value: "", label: t("admin.storage.paramsEngineNone") },
        ...emulator.presets.filter((item) => !item.error).map((item) => ({ value: item.id, label: item.name })),
    ];
    // Le nom d'un préréglage en lecture seule ne se reprend pas : le kernel le
    // refuserait (il compare les noms de FICHIER, ceci n'en est que l'écho).
    const readOnlyNames = new Set(emulator.presets
        .filter((item) => !item.override)
        .map((item) => String(item.name).trim().toLowerCase()));
    const presetSaveContext = {
        values: presetSave,
        setValues: (next) => setPresetSave({
            saveAsPreset: Boolean(next.saveAsPreset),
            presetName: String(next.presetName ?? ""),
        }),
        files: {},
        setFiles: () => { },
        rulesets: {
            // « Si le switch est coché, le champ est enabled, sinon l'inverse »
            // (le porteur).
            presetName: (data) => {
                if (!data.saveAsPreset) {
                    return Ruleset.isDisabled();
                }
                const name = String(data.presetName ?? "").trim();
                if (!name) {
                    return Ruleset.isInvalid(t("admin.storage.paramsPresetNameInvalid"));
                }
                if (readOnlyNames.has(name.toLowerCase())) {
                    return Ruleset.isInvalid(t("admin.storage.paramsPresetNameReadOnly"));
                }

                return null;
            },
        },
    };

    return (
        <>
            <bs.Alert variant="secondary" className="small">
                {emulator.requirements.length > 0 && (
                    <span className="d-block">
                        {t("admin.storage.paramsRequirements", { list: emulator.requirements.join(", ") })}
                    </span>
                )}
                <span className="d-block">{t("admin.storage.paramsFile", { file: emulator.file })}</span>
            </bs.Alert>
            {emulator.error && (
                <bs.Alert variant="danger">{t("admin.storage.paramsBroken", { error: emulator.error })}</bs.Alert>
            )}
            {!emulator.exists && (
                <bs.Alert variant="secondary" className="small">{t("admin.storage.paramsMissing")}</bs.Alert>
            )}
            <Form.Group>
                <Form.SkeletonSwitch
                    name="enabled"
                    context={enabledContext}
                    reverse={true}
                    label={t("admin.storage.paramsEnabled")}
                    helpText={t("admin.storage.paramsEnabledHelp")} />
            </Form.Group>
            {emulator.presets.length > 0 && (
                <>
                    <Form.Group
                        label={t("admin.storage.paramsPreset")}
                        helpText={selected && !selected.override
                            ? t("admin.storage.paramsPresetReadOnly")
                            : t("admin.storage.paramsPresetHelp")}>
                        <div className="d-flex align-items-end gap-2">
                            <div className="flex-grow-1">
                                <Form.Select
                                    name="preset"
                                    context={presetContext}
                                    options={[
                                        { value: "", label: t("admin.storage.paramsPresetNone") },
                                        ...emulator.presets.map((item) => ({
                                            value: item.id,
                                            label: item.error ? `${item.name} — ${t("admin.storage.presetBroken")}` : item.name,
                                            // Illisible, il ne remplit rien — mais
                                            // s'il est à l'admin, on doit pouvoir le
                                            // choisir pour le supprimer.
                                            attributes: item.error && !item.override ? { disabled: true } : {},
                                        })),
                                    ]} />
                            </div>
                            {/* SUPPRIMER, À CÔTÉ DE LA LISTE (le porteur) — grisé
                                pour un préréglage en lecture seule, et le texte
                                d'aide dit pourquoi : un téléphone n'affiche
                                jamais d'infobulle. */}
                            <bs.Button
                                variant="danger"
                                disabled={!selected || !selected.override || isDeleting}
                                title={t("admin.storage.paramsPresetDelete")}
                                aria-label={t("admin.storage.paramsPresetDelete")}
                                onClick={(event) => {
                                    event.preventDefault();
                                    confirmDelete();
                                }}>
                                {isDeleting
                                    ? <bs.Spinner animation="border" size="sm" />
                                    : <i className="bi bi-trash"></i>}
                            </bs.Button>
                        </div>
                    </Form.Group>
                    {/* Au-dessus, ce qui remplit ; en dessous, ce qui sera
                        écrit (« pour qu'on puisse savoir que c'est des
                        valeurs prédéfinies », le porteur). */}
                    <hr />
                </>
            )}
            {emulator.params.map((param) => (
                <ParamField key={param.id} param={param} context={formContext} />
            ))}
            {engines.length > 0 && (
                <>
                    <hr />
                    <h6>{t("admin.storage.paramsEngines")}</h6>
                    <p className="small text-body-secondary">{t("admin.storage.paramsEnginesHelp")}</p>
                    {engines.map((setting) => (
                        <Form.Group
                            key={setting.engine}
                            label={setting.name}
                            helpText={setting.error ? t("admin.storage.paramsEngineBroken", { error: setting.error }) : null}>
                            <div className="d-flex align-items-center gap-3">
                                <div className="flex-grow-1">
                                    <Form.Select name="preset" context={engineContext(setting)} options={engineOptions} />
                                </div>
                                <Form.SkeletonSwitch
                                    name="enabled"
                                    context={engineContext(setting)}
                                    reverse={true}
                                    label={t("admin.storage.paramsEngineEnabled")} />
                            </div>
                        </Form.Group>
                    ))}
                </>
            )}
            {/* Au-dessus, ce que le fichier recevra ; en dessous, de quoi le
                garder AUSSI en préréglage (« Save as preset », le porteur). */}
            <hr />
            <Form.Group>
                <Form.SkeletonSwitch
                    name="saveAsPreset"
                    context={presetSaveContext}
                    reverse={true}
                    label={t("admin.storage.paramsSaveAsPreset")}
                    helpText={t("admin.storage.paramsSaveAsPresetHelp")} />
            </Form.Group>
            <Form.Group
                label={t("admin.storage.paramsPresetName")}
                helpText={t("admin.storage.paramsPresetNameHelp")}>
                <Form.Text name="presetName" context={presetSaveContext}
                    props={{ autoComplete: "off", autoCapitalize: "off", maxLength: 80 }} />
            </Form.Group>
            <FormButton
                callback={confirmSave}
                label={t("app.save")}
                // Garder les valeurs en préréglage est un effet en soi, même
                // sans rien changer au fichier.
                isDisabled={(!isChanged && !presetSave.saveAsPreset)
                    || Boolean(Ruleset.hasFeedbacks(formContext))
                    || Boolean(Ruleset.hasFeedbacks(presetSaveContext))}
                isPending={isSaving} />
        </>
    );
}

// CHEZ UN SERVEUR FÉDÉRÉ (chantier 15) OU SUR UN DISQUE INTERNE (chantier
// 24) : ce qu'il prête — ses valeurs choisies, ses préréglages perso —,
// chacun avec le préréglage qu'il deviendrait ici (`<source>-<préréglage>`,
// décidé par le porteur), et « Importer ». Ce qu'on importe se retrouve
// ensuite parmi les préréglages d'ici, qu'on applique comme les autres. Rien
// n'écrase rien.
function SourceParams({ source, t }) {
    const latestContext = useLatestContext();
    const [offers, setOffers] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [pending, setPending] = useState(null);

    useEffect(() => {
        let cancelled = false;
        setLoading(true);
        fetchSourceParams(source)
            .then((answer) => {
                if (cancelled) {
                    return;
                }
                setOffers((answer.consoles || []).flatMap((section) => section.items.map((offer) => ({
                    ...offer, consoleId: section.consoleId, console: section.console,
                }))));
            })
            .finally(() => {
                if (!cancelled) {
                    setLoading(false);
                }
            });

        return () => {
            cancelled = true;
        };
    }, [source]);

    const keyOf = (offer) => `${offer.consoleId}/${offer.emulator}/${offer.preset || ""}`;

    const handleImport = async (offer) => {
        setPending(keyOf(offer));
        try {
            const answer = await importFromSource(source, "params", {
                consoleId: offer.consoleId, emulator: offer.emulator, preset: offer.preset || "",
            });
            latestContext.current.toast.open(
                answer.ok ? t("admin.storage.paramsImported", { name: answer.preset }) : (answer.errorMessage || t("app.error")),
                { variant: answer.ok ? "success" : "error" },
            );
            if (answer.ok) {
                setOffers((current) => current.map((other) => (
                    keyOf(other) === keyOf(offer) ? { ...other, installed: true } : other)));
            }
        } finally {
            setPending(null);
        }
    };

    const actionFor = (offer) => {
        if (pending === keyOf(offer)) {
            return <DownloadRing percent={null} label={t("admin.storage.paramsImport")} />;
        }
        if (offer.installed) {
            return <i className="bi bi-check-circle-fill text-success fs-4" title={t("admin.storage.installed")}></i>;
        }

        return (
            <bs.Button
                variant="primary"
                disabled={pending !== null}
                title={t("admin.storage.paramsImport")}
                onClick={(event) => {
                    event.preventDefault();
                    event.stopPropagation();
                    handleImport(offer);
                }}>
                <i className="bi bi-download"></i>
            </bs.Button>
        );
    };

    return (
        <ListingElement
            callback={(offer) => (
                <ListElement
                    key={keyOf(offer)}
                    icon="bi bi-sliders"
                    label={offer.preset ? offer.name : t("admin.storage.paramsTheirValues")}
                    helpText={`${offer.console} · ${offer.emulator} · ${t("admin.storage.paramsBecomes", { name: offer.as })}`}
                    next={actionFor(offer)} />
            )}
            items={offers}
            itemCount={offers.length}
            itemsPerPage={Math.max(offers.length, 1)}
            isLoading={isLoading}
            search={{ context: null }}
            texts={{
                itemsFound: t("admin.storage.paramsFound"),
                noItemsFound: t("admin.storage.noLentParams"),
            }} />
    );
}

export default function Params({ t }) {
    const [emulators, setEmulators] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [source, setSource] = useState("local");
    const [peers, setPeers] = useState([]);
    const diskSources = useDiskSources(t);
    // L'icône re-télécharge la copie d'un ami : sa liste se relit alors.
    const [generation, setGeneration] = useState(0);
    const latestContext = useLatestContext();

    useEffect(() => {
        let cancelled = false;
        federatedServers()
            .then((found) => {
                if (!cancelled) {
                    setPeers(found || []);
                }
            })
            .catch(() => undefined);

        return () => {
            cancelled = true;
        };
    }, []);

    const reload = useCallback(() => {
        setLoading(true);

        return fetchEmulatorParams()
            .then((result) => {
                if (!result.ok) {
                    latestContext.current.toast.open(result.errorMessage || t("admin.storage.loadFailed"),
                        { variant: "error" });
                }
                setEmulators(result.emulators || []);
            })
            .catch((failure) => {
                console.error("Chargement des paramètres des émulateurs", failure);
                latestContext.current.toast.open(t("admin.storage.loadFailed"), { variant: "error" });
            })
            .finally(() => setLoading(false));
    }, [t, latestContext]);

    useEffect(() => {
        reload();
    }, [reload]);

    // LA RÉPONSE DE L'ENREGISTREMENT EST LE NOUVEAU FICHIER : la ligne de la
    // liste change sur place, rien à relire.
    const replace = (saved) => setEmulators((current) => current.map(
        (item) => (item.consoleId === saved.consoleId && item.emulator === saved.emulator ? saved : item)));

    const open = (emulator) => latestContext.current.offcanvas.open(
        <OffCanvas icon="bi bi-sliders" label={titleOf(emulator)} helpText={emulator.description}>
            <EmulatorParams initial={emulator} onSaved={replace} t={t} />
        </OffCanvas>, { size: 600 });

    const sourceOptions = [
        { value: "local", label: t("admin.storage.sourceHere") },
        ...diskSources,
        ...peers.map((peer) => ({ value: `peer:${peer.id}`, label: peer.hostname || peer.link })),
    ];
    const { kind } = parseSource(source);

    return (
        <>
            <bs.Alert variant="secondary" className="small">
                {t("admin.storage.paramsNotice")}
            </bs.Alert>
            <MirrorNotice t={t} section="params" />
            <SourceSelector
                t={t}
                value={source}
                onChange={setSource}
                options={sourceOptions}
                onRefreshed={kind === "local" ? reload : () => setGeneration((current) => current + 1)}
                section="params"
                helpText={t("admin.storage.paramsSourceHelp")} />
            {kind !== "local" ? <SourceParams key={`${source}:${generation}`} source={source} t={t} /> : <ListingElement
                callback={(emulator) => (
                    <ListElement
                        key={`${emulator.consoleId}/${emulator.emulator}`}
                        icon="bi bi-sliders"
                        label={titleOf(emulator)}
                        helpText={stateOf(emulator, t)}
                        onClick={() => open(emulator)} />
                )}
                items={emulators}
                itemCount={emulators.length}
                itemsPerPage={Math.max(emulators.length, 1)}
                isLoading={isLoading}
                search={{ context: null }}
                texts={{
                    itemsFound: t("admin.storage.paramsFound"),
                    noItemsFound: t("admin.storage.noParams"),
                }} />}
        </>
    );
}
