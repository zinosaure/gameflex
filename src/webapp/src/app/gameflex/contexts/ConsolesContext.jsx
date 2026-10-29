import React, { createContext, useContext, useEffect, useState } from "react";
import { apiUrl } from "../libs/serverBase.js";

// Replaces the old static nodejs/src/app/gameflex/consoles.js — Go
// (golang/src/internal/consoles) is now the single source of truth for
// console metadata (a leftover of the client-side EmulatorJS/WASM era; the
// app is now fully server-side emulation, so this data belongs there, not
// duplicated in a JS file the Go server can't read). Fetched once here, at
// the same app-root level as AuthContext.jsx/PresenceContext.jsx, and
// consumed via useConsoles() by every component that used to `import
// consoles from "./consoles.js"` — same array shape, same `.id`/`.name`/
// `.publisher`/`.method`/`.cores`/`.maxPlayers` fields, just fetched instead
// of statically imported.
const ConsolesContext = createContext([]);
// Whether that fetch has settled, success or failure. An empty array alone
// can't say "not loaded yet" from "loaded, nothing there" — and the router
// needs exactly that difference before deciding a console path is unknown
// (see routesPending in index.js).
const ConsolesLoadedContext = createContext(false);

export function ConsolesProvider({ children }) {
    const [consoles, setConsoles] = useState([]);
    const [isLoaded, setIsLoaded] = useState(false);

    useEffect(() => {
        let cancelled = false;

        // UNE SEULE ROUTE, DEUX HÔTES (chantier 8, C8-P2).
        //
        // Le registre des consoles — ce qu'EST une console, pas ce qu'on en
        // a — vient de `/api/v1/consoles`. Le téléphone le servait sous
        // `/api/v1/library/consoles`, alors ce contexte devait deviner lequel
        // appeler ; interroger la mauvaise route laissait la liste vide pour
        // toute la session, et le menu des consoles n'affichait plus rien
        // (constaté le 2026-08-30). Le kernel du téléphone sert désormais le
        // même nom, donc il n'y a plus rien à deviner.
        fetch(apiUrl("/api/v1/consoles"))
            .then((res) => res.json())
            .then((json) => {
                const items = Array.isArray(json) ? json : json.data;
                if (!cancelled && Array.isArray(items)) {
                    setConsoles(items);
                }
            })
            .catch(() => {})
            .finally(() => {
                if (!cancelled) {
                    setIsLoaded(true);
                }
            });

        return () => { cancelled = true; };
    }, []);

    return (
        <ConsolesContext.Provider value={consoles}>
            <ConsolesLoadedContext.Provider value={isLoaded}>
                {children}
            </ConsolesLoadedContext.Provider>
        </ConsolesContext.Provider>
    );
}

// Returns the plain array (empty until the fetch resolves) — every consumer
// already tolerates a brief empty/undefined lookup during initial load; the
// router waits on useConsolesLoaded() instead (index.js, routesPending).
export function useConsoles() {
    return useContext(ConsolesContext);
}

export function useConsolesLoaded() {
    return useContext(ConsolesLoadedContext);
}
