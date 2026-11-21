import React, { useContext, useEffect, useRef, useState } from "react";
import * as bs from "react-bootstrap";
import { useSearchParams } from "react-router-dom";
import Skeleton from "../../../../skeleton/skeleton.js";
import {
    listUsers,
    createUser,
    updateUser,
    deleteUser,
    updateUserToken,
    regenerateUserToken,
} from "./api/users.js";
import {
    SectionPanel,
    OffCanvas,
    ListElement,
    ListingElement,
} from "../../../shared/templates/Essentials.jsx";
import { CopyableValue } from "../../../shared/CopyableValue.jsx";
import { useConsoles, useConsolesLoaded } from "../../../contexts/ConsolesContext.jsx";
import { GRANT_ALL, GRANT_RETRO } from "../../../libs/consoleGrants.js";
import { shareOrigin } from "../../../libs/serverBase.js";

const Form = Skeleton.Form;
const Ruleset = Skeleton.Ruleset;

// Matches python/src/app/users/models.py's ROLE_ROOT/ROLE_ADMIN/ROLE_USER —
// lower is more privileged.
const ROLE_ROOT = 0;
const ROLE_ADMIN = 1;
const ROLE_USER = 2;

const ITEMS_PER_PAGE = 10;
// Live-search debounce, same value as Games.jsx.
const SEARCH_DEBOUNCE_MS = 300;
// Same floor the backend applies (users/routes.py's PASSWORD_MIN_LENGTH).
const PASSWORD_MIN = 8;

// LES DURÉES D'UN JETON, « 2 ans maximum » (le porteur, chantier 20) — les
// mêmes que le service d'identité accepte (`tokenDurations`).
const TOKEN_DURATIONS = ["1d", "1w", "1m", "3m", "6m", "1y", "2y"];

// LES CONSOLES D'UN COMPTE, telles que le formulaire les coche (chantier 20) :
// « Tout le rétro » d'un interrupteur, puis chaque console streamex et chaque
// Remote Play. `*` coche tout.
const grantsToForm = (grants, streamexIds, remoteplayIds) => {
    const granted = grants || [];
    const all = granted.includes(GRANT_ALL);

    return {
        retro: all || granted.includes(GRANT_RETRO),
        streamex: all ? streamexIds : streamexIds.filter((id) => granted.includes(id)),
        remoteplay: all ? remoteplayIds : remoteplayIds.filter((id) => granted.includes(id)),
    };
};

// Et dans l'autre sens. TOUT COCHÉ S'ENREGISTRE `*` — « tout », y compris les
// consoles qui viendront : c'est ce qu'un nouveau compte reçoit.
const formToGrants = (values, streamexIds, remoteplayIds) => {
    const everything = values.retro
        && streamexIds.every((id) => values.streamex.includes(id))
        && remoteplayIds.every((id) => values.remoteplay.includes(id));
    if (everything) {
        return [GRANT_ALL];
    }

    return [
        ...(values.retro ? [GRANT_RETRO] : []),
        ...streamexIds.filter((id) => values.streamex.includes(id)),
        ...remoteplayIds.filter((id) => values.remoteplay.includes(id)),
    ];
};

const sameGrants = (a, b) => [...(a || [])].sort().join(",") === [...(b || [])].sort().join(",");

const emptyMemberValues = () => ({
    username: "",
    display_name: "",
    role_level: String(ROLE_USER),
    password: "",
    retro: true,
    streamex: [],
    remoteplay: [],
});

// yyyy-mm-dd hh:mm — an explicit format rather than toLocaleString(), so the
// column reads the same for everyone and sorts the way it looks.
const formatDateTime = (iso) => {
    if (!iso) return "—";
    const d = new Date(iso);
    const pad = (n) => String(n).padStart(2, "0");
    return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())} ${pad(d.getHours())}:${pad(d.getMinutes())}`;
};

// Create/edit form, rendered ABOVE the table rather than in an offcanvas:
// editing is started by the pencil in a row, and seeing the row and the form
// at the same time is the whole point.
//
// Validation goes through Skeleton rulesets (same rules as
// profile/Settings.jsx, which is where a user edits their own name/password)
// instead of the hand-rolled boolean this used to carry. The backend applies
// the same limits — see AGENTS.md on never leaving a rule to the form alone.
function MemberForm({ token, member, onSaved, t }) {
    const context = useContext(Skeleton.context);
    const consoles = useConsoles();
    const consolesLoaded = useConsolesLoaded();
    const streamexConsoles = consoles.filter((c) => c.method === "streamex");
    const remoteplayConsoles = consoles.filter((c) => c.method === "remoteplay");
    const streamexIds = streamexConsoles.map((c) => c.id);
    const remoteplayIds = remoteplayConsoles.map((c) => c.id);
    const [values, setValues] = useState(emptyMemberValues);
    const [isPending, setPending] = useState(false);
    const isEditing = !!member;
    // UN ADMINISTRATEUR A TOUT, coché et verrouillé : « quand c'est un admin,
    // toutes les consoles sont checked et on ne peut pas changer » (le
    // porteur). Le service d'identité le garantit de son côté.
    const isAdminRole = values.role_level === String(ROLE_ADMIN);

    // Re-seed whenever the pencil selects a different member (or "new" is
    // reset), since this component stays mounted between the two. Les
    // consoles attendent le registre : sans lui, rien ne se coche.
    useEffect(() => {
        const grants = member ? member.consoles : [GRANT_ALL];
        setValues({
            ...(member
                ? { username: member.username, display_name: member.display_name, role_level: String(member.role_level), password: "" }
                : emptyMemberValues()),
            ...grantsToForm(grants, streamexIds, remoteplayIds),
        });
    }, [member?.id, consolesLoaded]);

    const lockedForAdmin = (data) => (data.role_level === String(ROLE_ADMIN) ? Ruleset.isDisabled(null) : null);
    const contextForm = {
        values: isAdminRole ? { ...values, retro: true, streamex: streamexIds, remoteplay: remoteplayIds } : values,
        setValues: (v) => setValues((prev) => ({ ...prev, ...v })),
        files: {},
        setFiles: () => { },
        rulesets: {
            retro: lockedForAdmin,
            streamex: lockedForAdmin,
            remoteplay: lockedForAdmin,
            username: (data) => {
                if (!data.username || data.username.trim().length < 3) {
                    return Ruleset.isInvalid(t("admin.members.usernameMinChars", { min: 3 }));
                }
                // LE MOTIF DU SERVICE D'IDENTITÉ (#601) : le nom devient un dossier,
                // et le kernel ne lance une partie que pour un tel nom.
                if (!member && !/^[A-Za-z0-9._-]{3,32}$/.test(data.username.trim())) {
                    return Ruleset.isInvalid(t("admin.members.usernameChars", { max: 32 }));
                }
                if (member && data.username.trim().length > 0) {
                    return Ruleset.isDisabled(t("admin.members.usernameImmutable"));
                }
                return null;
            },
            display_name: (data) => {
                if (!data.display_name || data.display_name.length < 3) {
                    return Ruleset.isInvalid(t("profile.displayNameMinChars", { min: 3 }));
                }
                if (data.display_name.length > 25) {
                    return Ruleset.isInvalid(t("profile.displayNameMaxChars", { max: 25 }));
                }
                return null;
            },
            password: (data) => {
                // Editing: an empty box means "leave the password alone", so
                // it's only checked once something is typed.
                if (isEditing && !data.password) return null;
                if (!data.password || data.password.length < PASSWORD_MIN) {
                    return Ruleset.isInvalid(t("profile.newPasswordMinChars", { min: PASSWORD_MIN }));
                }
                return null;
            },
        },
    };

    // Nothing to submit until something actually differs from the member
    // being edited (see AGENTS.md) — a freshly opened edit form has its Save
    // disabled. On creation everything is new, so it's dirty by definition.
    //
    // LES CONSOLES NE PARTENT QU'AVEC LE REGISTRE, et pour un joueur : sans le
    // registre, rien n'est coché, et l'enregistrer retirerait tout ; un
    // administrateur a tout quoi qu'on envoie.
    const grants = formToGrants(values, streamexIds, remoteplayIds);
    const sendsGrants = consolesLoaded && consoles.length > 0 && !isAdminRole;
    const isReplaced = !isEditing || (
        values.username !== member.username
        || values.display_name !== member.display_name
        || values.role_level !== String(member.role_level)
        || values.password !== ""
        || (sendsGrants && !sameGrants(grants, member.consoles))
    );

    const handleSave = async () => {
        setPending(true);
        // LE NOM D'UTILISATEUR N'EST ENVOYÉ QU'À LA CRÉATION.
        //
        // Il est immuable côté serveur — `handleUpdateUser` ne l'accepte même
        // pas dans son corps — et `decodeBody` REFUSE tout champ inconnu. Une
        // modification l'incluant échouait donc entièrement, avec « Invalid
        // request body. » : impossible de changer un rôle ou un mot de passe,
        // sur mobile comme sur le web (signalé le 2026-09-04).
        const payload = {
            display_name: values.display_name,
            role_level: parseInt(values.role_level, 10),
        };
        if (!isEditing) {
            payload.username = values.username.trim();
        }
        if (values.password) payload.password = values.password;
        if (sendsGrants) payload.consoles = grants;

        try {
            const result = isEditing
                ? await updateUser(token, member.id, payload)
                : await createUser(token, payload);

            if (!result.ok) {
                context.toast.open(result.exception?.detail || t("admin.members.saveFailed"), { variant: "error" });

                return;
            }

            context.toast.open(
                t(isEditing ? "admin.members.updated" : "admin.members.created", { name: values.display_name }),
                { variant: "success" }
            );
            // REFERMER LE PANNEAU — c'est tout ce qui manquait.
            //
            // La liste DERRIÈRE un panneau resté ouvert, champs encore
            // remplis, ne montrait rien : l'écran était rigoureusement le même
            // avant et après, donc « il ne se passe rien » alors que le membre
            // est bien créé (signalé le 2026-09-25). Les deux écrans frères —
            // `Federations.jsx`, `RemoteServers.jsx` — referment ; celui-ci
            // était le seul à ne pas le faire.
            //
            // LA RÉPONSE EST LE MEMBRE : sa ligne change, ou entre, SUR PLACE
            // (#295) — la liste ne se relit pas.
            context.offcanvas.close();
            onSaved(result.item, !isEditing);
        } finally {
            // Même raison que l'écran de connexion : sans ce `finally`, un
            // appel qui jette laissait `isPending` à vrai, et le bouton
            // d'enregistrement restait figé sur « ••• » pour toujours.
            setPending(false);
        }
    };

    return (
        <OffCanvas
            icon={isEditing ? "bi bi-pencil" : "bi bi-person-plus"}
            label={isEditing ? t("admin.members.editTitle", { name: member.display_name }) : t("admin.members.create")}
            helpText={isEditing ? t("admin.members.editHelp") : t("admin.members.createHelp")}
            callback={handleSave}
            isDisabled={!isReplaced || Ruleset.hasFeedbacks(contextForm)}
            isPending={isPending}
            buttonLabel={t("app.save")}>
            <SectionPanel>

                <Form.Group
                    label={t("profile.username")}
                    helpText={isEditing ? null : t("admin.members.usernameHelp")}>
                    <Form.Text
                        name="username"
                        context={contextForm} props={{ autoComplete: "off" }} />
                </Form.Group>
                <Form.Group
                    label={t("profile.displayName")}
                    helpText={t("profile.displayNameHelp")}>
                    <Form.Text
                        name="display_name"
                        context={contextForm}
                        props={{ autoComplete: "off" }} />
                </Form.Group>
                {/* UN CHOIX, PAS UN INTERRUPTEUR.
                    C'était un SkeletonSwitch — donc un BOOLÉEN — branché sur
                    `role_level`. Cocher « administrateur » posait `true`, que
                    le `parseInt` du payload traduisait en NaN, sérialisé
                    `null` : le serveur voyait un rôle absent et retombait sur
                    « utilisateur ». D'où « je mets admin à true et le compte
                    sort en user » (signalé le 2026-09-04). L'interrupteur
                    affichait aussi « oui » pour tout le monde, `"2"` étant
                    une chaîne non vide.

                    Root n'est pas proposé : le serveur refuse d'en créer un
                    second, et offrir un choix qui sera rejeté n'est pas un
                    choix. « Administrateur » l'est partout, téléphone compris :
                    ses comptes sont ceux d'un serveur (2026-10-09) — c'est par
                    lui que root y crée le compte du propriétaire. */}
                <Form.Group
                    label={t("admin.members.role")}
                    helpText={t("admin.members.roleHelp")}>
                    <Form.Select
                        name="role_level"
                        context={contextForm}
                        options={[
                            { value: String(ROLE_USER), label: t("admin.members.roleUser") },
                            { value: String(ROLE_ADMIN), label: t("admin.members.roleAdmin") },
                        ]}
                    />
                </Form.Group>
                <Form.Group
                    label={isEditing ? t("admin.members.newPasswordOptional") : t("profile.newPassword")}
                    helpText={t("admin.members.passwordHelp")}
                    props={{ style: { marginBottom: 0 } }}>
                    <Form.Password name="password" context={contextForm} withStrengthEvaluation={!isEditing} />
                </Form.Group>
            </SectionPanel>

            {/* SES CONSOLES (chantier 20) : ce qu'il peut lancer, ici et
                depuis son serveur avec son jeton. Aucune : le catalogue
                seulement — voir et télécharger, sans jouer. */}
            <SectionPanel
                label={t("admin.members.consolesTitle")}
                helpText={isAdminRole ? t("admin.members.consolesAdmin") : t("admin.members.consolesHelp")}>
                <Form.Group>
                    <Form.SkeletonSwitch
                        name="retro"
                        reverse={true}
                        context={contextForm}
                        label={t("admin.members.consolesRetro")}
                        helpText={t("admin.members.consolesRetroHelp")} />
                </Form.Group>
                {streamexConsoles.length > 0 && (
                    <Form.Group label={t("menu.streamex")}>
                        <Form.Multicheckbox
                            name="streamex"
                            context={contextForm}
                            options={streamexConsoles.map((c) => ({ value: c.id, label: c.name }))} />
                    </Form.Group>
                )}
                {remoteplayConsoles.length > 0 && (
                    <Form.Group label={t("menu.remotePlay")} props={{ style: { marginBottom: 0 } }}>
                        <Form.Multicheckbox
                            name="remoteplay"
                            context={contextForm}
                            options={remoteplayConsoles.map((c) => ({ value: c.id, label: c.name }))} />
                    </Form.Group>
                )}
            </SectionPanel>

            {isEditing && (
                <MemberToken
                    authToken={token}
                    member={member}
                    onChanged={(changed) => onSaved(changed, false)}
                    t={t}
                    context={context} />
            )}
        </OffCanvas>
    );
}

/**
 * LE JETON D'UN COMPTE (chantier 20) — « juste après » ses consoles ; et, pour
 * un administrateur, le sien dans son Profil (ce n'est pas ici qu'on se gère
 * soi-même, voir `canManage`).
 *
 * Créé avec le compte, désactivé. L'administrateur l'active pour une durée, le
 * prolonge, le désactive, ou le régénère ; ces gestes valent tout de suite,
 * sans « Enregistrer ». Ce qu'il ouvre suit les consoles juste au-dessus :
 * les changer ne le régénère pas.
 */
// header : ce que le profil met en tête du bloc pour son propre jeton (« Mon QR ») ;
// la page Membres n'en passe pas.
export function MemberToken({ authToken, member, onChanged, header = null, t, context }) {
    const [state, setState] = useState(member.token || null);
    const [duration, setDuration] = useState("1m");
    const [isPending, setPending] = useState(false);

    if (!state) {
        return null;
    }

    const apply = async (call, done) => {
        setPending(true);
        try {
            const result = await call();
            if (!result.ok) {
                context.toast.open(result.exception?.detail || t("app.error"), { variant: "error" });

                return;
            }
            setState(result.item);
            onChanged({ ...member, token: result.item });
            context.toast.open(done, { variant: "success" });
        } finally {
            setPending(false);
        }
    };
    const enable = () => apply(
        () => updateUserToken(authToken, member.id, { enabled: true, duration }),
        t("admin.members.tokenEnabledToast", { name: member.display_name }),
    );
    const disable = () => apply(
        () => updateUserToken(authToken, member.id, { enabled: false }),
        t("admin.members.tokenDisabledToast", { name: member.display_name }),
    );
    const regenerate = () => context.modal.open(t("admin.members.tokenRegenerateConfirm", { name: member.display_name }), {
        variant: "confirm",
        title: t("admin.members.tokenRegenerate"),
        confirmLabel: t("admin.members.tokenRegenerate"),
        cancelLabel: t("app.cancel"),
        onConfirm: () => apply(
            () => regenerateUserToken(authToken, member.id),
            t("admin.members.tokenRegeneratedToast", { name: member.display_name }),
        ),
    });

    const isLive = state.enabled && state.expires_at && new Date(state.expires_at) > new Date();
    const status = !state.enabled
        ? t("admin.members.tokenDisabled")
        : t(isLive ? "admin.members.tokenEnabledUntil" : "admin.members.tokenExpired", { at: formatDateTime(state.expires_at) });
    const usage = state.last_used_at
        ? t("admin.members.tokenUsage", { count: state.use_count, at: formatDateTime(state.last_used_at) })
        : t("admin.members.tokenNeverUsed");
    const durationContext = {
        values: { duration },
        setValues: (next) => setDuration(next.duration || "1m"),
    };

    return (
        <SectionPanel label={t("admin.members.tokenTitle")} helpText={`${status} · ${usage}`}>
            {header}
            {isLive && (
                <>
                    <CopyableValue label={t("admin.members.tokenLink")} value={shareOrigin()} t={t} context={context} />
                    <CopyableValue label={t("admin.members.tokenValue")} value={state.token || ""} t={t} context={context} />
                </>
            )}
            <Form.Group label={t("admin.members.tokenDuration")} helpText={t("admin.members.tokenDurationHelp")}>
                <Form.Select
                    name="duration"
                    context={durationContext}
                    options={TOKEN_DURATIONS.map((value) => ({ value, label: t(`admin.members.tokenDurations.${value}`) }))} />
            </Form.Group>
            <ListElement
                light={true}
                icon={state.enabled ? "bi bi-clock-history" : "bi bi-unlock"}
                label={t(state.enabled ? "admin.members.tokenExtend" : "admin.members.tokenEnable")}
                helpText={t(state.enabled ? "admin.members.tokenExtendHelp" : "admin.members.tokenEnableHelp")}
                onClick={isPending ? null : enable} />
            {state.enabled && (
                <ListElement
                    light={true}
                    icon="bi bi-lock"
                    label={t("admin.members.tokenDisable")}
                    helpText={t("admin.members.tokenDisableHelp")}
                    onClick={isPending ? null : disable} />
            )}
            <ListElement
                light={true}
                icon="bi bi-arrow-repeat"
                label={t("admin.members.tokenRegenerate")}
                helpText={t("admin.members.tokenRegenerateHelp")}
                onClick={isPending ? null : regenerate} />
        </SectionPanel>
    );
}

const ROLE_BADGE = {
    [ROLE_ROOT]: { bg: "dark", key: "admin.members.roleRoot" },
    [ROLE_ADMIN]: { bg: "warning", key: "admin.members.roleAdmin" },
    [ROLE_USER]: { bg: "secondary", key: "admin.members.roleUser" },
};

export default function Members({ context, user, token, t }) {
    const [searchParams, setSearchParams] = useSearchParams();
    const initialPage = Math.max(1, parseInt(searchParams.get("page")) || 1);
    const currentUrlPageRef = useRef(initialPage);
    const [items, setItems] = useState([]);
    const [pagination, setPagination] = useState({ count: 0, page: initialPage });
    // Les rôles cochés, en niveaux (0/1/2) : c'est ce que la base stocke, et
    // traduire des libellés côté serveur n'aurait servi à rien.
    const [roles, setRoles] = useState([]);
    const [isPending, setPending] = useState(true);
    const [searchText, setSearchText] = useState("");
    // Debounced live search — same pattern as Games.jsx: typing alone
    // filters after a short pause, no Enter key and no button needed.
    const isFirstSearchRun = useRef(true);
    useEffect(() => {
        if (isFirstSearchRun.current) {
            isFirstSearchRun.current = false;
            return undefined;
        }
        const timeout = setTimeout(() => goToPage(1), SEARCH_DEBOUNCE_MS);
        return () => clearTimeout(timeout);
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [searchText, roles.join(",")]);

    const fetchUsers = async (page = 1) => {
        setPending(true);
        // The backend's search DSL is JSON, not a plain string (see
        // project_json_search.md): plain-text input becomes an OR across
        // username and display_name — a raw string would 400.
        // Le filtre est appliqué PAR LE SERVEUR : la liste est paginée
        // là-bas, et trier ici n'aurait filtré que la page affichée en
        // laissant le compteur annoncer le total.
        //
        // Les groupes se combinent en OU, les champs d'un groupe en ET : le
        // rôle est donc répété dans chacun des deux groupes de la recherche
        // texte, sans quoi il ne s'appliquerait qu'au premier.
        const trimmed = searchText.trim();
        const level = roles.length > 0 ? { role_level: { in: roles.map(Number) } } : null;
        const groups = trimmed
            ? [
                { username: { like: trimmed }, ...level },
                { display_name: { like: trimmed }, ...level },
            ]
            : (level ? [level] : []);
        const search = groups.length > 0 ? JSON.stringify(groups) : "";

        try {
            const result = await listUsers(token, { page, itemsPerPage: ITEMS_PER_PAGE, search });

            if (result.ok) {
                setItems(result.items || []);
                setPagination(result.pagination ? { ...result.pagination, items_per_page: ITEMS_PER_PAGE } : pagination);
            }
        } catch (failure) {
            // Pas de toast : ce chargement part au montage et à chaque page,
            // un échec bruyant y serait pire que la liste vide.
            console.error("échec du chargement des membres", failure);
        } finally {
            setPending(false);
        }
    };

    const goToPage = (page) => {
        currentUrlPageRef.current = page;
        if ((parseInt(searchParams.get("page")) || 1) !== page) {
            setSearchParams((prev) => {
                const next = new URLSearchParams(prev);
                if (page > 1) next.set("page", page); else next.delete("page");
                return next;
            });
        }
        fetchUsers(page);
    };

    useEffect(() => { fetchUsers(initialPage); }, []);
    useEffect(() => {
        const page = Math.max(1, parseInt(searchParams.get("page")) || 1);
        if (page !== currentUrlPageRef.current) {
            currentUrlPageRef.current = page;
            fetchUsers(page);
        }
        // eslint-disable-next-line react-hooks/exhaustive-deps
    }, [searchParams]);

    const confirmDelete = (member) => {
        context.modal.open(t("admin.members.deleteConfirm", { name: member.display_name }), {
            variant: "confirm",
            title: t("admin.members.deleteTitle"),
            confirmLabel: t("app.delete"),
            cancelLabel: t("app.cancel"),
            onConfirm: async () => {
                const result = await deleteUser(token, member.id);
                if (result.ok) {
                    context.toast.open(t("admin.members.deleted", { name: member.display_name }), { variant: "success" });
                    // Sa ligne part, SUR PLACE (#295).
                    setItems((current) => current.filter((other) => other.id !== member.id));
                    setPagination((current) => ({ ...current, count: Math.max(0, (current.count || 1) - 1) }));
                } else {
                    context.toast.open(result.exception?.detail || t("admin.members.deleteFailed"), { variant: "error" });
                }
            },
        });
    };

    // CE QUE LE FORMULAIRE A ENREGISTRÉ, sur place (#295) : un membre modifié
    // change sa ligne. Un membre créé a le plus grand numéro, et le serveur
    // trie par numéro : sa place est au bout de la DERNIÈRE page — il n'entre
    // que si c'est celle qu'on regarde, et le compteur avance. Sous un filtre,
    // seul le serveur sait s'il y répond (refaire son tri ici en ferait une
    // seconde définition) : la page se relit — c'est le seul cas, avec une
    // réponse illisible.
    const applySaved = (saved, created) => {
        const isFiltered = searchText.trim() !== "" || roles.length > 0;
        if (!saved || (created && isFiltered)) {
            fetchUsers(pagination.page);

            return;
        }
        if (!created) {
            setItems((current) => current.map((other) => (other.id === saved.id ? saved : other)));

            return;
        }
        const lastPage = Math.max(1, Math.ceil(((pagination.count || 0) + 1) / ITEMS_PER_PAGE));
        if ((pagination.page || 1) === lastPage) {
            setItems((current) => [...current, saved]);
        }
        setPagination((current) => ({ ...current, count: (current.count || 0) + 1 }));
    };

    // A member is manageable here if it's not root (never editable or
    // deletable, matching the backend's own hard rule) and it's not
    // yourself — self-management happens in the account settings, not here.
    // Admin-level targets used to be root-only; admins manage each other now
    // (same change in admin_update_user/admin_delete_user).
    const canManage = (member) => {
        if (member.role_level === ROLE_ROOT) return false;
        if (member.id === user?.id) return false;
        return true;
    };

    // Le filtre de rôle vit dans le MÊME contexte que la recherche : les
    // cases écrivaient dans un objet que personne ne lisait, et cochant
    // « Administrateur » on obtenait toujours la liste entière.
    const searchContext = {
        values: { search: searchText, roles },
        setValues: (values) => {
            setSearchText(values.search ?? searchText);
            if (Array.isArray(values.roles)) {
                setRoles(values.roles);
            }
        },
    };

    return (
        <>
            <ListElement
                light={true}
                onClick={() => {
                    context.offcanvas.open(
                        <MemberForm
                            token={token}
                            member={null}
                            onSaved={applySaved} t={t} />,
                        {
                            size: 600,
                        }
                    );
                }}
                icon="bi bi-person-plus"
                label={t("admin.members.create")}
                helpText={t("admin.members.createHelp")}
            />
            <SectionPanel>
                <ListingElement
                    callback={(item) => {
                        const badge = ROLE_BADGE[item.role_level] || ROLE_BADGE[ROLE_USER];
                        const manageable = canManage(item);
                        const guardedAction = (action) => manageable
                            ? action
                            : () => context.toast.open(t("admin.members.cannotManage"), { variant: "warning" });

                        return (
                            <ListElement
                                key={item.id}
                                icon="bi bi-person"
                                label={
                                    <div className="d-flex align-items-center gap-2">
                                        {item.display_name}
                                        <bs.Badge bg={badge.bg} className="text-uppercase small" style={{ fontSize: "0.65rem" }}>
                                            {t(badge.key)}
                                        </bs.Badge>
                                    </div>
                                }
                                helpText={
                                    <div className="d-flex align-items-center gap-2">
                                        @{item.username}
                                        <span className="text-muted">·</span>
                                        <span className="text-muted">{formatDateTime(item.created_at)}</span>
                                        {item.token?.enabled && (
                                            <i className="bi bi-key-fill text-muted" title={t("admin.members.tokenEnabledShort")}></i>
                                        )}
                                    </div>
                                }
                                onClick={canManage(item) ? () => context.offcanvas.open(
                                    <MemberForm
                                        token={token}
                                        member={item}
                                        onSaved={applySaved}
                                        t={t} />,
                                    { size: 600 }
                                ) : undefined}
                                next={manageable ? (
                                    <bs.Button
                                        variant="danger"
                                        onClick={(e) => {
                                            e.preventDefault();
                                            e.stopPropagation();
                                            guardedAction(() => confirmDelete(item))()
                                        }}>
                                        <i className="bi bi-trash"></i>
                                    </bs.Button>
                                ) : true}
                            />
                        );
                    }}
                    items={items}
                    itemCount={pagination.count}
                    isLoading={isPending}
                    currentPage={pagination.page}
                    itemsPerPage={ITEMS_PER_PAGE}
                    onPositionChange={(page) => goToPage(page)}
                    search={{
                        context: searchContext,
                        placeholder: t("admin.members.searchPlaceholder"),
                        filters: {
                            Component: () => (
                                <div>
                                    <Form.Group
                                        label={t("admin.members.filterRole")}
                                        helpText={t("admin.members.filterRoleHelp")}>
                                        <Form.Multicheckbox
                                            name="roles"
                                            context={searchContext}
                                            options={[
                                                { value: String(ROLE_USER), label: t("admin.members.rolePlayer") },
                                                { value: String(ROLE_ADMIN), label: t("admin.members.roleAdmin") },
                                                { value: String(ROLE_ROOT), label: t("admin.members.roleRoot") },
                                            ]} />
                                    </Form.Group>
                                </div>
                            ),
                        },
                    }}
                    texts={{
                        itemsFound: t("admin.members.found"),
                        noItemsFound: t("admin.members.empty"),
                    }}
                />
            </SectionPanel>
        </>
    );
}
