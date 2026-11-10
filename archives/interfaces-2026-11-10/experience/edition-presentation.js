const languages={fr:'Français',en:'Anglais',es:'Espagnol',de:'Allemand',it:'Italien',ja:'Japonais',nl:'Néerlandais',pt:'Portugais',ko:'Coréen',zh:'Chinois',ru:'Russe',unknown:'Langue non précisée'};
export function languageLabel(value) {
    const codes=Array.isArray(value)?value:typeof value==='string'?value.split(/[,/]/):[];
    return codes.filter(Boolean).map(x=>languages[x.trim().toLowerCase()]||x.trim()).join(' / ') || languages.unknown;
}
export function editionFields(game={}) {
    const title=typeof game.title==='string'?game.title:'';
    const tags=[...title.matchAll(/\(([^)]+)\)|\[([^\]]+)\]/g)].map(match=>match[1]||match[2]);
    const region=typeof game.region==='string'?game.region:tags.find(tag=>/^(USA|Europe|Japan|World|France|Germany|Spain|Italy|Korea|China|Australia)(?:$|[, /])/.test(tag))?.split(',')[0]||'';
    const explicit=game.languages||game.language;
    const inferred=tags.flatMap(tag=>tag.match(/\b(?:En|Fr|De|Es|It|Ja|Nl|Pt|Ko|Zh|Ru)\b/g)||[]).map(code=>code.toLowerCase());
    const language=languageLabel(explicit||[...new Set(inferred)]);
    const edition=typeof game.version==='string'?game.version:typeof game.edition==='string'?game.edition:tags.find(tag=>/^(?:Rev\b|v\d)/i.test(tag))||'';
    return {region,language,edition};
}
