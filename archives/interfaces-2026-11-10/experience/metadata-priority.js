// One field policy for catalogue cards, details and the existing library.
export const metadataFields=['title','description','attribution','consoleName','coverFront','coverBack','image','images','screenshots','trailers','releaseDate','genres','languages','version','region'];
const present=v=>v!==undefined&&v!==null&&v!==''&&(!Array.isArray(v)||v.length>0);
export function manualMetadata(game){
 const values={...(game.overrides||{}),...(game.manualMetadata||{})};
 for(const key of game.manualFields||[])if(Object.hasOwn(game,key)&&!Object.hasOwn(values,key))values[key]=game[key];
 for(const [key,source] of Object.entries(game.fieldSources||{}))if(source==='manual'&&Object.hasOwn(game,key)&&!Object.hasOwn(values,key))values[key]=game[key];
 return values;
}
export function prioritizeMetadata(base,emugen=base.emugen,secondary=[]){
 const manual=manualMetadata(base);if(!Object.hasOwn(manual,'image')&&Object.hasOwn(manual,'coverFront'))manual.image=manual.coverFront;
 if(emugen?.coverFront&&!emugen.image)emugen={...emugen,image:emugen.coverFront};
 const result={...base},sources={...(base.fieldSources||{})};
 if(emugen?.consoleId!==base.consoleId)emugen=null;
 for(const key of metadataFields){
  if(Object.hasOwn(manual,key)){result[key]=manual[key];sources[key]='manual';continue;}
  if(emugen&&present(emugen[key])){result[key]=emugen[key];sources[key]='EmuGen';continue;}
  if(present(base[key]))continue;
  const fallback=secondary.find(item=>(!item.consoleId||item.consoleId===base.consoleId)&&present(item[key]));
  if(fallback){result[key]=fallback[key];sources[key]=fallback.provider||'Catalogue';}
 }
 if(emugen){result.emugen=emugen;result.emugenSlug=emugen.emugenSlug;result.emugenUrl=emugen.emugenUrl;}
 return {...result,fieldSources:sources,manualMetadata:manual};
}
export function attachSavedMetadata(game,records){
 const saved=records.find(record=>record.consoleId===game.consoleId&&record.reference===game.reference);
 return saved?prioritizeMetadata({...game,...saved.item,reference:game.reference,files:game.files,manualMetadata:{...saved.manualMetadata,...manualMetadata(game)},metadataCandidates:saved.candidates||[]},saved.emugen):game;
}
export function safeExternal(url){try{const value=new URL(url);return ['http:','https:'].includes(value.protocol)&&!value.username&&!value.password;}catch{return false;}}
