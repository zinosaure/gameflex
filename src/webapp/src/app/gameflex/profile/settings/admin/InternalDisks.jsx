import React, { useState, useEffect, useCallback } from "react";
import * as bs from "react-bootstrap";
import { SectionPanel, ListElement } from "../../../shared/templates/Essentials.jsx";
import { listInternalDisks, rescanInternalDisk } from "../../../api/retrogaming.js";
import { capabilities } from "../../../libs/capabilities.js";

// LES RESSOURCES INTERNES (chantier 23) — les autres disques de jeux de cette
// machine.
//
// À LIRE, PAS À MODIFIER : un disque se déclare par une ligne de volume sur le
// kernel, dans le compose de l'installation (`<chemin>:/mnt/<nom>`), pas
// depuis la page — il faut qu'il soit monté dans les conteneurs. Sur le
// téléphone, depuis l'écran d'accueil de l'application (chantier 27), celui de
// qui le tient. D'où ni formulaire ni corbeille ici, à la différence des
// ressources distantes : l'écran dit ce que la machine voit, et relit un
// disque à la demande.
export default function InternalDisks({ context, t }) {
    const [disks, setDisks] = useState(null);
    const [busy, setBusy] = useState(null);
    // OÙ UN DISQUE S'AJOUTE, tel que l'hôte le déclare (`internalDisks`) : les
    // textes qui le disent changent, l'écran non.
    const text = (key, values) => t(
        `admin.disks.${key}${capabilities().internalDisks === "device" ? "Device" : ""}`, values);

    const load = useCallback(() => listInternalDisks().then((answer) => {
        if (!answer.ok) {
            setDisks([]);
            context.toast.open(answer.errorMessage || t("admin.disks.loadFailed"), { variant: "error" });

            return;
        }
        // Dans l'ordre que le serveur donne : celui des noms.
        setDisks(answer.data || []);
    }), [context, t]);

    useEffect(() => {
        load();
    }, [load]);

    const rescan = async (disk) => {
        setBusy(disk.name);
        const answer = await rescanInternalDisk(disk.name);
        setBusy(null);
        context.toast.open(
            answer.ok ? t("admin.disks.rescanned", { name: disk.name }) : (answer.errorMessage || t("admin.disks.rescanFailed")),
            { variant: answer.ok ? "success" : "error" },
        );
        if (answer.ok) {
            load();
        }
    };

    const gamesOf = (disk) => Object.values(disk.consoles || {}).reduce((sum, count) => sum + count, 0);

    return (
        // UN TITRE, SANS QUOI LE TEXTE NE S'AFFICHE PAS : SectionPanel ne montre
        // son `helpText` que sous un `label`. Celui qui dit comment ajouter un
        // disque ne s'est donc jamais affiché (relevé le 2026-10-23).
        <SectionPanel label={t("admin.disks.title")} helpText={text("intro")}>
            {disks === null && <div className="text-center p-3"><bs.Spinner animation="border" size="sm" /></div>}
            {disks !== null && disks.length === 0 && (
                <div className="text-muted p-3">{text("empty")}</div>
            )}
            {(disks || []).map((disk) => (
                <ListElement
                    key={disk.name}
                    icon={disk.present ? "bi bi-device-hdd" : "bi bi-exclamation-triangle"}
                    label={disk.name}
                    helpText={disk.present ? t("admin.disks.present", { count: gamesOf(disk) }) : text("absent")}
                    next={disk.present ? (
                        <bs.Button
                            variant="secondary"
                            title={t("admin.disks.rescan")}
                            disabled={busy !== null}
                            onClick={(event) => {
                                event.preventDefault();
                                event.stopPropagation();
                                rescan(disk);
                            }}
                        >
                            {busy === disk.name
                                ? <bs.Spinner animation="border" size="sm" />
                                : <i className="bi bi-arrow-repeat"></i>}
                        </bs.Button>
                    ) : null}
                />
            ))}
        </SectionPanel>
    );
}
