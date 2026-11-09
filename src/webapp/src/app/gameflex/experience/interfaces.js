// Stable identifiers are persisted; labels can evolve without resetting choices.
export const interfaces = [
    {id:'station',name:'Horizon',note:'Panorama et rail de consoles',icon:'◒'},
    {id:'crt',name:'Signal CRT',note:'Écran cathodique et liste',icon:'▣'},
    {id:'shelf',name:'La collection',note:'Étagère et cartouches',icon:'▥'},
    {id:'orbit',name:'Orbitale',note:'Sélection spatiale des systèmes',icon:'◎'},
    {id:'arcade',name:'Borne Arcade',note:'Marquise et sélection arcade',icon:'▰'},
    { id: 'minimal', name: 'Minimal', note: 'Liste compacte', icon: '≡' },
    { id: 'covers', name: 'Jaquettes', note: 'Galerie de jaquettes', icon: '▦' },
    { id: 'consoles', name: 'Consoles', note: 'Carrousel de consoles', icon: '⌘' },
    { id: 'favorites', name: 'Favoris', note: 'Jeux épinglés', icon: '♥' },
    { id: 'resume', name: 'Reprise rapide', note: 'Menu en ruban', icon: '▶' },
    { id: 'collections', name: 'Collections', note: 'Étagères de jeux', icon: '▱' },
    { id: 'detail', name: 'Fiche de jeu', note: 'Affiche et sélection', icon: '◈' },
    { id: 'archives', name: 'Archives', note: 'Liste de fichiers', icon: '▤' },
    { id: 'multiplayer', name: 'Salon multijoueur', note: 'Sélection arcade', icon: '♧' },
    { id: 'dashboard', name: 'Tableau de bord', note: 'Raccourcis et récents', icon: '⌂' },
];
// Keep the historical registry for saved preferences and old links. Only verified
// appearances belong in the picker; a pack can supply additional registry entries.
export const movedInterfaces = Object.freeze({
    favorites: 'favorites', resume: 'resume', collections: 'collections',
    detail: 'detail', archives: 'management', multiplayer: 'multiplayer',
});
export const selectableInterfaces = interfaces.filter(x => !Object.hasOwn(movedInterfaces, x.id));
export const interfacePackCapacity = 22;
export function migratePreference(preference) {
    if (!preference || !Object.hasOwn(movedInterfaces, preference.interface)) return preference;
    return { ...preference, interface: 'station', previousInterface: preference.interface,
        interfaceDestination: preference.interface };
}
export function legacyDestination(id, history = [], games = []) {
    if (!Object.hasOwn(movedInterfaces, id)) return null;
    if (id === 'archives') return { section: 'management', view: 'archives' };
    if (id === 'detail') {
        const game = history.map(key => games.find(g => `${g.consoleId}:${g.reference}` === key)).find(Boolean);
        return { section: 'library', game };
    }
    return { section: movedInterfaces[id] };
}
export function preferenceKey(username, origin = '') {
    return `gameflex:interface:v1:${encodeURIComponent(origin)}:${encodeURIComponent(username || 'device-owner')}`;
}
export function readChoice(key) {
    try { const id = localStorage.getItem(key); return interfaces.some(x => x.id === id) ? id : null; }
    catch { return null; }
}
