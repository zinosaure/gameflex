const fs=require('fs'),vm=require('vm'),assert=require('assert/strict'),path=require('path');
const root=path.resolve(__dirname,'../../src/webapp/src')+'/';
const base=fs.readFileSync(root+'app/gameflex/libs/appBase.js','utf8').replaceAll('export ','');
const server=fs.readFileSync(root+'app/gameflex/libs/serverBase.js','utf8').replace(/^import .*;$/gm,'').replaceAll('export ','');
for(const pathname of ['/', '/retrogaming','/gameflex/','/gameflex/retrogaming','/gameflexish/']){
 const window={location:{pathname,origin:'https://example.test',host:'example.test',protocol:'https:'}};
 const c=vm.createContext({window,capabilities:()=>({})});vm.runInContext(base+server,c);
 const prefix=/^\/gameflex(?:\/|$)/.test(pathname)?'/gameflex':'';
 assert.equal(c.apiUrl('/api/v1/auth/me'),(prefix?'https://example.test'+prefix:'')+'/api/v1/auth/me');
 assert.equal(c.homeWsUrl('/api/v1/facade/ws'),'wss://example.test'+prefix+'/api/v1/facade/ws');
 assert.equal(c.wsUrl('/experience-api/socket'),'wss://example.test'+prefix+'/experience-api/socket');
 assert.equal(c.absoluteApiUrl('/api/test'),'https://example.test'+prefix+'/api/test');
 c.setStreamBase('https://other.test','secret-not-printed');assert.equal(c.apiUrl('/api/test'),'https://other.test/api/test');assert.equal(c.homeWsUrl('/api/test'),'wss://example.test'+prefix+'/api/test');c.clearStreamBase();
 window.GAMEFLEX_SERVER='http://192.168.1.63:2000';assert.equal(c.apiUrl('/api/test'),'http://192.168.1.63:2000/api/test');
}
for(const scope of ['https://example.test/','https://example.test/gameflex/']){
 const handlers={},removed=[],kept=['nasflix-cache','unrelated-cache','gameflex-subpath-v40','gameflex-root-v40'];
 const self={registration:{scope},location:{origin:'https://example.test'},clients:{claim:()=>Promise.resolve()},addEventListener:(name,fn)=>handlers[name]=fn};
 const c=vm.createContext({self,URL,console:{log(){}},caches:{keys:async()=>kept,delete:async k=>removed.push(k)}});vm.runInContext(fs.readFileSync(root+'public/service-worker.js','utf8'),c);
 handlers.activate({waitUntil:async promise=>{await promise;assert(!removed.includes('nasflix-cache'));assert(!removed.includes('unrelated-cache'));assert.deepEqual(removed,[scope.endsWith('/gameflex/')?'gameflex-subpath-v40':'gameflex-root-v40']);}});
 let intercepted=false;handlers.fetch({request:{url:'https://example.test/api/v1/auth/me',mode:'cors'},respondWith:()=>intercepted=true});assert(!intercepted);
}
console.log('Root + /gameflex: API, WebSocket, absolute URLs, federation override and isolated service-worker caches passed.');
