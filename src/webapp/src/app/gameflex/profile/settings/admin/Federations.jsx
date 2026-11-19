import React, { useState, useEffect, useMemo, useCallback, useRef } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import {
    SectionPanel,
    ListElement,
    ListingElement,
    OffCanvas,
} from "../../../shared/templates/Essentials.jsx";
import Federation from "./Federation.jsx";
import { can } from "../../../libs/capabilities.js";
import { federatedServerNow, federatedServers } from "../../../libs/federatedHost.js";
import {
    checkFederationServer,
    listFederationServers,
    addFederationServer,
    removeFederationServer,
} from "../../api.js";
import { refreshSource } from "./api/storage-admin.js";
import { InvitationField } from "../../../shared/Invitation.jsx";
import { redeemServerInvitation } from "../../../api/invitations.js";

const Form = Skeleton.Form;
const ITEMS_PER_PAGE = 10;

// LES DEUX BOUTS D'UN MÊME ÉCHANGE, DANS UN SEUL ÉCRAN.
//
// Ils vivaient dans deux entrées de réglages séparées, et aucune ne se
// suffisait : « Fédérer le serveur » ÉMET un jeton qui ouvre nos jeux, et la
// moitié GameFlex de « Ressources distantes » ENREGISTRE le serveur d'en face
// avec le jeton qu'il nous a donné. Le texte l'avouait lui-même — le panneau
// de création finissait par « transmettez ces deux valeurs à la personne :
// elle les collera dans *Ajouter un serveur* », et chaque écran portait un
// encadré dont le seul rôle était de désigner l'autre. Deux encadrés pour
// rattraper un découpage, c'est le découpage qui était faux.
//
// Ce qui NE déménage pas : WebDAV, kDrive et Samba restent dans « Ressources
// distantes ». Ce sont des montages de stockage, servis par une autre API
// (`addRemoteMount`), et la couture était déjà là — seul `kind === "gameflex"`
// passait par la fédération.
//
// La forme suit « Serveur TURN » (TurnServer.jsx) : des onglets encadrés dans
// un seul panneau, parce que ce sont deux vues du même sujet et non deux
// sujets.

// Un serveur vierge. Une fonction et non une constante : deux ouvertures du
// formulaire ne doivent pas se passer le même objet (même raison que
// `emptyMount` côté ressources distantes).
const emptyServer = () => ({ mode: "invitation", invitation: "", name: "", url: "", token: "" });

/**
 * L'ÉTAT DU FORMULAIRE VIT ICI, et le parent le LIT par un ref.
 *
 * Le bouton « enregistrer » appartient à l'OffCanvas, donc au parent, qui doit
 * pouvoir lire la saisie ; et le formulaire doit se redessiner à chaque
 * frappe. Remonter l'état chez le parent règle la première et casse la
 * seconde : `context.offcanvas.open(<OffCanvas>…)` reçoit un élément construit
 * UNE fois, dont les props sont un instantané figé — le panneau ouvert
 * continuerait d'afficher les valeurs d'origine. Même patron que
 * `RemoteServerForm` et que le formulaire des membres.
 */
function AddServerForm({ t, valuesRef, context }) {
    const [values, setValues] = useState(valuesRef.current);
    const formContext = {
        values,
        setValues: (next) => {
            valuesRef.current = next;
            setValues(next);
        },
    };

    // L'INVITATION D'ABORD (chantier 33) : un lien, ou l'image de son QR, et
    // le jeton naît chez lui sans passer par une messagerie. Le lien et le
    // jeton d'avant restent, dans le second onglet.
    return (
        <bs.Tabs
            activeKey={values.mode}
            onSelect={(mode) => formContext.setValues({ ...values, mode })}
            id="add-server-tabs"
            className="nav-tabs-boxed mb-3"
            justify>
            <bs.Tab eventKey="invitation" title={t("invitations.tab")}>
                <InvitationField
                    formContext={formContext}
                    label={t("invitations.fieldLabel")}
                    helpText={t("invitations.serverFieldHelp")}
                    t={t}
                    context={context} />
                <Form.Group
                    label={t("admin.servers.nameLabel")}
                    helpText={t("invitations.nameHelp")}>
                    <Form.Input
                        name="name"
                        context={formContext}
                        props={{ placeholder: t("admin.servers.namePlaceholder") }} />
                </Form.Group>
            </bs.Tab>
            <bs.Tab eventKey="token" title={t("invitations.tokenTab")}>
                <LinkAndTokenFields t={t} formContext={formContext} />
            </bs.Tab>
        </bs.Tabs>
    );
}

/** Le lien et le jeton recopiés, comme avant les invitations : le repli. */
function LinkAndTokenFields({ t, formContext }) {
    return (
        <div>
            <Form.Group
                label={t("admin.servers.nameLabel")}
                helpText={t("admin.servers.nameHelp")}>
                <Form.Input
                    name="name"
                    context={formContext}
                    props={{ placeholder: t("admin.servers.namePlaceholder") }} />
            </Form.Group>
            <Form.Group
                label={t("admin.servers.urlLabel")}
                helpText={t("admin.servers.gameflexUrlHelp")}>
                <Form.Input
                    name="url"
                    context={formContext}
                    props={{ placeholder: "https://gameflex.com" }} />
            </Form.Group>
            <Form.Group
                label={t("admin.servers.tokenLabel")}
                helpText={t("admin.servers.tokenHelp")}>
                <Form.Input
                    name="token"
                    context={formContext}
                    props={{ placeholder: t("admin.servers.tokenLabel") }} />
            </Form.Group>
        </div>
    );
}

/**
 * LE PANNEAU D'AJOUT TIENT SON PROPRE « EN COURS ».
 *
 * `isPending`, passé depuis la liste, était figé comme le reste des props du
 * panneau ouvert (voir AddServerForm) : le bouton ne se grisait jamais pendant
 * la sonde, et un second appui ajoutait une seconde fois — vu sur le S25 le
 * 2026-10-21, un ajout parti deux fois en une seconde. Même patron que
 * `MemberForm`, qui tient le sien.
 */
function AddServerPanel({ t, valuesRef, onSave, context }) {
    const [isPending, setPending] = useState(false);
    const save = async () => {
        setPending(true);
        try {
            await onSave();
        } finally {
            setPending(false);
        }
    };

    return (
        <OffCanvas
            icon="bi bi-lightning-charge"
            label={t("admin.servers.addTitle")}
            helpText={t("admin.federations.addHelp")}
            callback={save}
            isDisabled={isPending}
            isPending={isPending}
            buttonLabel={t("app.save")}>
            <AddServerForm t={t} valuesRef={valuesRef} context={context} />
        </OffCanvas>
    );
}

/**
 * Ce que la copie du listing d'un serveur dit de lui (chantier 15) : sa date,
 * sa version, sa machine, et s'il nous ouvre ses consoles — d'où Play sur ses
 * jeux. Ce que le backend fournit, et rien de plus.
 */
function describeListing(listing, t) {
    if (!listing) {
        return t("admin.federations.noListing");
    }
    const gigabytes = (bytes) => Math.round(bytes / 2 ** 30);
    const parts = [t("admin.federations.listedAt", { at: new Date(listing.updatedAt).toLocaleString() })];
    if (listing.server?.version) {
        parts.push(`GameFlex ${listing.server.version}`);
    }
    const machine = listing.machine || {};
    // Un téléphone se nomme par son modèle — Android le dit, Linux non.
    if (machine.device) {
        parts.push(machine.device);
    }
    if (machine.cpu) {
        parts.push(machine.cpu);
    }
    if (machine.ramBytes) {
        parts.push(t("admin.federations.ram", { gb: gigabytes(machine.ramBytes) }));
    }
    for (const gpu of machine.gpus || []) {
        parts.push(gpu.vramBytes ? t("admin.federations.vram", { name: gpu.name, gb: gigabytes(gpu.vramBytes) }) : gpu.name);
    }
    parts.push((listing.scopes || []).includes("devices")
        ? t("admin.federations.consolesGranted")
        : t("admin.federations.consolesRefused"));

    return parts.filter(Boolean).join(" · ");
}

/**
 * L'onglet « Servers » — les GameFlex d'ailleurs qu'on a enregistrés.
 *
 * Extrait de `RemoteServers.jsx`, dont il ne reprend QUE la branche
 * `kind === "gameflex"` : une liste, un ajout sondé avant écriture, une
 * suppression. Le sondage n'est pas un ornement — pouvoir dire « serveur
 * trouvé, vous y êtes connu comme Marc » avant d'enregistrer vaut mieux
 * qu'une liste vide trois écrans plus loin.
 */
export function FederatedServers({ context, token, t }) {
    const [values, setValues] = useState({ q: "" });
    const [items, setItems] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [page, setPage] = useState(1);
    const formRef = useRef(emptyServer());

    // Une ligne : le serveur du registre, et ce que la copie de son listing dit
    // de lui.
    const rowOf = useCallback((item, listing) => ({
        id: item.id,
        label: item.hostname || item.link,
        detail: [item.link, describeListing(listing, t)].join(" · "),
    }), [t]);

    const load = useCallback(() => {
        setLoading(true);

        // LE REGISTRE, ET CE QUE LA COPIE DU LISTING DE CHACUN DIT DE LUI
        // (chantier 15) — notre kernel la garde, et le dit par `/api/v1/servers`.
        return Promise.all([listFederationServers(token), federatedServers({ fresh: true })]).then(([result, known]) => {
            const listings = new Map((known || []).map((server) => [server.id, server.listing]));
            setItems((result.ok ? result.items || [] : []).map((item) => rowOf(item, listings.get(item.id))));
        }).catch((failure) => {
            console.error("Chargement des serveurs fédérés", failure);
        }).finally(() => {
            setLoading(false);
        });
    }, [token, rowOf]);

    useEffect(() => {
        load();
    }, [load]);

    // LE SERVEUR ET SON JETON : tirés de l'invitation (échangée chez lui),
    // ou sondés comme avant à partir du lien et du jeton recopiés.
    const serverOf = async (form) => {
        if (form.mode === "invitation") {
            const redeemed = await redeemServerInvitation(form.invitation);
            if (!redeemed.ok) {
                context.toast.open(t(`common.errorCodes.${redeemed.code || "serverUnreachable"}`, {
                    server: "",
                    defaultValue: redeemed.message || t("admin.servers.unreachable"),
                }), { variant: "error" });

                return null;
            }

            return { link: redeemed.data.link, serverToken: redeemed.data.token, seen: redeemed.data };
        }
        const probe = await checkFederationServer(token, form.url.trim(), form.token.trim());
        if (!probe.ok) {
            context.toast.open(
                probe.exception?.detail || t("admin.servers.unreachable"), { variant: "error" },
            );

            return null;
        }

        return { link: form.url.trim(), serverToken: form.token.trim(), seen: probe.item || probe.data || {} };
    };

    const handleSave = async () => {
        const form = formRef.current;
        const found = await serverOf(form);
        if (!found) {
            return;
        }

        const { link, serverToken, seen } = found;
        const result = await addFederationServer(token, {
            link,
            serverToken,
            hostname: form.name.trim() || seen.hostname || link,
            identity: seen.identity || "",
        });
        if (!result.ok) {
            // Le détail du serveur plutôt qu'un message générique : « il
            // manque un jeton » et « ce serveur est déjà enregistré » ne
            // se corrigent pas de la même façon.
            context.toast.open(result.exception?.detail || t("app.error"), { variant: "error" });

            return;
        }

        // LA COPIE DE SON LISTING SE PREND À L'AJOUT (chantier 15) : ses jeux,
        // ses BIOS et ses cœurs se lisent ensuite chez nous. Un échec ici
        // n'annule pas l'ajout — la première liste ouverte la prendra.
        // Sa ligne entre dans la liste SUR PLACE (#295), avec ce que la
        // copie dit de lui — la liste ne se relit pas.
        if (result.item?.id) {
            await refreshSource(`peer:${result.item.id}`);
            const now = await federatedServerNow(result.item.id);
            setItems((current) => [...current, rowOf(result.item, now?.listing)]);
        }
        // LES AUTRES ÉCRANS L'APPRENNENT AUSSI : la liste des serveurs que la
        // page garde en mémoire (federatedServers) se relit. Elle ne se
        // relisait qu'en rouvrant cet écran, et Réglages › Jeux proposait
        // encore l'identifiant d'un serveur retiré puis rajouté — « unknown
        // server » à chaque appui (le S25, 2026-10-21).
        await federatedServers({ fresh: true });

        context.toast.open(t("admin.servers.added"), { variant: "success" });
        context.offcanvas.close();
        formRef.current = emptyServer();
    };

    const handleDelete = async (item) => {
        const result = await removeFederationServer(token, item.id);
        context.toast.open(
            result.ok ? t("admin.servers.removed", { name: item.label }) : t("app.error"),
            { variant: result.ok ? "success" : "error" },
        );
        if (result.ok) {
            setItems((current) => current.filter((other) => other.id !== item.id));
            // Les autres écrans l'oublient aussi (voir handleSave).
            await federatedServers({ fresh: true });
        }
    };

    const searchContext = { values, setValues };

    const filtered = useMemo(() => {
        const needle = values.q.trim().toLowerCase();

        return needle
            ? items.filter((item) => `${item.label} ${item.detail}`.toLowerCase().includes(needle))
            : items;
    }, [items, values.q]);

    const visible = useMemo(
        () => filtered.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE),
        [filtered, page],
    );

    return (
        <>
            <ListElement
                light={true}
                icon="bi bi-plus"
                label={t("admin.servers.addTitle")}
                helpText={t("admin.federations.addHelp")}
                onClick={() => context.offcanvas.open(
                    <AddServerPanel t={t} valuesRef={formRef} onSave={handleSave} context={context} />, { size: 600 },
                )}
            />
            <SectionPanel>
                <ListingElement
                    callback={(item) => (
                        <ListElement
                            key={item.id}
                            icon="bi bi-lightning-charge"
                            label={item.label}
                            helpText={item.detail}
                            onClick={null}
                            next={(
                                <bs.Button
                                    variant="danger"
                                    title={t("app.delete")}
                                    onClick={(e) => {
                                        e.preventDefault();
                                        context.modal.open(
                                            <div className="text-center">
                                                {t("admin.servers.removeConfirm")} <br />
                                                <strong>{item.label}</strong>
                                            </div>,
                                            {
                                                variant: "confirm",
                                                onConfirm: () => handleDelete(item),
                                                confirmLabel: t("app.delete"),
                                                cancelLabel: t("app.cancel"),
                                            },
                                        );
                                    }}>
                                    <i className="bi bi-trash"></i>
                                </bs.Button>
                            )} />
                    )}
                    items={visible}
                    itemCount={filtered.length}
                    itemsPerPage={ITEMS_PER_PAGE}
                    currentPage={page}
                    onPositionChange={setPage}
                    isLoading={isLoading}
                    // SES PROPRES TEXTES, et il a fallu une capture pour le
                    // voir : cet onglet réutilisait `admin.servers.search`
                    // /`found`/`empty`, qui appartiennent maintenant aux
                    // montages de stockage. Un onglet qui liste des serveurs
                    // fédérés annonçait donc « 0 resource(s) » et « No remote
                    // resource. » — faux, et faux d'une façon qu'aucun compte
                    // de clés n'aurait signalée.
                    search={{
                        context: searchContext,
                        placeholder: t("admin.federations.serversSearch"),
                        filters: null,
                    }}
                    texts={{
                        itemsFound: t("admin.federations.serversFound"),
                        noItemsFound: t("admin.federations.serversEmpty"),
                    }} />
            </SectionPanel>
        </>
    );
}

export default function Federations({ context, user, token, t }) {
    // UN HÔTE QUI N'ÉMET PAS DE JETONS n'a qu'un bout de l'échange : il
    // enregistre des serveurs. Pas d'onglets pour un seul contenu, et un
    // encadré qui ne parle pas d'un jeton qu'on ne peut pas créer ici.
    if (!can("issuesCatalogTokens")) {
        return (
            <>
                <bs.Alert variant="secondary" className="small mb-3">
                    <i className="bi bi-arrow-left-right me-2"></i>
                    {t("admin.federations.pairingRegisterOnly")}
                </bs.Alert>
                <SectionPanel>
                    <FederatedServers
                        context={context}
                        user={user}
                        token={token}
                        t={t} />
                </SectionPanel>
            </>
        );
    }

    return (
        <>
            {/* L'ÉCHANGE EN UNE PHRASE, UNE FOIS. Chacun des deux écrans
                portait son propre encadré pour désigner l'autre ; réunis, il
                n'en faut plus qu'un, et il explique le modèle au lieu de
                donner un itinéraire. */}
            <bs.Alert variant="secondary" className="small mb-3">
                <i className="bi bi-arrow-left-right me-2"></i>
                {t("admin.federations.pairing")}
            </bs.Alert>
            <SectionPanel>
                <bs.Tabs
                    defaultActiveKey="servers"
                    id="federations-tabs"
                    className="nav-tabs-boxed mb-3"
                    justify>
                    <bs.Tab eventKey="servers" title={t("admin.federations.serversTab")}>
                        <FederatedServers
                            context={context}
                            user={user}
                            token={token}
                            t={t} />
                    </bs.Tab>
                    <bs.Tab eventKey="tokens" title={t("admin.federations.tokensTab")}>
                        <Federation
                            context={context}
                            user={user}
                            token={token}
                            t={t} />
                    </bs.Tab>
                </bs.Tabs>
            </SectionPanel>
        </>
    );
}
