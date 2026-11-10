// noVNC-compatible, authenticated fallback when an upstream proxy blocks WebSockets.
export default class HttpVncChannel {
 constructor(request,viewer){
  this.request=request;this.viewer=viewer;this.readyState=1;this.protocol='binary';this.binaryType='arraybuffer';
  this.onopen=null;this.onmessage=null;this.onerror=null;this.onclose=null;
  this.abort=new AbortController();this.writes=Promise.resolve();
 }
 static async open(request){const value=await request({op:'open'});return new HttpVncChannel(request,value.viewer);}
 start(){this.read();}
 async read(){
  try{
   while(this.readyState===1){
    const result=await this.request({op:'read',viewer:this.viewer},this.abort.signal);
    if(this.readyState!==1)return;
    if(result.data){const bytes=Uint8Array.from(atob(result.data),c=>c.charCodeAt(0));this.onmessage?.({data:bytes.buffer});}
   }
  }catch{if(this.readyState===1){this.onerror?.(new Error('Connexion distante interrompue.'));this.close();}}
 }
 send(value){
  if(this.readyState!==1)return;
  const bytes=new Uint8Array(value.buffer||value,value.byteOffset||0,value.byteLength).slice();
  let data='';for(let i=0;i<bytes.length;i+=8192)data+=String.fromCharCode(...bytes.subarray(i,i+8192));
  const encoded=btoa(data);
  this.writes=this.writes.then(()=>this.readyState===1?this.request({op:'write',viewer:this.viewer,data:encoded}):null).catch(()=>{if(this.readyState===1){this.onerror?.(new Error('Contrôle distant interrompu.'));this.close();}});
 }
 close(){
  if(this.readyState===3)return;this.readyState=3;this.abort.abort();
  this.request({op:'close',viewer:this.viewer}).catch(()=>{});this.onclose?.({code:1000,wasClean:true});
 }
}
