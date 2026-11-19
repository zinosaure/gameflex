// Receives bootstrap credentials on stdin; never logs credentials, tokens or raw replies.
const fs=require('fs');
const WebSocket=require('/app/src/webapp/src/node_modules/ws');
const config=JSON.parse(fs.readFileSync(0,'utf8'));
const base='http://gameflex-web:8080';
async function main(){
 const login=await fetch(base+'/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:config.username,password:config.password})});
 if(!login.ok)throw new Error('login '+login.status);
 const identity=await login.json(),token=identity.item.access_token;
 const headers={Authorization:'Bearer '+token};
 const report={authentication:login.status,username:identity.item.user.username,role:identity.item.user.role_level};
 async function get(path){const r=await fetch(base+path,{headers});if(!r.ok)throw new Error('HTTP '+r.status+' for '+path);return r.json()}
 const consoles=await get('/api/v1/consoles');report.consoles=(Array.isArray(consoles)?consoles:consoles.data).length;
 const roms=await get('/api/v1/retrogaming/nes/roms');const demo=(roms.data||[]).find(x=>x.title.toLowerCase().includes('palette'));report.demoListed=Boolean(demo);
 let n64Target;
 if(config.testN64){
  const n64Roms=await get('/api/v1/retrogaming/n64/roms'),n64Cores=await get('/api/v1/retrogaming/n64/cores');
  const game=(n64Roms.data||[])[0],core=(n64Cores.data||[])[0];
  if(!game||!core)throw new Error('N64 ROM or core unavailable');
  n64Target={consoleId:'n64',reference:game.reference,title:game.title,core:core.name};report.n64Ready=true;
 }
 const prefs=await get('/experience-api/v1/preferences');report.preferenceEndpoint=200;report.initialInterface=prefs.interface;
 if(config.testCatalogue){const catalogue=await get('/experience-api/v1/planetemu/gbc?letter=A');report.planetemuTitles=catalogue.items.length;report.metadataOnly=catalogue.metadataOnly;}
 if(config.testFilter){
  const library=await get('/experience-api/v1/library');report.librarySetupRequired=!library.configured;
  if(!library.configured||!library.folder.endsWith('/roms')||library.organization!=='<console>/<jeu>')throw new Error('Default ROM folder mismatch');report.romsDefault=true;
  const sources=await get('/experience-api/v1/game-sources');const freeroms=sources.items.find(x=>x.id==='freeroms');if(!freeroms||freeroms.downloads!==true)throw new Error('External source capability mismatch');report.freeromsSource='catalogue-et-fichiers-directs';
  const external=await get('/experience-api/v1/freeroms/gba?letter=P');if(!external.items?.length)throw new Error('FreeROMs metadata unavailable');report.freeromsTitles=external.items.length;const externalImage=external.items.find(x=>x.downloadId==='freeroms:gba:pokemon_emerald_version_ue');if(externalImage){const art=await fetch(base+externalImage.image);if(!art.ok||!art.headers.get('content-type')?.startsWith('image/'))throw new Error('FreeROMs artwork unavailable');await art.arrayBuffer();report.freeromsImage=art.status;}
  const rescan=await fetch(base+'/api/v1/retrogaming/listing/rescan?section=roms',{method:'POST',headers});if(!rescan.ok)throw new Error('Library rescan unavailable');await rescan.arrayBuffer();report.rescanStatus=rescan.status;
  const roots=await get('/experience-api/v1/library/folders');if(!Array.isArray(roots.items))throw new Error('Folder browser unavailable');if(roots.items.length){const first=await get('/experience-api/v1/library/folders?folder='+encodeURIComponent(roots.items[0].folder));if(!first.selectable||first.items.some(x=>!x.folder))throw new Error('Folder root invalid');report.folderBrowser={roots:roots.items.length,folders:first.items.length};}
  const escape=await fetch(base+'/experience-api/v1/library/folders?folder='+encodeURIComponent('/etc'),{headers});if(escape.status!==400)throw new Error('Folder escape not blocked');
  const downloads=await get('/experience-api/v1/downloads');report.downloadHistoryAccessible=Array.isArray(downloads.items);
  const approved=await get('/experience-api/v1/download-catalogue?console=nes');if(!approved.items.some(x=>x.downloadId==='palette-libre'&&x.license==='CC0-1.0'))throw new Error('Free catalogue missing');
  const denied=await fetch(base+'/experience-api/v1/downloads',{method:'POST',headers:{...headers,'Content-Type':'application/json'},body:JSON.stringify({gameId:'unapproved-game'})});if(denied.status!==409)throw new Error('Unapproved download not blocked');report.unapprovedDownloadBlocked=true;
  const media=await get('/experience-api/v1/media');
  const photos=media.items.filter(x=>x.approved&&x.kind==='console-photo');
  if(photos.length!==33)throw new Error('Console photo registry incomplete');
  await Promise.all(photos.map(async photo=>{const r=await fetch(base+photo.url);if(!r.ok||!r.headers.get('content-type')?.startsWith('image/'))throw new Error('Console photo unavailable');await r.arrayBuffer()}));
  report.consolePhotos=photos.length;
  const counts={};for(const letter of ['A','M']){const catalogue=await get('/experience-api/v1/planetemu/nes?letter='+letter);if(!catalogue.items.length||catalogue.items.some(g=>g.title.normalize('NFKD').toUpperCase().match(/[A-Z0-9]/)?.[0]!==letter))throw new Error('Letter filter mismatch');counts[letter]=catalogue.items.length;
   if(letter==='M'){const item=catalogue.items.find(x=>x.title==='M.C. Kids (USA)');if(!item?.image)throw new Error('Cover missing');const r=await fetch(base+item.image);if(!r.ok||!r.headers.get('content-type')?.startsWith('image/'))throw new Error('Cover endpoint unavailable');report.coverStatus=r.status;report.coverMime=r.headers.get('content-type');}
  }report.letterCounts=counts;report.letterFilterValid=true;
 }
 if(config.testFilter){
  const started=performance.now();const catalogue=await get('/experience-api/v1/planetemu/gba?letter=P');report.gbaCatalogueMs=Math.round(performance.now()-started);
  const examples=catalogue.items.filter(x=>x.title.startsWith('Paard & Pony')||x.title.startsWith('Pac-Man Collection'));
  if(examples.length<6||examples.some(x=>x.title.endsWith('Retirée')))throw new Error('GBA title cleanup failed');
  const first=performance.now();const sizes=await Promise.all(examples.slice(0,8).map(async item=>{const r=await fetch(base+item.image);if(!r.ok||!r.headers.get('content-type')?.includes('image/webp'))throw new Error('GBA cover unavailable');return (await r.arrayBuffer()).byteLength}));
  report.gbaCovers={count:sizes.length,parallelMs:Math.round(performance.now()-first),totalBytes:sizes.reduce((a,b)=>a+b,0)};
 }
 if((config.testLaunch && demo) || config.testN64 || config.stopExisting){
  const ws=new WebSocket(base.replace('http','ws')+'/api/v1/facade/ws?token='+encodeURIComponent(token));
  let next=0;const pending=new Map();
  const guard=setTimeout(()=>{ws.terminate();console.log(JSON.stringify({...report,verification:'launch timeout'}));process.exit(1)},45000);
  ws.on('message',raw=>{const f=JSON.parse(raw.toString());if(f.type==='reply'&&pending.has(f.id)){pending.get(f.id)(f);pending.delete(f.id)}});
  await new Promise((resolve,reject)=>{ws.once('open',resolve);ws.once('error',()=>reject(new Error('websocket unavailable')));setTimeout(()=>reject(new Error('websocket timeout')),15000).unref()});
  function cmd(type,data){const id=String(++next);return new Promise((resolve,reject)=>{pending.set(id,resolve);ws.send(JSON.stringify({type,id,data}));setTimeout(()=>{if(pending.has(id)){pending.delete(id);reject(new Error('command timeout'))}},90000).unref()})}
  if(config.stopExisting){
   const lookup=await fetch(base+'/api/v1/sessions/'+encodeURIComponent(config.stopExisting),{headers});
   if(lookup.status===404){clearTimeout(guard);ws.terminate();console.log(JSON.stringify({...report,stopAlreadyAbsent:true}));setTimeout(()=>process.exit(0),100);return;}
   if(!lookup.ok)throw new Error('Session check refused');
   const session=await lookup.json();
   if(session.data?.consoleId!=='nes'||session.data?.reference!==demo?.reference)throw new Error('Refus : cette session ne correspond pas à la démo générée.');
   const stopped=await cmd('session.stop',{platform:'retrogaming',sessionId:config.stopExisting});
   clearTimeout(guard);ws.terminate();console.log(JSON.stringify({...report,stopStatus:stopped.status}));setTimeout(()=>process.exit(stopped.status===200?0:1),100);return;
  }
  const target=config.testN64?n64Target:{consoleId:'nes',reference:demo.reference,title:demo.title,core:'quicknes'};
  const result=await cmd('launch',{platform:'retrogaming',...target,bios:'',resolution:'sd',language:'fr'});
  report.launchStatus=result.status;
  console.log(JSON.stringify({launchStatus:result.status,answerKeys:Object.keys(result.data||{}),dataKeys:Object.keys(result.data?.data||{})}));
  if(result.status>=200&&result.status<300){const data=result.data?.data||result.data;const sessionId=data.sessionId||data.session_id;if(sessionId && config.keepLaunch){console.log(JSON.stringify({sessionId}));}else if(sessionId){let stream;if(config.testN64){stream=new WebSocket(base.replace('http','ws')+'/api/v1/retrogaming/stream/ws?token='+encodeURIComponent(token));await new Promise((resolve,reject)=>{const timer=setTimeout(()=>reject(new Error('N64 stream startup timeout')),15000);stream.once('open',()=>stream.send(JSON.stringify({type:'start',sessionId,username:identity.item.user.username})));stream.on('message',raw=>{const message=JSON.parse(raw.toString());if(message.type==='started'){clearTimeout(timer);resolve();}else if(message.type==='error'){clearTimeout(timer);reject(new Error('N64 stream refused'));}});stream.once('close',()=>{clearTimeout(timer);reject(new Error('N64 stream closed during startup'));});stream.once('error',()=>{clearTimeout(timer);reject(new Error('N64 stream unavailable'));});});report.n64StreamStarted=true;}await new Promise(r=>setTimeout(r,config.testN64?15000:3000));if(config.testN64){const alive=await fetch(base+'/api/v1/sessions/'+encodeURIComponent(sessionId),{headers});if(!alive.ok)throw new Error('N64 session ended before smoke-test deadline');report.n64AliveAfterSeconds=15;stream?.close();}const stop=await cmd('session.stop',{platform:'retrogaming',sessionId});report.stopStatus=stop.status}}
  else report.launchError='Le serveur a refusé le lancement.';
  clearTimeout(guard);ws.terminate();
 }
 console.log(JSON.stringify(report));
 setTimeout(()=>process.exit(0),100);
}
main().catch(e=>{console.log(JSON.stringify({verification:'failed',reason:e.message.replace(/token=[^\s]+/g,'token=REDACTED')}));process.exit(1)});
