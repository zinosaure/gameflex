const fs=require('node:fs'),vm=require('node:vm'),assert=require('node:assert/strict');
const handlers={};let cacheTouches=0;
const sandbox={self:{registration:{scope:'http://gameflex.local/'},addEventListener:(name,fn)=>handlers[name]=fn,location:{origin:'http://gameflex.local'}},URL,console,caches:{match:()=>{cacheTouches++;return Promise.resolve(null)}}};
vm.runInNewContext(fs.readFileSync('src/webapp/src/public/service-worker.js','utf8'),sandbox);
for(const path of ['/api/v1/consoles','/experience-api/v1/media','/experience-api/v1/preferences','/experience-api/v1/planetemu/gba?letter=P']){
 let intercepted=false;handlers.fetch({request:{url:'http://gameflex.local'+path,method:'GET',mode:'cors'},respondWith:()=>{intercepted=true}});
 assert.equal(intercepted,false,path+' must bypass service-worker caches');
}
assert.equal(cacheTouches,0);console.log('API et métadonnées : aucun accès au cache du service worker.');
