const assert=require('assert');
const {controllerURL,initialPairCode,clearPairCodeURL}=require('../../src/webapp/src/app/gameflex/tv/device-role.js');
const valid=controllerURL('12345678','https://fixture.invalid/gameflex/controller');assert.equal(valid,'https://fixture.invalid/gameflex/controller#code=12345678');assert.equal(new URL(valid).search,'');
assert(!controllerURL('invalid','https://fixture.invalid/controller').includes('#'));
const oldLocation=global.location,oldHistory=global.history;
global.location={href:valid};assert.equal(initialPairCode(),'12345678');global.location={href:'https://fixture.invalid/controller?code=87654321'};assert.equal(initialPairCode(),'87654321');
global.location={href:'https://fixture.invalid/controller#code=invalid'};assert.equal(initialPairCode(),'');let cleaned;global.history={state:{key:'fixture'},replaceState:(state,title,url)=>cleaned=url};global.location={href:valid};clearPairCodeURL();assert.equal(cleaned,'/gameflex/controller');global.location=oldLocation;global.history=oldHistory;
console.log('Role QR links: code in fragment, valid prefill, invalid code refusal and URL cleanup passed.');
