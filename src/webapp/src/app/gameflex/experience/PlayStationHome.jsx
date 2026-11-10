import ConsoleEmulatorBanner from './ConsoleEmulatorBanner.jsx';
import {appPath} from '../libs/appBase.js';
import React,{useEffect,useRef,useState} from 'react';
import {Art,useDirectionalFocus} from './ExperienceView.jsx';
import {Icon} from './EmulatorUI.jsx';
import {useDialogFocus} from './dialog-focus.js';
import {matchesGameSearch} from './catalogue-search.js';
import {gameStatusCode,gameStatusLabel,canResumeGame} from './game-presentation.js';
import {libraryWithDownloads} from './download-state.js';
import SectionErrorBoundary from './SectionErrorBoundary.jsx';
import './playstation-home.css';

const key=game=>`${game.consoleId}:${game.reference||game.id||game.title}`;
export const shortTitle=game=>(game?.displayTitle||game?.title||'').replace(/\.(?:gba|gbc|gb|nes|sfc|smc|z64|n64|nds|iso|bin|chd|zip|7z)$/i,'').replace(/\s*[([][^)\]]*[)\]]/g,'').trim();
const sections=[['home','Accueil','home'],['library','Bibliothèque','library'],['favorites','Favoris','favorites'],['resume','Récents','resume'],['collections','Collections','collections'],['consoles','Consoles','consoles'],['multiplayer','Multijoueur','multiplayer'],['management','Gestion / Archives','archives'],['settings','Paramètres','settings']];
const time=()=>new Date().toLocaleTimeString('fr-FR',{hour:'2-digit',minute:'2-digit'});
function ControlCenter({onClose,onNavigate}){
 const root=useRef(null);useDialogFocus(root);useDirectionalFocus(root);
 return <div className="ps-control-backdrop" onClick={e=>{if(e.target===e.currentTarget)onClose();}}><section ref={root} className="ps-control" role="dialog" aria-modal="true" aria-label="Centre de contrôle" onKeyDown={e=>{if(e.key==='Escape'){e.preventDefault();onClose();}if(e.key==='Tab'){const buttons=[...root.current.querySelectorAll('button')];if(e.shiftKey&&document.activeElement===buttons[0]){e.preventDefault();buttons.at(-1)?.focus();}else if(!e.shiftKey&&document.activeElement===buttons.at(-1)){e.preventDefault();buttons[0]?.focus();}}}}>
 <header><div><small>GAMEFLEX</small><h2>Centre de contrôle</h2></div><button aria-label="Fermer le centre de contrôle" onClick={onClose}><Icon name="close"/></button></header>
 <nav>{sections.map(([id,label,icon])=><button key={id} onClick={()=>{onClose();onNavigate(id);}}><Icon name={icon}/><span>{label}</span><span aria-hidden="true">↗</span></button>)}</nav>
 </section></div>;
}
function NewCollection({onClose,onCreate}){
 const root=useRef(null),[name,setName]=useState('');useDialogFocus(root);
 useEffect(()=>{root.current?.querySelector('input')?.focus();},[]);
 return <div className="ps-control-backdrop"><section ref={root} className="ps-control" role="dialog" aria-modal="true" aria-label="Nouvelle collection" onKeyDown={event=>{if(event.key==='Escape'){event.preventDefault();onClose();}if(event.key==='Tab'){const controls=[...root.current.querySelectorAll('input,button:not(:disabled)')];if(event.shiftKey&&document.activeElement===controls[0]){event.preventDefault();controls.at(-1)?.focus();}else if(!event.shiftKey&&document.activeElement===controls.at(-1)){event.preventDefault();controls[0]?.focus();}}}}><header><h2>Nouvelle collection</h2><button aria-label="Fermer" onClick={onClose}><Icon name="close"/></button></header><form className="ps-new-collection" onSubmit={event=>{event.preventDefault();if(name.trim()){onCreate(name.trim());onClose();}}}><label>Nom de la collection<input autoComplete="off" maxLength={80} required value={name} onChange={event=>setName(event.target.value)}/></label><div><button type="button" onClick={onClose}>Annuler</button><button type="submit" disabled={!name.trim()}>Créer</button></div></form></section></div>;
}
export function PlayStationNavigation({active='home',onNavigate=()=>{},onSettings,user='Joueur',onSearch}){
 const [control,setControl]=useState(false),[clock,setClock]=useState(time);
 useEffect(()=>{const timer=setInterval(()=>setClock(time()),30000);return()=>clearInterval(timer);},[]);
 const go=id=>id==='settings'&&onSettings?onSettings():onNavigate(id);
 return <><header className="ps-topbar"><button className="ps-brand" aria-label="Accueil GameFlex" onClick={()=>go('home')}><img className="ps-app-logo" src={appPath('/images/icon-192.png')} alt="" width="44" height="44"/></button>
 <nav className="ps-tabs" aria-label="Navigation principale">{[['home','Jeux'],['library','Bibliothèque'],['collections','Collections']].map(([id,label])=><button key={id} aria-current={active===id?'page':undefined} onClick={()=>go(id)}>{label}</button>)}</nav>
 <div className="ps-utilities"><button aria-label="Rechercher un jeu" onClick={()=>{go('library');onSearch?.();}}><Icon name="search"/></button><button aria-label="Paramètres" onClick={()=>go('settings')}><Icon name="settings"/></button><button className="ps-avatar" aria-label={`Centre de contrôle · ${user}`} onClick={()=>setControl(true)}>{String(user||'J').slice(0,1).toUpperCase()}</button><time>{clock}</time></div></header>
 {control&&<ControlCenter onClose={()=>setControl(false)} onNavigate={go}/>}</>;
}
function Empty({query,section,onNavigate}){
 const text=query?'Aucun jeu ne correspond à votre recherche.':section==='favorites'?'Vos coups de cœur vous attendent ici.':section==='resume'?'Vos dernières sessions apparaîtront ici.':'Aucun jeu dans cette rubrique.';
 return <div className="ps-empty"><Icon name={section==='favorites'?'favorites':'library'} size={40}/><h2>{query?'Aucun résultat':section==='favorites'?'Gardez vos favoris à portée de main':section==='resume'?'Votre prochaine partie commence ici':'Votre bibliothèque prend vie ici'}</h2><p>{text}</p>{!query&&<button onClick={()=>onNavigate('library')}>Parcourir la bibliothèque</button>}</div>;
}
export default function PlayStationHome({games:suppliedGames=[],downloadJobs=[],live=[],onWatch,downloadable=[],consoles=[],favorites=[],history=[],collections=[],onOpen=()=>{},onDirectLaunch,onGameOptions,launchingKey='',directError,onFavorite=()=>{},onConsole=()=>{},onSettings,onNavigate=()=>{},section='home',settingsContent,loading,error,onRetry,onCreateCollection,onRefreshLibrary,refreshing,onImportRom,user='Joueur',selectedConsole,libraryQuery,onLibraryQuery,onSearchQuery}){
 const games=libraryWithDownloads(suppliedGames,downloadJobs);
 const [newCollection,setNewCollection]=useState(false);
 const root=useRef(null),search=useRef(null);useDirectionalFocus(root);
 const [localQuery,setLocalQuery]=useState(''),[system,setSystem]=useState(''),[status,setStatus]=useState(''),[active,setActive]=useState(()=>{try{return sessionStorage.getItem('gameflex:active-console')||'';}catch{return '';}}),[limit,setLimit]=useState(48),[source,setSource]=useState('installed');
 const query=libraryQuery??localQuery,setQuery=onLibraryQuery||setLocalQuery;
 const currentSection=selectedConsole?'library':section==='dashboard'?'home':section==='archives'?'management':section;
 const installed=games.filter(game=>game.reference&&!game.pendingDownload), count=installed.length;
 const candidates=source==='discover'&&selectedConsole?downloadable:games;
 const filtered=candidates.filter(game=>matchesGameSearch(game,query)&&(!system||game.consoleId===system)&&(!status||gameStatusCode(game.status)===status));
 const ownedConsoles=consoles.filter(console=>installed.some(game=>game.consoleId===console.id));
 const selectedSystem=ownedConsoles.find(console=>console.id===(selectedConsole?.id||active))||ownedConsoles[0];
 const consoleGames=installed.filter(game=>game.consoleId===selectedSystem?.id);
 const consoleList=consoleGames.filter(game=>matchesGameSearch(game,query));
 const selectConsole=console=>{setActive(console.id);setQuery('');try{sessionStorage.setItem('gameflex:active-console',console.id);}catch{}};
 const list=currentSection==='favorites'?filtered.filter(game=>favorites.includes(key(game))):currentSection==='resume'?history.map(id=>filtered.find(game=>key(game)===id)).filter(Boolean):currentSection==='multiplayer'?filtered.filter(game=>Number(consoles.find(c=>c.id===game.consoleId)?.maxPlayers)>=2):filtered;
 useEffect(()=>{onSearchQuery?.(source==='discover'?query:'');},[query,source]);
 useEffect(()=>{setLimit(48);},[query,system,status,currentSection,source]);
 useEffect(()=>{setSystem('');setStatus('');setSource('installed');},[selectedConsole?.id]);
 const open=game=>onOpen(game);
 const move=(event,index)=>{if(!['ArrowLeft','ArrowRight'].includes(event.key))return;event.preventDefault();const buttons=[...root.current.querySelectorAll('.ps-console-tile')];const next=buttons[Math.max(0,Math.min(buttons.length-1,index+(event.key==='ArrowRight'?1:-1)))];next?.focus();next?.scrollIntoView({block:'nearest',inline:'nearest',behavior:'smooth'});};
 const card=game=><article className="ps-game-row" key={key(game)}>
 <button className="ps-row-art" data-focus-key={key(game)} onClick={()=>open(game)} aria-label={`Fiche : ${shortTitle(game)}`}><Art item={game}/></button>
 <div className="ps-row-info"><button className="ps-row-title" onClick={()=>open(game)}><h3>{shortTitle(game)}</h3></button><p>{game.consoleName||consoles.find(c=>c.id===game.consoleId)?.name||game.consoleId}{currentSection==='management'?` · ${gameStatusLabel(game.status)}`:''}</p>{directError?.key===key(game)&&<p className="ps-row-error" role="alert">{directError.message}</p>}</div>
 <div className="ps-row-actions"><button className="ps-row-launch" disabled={Boolean(launchingKey)||game.pendingDownload} onClick={()=>game.reference?onDirectLaunch?.(game):open(game)} aria-label={`${game.reference?'Lancer':'Fiche de'} ${shortTitle(game)}`}><Icon name="play" size={18}/><span>{launchingKey===key(game)?'Lancement…':game.reference?'Lancer':'Voir la fiche'}</span></button><button className="ps-favorite" aria-label={`${favorites.includes(key(game))?'Retirer des':'Ajouter aux'} favoris : ${shortTitle(game)}`} aria-pressed={favorites.includes(key(game))} onClick={()=>onFavorite(game)}><Icon name="favorites" size={20}/></button><button className="ps-row-more" data-focus-key={`settings:${key(game)}`} aria-label={`Paramètres de ${shortTitle(game)}`} onClick={()=>onGameOptions?.(game)}><Icon name="more" size={23}/></button></div>
 </article>;
 const grid=items=>items.length?<><div className="ps-game-list">{items.slice(0,limit).map(card)}</div>{items.length>limit&&<button className="ps-load-more" onClick={()=>setLimit(n=>n+48)}>Afficher plus de jeux</button>}</>:<Empty query={query} section={currentSection} onNavigate={onNavigate}/>;
 const title=currentSection==='library'?(selectedConsole?.name||'Votre bibliothèque'):sections.find(x=>x[0]===currentSection)?.[1]||'Votre bibliothèque';
 const toolbar=<div className="ps-library-toolbar"><label className="ps-search"><Icon name="search" size={20}/><input ref={search} type="search" aria-label="Rechercher dans mes jeux" placeholder="Rechercher un jeu" value={query} onChange={event=>setQuery(event.target.value)}/></label><label className="ps-filter"><span>Console</span><select aria-label="Filtrer par console" value={system} onChange={event=>setSystem(event.target.value)}><option value="">Toutes les consoles</option>{consoles.map(c=><option key={c.id} value={c.id}>{c.name}</option>)}</select></label>{currentSection==='management'&&<label className="ps-filter"><span>État</span><select aria-label="Filtrer par état" value={status} onChange={event=>setStatus(event.target.value)}><option value="">Tous les états</option>{[...new Set(games.map(g=>gameStatusCode(g.status)).filter(Boolean))].map(s=><option key={s} value={s}>{gameStatusLabel(s)}</option>)}</select></label>}<span className="ps-result-count" role="status">{list.length} résultat{list.length===1?'':'s'}</span></div>;
 return <div ref={root} className={`ps-console ${currentSection==='home'?'ps-is-home':''}`} data-interface="ps-home">
 <PlayStationNavigation active={currentSection} onNavigate={onNavigate} onSettings={onSettings} user={user} onSearch={()=>setTimeout(()=>search.current?.focus(),50)}/>
 <main id="ps-main"><SectionErrorBoundary resetKey={`${currentSection}:${loading}`} onRetry={onRetry}>
 {loading&&<div className="ps-loading" role="status"><span/>Chargement de votre bibliothèque…</div>}
 {error&&<div className="ps-error" role="alert"><p>{error}</p><button onClick={onRetry}>Réessayer</button></div>}
 {!loading&&!error&&!installed.length&&currentSection==='home'?<Empty section="home" onNavigate={onNavigate}/>:null}
 {currentSection==='home'&&selectedSystem&&<>
 <div className="ps-home-backdrop ps-console-backdrop" key={selectedSystem.id} aria-hidden="true"><Art item={selectedSystem} kind="console" wide/></div>
 <section className="ps-recent-strip ps-consoles-strip" aria-label="Vos consoles"><div className="ps-game-rail">{ownedConsoles.map((console,index)=><button key={console.id} className="ps-console-tile" data-system={console.id} aria-pressed={console.id===selectedSystem.id} aria-label={`Sélectionner ${console.name}`} onFocus={()=>selectConsole(console)} onClick={()=>selectConsole(console)} onKeyDown={event=>move(event,index)}><Art item={console} kind="console"/><span>{console.name}</span></button>)}</div></section>
 <section className="ps-console-library" aria-labelledby="ps-console-title"><header className="ps-console-heading"><div><span className="ps-eyebrow">VOS CONSOLES · VOS JEUX</span><h1 id="ps-console-title">{selectedSystem.name}</h1><p>{consoleGames.length} jeu{consoleGames.length===1?'':'x'} installé{consoleGames.length===1?'':'s'}<span> · </span>{count} référence{count===1?'':'s'} dans votre bibliothèque</p></div><label className="ps-search"><Icon name="search" size={20}/><input type="search" aria-label="Rechercher dans cette console" placeholder="Rechercher un jeu" value={query} onChange={event=>setQuery(event.target.value)}/></label></header>
 <ConsoleEmulatorBanner key={selectedSystem.id} consoleId={selectedSystem.id} consoleName={selectedSystem.name} onReady={onRefreshLibrary}/>
 {consoleGames.length>0&&grid(consoleList)}
 </section>
 </>}
 {currentSection!=='home'&&<div className="ps-page"><header className="ps-page-heading"><div><span className="ps-eyebrow">VOTRE ESPACE GAMEFLEX</span><h1>{title}</h1>{currentSection!=='settings'&&<p className="em-installed-total">{loading?'Chargement…':`${count} référence${count===1?'':'s'} installée${count===1?'':'s'}`}</p>}</div><div>{onImportRom&&<button className="ps-subtle" onClick={onImportRom}>＋ Ajouter un jeu</button>}{onRefreshLibrary&&<button className="ps-icon-button" aria-label="Actualiser la bibliothèque" disabled={refreshing} onClick={onRefreshLibrary}><Icon name="refresh"/></button>}</div></header>
 {currentSection==='settings'?<div className="ps-settings-content">{settingsContent}</div>:currentSection==='consoles'?<div className="ps-system-grid">{ownedConsoles.map(c=><button key={c.id} onClick={()=>onConsole(c)}><Art item={c} kind="console"/><strong>{c.name}</strong><span>{games.filter(g=>g.consoleId===c.id).length} jeu{games.filter(g=>g.consoleId===c.id).length===1?'':'x'}</span></button>)}</div>:currentSection==='collections'?<><div className="ps-collection-heading"><p>Vos jeux, à votre façon.</p><button className="ps-subtle" onClick={()=>setNewCollection(true)}>＋ Créer une collection</button></div>{collections.length?collections.map(c=><section className="ps-collection" key={c.id}><h2>{c.name}</h2>{grid(games.filter(g=>c.games.includes(key(g))))}</section>):<Empty section="collections" onNavigate={onNavigate}/>}</>:<>{selectedConsole&&downloadable.length>0&&<div className="ps-source-tabs"><button aria-pressed={source==='installed'} onClick={()=>setSource('installed')}>Installés</button><button aria-pressed={source==='discover'} onClick={()=>setSource('discover')}>À découvrir</button></div>}{currentSection==='multiplayer'&&live.length>0&&<section className="ps-live-sessions"><h2>Parties en cours</h2>{live.map((session,index)=><button key={session.id||index} onClick={()=>onWatch?.(session)}><Icon name="multiplayer"/><span>{typeof session.title==='string'?session.title:'Partie en cours'}</span><span>Rejoindre</span></button>)}</section>}{selectedConsole&&<ConsoleEmulatorBanner key={selectedConsole.id} consoleId={selectedConsole.id} consoleName={selectedConsole.name} onReady={onRefreshLibrary}/>}{toolbar}{!loading&&!(error&&!games.length)&&grid(list)}{currentSection==='management'&&<p className="ps-archive-count">{list.reduce((n,g)=>n+(g.files?.length||0),0)} fichiers · {list.length} références</p>}</>}
 </div>}
 </SectionErrorBoundary></main>{newCollection&&<NewCollection onClose={()=>setNewCollection(false)} onCreate={onCreateCollection}/>}<footer className="ps-footer"><button aria-label="Ouvrir le centre de contrôle" onClick={()=>root.current.querySelector('.ps-avatar')?.click()}><img className="ps-footer-logo" src={appPath('/images/icon-192.png')} alt="" width="26" height="26"/><span>Centre de contrôle</span></button><span className="ps-controller-hint"><kbd>←</kbd><kbd>→</kbd> Naviguer <kbd>↵</kbd> Sélectionner</span><span>GAMEFLEX</span></footer>
 </div>;
}
