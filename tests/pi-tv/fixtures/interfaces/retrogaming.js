export async function listRoms(consoleId) {
    await new Promise(resolve=>setTimeout(resolve,450));
    if(consoleId==='n64')return {ok:true,data:Array.from({length:24},(_,index)=>({consoleId:'n64',consoleName:'Nintendo 64',reference:`fixture-${index+1}`,title:['007 — GoldenEye','Banjo-Kazooie','Diddy Kong Racing','F-Zero X','Mario Kart 64','Perfect Dark','Pokémon Stadium','Star Fox 64','Super Mario 64','The Legend of Zelda'][index%10]+(index>9?` · ${index+1}`:''),status:'Complete',files:[`fixture-${index+1}.z64`]}))};
    return {ok:true,data:consoleId==='nes'?[{consoleId:'nes',consoleName:'NES',reference:'palette',title:'Palette libre',status:'Complete',files:['palette.nes']},{consoleId:'nes',consoleName:'NES',reference:'test',title:'Essai clavier',status:'Unknown',files:['test.nes']}]:[]};
}
export async function listCores(){return {ok:true,data:[{installed:true,name:'fixture-core'}]};}
export async function createSession(...args){window.__launchArgs=args;return {ok:true,data:{sessionId:'preview-session'}};}
