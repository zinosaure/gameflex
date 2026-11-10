// A phone can supply pad state, never a session token or a server command.
let state={buttons:[],axes:[0,0,0,0]},consumerCount=0;
const listeners=new Set(),consumerListeners=new Set();
export const getPhonePadState=()=>state;
export function publishPhonePad(value){state={buttons:[...value.buttons],axes:[...value.axes]};listeners.forEach(listener=>listener(state));}
export const clearPhonePad=()=>publishPhonePad({buttons:[],axes:[0,0,0,0]});
export function subscribePhonePad(listener){listeners.add(listener);return()=>listeners.delete(listener);}
export const hasPhoneGameConsumer=()=>consumerCount>0;
export function subscribePhoneConsumer(listener){consumerListeners.add(listener);return()=>consumerListeners.delete(listener);}
export function claimPhoneGameConsumer(){consumerCount++;consumerListeners.forEach(listener=>listener());let active=true;return()=>{if(!active)return;active=false;consumerCount--;clearPhonePad();consumerListeners.forEach(listener=>listener());};}
