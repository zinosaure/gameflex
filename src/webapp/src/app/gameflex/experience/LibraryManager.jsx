import React, { useCallback, useEffect, useRef, useState } from 'react';
import { apiUrl } from '../libs/serverBase.js';
import { authHeaders } from '../profile/auth/AuthContext.jsx';
import {activeDownload,freshTerminalJobs} from './download-state.js';
import { Icon } from './EmulatorUI.jsx';
import './library-manager.css';

export async function libraryRequest(token,path,options={}) {
    const abort=new AbortController(),cancel=()=>abort.abort();if(options.signal?.aborted)abort.abort();else options.signal?.addEventListener('abort',cancel,{once:true});
    const timer=setTimeout(cancel,8000);
    try{
        const r=await fetch(apiUrl('/experience-api/v1/'+path),{...options,signal:abort.signal,cache:'no-store',headers:{...authHeaders(token),...(options.body?{'Content-Type':'application/json'}:{})},body:options.body?JSON.stringify(options.body):undefined});
        const data=await r.json();if(!r.ok)throw new Error(data.error||'Le serveur est indisponible.');return data;
    }finally{clearTimeout(timer);options.signal?.removeEventListener('abort',cancel);}
}
export function useLibraryDownloads({token,username,enabled,onInstalled}) {
    const [library,setLibrary]=useState(null),[setupOpen,setSetupOpen]=useState(false),[pendingGame,setPendingGame]=useState(null),[toasts,setToasts]=useState([]),[jobs,setJobs]=useState([]);
    const account=useRef(token);account.current=token;
    const seen=useRef(new Set()),requests=useRef(new Map()),current=useRef([]),warnings=useRef(new Set()),reload=useRef(onInstalled);reload.current=onInstalled;current.current=jobs;
    const notify=useCallback((kind,title,message,id)=>setToasts(xs=>{const item={id:id||globalThis.crypto?.randomUUID?.()||`${Date.now()}-${Math.random()}`,kind,title,message};return [...xs.filter(x=>x.id!==item.id).slice(-3),item];}),[]);
    const dismiss=useCallback(id=>setToasts(xs=>xs.filter(x=>x.id!==id)),[]);
    useEffect(()=>{
        setToasts([]);setLibrary(null);setPendingGame(null);setSetupOpen(false);setJobs([]);requests.current.clear();warnings.current.clear();
        if(!token||!username)return undefined;
        let active=true,timer;const abort=new AbortController(),storageKey=`gameflex:downloads:seen:${username}`;
        try{seen.current=new Set(JSON.parse(localStorage.getItem(storageKey)||'[]'));}catch{seen.current=new Set();}
        libraryRequest(token,'library',{signal:abort.signal}).then(data=>{if(active){setLibrary(data);if(!data.configured)setSetupOpen(true);}}).catch(e=>{if(active&&!abort.signal.aborted){setLibrary({error:e.message});notify('error','Dossier de jeux',e.message);}});
        const poll=async()=>{
            try{
                const data=await libraryRequest(token,'downloads',{signal:abort.signal});if(!active)return;
                const received=data.items||[];
                setJobs(previous=>[...received.map(job=>job.status==='succeeded'&&!seen.current.has(job.id)?{...job,phase:'indexing'}:{...job,syncWarning:warnings.current.has(job.id)}),...[...requests.current.values()].filter(job=>!received.some(item=>item.gameId===job.gameId)),...previous.filter(job=>activeDownload(job)&&Date.now()/1000-job.createdAt<10&&!received.some(item=>item.gameId===job.gameId)&&!requests.current.has(job.gameId))]);
                const fresh=freshTerminalJobs(received,seen.current);
                let indexed=true;
                if(fresh.some(job=>job.status==='succeeded')){try{indexed=(await reload.current?.())!==false;}catch{indexed=false;}}
                if(!active)return;
                for(const job of fresh){
                    if(job.status==='succeeded'&&!indexed)warnings.current.add(job.id);
                    notify(job.status==='failed'?'error':indexed?'success':'warning',job.title,job.status==='failed'?job.message:indexed?'Jeu ajouté à la bibliothèque.':'Jeu rangé. Rechargez la bibliothèque.',`download:${job.id}`);
                }
                setJobs(previous=>previous.map(job=>job.status==='succeeded'?{...job,phase:'completed',syncWarning:warnings.current.has(job.id)}:job));
                try{localStorage.setItem(storageKey,JSON.stringify([...seen.current].slice(-64)));}catch{}
            }catch{if(active)setJobs(previous=>previous.map(job=>activeDownload(job)?{...job,trackingLost:true}:job));}
            if(active)timer=setTimeout(poll,current.current.some(activeDownload)||requests.current.size?750:2000);
        };poll();
        return()=>{active=false;clearTimeout(timer);abort.abort();};
    },[token,username,notify]);
    async function start(game){
        if(!game.downloadId){notify('error',game.title,'Aucun lien de téléchargement pour cette source.');return;}
        if(requests.current.has(game.downloadId)||current.current.some(job=>job.gameId===game.downloadId&&activeDownload(job)))return;
        if(!library?.configured){setPendingGame(game);setSetupOpen(true);return;}
        const title=game.displayTitle||game.title;
        const optimistic={id:`request:${game.downloadId}`,gameId:game.downloadId,title,consoleId:game.consoleId,image:game.image,status:'submitting',phase:'submitting',progress:0,bytesReceived:0,createdAt:Date.now()/1000,updatedAt:Date.now()/1000,message:'Demande en cours de confirmation…'};
        requests.current.set(game.downloadId,optimistic);setJobs(previous=>[optimistic,...previous.filter(job=>job.gameId!==game.downloadId)]);
        try{
            const response=await libraryRequest(token,'downloads',{method:'POST',body:{gameId:game.downloadId,presentation:{candidateIds:game.candidateIds||[],title,displayTitle:title,image:game.image,attribution:game.attribution}}});
            if(account.current!==token)return;requests.current.delete(game.downloadId);setJobs(previous=>[response.item,...previous.filter(job=>job.gameId!==game.downloadId)]);
        }catch(e){if(account.current!==token)return;requests.current.delete(game.downloadId);setJobs(previous=>previous.filter(job=>job.id!==optimistic.id));notify('warning',title,'Demande non confirmée. Vérification en cours.');}
    }
    async function configure(folder){
        const result=await libraryRequest(token,'library',{method:'PUT',body:{folder}});setLibrary(result);setSetupOpen(false);notify('success','Dossier configuré','Les jeux seront rangés par console.');
        if(pendingGame){const game=pendingGame;setPendingGame(null);await startWithFolder(game,result);}
    }
    async function startWithFolder(game,folder){
        // Configuration just completed: submit directly without stale React state.
        try{const response=await libraryRequest(token,'downloads',{method:'POST',body:{gameId:game.downloadId,presentation:{candidateIds:game.candidateIds||[],title:game.displayTitle||game.title,image:game.image,attribution:game.attribution}}});if(account.current!==token)return;setJobs(previous=>[response.item,...previous.filter(job=>job.gameId!==game.downloadId)]);}catch(e){if(account.current===token)notify('error',game.title,e.message);}
    }
    return {library,jobs,start,notify,openSetup:()=>setSetupOpen(true),ui:<><DownloadNotifications items={toasts} onDismiss={dismiss}/>{setupOpen&&enabled&&<LibrarySetup library={library} onBrowse={(folder,signal)=>libraryRequest(token,`library/folders?folder=${encodeURIComponent(folder)}`,{signal})} onSave={configure} onLater={()=>{setSetupOpen(false);setPendingGame(null);}}/>}</>};
}

export function DownloadNotifications({items,onDismiss}){
    return <aside className="gf-notifications" aria-label="Notifications d’installation">{items.map(item=><Notice key={item.id} item={item} onDismiss={onDismiss}/>)}</aside>;
}
function Notice({item,onDismiss}){
    useEffect(()=>{const timer=setTimeout(()=>onDismiss(item.id),item.kind==='error'?12000:8000);return()=>clearTimeout(timer);},[item.id,item.kind,onDismiss]);
    return <section className={`gf-notice gf-notice-${item.kind}`} role={item.kind==='error'?'alert':'status'}><span className="gf-notice-mark">{item.kind==='success'?'✓':item.kind==='error'||item.kind==='warning'?'!':'↓'}</span><div><strong>{item.title}</strong><p>{item.message}</p></div><button onClick={()=>onDismiss(item.id)} aria-label="Fermer la notification">×</button></section>;
}
export function LibrarySetup({library,onSave,onLater,onBrowse}){
    const [location,setLocation]=useState(''),[listing,setListing]=useState(null),[busy,setBusy]=useState(false),[loading,setLoading]=useState(true),[error,setError]=useState(''),[revision,setRevision]=useState(0);const root=useRef(null),list=useRef(null);
    useEffect(()=>{const previous=document.activeElement;root.current?.querySelector('button')?.focus({preventScroll:true});return()=>previous?.focus();},[]);
    useEffect(()=>{
        const abort=new AbortController();setLoading(true);setError('');setListing(null);
        if(!library?.canConfigure){setLoading(false);return()=>abort.abort();}
        if(!onBrowse){setListing({folder:'',items:[],breadcrumbs:[],selectable:false});setLoading(false);setError('Le navigateur de dossiers est indisponible.');return()=>abort.abort();}
        onBrowse(location,abort.signal).then(data=>{if(!abort.signal.aborted){setListing(data);if(list.current)list.current.scrollTop=0;}}).catch(e=>{if(!abort.signal.aborted)setError(e.message);}).finally(()=>{if(!abort.signal.aborted)setLoading(false);});
        return()=>abort.abort();
    },[location,library?.canConfigure,revision]);
    const navigate=folder=>{setLocation(folder);setError('');};
    const save=async()=>{if(!listing?.selectable)return;setBusy(true);setError('');try{await onSave(listing.folder);}catch(e){setError(e.message);}finally{setBusy(false);}};
    const key=e=>{if(e.key==='Escape'&&!busy){e.stopPropagation();onLater();}if(e.key==='Tab'){const nodes=[...root.current.querySelectorAll('button:not(:disabled)')];const first=nodes[0],last=nodes[nodes.length-1];if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus();}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus();}}};
    return <div className="gf-setup-backdrop"><section className="gf-library-setup gf-folder-dialog" role="dialog" aria-modal="true" aria-labelledby="gf-library-title" ref={root} onKeyDown={key}>
        <header className="gf-folder-header"><span className="gf-folder-header-icon"><Icon name="library" size={25}/></span><div><small>RASPBERRY PI</small><h2 id="gf-library-title">Choisir un dossier</h2></div><button onClick={onLater} disabled={busy} className="gf-folder-close" aria-label="Fermer le navigateur de dossiers">×</button></header>
        <p className="gf-folder-intro">Jeux classés par console.</p>
        <nav className="gf-folder-breadcrumbs" aria-label="Emplacement actuel"><button onClick={()=>navigate('')} disabled={busy} title="Dossiers accessibles"><Icon name="home" size={17}/><span>Emplacements</span></button>{(listing?.breadcrumbs||[]).map((crumb,index)=><React.Fragment key={crumb.folder}><span aria-hidden="true">/</span><button onClick={()=>navigate(crumb.folder)} disabled={busy} aria-current={index===listing.breadcrumbs.length-1?'location':undefined}>{crumb.name}</button></React.Fragment>)}</nav>
        <div className="gf-folder-toolbar"><button onClick={()=>navigate(listing?.parent??'')} disabled={busy||loading||!location} aria-label="Dossier parent"><Icon name="back" size={17}/>Remonter</button><span>{loading?'Chargement…':`${listing?.items?.length||0} dossier${listing?.items?.length===1?'':'s'}`}</span><button onClick={()=>setRevision(x=>x+1)} disabled={busy||loading} aria-label="Actualiser les dossiers">↻</button></div>
        <div className="gf-folder-list" ref={list} aria-label="Dossiers du Pi" aria-busy={loading}>{loading?<p role="status">Chargement des dossiers…</p>:error?<div className="gf-folder-state" role="alert"><strong>Dossier inaccessible</strong><p>{error}</p><button onClick={()=>{navigate('');setRevision(x=>x+1);}}>Revenir aux emplacements</button></div>:!library?.canConfigure?<p>Choix réservé à l’administrateur.</p>:listing?.items?.length?listing.items.map(item=><button className="gf-folder-row" key={item.folder} onClick={()=>navigate(item.folder)} disabled={busy}><svg className="gf-folder-icon" width="32" height="28" viewBox="0 0 32 28" fill="none" aria-hidden="true"><path d="M2 7a3 3 0 0 1 3-3h7l3 3h12a3 3 0 0 1 3 3v12a3 3 0 0 1-3 3H5a3 3 0 0 1-3-3Z" fill="currentColor" opacity=".2"/><path d="M2 11a3 3 0 0 1 3-3h22a3 3 0 0 1 3 3v11a3 3 0 0 1-3 3H5a3 3 0 0 1-3-3Z" stroke="currentColor" strokeWidth="1.6"/><path d="M2 12V7a3 3 0 0 1 3-3h7l3 3" stroke="currentColor" strokeWidth="1.6"/></svg><span><strong>{item.name}</strong><small>{item.library?'Bibliothèque GameFlex':'Dossier'}</small></span><Icon name="arrow" size={17}/></button>):<div className="gf-folder-state"><Icon name="library" size={30}/><strong>{location?'Ce dossier est vide':'Aucun dossier accessible'}</strong><p>{location?'Remontez pour choisir un dossier.':'Aucun dossier monté.'}</p></div>}{listing?.truncated&&<p>200 dossiers affichés.</p>}</div>
        <footer className="gf-folder-footer"><div className="gf-folder-selection"><small>{listing?.selectable?'DOSSIER CHOISI':'EMPLACEMENT'}</small><span title={listing?.folder||''}>{listing?.folder||'Choisissez un dossier.'}</span></div>{listing?.selectionHint&&<p className="gf-folder-hint">Choisissez le dossier racine.</p>}<div className="gf-setup-actions"><button type="button" onClick={onLater} disabled={busy}>Plus tard</button><button type="button" onClick={save} disabled={busy||loading||!library?.canConfigure||!listing?.selectable}>{busy?'Enregistrement…':'Utiliser ce dossier'}</button></div></footer>
    </section></div>;
}
