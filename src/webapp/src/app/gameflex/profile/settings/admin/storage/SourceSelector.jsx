// D'OÙ ON LISTE, et de quoi relire ce que cette source annonce.
//
// UN SEUL EXEMPLAIRE, et c'est tout l'intérêt de ce fichier. Les écrans des
// cœurs, des BIOS et des jeux posent la même question — « quelle source ? » —
// et offrent le même geste à côté — « relis-la ». Écrit trois fois, l'un des
// trois aurait fini par oublier le bouton, ou par le nommer autrement.
//
// LE BOUTON EST ICI, À CÔTÉ DE LA SOURCE : il relit un LISTAGE. « D'où je
// liste » et « relis-le » se lisent donc au même endroit (tranché avec
// l'utilisateur le 2026-09-17).
//
// Ce composant ne construit PAS la liste des sources : elle dépend de la
// section (la matrice #154 — un montage Samba ne porte que des ROMs, les BIOS
// n'ont pas de source officielle), donc l'appelant la passe déjà faite. Ce qui
// est partagé, c'est la mise en page et la remise en cache, pas la matrice.
import React, { useState, useEffect, useContext } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../../../skeleton/skeleton.js";
import { refreshSource, canRefreshSource, listingDateOf } from "../api/storage-admin.js";
import { listInternalDisks } from "../../../../api/retrogaming.js";

const Form = Skeleton.Form;

// LES DISQUES INTERNES DE CETTE MACHINE (chantiers 23, 24 et 27 — le
// téléphone a les siens, ajoutés depuis son écran d'accueil), présents, en
// options d'un sélecteur de source (`disk:<nom>`). Un seul exemplaire : les
// écrans des jeux, des cœurs, des BIOS, des paramètres et des correctifs les
// proposent de la même façon. Sans disque, aucune option.
export function useDiskSources(t) {
    const [disks, setDisks] = useState([]);

    useEffect(() => {
        let cancelled = false;
        listInternalDisks()
            .then((answer) => {
                if (!cancelled && answer.ok) {
                    setDisks((answer.data || []).filter((disk) => disk.present));
                }
            })
            .catch(() => undefined);

        return () => {
            cancelled = true;
        };
    }, []);

    return disks.map((disk) => ({
        value: `disk:${disk.name}`,
        label: t("retrogaming.diskSource", { name: disk.name }),
    }));
}

// `canRefresh` — le bouton de remise en cache est-il PROPOSABLE ici.
//
// Vrai par défaut, donc les écrans d'administration ne changent pas. L'écran
// des jeux passe `false`, et ce n'est pas un choix de mise en page : les deux
// routes de relecture sont réservées aux administrateurs
// (`POST /api/v1/servers/{id}/rescan` est `adminOnly`, api.go), alors que la
// liste des jeux est ouverte à tous les rôles. Un bouton visible qui répond
// 401 est pire que pas de bouton.
// `label` — le titre du champ, parce que ce composant ne sert plus qu'aux
// écrans d'administration. Son défaut reste la clé d'origine
// (`admin.storage.source`), donc rien ne change là où il est né ; l'écran des
// jeux, lui, passe une clé de son propre espace de noms plutôt que d'afficher
// une chaîne d'administration à un joueur.
// `section` — la partie de l'écran (« roms », « cores », « bios ») : pour cette
// machine, l'icône ne refait QU'ELLE dans le listing (chantier 15 — « Rescan
// all », dans les Réglages, refait tout), et sa date s'écrit sous le
// sélecteur, en texte d'aide, comme celle de la copie d'un serveur fédéré.
export default function SourceSelector({
    t, value, onChange, options, onRefreshed = null, helpText = "", canRefresh = true,
    label = null, section = null,
}) {
    const [isRefreshing, setRefreshing] = useState(false);
    const [listedAt, setListedAt] = useState(null);
    const context = useContext(Skeleton.context);

    // LA DATE DE CE QUI EST LISTÉ, relue à chaque source choisie. Seulement là
    // où l'icône existe : c'est elle qui la fait avancer.
    useEffect(() => {
        let current = true;
        setListedAt(null);
        if (canRefresh && section) {
            listingDateOf(value, section).then((at) => {
                if (current) {
                    setListedAt(at);
                }
            });
        }

        return () => {
            current = false;
        };
    }, [value, section, canRefresh]);

    // Le sélecteur de Skeleton parle en « contexte de formulaire » : un objet
    // de valeurs et son setter. On l'adapte ici pour que l'appelant n'ait
    // qu'une valeur et un callback à fournir.
    const formContext = {
        values: { source: value },
        setValues: (next) => {
            const resolved = typeof next === "function" ? next({ source: value }) : next;

            onChange(resolved.source || "local");
        },
    };

    // Relire, puis réafficher : un bouton qui ne montre pas son effet ne sert
    // à rien.
    const rescan = async () => {
        setRefreshing(true);
        const result = await refreshSource(value, section || "");

        setRefreshing(false);
        if (!result.ok) {
            context.toast.open(result.errorMessage || t("admin.storage.loadFailed"),
                { variant: "error" });

            return;
        }
        if (section) {
            setListedAt(await listingDateOf(value, section));
        }
        if (onRefreshed) {
            onRefreshed();
        }
    };

    if (options.length === 0) {
        return null;
    }

    // LE TRAIT SÉPARE « D'OÙ JE LISTE » DE CE QUI EST LISTÉ. Les quatre écrans
    // qui montent ce composant enchaînent aussitôt un `<ListingElement>`, sans
    // rien entre les deux : le sélecteur se confondait avec la première ligne
    // de la liste (demandé par l'utilisateur le 2026-09-18).
    //
    // Posé ICI et non chez les quatre appelants — un écart se définit une fois,
    // sinon un écran finit par l'oublier. Utilitaires Bootstrap seuls, et SANS
    // `pb-3` : `Form.Group` porte déjà son propre `marginBottom: 1rem`
    // (skeleton/libs/Form.jsx), qui fait l'espace au-dessus du trait — l'ajouter
    // en doublerait l'écart.
    return (
        <div className="border-bottom mb-3">
            <Form.Group
                label={label || t("admin.storage.source")}
                helpText={[helpText, listedAt && t("admin.storage.listedAt", { at: new Date(listedAt).toLocaleString() })]
                    .filter(Boolean).join(" ")}>
                <div className="d-flex align-items-end gap-2">
                    <div className="flex-grow-1">
                        <Form.Select name="source" context={formContext} options={options} />
                    </div>
                    {/* Chaque source a son listing (chantier 15) — cette
                        machine comprise —, sauf la source officielle, qui
                        n'annonce aucune liste : pas de bouton pour elle, plutôt
                        qu'un bouton grisé sans explication. */}
                    {canRefresh && canRefreshSource(value, section || "") && (
                        <bs.Button
                            variant="secondary"
                            disabled={isRefreshing}
                            title={t("admin.storage.rescanSource")}
                            onClick={(event) => {
                                event.preventDefault();
                                rescan();
                            }}
                        >
                            {isRefreshing
                                ? <bs.Spinner animation="border" size="sm" />
                                : <i className="bi bi-arrow-repeat"></i>}
                        </bs.Button>
                    )}
                </div>
            </Form.Group>
        </div>
    );
}
