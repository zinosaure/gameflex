import React,{useState} from 'react';
import {useSourceSettings} from './SourceSettings.jsx';
import {Icon} from './EmulatorUI.jsx';
import './game-sources.css';
export default function GameSources(){
 const settings=useSourceSettings(),[saving,setSaving]=useState(false),[message,setMessage]=useState(''),[error,setError]=useState('');
 async function toggle(source){
  setSaving(true);setMessage('');setError('');
  try{await settings.save(source.enabled?settings.enabledIds.filter(id=>id!==source.id):[...settings.enabledIds,source.id]);setMessage('Enregistré.');}
  catch(e){setError(e.message);}finally{setSaving(false);}
 }
 return <section className="gf-game-sources" aria-labelledby="gf-sources-title"><header><Icon name="library" size={20}/><h3 id="gf-sources-title">Fournisseurs</h3><small>{settings.enabledIds.length} actif(s)</small></header>{!settings.ready&&<p role="status">Chargement des fournisseurs…</p>}{(error||settings.error)&&<p role="alert">{error||settings.error}</p>}{message&&<p role="status">{message}</p>}<div className="gf-source-list">{settings.items.map(source=><article key={source.id} data-source={source.id} data-enabled={source.enabled}><div className="gf-source-identity"><span className="gf-source-symbol" aria-hidden="true">{source.id==='emugen'?'1':source.enabled?'✓':'−'}</span><div><strong>{source.name}{source.id==='emugen'&&<span className="gf-provider-priority">Prioritaire</span>}</strong><small>{source.enabled?'Activé':'En sourdine'}</small></div></div><label className="gf-provider-switch"><span className="gx-sr">Activer {source.name}</span><input type="checkbox" role="switch" aria-label={`Activer ${source.name}`} checked={source.enabled} disabled={!settings.canConfigure||!settings.ready||saving} onChange={()=>toggle(source)}/><span aria-hidden="true"/></label><p>{source.id==='emugen'?'Fiches et liens':source.label}</p><details><summary>Détails</summary><p>{source.description}</p></details><a href={source.url} target={source.kind==='approved-local'?undefined:'_blank'} rel={source.kind==='approved-local'?undefined:'noopener noreferrer'}>Consulter ↗</a></article>)}</div>{!settings.canConfigure&&settings.ready&&<p>Modification réservée à l’administrateur.</p>}</section>;
}
