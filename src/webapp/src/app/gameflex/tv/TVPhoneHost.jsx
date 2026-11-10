import React,{createContext,useCallback,useContext,useEffect,useRef,useState} from 'react';
import {useLocation,useNavigate} from 'react-router-dom';
import {appPath} from '../libs/appBase.js';
import {useAuth} from '../profile/auth/AuthContext.jsx';
import {clearPhonePad,publishPhonePad,hasPhoneGameConsumer,subscribePhoneConsumer} from '../libs/phonePad.js';
import {isGameInputSuspended,setGameInputSuspended} from '../libs/gameInput.js';
import {phoneRequest,phoneSocket} from './phone-pad-api.js';
import {useDialogFocus} from '../experience/dialog-focus.js';
import {useDirectionalFocus} from '../experience/ExperienceView.jsx';
import {Icon} from '../experience/EmulatorUI.jsx';
import './tv-phone.css';
const TVContext=createContext(null);
export const useTVPhone=()=>useContext(TVContext);
function PairDialog({ctx}){
 const root=useRef(null);useDialogFocus(root);useDirectionalFocus(root);
 useEffect(()=>{const previous=isGameInputSuspended();setGameInputSuspended(true);clearPhonePad();return()=>setGameInputSuspended(previous);},[]);
 return <div className="ps-control-backdrop"><section ref={root} className="ps-control gf-tv-pair" role="dialog" aria-modal="true" aria-labelledby="tv-pair-title" onKeyDown={e=>{if(e.key==='Escape'){e.preventDefault();ctx.close();}if(e.key==='Tab'){const all=[...root.current.querySelectorAll('button:not(:disabled),a[href]')];if(e.shiftKey&&document.activeElement===all[0]){e.preventDefault();all.at(-1)?.focus();}else if(!e.shiftKey&&document.activeElement===all.at(-1)){e.preventDefault();all[0]?.focus();}}}}>
 <header><div><small>GAMEFLEX · SUR GRAND ÉCRAN</small><h2 id="tv-pair-title">Votre téléphone devient une manette</h2></div><button aria-label="Fermer l’association" onClick={ctx.close}><Icon name="close"/></button></header>
 {ctx.error&&<p role="alert">{ctx.error}</p>}
 {!ctx.pair?<><p>Ouvrez GameFlex sur le téléviseur, puis associez votre téléphone pour naviguer et jouer.</p><button className="gf-tv-primary" disabled={ctx.busy} onClick={ctx.create}>{ctx.busy?'Création du code…':'Afficher un code'}</button></>:ctx.status.approved?<><div className="gf-tv-connected"><Icon name="consoles" size={40}/><h3>{ctx.status.phoneConnected?'Manette connectée':'Téléphone déconnecté'}</h3><p>{ctx.status.name} · {ctx.status.phoneConnected?'Prêt pour la navigation et les parties.':'Rouvrez la page manette sur le téléphone.'}</p></div><div className="gf-tv-pair-actions"><button className="gf-tv-primary" onClick={ctx.close}>Continuer sur la TV</button><button onClick={ctx.revoke}>Déconnecter le téléphone</button></div></>:ctx.status.pending?<><div className="gf-tv-connected"><Icon name="multiplayer" size={40}/><h3>Autoriser « {ctx.status.name} » ?</h3><p>Ce téléphone pourra naviguer et jouer sur cette TV. Il ne recevra pas vos identifiants.</p></div><div className="gf-tv-pair-actions"><button onClick={ctx.revoke}>Refuser</button><button className="gf-tv-primary" disabled={ctx.busy} onClick={ctx.approve}>Autoriser ce téléphone</button></div></>:<><ol><li>Sur le téléphone, ouvrez cette adresse :</li></ol><p className="gf-tv-phone-address">{location.origin+appPath('/controller')}</p><p>Saisissez le code affiché ici.</p><div className="gf-tv-code" aria-label={`Code d’association ${ctx.pair.code}`}>{ctx.pair.code.slice(0,4)} <span>{ctx.pair.code.slice(4)}</span></div><p className="gf-tv-countdown" role="status">Code valable {ctx.remaining} s · validation sur la TV requise</p><button disabled={ctx.busy} onClick={ctx.create}>Générer un nouveau code</button></>}
 <footer className="gf-tv-display-tools"><button aria-pressed={ctx.tv} onClick={()=>ctx.setTV(!ctx.tv)}>{ctx.tv?'Mode TV activé':'Activer le mode TV'}</button><button onClick={()=>Promise.resolve(document.documentElement.requestFullscreen?.()).catch(()=>{})}>Plein écran</button></footer>
 </section></div>;
}
export function TVPhoneSettings(){const ctx=useTVPhone();if(!ctx)return null;return <section className="gf-tv-settings"><h3>Télévision et manette téléphone</h3><p>Grandes commandes, marges TV et navigation à la télécommande. Le téléphone sert de manette ; la partie reste à l’écran.</p><button aria-pressed={ctx.tv} onClick={()=>ctx.setTV(!ctx.tv)}>{ctx.tv?'Désactiver le mode TV':'Activer le mode TV'}</button><button onClick={ctx.open}>Associer mon téléphone</button></section>;}
export function TVPhoneButton(){const ctx=useTVPhone();return ctx?<button className="gf-tv-phone-button" aria-label="Manette téléphone" title="Manette téléphone" onClick={ctx.open}><svg viewBox="0 0 24 24" width="23" height="23" fill="none" stroke="currentColor" strokeWidth="1.7"><rect x="6" y="2" width="12" height="20" rx="3"/><path d="M10 5h4m-2 11v4m-2-2h4M9 10h.01M15 10h.01"/></svg></button>:null;}
export default function TVPhoneHost({children}){
 const {token}=useAuth()||{},route=useLocation(),navigate=useNavigate();
 const [tv,setTVState]=useState(()=>{try{return new URLSearchParams(location.search).get('tv')==='1'||localStorage.getItem('gameflex:tv-mode')==='1';}catch{return false;}});
 const [open,setOpen]=useState(false),[pair,setPair]=useState(null),[status,setStatus]=useState({}),[error,setError]=useState(''),[busy,setBusy]=useState(false),[remaining,setRemaining]=useState(120),[consumer,setConsumer]=useState(hasPhoneGameConsumer);
 const socket=useRef(null),pairRef=useRef(null),lastInput=useRef(0),latest=useRef({}),navState=useRef(new Set()),epoch=useRef(0),repeat=useRef({});
 const streaming=route.pathname==='/streaming',screen=open?'library':streaming?(consumer?'game':'unsupported'):'library';latest.current={open,streaming,screen};pairRef.current=pair;
 const setTV=value=>{setTVState(value);try{localStorage.setItem('gameflex:tv-mode',value?'1':'0');}catch{}};
 useEffect(()=>{document.body.classList.toggle('gf-tv-mode',tv);return()=>document.body.classList.remove('gf-tv-mode');},[tv]);
 useEffect(()=>subscribePhoneConsumer(()=>setConsumer(hasPhoneGameConsumer())),[]);
 const revoke=useCallback(async()=>{const current=pairRef.current;if(current)try{await phoneRequest(`/${current.id}/revoke`,{hostToken:current.hostToken},token);}catch{}socket.current?.close();clearPhonePad();setPair(null);setStatus({});},[token]);
 const create=async()=>{if(busy||!token)return;setBusy(true);setError('');await revoke();try{const value=await phoneRequest('/create',{},token);setRemaining(120);setPair({...value,createdAt:Date.now()});setStatus({});}catch(e){setError(e.message);}finally{setBusy(false);}};
 const approve=async()=>{if(!pair||busy)return;setBusy(true);try{await phoneRequest(`/${pair.id}/approve`,{hostToken:pair.hostToken},token);}catch(e){setError(e.message);}finally{setBusy(false);}};
 useEffect(()=>{
  if(!pair||!token)return;let stopped=false,timer=null;const reconnect=()=>{if(stopped||timer)return;timer=setTimeout(()=>{timer=null;connect();},1500);};const connect=()=>{
   const ws=phoneSocket(pair.id);socket.current=ws;ws.onopen=()=>{ws.send(JSON.stringify({role:'host',token:pair.hostToken,accessToken:token}));};
   ws.onmessage=event=>{const value=JSON.parse(event.data);if(value.type==='status'){epoch.current=value.epoch;setStatus(value);setError('');if(!value.phoneConnected){clearPhonePad();navState.current.clear();}if(value.screen!==latest.current.screen)ws.send(JSON.stringify({type:'context',screen:latest.current.screen}));}
    if(value.type==='closed'){stopped=true;clearPhonePad();setPair(null);setStatus({});setError('Association terminée ou expirée. Affichez un nouveau code.');ws.close();}
    if(value.type==='pad'&&value.epoch===epoch.current){lastInput.current=Date.now();const previous=navState.current;navState.current=new Set(value.buttons);if(value.buttons.includes(16)&&!previous.has(16)){clearPhonePad();setOpen(value=>!value);return;}if(latest.current.screen==='game')publishPhonePad(value);else{clearPhonePad();if(latest.current.screen==='library'){
     const surface=document.querySelector('[aria-modal="true"]')||document.querySelector('.ps-console');if(!surface)return;let target=document.activeElement;if(!surface.contains(target)){target=surface.querySelector('button:not(:disabled)');target?.focus();}
     for(const [index,key] of [[12,'ArrowUp'],[13,'ArrowDown'],[14,'ArrowLeft'],[15,'ArrowRight']])if(value.buttons.includes(index)&&(!previous.has(index)||Date.now()-(repeat.current[index]||0)>220)){target?.dispatchEvent(new KeyboardEvent('keydown',{key,bubbles:true}));repeat.current[index]=Date.now();}
     if(value.buttons.includes(0)&&!previous.has(0))target?.click();if(value.buttons.includes(1)&&!previous.has(1)){const close=surface.querySelector('button[aria-label^="Fermer"]');if(close)close.click();else navigate('/?section=home');}
    }}}};
   ws.onclose=event=>{clearPhonePad();navState.current.clear();if(event.code===1008){stopped=true;setPair(null);setStatus({});setError('Association terminée. Affichez un nouveau code.');}if(!stopped){setStatus(value=>({...value,hostConnected:false,phoneConnected:false}));reconnect();}};
   ws.onerror=()=>setError('Connexion à la manette indisponible. Réessayez.');
  };connect();const heartbeat=setInterval(()=>{if(socket.current?.readyState===1)socket.current.send('{"type":"ping"}');else if(socket.current?.readyState===3)reconnect();if(Date.now()-lastInput.current>1200){clearPhonePad();navState.current.clear();}},5000);
  return()=>{stopped=true;clearTimeout(timer);clearInterval(heartbeat);socket.current?.close();clearPhonePad();};
 },[pair?.id,token]);
 useEffect(()=>{clearPhonePad();navState.current.clear();if(socket.current?.readyState===1)socket.current.send(JSON.stringify({type:'context',screen}));},[route.pathname,route.search,screen]);
 useEffect(()=>{if(!pair)return;const timer=setInterval(()=>{setRemaining(Math.max(0,120-Math.floor((Date.now()-pair.createdAt)/1000)));},1000);return()=>clearInterval(timer);},[pair]);
 useEffect(()=>()=>{const current=pairRef.current;if(current&&token)phoneRequest(`/${current.id}/revoke`,{hostToken:current.hostToken},token).catch(()=>{});clearPhonePad();},[token]);
 useEffect(()=>{if(!tv||streaming)return;const timer=setTimeout(()=>{if(document.activeElement===document.body)document.querySelector('.ps-console-tile[aria-pressed="true"],.ps-console button')?.focus();},200);return()=>clearTimeout(timer);},[tv,streaming,route.pathname]);
 const ctx={tv,setTV,pair,status,error,busy,remaining,create,approve,revoke,open:()=>{setOpen(true);setError('');},close:()=>setOpen(false)};
 return <TVContext.Provider value={ctx}>{children}{token&&streaming&&(tv||pair)&&<button className="gf-tv-stream-phone" onClick={()=>setOpen(true)}>Manette téléphone{status.phoneConnected?' · connectée':''}</button>}{open&&<PairDialog ctx={ctx}/>}</TVContext.Provider>;
}
