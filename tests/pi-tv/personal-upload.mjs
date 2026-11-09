import assert from 'node:assert/strict';
import fs from 'node:fs';
const file=new URL('../../src/webapp/src/app/gameflex/experience/personal-upload.js',import.meta.url);
const source=fs.readFileSync(file,'utf8').replace(/^import .*;\n/gm,'');
const {uploadPersonalFile}=await import('data:text/javascript;base64,'+Buffer.from("const apiUrl=p=>p;const authHeaders=()=>({Authorization:'Bearer fixture'});"+source).toString('base64'));
const calls=[],progress=[];let received=0;
globalThis.fetch=async(url,options)=>{
 calls.push([url,options]);
 if(url.endsWith('/sessions'))return Response.json({id:'fixture',chunkSize:4});
 if(url.includes('/chunk?offset=')){assert.equal(Number(url.split('offset=')[1]),received);received+=options.body.size;return Response.json({bytesReceived:received});}
 if(url.endsWith('/complete'))return Response.json({item:{title:'Original demo'}});
 return Response.json({cancelled:true});
};
const demo=new File(['our original demo'],'demo.nes');
await uploadPersonalFile('fixture',demo,'nes',p=>progress.push(p));
assert.equal(received,demo.size);assert.equal(progress.at(-1),100);assert(progress.some(p=>p>0&&p<98));assert(!calls.some(c=>c[1].method==='DELETE'));
let cancelled=false;
globalThis.fetch=async(url,options)=>{
 if(options.method==='DELETE'){cancelled=true;return Response.json({});}
 if(url.endsWith('/sessions'))return Response.json({id:'fixture',chunkSize:4});
 return Response.json({error:'Interrupted'},{status:409});
};
await assert.rejects(()=>uploadPersonalFile('fixture',demo,'nes',()=>{}),/Interrupted/);assert(cancelled);
console.log('Personal upload: confirmed byte progress, completion and cancellation passed.');
