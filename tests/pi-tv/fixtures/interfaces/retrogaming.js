export async function listRoms(consoleId) {
    await new Promise(resolve=>setTimeout(resolve,450));
    return {ok:true,data:consoleId==='nes'?[{consoleId:'nes',consoleName:'NES',reference:'palette',title:'Palette libre',status:'Complete',files:['palette.nes']},{consoleId:'nes',consoleName:'NES',reference:'test',title:'Essai clavier',status:'Unknown',files:['test.nes']}]:[]};
}
export async function listCores(){return {ok:true,data:[{installed:true,name:'fixture-core'}]};}
export async function createSession(...args){window.__launchArgs=args;return {ok:true,data:{sessionId:'preview-session'}};}
