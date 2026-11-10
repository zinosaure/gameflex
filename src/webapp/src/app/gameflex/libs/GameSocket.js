import {getStoredToken} from '../profile/auth/AuthContext.jsx';
import {streamAuthToken} from './serverBase.js';
// Keep native WebSockets on working networks. Fall back only before the first open:
// commands already sent are NEVER replayed after a lost connection.
export class GameSocket extends EventTarget {
 constructor(address,token=''){
  super();this.url=String(address);this.readyState=0;this.binaryType='blob';this.protocol='';this.bufferedAmount=0;
  this.onopen=null;this.onmessage=null;this.onclose=null;this.onerror=null;this.id=null;this.writes=Promise.resolve();this.abort=new AbortController();this.usedFallback=false;this.everOpened=false;
  const u=new URL(this.url,window.location.href);this.token=token||u.searchParams.get('token')||streamAuthToken()||getStoredToken()||'';u.searchParams.delete('token');
  const prefix=u.pathname.startsWith('/gameflex/')?'/gameflex':'';
  this.endpoint=u.origin.replace(/^ws/,'http')+prefix+'/experience-api/v1/socket-bridge';this.path=u.pathname.slice(prefix.length)+u.search;
  // The public QNAP edge serves normal HTTPS correctly but strips native
  // WebSocket upgrades on this published prefix. Waiting four seconds for a
  // handshake known to fail made every video negotiation lose a third of its
  // 12-second picture deadline. Start the authenticated HTTPS bridge
  // immediately there; keep native WebSockets everywhere else.
  if(u.protocol==='wss:'&&u.hostname==='loutrenas.myqnapcloud.com'&&u.pathname.startsWith('/gameflex/')){
   this.native={close(){}};this.timer=setTimeout(()=>this.fallback(),0);
  }else{
   this.native=new WebSocket(address);this.timer=setTimeout(()=>this.fallback(),4000);
   this.native.onopen=()=>{if(this.usedFallback||this.readyState===3)return;clearTimeout(this.timer);this.everOpened=true;this.readyState=1;this.emit('open');};
   this.native.onmessage=e=>{if(!this.usedFallback)this.emit('message',e.data);};
   this.native.onerror=()=>{if(!this.everOpened)this.fallback();};
   this.native.onclose=e=>{if(this.usedFallback)return;if(!this.everOpened&&this.readyState!==3)this.fallback();else this.finish(e.code,e.reason,e.wasClean);};
  }
 }
 emit(type,data){const event=type==='message'?new MessageEvent(type,{data}):new Event(type);this['on'+type]?.(event);this.dispatchEvent(event);}
 async request(op,data={},signal=this.abort.signal){
  const controller=new AbortController(),stop=()=>controller.abort();if(signal?.aborted)stop();signal?.addEventListener('abort',stop,{once:true});const timer=setTimeout(stop,30000);
  try{
   const response=await fetch(this.endpoint+(op?'/'+this.id+'/'+op:''),{method:'POST',headers:{Authorization:'Bearer '+this.token,'Content-Type':'application/json'},body:JSON.stringify(data),signal:controller.signal,cache:'no-store'});
   const body=await response.json();if(!response.ok)throw new Error('Canal HTTPS indisponible.');return body;
  }finally{clearTimeout(timer);signal?.removeEventListener('abort',stop);}
 }
 async fallback(){
  if(this.usedFallback||this.everOpened||this.readyState===3)return;this.usedFallback=true;clearTimeout(this.timer);
  this.native.onopen=this.native.onmessage=this.native.onerror=this.native.onclose=null;this.native.close();
  try{
   if(!this.token)throw new Error('Connexion requise.');
   const result=await this.request('',{path:this.path});this.id=result.id;
   if(this.readyState===3){this.request('close',{},null).catch(()=>{});return;}
   this.readyState=1;this.emit('open');this.poll();
  }catch{if(this.readyState!==3){this.emit('error');this.finish(1006,'http_unavailable',false);}}
 }
 async poll(){
  try{
   while(this.readyState===1){
    const result=await this.request('poll');
    if(this.readyState!==1)return;
    for(const frame of result.frames||[]){
     if(typeof frame.text==='string')this.emit('message',frame.text);
     else if(frame.binary){const data=Uint8Array.from(atob(frame.binary),c=>c.charCodeAt(0));this.emit('message',this.binaryType==='arraybuffer'?data.buffer:new Blob([data]));}
    }
    if(result.closed){this.finish(1000,'',true);return;}
   }
  }catch{if(this.readyState!==3){this.emit('error');this.finish(1006,'http_interrupted',false);}}
 }
 send(value){
  if(this.readyState!==1)throw new Error('Canal non connecté.');
  if(!this.usedFallback){this.native.binaryType=this.binaryType;this.native.send(value);return;}
  let body;
  if(typeof value==='string')body={text:value};
  else{const bytes=new Uint8Array(value.buffer||value,value.byteOffset||0,value.byteLength);let raw='';for(let i=0;i<bytes.length;i+=8192)raw+=String.fromCharCode(...bytes.subarray(i,i+8192));body={binary:btoa(raw)};}
  this.writes=this.writes.then(()=>this.readyState!==3?this.request('send',body):null).catch(()=>{if(this.readyState!==3){this.emit('error');this.finish(1006,'send_failed',false);}});
 }
 finish(code=1000,reason='',clean=true){
  if(this.readyState===3)return;this.readyState=3;clearTimeout(this.timer);this.abort.abort();
  if(this.id)this.request('close',{},null).catch(()=>{});
  const event=new CloseEvent('close',{code,reason,wasClean:clean});this.onclose?.(event);this.dispatchEvent(event);
 }
 close(code=1000,reason=''){
  if(this.usedFallback&&this.readyState===1){this.readyState=2;this.writes.finally(()=>this.finish(code,reason,true));return;}
  this.native.onopen=this.native.onmessage=this.native.onerror=this.native.onclose=null;this.native.close(code,reason);this.finish(code,reason,true);
 }
}
export function gameSocket(url,token=''){return new GameSocket(url,token);}
