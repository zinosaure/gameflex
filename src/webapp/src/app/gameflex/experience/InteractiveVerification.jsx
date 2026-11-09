import React,{useEffect,useRef,useState} from 'react';
import {createPortal} from 'react-dom';
import {apiUrl} from '../libs/serverBase.js';
import {authHeaders} from '../profile/auth/AuthContext.jsx';
import './interactive-verification.css';
import HttpVncChannel from './HttpVncChannel.js';

export async function verificationRequest(token,path='',options={}){
 const controller=new AbortController(),cancel=()=>controller.abort();
 if(options.signal?.aborted)controller.abort();else options.signal?.addEventListener('abort',cancel,{once:true});
 const timer=setTimeout(cancel,60000);
 try{
  const response=await fetch(apiUrl('/experience-api/v1/emugen/verification'+path),{...options,signal:controller.signal,cache:'no-store',headers:{...authHeaders(token),...(options.body?{'Content-Type':'application/json'}:{})},body:options.body?JSON.stringify(options.body):undefined});
  const value=await response.json();if(!response.ok){const error=new Error(value.error||'Vérification indisponible.');error.status=value.status;error.code=response.status;throw error;}return value;
 }catch(error){if(error.name==='AbortError')throw new Error('Réponse trop lente. Réessayez.');throw error;}finally{clearTimeout(timer);options.signal?.removeEventListener('abort',cancel);}
}
export const verificationLink=info=>`/retrogaming/${encodeURIComponent(info.consoleId)}?emugenGame=${encodeURIComponent(info.slug)}&emugenVerification=${info.id}`;

export default function InteractiveVerification({token,sessionId,gameSlug,consoleId,onClose,onResolved,onSession}){
 const [session,setSession]=useState(null),[busy,setBusy]=useState(false),[status,setStatus]=useState('Chargement de la session…'),[error,setError]=useState(''),[connection,setConnection]=useState('connecting'),[result,setResult]=useState(null),[attempt,setAttempt]=useState(0),[large,setLarge]=useState(false),[finding,setFinding]=useState(false),[now,setNow]=useState(Date.now());
 const sessionUpdate=useRef(onSession);sessionUpdate.current=onSession;
 const screen=useRef(null),rfb=useRef(null),activeId=useRef(sessionId);activeId.current=session?.id||sessionId;
 useEffect(()=>{
  const abort=new AbortController();let timer;
  const read=async()=>{
   try{
    const value=await verificationRequest(token,'/'+activeId.current,{signal:abort.signal});if(abort.signal.aborted)return;
    setSession(value.item);sessionUpdate.current?.(value.item);setError('');
    if(value.item.status==='expired'||['closed','error'].includes(value.item.status)){setStatus('Session expirée. Relancez la vérification.');setConnection('expired');}
    else if(value.item.status==='starting')setStatus('Ouverture du navigateur…');
    else if(value.item.result?.status==='links')setStatus('Liens disponibles.');
    else if(value.item.result?.status==='empty')setStatus('Aucun lien disponible.');
    else setStatus('');
   }catch(e){if(!abort.signal.aborted){setError(e.message);if(e.status==='session_expired')setConnection('expired');}}
   if(!abort.signal.aborted)timer=setTimeout(read,4000);
  };read();const tick=setInterval(()=>setNow(Date.now()),1000);
  return()=>{abort.abort();clearTimeout(timer);clearInterval(tick);};
 },[sessionId,token,attempt]);
 const failed=session?.status==='error';
 const expired=connection==='expired'||session?.status==='expired'||['closed','error'].includes(session?.status)||Boolean(session&&now>=session.expiresAt*1000);
 const readySessionId=session?.status==='ready'?session.id:null;
 useEffect(()=>{
  if(!readySessionId||expired||!screen.current)return undefined;
  let stopped=false,socket,remote,fallbackStarted=false,timer;setConnection('connecting');
  const connect=async()=>{
   try{
    const {default:RFB}=await import('@novnc/novnc');if(stopped)return;
    const attach=channel=>{
      remote=new RFB(screen.current,channel,{shared:true});rfb.current=remote;remote.scaleViewport=true;remote.resizeSession=false;remote.clipViewport=false;
      remote.addEventListener('connect',()=>{clearTimeout(timer);if(!stopped)setConnection('connected');});
      remote.addEventListener('disconnect',()=>{if(!stopped)setConnection('disconnected');});
      remote.addEventListener('credentialsrequired',()=>{remote.disconnect();if(!stopped)setError('Le navigateur privé a refusé la connexion.');});
    };
    const fallback=async()=>{
     if(stopped||fallbackStarted||remote)return;fallbackStarted=true;clearTimeout(timer);
     socket.onclose=null;socket.onerror=null;socket.onmessage=null;socket.close();
     try{
      const channel=await HttpVncChannel.open((body,signal)=>verificationRequest(token,'/'+readySessionId+'/transport',{method:'POST',body,signal}));
      if(stopped){channel.close();return;}socket=channel;attach(channel);channel.start();
     }catch{if(!stopped){setError('Connexion distante impossible. Reconnectez la fenêtre.');setConnection('disconnected');}}
    };
    const url=new URL(apiUrl(`/experience-api/v1/emugen/verification/${readySessionId}/socket`),window.location.href);url.protocol=url.protocol==='https:'?'wss:':'ws:';
    socket=new WebSocket(url,['binary']);socket.binaryType='arraybuffer';
    timer=setTimeout(fallback,5000);
    socket.onopen=()=>socket.send(JSON.stringify({type:'authenticate',token}));
    socket.onmessage=event=>{
     try{
      if(typeof event.data!=='string'||JSON.parse(event.data).type!=='authorized')throw new Error('auth');
      clearTimeout(timer);attach(socket);socket.send(JSON.stringify({type:'viewer_ready'}));
     }catch{socket.close();if(!stopped){setError('Connexion privée refusée. Actualisez la session.');setConnection('disconnected');}}
    };
    socket.onerror=fallback;socket.onclose=fallback;
   }catch{if(!stopped){setError('Impossible de charger la fenêtre distante. Réessayez.');setConnection('disconnected');}}
  };connect();
  return()=>{stopped=true;clearTimeout(timer);rfb.current=null;try{remote?.disconnect();socket?.close();}catch{}};
 },[readySessionId,expired,token,attempt]);
 async function resume(){
  if(!session||busy)return;setBusy(true);setError('');
  try{
   const value=await verificationRequest(token,'/'+session.id+'/resume',{method:'POST',body:{}});setResult(value);
   if(value.status==='session_expired')setConnection('expired');
   if(value.status==='links'||value.status==='empty'){onResolved?.(value);setStatus(value.status==='links'?'Liens disponibles.':'Aucun lien disponible.');}
   else setStatus(value.message||'Termine la vérification dans la fenêtre ci-dessus.');
  }catch(e){setError(e.message);if(e.status==='session_expired')setConnection('expired');}
  finally{setBusy(false);}
 }
 async function restart(){
  if(busy||!(session?.slug||gameSlug))return;setBusy(true);setError('');
  try{
   try{await verificationRequest(token,'/'+activeId.current,{method:'DELETE'});}catch{}
   const value=await verificationRequest(token,'',{method:'POST',body:{slug:session?.slug||gameSlug,consoleId:session?.consoleId||consoleId}});setSession(value.item);setResult(null);setAttempt(x=>x+1);setConnection('connecting');onSession?.(value.item);
   const url=new URL(window.location.href);url.searchParams.set('emugenVerification',value.item.id);url.searchParams.set('emugenGame',value.item.slug);window.history.replaceState(null,'',url);
  }catch(e){setError(e.message);}finally{setBusy(false);}
 }
 async function finish(){
  setBusy(true);try{await verificationRequest(token,'/'+activeId.current,{method:'DELETE'});onClose?.();}catch(e){setError(e.message);}finally{setBusy(false);}
 }
 async function findCookie(){
  const remote=rfb.current;if(!remote||connection!=='connected'||finding)return;
  setFinding(true);
  const chord=code=>{remote.sendKey(0xffe3,'ControlLeft',true);remote.sendKey(code);remote.sendKey(0xffe3,'ControlLeft',false);};
  const pause=()=>new Promise(resolve=>setTimeout(resolve,180));
  try{
   // Browser Find only scrolls/highlights. Cloudflare and the cookie click stay manual.
   chord(0x0066);await pause();if(rfb.current!==remote)return;
   chord(0x0061);for(const letter of 'magic cookie')remote.sendKey(letter.codePointAt(0));
   await pause();if(rfb.current!==remote)return;remote.sendKey(0xff1b);
   setStatus('Cliquez sur Magic Cookie dans la fenêtre.');
  }finally{setFinding(false);}
 }
 const key=code=>rfb.current?.sendKey(code);
 const zoom=direction=>{const remote=rfb.current;if(!remote)return;remote.sendKey(0xffe3,'ControlLeft',true);remote.sendKey(direction>0?0x002b:0x002d);remote.sendKey(0xffe3,'ControlLeft',false);};
 if(typeof document==='undefined')return null;
 const seconds=session?Math.max(0,Math.ceil((session.expiresAt*1000-now)/1000)):0;
 return createPortal(<div className="gf-manual-backdrop" onKeyDown={e=>e.stopPropagation()}><section role="dialog" aria-modal="true" aria-labelledby="gf-manual-title" className={`gf-manual-dialog ${large?'gf-manual-large':''}`}>
  <header><div><small>NAVIGATEUR PRIVÉ</small><h2 id="gf-manual-title">Vérifier EmuGen</h2></div><button onClick={onClose} aria-label="Réduire la fenêtre">×</button></header>
  <p className="gf-manual-instructions">Dans la fenêtre : validez Cloudflare si demandé, puis cliquez sur Magic Cookie.</p>
  <div className="gf-manual-toolbar"><button onClick={findCookie} disabled={connection!=='connected'||expired||finding}>{finding?'Recherche…':'Trouver Magic Cookie'}</button><button onClick={()=>setLarge(x=>!x)}>{large?'Réduire':'Grand écran'}</button><button onClick={()=>zoom(1)} aria-label="Zoomer la page distante">Zoom +</button><button onClick={()=>zoom(-1)} aria-label="Dézoomer la page distante">Zoom −</button><button onClick={()=>key(0xff55)} aria-label="Défiler vers le haut">↑</button><button onClick={()=>key(0xff56)} aria-label="Défiler vers le bas">↓</button><small>{session&&!expired?`${Math.floor(seconds/60)}:${String(seconds%60).padStart(2,'0')} restantes`:''}</small></div>
  {!expired&&<div ref={screen} className="gf-manual-screen" aria-label="Navigateur Chromium distant, contrôle tactile"/>}
  {connection==='connecting'&&!expired&&<p role="status">Connexion à la fenêtre du Pi…</p>}
  {connection==='disconnected'&&!expired&&<button onClick={()=>setAttempt(x=>x+1)}>Reconnecter la fenêtre</button>}
  <p role="status" hidden={!failed&&!expired&&!status} className="gf-manual-state">{failed?(session.message||'Ouverture impossible. Relancez la vérification.'):expired?'Session expirée. Relancez la vérification.':status}</p>{error&&<p role="alert">{error}</p>}

  <footer>{expired?<button onClick={restart} disabled={busy||!(session?.slug||gameSlug)}>Relancer la vérification</button>:<button className="gf-manual-resume" onClick={resume} disabled={busy||session?.status!=='ready'}>{busy?'Lecture de la page…':'Reprendre la recherche'}</button>}<button onClick={onClose}>Revenir à la fiche</button><button onClick={finish} disabled={busy}>Terminer la session</button></footer>
  <details className="gf-manual-help"><summary>Aide</summary><p>Touchez pour cliquer. Sur téléphone, le mode paysage agrandit la fenêtre. Fermer conserve la session ; « Terminer » la ferme.</p></details>
 </section></div>,document.body);
}
