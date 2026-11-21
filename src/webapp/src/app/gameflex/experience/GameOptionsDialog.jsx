import React,{useEffect,useRef,useState} from 'react';
import {listCores} from '../api/retrogaming.js';
import {useDialogFocus} from './dialog-focus.js';
import {useDirectionalFocus} from './ExperienceView.jsx';
import {Icon} from './EmulatorUI.jsx';

export function gameOptionsKey(scope,game){return `${scope}:game-options:${encodeURIComponent(game.consoleId)}:${encodeURIComponent(game.reference)}`;}
export function readGameOptions(scope,game){
 try{const value=JSON.parse(localStorage.getItem(gameOptionsKey(scope,game))||'{}');return typeof value.core==='string'?{core:value.core}:{};}catch{return {};}
}
export default function GameOptionsDialog({game,scope,onClose,onDetails}){
 const root=useRef(null);useDialogFocus(root);useDirectionalFocus(root);
 const [cores,setCores]=useState(null),[core,setCore]=useState(()=>readGameOptions(scope,game).core||''),[error,setError]=useState('');
 useEffect(()=>{let active=true;listCores(game.consoleId).then(result=>{if(!active)return;if(!result.ok)throw new Error('Lecture des émulateurs indisponible.');setCores((result.data||[]).filter(item=>item.installed));}).catch(()=>{if(active)setError('Impossible de lire les émulateurs. Réessayez plus tard.');});return()=>{active=false;};},[game.consoleId]);
 const save=()=>{try{localStorage.setItem(gameOptionsKey(scope,game),JSON.stringify(core?{core}:{}));onClose();}catch{setError('Impossible d’enregistrer les paramètres sur cet appareil.');}};
 const onKey=event=>{if(event.key==='Escape'){event.preventDefault();event.stopPropagation();onClose();}if(event.key==='Tab'){const nodes=[...root.current.querySelectorAll('button:not(:disabled),select')];if(event.shiftKey&&document.activeElement===nodes[0]){event.preventDefault();nodes.at(-1)?.focus();}else if(!event.shiftKey&&document.activeElement===nodes.at(-1)){event.preventDefault();nodes[0]?.focus();}}};
 return <div className="ps-control-backdrop"><section ref={root} className="ps-control ps-game-options" role="dialog" aria-modal="true" aria-labelledby="ps-options-title" onKeyDown={onKey}>
 <header><div><small>PARAMÈTRES DU JEU</small><h2 id="ps-options-title">{game.displayTitle||game.title.replace(/\s*[([][^)\]]*[)\]]/g,'').replace(/\.[a-z0-9]+$/i,'').trim()}</h2></div><button aria-label="Fermer les paramètres du jeu" onClick={onClose}><Icon name="close"/></button></header>
 {error&&<p role="alert">{error}</p>}
 <label>Émulateur utilisé<select aria-label="Émulateur utilisé" disabled={!cores} value={core} onChange={event=>setCore(event.target.value)}><option value="">Automatique</option>{core&&cores&&!cores.some(item=>item.name===core)&&<option value={core}>{core} · indisponible</option>}{cores?.map(item=><option key={item.name} value={item.name}>{item.displayName||item.name}</option>)}</select></label>
 {!cores&&!error&&<p role="status">Lecture des émulateurs…</p>}{cores?.length===0&&<p>Aucun émulateur installé pour cette console.</p>}
 <p className="ps-options-note">Ce choix est enregistré pour ce jeu, sur cet appareil. Les sauvegardes sont conservées.</p>
 <button className="ps-options-details" onClick={()=>{onClose();onDetails(game);}}>Fiche, éditions et collections <span aria-hidden="true">↗</span></button>
 <footer><button onClick={onClose}>Annuler</button><button disabled={!cores||Boolean(core&&!cores.some(item=>item.name===core))} onClick={save}>Enregistrer</button></footer>
 </section></div>;
}
