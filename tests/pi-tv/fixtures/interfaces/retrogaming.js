import showcase from './showcase.json';
export async function listRoms(consoleId) {
    const config=window.__previewConfig||{};
    await new Promise(resolve=>setTimeout(resolve,config.romDelay??450));
    if(config.libraryError)return {ok:false,data:[]};
    if(config.empty)return {ok:true,data:[]};
    let items;
    if(consoleId==='gba')items=Array.from({length:config.large?432:4},(_,i)=>({consoleId:'gba',consoleName:'Game Boy Advance',reference:`gba-${i}`,title:i===0?'Advance Wars (Europe) (En,Fr).gba':i===1?'Advance Wars 2 - Black Hole Rising (USA) (En).gba':`Jeu de test ${i+1}`,files:[`fixture-${i}.gba`]}));
    else if(consoleId==='n64')items=Array.from({length:24},(_,i)=>({consoleId:'n64',consoleName:'Nintendo 64',reference:`fixture-${i+1}`,title:['007 — GoldenEye','Banjo-Kazooie','Diddy Kong Racing','F-Zero X','Mario Kart 64','Perfect Dark','Pokémon Stadium','Star Fox 64','Super Mario 64','The Legend of Zelda'][i%10]+(i>9?` · ${i+1}`:''),files:[`fixture-${i+1}.z64`]}));
    else items=consoleId==='nes'?[{consoleId:'nes',consoleName:'NES',reference:'palette',title:'Palette libre',files:['palette.nes']},{consoleId:'nes',consoleName:'NES',reference:'test',title:'Essai clavier',files:['test.nes']}]:[];
    if(config.showcase)items=consoleId==='gba'?showcase:[];
    items=items.map((g,i)=>({...g,status:config.objectStatus?{status:i%2?'untested':'playable',emulators:{fixture:'playable'}}:i%2?'Unknown':'Complete'}));
    if(config.faultOnce&&consoleId==='gba'){config.faultOnce=false;items[0].year={invalid:true};}
    return {ok:true,data:items};
}
export async function listCores(){return {ok:true,data:[{installed:true,name:'fixture-core'}]};}
export async function createSession(...args){window.__launchArgs=args;return {ok:true,data:{sessionId:'preview-session'}};}
