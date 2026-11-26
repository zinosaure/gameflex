// Route : /settings/consoles/<console>/metadata-orphans — les métadonnées sans jeu.
import { useFeedback } from "../../bricks/useFeedback.jsx";
import React, { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useParams } from "react-router-dom";
import { EmptyState } from "../../bricks/EmptyState.jsx";
import { Listing } from "../../bricks/Listing.jsx";
import { Chip } from "../../bricks/Chip.jsx";
import NavRow from "../../bricks/NavRow.jsx";
import { PlaceholderRow } from "../../bricks/PlaceholderRow.jsx";
import { Section } from "../../bricks/Section.jsx";
import { useConfirm } from "../../bricks/useConfirm.jsx";
import { deleteMetadataOrphan, listMetadataOrphans, metadataOrphanCoverPath, readToken, remoteTargetNow } from "../../libs/api.js";
import { useImage } from "../../libs/images.js";
import { kernelReachable, kernelTarget, useKernelChannel, useKernelTopic } from "../../libs/kernelChannel.js";
import { toast } from "../../shell/Shell.jsx";
import SettingsPage, { useQueryState } from "../SettingsPage.jsx";
import { consolePath, useEmulated } from "../Consoles.jsx";

// /settings/consoles/* (Settings.jsx).
// LA JAQUETTE QUE GARDE UN DOSSIER SANS JEU, en tête de sa ligne ; l'icône d'une image s'il n'en garde pas.
function OrphanCover({ consoleId, orphan }) {
    const image = useImage(orphan.cover ? metadataOrphanCoverPath(consoleId, orphan.gameId, orphan.disk) : null);

    // LA JAQUETTE COMME CELLE D'UN JEU (le porteur, 2026-11-25 : « le listing des jeux dans metadata without a game est
    // différent ») : celle de GameCover (HeroArt.jsx), le disque quand il n'y en a pas.
    return image?.url ? <img className="gf-cover rounded object-fit-cover" src={image.url} alt="" /> : <Chip icon="bi-disc" tone="slate" />;
}

// /settings/consoles/<console>/metadata-orphans : LES MÉTADONNÉES SANS JEU (le porteur, 2026-11-14 : « on peut se
// retrouver avec des metadata des jeux qu'on n'a plus accès. Ce qui fera des métadata orphelin. […] Cette page listera
// que les metadata qui non pas jeux et quand on clique dessus ça propose de le supprimer avec confirmation. ») — une
// ligne de « Plus » de la console, sous ses jeux. Chaque dossier qu'aucun jeu accessible ne porte : son titre gardé
// (sinon son identifiant), sa jaquette, où il vit (le serveur, ou le disque) ; le toucher demande, puis le retire — le
// kernel revérifie qu'il est toujours sans jeu (un disque rebranché entre-temps le garde). Une liste lue en asynchrone :
// feuilletée, ses fantômes pendant la lecture, sa ligne grisée quand elle est vide.
//
// LES JEUX DE LA CONSOLE ENCORE EN LECTURE (chantier 53, 2026-11-14) : le kernel ne juge un dossier orphelin qu'avec
// l'empreinte de chaque jeu en mémoire, et la lit en fond quand elle manque (après un redémarrage : une minute et plus
// pour une grande bibliothèque) — il répond 503 `games_not_ready` sans attendre. La liste le dit d'une ligne grisée, et
// s'abonne au sujet `metadata-orphans/<console>` : le kernel lui pousse l'instant où les empreintes sont lues, et elle
// se relit alors, une fois (#751 : elle se relisait toutes les quatre secondes). Une SUPPRESSION confirmée attend sa réponse sur sa
// ligne (`pending` : grisée, son tourniquet ; un second geste ne repart pas) : partie (ou déjà partie, 404), la ligne
// s'en va ; gardée — un jeu la porte de nouveau (409), les jeux pas encore lus (503) —, l'avis le dit et la ligne revient.
export default function Orphans() {
    const { t } = useTranslation();
    const feedback = useFeedback();
    const { consoleId } = useParams();
    const confirm = useConfirm();
    const entry = useEmulated().find((item) => item.id === consoleId);
    const [read, update] = useQueryState({ page: "1", q: "" });
    // SA RECHERCHE (un listing a toujours la sienne) : le titre, l'identifiant, le disque.
    const needle = (read("q") || "").trim().toLowerCase();
    const [orphans, setOrphans] = useState(undefined);
    const [reading, setReading] = useState(false);
    const [readAgain, setReadAgain] = useState(0);
    const [removing, setRemoving] = useState([]);
    const removingNow = useRef(new Set());
    const keyOf = (orphan) => `or-${orphan.disk || "server"}-${orphan.gameId}`;

    // Tant que le kernel lit les empreintes : son sujet dit quand c'est fait.
    const token = readToken();
    // Chez un serveur fédéré administré d'ici (chantier 73), son canal à lui.
    const channel = useKernelChannel(reading && kernelReachable(token) ? remoteTargetNow() || kernelTarget(null, token) : null);
    useKernelTopic(channel, reading ? `metadata-orphans/${consoleId}` : null, null, (state) => {
        if (state?.ready) {
            setReadAgain((count) => count + 1);
        }
    });

    useEffect(() => {
        let cancelled = false;
        const fetchList = () => listMetadataOrphans(consoleId).then((answer) => {
            if (cancelled) {
                return;
            }

            if (answer.errorCode === "games_not_ready") {
                setReading(true);

                return;
            }

            if (!answer.ok) {
                toast(answer.errorMessage || t("settings.loadFailed"));
            }
            setReading(false);
            setOrphans(answer.ok ? answer.data?.orphans || [] : []);
        });

        setOrphans(undefined);
        fetchList();

        return () => {
            cancelled = true;
        };
    }, [consoleId, readAgain, t]);

    const remove = async (orphan, name) => {
        const key = keyOf(orphan);

        if (removingNow.current.has(key)) {
            return;
        }
        removingNow.current.add(key);
        setRemoving((current) => [...current, key]);
        const answer = await deleteMetadataOrphan(consoleId, orphan.gameId, orphan.disk);

        removingNow.current.delete(key);
        setRemoving((current) => current.filter((other) => other !== key));

        if (!answer.ok && answer.status !== 404) {
            feedback.failed(answer.errorMessage || t("settings.actionFailed"));

            return;
        }
        setOrphans((current) => (current || []).filter((other) => keyOf(other) !== key));
        feedback.success(t("consoles.orphanRemoved", { name }));
    };
    const ask = (orphan) => {
        const name = orphan.title || orphan.gameId;

        confirm({
            title: t("consoles.orphanRemove"),
            message: t("consoles.orphanRemoveConfirm", { name }),
            confirmLabel: t("consoles.orphanRemove"),
            onConfirm: () => remove(orphan, name),
        });
    };

    return (
        <SettingsPage title={t("consoles.orphans")} parent={consolePath(consoleId)} levels={{ [consolePath(consoleId)]: entry?.name }} guides={[["consoles"], ["library", "3-get-the-emulators"]]}>
            <Section title={t("consoles.orphansTitle")} help={t("consoles.orphansHelp")} sense="modify">
                {reading ? <EmptyState icon="bi-hourglass-split" text={t("consoles.orphansReading")} /> : (
                    <Listing items={(orphans || []).filter((orphan) => !needle || [orphan.title, orphan.gameId, orphan.disk].some((text) => String(text || "").toLowerCase().includes(needle)))}
                        loading={orphans === undefined} page={read("page")} onPage={(page) => update({ page })} search={read("q")} onSearch={(q) => update({ q, page: "1" })}
                        placeholder={t("consoles.orphansSearch")} searchKey="or-search"
                        found={t("consoles.orphansFound", { count: (orphans || []).length })} empty={t("consoles.orphansNone")}
                        placeholderRow={() => <PlaceholderRow lead="cover" />}
                        render={(orphan) => {
                            const key = keyOf(orphan);

                            return (
                                <NavRow key={key} id={key} lead={<OrphanCover consoleId={consoleId} orphan={orphan} />} label={orphan.title || orphan.gameId}
                                    help={[orphan.disk || t("games.source.library"), orphan.title ? orphan.gameId : null]} opens="confirm" pending={removing.includes(key)}
                                    onOpen={() => ask(orphan)} />
                            );
                        }} />
                )}
            </Section>
        </SettingsPage>
    );
}
