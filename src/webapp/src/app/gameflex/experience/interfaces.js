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
export function preferenceKey(username, origin = '') {
    return `gameflex:interface:v1:${encodeURIComponent(origin)}:${encodeURIComponent(username || 'device-owner')}`;
}
export function readChoice(key) {
    try { const id = localStorage.getItem(key); return interfaces.some(x => x.id === id) ? id : null; }
    catch { return null; }
}
