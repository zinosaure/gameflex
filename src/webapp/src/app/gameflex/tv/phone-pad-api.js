import {apiUrl} from '../libs/serverBase.js';
import {authHeaders} from '../profile/auth/AuthContext.jsx';
export async function phoneRequest(path,body,token,signal){
 const r=await fetch(apiUrl('/experience-api/v1/phone-pad'+path),{method:'POST',headers:{'Content-Type':'application/json',...(token?authHeaders(token):{})},body:JSON.stringify(body),cache:'no-store',signal});
 const value=await r.json();if(!r.ok){const error=new Error(value.error||'Association indisponible.');error.status=r.status;throw error;}return value;
}
// WebSocket first; bounded HTTPS long-polling for NAS proxies stripping upgrades.
export class CompatiblePadChannel {
 get readyState(){return this.mode==='ws'&&this.primary?this.primary.readyState:this.state;}
 set readyState(value){this.state=value;}
 constructor(id){
  this.id=id;this.readyState=0;this.mode='ws';this.stopped=false;this.pending=[];this.abort=new AbortController();this.lastResponse=Date.now();
  const url=new URL(apiUrl(`/experience-api/v1/phone-pad/${id}/ws`),location.href);url.protocol=url.protocol==='https:'?'wss:':'ws:';
  this.primary=new WebSocket(url);
  this.timer=setTimeout(()=>this.fallback(),2000);
  this.primary.onopen=()=>{if(this.mode!=='ws'||this.stopped)return;clearTimeout(this.timer);this.readyState=1;this.onopen?.({});};
  this.primary.onmessage=event=>{if(this.mode==='ws'&&!this.stopped)this.onmessage?.(event);};
  this.primary.onerror=()=>this.fallback();
  this.primary.onclose=event=>{if(this.stopped||this.mode!=='ws')return;if(event.code===1006)this.fallback();else this.fail(event.code);};
 }
 fallback(){
  if(this.stopped||this.mode==='http')return;this.mode='http';clearTimeout(this.timer);this.primary.close();
  if(this.auth)this.connectHTTP();else{this.readyState=1;this.onopen?.({});}
 }
 async post(action,extra={}){const timeout=new AbortController(),cancel=()=>timeout.abort();this.abort.signal.addEventListener('abort',cancel,{once:true});if(this.abort.signal.aborted)cancel();const timer=setTimeout(cancel,5000);try{const value=await phoneRequest(`/${this.id}/channel/${action}`,{role:this.auth.role,token:this.auth.token,connection:this.connection,...extra},this.auth.accessToken,timeout.signal);this.lastResponse=Date.now();return value;}finally{clearTimeout(timer);this.abort.signal.removeEventListener('abort',cancel);}}
 async connectHTTP(){
  if(this.connecting||this.stopped)return;this.connecting=true;this.readyState=0;
  try{const value=await this.post('connect');if(this.stopped)return;this.connection=value.connection;this.readyState=1;this.poll();this.pump();}
  catch(error){this.fail([400,401,403].includes(error.status)?1008:1006);}finally{this.connecting=false;}
 }
 async poll(){
  try{while(!this.stopped&&this.readyState===1){const value=await this.post('poll');for(const event of value.events||[]){if(this.stopped)return;this.onmessage?.({data:JSON.stringify(event)});}await new Promise(resolve=>setTimeout(resolve,20));}}
  catch(error){this.fail([400,401,403].includes(error.status)?1008:1006);}
 }
 send(text){
  if(this.stopped)return;
  // A suspended mobile tab can lose fetch/timer callbacks. The next heartbeat
  // must restart that channel even if its pending promise never settles.
  if(this.mode==='http'&&this.connection&&Date.now()-this.lastResponse>6500){this.fail(1006);return;}
  const data=JSON.parse(text);
  if(data.role){this.auth=data;if(this.mode==='http'){this.connectHTTP();return;}}
  if(this.mode==='ws'){if(this.primary.readyState===1)this.primary.send(text);return;}
  if(data.role)return;
  const last=this.pending[this.pending.length-1];
  if(data.type==='pad'&&last?.type==='pad'&&JSON.stringify(data.buttons)===JSON.stringify(last.buttons))this.pending[this.pending.length-1]=data;
  else if(!(data.type==='ping'&&last?.type==='ping'))this.pending.push(data);
  if(this.pending.length>32)this.pending.shift();this.pump();
 }
 async pump(){
  if(this.sending||this.readyState!==1||!this.connection||this.stopped)return;this.sending=true;
  try{while(this.pending.length&&!this.stopped){await this.post('send',{data:this.pending.shift()});await new Promise(resolve=>setTimeout(resolve,20));}}
  catch(error){this.fail([400,401,403].includes(error.status)?1008:1006);}finally{this.sending=false;}
 }
 fail(code){if(this.stopped)return;this.readyState=3;this.stopped=true;clearTimeout(this.timer);this.abort.abort();this.primary.close();this.onclose?.({code});}
 close(){
  if(this.stopped)return;
  if(this.mode==='http'&&this.auth&&this.connection)phoneRequest(`/${this.id}/channel/close`,{role:this.auth.role,token:this.auth.token,connection:this.connection},this.auth.accessToken).catch(()=>{});
  this.stopped=true;this.readyState=3;clearTimeout(this.timer);this.abort.abort();this.primary.close();
 }
}
export const phoneSocket=id=>new CompatiblePadChannel(id);
