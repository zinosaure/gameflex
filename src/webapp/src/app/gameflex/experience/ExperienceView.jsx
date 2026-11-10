import RetroLibrary,{retroModes} from './RetroLibrary.jsx';
import HubCanape from './HubCanape.jsx';
import LigneCinema from './LigneCinema.jsx';
import MosaiqueJoy from './MosaiqueJoy.jsx';
import RoueRetro from './RoueRetro.jsx';
import TerminalPoche from './TerminalPoche.jsx';
import Cartouches from './Cartouches.jsx';
import CarteEpoques from './CarteEpoques.jsx';
import DoubleEcran from './DoubleEcran.jsx';
import ClubLocal from './ClubLocal.jsx';
import QuaiSystemes from './QuaiSystemes.jsx';
import MagazineRetro from './MagazineRetro.jsx';
import BureauCollectionneur from './BureauCollectionneur.jsx';
import GrandEcranAccessible from './GrandEcranAccessible.jsx';
import { apiUrl } from "../libs/serverBase.js";
import {DownloadOverlay,DownloadList} from './DownloadProgress.jsx';
import {libraryWithDownloads,jobForGame} from './download-state.js';
import { foldSearch, matchesGameSearch } from './catalogue-search.js';
import { mergeGames } from './merge-games.js';
import React, { useEffect, useRef, useState } from 'react';
import { interfaces, selectableInterfaces } from './interfaces.js';
import './experience.css';
import './emulator-layouts.css';
import { Icon, LayoutMiniature, EmulatorNavigation, GameBrowser, SystemBrowser, ConsoleIdentity, destinations } from './EmulatorUI.jsx';

export function InterfacePicker({ onChoose, selected, onCancel, options = selectableInterfaces }) {
    const [query, setQuery] = useState('');
    const choices = options.filter(x => x.name.toLocaleLowerCase('fr').includes(query.toLocaleLowerCase('fr')));
    const ref = useRef(null); useDirectionalFocus(ref);
    useEffect(() => { (ref.current?.querySelector('[aria-pressed="true"]') || ref.current?.querySelector('button'))?.focus(); }, [selected]);
    return <section ref={ref} className="gx-picker" aria-label="Choix de l’interface">
        <small>GAMEFLEX</small><h1>Choisir une interface</h1>
        <p>Modifiable dans les paramètres.</p>
        <label className="gx-search">Rechercher une interface<input type="search" value={query} onChange={e => setQuery(e.target.value)}/></label>
        <p role="status">{choices.length} interface{choices.length === 1 ? '' : 's'}</p>
        <div className="gx-choices">{choices.map((x, i) => <button key={x.id} className={`gx-choice ${retroModes.includes(x.id) ? `gf-choice-${x.id}` : `gx-${x.id}`}`} aria-pressed={selected === x.id} onClick={() => onChoose(x.id)}>
            <LayoutMiniature mode={x.id}/>
            <small>{String(i + 1).padStart(2, '0')}</small><strong>{x.name}</strong><span>{x.note}</span>
        </button>)}</div>{onCancel && <button onClick={onCancel}>Retour</button>}
    </section>;
}

// TV arrows operate on the actual rendered geometry, also on touch layouts.
export function useDirectionalFocus(ref) {
    useEffect(() => {
        const root = ref.current;
        if (!root) return undefined;
        const onKey = e => {
            if (!['ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown'].includes(e.key) || ['INPUT', 'SELECT', 'TEXTAREA'].includes(e.target.tagName)) return;
            const surface = root.querySelector('[aria-modal="true"]') || root;
            const all = [...surface.querySelectorAll('button:not(:disabled),a[href],input,select,summary')].filter(el => el.getClientRects().length);
            if (!all.length) return;
            const current = document.activeElement;
            if (!all.includes(current)) { all[0].focus(); e.preventDefault(); return; }
            const a = current.getBoundingClientRect(), ax = a.x + a.width / 2, ay = a.y + a.height / 2;
            const dx = e.key === 'ArrowLeft' ? -1 : e.key === 'ArrowRight' ? 1 : 0;
            const dy = e.key === 'ArrowUp' ? -1 : e.key === 'ArrowDown' ? 1 : 0;
            const candidates = all.filter(x => x !== current).map(el => {
                const b = el.getBoundingClientRect(), x = b.x + b.width / 2 - ax, y = b.y + b.height / 2 - ay;
                return { el, ahead: x * dx + y * dy, score: Math.hypot(x, y) + Math.abs(x * dy - y * dx) * 2 };
            }).filter(x => x.ahead > 5).sort((a, b) => a.score - b.score);
            if (candidates[0]) { candidates[0].el.focus(); candidates[0].el.scrollIntoView({ block: 'nearest', inline: 'nearest' }); e.preventDefault(); }
        };
        root.addEventListener('keydown', onKey);
        return () => root.removeEventListener('keydown', onKey);
    }, [ref]);
}

export function Art({ item, wide = false, kind = 'game' }) {
    const identity=`${item.image||''}:${item.fallbackImage||''}`;
    const [fallbackFor,setFallbackFor]=useState(null),[failedFor,setFailedFor]=useState(null),[loadedSrc,setLoadedSrc]=useState('');
    const [visible,setVisible]=useState(()=>typeof IntersectionObserver==='undefined');
    const outer=useRef(null),photo=useRef(null);
    const fallback=fallbackFor===identity&&Boolean(item.fallbackImage)&&item.fallbackImage!==item.image;
    const url=fallback?item.fallbackImage:item.image;
    const allowed=typeof url==='string'&&(/^\/experience-api\/v1\/media\/[a-f0-9]{64}$/.test(url)||/^\/experience-api\/v1\/covers\/[a-z0-9_-]+\/[A-Za-z0-9_-]{1,800}$/.test(url)||/^\/experience-api\/v1\/emugen-art\/[A-Za-z0-9_-]{1,2500}$/.test(url)||/^\/experience-api\/v1\/(?:freeroms-art\/[a-z0-9_-]+\/[a-z0-9_-]{1,200}|romspure-art\/[a-z0-9_-]+\/[a-z0-9~-]{1,301})$/.test(url));
    const requestKey=`${identity}:${url}`;
    const failed=failedFor===requestKey;
    const loaded=loadedSrc===requestKey;
    const fail=()=>{if(!fallback&&item.fallbackImage&&item.fallbackImage!==item.image)setFallbackFor(identity);else setFailedFor(requestKey);};
    useEffect(()=>{
        if(visible||typeof IntersectionObserver==='undefined')return undefined;
        const observer=new IntersectionObserver(entries=>{if(entries.some(entry=>entry.isIntersecting)){setVisible(true);observer.disconnect();}},{rootMargin:'160px'});
        if(outer.current)observer.observe(outer.current);return()=>observer.disconnect();
    },[visible]);
    useEffect(()=>{
        if(!visible||!allowed||failed||loaded)return undefined;
        if(photo.current?.complete&&photo.current.naturalWidth>0){setLoadedSrc(requestKey);return undefined;}
        const timer=setTimeout(()=>{if(photo.current?.complete&&photo.current.naturalWidth>0)setLoadedSrc(requestKey);else fail();},8000);
        return()=>clearTimeout(timer);
    },[requestKey,visible,allowed,failed,loaded]);
    const safe=allowed&&!failed;
    const credit=fallback?(item.fallbackAttribution||'Image du cache local'):(item.attribution||'Image du cache local');
    return <div ref={outer} className={`gx-art ${wide?'gx-wide':''} ${kind==='console'?'gx-console-art':''} ${!safe?'em-art-missing':''}`} style={{'--seed':(item.title||item.name||'').length%7}}>
        {safe?<>{visible&&<img key={requestKey} ref={photo} src={apiUrl(url)} onLoad={()=>setLoadedSrc(requestKey)} onError={fail} decoding="async" loading="eager" fetchpriority="high" alt={kind==='console'?`Photo de ${item.name}`:''}/>} {!loaded&&visible&&<span className="em-cover-loading" role="status">{kind==='console'?'Chargement de la photo…':'Chargement de la jaquette…'}</span>}{loaded&&<span className="gx-credit">{credit}</span>}</>:<><Icon name={kind==='console'?'consoles':'library'} size={32}/><span className="gx-no-cover">{failed?'Image momentanément indisponible':kind==='console'?'Photo indisponible':'Jaquette indisponible'}</span></>}
        <DownloadOverlay job={item.download} pending={item.pendingDownload}/>
    </div>;
}

export default function ExperienceView({ mode, games: suppliedGames = [], downloadJobs = [], consoles = [], favorites = [], history = [], collections = [], onFavorite, onCreateCollection, onAddToCollection, onOpen, onConsole, onSettings, section = 'home', onNavigate = () => {}, settingsContent, planet = {}, downloadable = [], planetLetter = '', onPlanetLetter, onSearchQuery, initialQuery = '', onAppearance, loading, error, onRetry, onRefreshMetadata, metadataProgress, onImportRom, onRefreshLibrary, refreshing = false, refreshMessage = '', user = 'Joueur', live = [], onWatch, selectedConsole, onHome }) {
    const games=libraryWithDownloads(suppliedGames,downloadJobs);
    const [query, setQuery] = useState(initialQuery), [system,setSystem] = useState(''), [status,setStatus] = useState(''), [catalogueView,setCatalogueView] = useState('all');
    const [coverFilter,setCoverFilter]=useState(()=>{try{return sessionStorage.getItem('gameflex:covers-filter')||'all';}catch{return 'all';}}),[coverLetter,setCoverLetter]=useState(()=>{try{return sessionStorage.getItem('gameflex:covers-letter')||'';}catch{return '';}});
    const [page,setPage] = useState({scope:'',count:24});
    const catalogueScope = `${selectedConsole?.id}:${planetLetter}:${query}:${catalogueView}`;
    const visibleCount = page.scope === catalogueScope ? page.count : 24;
    const root = useRef(null); useDirectionalFocus(root);
    useEffect(() => { setSystem(''); setStatus(''); setQuery(initialQuery); setCatalogueView('all'); }, [selectedConsole?.id]);
    useEffect(()=>{onSearchQuery?.(selectedConsole&&catalogueView!=='installed'?query:'');},[query,catalogueView,selectedConsole?.id]);
    useEffect(()=>{if(mode!=='covers')return undefined;let key='gameflex:covers-scroll:'+section,timer=requestAnimationFrame(()=>{try{window.scrollTo({top:Number(sessionStorage.getItem(key)||0),behavior:'instant'});}catch{}});const save=()=>{try{sessionStorage.setItem(key,String(window.scrollY));}catch{}};window.addEventListener('scroll',save,{passive:true});return()=>{cancelAnimationFrame(timer);save();window.removeEventListener('scroll',save);};},[mode,section]);
    const searching=Boolean(foldSearch(query));
    const config = interfaces.find(x => x.id === mode) || interfaces[0];
    const key = g => `${g.consoleId}:${g.reference}`;
    const filtered = games.filter(g => matchesGameSearch(g,query) && (!system || g.consoleId === system) && (!status || g.status === status));
    const coverItems=filtered.filter(g=>coverFilter==='installed'?Boolean(g.reference):coverFilter==='discover'?!g.reference:true).filter(g=>{if(!coverLetter)return true;const first=g.title.normalize('NFKD').toUpperCase().match(/[A-Z0-9]/)?.[0];return coverLetter==='0'?/[0-9]/.test(first||''):first===coverLetter;});
    const favs = filtered.filter(g => favorites.includes(key(g)));
    const multiplayer = filtered.filter(g => Number(consoles.find(c => c.id === g.consoleId)?.maxPlayers) >= 2);
    const recent = history.map(id => filtered.find(g => key(g) === id)).filter(Boolean);
    const dashboardGame=recent[0],dashboardResume=Boolean(dashboardGame&&(dashboardGame.saveCompatible||dashboardGame.hasSave||dashboardGame.saveState||dashboardGame.savestate||dashboardGame.saves?.length));
    const browser = (items, label = 'Jeux') => <GameBrowser mode={retroModes.includes(config.id) ? 'covers' : config.id} items={items.map(g=>({...g,year:g.year||consoles.find(c=>c.id===g.consoleId)?.year,download:jobForGame(g,downloadJobs)||g.download}))} label={label} consoleName={selectedConsole?.name} onOpen={onOpen} Art={Art} onFavorite={onFavorite} favorites={favorites} collections={collections} onAddToCollection={onAddToCollection}/>;
    const systems = <SystemBrowser mode={config.id} consoles={consoles.filter(c => c.name.toLowerCase().includes(query.toLowerCase()))} games={games} Art={Art} onConsole={onConsole}/>;
    const activeSection = selectedConsole ? 'consoles' : section;
    const view = section === 'home' ? mode : section;
    let content;
    if (view === 'consoles') content = systems;
    else if (view === 'favorites') content = <><div className="em-section-heading"><h2>{favs.length ? 'Épinglés' : 'À épingler'}</h2><small>{favs.length} favori{favs.length !== 1 ? 's' : ''}</small></div>{browser(favs,'Favoris')}</>;
    else if (view === 'resume') content = <><div className="em-section-heading"><h2>{recent.length ? 'Reprendre' : 'Choisir un jeu'}</h2><small>{recent.length} récent{recent.length !== 1 ? 's' : ''}</small></div>{browser(recent,'Récents')}</>;
    else if (view === 'collections') content = <><div className="em-section-heading"><h2>Collections</h2><button onClick={onCreateCollection}>＋ Créer</button></div>{collections.map(c => <section className="em-user-collection" key={c.id}><h3>{c.name}</h3>{browser(filtered.filter(g => c.games.includes(key(g))),c.name)}</section>)}{!collections.length && <p>Créez une collection et ajoutez-y vos jeux depuis leur fiche.</p>}</>;
    else if (view === 'multiplayer') content = <><section className="em-arcade-marquee"><Icon name="consoles" size={46}/><div><small>MULTIJOUEUR</small><h2>À vous de jouer.</h2></div><span>{multiplayer.length}<small>JEU{multiplayer.length === 1 ? '' : 'X'}</small></span></section>{browser(multiplayer,'Multijoueur')}<div className="em-session-line"><Icon name="multiplayer" size={17}/>{live.length ? `${live.length} session(s) en cours` : 'Aucune session en cours'}</div></>;
    else if (view === 'dashboard') content = <div className="em-dashboard-workspace"><div className="em-dashboard-top">{dashboardGame?<button className="em-dashboard-resume" onClick={()=>onOpen(dashboardGame)}><Art item={dashboardGame}/><span><small>{dashboardResume?'SAUVEGARDE DISPONIBLE':'DERNIÈRE SESSION'}</small><b>{dashboardResume?'Reprendre':'Relancer'}</b><strong>{dashboardGame.title}</strong><span><Icon name="play" size={17}/> Ouvrir la fiche</span></span></button>:<section className="em-dashboard-empty"><Icon name="library" size={34}/><strong>{games.length?'Aucun historique de jeu':'Votre bibliothèque est vide'}</strong><small>{games.length?'Lancez un jeu pour le retrouver ici.':'Ajoutez un jeu pour commencer.'}</small></section>}<div className="em-dashboard-priority"><button onClick={()=>onNavigate('favorites')}><Icon name="favorites" size={28}/><strong>Favoris</strong><span>{favorites.length} favori{favorites.length===1?'':'s'}</span></button><button onClick={()=>onNavigate('consoles')}><Icon name="consoles" size={28}/><strong>Systèmes</strong><span>{consoles.length} système{consoles.length===1?'':'s'}</span></button></div></div><div className="em-library-stats"><span><b>{games.length}</b> référence{games.length === 1 ? '' : 's'}</span><span><b>{consoles.length}</b> systèmes</span><span><b>{favorites.length}</b> favoris</span></div><section className="em-dashboard-recent"><h2>Derniers jeux ajoutés</h2>{browser(filtered.slice(0,8),'Derniers jeux ajoutés')}</section></div>;
    else if (view === 'management' || view === 'archives') content = <div className="em-explorer-workspace"><aside className="em-explorer-tree"><small>SYSTÈMES</small>{consoles.map(c => <button key={c.id} onClick={() => onConsole(c)}><Icon name="archives" size={16}/>{c.name}</button>)}</aside><section><div className="em-archive-filters"><select aria-label="Filtrer par console" value={system} onChange={e => setSystem(e.target.value)}><option value="">Toutes les consoles</option>{consoles.map(c => <option key={c.id} value={c.id}>{c.name}</option>)}</select><select aria-label="Filtrer par état" value={status} onChange={e => setStatus(e.target.value)}><option value="">Tous les états</option>{[...new Set(games.map(g => g.status).filter(Boolean))].map(x => <option key={x}>{x}</option>)}</select></div><h2>Archives</h2><div className="em-explorer-path">Bibliothèque / {filtered.reduce((n,g) => n + (g.files?.length || 0), 0)} fichiers · {filtered.length} référence{filtered.length === 1 ? '' : 's'}</div>{browser(filtered,'Archives')}</section></div>;
    else content = config.id==='minimal'?<div className="em-minimal-workspace"><aside className="em-minimal-filters" aria-label="Filtres de la bibliothèque"><strong>Bibliothèque</strong><label>Filtrer par console<select aria-label="Filtrer par console" value={system} onChange={e=>setSystem(e.target.value)}><option value="">Toutes les consoles · {games.length}</option>{consoles.map(c=><option key={c.id} value={c.id}>{c.name}</option>)}</select></label><label>Filtrer par état<select aria-label="Filtrer par état" value={status} onChange={e=>setStatus(e.target.value)}><option value="">Tous les états</option>{[...new Set(games.map(g=>g.status).filter(Boolean))].map(x=><option key={x}>{x}</option>)}</select></label><small>{filtered.length} jeu{filtered.length===1?'':'x'} affiché{filtered.length===1?'':'s'}</small></aside><section className="em-minimal-results"><div className="em-minimal-columns"><span>Jeu</span><span>Console</span><span>Sortie</span><span>Statut</span></div>{browser(filtered,'Bibliothèque')}</section></div>:config.id==='covers'?<div className="em-covers-workspace"><div className="gf-cover-tabs" aria-label="Filtrer les jeux">{[['all','Tous'],['installed','Installés'],['discover','À découvrir']].map(([id,label])=><button key={id} aria-pressed={coverFilter===id} onClick={()=>{setCoverFilter(id);try{sessionStorage.setItem('gameflex:covers-filter',id);}catch{}}}>{label}<small>{id==='all'?filtered.length:id==='installed'?filtered.filter(g=>g.reference).length:filtered.filter(g=>!g.reference).length}</small></button>)}</div><div className="gx-alphabet" aria-label="Index alphabétique"><button aria-pressed={!coverLetter} onClick={()=>{setCoverLetter('');try{sessionStorage.setItem('gameflex:covers-letter','');}catch{}}}>A–Z</button>{'0ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('').map(letter=><button key={letter} aria-pressed={coverLetter===letter} onClick={()=>{const next=coverLetter===letter?'':letter;setCoverLetter(next);try{sessionStorage.setItem('gameflex:covers-letter',next);}catch{}}}>{letter==='0'?'0–9':letter}</button>)}</div>{browser(coverItems,'Bibliothèque')}</div>:browser(filtered,'Bibliothèque');
    if (section === 'home' && !selectedConsole && (mode === 'multiplayer' || mode === 'dashboard')) content = <>{content}<section className="em-home-systems"><h2>Vos systèmes</h2><SystemBrowser mode={config.id} consoles={consoles.slice(0,6)} games={games} Art={Art} onConsole={onConsole}/></section></>;
    if (section === 'settings') content = settingsContent;
    if (selectedConsole) {
        const allGames=mergeGames([{items:games.filter(g=>g.consoleId===selectedConsole.id),provider:'Bibliothèque'},{items:downloadable,provider:'GameFlex · Jeux libres'},{items:[...(planet.items||[])].sort((a,b)=>Number(a.provider==='FreeROMs')-Number(b.provider==='FreeROMs')),provider:'Planetemu'}],selectedConsole.id);
        const installed=allGames.filter(g=>g.reference);
        const catalogue=allGames.filter(g=>{
            if(!matchesGameSearch(g,query))return false;
            if(catalogueView==='installed')return Boolean(g.reference||g.pendingDownload);
            if(catalogueView==='discover'&&g.reference)return false;
            if(searching||!planetLetter)return true;
            const first=g.title.normalize('NFKD').toUpperCase().match(/[A-Z0-9]/)?.[0];return planetLetter==='0'?/[0-9]/.test(first||''):first===planetLetter;
        }).sort((a,b)=>a.title.localeCompare(b.title,'fr',{sensitivity:'base',numeric:true}));
        content = <div className={`em-console-library em-console-library-${config.id}`}><ConsoleIdentity console={selectedConsole} count={loading ? null : installed.length} Art={Art} mode={config.id}/>
        <section className={`gx-library-panel gf-unified-catalogue catalogue-view-${catalogueView}`}><div className="gx-section-title"><h2>Jeux <small>{loading || planet.loading ? '…' : catalogue.length}</small></h2></div>
        <div className="gf-catalogue-tabs" aria-label="Filtrer les jeux">{[['all','Tous'],['installed',`Installés · ${loading ? '…' : installed.length}`],['discover','À découvrir']].map(([id,label])=><button key={id} aria-pressed={catalogueView===id} onClick={()=>setCatalogueView(id)}>{label}</button>)}</div>
        {searching&&catalogueView!=='installed'&&<p className="gf-global-search" role="status">Recherche…{planet.loading||planet.query!==foldSearch(query)?` · ${planet.searchCompleted||0}/${planet.searchTotal||27}`:''}</p>}
        {!searching&&catalogueView!=='installed'&&<div className="gx-alphabet" aria-label="Lettre du catalogue">{'0ABCDEFGHIJKLMNOPQRSTUVWXYZ'.split('').map(letter=><button key={letter} aria-pressed={planetLetter===letter} onClick={()=>onPlanetLetter?.(planetLetter===letter?'':letter)}>{letter==='0'?'0–9':letter}</button>)}</div>}
        {catalogue.length?<>{browser(catalogue.slice(0,visibleCount),'Jeux')}{visibleCount<catalogue.length&&<button className="em-load-more" onClick={()=>setPage({scope:catalogueScope,count:visibleCount+24})}>Afficher la suite · {Math.min(visibleCount,catalogue.length)} / {catalogue.length}</button>}</>:!(planet.loading||(searching&&planet.query!==foldSearch(query)))&&<p>{catalogueView==='installed'?'Aucun jeu installé pour cette console.':searching?'Aucun jeu ne correspond à la recherche.':planetLetter?'Aucun titre pour cette lettre.':'Aucun jeu disponible.'}</p>}
        {!searching&&planet.loading&&catalogueView!=='installed'&&<p role="status">Chargement du catalogue…</p>}{planet.error&&catalogueView!=='installed'&&<p role="status">{planet.error}</p>}
        <small className="gx-muted">Jaquettes : Libretro Thumbnails · droits des éditeurs.</small></section></div>;

    }
    const heading = selectedConsole ? selectedConsole.name : section === 'archives' ? 'Gestion / Archives' : destinations.find(x => x[0] === section)?.[1] || (section === 'settings' ? 'Paramètres' : config.name);
    if(section==='home'&&!selectedConsole){
        if(mode==='hub-canape')return <HubCanape games={games} history={history} favorites={favorites} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onSettings={onSettings}/>;
        if(mode==='ligne-cinema')return <LigneCinema games={games} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onSettings={onSettings}/>;
        if(mode==='mosaique-joy')return <MosaiqueJoy games={games} consoles={consoles} favorites={favorites} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onConsole={onConsole}/>;
        if(mode==='roue-retro')return <RoueRetro games={games} consoles={consoles} Art={Art} onConsole={onConsole} onNavigate={onNavigate} onSettings={onSettings}/>;
        if(mode==='terminal-poche')return <TerminalPoche games={games} favorites={favorites} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onSettings={onSettings} onFavorite={onFavorite}/>;
        if(mode==='cartouches')return <Cartouches games={games} favorites={favorites} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onSettings={onSettings} onFavorite={onFavorite}/>;
        if(mode==='carte-epoques')return <CarteEpoques games={games} consoles={consoles} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onSettings={onSettings} onConsole={onConsole}/>;
        if(mode==='double-ecran')return <DoubleEcran games={games} favorites={favorites} history={history} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onSettings={onSettings}/>;
        if(mode==='club-local')return <ClubLocal games={games} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onSettings={onSettings}/>;
        if(mode==='quai-systemes')return <QuaiSystemes games={games} consoles={consoles} Art={Art} onOpen={onOpen}/>;
        if(mode==='magazine-retro')return <MagazineRetro games={games} collections={collections} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onSettings={onSettings}/>;
        if(mode==='bureau-collectionneur')return <BureauCollectionneur games={games} consoles={consoles} Art={Art} onOpen={onOpen} onSettings={onSettings}/>;
        if(mode==='grand-ecran-accessible')return <GrandEcranAccessible games={games} favorites={favorites} Art={Art} onOpen={onOpen} onNavigate={onNavigate} onSettings={onSettings} onFavorite={onFavorite}/>;
    }
    if(retroModes.includes(mode) && ['home','consoles','settings'].includes(section))return <RetroLibrary mode={mode} onNavigate={onNavigate} games={games} consoles={consoles} selectedConsole={selectedConsole} Art={Art} onConsole={onConsole} onOpen={onOpen} onHome={onHome} onSettings={onSettings} onAppearance={onAppearance} section={section} settingsContent={settingsContent} loading={loading} error={error} onRetry={onRetry} onImportRom={onImportRom} onRefreshLibrary={onRefreshLibrary} refreshing={refreshing}/>;
    return <div className={`gx-app gx-${retroModes.includes(config.id) ? 'common-' + config.id : config.id} em-app em-layout-${config.id}`} data-interface={config.id} ref={root}>
        <EmulatorNavigation mode={config.id} active={activeSection} onNavigate={onNavigate} onSettings={onSettings} onAppearance={onAppearance} user={user}/>
        <main className="em-main">{selectedConsole && <button className="gx-breadcrumb em-back" onClick={onHome}><Icon name="back" size={16}/> Consoles</button>}<header className={`em-page-heading ${selectedConsole ? 'em-console-heading' : ''}`}><div><h1>{heading}</h1></div>{section !== 'settings' && <label className="gx-search em-search"><Icon name="search" size={17}/><span className="gx-sr">Rechercher dans les jeux</span><input type="search" placeholder="Rechercher…" value={query} onChange={e => setQuery(e.target.value)}/></label>}</header>
        {(section==='library'||selectedConsole)&&(onRefreshLibrary||onImportRom)&&<div className="gf-library-refresh">{onRefreshMetadata&&<button disabled={metadataProgress?.startsWith('Métadonnées ·')||metadataProgress==='Rafraîchissement des métadonnées…'} onClick={onRefreshMetadata}><Icon name="refresh" size={17}/>Métadonnées</button>}{onImportRom&&<button onClick={onImportRom}><Icon name="add" size={17}/>Ajouter une ROM</button>}{onRefreshLibrary&&<button onClick={onRefreshLibrary} disabled={refreshing} aria-busy={refreshing}><Icon name="refresh" size={17}/>{refreshing?'Rechargement…':'Recharger'}</button>}<span role="status">{metadataProgress||(refreshing?'Recherche des jeux…':refreshMessage)}</span></div>}
        {section==='library'&&<DownloadList jobs={downloadJobs}/>}
        {live.length > 0 && section !== 'multiplayer' && <section className="gx-live-bar"><h3>En cours</h3>{live.map(s => <button key={s.id} onClick={() => onWatch?.(s)}>{s.title} · Ouvrir</button>)}</section>}{loading && <p role="status">Chargement…</p>}{error && <div role="alert" className="gx-empty">{error} <button onClick={onRetry}>Réessayer</button></div>}{loading ? null : content}
        <footer className="em-controller-hint"><span><kbd>←</kbd><kbd>→</kbd> Naviguer</span><span><kbd>↵</kbd> Sélectionner</span><span>{config.name}</span></footer></main>
    </div>;
}
