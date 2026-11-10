import {useDialogFocus} from './dialog-focus.js';
import {MetadataContent} from './EmugenDetails.jsx';
import React,{useRef,useState} from 'react';
import {Art,useDirectionalFocus} from './ExperienceView.jsx';
import {displayTitle} from './merge-games.js';
import {editionFields} from './edition-presentation.js';
import {canResumeGame,recentAction} from './game-presentation.js';
import './catalog-game-dialog.css';
export {languageLabel} from './edition-presentation.js';

export default function CatalogGameDialog({game,mode='covers',onClose,onPlay,onFavorite,favorites=[],history=[],collections=[],onAddToCollection}) {
    const candidates=game.variants?.length?game.variants:[game];
    const identity=v=>v.reference||v.downloadId||v.source||JSON.stringify([v.title,v.language,v.region,v.version]);
    const versions=candidates.filter((v,i)=>!candidates.slice(0,i).some(other=>identity(other)===identity(v)));
    const [selected,setSelected]=useState(()=>{
        const owned=versions.findIndex(v=>v.reference);
        if(owned>=0)return owned;
        const french=versions.findIndex(v=>v.language?.split(',').includes('fr'));
        return Math.max(0,french);
    });
    const version=versions[selected]||versions[0],fields=editionFields({...game,...version});
    const metadata={game:{...game,region:'',version:'',languages:[]},loading:false,error:'',choices:[],removed:true};
    const root=useRef(null);useDirectionalFocus(root);useDialogFocus(root);
    const key=e=>{
        if(e.key==='Escape'){e.stopPropagation();onClose();}
        if(e.key==='Tab'){
            const nodes=[...root.current.querySelectorAll('button:not(:disabled),select,a[href]')],first=nodes[0],last=nodes[nodes.length-1];
            if(e.shiftKey&&document.activeElement===first){e.preventDefault();last?.focus();}
            else if(!e.shiftKey&&document.activeElement===last){e.preventDefault();first?.focus();}
        }
    };
    const gameId=`${version.consoleId||game.consoleId}:${version.reference}`;
    const action=canResumeGame({...game,...version})?'Reprendre':history.includes(gameId)?recentAction({...game,...version}):'Jouer';
    return <div className={`gf-catalogue-backdrop gf-theme-${mode}`} data-interface-theme={mode}>
        <section role="dialog" aria-modal="true" aria-labelledby="gf-catalogue-title" className="gf-catalogue-dialog" ref={root} onKeyDown={key}>
            <header><small>FICHE DU JEU</small><button onClick={onClose} aria-label="Fermer la fiche">×</button></header>
            <div className="gf-catalogue-hero"><Art item={{...game,...version,image:game.image||version.image,attribution:game.image?game.attribution:version.attribution}}/>
                <div><h2 id="gf-catalogue-title">{displayTitle(game.title)}</h2><p className="gf-catalogue-status">{version.reference?'Installé':'Non installé'}</p><small>{versions.length>1?`${versions.length} éditions`:version.reference?'Édition installée':'Édition disponible'}</small></div>
            </div>
            {versions.length>1&&<><label htmlFor="gf-game-version">Langue et édition</label><select id="gf-game-version" value={selected} onChange={e=>setSelected(Number(e.target.value))}>{versions.map((v,i)=>{
                const details=editionFields({...game,...v});
                return <option key={identity(v)} value={i}>{[details.edition,details.region,details.language].filter(Boolean).join(' · ')}</option>;
            })}</select></>}
            <dl className="gf-edition-details">{fields.edition&&<><dt>Édition</dt><dd>{fields.edition}</dd></>}{fields.region&&<><dt>Région</dt><dd>{fields.region}</dd></>}{fields.language!=='Langue non précisée'&&<><dt>Langues</dt><dd>{fields.language}</dd></>}</dl>
            <MetadataContent state={metadata}/>
            <div className="gf-catalogue-actions">{version.reference?<button onClick={()=>onPlay({...game,...version,title:game.title,image:game.image})}>{action}</button>:<p>Importez votre fichier personnel pour jouer.</p>}</div>
            {version.reference&&<div className="em-collection-actions">{onFavorite&&<button aria-pressed={favorites.includes(gameId)} onClick={()=>onFavorite(version)}>Favori</button>}{collections.map(c=><button key={c.id} aria-pressed={c.games.includes(gameId)} onClick={()=>onAddToCollection?.(c.id,version)}>{c.games.includes(gameId)?'✓ ': '+ '}{c.name}</button>)}</div>}
            <footer><button onClick={onClose}>Retour</button></footer>
        </section>
    </div>;
}
