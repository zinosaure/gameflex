import {retroModes} from './RetroLibrary.jsx';
import EmulatorAssistant from './EmulatorAssistant.jsx';
import LibrarySources from './LibrarySources.jsx';
import { appPath } from "../libs/appBase.js";
import {attachSavedMetadata,prioritizeMetadata,manualMetadata,metadataFields} from './metadata-priority.js';
import { loadConsoleCatalogue } from './catalogue-search.js';
import RomImportDialog from './RomImportDialog.jsx';
import InteractiveVerification from './InteractiveVerification.jsx';
import {metadataRequest} from './EmugenDetails.jsx';
import CatalogGameDialog from './CatalogGameDialog.jsx';
import {SourceSettingsProvider,useSourceSettings} from './SourceSettings.jsx';
import GameSources from './GameSources.jsx';
import LaunchDialog from './LaunchDialog.jsx';
import { useLibraryDownloads, libraryRequest } from './LibraryManager.jsx';
import React, { createContext, useContext, useEffect, useRef, useState } from 'react';
import { useLocation, useNavigate } from 'react-router-dom';
import { useAuth, authHeaders } from '../profile/auth/AuthContext.jsx';
import { useConsoles } from '../contexts/ConsolesContext.jsx';
import { apiUrl } from '../libs/serverBase.js';
import { useSessionPool } from '../libs/SessionPool.js';
import { buildStreamingUrl } from '../libs/sessionLinks.js';
import { listRoms, listCores, createSession } from '../api/retrogaming.js';
import ExperienceView, { InterfacePicker } from './ExperienceView.jsx';
import { interfaces, preferenceKey, readChoice } from './interfaces.js';

const Context = createContext(null);
const gameKey = g => `${g.consoleId}:${g.reference}`;
export function ExperienceProvider({ children }) {
    const { user, token } = useAuth();
    const key = preferenceKey(user?.username, apiUrl('/'));
    const [mode, setMode] = useState(() => readChoice(key));
    const [choosing, setChoosing] = useState(false);
    const [data, setData] = useState(() => {
        try { return { favorites: [], history: [], collections: [], ...JSON.parse(localStorage.getItem(`${key}:library`) || '{}') }; }
        catch { return { favorites: [], history: [], collections: [] }; }
    });
    const [notice, setNotice] = useState('');
    const [syncReady, setSyncReady] = useState(false);
    useEffect(() => {
        let active = true;
        const controller = new AbortController();
        const timer = setTimeout(() => controller.abort(), 2500);
        fetch(apiUrl('/experience-api/v1/preferences'), { headers: authHeaders(token), signal: controller.signal })
            .then(r => r.ok ? r.json() : null).then(p => {
                if (active && p && interfaces.some(x => x.id === p.interface)) {
                    setMode(p.interface);
                    setData({ favorites: p.favorites || [], history: p.history || [], collections: p.collections || [] });
                }
            }).catch(() => {}).finally(() => { clearTimeout(timer); if (active) setSyncReady(true); });
        return () => { active = false; clearTimeout(timer); controller.abort(); };
    }, [token]);
    useEffect(() => {
        if (!syncReady || !mode) return undefined;
        try {
            if (mode) localStorage.setItem(key, mode);
            localStorage.setItem(`${key}:library`, JSON.stringify(data));
        } catch { setNotice('Le stockage du navigateur est indisponible.'); }
        const controller = new AbortController();
        const timer = setTimeout(() => {
            fetch(apiUrl('/experience-api/v1/preferences'), { method: 'PUT', headers: { ...authHeaders(token), 'Content-Type': 'application/json' }, body: JSON.stringify({ interface: mode, ...data }), signal: controller.signal })
                .then(r => { if (!r.ok) throw new Error('sync'); setNotice(''); })
                .catch(e => { if (e.name !== 'AbortError') setNotice('Synchronisation indisponible · sauvegarde locale.'); });
        }, 400);
        return () => { clearTimeout(timer); controller.abort(); };
    }, [mode, data, key, token, syncReady]);
    function choose(id) { if (interfaces.some(x => x.id === id)) { setMode(id); setChoosing(false); } }
    function favorite(g) { const id = gameKey(g); setData(d => ({ ...d, favorites: d.favorites.includes(id) ? d.favorites.filter(x => x !== id) : [...d.favorites, id] })); }
    const [libraryRevision,setLibraryRevision]=useState(0);
    const downloads=useLibraryDownloads({token,username:user?.username,enabled:syncReady&&Boolean(mode)&&!choosing,onInstalled:async()=>{
        try{
            const response=await fetch(apiUrl('/api/v1/retrogaming/listing/rescan?section=roms'),{method:'POST',headers:authHeaders(token)});
            if(!response.ok)throw new Error('rescan');
        }catch{return false;}
        finally{setLibraryRevision(x=>x+1);}
        return true;
    }});
    const [refreshing,setRefreshing]=useState(false);
    const [refreshMessage,setRefreshMessage]=useState('');
    const refreshLock=useRef(false);
    async function refreshLibrary(){
        if(refreshLock.current)return;
        refreshLock.current=true;setRefreshing(true);setRefreshMessage('');
        const controller=new AbortController();
        const timer=setTimeout(()=>controller.abort(),30000);
        try{
            const response=await fetch(apiUrl('/api/v1/retrogaming/listing/rescan?section=roms'),{method:'POST',headers:authHeaders(token),signal:controller.signal});
            if(!response.ok)throw new Error('rescan');
            setLibraryRevision(x=>x+1);
            setRefreshMessage('Bibliothèque rechargée.');
        }catch{setRefreshMessage('Impossible de recharger la bibliothèque. Réessayez.');}
        finally{clearTimeout(timer);refreshLock.current=false;setRefreshing(false);}
    }
    const value = { downloads, libraryRevision, refreshLibrary, refreshing, refreshMessage, mode, choose, choosing, setChoosing, data, setData, favorite, notice, syncReady };
    return <SourceSettingsProvider><Context.Provider value={value}>{children}{downloads.ui}</Context.Provider></SourceSettingsProvider>;
}
export function InterfaceSettings() {
    const ctx = useContext(Context);
    if (!ctx) return null;
    return <section className="gx-settings"><h2>Votre interface</h2><p>{interfaces.find(x => x.id === ctx.mode)?.name || 'Aucune interface choisie'}</p><button onClick={() => ctx.setChoosing(true)}>Changer d’interface</button>{ctx.notice && <p role="status">{ctx.notice}</p>}<p><a href={appPath("/settings?advanced=1")}>Compte et bibliothèque ↗</a></p><section className="gf-free-library"><h3>Dossier de jeux</h3><p>{ctx.downloads.library?.folder || "Aucun dossier choisi."}</p><button onClick={ctx.downloads.openSetup}>Choisir le dossier</button></section><LibrarySources onImported={async item=>{await ctx.refreshLibrary();ctx.downloads.notify('success',item.title,'Jeu importé.');}}/><EmulatorAssistant/><GameSources/><ArtworkCredits /></section>;
}

function ArtworkCredits() {
    const [items, setItems] = useState([]);
    useEffect(() => {
        let active = true;
        fetch(apiUrl('/experience-api/v1/media'), { cache: 'no-store' }).then(r => r.ok ? r.json() : null).then(p => { if (active && Array.isArray(p?.items)) setItems(p.items.filter(x => x.approved)); }).catch(() => {});
        return () => { active = false; };
    }, []);
    const link = (url, label) => typeof url === 'string' && url.startsWith('https://') ? <a href={url} target="_blank" rel="noreferrer">{label}</a> : <span>{label}</span>;
    return <details><summary>Crédits des images</summary>{items.length ? items.map(x => <p key={x.id}>{link(x.source, x.title)} · {x.author} · {link(x.licenseUrl, x.license)} · {x.provider}<br />{x.modifications || "Miniature de la source."}</p>) : <p>Aucune image externe approuvée. Illustrations GameFlex.</p>}</details>;
}

export function ExperienceGate({ children, enabled }) {
    const ctx = useContext(Context), location = useLocation();
    if (!enabled || location.pathname === '/streaming') return children;
    if (!ctx.syncReady) return <div className="gx-picker" role="status">Chargement de vos préférences…</div>;
    if (!ctx.mode || ctx.choosing) return <InterfacePicker selected={ctx.mode} onChoose={ctx.choose} onCancel={ctx.mode ? () => ctx.setChoosing(false) : null} />;
    if (location.pathname === '/settings' && new URLSearchParams(location.search).get('advanced') === '1') return children;
    const libraryRoute = /^\/retrogaming\/([a-z0-9_-]+)$/.exec(location.pathname);
    return location.pathname === '/' || location.pathname === '/settings' || libraryRoute ? <LibraryExperience consoleId={libraryRoute?.[1] || ''} /> : children;
}
function LibraryExperience({ consoleId = '' }) {
    const ctx = useContext(Context), { user, token } = useAuth(), consoles = useConsoles(), navigate = useNavigate();
    const location = useLocation();
    const sources=useSourceSettings(),sourceKey=sources.enabledIds.join(',');
    const section = location.pathname === '/settings' ? 'settings' : new URLSearchParams(location.search).get('section') || 'home';
    const [planet, setPlanet] = useState({}), [planetLetter, setPlanetLetter] = useState(''), [catalogueQuery,setCatalogueQuery]=useState('');
    const [catalogGame,setCatalogGame]=useState(null);
    const verificationQuery=new URLSearchParams(location.search),verificationId=verificationQuery.get('emugenVerification'),verificationGame=verificationQuery.get('emugenGame');
    const [verificationResult,setVerificationResult]=useState(null);
    useEffect(()=>{
        if(!sources.enabledIds.includes('emugen')||!verificationGame||!consoleId||!token)return undefined;
        const abort=new AbortController();
        metadataRequest(token,`emugen/games/${encodeURIComponent(verificationGame)}?console=${encodeURIComponent(consoleId)}`,{signal:abort.signal}).then(value=>{if(!abort.signal.aborted)setCatalogGame({...value.item,emugen:value.item,emugenSlug:verificationGame,consoleId});}).catch(()=>{});
        return()=>abort.abort();
    },[verificationGame,consoleId,token]);
    const closeVerification=()=>{const query=new URLSearchParams(location.search);query.delete('emugenVerification');navigate(location.pathname+'?'+query.toString(),{replace:true});};

    const [launchGame, setLaunchGame] = useState(null), [launchError, setLaunchError] = useState(''), [launching, setLaunching] = useState(false);
    const live = useSessionPool();
    useEffect(() => {
        if(!consoleId||!sources.ready||retroModes.includes(ctx.mode))return undefined;
        const abort=new AbortController();
        const timer=setTimeout(()=>{
            loadConsoleCatalogue({consoleId,letter:planetLetter,query:catalogueQuery,signal:abort.signal,enabledSourceIds:sourceKey.split(','),
                request:async(source,letter,signal,query,page)=>{
                    const r=await fetch(apiUrl(`/experience-api/v1/${source}/${encodeURIComponent(consoleId)}?letter=${letter}${source==='emugen'?'&page='+page:''}${['romspure','emugen'].includes(source)&&query?'&q='+encodeURIComponent(query):''}`),{headers:authHeaders(token),signal,cache:'no-store',priority:'low'});
                    if(!r.ok)throw new Error('catalogue');return r.json();
                },onUpdate:setPlanet
            }).catch(()=>{if(!abort.signal.aborted)setPlanet(p=>({...p,loading:false,error:'Catalogue momentanément indisponible.'}));});
        },catalogueQuery.trim()?300:0);
        return()=>{clearTimeout(timer);abort.abort();};
    },[consoleId,planetLetter,catalogueQuery,token,sourceKey,sources.ready,ctx.mode]);
    const [media, setMedia] = useState([]);
    const [downloadable,setDownloadable]=useState([]);
    useEffect(()=>{
        const abort=new AbortController();setDownloadable([]);
        if(consoleId&&sourceKey.split(',').includes('gameflex-free'))libraryRequest(token,`download-catalogue?console=${encodeURIComponent(consoleId)}`,{signal:abort.signal}).then(p=>{if(!abort.signal.aborted)setDownloadable(p.items||[]);}).catch(()=>{});
        return()=>abort.abort();
    },[consoleId,token,sourceKey]);
    useEffect(()=>{setPlanetLetter('');},[consoleId]);
    const [importOpen,setImportOpen]=useState(false);
    const [metadataProgress,setMetadataProgress]=useState('');
    const metadataBusy=useRef(false);
    const metadataAbort=useRef(null);
    useEffect(()=>()=>metadataAbort.current?.abort(),[]);
    async function refreshMetadata(){
        if(metadataBusy.current)return;
        metadataBusy.current=true;setMetadataProgress('Rafraîchissement des métadonnées…');
        const abort=new AbortController();metadataAbort.current=abort;
        try{
            const items=games.map(g=>({...Object.fromEntries(metadataFields.filter(key=>typeof g[key]==='string'||Array.isArray(g[key])).map(key=>[key,g[key]])),consoleId:g.consoleId,reference:g.reference,title:g.title,...(g.region?{region:g.region}:{}),...(g.version?{version:g.version}:{}),manualMetadata:manualMetadata(g)}));
            if(!items.length){setMetadataProgress('Aucun jeu installé à actualiser.');return;}
            let job=await libraryRequest(token,'metadata/refresh',{method:'POST',body:{items},signal:abort.signal});
            while(job.status==='running'){
                setMetadataProgress(`Métadonnées · ${job.completed}/${job.total}`);
                await new Promise((resolve,reject)=>{const timer=setTimeout(()=>{abort.signal.removeEventListener('abort',cancel);resolve();},1500);const cancel=()=>{clearTimeout(timer);reject(new Error('cancelled'));};abort.signal.addEventListener('abort',cancel,{once:true});});
                job=await libraryRequest(token,`metadata/jobs/${job.id}`,{signal:abort.signal});
            }
            retry(x=>x+1);setMetadataProgress(job.failed?`${job.matched} jeu(x) actualisé(s), ${job.failed} erreur(s). Les données précédentes sont conservées.`:`Métadonnées actualisées${job.choices?` · ${job.choices} correspondance(s) à choisir`:'.'}`);
        }catch(e){if(!abort.signal.aborted)setMetadataProgress(e.message||'Actualisation indisponible. Réessayez.');}
        finally{metadataBusy.current=false;}
    }
    const [games, setGames] = useState([]), [loading, setLoading] = useState(true), [error, setError] = useState(''), [epoch, retry] = useState(0);
    useEffect(() => {
        let active = true, timer;
        const abort = new AbortController();
        async function load(attempt = 0) {
            try {
                const r = await fetch(apiUrl('/experience-api/v1/media'), { cache: 'no-store', signal: abort.signal });
                if (!r.ok) throw new Error('media unavailable');
                const p = await r.json();
                if (!Array.isArray(p?.items)) throw new Error('invalid media');
                if (active) setMedia(p.items.filter(x => x.approved && x.target));
            } catch (e) {
                if (active && e.name !== 'AbortError' && attempt < 3) timer = setTimeout(() => load(attempt + 1), 750 * (attempt + 1));
            }
        }
        load();
        return () => { active = false; clearTimeout(timer); abort.abort(); };
    }, []);
    const withImage = (item, target) => {
        const m = media.find(x => x.target === target);
        return m ? prioritizeMetadata({...item,manualMetadata:{...manualMetadata(item),image:m.url,coverFront:m.url,attribution:`${m.author} · ${m.license}`}},item.emugen) : item;
    };
    const playable = consoles.filter(c => c.method === 'retrogaming' && c.enabled !== false);
    // Explicit filter prevents querying remote-play devices as ROM consoles.
    const ids = playable.map(c => c.id).join(',');
    useEffect(() => {
        let cancelled = false;
        setLoading(true); setError('');
        const selectedConsoles = consoles.filter(c => ids.split(',').includes(c.id) && (!consoleId || c.id === consoleId));
        // Three workers bound load on the Pi, rather than starting 33 scans at once.
        const queue = [...selectedConsoles], found = []; let failed = 0;
        const imported=libraryRequest(token,'imports').then(x=>x.items||[]).catch(()=>[]);
        const savedMetadata=libraryRequest(token,`metadata${consoleId?'?console='+encodeURIComponent(consoleId):''}`).then(x=>x.items||[]).catch(()=>[]);
        const worker = async () => {
            while (queue.length && !cancelled) {
                const c = queue.shift();
                try {
                    const result = await listRoms(c.id);
                    if (!result.ok) { failed++; continue; }
                    for (const g of result.data || []) found.push({ ...g, consoleId: c.id, consoleName: c.name });
                } catch { failed++; }
            }
        };
        Promise.all([worker(), worker(), worker(),imported,savedMetadata]).then((results) => {
            const metadata=results[3];
            const enriched=found.map(game=>{const own=metadata.find(item=>item.consoleId===game.consoleId&&(game.files||[]).some(file=>file.split('/').pop()===item.filename));return own?{...game,title:own.title,image:own.image,attribution:own.attribution,fallbackImage:own.fallbackImage,fallbackAttribution:own.fallbackAttribution}:game;});
            if (!cancelled) { setGames(enriched.map(game=>attachSavedMetadata(game,results[4]))); setLoading(false); if (failed) setError(`${failed} console(s) indisponible(s). La liste peut être incomplète.`); }
        });
        return () => { cancelled = true; };
    }, [ids, epoch, consoleId, ctx.libraryRevision]); // eslint-disable-line react-hooks/exhaustive-deps
    const openOwned = g => {
        ctx.setData(d => ({ ...d, history: [gameKey(g), ...d.history.filter(x => x !== gameKey(g))].slice(0, 40) }));
        setLaunchError(''); setLaunchGame(g);
    };
    const open=g=>{
        if(retroModes.includes(ctx.mode)&&g.reference){openOwned(g);return;}
        setCatalogGame(g);
    };
    async function launch() {
        setLaunching(true); setLaunchError('');
        try {
            const result = await listCores(launchGame.consoleId);
            const core = (result.data || []).find(c => c.installed);
            if (!result.ok || !core) throw new Error('Aucun émulateur installé pour cette console.');
            const reply = await createSession(launchGame.consoleId, launchGame.reference, launchGame.title, core.name, '', user?.username, null, { resolution: 'sd' });
            if (!reply.ok || !reply.data?.sessionId) throw new Error(reply.errorMessage || 'Le lancement a échoué.');
            navigate(buildStreamingUrl({ sessionId: reply.data.sessionId, play: true }));
        } catch (e) { setLaunchError(e.message); }
        finally { setLaunching(false); }
    }
    function createCollection() {
        const name = window.prompt('Nom de votre collection');
        if (name?.trim()) ctx.setData(d => ({ ...d, collections: [...d.collections, { id: `${Date.now()}-${Math.random().toString(36).slice(2, 8)}`, name: name.trim().slice(0, 80), games: [] }] }));
    }
    return <><ExperienceView onRefreshMetadata={undefined} metadataProgress={metadataProgress} onImportRom={ctx.downloads.library?.canConfigure?()=>setImportOpen(true):undefined} onRefreshLibrary={ctx.refreshLibrary} refreshing={ctx.refreshing} refreshMessage={ctx.refreshMessage} onAppearance={() => ctx.setChoosing(true)} section={section} onNavigate={id => navigate(`/?section=${id}`)} settingsContent={<InterfaceSettings />} planet={planet} downloadJobs={ctx.downloads.jobs} onSearchQuery={setCatalogueQuery} downloadable={downloadable.filter(item=>!games.some(g=>g.consoleId===item.consoleId&&g.title.toLowerCase().replace(/[^a-z0-9]/g,'')===item.title.toLowerCase().replace(/[^a-z0-9]/g,'')))} planetLetter={planetLetter} onPlanetLetter={setPlanetLetter} selectedConsole={playable.find(c => c.id === consoleId)} onHome={() => navigate('/?section=consoles')} live={live} onWatch={s => navigate(buildStreamingUrl({ sessionId: s.id }))} mode={ctx.mode} games={games.map(g => withImage(g, gameKey(g)))} consoles={playable.map(c => withImage(c, c.id))} {...ctx.data} onFavorite={ctx.favorite} onOpen={open} onConsole={c => navigate(`/retrogaming/${encodeURIComponent(c.id)}`)} onSettings={() => navigate('/settings')} loading={loading} error={error} onRetry={() => retry(x => x + 1)} user={user?.display_name || user?.username} onCreateCollection={createCollection} onAddToCollection={(id, g) => ctx.setData(d => ({ ...d, collections: d.collections.map(c => c.id === id ? { ...c, games: c.games.includes(gameKey(g)) ? c.games.filter(x => x !== gameKey(g)) : [...c.games, gameKey(g)] } : c) }))} />{importOpen&&<RomImportDialog token={token} consoles={playable} initialConsole={consoleId||''} onClose={()=>setImportOpen(false)} onImported={async item=>{await ctx.refreshLibrary();ctx.downloads.notify('success',item.title,'Jeu importé.');}}/>}{catalogGame&&<CatalogGameDialog canEditMetadata={user?.role_level===0} token={token} onMetadata={updated=>{setCatalogGame(updated);retry(x=>x+1);}} game={{...catalogGame,...(verificationResult?.slug===catalogGame.emugenSlug?{manualVerificationResult:verificationResult}:{})}} onClose={()=>setCatalogGame(null)} onInstall={game=>{setCatalogGame(null);ctx.downloads.start(game);}} onPlay={game=>{setCatalogGame(null);openOwned(game);}}/>}{sources.enabledIds.includes('emugen')&&verificationId&&<InteractiveVerification token={token} sessionId={verificationId} gameSlug={verificationGame} consoleId={consoleId} onClose={closeVerification} onResolved={setVerificationResult}/>}{launchGame && <LaunchDialog onEmulatorReady={()=>setLaunchError('')} game={launchGame} mode={ctx.mode} error={launchError} launching={launching} onPlay={launch} onClose={() => setLaunchGame(null)}/>}{ctx.notice && <p className="gx-sync-notice" role="status">{ctx.notice}</p>}</>;
}
