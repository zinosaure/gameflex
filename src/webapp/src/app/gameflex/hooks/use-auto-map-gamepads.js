// use-auto-map-gamepads.js — auto-maps EVERY physical gamepad against the
// virtual-controller presets: pads already plugged in at load, pads plugged
// in later (gamepadconnected), AND pads the browser only reveals lazily —
// some browsers/tablets don't fire gamepadconnected and only expose a pad
// in getGamepads() after its first press, so the seen-set keeps a 2 s poll
// cheap and idempotent. Create-only: see
// gameflex/libs/keyMapping.js's ensureAutoMappedGamepad doc.
import { useEffect, useRef } from "react";
import { ensureAutoMappedGamepad, gamepadDeviceId } from "../libs/keyMapping.js";
import { connectedGamepads } from "../libs/keyMapping.js";

const POLL_MS = 2000;

export function useAutoMapGamepads(token) {
    const seenRef = useRef(new Set());

    useEffect(() => {
        if (!token) return;

        const mapPad = (pad) => {
            if (!pad) return;
            const deviceId = gamepadDeviceId(pad);
            if (seenRef.current.has(deviceId)) return;
            seenRef.current.add(deviceId);

            // SEULEMENT LES MANETTES À DISPOSITION NORMALISÉE.
            //
            // La table par défaut suppose la disposition « standard » de
            // l'API Gamepad : 0 = bouton du bas, 1 = droite, 2 = gauche,
            // 3 = haut, 12-15 = croix directionnelle. Le navigateur dit
            // lui-même s'il a pu y ramener la manette : `pad.mapping` vaut
            // "standard", ou une chaîne vide quand les index sont ceux du
            // pilote, tels quels.
            //
            // On appliquait la table aux deux. Sur une manette non
            // normalisée, ça produit un mapping FAUX — et qui a l'air
            // délibéré, puisqu'il est enregistré : « l'auto-mapping a mal
            // attribué les touches » (signalé le 2026-09-03, PlayStation).
            //
            // Mieux vaut ne rien écrire : l'écran de mapping reste vide, ce
            // qui se voit et s'explique, au lieu d'une table plausible et
            // fausse qu'il faut d'abord soupçonner.
            if (pad.mapping !== "standard") {
                return;
            }

            ensureAutoMappedGamepad(token, deviceId);
        };

        const mapAll = () => {
            for (const pad of connectedGamepads()) {
                mapPad(pad);
            }
        };

        const handleConnect = (event) => mapPad(event.gamepad);

        mapAll();
        const interval = setInterval(mapAll, POLL_MS);
        window.addEventListener("gamepadconnected", handleConnect);
        return () => {
            clearInterval(interval);
            window.removeEventListener("gamepadconnected", handleConnect);
        };
    }, [token]);
}
