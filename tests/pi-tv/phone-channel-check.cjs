const assert=require('assert');
const {CompatiblePadChannel}=require('../../src/webapp/src/app/gameflex/tv/phone-pad-api.js');
const channel=Object.create(CompatiblePadChannel.prototype);let closed;
Object.assign(channel,{mode:'http',connection:'fixture',state:1,stopped:false,lastResponse:Date.now()-7000,abort:new AbortController(),primary:{close(){}},onclose:event=>closed=event.code});
channel.send('{"type":"ping"}');
assert.equal(channel.readyState,3);assert.equal(closed,1006);assert(channel.abort.signal.aborted);
console.log('HTTP phone channel resumes from a lost fetch callback instead of remaining falsely connected.');
