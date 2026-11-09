// Real provider, routing, shared views and dialogs with an entirely local fixture
// transport. No request, preference write or launch reaches the public instance.
import React from 'react';
import {createRoot} from 'react-dom/client';
import {BrowserRouter,useLocation} from 'react-router-dom';
import {ConsolesProvider} from '../../src/webapp/src/app/gameflex/contexts/ConsolesContext.jsx';
import {ExperienceProvider,ExperienceGate} from '../../src/webapp/src/app/gameflex/experience/ExperienceProvider.jsx';
import {InterfacePicker} from '../../src/webapp/src/app/gameflex/experience/ExperienceView.jsx';
import {selectableInterfaces} from '../../src/webapp/src/app/gameflex/experience/interfaces.js';
const params=new URLSearchParams(location.search);
let preferences=JSON.parse(localStorage.getItem('preview:preferences')||'null')||{interface:params.get('saved')||'station',favorites:['nes:palette'],history:['nes:palette'],collections:[{id:'test',name:'Jeux de test',games:['nes:palette']}]};
window.__requests=[];
window.fetch=async(url,options={})=>{
 const path=new URL(url,location.origin).pathname.replace(/^\/gameflex/,'');
 window.__requests.push({path,method:options.method||'GET'});
 let value={items:[]};
 if(path.endsWith('/preferences')){if(options.method==='PUT'){preferences=JSON.parse(options.body);localStorage.setItem('preview:preferences',options.body);}value=preferences;}
 else if(path.endsWith('/consoles'))value={data:[{id:'nes',name:'NES',method:'retrogaming',maxPlayers:2,enabled:true}]};
 else if(path.endsWith('/game-sources'))value={enabledIds:[],items:[],ready:true,canConfigure:false};
 else if(path.endsWith('/library'))value={configured:true,canConfigure:false};
 else if(path.endsWith('/emulators'))value={items:[{consoleId:'nes',name:'Émulateur de test',installed:true,core:'fixture-core'}],hardware:{},canInstall:false};
 return new Response(JSON.stringify(value),{headers:{'Content-Type':'application/json'}});
};
function Fallback(){const loc=useLocation();return <p data-testid="stream-route">{loc.pathname} {loc.search}</p>;}
const pack=params.has('pack');
createRoot(document.getElementById('root')).render(<><aside style={{background:'#293748',color:'white',textAlign:'center',padding:8}}>PRÉVISUALISATION ISOLÉE · données et lancement simulés</aside>{pack?<InterfacePicker options={[...selectableInterfaces,...Array.from({length:13},(_,i)=>({id:`fixture-${i}`,name:`Interface de test ${i+10}`,note:'Fixture du pack · pas un mode publié'}))]} onChoose={()=>{}}/>:<BrowserRouter basename="/gameflex"><ConsolesProvider><ExperienceProvider><ExperienceGate enabled><Fallback/></ExperienceGate></ExperienceProvider></ConsolesProvider></BrowserRouter>}</>);
