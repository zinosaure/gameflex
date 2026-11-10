import React,{useCallback,useEffect,useRef,useState} from 'react';
import {appPath} from '../libs/appBase.js';
import {phoneRequest,phoneSocket} from './phone-pad-api.js';
import './tv-phone.css';
function PadButton({label,buttons,held,onChange,className=''}){
 const pointers=useRef(new Set());const set=(id,down)=>{if(down)pointers.current.add(id);else pointers.current.delete(id);onChange(buttons,pointers.current.size>0);};
 return <button className={'gf-pad-key '+className} aria-label={label} aria-pressed={buttons.every(index=>held.includes(index))} onContextMenu={event=>event.preventDefault()} onPointerDown={event=>{event.preventDefault();event.currentTarget.setPointerCapture(event.pointerId);set(event.pointerId,true);}} onPointerUp={event=>set(event.pointerId,false)} onPointerCancel={event=>set(event.pointerId,false)} onLostPointerCapture={event=>set(event.pointerId,false)} onKeyDown={event=>{if(['Enter',' '].includes(event.key)){event.preventDefault();set('keyboard',true);}}} onKeyUp={event=>{if(['Enter',' '].includes(event.key)){event.preventDefault();set('keyboard',false);}}}>{label}</button>;
}
function Stick({side,axes,onChange}){
 const active=useRef(null);const index=side*2;
 const move=event=>{const box=event.currentTarget.getBoundingClientRect();const x=(event.clientX-box.left-box.width/2)/(box.width*.38),y=(event.clientY-box.top-box.height/2)/(box.height*.38),length=Math.max(1,Math.hypot(x,y));onChange(index,x/length,y/length);};
 const release=event=>{if(active.current===event.pointerId){active.current=null;onChange(index,0,0);}};
 return <div className="gf-pad-stick" role="application" aria-label={side?'Stick droit':'Stick gauche'} onPointerDown={event=>{if(active.current!==null)return;event.preventDefault();active.current=event.pointerId;event.currentTarget.setPointerCapture(event.pointerId);move(event);}} onPointerMove={event=>{if(active.current===event.pointerId)move(event);}} onPointerUp={release} onPointerCancel={release} onLostPointerCapture={release}><span style={{transform:`translate(${axes[index]*35}%,${axes[index+1]*35}%)`}}/><small>{side?'R':'L'}</small></div>;
}
export default function PhoneControllerPage(){
 const [pair,setPair]=useState(()=>{try{return JSON.parse(sessionStorage.getItem('gameflex:phone-pad')||'null');}catch{return null;}}),[code,setCode]=useState(''),[name,setName]=useState('Mon téléphone'),[status,setStatus]=useState({}),[error,setError]=useState(''),[busy,setBusy]=useState(false),[pad,setPad]=useState({buttons:[],axes:[0,0,0,0]});
 const socket=useRef(null),state=useRef(pad),sequence=useRef(0),epoch=useRef(0),approved=useRef(false),active=useRef(false);
 const send=useCallback(()=>{if(socket.current?.readyState===1&&approved.current&&active.current)socket.current.send(JSON.stringify({type:'pad',buttons:state.current.buttons,axes:state.current.axes,seq:sequence.current++,epoch:epoch.current}));},[]);
 const release=useCallback(()=>{state.current={buttons:[],axes:[0,0,0,0]};setPad(state.current);send();},[send]);
 const update=useCallback(value=>{state.current=value;setPad(value);send();},[send]);
 const button=(indices,down)=>{const next=new Set(state.current.buttons);indices.forEach(index=>down?next.add(index):next.delete(index));update({...state.current,buttons:[...next]});};
 const stick=(index,x,y)=>{const axes=[...state.current.axes];axes[index]=x;axes[index+1]=y;update({...state.current,axes});};
 const forget=()=>{release();socket.current?.close();sessionStorage.removeItem('gameflex:phone-pad');setPair(null);setStatus({});};
 const join=async event=>{event.preventDefault();setBusy(true);setError('');try{const value=await phoneRequest('/join',{code:code.replace(/\s/g,''),name});sessionStorage.setItem('gameflex:phone-pad',JSON.stringify(value));setPair(value);}catch(e){setError(e.message);}finally{setBusy(false);}};
 useEffect(()=>{
  if(!pair)return;let stopped=false,retry=null;sequence.current=0;
  const reconnect=()=>{if(stopped||retry)return;retry=setTimeout(()=>{retry=null;connect();},1500);};
  const connect=()=>{const ws=phoneSocket(pair.id);socket.current=ws;ws.onopen=()=>ws.send(JSON.stringify({role:'phone',token:pair.phoneToken}));
   ws.onmessage=event=>{const value=JSON.parse(event.data);if(value.type==='status'){const changed=epoch.current!==value.epoch;epoch.current=value.epoch;approved.current=value.approved;active.current=value.hostConnected;setStatus(value);setError('');if(changed||!value.hostConnected)release();}
    if(value.type==='closed'){stopped=true;approved.current=false;active.current=false;release();sessionStorage.removeItem('gameflex:phone-pad');setPair(null);setStatus({});setError('Association terminée. Saisissez un nouveau code.');ws.close();}};
   ws.onclose=event=>{active.current=false;release();setStatus(value=>({...value,hostConnected:false}));if(event.code===1008){stopped=true;approved.current=false;sessionStorage.removeItem('gameflex:phone-pad');setPair(null);setStatus({});setError('Association terminée. Saisissez un nouveau code.');}else reconnect();};
   ws.onerror=()=>setError('Connexion interrompue. Les touches sont relâchées.');};
  connect();const heartbeat=setInterval(()=>{send();if(socket.current?.readyState===1)socket.current.send('{"type":"ping"}');else if(socket.current?.readyState===3){active.current=false;release();setStatus(value=>({...value,hostConnected:false}));reconnect();}},500);
  return()=>{stopped=true;clearTimeout(retry);clearInterval(heartbeat);release();socket.current?.close();approved.current=false;active.current=false;};
 },[pair?.id,send,release]);
 useEffect(()=>{window.addEventListener('blur',release);window.addEventListener('pagehide',release);document.addEventListener('visibilitychange',release);return()=>{window.removeEventListener('blur',release);window.removeEventListener('pagehide',release);document.removeEventListener('visibilitychange',release);};},[release]);
 const key=(label,buttons,className='')=><PadButton label={label} buttons={buttons} held={pad.buttons} onChange={button} className={className}/>;
 return <div className="gf-phone-controller"><header className="gf-phone-header"><img src={appPath('/images/icon-192.png')} alt="" width="38" height="38"/><div><strong>GameFlex</strong><small>{status.approved?(status.hostConnected?'Manette connectée':'TV déconnectée'):'Votre téléphone, votre manette'}</small></div>{pair&&<button onClick={forget}>Déconnecter</button>}</header>
 {error&&<p className="gf-phone-error" role="alert">{error}</p>}
 {!pair?<main className="gf-phone-join"><span className="gf-phone-symbol" aria-hidden="true">↔</span><h1>Jouez sur la TV.<br/>Gardez les commandes.</h1><p>Sur la télévision, ouvrez « Manette téléphone », puis affichez un code.</p><form onSubmit={join}><label>Code affiché sur la TV<input aria-label="Code affiché sur la TV" type="text" inputMode="numeric" autoComplete="one-time-code" pattern="[0-9 ]{8,10}" maxLength={9} value={code} onChange={event=>setCode(event.target.value.replace(/[^0-9 ]/g,''))} placeholder="0000 0000" required/></label><label>Nom du téléphone<input value={name} maxLength={40} onChange={event=>setName(event.target.value)} required/></label><button disabled={busy||code.replace(/\s/g,'').length!==8}>{busy?'Association…':'Associer à ma TV'}</button></form><small>Le code ne connecte pas votre téléphone au compte GameFlex. La TV doit confirmer l’association.</small></main>:!status.approved?<main className="gf-phone-wait" role="status"><div className="gf-phone-pulse"/><h1>Regardez la télévision</h1><p>Autorisez « {name} » sur l’écran pour activer cette manette.</p></main>:<main className="gf-phone-pad" aria-label="Manette tactile">
 <div className="gf-pad-context" role="status">{!status.hostConnected?'En attente de la TV · commandes relâchées':status.screen==='game'?'La partie est sur votre télévision':status.screen==='unsupported'?'En attente des commandes du jeu sur la TV':'Naviguez sur la TV avec la croix et A'}</div>
 <div className="gf-pad-shoulders"><div>{key('L1',[4])}{key('L2',[6])}</div><button className="gf-pad-fullscreen" onClick={()=>{Promise.resolve(document.documentElement.requestFullscreen?.()).catch(()=>{});navigator.wakeLock?.request('screen').catch(()=>{});}}>Plein écran</button><div>{key('R2',[7])}{key('R1',[5])}</div></div>
 <div className="gf-pad-main"><div className="gf-pad-dpad">{key('↑',[12],'up')}{key('←',[14],'left')}{key('→',[15],'right')}{key('↓',[13],'down')}<span aria-hidden="true"/></div><div className="gf-pad-identity"><img src={appPath('/images/icon-192.png')} alt=""/><small>ÉCRAN TV<br/>MANETTE MOBILE</small></div><div className="gf-pad-face">{key('Y',[3],'y')}{key('X',[2],'x')}{key('B',[1],'b')}{key('A',[0],'a')}</div></div>
 <div className="gf-pad-bottom"><div className="gf-pad-stick-block"><Stick side={0} axes={pad.axes} onChange={stick}/>{key('L3',[10],'gf-pad-stick-click')}</div><div className="gf-pad-system">{key('Select',[8])}{key('Menu TV',[16])}{key('Start',[9])}</div><div className="gf-pad-stick-block"><Stick side={1} axes={pad.axes} onChange={stick}/>{key('R3',[11],'gf-pad-stick-click')}</div></div>
 </main>}
 </div>;
}
