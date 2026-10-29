import React, { useState, useEffect, useMemo, useCallback } from "react";
import * as bs from "react-bootstrap";
import {
    SectionPanel,
    ListElement,
    ListingElement,
    useLatestContext,
} from "../../../../shared/templates/Essentials.jsx";
import { formatSize } from "../api/library.js";
import { fetchUserSaves, deleteUserSaves, savesArchiveUrl } from "../api/storage-admin.js";
import { hasLocalLibrary } from "../../../../libs/localLibrary.js";

const ITEMS_PER_PAGE = 10;

// LES SAUVEGARDES, PAR COMPTE — et de loin, volontairement.
//
// Le joueur gère les siennes une par une dans « Mes sauvegardes ». Ici,
// l'administrateur voit de la place occupée par un compte, et n'a que deux
// gestes : l'emporter, ou la reprendre. Rendre le détail à cet endroit
// serait refaire l'écran du joueur, avec les sauvegardes des autres.
//
// À VENIR : synchroniser ces dossiers avec un partage distant — SMB,
// WebDAV, kDrive, ou un autre serveur GameFlex. Le listing par compte est
// déjà la forme dont cette synchronisation aura besoin.
export default function Saves({ t }) {
    const [values, setValues] = useState({ q: "" });
    const [users, setUsers] = useState([]);
    const [isLoading, setLoading] = useState(true);
    const [page, setPage] = useState(1);

    // Le contexte À JOUR, sans que la lecture en dépende : il change d'identité
    // à chaque fenêtre qui s'ouvre (voir `useLatestContext`).
    const latestContext = useLatestContext();
    const context = latestContext.current;

    const reload = useCallback(() => {
        setLoading(true);

        return fetchUserSaves()
            .then((result) => {
                if (!result.ok) {
                    latestContext.current.toast.open(result.errorMessage || t("admin.storage.loadFailed"),
                        { variant: "error" });
                }
                setUsers(result.users || []);
            })
            .catch((failure) => {
                console.error("Chargement des sauvegardes par compte", failure);
                latestContext.current.toast.open(t("admin.storage.loadFailed"), { variant: "error" });
            })
            .finally(() => setLoading(false));
    }, [t, latestContext]);

    useEffect(() => {
        reload();
    }, [reload]);

    const filtered = useMemo(() => {
        const needle = (values.q || "").trim().toLowerCase();

        return needle
            ? users.filter((user) => user.username.toLowerCase().includes(needle))
            : users;
    }, [users, values.q]);

    const visible = useMemo(
        () => filtered.slice((page - 1) * ITEMS_PER_PAGE, page * ITEMS_PER_PAGE),
        [filtered, page],
    );

    // Une ligne supprimée sur la dernière page : on recule d'une page plutôt
    // que de montrer une page vide.
    useEffect(() => {
        const last = Math.max(1, Math.ceil(filtered.length / ITEMS_PER_PAGE));
        if (page > last) {
            setPage(last);
        }
    }, [filtered.length, page]);

    // « LANCÉE », ET NON « SUPPRIMÉE ». La route répond maintenant tout de
    // suite et efface en fond : effacer 29 Go prend des minutes, et tenir la
    // requête pendant ce temps faisait abandonner le navigateur du téléphone —
    // d'où « impossible de lire cette section » sur une liste vide, signalé en
    // vrai le 2026-09-18. Annoncer « supprimé » ici serait annoncer une chose
    // qui n'a pas encore eu lieu.
    //
    // Et l'inverse vaut aussi : le téléphone efface sur-le-champ (un seul
    // porteur, quelques mégaoctets) et le dit par `finished` — lui annoncer
    // une suppression « en arrière-plan » déjà finie serait aussi faux.
    const handleDelete = async (user) => {
        const result = await deleteUserSaves(user.username);
        const done = result.finished
            ? t("admin.storage.savesDeleted", { user: user.username })
            : t("admin.storage.savesDeleteStarted", { user: user.username });
        context.toast.open(
            result.ok
                ? done
                : (result.errorMessage || t("admin.storage.deleteFailed")),
            { variant: result.ok ? "success" : "error" },
        );
        // LA LIGNE CHANGE SUR PLACE, sans relecture (2026-10-10) : partie si
        // l'effacement est fini (le téléphone), « en cours » sinon (le serveur
        // efface en fond — voir plus haut).
        if (result.ok) {
            setUsers((current) => (result.finished
                ? current.filter((entry) => entry.username !== user.username)
                : current.map((entry) => (entry.username === user.username
                    ? { ...entry, deleting: true }
                    : entry))));
        }
    };

    const searchContext = { values, setValues };

    return (
        <SectionPanel>
            <ListingElement
                callback={(user) => (
                    <ListElement
                        key={user.username}
                        icon="bi bi-person"
                        label={user.username}
                        helpText={user.deleting
                            ? t("admin.storage.savesDeleting")
                            : t("admin.storage.savesSummary", {
                                count: user.files,
                                size: formatSize(user.sizeBytes),
                            })}
                        next={(
                            <div className="d-flex gap-2">
                                {/* Sur le téléphone, les sauvegardes SONT déjà
                                    dans `gameflex/saves` : rien ne se
                                    télécharge hors du dossier gameflex (le
                                    porteur, 2026-10-12). */}
                                {!hasLocalLibrary() && (
                                    <bs.Button
                                        variant="secondary"
                                        href={savesArchiveUrl(user.username)}
                                        target="_blank"
                                        rel="noreferrer"
                                        title={t("admin.storage.download")}
                                    >
                                        <i className="bi bi-download"></i>
                                    </bs.Button>
                                )}
                                <bs.Button
                                    variant="danger"
                                    onClick={(e) => {
                                        e.preventDefault();
                                        // Les DEUX, par cohérence avec les
                                        // autres écrans : ici la ligne n'a pas
                                        // de `onClick`, donc rien ne remonte —
                                        // mais un `ListElement` qui en gagne un
                                        // demain rendrait ce bouton fautif sans
                                        // que personne ne pense à revenir ici.
                                        e.stopPropagation();
                                        // Le geste emporte TOUT le dossier
                                        // d'un joueur — des parties en cours
                                        // qui ne sont pas celles de qui
                                        // appuie.
                                        context.modal.open(
                                            <div className="text-center">
                                                {t("admin.storage.deleteSavesConfirm")} <br />
                                                <strong>
                                                    {user.username} — {formatSize(user.sizeBytes)}
                                                </strong>
                                            </div>,
                                            {
                                                variant: "confirm",
                                                onConfirm: () => handleDelete(user),
                                                confirmLabel: t("app.delete"),
                                                cancelLabel: t("app.cancel"),
                                            },
                                        );
                                    }}
                                >
                                    <i className="bi bi-trash"></i>
                                </bs.Button>
                            </div>
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
                    placeholder: t("admin.storage.searchUser"),
                }}
                texts={{
                    itemsFound: t("admin.storage.savesFound"),
                    noItemsFound: t("admin.storage.noSaves"),
                }}
            />
        </SectionPanel>
    );
}
