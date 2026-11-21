// Private API-only test: credentials arrive on stdin, never enter the browser.
const fs=require('fs');const WebSocket=require('/app/src/webapp/src/node_modules/ws');
const input=JSON.parse(fs.readFileSync(0,'utf8')),base='http://gameflex-web:8080';
(async()=>{
 const response=await fetch(base+'/api/v1/auth/login',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({username:input.username,password:input.password})});
 if(!response.ok)throw Error('authentication refused');
 const token=(await response.json()).item.access_token;
 const ws=new WebSocket(base.replace('http','ws')+'/api/v1/retrogaming/stream/ws?token='+encodeURIComponent(token));
 const guard=setTimeout(()=>{ws.terminate();process.exit(0)},240000);
 ws.on('open',()=>ws.send(JSON.stringify({type:'start',sessionId:input.sessionId,username:input.username})));
 ws.on('message',raw=>{const m=JSON.parse(raw.toString());console.log(JSON.stringify({type:m.type,step:m.step,code:m.errorCode}));});
 ws.on('error',()=>{console.log('Control channel unavailable');clearTimeout(guard);process.exit(1)});
})().catch(()=>{console.log('Stream check unavailable');process.exit(1)});
