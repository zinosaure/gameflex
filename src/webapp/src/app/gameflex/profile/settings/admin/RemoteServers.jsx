import React, { useState, useEffect, useMemo, useCallback, useRef } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../skeleton/skeleton.js";
import {
    SectionPanel,
    ListElement,
    ListingElement,
    OffCanvas,
} from "../../../shared/templates/Essentials.jsx";
import ResourceContents from "./ResourceContents.jsx";
import {
    listRemoteMounts,
    addRemoteMount,
    removeRemoteMount,
    checkRemoteMount,
    rescanRemoteMount,
} from "../../api.js";

const Form = Skeleton.Form;
const ITEMS_PER_PAGE = 10;

// DU STOCKAGE DISTANT, ET RIEN D'AUTRE — depuis le 2026-09-14.
//
// Cet écran gérait quatre types dans une seule liste, dont `gameflex` : un
// serveur fédéré s'y ajoutait à côté d'un partage WebDAV. Les deux passaient
// pourtant par des API différentes (`addFederationServer` contre
// `addRemoteMount`), et surtout le geste « ajouter un serveur GameFlex » est
// l'autre bout de « émettre un jeton », qui vivait dans un troisième écran.
//
// Les deux moitiés de la fédération sont donc réunies dans `Federations.jsx`
// (onglets Serveurs/Jetons), et il ne reste ici que ce dont c'était vraiment
// le sujet : les dossiers qu'on monte. La couture existait déjà — seul
// `kind === "gameflex"` appelait la fédération —, ce fichier ne fait que
// cesser de l'enjamber.
//
// L'encadré « le lien et le jeton sont fabriqués par l'autre personne » a
// disparu avec : il ne servait qu'à désigner l'écran d'en face, et les deux
// bouts se voient maintenant d'un seul endroit.

// Une ligne de la liste, depuis ce que le registre rend — la liste entière,
// ou le montage tout juste ajouté (sur place, #295).
const mountRow = (item) => ({
    key: `mnt-${item.id}`,
    id: item.id,
    kind: item.kind,
    label: item.name,
    // Le login, l'adresse ET LE DOSSIER : c'est le dossier qu'on parcourt, et
    // sans lui la ligne ne disait pas ce qui est réellement synchronisé
    // (2026-09-02). Le mot de passe, lui, ne ressort jamais. Un partage SMB
    // ouvert n'a pas de login : pas de « · » orphelin en tête de ligne.
    detail: [item.username, item.base_url].filter(Boolean).join(" · ")
        + (item.folder ? ` / ${item.folder}` : ""),
});

// Un montage vierge. Une fonction et non une constante partagée : deux
// ouvertures du formulaire ne doivent pas se passer le même objet.
const emptyMount = () => ({
    type: "webdav",
    name: "",
    address: "",
    url: "",
    driveId: "",
    share: "",
    folder: "",
    port: 445,
    anonymous: true,
    username: "",
    password: "",
});

// Les types enregistrables, et sous quel libellé ils s'affichent. Le type
// rangé est `smb` — le protocole —, le libellé « Samba », le nom sous lequel
// on le connaît.
const MOUNT_LABELS = { webdav: "WebDAV", kdrive: "kDrive", smb: "Samba" };

/**
 * CE QU'ON A VU, en toutes lettres.
 *
 * Les NOMS des dossiers et non leur nombre : « 3 dossiers trouvés » ne dit
 * pas si on regarde au bon endroit, alors que « bios, cores, roms » le dit
 * d'un coup d'œil — et « Photos, Documents » dit tout aussi vite qu'on s'est
 * trompé (signalé le 2026-09-02 : la sonde disait « 3 dossiers » pendant
 * qu'aucun jeu n'apparaissait, et rien ne permettait de trancher).
 */
function describeContents(seen, t) {
    const folders = seen.folders || [];
    const files = seen.files || 0;

    if (folders.length === 0) {
        return t("admin.servers.checkFilesOnly", { files });
    }

    // Quatre suffisent à reconnaître un dossier ; au-delà on compte.
    const shown = folders.slice(0, 4).join(", ");

    return folders.length > 4
        ? t("admin.servers.checkFoldersMore", { shown, rest: folders.length - 4, files })
        : t("admin.servers.checkFolders", { shown, files });
}


/**
 * L'ÉTAT DU FORMULAIRE VIT ICI, et le parent le LIT par un ref.
 *
 * Les deux contraintes se contredisent en apparence : le bouton
 * « enregistrer » appartient au panneau, donc au parent, qui doit pouvoir
 * lire la saisie ; et le formulaire doit se redessiner à chaque frappe.
 *
 * Remonter l'état chez le parent règle la première et casse la seconde :
 * `context.offcanvas.open(<OffCanvas>…)` reçoit un élément React construit
 * UNE fois, dont les props sont un instantané figé. Le parent a beau se
 * redessiner, le panneau ouvert continue d'afficher les valeurs d'origine —
 * on cliquait « Samba » et le choix ne bougeait pas (signalé le 2026-09-01,
 * régression de ma remontée d'état). C'est le même piège que la fiche d'un
 * jeu documente de son côté.
 *
 * Donc : l'état reste local — un composant se redessine toujours sur son
 * PROPRE setState — et chaque changement est recopié dans le ref que le
 * parent tient. Même patron que le formRef de l'écran des membres.
 */
function RemoteServerForm({ t, valuesRef }) {
    const [values, setValues] = useState(valuesRef.current);
    const formContext = {
        values,
        setValues: (next) => {
            valuesRef.current = next;
            setValues(next);
        },
    };
    return (
        <div>
            <Form.Group
                label={t("admin.servers.nameLabel")}
                helpText={t("admin.servers.nameHelp")}
            >
                <Form.Input
                    name="name"
                    context={formContext}
                    props={{ placeholder: t("admin.servers.namePlaceholder") }}
                />
            </Form.Group>
            <hr />
            <Form.Group
                label={t("admin.servers.typeLabel")}
                helpText={t("admin.servers.typeHelp")}
            >
                <Form.Radio
                    name="type"
                    context={formContext}
                    options={[
                        { value: "smb", label: "Samba" },
                        { value: "webdav", label: "WebDAV" },
                        { value: "kdrive", label: "kDrive" },
                    ]}
                />
            </Form.Group>
            {values.type === "smb" && (
                <>
                    <Form.Group
                        label={t("admin.servers.hostLabel")}
                        helpText={t("admin.servers.hostHelp")}
                    >
                        <div className="d-flex gap-2">
                            <div className="flex-grow-1">
                                <Form.Input
                                    name="address"
                                    context={formContext}
                                    props={{
                                        placeholder: t("admin.servers.hostLabel")
                                    }}
                                />
                            </div>
                            <Form.IntFloat
                                name="port"
                                context={formContext}
                                props={{ placeholder: t("admin.servers.portLabel") }}
                                min={1}
                                max={65535}
                            />
                        </div>
                    </Form.Group>
                    <Form.Group
                        label={t("admin.servers.shareNameLabel")}
                        helpText={t("admin.servers.shareNameHelp")}
                    >
                        <Form.Input
                            name="share"
                            context={formContext}
                            props={{ placeholder: "roms" }}
                        />
                    </Form.Group>
                    <Form.Group
                        label={t("admin.servers.folderLabel")}
                        helpText={t("admin.servers.folderShareHelp")}
                    >
                        <Form.Input
                            name="folder"
                            context={formContext}
                            props={{ placeholder: "gameflex" }}
                        />
                    </Form.Group>
                    <Form.Group>
                        <Form.SkeletonSwitch
                            reverse={true}
                            name="anonymous"
                            context={formContext}
                            label={t("admin.servers.anonymousLabel")}
                            helpText={t("admin.servers.anonymousHelp")}
                        />
                    </Form.Group>
                    {/* L'INTERRUPTEUR NE COMMANDAIT RIEN. Il annonce « cochez si
                        le partage ne demande ni nom d'utilisateur ni mot de
                        passe » — et le décocher ne révélait aucun champ, parce
                        qu'il n'y en avait aucun dans cette branche. Signalé par
                        l'utilisateur le 2026-09-14, deux fois : la première
                        réponse a été de cadrer le backend manquant, ce qui
                        laissait le défaut visible entier.
                        Anonyme, les deux restent vides et le serveur se
                        présente en invité (voir storage/smb).
                        `!values.anonymous` sans inversion : `SkeletonSwitch` lit
                        `context.values[name]` tel quel, et son `reverse` ne
                        concerne que la mise en page (Form.jsx:1734, 1744). */}
                    {!values.anonymous && (
                        <>
                            <Form.Group
                                label={t("admin.servers.usernameLabel")}
                                helpText={t("admin.servers.usernameHelp")}
                            >
                                <Form.Input
                                    name="username"
                                    context={formContext}
                                    props={{ placeholder: t("admin.servers.usernameLabel") }}
                                />
                            </Form.Group>
                            <Form.Group
                                label={t("admin.servers.passwordLabel")}
                                helpText={t("admin.servers.passwordHelp")}
                            >
                                <Form.Input
                                    name="password"
                                    context={formContext}
                                    props={{ placeholder: t("admin.servers.passwordLabel") }}
                                    type="password"
                                />
                            </Form.Group>
                        </>
                    )}
                </>
            )}
            {values.type === "webdav" && (
                <>
                    <Form.Group
                        label={t("admin.servers.shareLabel")}
                        helpText={t("admin.servers.shareHelp")}
                    >
                        <Form.Input
                            name="url"
                            context={formContext}
                            props={{ placeholder: "https://exemple.net/remote.php/dav/files/marc" }}
                        />
                    </Form.Group>
                    <Form.Group
                        label={t("admin.servers.folderLabel")}
                        helpText={t("admin.servers.folderShareHelp")}
                    >
                        <Form.Input
                            name="folder"
                            context={formContext}
                            props={{ placeholder: "gameflex" }}
                        />
                    </Form.Group>
                    <Form.Group
                        label={t("admin.servers.usernameLabel")}
                        helpText={t("admin.servers.usernameHelp")}
                    >
                        <Form.Input
                            name="username"
                            context={formContext}
                            props={{ placeholder: t("admin.servers.usernameLabel") }}
                        />
                    </Form.Group>
                    <Form.Group
                        label={t("admin.servers.passwordLabel")}
                        helpText={t("admin.servers.passwordHelp")}
                    >
                        <Form.Input
                            name="password"
                            context={formContext}
                            props={{ placeholder: t("admin.servers.passwordLabel") }}
                            type="password"
                        />
                    </Form.Group>
                </>
            )}
            {values.type === "kdrive" && (
                <>
                    {/* kDrive passe par son API, PAS par WebDAV : il ne
                        l'ouvre qu'aux offres Team et Pro, et sur les autres le
                        drive refuse tout sans même demander qui vous êtes
                        (constaté le 2026-09-02). L'API, elle, s'authentifie
                        par jeton — il n'y a donc ni login ni mot de passe
                        ici. */}
                    <Form.Group
                        label={t("admin.servers.driveIdLabel")}
                        helpText={t("admin.servers.driveIdHelp")}
                    >
                        <Form.Input
                            name="driveId"
                            context={formContext}
                            props={{ placeholder: "123456" }}
                        />
                    </Form.Group>
                    <Form.Group
                        label={t("admin.servers.folderLabel")}
                        helpText={t("admin.servers.folderDriveHelp")}
                    >
                        <Form.Input
                            name="folder"
                            context={formContext}
                            props={{ placeholder: "gameflex" }}
                        />
                    </Form.Group>
                    <Form.Group
                        label={t("admin.servers.apiTokenLabel")}
                        helpText={t("admin.servers.apiTokenHelp")}
                    >
                        <Form.Input
                            name="password"
                            context={formContext}
                            props={{ placeholder: t("admin.servers.apiTokenLabel") }}
                            type="password"
                        />
                    </Form.Group>
                </>
            )}
        </div>
    );
}


export default function RemoteServers({ context, token, t }) {
    const [values, setValues] = useState({ q: "", protocols: [] });
    const [items, setItems] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [page, setPage] = useState(1);
    const [isPending, setPending] = useState(false);

    // La saisie du formulaire, LUE par le bouton de ce panneau.
    //
    // Un ref et non un état : le panneau ouvert ne se redessine pas quand le
    // parent change (voir RemoteServerForm), donc en faire un état ne
    // rafraîchirait rien et provoquerait des rendus pour rien.
    const formRef = useRef(emptyMount());

    /**
     * Les partages montés. UNE source depuis le 2026-09-14 : les serveurs
     * fédérés se listent dans « Fédérations », avec les jetons qui les
     * accompagnent. Cette fonction en interrogeait deux et les fusionnait.
     */
    const load = useCallback(() => {
        setLoading(true);

        return listRemoteMounts(token).then((mounts) => {
            setItems((mounts.ok ? mounts.items || [] : []).map(mountRow));
        }).catch((failure) => {
            console.error("Chargement des ressources distantes", failure);
        }).finally(() => {
            setLoading(false);
        });
    }, [token]);

    useEffect(() => {
        load();
    }, [load]);

    /**
     * Enregistrer un partage.
     *
     * On SONDE avant d'écrire : un partage s'enregistrait sans que rien ne
     * l'ait jamais lu, et il fallait attendre la bibliothèque pour découvrir
     * qu'il ne rendait rien, sans savoir si c'était l'adresse, le mot de passe
     * ou le dossier (signalé le 2026-09-02).
     */
    const handleSave = async () => {
        const form = formRef.current;
        // Anonyme = ni compte ni mot de passe, même si les champs, masqués,
        // gardent ce qu'on y avait tapé avant de recocher l'interrupteur.
        const anonymous = form.type === "smb" && form.anonymous;
        const username = anonymous ? "" : form.username.trim();
        const password = anonymous ? "" : form.password;

        setPending(true);
        try {
            const candidate = {
                kind: form.type,
                base_url: form.url.trim(),
                drive_id: form.driveId.trim(),
                host: form.address.trim(),
                port: Number(form.port) || 0,
                share: form.share.trim(),
                folder: form.folder.trim(),
                username,
                password,
            };

            const probe = await checkRemoteMount(token, candidate);
            const seen = probe.item || probe.data || {};
            if (!probe.ok || !seen.reachable) {
                context.toast.open(
                    seen.detail || probe.exception?.detail || t("admin.servers.unreachable"),
                    { variant: "error" },
                );

                return;
            }

            const result = await addRemoteMount(token, {
                name: form.name.trim(),
                kind: form.type,
                baseUrl: candidate.base_url,
                driveId: candidate.drive_id,
                host: candidate.host,
                port: candidate.port,
                share: candidate.share,
                folder: candidate.folder,
                username,
                password,
            });
            if (!result.ok) {
                // Le détail du serveur PLUTÔT qu'un message générique :
                // « il manque un mot de passe » et « ce nom est déjà
                // pris » ne se corrigent pas de la même façon.
                context.toast.open(
                    result.exception?.detail || t("app.error"), { variant: "error" },
                );

                return;
            }

            // CE QU'ON A VU, pas juste « ajouté » : LES NOMS, pas le compte.
            // « 3 dossiers trouvés » ne dit pas si on est au bon endroit ;
            // « bios, cores, roms » le dit d'un coup d'œil — et « Photos,
            // Documents » dit tout aussi vite qu'on s'est trompé de dossier
            // (2026-09-02).
            // LE PARCOURS PART À L'AJOUT (chantier 15) : ses jeux sont listés
            // en fond, au format de tous les listings, avant qu'on ouvre la
            // ressource. Un échec ici n'annule pas l'ajout — l'icône le relance.
            if (result.item?.id) {
                rescanRemoteMount(token, result.item.id);
            }

            context.toast.open(describeContents(seen, t), { variant: "success" });
            context.offcanvas.close();
            formRef.current = emptyMount();
            // Sa ligne entre dans la liste SUR PLACE (#295) : la liste ne se
            // relit pas. À sa place — le registre les rend triés par nom.
            if (result.item) {
                setItems((current) => [...current, mountRow(result.item)]
                    .sort((a, b) => (a.label < b.label ? -1 : (a.label > b.label ? 1 : 0))));
            }
        } finally {
            setPending(false);
        }
    };

    const handleDelete = async (item) => {
        const result = await removeRemoteMount(token, item.id);
        context.toast.open(
            result.ok ? t("admin.servers.removed", { name: item.label }) : t("app.error"),
            { variant: result.ok ? "success" : "error" },
        );
        if (result.ok) {
            setItems((current) => current.filter((other) => other.id !== item.id));
        }
    };

    const searchContext = { values, setValues };

    // Le filtre porte sur quelque chose : deux protocoles peuvent coexister
    // dans cette liste. Aucun coché veut dire « tous » — c'est ce qu'on
    // attend d'un filtre qu'on n'a pas encore touché, et ça évite une liste
    // vide au premier affichage.
    const filtered = useMemo(() => {
        const needle = values.q.trim().toLowerCase();
        const kinds = values.protocols || [];

        return items.filter((item) => (!needle
            || `${item.label} ${item.detail}`.toLowerCase().includes(needle))
            && (kinds.length === 0 || kinds.includes(item.kind)));
    }, [items, values.q, values.protocols]);

    const visible = useMemo(
        () => filtered.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE),
        [filtered, page],
    );

    return (
        <>
            <ListElement
                light={true}
                onClick={() => context.offcanvas.open(
                    <OffCanvas
                        icon="bi bi-hdd-network"
                        label={t("admin.servers.addTitle")}
                        helpText={t("admin.servers.addHelp")}
                        callback={handleSave}
                        isDisabled={isPending}
                        isPending={isPending}
                        buttonLabel={t("app.save")}>
                        <RemoteServerForm
                            t={t}
                            valuesRef={formRef}
                        />
                    </OffCanvas>, {
                    size: 600,
                })}
                icon="bi bi-plus"
                label={t("admin.servers.addTitle")}
                helpText={t("admin.servers.addHelp")}
            />
            <SectionPanel>
                <ListingElement
                    callback={(item) => (
                        <ListElement
                            key={item.key}
                            // CLIQUER UNE RESSOURCE OUVRE SON CONTENU.
                            //
                            // La ligne ne menait nulle part : on pouvait
                            // ajouter et supprimer, jamais regarder. Or c'est
                            // la question qu'on se pose le plus souvent
                            // devant un partage — qu'est-ce qu'il y a dedans
                            // (demandé le 2026-09-02).
                            onClick={() => context.offcanvas.open(
                                <OffCanvas
                                    icon="bi bi-hdd-network"
                                    label={item.label}
                                    helpText={item.detail}>
                                    <ResourceContents
                                        item={item}
                                        token={token}
                                        context={context}
                                        t={t} />
                                </OffCanvas>, { size: 600 },
                            )}
                            icon="bi bi-hdd-network"
                            label={item.label}
                            helpText={`${MOUNT_LABELS[item.kind] || item.kind} ${item.detail}`}
                            next={(

                                <bs.Button
                                    variant="danger"
                                    onClick={(e) => {
                                        // La corbeille est DANS la ligne, qui
                                        // ouvre le contenu au clic : sans
                                        // stopPropagation, supprimer ouvrait
                                        // aussi le panneau du contenu, par-
                                        // dessus la confirmation (vu le
                                        // 2026-09-15 en testant SMB).
                                        e.preventDefault();
                                        e.stopPropagation();
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
                                            }
                                        );
                                    }}
                                >
                                    <i className="bi bi-trash"></i>
                                </bs.Button>
                            )}
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
                        placeholder: t("admin.servers.search"),
                        filters: {
                            Component: () => (
                                <div>
                                    <Form.Group
                                        label={t("admin.servers.filterTypeLabel")}
                                        helpText={t("admin.servers.filterTypeHelp")}
                                    >
                                        <Form.Multicheckbox
                                            name="protocols"
                                            context={searchContext}
                                            options={[
                                                { value: "smb", label: "Samba" },
                                                { value: "webdav", label: "WebDAV" },
                                                { value: "kdrive", label: "kDrive" },
                                            ]} />
                                    </Form.Group>
                                </div>
                            )
                        }
                    }}
                    texts={{
                        itemsFound: t("admin.servers.found"),
                        noItemsFound: t("admin.servers.empty"),
                    }}

                />
            </SectionPanel>
        </>
    );
}
