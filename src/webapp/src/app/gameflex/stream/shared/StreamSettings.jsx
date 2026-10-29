import React, { useState } from "react";
import * as bs from "react-bootstrap";
import Skeleton from "../../../skeleton/skeleton.js";
import { SectionPanel } from "../../shared/templates/Essentials.jsx";
import { probeNetworkAndSuggestPreset } from "./libs/network-probe.js";

const Form = Skeleton.Form;

// LES RÉGLAGES DE FLUX — définition, images par seconde, débit.
//
// Le même panneau ET les mêmes options pour remote-play et pour streamex.
//
// Il était écrit deux fois, avec les MÊMES clés de traduction
// (`remoteplay.settings.*`) des deux côtés — ce qui disait déjà que c'était le
// même écran. Le panneau a été partagé le 2026-09-09 ; ses OPTIONS ne
// l'étaient pas, et les deux listes avaient dérivé : remote-play proposait
// 360p/540p et pas 480p, streamex l'inverse, avec des débits différents et
// une détection automatique d'un seul côté (2026-09-10).
//
// L'ARGUMENT ÉCARTÉ ALORS ÉTAIT LE BON, ET LA CONSOLE L'A TRANCHÉ.
//
// On avait écrit ici que « une PS5 déclare ce qu'elle sait faire, un conteneur
// prend ce qu'on lui donne » ne tenait pas, puisque les deux listes étaient
// deux tables écrites par nous. C'était confondre l'ENDROIT où la table est
// écrite avec ce qui la CONTRAINT : un conteneur rend la géométrie qu'on lui
// demande, une PS5 n'accepte que ses quatre modes de Remote Play.
//
// Le 2026-09-20, une session lancée en « 480p » a fait exactement ce que cet
// argument prédisait : poignée de main réussie, GKCrypt initialisé, puis la
// console raccroche en 0,7 s — « AvCap failed to initialize video ». 854×480
// n'est pas un mode qu'elle sait ouvrir.
//
// DÉCISION DE L'UTILISATEUR, LE JOUR MÊME : 480p disparaît PARTOUT — rétro,
// streamex et remote-play. On aurait pu ne le retirer que du remote-play et
// garder deux listes ; garder un réglage qui rend bien d'un côté et TUE une
// partie de l'autre est précisément le genre d'écart qui se paie plus tard.
// Une seule échelle, la même pour les trois.

/** L'échelle unique — les trois plateformes offrent exactement ceci. */
export const STREAM_OPTIONS = {
    resolutions: [
        { value: "540p", label: "540p" },
        { value: "720p", label: "720p" },
        { value: "1080p", label: "1080p" },
    ],
    frameRates: [
        { value: "30", label: "30 FPS" },
        { value: "60", label: "60 FPS" },
    ],
    // Le débit reste en kbps — c'est ce que les deux backends valident (le
    // kernel borne à 500-20000, le launch_spec de la PS5 y lit son plafond de
    // départ). Les libellés, eux, parlent la langue de tout le monde.
    bitrates: [
        { value: "auto", label: "Auto" },
        { value: "2000", label: "2 Mbps" },
        { value: "4000", label: "4 Mbps" },
        { value: "8000", label: "8 Mbps" },
        { value: "15000", label: "15 Mbps" },
    ],
};

// `toast` : celui du contexte Skeleton, pas du formulaire. Le `context` d'un
// panneau de réglages est le contexte de FORMULAIRE (`values`/`setValues`) —
// il ne sait pas parler à l'écran, et c'est très bien ainsi. Absent, la
// détection s'applique en silence plutôt que de tomber en panne.
export function StreamSettings({ t, context, toast = null, options = STREAM_OPTIONS }) {
    const [isProbing, setIsProbing] = useState(false);

    // LA DÉTECTION MESURE LE RÉSEAU, PAS LA CONSOLE.
    //
    // Elle sonde la latence et le débit vers notre propre API, puis propose
    // une définition et une cadence que le lien peut tenir. Rien là-dedans ne
    // regarde une PS5 — et c'est même EXACTEMENT le chemin qu'emprunte une
    // session streamex. Elle n'avait aucune raison de rester d'un seul côté.
    //
    // Le débit n'est délibérément pas proposé : il doit rester sur « Auto »
    // pour que l'adaptation temps réel (côté PS5 comme côté encodeur) fasse
    // son travail — voir le commentaire de `network-probe.js`.
    const autoDetect = async () => {
        setIsProbing(true);
        try {
            const { suggestion } = await probeNetworkAndSuggestPreset();
            if (suggestion) {
                context.setValues({
                    ...context.values,
                    resolution: suggestion.resolution,
                    frameRate: suggestion.frameRate,
                });
                toast?.open(t("remoteplay.settings.probeApplied"), { variant: "normal" });
            } else {
                toast?.open(t("remoteplay.settings.probeUnreachable"), { variant: "error" });
            }
        } catch {
            toast?.open(t("remoteplay.settings.probeFailed"), { variant: "error" });
        }
        setIsProbing(false);
    };

    return (
        <SectionPanel
            label={t("remoteplay.settings.title")}
            helpText={t("remoteplay.settings.help")}>
            <div className="mb-4 border-bottom pb-3">
                <bs.Button
                    variant="secondary"
                    className="flex-fill py-2 w-100"
                    disabled={isProbing}
                    onClick={autoDetect}
                >
                    {isProbing && (
                        <bs.Spinner animation="border" size="sm" className="me-2" />
                    )}
                    {t("remoteplay.settings.autoDetect")}
                </bs.Button>
            </div>
            {/* CHAQUE Form.Group PORTE SON helpText (demandé le 2026-09-18).
                Les trois en étaient dépourvus, alors que `bitrateHelp`
                existait déjà dans les deux langues sans être affiché : un
                libellé seul dit CE QU'ON RÈGLE, jamais ce que le réglage
                change — et ici il change la qualité d'un flux en direct. */}
            <Form.Group
                label={t("remoteplay.settings.resolution")}
                helpText={t("remoteplay.settings.resolutionHelp")}>
                <Form.Select
                    name="resolution"
                    context={context}
                    options={options.resolutions || []} />
            </Form.Group>
            <Form.Group
                label={t("remoteplay.settings.frameRate")}
                helpText={t("remoteplay.settings.frameRateHelp")}>
                <Form.Select
                    name="frameRate"
                    context={context}
                    options={options.frameRates || []} />
            </Form.Group>
            <Form.Group
                label={t("remoteplay.settings.bitrate")}
                helpText={t("remoteplay.settings.bitrateHelp")}
                props={{ style: { marginBottom: 0 } }}
            >
                <Form.Select
                    name="bitrate"
                    context={context}
                    options={options.bitrates || []} />
            </Form.Group>
        </SectionPanel>
    );
}
