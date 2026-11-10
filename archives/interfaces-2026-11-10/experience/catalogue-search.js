export const foldSearch=value=>String(value||'').normalize('NFKD').replace(/[\u0300-\u036f]/g,'').toLowerCase().trim().replace(/\s+/g,' ');
export function matchesGameSearch(game,query){
    const words=foldSearch(query).split(' ').filter(Boolean);
    if(!words.length)return true;
    const texts=[game.title,game.consoleName,...(game.variants||[]).map(v=>v.title)].map(foldSearch);
    return texts.some(text=>words.every(word=>text.includes(word)));
}
const pages=new Map();
const TTL=5*60*1000,MAX_PAGES=128;
const alphabet='0ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('');
const cacheKey=(consoleId,source,letter)=>`${consoleId}:${source}:${letter}`;
function cached(key){const page=pages.get(key);return page&&Date.now()-page.at<TTL?page.value:null;}
function store(key,value){pages.delete(key);pages.set(key,{at:Date.now(),value});while(pages.size>MAX_PAGES)pages.delete(pages.keys().next().value);}
export function clearCatalogueSearchCache(){pages.clear();}
// Requested-console metadata only. One letter worker keeps connections available for visible artwork; abort stops queued work.
export async function loadConsoleCatalogue({consoleId,letter,query='',signal,request,onUpdate,enabledSourceIds=['emugen','planetemu','freeroms','romspure']}){
    const needle=foldSearch(query),searching=Boolean(needle),sources=['planetemu','freeroms','romspure'].filter(id=>enabledSourceIds.includes(id));
    const first=needle.normalize('NFKD').toUpperCase().match(/[A-Z0-9]/)?.[0];
    const priority=first&&/[0-9]/.test(first)?'0':first;
    const queue=searching||!letter?[...new Set([priority,letter,...alphabet].filter(Boolean))]:[letter];
    const total=queue.length,results=new Map(),unavailable=new Set();let completed=0,error='';
    if(searching||!letter){for(const char of alphabet)for(const source of sources){const value=cached(cacheKey(consoleId,source,char)+(source==='romspure'&&searching?':'+needle:''));if(value)results.set(source+':'+char+':1',value);}}
    const publish=()=>{if(!signal?.aborted)onUpdate({items:[...results.values()].flatMap(p=>p.items),query:needle,searching,loading:completed<total,searchCompleted:completed,searchTotal:total,...(error?{error}:{})});};
    publish();
    async function read(source,char,page=1){
        const key=cacheKey(consoleId,source,char)+(['romspure','emugen'].includes(source)&&searching?':'+needle:'')+(page>1?':page'+page:'');let value=cached(key);
        if(!value&&!unavailable.has(source)){
            try{
                const data=await request(source,char,signal,['romspure','emugen'].includes(source)?needle:'',source==='emugen'?page:undefined);
                if(signal?.aborted)return;
                if(!Array.isArray(data.items)||data.error)throw new Error('catalogue unavailable');
                value={nextPage:data.nextPage,items:data.items.map(game=>({...game,provider:source==='emugen'?'EmuGen':source==='freeroms'?'FreeROMs':source==='romspure'?'ROMsPure':'Planetemu'}))};store(key,value);
            }catch(e){if(signal?.aborted||e.name==='AbortError')return;unavailable.add(source);error='Une source est momentanément inaccessible. Les résultats disponibles restent affichés.';}
        }
        if(value)results.set(source+':'+char+':'+page,value);
        return Number.isInteger(value?.nextPage)&&value.nextPage>page&&value.nextPage<=1000?value.nextPage:null;
    }
    async function worker(){
        while(queue.length&&!signal?.aborted){const char=queue.shift();const sourceQueue=[...sources];const readNext=async()=>{while(sourceQueue.length&&!signal?.aborted)await read(sourceQueue.shift(),char);};await Promise.all([readNext(),readNext()]);if(signal?.aborted)return;completed++;publish();}
    }
    // EmuGen is asked first once per search. Secondary providers keep their existing order.
    if(enabledSourceIds.includes('emugen')){let page=1;do{page=await read('emugen',searching?(priority||letter):letter,page);if(signal?.aborted)return;publish();}while(page);}
    if(signal?.aborted)return;
    if(!sources.length){completed=total;publish();return;}
    publish();
    await Promise.all(Array.from({length:Math.min(1,total)},worker));
}
