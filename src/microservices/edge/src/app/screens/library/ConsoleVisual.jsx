import React, { useEffect, useState } from "react";
import { listConsoleBackgrounds } from "../../libs/api.js";
import { consoleBackgroundPath, useImage } from "../../libs/images.js";
import { useLayoutKind } from "../../libs/layout.js";
import { consoleArt } from "../../libs/visuals.js";

// LE VISUEL D'UNE CONSOLE (le porteur, 2026-11-12) :
//   - DERRIÈRE, LA BANDE : un accordéon fixe de panneaux inclinés, faits des fonds
//     de la console (`GET …/consoles/{id}/backgrounds`, ses emplacements dans leur
//     ordre) — six en paysage, cinq en portrait, moins s'il y a moins d'images.
//     Plus les jeux de la bibliothèque : une console sans jeux ressemble aux autres.
//     Une console sans aucune image ici a celles du premier serveur fédéré qui en
//     prête (`#740`), relayées par le kernel. Sans aucun fond, le visuel généré
//     propre à la console.
// Plus d'icône de console devant (le porteur, 2026-11-12 : « on les retire ») ; la
// route du kernel reste.
//
// UNE CONSOLE REMOTE PLAY A UNE SEULE GRANDE IMAGE (le porteur, 2026-11-14, chantier 53, P12 : « quand il y a une ou
// plusieurs consoles, celui-ci affiche un background qu'on choisit dans settings / remote play / la console. Ce serait le
// même mécanisme que pour les autres consoles sauf que là on choisit une seule grande image. ») : le premier emplacement
// du fond de sa console (`rp-ps5`…, le même rangement et les mêmes routes que les autres consoles ; il se remplit par
// défaut des jaquettes de la console qu'elle joue), en photo sur tout l'écran ; son visuel généré le temps qu'elle
// arrive, ou sans image. Elle se choisit dans Réglages › Remote Play › la console (settings/server/RemotePlay.jsx).

const STRIP_WIDTH = 512;
const backgrounds = new Map();

// Les emplacements d'une console changés (Réglages › une console › Apparence) : sa liste se relit à la prochaine bande.
export const forgetBand = (consoleId) => backgrounds.delete(consoleId);

// La liste des fonds d'une console, lue une fois pour la page : `{names, server}`, `undefined` tant qu'on ne sait pas.
// Ce sont ses huit emplacements, dans l'ordre, les libres sautés (le porteur, 2026-11-14, remarque e) ; une image du
// dossier en trop n'y est pas. `server` : le serveur fédéré qui les prête, quand la console n'a aucune image ici
// (`#740`) — ses images se demandent avec lui.
function useBackgrounds(consoleId) {
    const [items, setItems] = useState(() => (consoleId ? backgrounds.get(consoleId) : undefined));

    useEffect(() => {
        if (!consoleId) {
            return undefined;
        }

        if (backgrounds.has(consoleId)) {
            setItems(backgrounds.get(consoleId));

            return undefined;
        }
        let cancelled = false;
        listConsoleBackgrounds(consoleId).then((answer) => {
            const list = { names: answer.ok ? (answer.data?.slots || []).map((slot) => slot.name).filter(Boolean) : [], server: answer.ok ? answer.data?.server || null : null };
            backgrounds.set(consoleId, list);

            if (!cancelled) {
                setItems(list);
            }
        });

        return () => {
            cancelled = true;
        };
    }, [consoleId]);

    return items;
}

// Un panneau de la bande : incliné, l'image redressée dedans.
function StripPanel({ consoleId, name, server }) {
    const image = useImage(consoleBackgroundPath(consoleId, name, STRIP_WIDTH, server));

    return (
        <div className="vpanel">
            {image && <div className="vpanel-img" style={{ backgroundImage: `url("${image.url}")` }}></div>}
        </div>
    );
}

// Une image portrait ou le visuel généré : en panneau, sur son reflet flouté.
function Panel({ image }) {
    return (
        <>
            <div className="vis-bg blur" style={{ backgroundImage: image }}></div>
            <div className="vis-panel" style={{ backgroundImage: image }}></div>
        </>
    );
}

// L'IMAGE D'UNE CONSOLE REMOTE PLAY (un appareil) : l'adresse de sa grande image, `null` sans image, `undefined` tant
// qu'on ne sait pas.
export function useRemoteImage(plat) {
    const consoleId = plat?.kind === "device" ? plat.console.id : null;
    const items = useBackgrounds(consoleId);
    const image = useImage(items?.names.length ? consoleBackgroundPath(consoleId, items.names[0], 1280, items.server) : null);

    if (!consoleId || items === undefined) {
        return undefined;
    }

    return items.names.length ? (image ? image.url : image) : null;
}

// Le visuel en tête de l'engrenage d'une console Remote Play, comme celui d'un jeu (GameBanner, HeroArt.jsx) : sa
// grande image dans un 16:9, sinon son visuel généré.
export function ConsoleBanner({ plat }) {
    const image = useRemoteImage(plat);

    return (
        <div className="gbanner ratio ratio-16x9 rounded-4 overflow-hidden" aria-hidden="true">
            <div>
                <div className="vis-bg" style={{ backgroundImage: consoleArt(plat.console?.id || "console", true) }}></div>
                {image && <div className="vis-bg photo" style={{ backgroundImage: `url("${image}")` }}></div>}
            </div>
        </div>
    );
}

// Le fond d'une plateforme : un nœud.
export function useVisual(plat) {
    const portrait = useLayoutKind() === "portrait";
    const consoleId = plat.kind === "console" ? plat.console.id : null;
    const items = useBackgrounds(consoleId);
    const remote = useRemoteImage(plat);

    if (plat.kind === "device") {
        return (
            <>
                <Panel image={consoleArt(plat.console?.id || "console", true)} />
                {remote && <div className="vis-bg photo" style={{ backgroundImage: `url("${remote}")` }}></div>}
            </>
        );
    }

    if (items?.names.length) {
        return (
            <div className="vstrip d-flex" aria-hidden="true">
                {items.names.slice(0, portrait ? 5 : 6).map((name) => <StripPanel key={name} consoleId={consoleId} name={name} server={items.server} />)}
            </div>
        );
    }

    return <Panel image={consoleArt(plat.console?.id || "console")} />;
}
