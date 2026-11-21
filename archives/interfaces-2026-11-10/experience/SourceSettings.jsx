import React,{createContext,useContext,useEffect,useRef,useState} from 'react';
import {useAuth} from '../profile/auth/AuthContext.jsx';
import {libraryRequest} from './LibraryManager.jsx';
import registry from './game-sources.json';
const defaults={items:registry.items.map(item=>({...item,enabled:true})),enabledIds:registry.items.map(item=>item.id),canConfigure:false,ready:true};
const Context=createContext({...defaults,save:async()=>{}});
export const useSourceSettings=()=>useContext(Context);
const names={'EmuGen':'emugen','FreeROMs':'freeroms','ROMsPure':'romspure','Planetemu':'planetemu','GameFlex · Jeux libres':'gameflex-free'};
export const sourceId=name=>names[name]||name;
export function SourceSettingsProvider({children}){
 const {token}=useAuth(),[state,setState]=useState({...defaults,ready:false}),[error,setError]=useState('');
 const revision=useRef(0);
 const request=options=>libraryRequest(token,'game-sources',options);
 useEffect(()=>{
  let active=true;const controller=new AbortController();setState({...defaults,ready:false});
  const read=()=>{const at=revision.current;return request({signal:controller.signal}).then(value=>{if(active&&at===revision.current){setState({...value,ready:true});setError('');}}).catch(e=>{if(active){setError(e.message);setState(s=>({...s,ready:false}));}});};
  read();window.addEventListener('focus',read);const timer=setInterval(read,30000);
  return()=>{active=false;controller.abort();window.removeEventListener('focus',read);clearInterval(timer);};
 // The token owns this authenticated configuration subscription.
 // eslint-disable-next-line react-hooks/exhaustive-deps
 },[token]);
 async function save(enabledIds){revision.current++;const value=await request({method:'PUT',body:{enabledIds}});setState({...value,ready:true});setError('');}
 return <Context.Provider value={{...state,error,save}}>{children}</Context.Provider>;
}
