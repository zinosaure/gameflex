// Fixture-only entry. This token is a local stub, never a production credential.
import React,{useState} from 'react';
import {createRoot} from 'react-dom/client';
import InteractiveVerification from '../../src/webapp/src/app/gameflex/experience/InteractiveVerification.jsx';
function Preview(){
 const [result,setResult]=useState(null),[open,setOpen]=useState(true);
 return <main style={{fontFamily:'system-ui',padding:30}}><h1>GameFlex · Fiche de test</h1><p>EmuGen · Démo libre</p>{open&&<InteractiveVerification token="fixture-alice" sessionId={new URLSearchParams(location.search).get('session')} gameSlug="free-demo-wii" consoleId="wii" onClose={()=>setOpen(false)} onResolved={setResult}/>}{result?.links.map(link=><a key={link.url} href={link.url}>{link.label}</a>)}{!open&&<button onClick={()=>setOpen(true)}>Vérifier EmuGen</button>}</main>;
}
createRoot(document.getElementById('root')).render(<Preview/>);
