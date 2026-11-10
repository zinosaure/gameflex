// Real provider, routing, shared views and dialogs with an entirely local fixture
// transport. No request, preference write or launch reaches the public instance.
import React from 'react';
import consolePhotos from './fixtures/interfaces/console-photos.json';
import Settings from '../../src/webapp/src/app/gameflex/profile/Settings.jsx';
import Skeleton from '../../src/webapp/src/app/skeleton/skeleton.js';
import '../../src/webapp/src/app/i18n/index.js';
import 'bootstrap/dist/css/bootstrap.min.css';
import RouteErrorBoundary from '../../src/webapp/src/app/gameflex/experience/RouteErrorBoundary.jsx';
import {createRoot} from 'react-dom/client';
import {BrowserRouter,useLocation} from 'react-router-dom';
import {ConsolesProvider} from '../../src/webapp/src/app/gameflex/contexts/ConsolesContext.jsx';
import {ExperienceProvider,ExperienceGate} from '../../src/webapp/src/app/gameflex/experience/ExperienceProvider.jsx';
import {InterfacePicker} from '../../src/webapp/src/app/gameflex/experience/ExperienceView.jsx';
import {selectableInterfaces} from '../../src/webapp/src/app/gameflex/experience/interfaces.js';
const params=new URLSearchParams(location.search);
let preferences=JSON.parse(localStorage.getItem('preview:preferences')||'null')||{interface:params.get('saved')||'station',favorites:['nes:palette'],history:['nes:palette'],collections:[{id:'test',name:'Jeux de test',games:['nes:palette']}]};
window.__requests=[];
window.__previewConfig={objectStatus:!params.has('legacyStatus'),large:params.get('scenario')==='large',romDelay:Number(params.get('romDelay')||450),consoleDelay:Number(params.get('consoleDelay')||0),preferenceDelay:Number(params.get('preferenceDelay')||0),preferenceOffline:params.has('preferenceOffline'),libraryError:params.has('libraryError'),empty:params.has('empty'),faultOnce:params.has('faultOnce')};
const config=window.__previewConfig;config.showcase=params.has('showcase');config.missingEmulator=params.has('missingEmulator');config.launchFailure=params.has('launchFailure');config.readOnly=params.has('readOnly');config.emulatorOffline=params.has('emulatorOffline');

window.fetch=async(url,options={})=>{
 const path=new URL(url,location.origin).pathname.replace(/^\/gameflex/,'');
 window.__requests.push({path,method:options.method||'GET'});
 let value={items:[]};
 if(path.endsWith('/preferences')){
  const snapshot=JSON.parse(JSON.stringify(preferences));
  if(config.preferenceDelay)await new Promise(resolve=>setTimeout(resolve,config.preferenceDelay));
  if(config.preferenceOffline)throw new Error('Offline fixture');
  if(options.method==='PUT'){preferences=JSON.parse(options.body);localStorage.setItem('preview:preferences',options.body);value=preferences;}else value=snapshot;
 }
 else if(path.endsWith('/consoles')){if(config.consoleDelay)await new Promise(resolve=>setTimeout(resolve,config.consoleDelay));value={data:[{id:'nds',name:'Nintendo DS',year:2004,publisher:'Nintendo',method:'retrogaming',maxPlayers:2,enabled:true},{id:'nes',name:'NES',method:'retrogaming',maxPlayers:2,enabled:true},{id:'n64',name:'Nintendo 64',manufacturer:'Nintendo',year:1996,method:'retrogaming',maxPlayers:4,enabled:true},{id:'gba',name:'Game Boy Advance',year:2001,publisher:'Nintendo',method:'retrogaming',maxPlayers:2,enabled:true}]};}
 else if(path.endsWith('/media')&&config.showcase)value={items:consolePhotos};
 else if(path.endsWith('/game-sources'))value={enabledIds:[],items:[],ready:true,canConfigure:false};
 else if(path.endsWith('/library'))value={configured:true,canConfigure:false};
 else if(path.endsWith('/covers/search'))value={items:(JSON.parse(options.body||'{}').items||[]).map(item=>({...item,image:`/experience-api/v1/covers/${item.consoleId}/fixture`,attribution:'Libretro Thumbnails · test'}))};
 else if(path.endsWith('/emulators/gba/install')){window.__emuInstalled=true;value={item:{status:'succeeded'}};}
 else if(path.endsWith('/emulators')&&config.emulatorOffline)throw new Error('Service de test indisponible');
 else if(path.endsWith('/emulators'))value={items:['nes','gba','nds','n64'].map(consoleId=>({consoleId,name:'Émulateur de test',installed:!(config.missingEmulator&&consoleId==='gba'&&!window.__emuInstalled),core:'fixture-core',canInstall:true,...(window.__emuInstalled&&consoleId==='gba'?{job:{status:'succeeded',message:'Installation terminée',id:'fixture-install'}}:{})})),hardware:{},canInstall:!config.readOnly};
 return new Response(JSON.stringify(value),{headers:{'Content-Type':'application/json'}});
};
function Fallback(){const loc=useLocation();if(loc.pathname==='/settings')return <Skeleton.context.Provider value={{offcanvas:{open:()=>{}},toast:{open:()=>{}}}}><Settings/></Skeleton.context.Provider>;return <p data-testid="stream-route">{loc.pathname} {loc.search}</p>;}
const pack=params.has('pack');
createRoot(document.getElementById('root')).render(<><aside style={{background:'#293748',color:'white',textAlign:'center',padding:8}}>PRÉVISUALISATION ISOLÉE · données et lancement simulés</aside>{pack?<InterfacePicker options={selectableInterfaces} onChoose={()=>{}}/>:<BrowserRouter basename="/gameflex"><ConsolesProvider><ExperienceProvider><RouteErrorBoundary><ExperienceGate enabled><Fallback/></ExperienceGate></RouteErrorBoundary></ExperienceProvider></ConsolesProvider></BrowserRouter>}</>);
