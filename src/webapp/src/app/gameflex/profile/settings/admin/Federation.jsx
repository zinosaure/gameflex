import React, { useState, useEffect, useMemo, useCallback } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import {
    OffCanvas,
    FormButton,
    SectionPanel,
    ListElement,
    ListingElement,
} from "../../../shared/templates/Essentials.jsx";
import {
    createFederationToken,
    updateFederationTokenScopes,
    revokeFederationToken,
    listFederationTokens,
} from "../../api.js";
import { CopyableValue } from "../../../shared/CopyableValue.jsx";
import { InvitationReveal } from "../../../shared/Invitation.jsx";
import { createInvitation, invitationLink, listInvitations, revokeInvitation } from "../../../api/invitations.js";
import { shareOrigin } from "../../../libs/serverBase.js";

const Form = Skeleton.Form;
const ITEMS_PER_PAGE = 10;


/**
 * LE SECRET, MONTRÉ UNE FOIS.
 *
 * La base n'en garde que l'empreinte : après cet écran, plus personne — pas
 * même le serveur — ne peut le relire. Cet écran EST donc la seule occasion
 * de le copier, et il doit le dire.
 *
 * Il manquait purement et simplement : le composant était appelé mais
 * n'existait nulle part, et créer un jeton faisait tomber la page. Le jeton
 * était bien créé — perdu dans la seconde (signalé le 2026-09-02).
 *
 * DEUX valeurs, pas une : l'adresse du serveur et le jeton. C'est ce que le
 * formulaire d'en face réclame, et les textes de cet écran le disaient déjà.
 */
function NewTokenReveal({ created, onDone, t, context }) {
    return (
        <SectionPanel
            label={t("admin.federation.createdTitle", { label: created?.label })}
            helpText={t("admin.federation.createdHelp")}>
            <bs.Alert variant="warning" className="d-flex gap-2 align-items-start">
                <i className="bi bi-exclamation-triangle-fill mt-1"></i>
                <div>{t("admin.federation.onceOnly")}</div>
            </bs.Alert>

            <CopyableValue
                label={t("admin.federation.linkLabel")}
                value={shareOrigin()}
                t={t}
                context={context} />
            <CopyableValue
                label={t("admin.federation.tokenLabel")}
                value={created?.token || ""}
                t={t}
                context={context} />

            <FormButton callback={onDone} label={t("admin.federation.done")} />
        </SectionPanel>
    );
}

/**
 * UNE INVITATION PLUTÔT QU'UN JETON (chantier 33, le porteur : « ça me donne
 * aussi un QR code à l'écran et la proposition de télécharger et partager »).
 * Le même formulaire — pour qui, prêter ses consoles — crée une invitation à
 * usage unique, valable 12 heures, dont le serveur d'en face tire le jeton.
 * « Plutôt le lien et le jeton » garde l'échange d'avant, en repli.
 */
function CreateTokenForm({ context, token, t, onCreated, onInvited }) {
    const [isCreating, setCreating] = useState(false);
    const [created, setCreated] = useState(null);
    const [invited, setInvited] = useState(null);
    const [label, setLabel] = useState("");
    // Le catalogue est toujours accordé — c'est ce qu'un jeton EST. Prêter
    // ses consoles se coche en plus, parce que c'est une autre décision :
    // l'une ouvre une bibliothèque, l'autre montre ce qu'il y a dans le
    // salon (chantier 9, C9-P2).
    const [sharesDevices, setSharesDevices] = useState(false);

    const contextForm = {
        values: { label, devices: sharesDevices },
        setValues: (values) => {
            setLabel(values.label ?? "");
            setSharesDevices(Boolean(values.devices));
        },
        feedbacks: {},
        setFeedbacks: () => { },
    };

    // LA LIGNE ENTRE DANS LA LISTE, SUR PLACE : la réponse est le jeton créé.
    // Ce formulaire relisait une liste à lui qu'il n'affichait pas, et la
    // liste, derrière, n'apprenait rien. Le secret reste ici : seul l'écran de
    // révélation le montre, une fois.
    const scopes = () => (sharesDevices ? ["catalog", "devices"] : ["catalog"]);

    const invite = async () => {
        setCreating(true);
        const result = await createInvitation(token, { kind: "federation", label: label.trim(), scopes: scopes() });
        setCreating(false);
        if (!result.ok) {
            context.toast.open(result.exception?.detail || t("app.error"), { variant: "error" });
            return;
        }
        setInvited(result.item);
        setLabel("");
        setSharesDevices(false);
        if (onInvited && result.item) {
            const { code: _code, ...row } = result.item;
            onInvited(row);
        }
    };

    const create = async () => {
        setCreating(true);
        const result = await createFederationToken(
            token,
            label.trim(),
            scopes(),
        );
        setCreating(false);
        if (!result.ok) {
            context.toast.open(result.exception?.detail || t("app.error"), { variant: "error" });
            return;
        }
        setCreated(result.item);
        setLabel("");
        setSharesDevices(false);
        if (onCreated && result.item) {
            const { token: _secret, ...row } = result.item;
            onCreated(row);
        }
    };

    if (invited) {
        return (
            <InvitationReveal
                link={invitationLink(invited.code)}
                expiresAt={invited.expires_at}
                title={t("invitations.federationTitle", { label: invited.label })}
                onDone={() => setInvited(null)}
                t={t}
                context={context} />
        );
    }

    if (created) {
        return (
            <NewTokenReveal
                created={created}
                onDone={() => setCreated(null)}
                t={t}
                context={context} />
        );
    }

    return (
        <SectionPanel
            label={t("admin.federation.createTitle")}
            helpText={t("admin.federation.createHelp")}>
            <Form.Group
                label={t("admin.federation.nameLabel")}
                helpText={t("admin.federation.nameHelp")}>
                <Form.Input
                    name="label"
                    context={contextForm}
                    props={{ placeholder: t("admin.federation.namePlaceholder") }} />
            </Form.Group>
            <Form.Group>
                <Form.SkeletonSwitch
                    name="devices"
                    reverse={true}
                    context={contextForm}
                    label={t("admin.federation.devicesLabel")}
                    helpText={t("admin.federation.devicesHelp")} />
            </Form.Group>
            <FormButton
                callback={invite}
                label={t("invitations.create")}
                isDisabled={isCreating || label.trim() === ""}
                isPending={isCreating} />
            <div className="text-end">
                <bs.Button
                    variant="link"
                    className="px-0"
                    disabled={isCreating || label.trim() === ""}
                    onClick={create}>
                    {t("invitations.tokenInstead")}
                </bs.Button>
            </div>
        </SectionPanel>
    );
}

/**
 * LES INVITATIONS EN ATTENTE (chantier 33) — pour qui, jusqu'à quand, et la
 * révocation. Une invitation échangée devient un jeton de la liste d'en
 * dessous ; échue, elle disparaît.
 */
function PendingInvitations({ items, onRevoke, t, context }) {
    if (items.length === 0) {
        return null;
    }
    const what = (item) => (item.kind === "account"
        ? t("invitations.kindAccount")
        : String(item.scopes || "").split(",").filter(Boolean)
            .map((scope) => t(`admin.federation.scope.${scope}`, { defaultValue: scope })).join(" · "));

    return (
        <SectionPanel label={t("invitations.pendingTitle")} helpText={t("invitations.pendingHelp")}>
            {items.map((item) => (
                <ListElement
                    key={item.id}
                    icon="bi bi-qr-code"
                    label={item.label}
                    helpText={`${what(item)} · ${t("invitations.until", { at: new Date(item.expires_at).toLocaleString() })}`}
                    onClick={null}
                    next={(
                        <bs.Button
                            variant="danger"
                            title={t("admin.federation.revoke")}
                            onClick={(e) => {
                                e.preventDefault();
                                context.modal.open(
                                    <div className="text-center">
                                        {t("invitations.revokeConfirm")} <br />
                                        <strong>{item.label}</strong>
                                    </div>,
                                    {
                                        variant: "confirm",
                                        onConfirm: () => onRevoke(item),
                                        confirmLabel: t("app.delete"),
                                        cancelLabel: t("app.cancel"),
                                    },
                                );
                            }}>
                            <i className="bi bi-trash"></i>
                        </bs.Button>
                    )} />
            ))}
        </SectionPanel>
    );
}

export default function Federation({ context, token, t }) {
    const [values, setValues] = useState({ q: "" });
    const [items, setItems] = useState([]);
    const [invitations, setInvitations] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [page, setPage] = useState(1);

    // Les jetons émis par CE serveur. Le secret n'y est pas — la base n'en
    // garde que l'empreinte — donc cette liste ne sert qu'à deux choses :
    // voir à qui on a ouvert, et refermer.
    const load = useCallback(() => {
        setLoading(true);

        return Promise.all([listFederationTokens(token), listInvitations(token)]).then(([result, pending]) => {
            setItems(result.ok ? (result.items || []) : []);
            setInvitations(pending.ok ? (pending.items || []) : []);
        }).catch((failure) => {
            console.error("Chargement des jetons de fédération", failure);
        }).finally(() => {
            setLoading(false);
        });
    }, [token]);

    useEffect(() => {
        load();
    }, [load]);

    // UN GESTE SUR UNE LIGNE LA CHANGE SUR PLACE (#295) : révoquer la retire,
    // les consoles changent son interrupteur — la liste ne se relit pas, et
    // ne revient pas en haut.
    const handleDelete = async (item) => {
        const result = await revokeFederationToken(token, item.id);
        context.toast.open(
            result.ok ? t("admin.federation.revoked", { name: item.label }) : t("app.error"),
            { variant: result.ok ? "success" : "error" },
        );
        if (result.ok) {
            setItems((current) => current.filter((other) => other.id !== item.id));
        }
    };

    const handleRevokeInvitation = async (item) => {
        const result = await revokeInvitation(token, item.id);
        context.toast.open(
            result.ok ? t("invitations.revoked", { name: item.label }) : t("app.error"),
            { variant: result.ok ? "success" : "error" },
        );
        if (result.ok) {
            setInvitations((current) => current.filter((other) => other.id !== item.id));
        }
    };

    const filtered = useMemo(() => {
        const needle = values.q.trim().toLowerCase();

        return needle
            ? items.filter((item) => String(item.label || "").toLowerCase().includes(needle))
            : items;
    }, [items, values.q]);

    const visible = useMemo(
        () => filtered.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE),
        [filtered, page],
    );

    // « Jamais utilisé » plutôt qu'une date vide : un jeton créé et jamais
    // employé est une information, pas une case à remplir.
    const usedAt = (item) => (item.last_used_at
        ? t("admin.federation.lastUsed", { at: new Date(item.last_used_at).toLocaleString() })
        : t("admin.federation.neverUsed"));

    // CE QU'UN JETON OUVRE SE LIT DANS LA LISTE, en toutes lettres.
    //
    // Sans ça, deux lignes identiques à l'écran accordaient des choses
    // différentes, et rien ne permettait de savoir laquelle — pas même en
    // l'ouvrant, puisqu'un jeton ne se réaffiche jamais.
    const grants = (item) => String(item.scopes || "")
        .split(",")
        .map((scope) => scope.trim())
        .filter(Boolean)
        .map((scope) => t(`admin.federation.scope.${scope}`, { defaultValue: scope }))
        .join(" · ");

    const sharesDevices = (item) => String(item.scopes || "")
        .split(",")
        .some((scope) => scope.trim() === "devices");

    // Élargir ou refermer un accès existant, sans le réémettre — c'est ce qui
    // évite d'aller recoller un jeton sur chaque appareil qui l'avait.
    const setDevices = async (item, share) => {
        const result = await updateFederationTokenScopes(token, item.id, share ? ["catalog", "devices"] : ["catalog"]);
        if (!result.ok) {
            context.toast.open(result.exception?.detail || t("app.error"), { variant: "error" });

            return;
        }
        context.toast.open(
            t(share ? "admin.federation.devicesShared" : "admin.federation.devicesUnshared", { label: item.label }),
            { variant: "success" },
        );
        const scopes = result.item?.scopes || (share ? "catalog,devices" : "catalog");
        setItems((current) => current.map((other) => (other.id === item.id ? { ...other, scopes } : other)));
    };

    // REFERMER SE CONFIRME, comme révoquer. Le bouton n'a qu'une icône et une
    // infobulle, qu'un téléphone n'affiche jamais : un appui de trop retirait
    // les consoles au serveur d'en face sans un mot — sa liste se vidait, et
    // avec elle l'écran pour s'y connecter et lancer (vu le 2026-10-09 entre
    // deux de nos serveurs). Ouvrir, lui, ne retire rien à personne.
    const toggleDevices = (item) => {
        if (!sharesDevices(item)) {
            setDevices(item, true);

            return;
        }
        context.modal.open(
            <div className="text-center">
                {t("admin.federation.devicesUnshareConfirm")} <br />
                <strong>{item.label}</strong>
            </div>,
            {
                variant: "confirm",
                onConfirm: () => setDevices(item, false),
                confirmLabel: t("admin.federation.devicesUnshare"),
                cancelLabel: t("app.cancel"),
            }
        );
    };

    const searchContext = {
        values: values,
        setValues: setValues,
    };

    return (
        <>
            <ListElement
                light={true}
                onClick={() => {
                    // SON PROPRE TITRE. Ce panneau CRÉE un jeton ; il
                    // s'intitulait « Ajouter un serveur »
                    // (`admin.servers.addTitle`), c'est-à-dire le nom du geste
                    // d'en face. Exactement la confusion que la réunion des
                    // deux écrans supprime — autant ne pas la déplacer avec.
                    //
                    // EN `//` ET NON EN `{/* */}` : ici on est dans la LISTE
                    // D'ARGUMENTS de `offcanvas.open()`, pas en position
                    // d'enfant JSX. Un `{` y ouvre un objet littéral, et Babel
                    // rend « Unexpected token, expected "," ». Cassé deux fois
                    // le 2026-09-14, par le même geste.
                    context.offcanvas.open(
                        <OffCanvas
                            icon="bi bi-unlock"
                            label={t("admin.federation.createTitle")}
                            helpText={t("admin.federation.createHelp")}>
                            <CreateTokenForm
                                context={context}
                                token={token}
                                t={t}
                                onCreated={(created) => setItems((current) => [created, ...current])}
                                onInvited={(invited) => setInvitations((current) => [invited, ...current])} />
                        </OffCanvas>, {
                        size: 600,
                    });
                }}
                icon="bi bi-plus"
                label={t("admin.federation.generate")}
                helpText={t("admin.federation.generateHelp")}
            />
            {/* L'ENCADRÉ « les deux moitiés d'un même échange » A DÉMÉNAGÉ
                (2026-09-14). Il expliquait qu'un jeton créé ici se colle dans
                l'autre écran — utile quand les deux étaient séparés, redondant
                maintenant que ce composant est l'onglet « Jetons » d'un écran
                qui porte déjà cette phrase une fois, en tête
                (`admin.federations.pairing`, voir Federations.jsx). Deux fois
                la même explication dans un seul panneau se lit comme deux
                règles différentes. */}
            <PendingInvitations
                items={invitations}
                onRevoke={handleRevokeInvitation}
                t={t}
                context={context} />
            <SectionPanel>
                <ListingElement
                    callback={(item) => (
                        <ListElement
                            key={item.id}
                            icon="bi bi-key"
                            label={item.label}
                            helpText={`${grants(item)} · ${usedAt(item)}`}
                            onClick={null}
                            next={(<div className="d-flex gap-2">
                                <bs.Button
                                    variant={sharesDevices(item) ? "primary" : "secondary"}
                                    title={t("admin.federation.devicesLabel")}
                                    onClick={(e) => {
                                        e.preventDefault();
                                        toggleDevices(item);
                                    }}
                                >
                                    <i className="bi bi-controller"></i>
                                </bs.Button>
                                <bs.Button
                                    variant="danger"
                                    title={t("admin.federation.revoke")}
                                    onClick={(e) => {
                                        e.preventDefault();
                                        context.modal.open(
                                            <div className="text-center">
                                                {t("admin.federation.revokeConfirm")} <br />
                                                <strong>{item.label}</strong>
                                            </div>,
                                            {
                                                variant: "confirm",
                                                onConfirm: () => handleDelete(item),
                                                confirmLabel: t("app.delete"),
                                                cancelLabel: t("app.cancel"),
                                            }
                                        );
                                    }}
                                >
                                    <i className="bi bi-trash"></i>
                                </bs.Button>
                            </div>)}
                        />
                    )}
                    items={visible}
                    itemCount={filtered.length}
                    itemsPerPage={ITEMS_PER_PAGE}
                    currentPage={page}
                    onPositionChange={setPage}
                    isLoading={isLoading}
                    search={{
                        context: searchContext,
                        placeholder: t("admin.federation.search"),
                        filters: null,
                    }}
                    texts={{
                        itemsFound: t("admin.federation.found"),
                        noItemsFound: t("admin.federation.empty"),
                    }}

                />
            </SectionPanel>
        </>
    );
}