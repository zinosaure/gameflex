import React,{useEffect,useRef,useState} from 'react';
import {uploadPersonalFile} from './personal-upload.js';
import {libraryRequest} from './LibraryManager.jsx';
import {useDirectionalFocus} from './ExperienceView.jsx';
import './catalog-game-dialog.css';

export default function RomImportDialog({token,consoles=[],initialConsole='',onClose,onImported}){
    const [consoleId,setConsoleId]=useState(initialConsole),[file,setFile]=useState(null),[formats,setFormats]=useState(null),[busy,setBusy]=useState(false),[progress,setProgress]=useState(0),[error,setError]=useState('');
    const root=useRef(null),request=useRef(null);useDirectionalFocus(root);
    useEffect(()=>{const controller=new AbortController();libraryRequest(token,'imports',{signal:controller.signal}).then(x=>setFormats(x.formats)).catch(()=>setError('Impossible de charger les consoles. Fermez puis réessayez.'));return()=>{controller.abort();request.current?.abort();};},[token]);
    useEffect(()=>{const previous=document.activeElement;root.current?.querySelector('select')?.focus();return()=>previous?.focus();},[]);
    const submit=async()=>{
        if(!file||!consoleId||busy)return;
        if(file.size===0){setError('Choisissez un fichier non vide.');return;}
        const extension=file.name.split('.').pop().toLowerCase();
        if(!formats?.[consoleId]?.includes(extension)){setError('Format incompatible avec cette console.');return;}
        setError('');setBusy(true);setProgress(0);
        const controller=new AbortController();request.current=controller;
        try{
            const item=await uploadPersonalFile(token,file,consoleId,setProgress,controller.signal);
            request.current=null;await onImported(item);onClose();
        }catch(e){setError(e.name==='AbortError'?'Import annulé.':e.message);}
        finally{request.current=null;setBusy(false);}
    };
    const key=e=>{if(e.key==='Escape'&&!busy){e.stopPropagation();onClose();}if(e.key==='Tab'){const nodes=[...root.current.querySelectorAll('button:not(:disabled),select:not(:disabled),input:not(:disabled)')];const first=nodes[0],last=nodes[nodes.length-1];if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus();}else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus();}}};
    return <div className="gf-catalogue-backdrop"><section className="gf-catalogue-dialog gf-import-dialog" ref={root} role="dialog" aria-modal="true" aria-labelledby="gf-import-title" onKeyDown={key}><header><small>VOTRE BIBLIOTHÈQUE</small><button onClick={onClose} disabled={busy} aria-label="Fermer l’import">×</button></header><h2 id="gf-import-title">Ajouter une ROM</h2><label htmlFor="gf-import-console">Console</label><select id="gf-import-console" value={consoleId} disabled={busy||!formats} onChange={e=>{setConsoleId(e.target.value);setFile(null);setError('');}}><option value="">Choisir une console…</option>{consoles.filter(c=>formats?.[c.id]).map(c=><option key={c.id} value={c.id}>{c.name}</option>)}</select><label htmlFor="gf-import-file">Fichier de jeu</label><input key={consoleId} id="gf-import-file" type="file" accept={(formats?.[consoleId]||[]).map(x=>'.'+x).join(',')} disabled={busy||!consoleId} onChange={e=>{setFile(e.target.files?.[0]||null);setError('');}}/><small>Fichier décompressé, sauf ZIP d’arcade.</small>{busy&&<div role="status"><p>{progress===0?'Préparation du fichier…':progress<99?`Reçu sur le Pi · ${progress} %`:progress===100?'Actualisation…':'Vérification et rangement…'}</p><progress max="100" value={progress}/></div>}{error&&<p role="alert">{error}</p>}<footer><button onClick={()=>{if(busy)request.current?.abort();else onClose();}}>{busy?'Annuler l’envoi':'Annuler'}</button><button disabled={busy||!file||!consoleId||!formats} onClick={submit}>{busy?'Import en cours…':'Importer'}</button></footer></section></div>;
}
