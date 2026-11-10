import React,{useEffect,useRef,useState} from 'react';
import {apiUrl} from '../libs/serverBase.js';
import {authHeaders} from '../profile/auth/AuthContext.jsx';
import {prioritizeMetadata,manualMetadata,safeExternal,metadataFields} from './metadata-priority.js';
import {Art} from './ExperienceView.jsx';
import './emugen-details.css';
import {useSourceSettings,sourceId} from './SourceSettings.jsx';
import InteractiveVerification,{verificationRequest} from './InteractiveVerification.jsx';

export async function metadataRequest(token,path,options={}){
 const abort=new AbortController(),cancel=()=>abort.abort();
 if(options.signal?.aborted)abort.abort();else options.signal?.addEventListener('abort',cancel,{once:true});
 const timer=setTimeout(cancel,path==='emugen/sources'?60000:18000);
 try{
  const result=await fetch(apiUrl('/experience-api/v1/'+path),{...options,signal:abort.signal,cache:'no-store',headers:{...authHeaders(token),...(options.body?{'Content-Type':'application/json'}:{})},body:options.body?JSON.stringify(options.body):undefined});
  const data=await result.json();if(!result.ok)throw new Error(data.error||'Métadonnées indisponibles.');return data;
 }finally{clearTimeout(timer);options.signal?.removeEventListener('abort',cancel);}
}
export function useEmugenDetails(game,token,onMetadata,canEditMetadata=true){
 const emugenEnabled=useSourceSettings().enabledIds.includes('emugen');
 const [record,setRecord]=useState(game.emugen),[extras,setExtras]=useState({}),[slug,setSlug]=useState(game.emugenSlug||''),[choices,setChoices]=useState(game.metadataCandidates||[]),[query,setQuery]=useState(game.title),[loading,setLoading]=useState(false),[error,setError]=useState('');
 const signal=useRef(null),account=useRef(token);account.current=token;
 useEffect(()=>{setExtras({});setRecord(game.emugen);setSlug(game.emugenSlug||'');setChoices(game.metadataCandidates||[]);},[game.emugenSlug,game.reference,game.emugen,game.metadataCandidates]);
 useEffect(()=>{
  if(!token||!slug||!emugenEnabled){setLoading(false);return undefined;}
  const abort=new AbortController();signal.current=abort;setLoading(true);setError('');
  metadataRequest(token,'metadata/enrich',{method:'POST',body:{slug,game:{...Object.fromEntries(metadataFields.filter(key=>typeof game[key]==='string'||Array.isArray(game[key])).map(key=>[key,game[key]])),consoleId:game.consoleId,manualMetadata:manualMetadata(game)},variants:(game.variants||[]).slice(0,20)},signal:abort.signal}).then(data=>{if(!abort.signal.aborted){setRecord(data.item.emugen);setExtras(data.item);}}).catch(e=>{if(!abort.signal.aborted)setError(e.message);}).finally(()=>{if(!abort.signal.aborted)setLoading(false);});
  return()=>abort.abort();
 },[slug,token,game,emugenEnabled]);
 async function search(){
  if(!query.trim()||loading)return;
  signal.current?.abort();const abort=new AbortController();signal.current=abort;setLoading(true);setError('');
  try{const data=await metadataRequest(token,`emugen/${encodeURIComponent(game.consoleId)}?q=${encodeURIComponent(query.trim())}&letter=A`,{signal:abort.signal});setChoices(data.items||[]);if(!data.items?.length)setError('Aucune correspondance EmuGen pour cette console.');}catch(e){if(!abort.signal.aborted)setError(e.message);}finally{if(!abort.signal.aborted)setLoading(false);}
 }
 async function choose(candidate){
  setLoading(true);setError('');const abort=new AbortController();signal.current=abort;
  try{
   const persist=Boolean(game.reference)&&canEditMetadata;
   const result=persist?await metadataRequest(token,'metadata/bind',{method:'POST',body:{game:{consoleId:game.consoleId,reference:game.reference,title:game.title,manualMetadata:manualMetadata(game)},slug:candidate.emugenSlug},signal:abort.signal}):await metadataRequest(token,`emugen/games/${candidate.emugenSlug}?console=${game.consoleId}`,{signal:abort.signal});
   if(account.current!==token||abort.signal.aborted)return;
   const selected=persist?result.item.emugen:result.item;
   setExtras({});setRecord(selected);setSlug(candidate.emugenSlug);setChoices([]);
   onMetadata?.(prioritizeMetadata({...game,metadataCandidates:[],manualMetadata:manualMetadata(game)},selected,game.variants||[]));
  }catch(e){if(!abort.signal.aborted)setError(e.message);}finally{if(!abort.signal.aborted)setLoading(false);}
 }
 useEffect(()=>()=>signal.current?.abort(),[]);
 return {game:prioritizeMetadata({...game,...extras,reference:game.reference,manualMetadata:manualMetadata(game)},record,game.variants||[]),loading,error,choices,query,setQuery,search,choose};
}
export function MetadataContent({state}){
 const game=state.game;
 return <section className="gf-emugen-metadata" aria-label="Métadonnées du jeu">
  {state.loading&&<p role="status">Chargement de la fiche…</p>}{state.error&&<p role="status">{state.error}</p>}
  {game.description&&<details className="gf-game-description"><summary>Résumé</summary><p>{game.description}</p></details>}
  <dl>{[['consoleName','Console'],['releaseDate','Sortie'],['genres','Genre'],['languages','Langues'],['version','Version'],['region','Région']].map(([key,label])=>game[key]?.length>0&&<React.Fragment key={key}><dt>{label}</dt><dd title={game.fieldSources?.[key]?`Source : ${game.fieldSources[key]==='manual'?'correction manuelle':game.fieldSources[key]}`:undefined}>{Array.isArray(game[key])?game[key].join(' · '):game[key]}{game.fieldSources?.[key]&&game.fieldSources[key]!=='EmuGen'&&<small>{game.fieldSources[key]==='manual'?'Correction manuelle':game.fieldSources[key]}</small>}</dd></React.Fragment>)}</dl>
  {game.coverBack&&<details><summary>Jaquette arrière</summary><div className="gf-back-cover"><Art item={{image:game.coverBack,attribution:'EmuGen · Ayants droit du jeu'}}/></div></details>}
  {['screenshots','images'].map(field=>game[field]?.length>0&&<section className="gf-game-visuals" key={field}><h3>{field==='screenshots'?'Captures d’écran':'Autres visuels'}</h3><div>{game[field].map(image=><Art key={image} item={{image,attribution:game.fieldSources?.[field]||'Ayants droit du jeu'}}/>)}</div></section>)}
  {game.trailers?.filter(safeExternal).map((url,index)=><a key={url} href={url} target="_blank" rel="noopener noreferrer">Bande-annonce {index+1} ↗</a>)}
  {!state.removed&&safeExternal(game.emugenUrl)&&<a href={game.emugenUrl} target="_blank" rel="noopener noreferrer">Fiche EmuGen ↗</a>}
  {!state.removed&&<details open={state.choices.length>0}><summary>{game.emugenSlug?'Changer de fiche EmuGen':'Identifier sur EmuGen'}</summary><form onSubmit={e=>{e.preventDefault();state.search();}}><label htmlFor="gf-emugen-query">Titre à rechercher</label><div><input id="gf-emugen-query" type="search" value={state.query} onChange={e=>state.setQuery(e.target.value)}/><button disabled={state.loading}>Rechercher</button></div></form>{state.choices.length>0&&<><p>Choisissez une fiche.</p><div className="gf-emugen-choices">{state.choices.map(candidate=><button disabled={state.loading} key={candidate.emugenSlug} onClick={()=>state.choose(candidate)}><strong>{candidate.title}</strong><small>{candidate.consoleName||candidate.consoleId}{candidate.region?' · '+candidate.region:''}{candidate.version?' · '+candidate.version:''}</small></button>)}</div></>}</details>}
 </section>;
}
export function EmugenSources({game,token,versions=[],onInstall}){
 const sources=useSourceSettings(),emugenEnabled=sources.enabledIds.includes('emugen');
 const [opened,setOpened]=useState(Boolean(game.manualVerificationResult)),[result,setResult]=useState(game.manualVerificationResult||null);
 const [manual,setManual]=useState(null),[viewer,setViewer]=useState(false),[manualBusy,setManualBusy]=useState(false),[manualError,setManualError]=useState('');
 useEffect(()=>{if(game.manualVerificationResult){setResult(game.manualVerificationResult);setOpened(true);}},[game.manualVerificationResult]);
 useEffect(()=>{
  if(!token||!game.emugenSlug||!emugenEnabled)return undefined;
  const abort=new AbortController();
  verificationRequest(token,'/current?slug='+encodeURIComponent(game.emugenSlug),{signal:abort.signal}).then(value=>{if(!abort.signal.aborted){setManual(value.item);if(value.item?.status==='ready'&&value.item.result){setResult(value.item.result);setOpened(true);}}}).catch(()=>{});
  return()=>abort.abort();
 },[token,game.emugenSlug,emugenEnabled]);
 async function verify(){
  setManualBusy(true);setManualError('');
  try{const value=await verificationRequest(token,'',{method:'POST',body:{slug:game.emugenSlug,consoleId:game.consoleId}});setManual(value.item);setViewer(true);}
  catch(e){setManualError(e.message);}finally{setManualBusy(false);}
 }
 async function resume(){
  setManualBusy(true);setManualError('');
  try{setResult(await verificationRequest(token,'/'+manual.id+'/resume',{method:'POST',body:{}}));}
  catch(e){setManualError(e.message);if(e.status==='session_expired')setManual(m=>({...m,status:'expired'}));}finally{setManualBusy(false);}
 }
 const others=versions.filter(v=>sources.enabledIds.includes(sourceId(v.provider))&&v.provider!=='EmuGen'&&(safeExternal(v.source)||v.downloadId)).filter((v,index,all)=>all.findIndex(x=>x.source===v.source&&x.downloadId===v.downloadId)===index);
 return <section className="gf-emugen-sources"><button className="gf-sources-open" onClick={()=>setOpened(x=>!x)} aria-expanded={opened}>{opened?'Masquer les sources':'Sources de téléchargement'}</button>{opened&&<div>
  {!emugenEnabled&&<p>EmuGen est désactivé dans les paramètres.</p>}

  {result?.message&&<p role="status">{result.message}</p>}

  {(emugenEnabled?result?.links||[]:[]).filter(link=>safeExternal(link.url)).map((link,index)=><a className="gf-source-entry" key={link.url+index} href={link.url} target="_blank" rel="noopener noreferrer"><strong>EmuGen · {link.label}</strong><small>{link.url}</small></a>)}
  {!game.emugenSlug&&<p>Choisissez une fiche EmuGen pour voir ses liens.</p>}
  {emugenEnabled&&safeExternal(game.emugenUrl)&&<a href={game.emugenUrl} target="_blank" rel="noopener noreferrer">Fiche EmuGen ↗</a>}
  {emugenEnabled&&game.emugenSlug&&<div className="gf-verification-actions">
   <button onClick={['ready','starting'].includes(manual?.status)?()=>setViewer(true):verify} disabled={manualBusy}>{manualBusy?'Ouverture…':['ready','starting'].includes(manual?.status)?'Ouvrir le navigateur EmuGen':manual?'Relancer la vérification':'Vérifier EmuGen'}</button>
   {manual?.status==='ready'&&<button onClick={resume} disabled={manualBusy}>Reprendre la recherche</button>}
  </div>}
  {manualError&&<p role="alert">{manualError}</p>}
  {others.map((v,index)=><div className="gf-source-entry" key={(v.source||v.downloadId)+index}><strong>{v.provider||'Catalogue'}</strong><span>{v.title}</span>{v.language&&<small>{v.language==='unknown'?'Langue non confirmée':v.language.split(',').join(' / ')}</small>}{safeExternal(v.source)&&<a href={v.source} target="_blank" rel="noopener noreferrer">Fiche du jeu ↗</a>}{v.downloadId&&!v.reference&&<button onClick={()=>onInstall(v)}>Installer cette édition</button>}</div>)}

 </div>}{viewer&&manual&&<InteractiveVerification token={token} sessionId={manual.id} gameSlug={game.emugenSlug} consoleId={game.consoleId} onClose={()=>setViewer(false)} onSession={setManual} onResolved={value=>{if(value.slug===game.emugenSlug)setResult(value);}}/>}</section>;
}
