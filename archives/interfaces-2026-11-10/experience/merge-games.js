import {prioritizeMetadata,manualMetadata} from './metadata-priority.js';
import identities from './game-identities.json';
// Group the same game across providers/regions. Keep sequels and unreleased builds distinct.
export function displayTitle(title=''){
    return title.replace(/\s+(?:Retirée?|Removed|Withdrawn)\s*$/i,'').replace(/\.(nes|sfc|smc|gba|gbc?|nds|n64|z64|v64|bin|iso|chd|cue|zip|7z)$/i,'').replace(/\([^)]*\)|\[[^\]]*\]/g,tag=>{
        const value=tag.slice(1,-1).trim();
        if(/^(?:USA|Europe|Japan|World|UE|EU|Australia|Asia|France|Germany|Spain|Italy|Korea|China|Taiwan|Brazil|Netherlands|Sweden|Canada|Russia|UK|Hong Kong)(?:$|[, /])/i.test(value)||/^(?:Rev\s*\d|Virtual Console|Switch Online|SGB Enhanced|GB Compatible|v?\d+(?:\.\d+)*$)/i.test(value)||/^(?:(?:En|Fr|De|Es|It|Ja|Nl|Sv|No|Da|Fi|Pt|Zh|Ko|Ru|Pl)(?:[, +/]|$))+$/i.test(value)||/^[!b]\d*$/i.test(value))return '';
        return tag;
    }).replace(/\s+/g,' ').trim();
}
const compact=title=>title.normalize('NFKD').replace(/[\u0300-\u036f]/g,'').toLowerCase().replace(/[^a-z0-9]/g,'');
const aliases=new Map();
for(const [consoleId,games] of Object.entries(identities))for(const game of games){
    aliases.set(`${consoleId}:${compact(game.title)}`,{id:game.id,title:game.title,language:'fr'});
    for(const [title,language] of Object.entries(game.aliases)){
        const value={id:game.id,title:game.title,language};
        aliases.set(`${consoleId}:${compact(title)}`,value);
        // Some catalogues omit “Version” or “Edition” from these explicit aliases.
        aliases.set(`${consoleId}:${compact(title.replace(/\b(?:version|edition|edicion|versione)\b/gi,''))}`,value);
    }
}
export const translatedIdentity=(game,consoleId='')=>aliases.get(`${game.consoleId||consoleId}:${compact(displayTitle(game.title))}`);
export const gameIdentity=(game,consoleId='')=>`${game.consoleId||consoleId}:${translatedIdentity(game,consoleId)?.id||compact(displayTitle(game.title))}`;
export function variantLanguage(game,consoleId=''){
    const language=translatedIdentity(game,consoleId)?.language;
    const tags=[...game.title.matchAll(/\(([^)]+)\)/g)].map(x=>x[1]).join(',');
    const codes=[...new Set((tags.match(/\b(?:En|Fr|De|Es|It|Ja|Nl|Pt|Ko|Zh|Ru)\b/g)||[]).map(x=>x.toLowerCase()))];
    return codes.length?codes.join(','):language||'unknown';
}
const sourceFor=(game,fallback)=>game.provider||fallback;
export function mergeGames(groups,consoleId=''){
    const merged=new Map();
    for(const {items=[],provider} of groups)for(const game of items){
        const key=gameIdentity(game,consoleId);if(!displayTitle(game.title))continue;
        const origin=sourceFor(game,provider),old=merged.get(key);
        const translated=translatedIdentity(game,consoleId);
        const variant={emugenSlug:game.emugenSlug,emugenUrl:game.emugenUrl,emugen:game.emugen,fieldSources:game.fieldSources,description:game.description,title:game.title,source:game.source||'',path:game.path||'',provider:origin,reference:game.reference||'',consoleId:game.consoleId||consoleId,image:game.image,attribution:game.attribution,fallbackImage:game.fallbackImage,fallbackAttribution:game.fallbackAttribution,downloadId:game.downloadId,license:game.license,language:variantLanguage(game,consoleId)};
        if(!old){merged.set(key,{...game,consoleId:game.consoleId||consoleId,title:translated?.title||displayTitle(game.title),sources:origin?[origin]:[],variants:[variant],...(game.provider==='EmuGen'?{emugen:game.emugen||game}:{})});continue;}
        const rank=g=>g.reference?3:g.downloadId?2:1;
        const preferred=rank(game)>rank(old)?game:old;
        const imagePreferred=game.provider==='FreeROMs'&&old.image?old:preferred;
        const title=translated?.title||(old.title===old.title.toLowerCase()&&game.title!==game.title.toLowerCase()?displayTitle(game.title):old.title);
        const sources=[...new Set([...old.sources,...(origin?[origin]:[])])];
        const variants=old.variants.some(v=>v.source===variant.source&&v.title===variant.title&&v.provider===variant.provider)?old.variants:[...old.variants,variant];
        merged.set(key,{...old,...preferred,title,image:!old.reference&&translated?.language==='fr'&&game.image&&game.provider!=='FreeROMs'?game.image:imagePreferred.image||old.image||game.image,attribution:!old.reference&&translated?.language==='fr'&&game.image&&game.provider!=='FreeROMs'?game.attribution:imagePreferred.image?imagePreferred.attribution:old.image?old.attribution:game.attribution,fallbackImage:game.provider==='FreeROMs'?game.image:old.fallbackImage||game.fallbackImage||((game.image!==old.image)?game.image:undefined),fallbackAttribution:game.provider==='FreeROMs'?game.attribution:old.fallbackAttribution||game.fallbackAttribution||game.attribution,emugen:game.provider==='EmuGen'?(game.emugen||game):old.emugen,manualMetadata:{...manualMetadata(game),...manualMetadata(old)},download:game.download||old.download,pendingDownload:game.pendingDownload||old.pendingDownload,sources,variants});
    }
    // Apply the same priority after grouping, preserving installed references and manual edits.
    for(const [key,game] of merged){
        const emu=game.emugen||(game.provider==='EmuGen'?game:game.variants.find(v=>v.provider==='EmuGen')?.emugen);
        merged.set(key,prioritizeMetadata({...game,manualMetadata:manualMetadata(game)},emu,game.variants));
    }
    return [...merged.values()].sort((a,b)=>a.title.localeCompare(b.title,'fr',{numeric:true}));
}
