import {useEffect,useRef} from 'react';
import {subscribePhonePad,claimPhoneGameConsumer} from '../libs/phonePad.js';
import {isGameInputSuspended,onGameInputSuspension} from '../libs/gameInput.js';
// Diff complete snapshots; releases always reach the existing game-input channel.
export function usePhoneGameInput(enabled,onButton,onStick){
 const callbacks=useRef({onButton,onStick});callbacks.current={onButton,onStick};
 useEffect(()=>{
  if(!enabled)return;
  const unclaim=claimPhoneGameConsumer();let held=new Set(),axes=[0,0,0,0],last=0;
  const apply=value=>{const allowed=!isGameInputSuspended()&&!document.hidden;const desired=new Set(allowed?value.buttons.filter(index=>index<16):[]),next=allowed?value.axes:[0,0,0,0];
   for(const index of held)if(!desired.has(index))callbacks.current.onButton(index,false);
   for(const index of desired)if(!held.has(index))callbacks.current.onButton(index,true);
   for(let stick=0;stick<2;stick++)if(next[stick*2]!==axes[stick*2]||next[stick*2+1]!==axes[stick*2+1])callbacks.current.onStick?.(stick,next[stick*2],next[stick*2+1]);
   held=desired;axes=[...next];last=Date.now();};
  const release=()=>apply({buttons:[],axes:[0,0,0,0]});
  const unsubscribe=subscribePhonePad(apply),unsuspend=onGameInputSuspension(value=>{if(value)release();});
  const watchdog=setInterval(()=>{if(Date.now()-last>1000&&(held.size||axes.some(Boolean)))release();},100);
  window.addEventListener('blur',release);document.addEventListener('visibilitychange',release);
  return()=>{release();unsubscribe();unsuspend();unclaim();clearInterval(watchdog);window.removeEventListener('blur',release);document.removeEventListener('visibilitychange',release);};
 },[enabled]);
}
