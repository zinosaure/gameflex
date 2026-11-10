import React,{useState} from 'react';
import {useAuth} from '../profile/auth/AuthContext.jsx';
import {useConsoles} from '../contexts/ConsolesContext.jsx';
import NetworkSources from './NetworkSources.jsx';
import RomImportDialog from './RomImportDialog.jsx';
export default function LibrarySources({onImported}){
 const {token,user}=useAuth(),consoles=useConsoles();
 const [importing,setImporting]=useState(false),[networkOpen,setNetworkOpen]=useState(false);
 if(user?.role_level!==0)return null;

 return <section className="gf-library-sources"><h3>Sources de jeux</h3><div className="gf-library-source-actions"><button onClick={()=>setImporting(true)}>＋ Ajouter un fichier</button><button onClick={()=>setNetworkOpen(true)}>＋ Source réseau</button></div><small>Samba, WebDAV ou kDrive.</small>{networkOpen&&<NetworkSources token={token} onClose={()=>setNetworkOpen(false)}/>} {importing&&<RomImportDialog token={token} consoles={consoles.filter(c=>c.method==='retrogaming'&&c.enabled!==false)} onClose={()=>setImporting(false)} onImported={onImported}/>}</section>;
}
