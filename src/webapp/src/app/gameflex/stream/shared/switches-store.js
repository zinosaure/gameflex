// LE MAGASIN DES INTERRUPTEURS D'UNE PAGE DE STREAMING.
//
// Un magasin externe, et non un `useState` — parce que le menu vit dans un
// OFFCANVAS. `offcanvas.open` range un ÉLÉMENT React construit une fois, dont
// les props sont un instantané gelé : un interrupteur qui lit une valeur
// passée en prop ne bouge jamais, et le `setValues` qu'il appelle écrit dans
// une fermeture périmée. Le symptôme est toujours le même — « le bouton reste
// sur off » — et il est revenu sur la page de test des manettes le
// 2026-09-09, faute d'avoir réutilisé ce qui existait ici.
//
// Avec un magasin, le panneau s'ABONNE (`useSyncExternalStore`) au lieu de
// recevoir : il se redessine tout seul, où qu'il soit monté.

// context.offcanvas.open(<FloatingPanel .../>, opts) captures that element
// (and its props) ONCE, at click time, then portals it from a totally
// different point in the React tree (Skeleton's own root, not
// StreamingShell's) — StreamingShell re-rendering with a new `switches`
// value never reaches an already-open panel, since its props are frozen to
// whatever they were when `.open()` was called. Same fix already used
// elsewhere in this app for exactly this class of problem
// (libs/gamepadPreferences.js's useSyncExternalStore) — a plain external
// store both StreamingShell and FloatingPanel subscribe to independently,
// so FloatingPanel re-renders itself from the inside on every toggle
// regardless of whether its own props ever change.
export function createSwitchesStore(initial) {
    let state = initial;
    const listeners = new Set();
    return {
        getSnapshot: () => state,
        // MERGES, like React's own setState with an object — every caller
        // already assumed that and passed only the keys it meant to change.
        // Replacing instead wiped the rest of the panel: the effect that
        // gives a player back the controller passes just
        // {isControllerVisible, isChainButtons300}, so becoming a player
        // dropped isMuted, isResourceMonitoringVisible, the rumble
        // preferences... — with isMuted gone, the <audio> element kept the
        // muted state ontrack had set imperatively and the switch could no
        // longer turn it off ("video but no sound even when unmuting",
        // reported live 2026-08-24).
        setValues: (next) => {
            const patch = typeof next === "function" ? next(state) : next;
            state = { ...state, ...patch };
            listeners.forEach((listener) => listener());
        },
        subscribe: (listener) => {
            listeners.add(listener);
            return () => listeners.delete(listener);
        },
    };
}
