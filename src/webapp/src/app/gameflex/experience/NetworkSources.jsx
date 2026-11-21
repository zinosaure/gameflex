import React,{useEffect,useState} from 'react';
import {listRemoteMounts,checkRemoteMount,addRemoteMount,rescanRemoteMount} from '../profile/api.js';
export default function NetworkSources({token,onClose}){
 const [items,setItems]=useState([]),[busy,setBusy]=useState(false),[error,setError]=useState(''),[message,setMessage]=useState('');
 const [form,setForm]=useState({name:'',kind:'smb',host:'',share:'',baseUrl:'',driveId:'',folder:'',username:'',password:'',anonymous:true});
 const change=(key,value)=>setForm(f=>({...f,[key]:value}));
 const load=async()=>{const r=await listRemoteMounts(token);if(!r.ok)throw new Error(r.errorMessage||'Sources indisponibles.');setItems(r.items||[]);};
 useEffect(()=>{load().catch(e=>setError(e.message));},[token]);
 const problem=r=>r.exception?.detail||r.errorMessage||'Connexion impossible.';
 async function save(e){
  e.preventDefault();setBusy(true);setError('');setMessage('');
  const anonymous=form.kind==='smb'&&form.anonymous;
  const candidate={kind:form.kind,host:form.host.trim(),port:445,share:form.share.trim(),base_url:form.baseUrl.trim(),drive_id:form.driveId.trim(),folder:form.folder.trim(),username:anonymous?'':form.username.trim(),password:anonymous?'':form.password};
  try{
   const probe=await checkRemoteMount(token,candidate),seen=probe.item||probe.data||{};
   if(!probe.ok||!seen.reachable)throw new Error(seen.detail||problem(probe));
   const r=await addRemoteMount(token,{...candidate,name:form.name.trim(),baseUrl:candidate.base_url,driveId:candidate.drive_id});
   if(!r.ok)throw new Error(problem(r));change('password','');await load();setMessage('Source ajoutée.');
  }catch(e){setError(e.message);}finally{setBusy(false);}
 }
 async function scan(item){setBusy(true);setError('');try{const r=await rescanRemoteMount(token,item.id);if(!r.ok)throw new Error(problem(r));setMessage('Analyse de la source lancée.');}catch(e){setError(e.message);}finally{setBusy(false);}}
 const field=(key,label,props={})=><label>{label}<input value={form[key]} onChange={e=>change(key,e.target.value)} disabled={busy} {...props}/></label>;
 return <div className="gf-catalogue-backdrop"><section role="dialog" aria-modal="true" aria-labelledby="gf-network-title" className="gf-catalogue-dialog gf-network-dialog"><header><h2 id="gf-network-title">Sources réseau</h2><button onClick={onClose} disabled={busy} aria-label="Fermer les sources réseau">×</button></header>{items.map(item=><article key={item.id}><strong>{item.name}</strong><small> · {item.kind}</small><button onClick={()=>scan(item)} disabled={busy}>Analyser</button></article>)}<form onSubmit={save}><h3>Ajouter une source</h3>{field('name','Nom',{required:true,maxLength:80})}<label>Type<select value={form.kind} disabled={busy} onChange={e=>change('kind',e.target.value)}><option value="smb">Samba (SMB)</option><option value="webdav">WebDAV</option><option value="kdrive">kDrive</option></select></label>{form.kind==='smb'?<>{field('host','Serveur',{required:true,placeholder:'Adresse du NAS'})}{field('share','Partage',{required:true})}<label><input type="checkbox" checked={form.anonymous} disabled={busy} onChange={e=>change('anonymous',e.target.checked)}/>Accès invité</label></>:form.kind==='webdav'?field('baseUrl','Adresse WebDAV',{required:true,type:'url',placeholder:'https://…'}):field('driveId','Identifiant kDrive',{required:true})}{field('folder','Dossier (facultatif)')}{!(form.kind==='smb'&&form.anonymous)&&<>{field('username','Identifiant',{autoComplete:'off'})}{field('password','Mot de passe',{type:'password',autoComplete:'new-password'})}</>}{error&&<p role="alert">{error}</p>}{message&&<p role="status">{message}</p>}<footer><button type="button" onClick={onClose} disabled={busy}>Fermer</button><button disabled={busy}>{busy?'Vérification…':'Vérifier et ajouter'}</button></footer></form></section></div>;
}
