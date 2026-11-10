import {apiUrl} from '../libs/serverBase.js';
import {authHeaders} from '../profile/auth/AuthContext.jsx';
export async function uploadPersonalFile(token,file,consoleId,onProgress,signal){
 let id=null;
 const request=async(path,body,raw=false)=>{
  const cancel=new AbortController(),abort=()=>cancel.abort();signal?.addEventListener('abort',abort,{once:true});if(signal?.aborted)abort();
  const timeout=setTimeout(abort,120000);
  try{
  const response=await fetch(apiUrl('/experience-api/v1/imports/sessions'+path),{method:'POST',signal:cancel.signal,headers:{...authHeaders(token),'Content-Type':raw?'application/octet-stream':'application/json'},body:raw?body:JSON.stringify(body)});
  let data;try{data=await response.json();}catch{throw new Error('Réponse du serveur invalide.');}
  if(!response.ok)throw new Error(data.error||'Import interrompu. Réessayez.');return data;
  }finally{clearTimeout(timeout);signal?.removeEventListener('abort',abort);}
 };
 try{
  const session=await request('',{consoleId,filename:file.name,size:file.size});id=session.id;
  for(let offset=0;offset<file.size;){
   const end=Math.min(file.size,offset+session.chunkSize);
   const data=await request('/'+id+'/chunk?offset='+offset,file.slice(offset,end),true);
   if(data.bytesReceived!==end)throw new Error('Bloc non confirmé par le Pi. Réessayez.');
   offset=end;onProgress(Math.floor(offset/file.size*98),offset);
  }
  onProgress(99,file.size);const result=await request('/'+id+'/complete',{});id=null;onProgress(100,file.size);return result.item;
 }finally{
  if(id)fetch(apiUrl('/experience-api/v1/imports/sessions/'+id),{method:'DELETE',headers:authHeaders(token)}).catch(()=>{});
 }
}
