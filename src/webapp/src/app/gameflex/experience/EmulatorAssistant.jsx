import React,{useEffect,useState} from 'react';
import {apiUrl} from '../libs/serverBase.js';
import {authHeaders,useAuth} from '../profile/auth/AuthContext.jsx';
export async function emulatorRequest(token,path='',method='GET'){
 const r=await fetch(apiUrl('/experience-api/v1/emulators'+path),{method,headers:authHeaders(token),cache:'no-store'}),v=await r.json();if(!r.ok)throw new Error(v.error||'Émulateurs indisponibles.');return v;
}
export function useEmulators(token){
 const [state,setState]=useState(null),[error,setError]=useState(''),[revision,setRevision]=useState(0);
 useEffect(()=>{if(!token)return;let stopped=false,timer;const read=async()=>{try{const v=await emulatorRequest(token);if(!stopped){setState(v);setError('');timer=setTimeout(read,v.items.some(i=>i.job?.status==='running')?1000:15000);}}catch(e){if(!stopped)setError(e.message);}};read();return()=>{stopped=true;clearTimeout(timer);};},[token,revision]);
 const install=async id=>{setError('');try{await emulatorRequest(token,'/'+id+'/install','POST');setRevision(x=>x+1);}catch(e){setError(e.message);}};
 return {state,error,install,refresh:()=>setRevision(x=>x+1)};
}
export async function refreshCoreListing(token){const r=await fetch(apiUrl('/api/v1/retrogaming/listing/rescan?section=cores'),{method:'POST',headers:authHeaders(token)});if(!r.ok)throw new Error('Core installé. Rechargez la bibliothèque pour le détecter.');}
export function EmulatorRequirement({consoleId,token,onReady,onStatus}){
 const {state,error,install}=useEmulators(token),item=state?.items.find(x=>x.consoleId===consoleId);
 const [syncError,setSyncError]=useState('');
 useEffect(()=>{if(item?.core)onStatus?.(item.installed);},[item?.installed,item?.core,onStatus]);
 useEffect(()=>{if(item?.job?.status==='succeeded')refreshCoreListing(token).then(()=>onReady?.()).catch(e=>setSyncError(e.message));},[item?.job?.status,token]);
 if(!item)return error?<p role="alert">{error}</p>:null;
 return <section className="gf-emulator-requirement"><p>{item.installed?`${item.name} prêt`:`${item.name} manquant · émulation sur le serveur`}</p>{(error||syncError)&&<p role="alert">{error||syncError}</p>}{item.job?.status==='failed'&&<p role="alert">{item.job.message}</p>}{item.job?.status==='running'?<p role="status">{item.job.message}</p>:!item.installed&&item.canInstall&&state.canInstall?<button onClick={()=>install(consoleId)}>Installer {item.name}</button>:!item.installed?<small>Configuration manuelle nécessaire pour cette console.</small>:null}</section>;
}
export default function EmulatorAssistant(){
 const {token}=useAuth(),{state,error,install}=useEmulators(token);
 return <section className="gf-emulator-assistant"><h3>Émulateurs de votre bibliothèque</h3>{state&&<small>Serveur · {state.hardware.platform||state.hardware.architecture} · {state.hardware.ramGiB} Go</small>}{error&&<p role="alert">{error}</p>}{!state&&<p>Analyse…</p>}{state?.items.map(item=><article key={item.consoleId}><div><strong>{item.name}</strong><small>{item.consoleId.toUpperCase()} · {item.games} jeux · {item.installed?'Installé':'À installer'}</small>{item.job&&<p role="status">{item.job.message}</p>}</div>{!item.installed&&item.canInstall&&state.canInstall&&<button disabled={item.job?.status==='running'} onClick={()=>install(item.consoleId)}>{item.job?.status==='running'?'Installation…':'Installer'}</button>}{!item.installed&&!item.canInstall&&<small>Configuration manuelle</small>}</article>)}</section>;
}
