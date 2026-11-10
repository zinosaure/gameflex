import {apiUrl} from '../libs/serverBase.js';
import {authHeaders} from '../profile/auth/AuthContext.jsx';
export async function phoneRequest(path,body,token){
 const r=await fetch(apiUrl('/experience-api/v1/phone-pad'+path),{method:'POST',headers:{'Content-Type':'application/json',...(token?authHeaders(token):{})},body:JSON.stringify(body),cache:'no-store'});
 const value=await r.json();if(!r.ok)throw new Error(value.error||'Association indisponible.');return value;
}
export function phoneSocket(id){const url=new URL(apiUrl(`/experience-api/v1/phone-pad/${id}/ws`),location.href);url.protocol=url.protocol==='https:'?'wss:':'ws:';return new WebSocket(url);}
