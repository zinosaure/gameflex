// Run through run-playstation-tests.cjs (existing Babel/React test harness).
const assert=require('assert'),fs=require('fs'),path=require('path'),crypto=require('crypto');
const root=path.resolve(__dirname,'../..');
const {consoleIcon,consoleIconAliases}=require('../../src/webapp/src/app/gameflex/experience/console-icons.js');
const manifest=require('../../assets/console-icons-pack/manifest.json');
assert.equal(Object.keys(manifest.icons).length,74);
for(const [slug,entry] of Object.entries(manifest.icons)){
 const icon=consoleIcon(slug);assert(icon,slug);assert.equal(icon.file,entry.file);
 const bytes=fs.readFileSync(path.join(root,'src/webapp/src/public/images/console-icons',icon.file));
 assert.equal(crypto.createHash('sha256').update(bytes).digest('hex'),entry.sha256);
 assert.equal(bytes.subarray(8,12).toString(),'WEBP');
 assert.deepEqual([icon.width,icon.height],entry.originalSize,slug);
}
for(const [id,slug] of Object.entries(consoleIconAliases))assert.equal(consoleIcon(id).slug,slug);
for(const id of ['c64','amstrad','cps1','cps2','rp-steam','unknown','__proto__','constructor'])assert.equal(consoleIcon(id),null,id);
assert.equal(consoleIcon('nds').slug,'nintendo-ds');assert.equal(consoleIcon('gc').slug,'nintendo-gamecube');
assert.equal(consoleIcon('ps1').slug,'playstation-1');assert.equal(consoleIcon('gbc').slug,'game-boy-color');
console.log('74 supplied icons: hashes, format, exact console aliases and uncovered-machine fallback passed.');
