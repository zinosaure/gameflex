// The kernel sends a gamestatus.View, not a display string. Keep the original
// object (including emulator reports) intact and derive only presentation values.
export function gameStatusCode(value) {
    const code = typeof value === 'string' ? value : value?.status;
    return typeof code === 'string' ? code : '';
}
export function gameStatusLabel(value) {
    const code = gameStatusCode(value);
    return ({playable:'Jouable',crashes:'Plante en jeu',unplayable:'Injouable',untested:'Non testé',Complete:'Complet',Unknown:'Non testé'})[code] || code || 'Non testé';
}
export function canResumeGame(game) {
    if (!game) return false;
    // Existence alone does not prove compatibility with the selected emulator.
    return game.saveCompatible === true || (Array.isArray(game.saves) && game.saves.some(save => save && typeof save === 'object' && save.compatible === true));
}
export function recentAction(game) { return canResumeGame(game) ? 'Reprendre' : 'Relancer'; }
