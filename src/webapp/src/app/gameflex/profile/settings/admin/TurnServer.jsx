import React, { useEffect, useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import { readTurnConfig, saveTurnConfig } from "./api/turnConfig.js";
import {
    SectionPanel,
    FormButton,
} from "../../../shared/templates/Essentials.jsx";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

const GENERIC_TYPES = ["coturn", "eturnal"];

const emptyGenericEntry = () => ({ urls: "", username: "", passphrase: "", setted: false });
const emptyConfig = () => ({
    active: { category: "", genericType: "coturn" },
    generic: { coturn: emptyGenericEntry(), eturnal: emptyGenericEntry() },
    cloudflare: { tokenId: "", apiToken: "", setted: false },
});

const isUrlListValid = (raw) => {
    if (!raw || raw.trim() === "") return true; // emptiness is what `setted` is for, not a format error
    return raw.split(",").every((url) => /^turns?:/.test(url.trim()));
};

// The provider select and the generic form's coturn/eturnal switch are
// deliberately INDEPENDENT: the switch only picks which generic form you're
// looking at, while each generic type is its own entry here, selectable on
// its own. So editing coturn's settings while eturnal is the live provider
// is a perfectly ordinary thing to do, and neither one moves the other.
const activeToValue = (active) => {
    if (active.category === "generic") return `generic:${active.genericType}`;
    if (active.category === "cloudflare") return "cloudflare";
    return "";
};

const valueToActive = (value, fallbackGenericType) => {
    if (value.startsWith("generic:")) return { category: "generic", genericType: value.split(":")[1] };
    // genericType is meaningless for cloudflare (the backend ignores it) —
    // carry the existing one rather than resetting it, so flipping to
    // Cloudflare and back doesn't quietly change which generic type was live.
    if (value === "cloudflare") return { category: "cloudflare", genericType: fallbackGenericType };
    return { category: "", genericType: fallbackGenericType };
};

// The backend stores the generic entries as a list (one per type); the form
// is far easier to write against a map keyed by type.
function configFromResponse(data) {
    const next = emptyConfig();
    next.active = {
        category: data?.active?.category || "",
        genericType: data?.active?.genericType || "coturn",
    };
    for (const entry of data?.generic || []) {
        if (GENERIC_TYPES.includes(entry.type)) {
            next.generic[entry.type] = {
                urls: entry.urls || "",
                username: entry.username || "",
                passphrase: entry.passphrase || "",
                setted: !!entry.setted,
            };
        }
    }
    next.cloudflare = {
        tokenId: data?.cloudflare?.tokenId || "",
        apiToken: data?.cloudflare?.apiToken || "",
        setted: !!data?.cloudflare?.setted,
    };
    return next;
}

function GenericSection({ config, genericType, onChange, onTypeChange, onSave, isSaving, isReplaced, t }) {
    const entry = config.generic[genericType];
    const contextForm = {
        // The switch's own value rides along in the same object as the
        // fields, since Form.SkeletonSwitchMode and the text inputs share one
        // context — `type` is what tells the rulesets which shape to enforce.
        values: { type: genericType, ...entry },
        setValues: (v) => {
            // Both SkeletonSwitchMode and the text inputs call this with the
            // FULL merged values object, not just what changed (see
            // skeleton/libs/Form.jsx), so a changed `type` is the only signal
            // that the switch — rather than a field — was touched. Switching
            // only changes which entry is displayed: the other one stays in
            // state untouched, which is the whole point of keeping both.
            if (v.type !== genericType) {
                onTypeChange(v.type);
                return;
            }
            onChange({ urls: v.urls, username: v.username, passphrase: v.passphrase });
        },
        files: {},
        setFiles: () => { },
        rulesets: {
            urls: (data) => (isUrlListValid(data.urls) ? null : Ruleset.isInvalid(t("admin.turnConfig.urlsInvalid"))),
            username: (data) => {
                // eturnal never uses a stored username — it derives a
                // short-lived one from the shared secret — so it's only
                // required for coturn. Staying quiet while the whole entry is
                // still blank keeps an untouched form from opening on a red
                // error; it only speaks up once you've started filling it in.
                if (data.type !== "coturn") return null;
                if (!data.urls && !data.passphrase && !data.username) return null;
                if (!data.username.trim()) return Ruleset.isInvalid(t("admin.turnConfig.usernameRequired"));
                return null;
            },
        },
    };

    return (
        <SectionPanel
            label={t("admin.turnConfig.genericLabel")}
            helpText={t("admin.turnConfig.genericDescription")}>

            <Form.Group
                label={t("admin.turnConfig.genericTypeLabel")}
                helpText={t("admin.turnConfig.genericTypeHelp")}>
                <Form.SkeletonSwitchMode
                    name="type"
                    context={contextForm}
                    options={[
                        { label: "Coturn", value: "coturn", icon: "bi bi-hdd-rack" },
                        { label: "Eturnal", value: "eturnal", icon: "bi bi-hdd-network" },
                    ]} />
            </Form.Group>

            <Form.Group label={t("admin.turnConfig.urls")} helpText={t("admin.turnConfig.urlsHelp")}>
                <Form.Text name="urls" context={contextForm} props={{ placeholder: "turn:example.com:3478", autoComplete: "off" }} />
            </Form.Group>

            {genericType === "coturn" && (
                <Form.Group label={t("admin.turnConfig.username")} helpText={t("admin.turnConfig.usernameHelp")}>
                    <Form.Text name="username" context={contextForm} props={{ autoComplete: "off" }} />
                </Form.Group>
            )}

            <Form.Group
                label={t("admin.turnConfig.passphrase")}
                helpText={genericType === "eturnal" ? t("admin.turnConfig.eturnalPassphraseHelp") : t("admin.turnConfig.coturnPassphraseHelp")}>
                <Form.Password name="passphrase" context={contextForm} />
            </Form.Group>

            <FormButton
                callback={onSave}
                label={t("app.save")}
                isDisabled={isSaving || !isReplaced || Ruleset.hasFeedbacks(contextForm)}
                isPending={isSaving} />
        </SectionPanel>
    );
}

function CloudflareSection({ config, onChange, onSave, isSaving, isReplaced, t }) {
    const contextForm = {
        values: config.cloudflare,
        setValues: (v) => onChange({ tokenId: v.tokenId, apiToken: v.apiToken }),
        files: {},
        setFiles: () => { },
        rulesets: {},
    };

    return (
        <SectionPanel
            label={t("admin.turnConfig.cloudflareLabel")}
            helpText={t("admin.turnConfig.cloudflareDescription")}>
            <Form.Group
                label={t("admin.turnConfig.cloudflareTokenId")}
                helpText={t("admin.turnConfig.cloudflareTokenIdHelp")}>
                <Form.Text name="tokenId" context={contextForm} props={{ autoComplete: "off" }} />
            </Form.Group>
            <Form.Group
                label={t("admin.turnConfig.cloudflareApiToken")}
                helpText={t("admin.turnConfig.cloudflareApiTokenHelp")}>
                <Form.Password name="apiToken" context={contextForm} />
            </Form.Group>

            <FormButton
                callback={onSave}
                label={t("app.save")}
                isDisabled={isSaving || !isReplaced}
                isPending={isSaving} />
        </SectionPanel>
    );
}

// Admin page for golang/src/internal/iceservers — the ICE (STUN/TURN) server
// list every WebRTC PeerConnection in this backend uses, retro-gaming and
// remote-play alike. It is now the ONLY way to configure TURN; the env vars
// that used to do it were removed (see docker-compose.yaml).
//
// Two categories, two forms, and one separate activation step:
//   1. "Generic" holds credentials for a TURN server we talk to directly.
//      Coturn and eturnal each keep their OWN saved entry — the switch picks
//      which one you're editing, it never overwrites the other — so you can
//      keep both configured and flip between them.
//   2. "Cloudflare" mints credentials through their API instead.
//   3. "Active provider" at the bottom is the only thing that changes which
//      one List() actually uses. A category can only be selected once its
//      credentials are SAVED and complete (`setted`, computed by the backend
//      — so what you see here matches what the server would accept).
//
// Saving from either form persists the whole configuration, since the page
// always holds the complete state it loaded.
export default function TurnServer({ context, t }) {
    const [config, setConfig] = useState(emptyConfig);
    const [savedConfig, setSavedConfig] = useState(emptyConfig);
    // Which generic form is on screen. Separate from selectedProvider below —
    // see activeToValue's note.
    const [genericType, setGenericType] = useState("coturn");
    const [selectedProvider, setSelectedProvider] = useState("");
    const [isLoading, setLoading] = useState(true);
    const [isSaving, setSaving] = useState(false);
    const [isActivating, setActivating] = useState(false);

    const load = async () => {
        const result = await readTurnConfig();
        if (!result.ok) {
            context.toast.open(t("admin.turnConfig.loadFailed"), { variant: "error" });
            return null;
        }

        const next = configFromResponse(result.data);
        setConfig(next);
        setSavedConfig(next);
        setSelectedProvider(activeToValue(next.active));
        if (next.active.category === "generic") {
            // Only an opening default — open on the form for whatever is
            // live. Nothing keeps the two in step afterwards.
            setGenericType(next.active.genericType);
        }
        return next;
    };

    useEffect(() => {
        load().finally(() => setLoading(false));
    }, []);

    const save = async (active) => {
        const result = await saveTurnConfig({
            active,
            generic: GENERIC_TYPES.map((type) => ({ type, ...config.generic[type] })),
            cloudflare: config.cloudflare,
        });

        if (result.ok) {
            // Re-read rather than assume: `setted` is computed by the backend,
            // and it's what gates the activation select below — deriving it a
            // second time here would be a second definition free to drift.
            //
            // MAIS UNE RELECTURE RATÉE N'EST PAS UNE ÉCRITURE RATÉE : les
            // réglages sont déjà sur le disque à ce point. Sans cette garde,
            // le try/catch des appelants annonçait « impossible d'enregistrer »
            // pour un enregistrement qui avait parfaitement réussi.
            try {
                await load();
            } catch (failure) {
                console.error("relecture de la configuration TURN", failure);
            }

            context.toast.open(t("admin.turnConfig.saved"), { variant: "success" });
        } else {
            context.toast.open(t("admin.turnConfig.saveFailed"), { variant: "error" });
        }
        return result;
    };

    // Each generic type answers for itself, independently of which form the
    // switch happens to be showing.
    const isProviderReady = (value) => {
        if (value.startsWith("generic:")) return !!config.generic[value.split(":")[1]]?.setted;
        if (value === "cloudflare") return config.cloudflare.setted;
        return value === ""; // "none" is always a valid thing to switch to
    };

    const isGenericDirty = GENERIC_TYPES.some((type) => (
        ["urls", "username", "passphrase"].some((key) => config.generic[type][key] !== savedConfig.generic[type][key])
    ));
    const isCloudflareDirty = ["tokenId", "apiToken"].some((key) => config.cloudflare[key] !== savedConfig.cloudflare[key]);

    if (isLoading) {
        return (
            <bs.Container className="py-2 text-center">
                <bs.Spinner animation="border" role="status" />
            </bs.Container>
        );
    }

    const PROVIDERS = [
        { value: "generic:coturn", label: `${t("admin.turnConfig.genericLabel")} (Coturn)` },
        { value: "generic:eturnal", label: `${t("admin.turnConfig.genericLabel")} (Eturnal)` },
        { value: "cloudflare", label: t("admin.turnConfig.cloudflareLabel") },
    ];

    return (
        <>
            <SectionPanel>
                <bs.Tabs defaultActiveKey="generic" id="providers-tabs" className="nav-tabs-boxed mb-3" justify>
                    <bs.Tab eventKey="generic" title={t("admin.turnConfig.genericLabel")}>
                        <GenericSection
                            config={config}
                            genericType={genericType}
                            onTypeChange={setGenericType}
                            onChange={(fields) => setConfig((prev) => ({
                                ...prev,
                                generic: { ...prev.generic, [genericType]: { ...prev.generic[genericType], ...fields } },
                            }))}
                            onSave={async () => {
                                setSaving(true);

                                // Même raison que le `finally` de `load()` juste au-dessus :
                                // sans lui, un appel qui jette laisse le bouton figé sur
                                // « ••• » pour toujours, et la page n'a plus de sortie.
                                try {
                                    await save(config.active);
                                } catch (failure) {
                                    console.error("échec de l'enregistrement de la configuration TURN", failure);
                                    context.toast.open(t("admin.turnConfig.saveFailed"), { variant: "error" });
                                } finally {
                                    setSaving(false);
                                }
                            }}
                            isSaving={isSaving}
                            isReplaced={isGenericDirty}
                            t={t} />
                    </bs.Tab>
                    <bs.Tab eventKey="cloudflare" title={t("admin.turnConfig.cloudflareLabel")}>
                        <CloudflareSection
                            config={config}
                            onChange={(fields) => setConfig((prev) => ({ ...prev, cloudflare: { ...prev.cloudflare, ...fields } }))}
                            onSave={async () => {
                                setSaving(true);

                                try {
                                    await save(config.active);
                                } catch (failure) {
                                    console.error("échec de l'enregistrement de la configuration TURN", failure);
                                    context.toast.open(t("admin.turnConfig.saveFailed"), { variant: "error" });
                                } finally {
                                    setSaving(false);
                                }
                            }}
                            isSaving={isSaving}
                            isReplaced={isCloudflareDirty}
                            t={t} />
                    </bs.Tab>
                </bs.Tabs>
            </SectionPanel>

            <div className="inter-section" />

            <SectionPanel
                label={t("admin.turnConfig.activeProvider")}
                helpText={t("admin.turnConfig.activeProviderHelp")}>
                {/* react-bootstrap's Select directly, not Skeleton's Form.Select:
                options here need to be individually disabled (a category with
                nothing saved yet can't be activated) and Skeleton's doesn't
                support that. */}
                <bs.Form.Select
                    className="mb-3"
                    value={selectedProvider}
                    onChange={(e) => setSelectedProvider(e.target.value)}
                >
                    <option value="">{t("admin.turnConfig.noneSelected")}</option>
                    {PROVIDERS.map(({ value, label }) => (
                        <option key={value} value={value} disabled={!isProviderReady(value)}>
                            {label}{!isProviderReady(value) ? ` — ${t("admin.turnConfig.incomplete")}` : ""}
                        </option>
                    ))}
                </bs.Form.Select>
                <FormButton
                    callback={async () => {
                        setActivating(true);

                        try {
                            await save(valueToActive(selectedProvider, config.active.genericType));
                        } catch (failure) {
                            console.error("échec de l'activation du fournisseur TURN", failure);
                            context.toast.open(t("admin.turnConfig.saveFailed"), { variant: "error" });
                        } finally {
                            setActivating(false);
                        }
                    }}
                    label={t("app.save")}
                    isDisabled={isActivating || selectedProvider === activeToValue(config.active) || !isProviderReady(selectedProvider)}
                    isPending={isActivating} />
            </SectionPanel>
        </>
    );
}
