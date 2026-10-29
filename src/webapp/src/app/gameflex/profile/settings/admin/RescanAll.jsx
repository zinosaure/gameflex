import React, { useState, useEffect, useContext } from "react";
import Skeleton from "../../../../skeleton/skeleton.js";
import { SectionPanel, FormButton } from "../../../shared/templates/Essentials.jsx";
import { fetchSelfListing, rescanSelfListing } from "./api/storage-admin.js";

// « RESCAN ALL » (chantier 15, décidé par le porteur le 2026-10-13) : une
// section à part, juste avant « Stockage », qui ne porte que ce bouton — comme
// celle de la déconnexion —, et la date du listing en texte d'aide.
//
// Elle refait le listing de cette machine EN ENTIER : jeux, BIOS, cœurs,
// correctifs, paramètres. Les icônes à côté des sélecteurs de source, elles,
// ne refont que la section de leur écran. GameFlex refait de lui-même ce qu'il
// écrit ; ce bouton sert à ce qu'on a déposé à la main sur le disque.
export default function RescanAll({ t }) {
    const context = useContext(Skeleton.context);
    const [updatedAt, setUpdatedAt] = useState(null);
    const [isPending, setPending] = useState(false);

    useEffect(() => {
        fetchSelfListing().then((answer) => {
            if (answer.ok) {
                setUpdatedAt(answer.updatedAt || null);
            }
        });
    }, []);

    const rescan = async () => {
        setPending(true);
        const answer = await rescanSelfListing();
        setPending(false);
        if (!answer.ok) {
            context.toast.open(answer.errorMessage || t("app.error"), { variant: "error" });

            return;
        }
        setUpdatedAt(answer.updatedAt || null);
        context.toast.open(t("admin.listing.rescanned"), { variant: "success" });
    };

    return (
        <SectionPanel
            label={t("admin.listing.rescanAll")}
            helpText={updatedAt
                ? t("admin.listing.updatedAt", { at: new Date(updatedAt).toLocaleString() })
                : t("admin.listing.never")}>
            <FormButton
                callback={rescan}
                label={t("admin.listing.rescanAll")}
                isPending={isPending} />
        </SectionPanel>
    );
}
