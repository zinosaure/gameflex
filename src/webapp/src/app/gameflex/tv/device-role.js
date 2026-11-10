import {appPath} from '../libs/appBase.js';
export function controllerURL(code,base=location.origin+appPath('/controller')){
 const url=new URL(base);if(/^\d{8}$/.test(code||''))url.hash='code='+code;return url.href;
}
export function initialPairCode(){
 const url=new URL(location.href),code=new URLSearchParams(url.hash.slice(1)).get('code')||url.searchParams.get('code')||'';
 return /^\d{8}$/.test(code)?code:'';
}
export function clearPairCodeURL(){
 const url=new URL(location.href);url.searchParams.delete('code');const hash=new URLSearchParams(url.hash.slice(1));if(hash.has('code')){hash.delete('code');url.hash=hash.toString();}history.replaceState(history.state,'',url.pathname+url.search+url.hash);
}
export function becomeController(){
 location.assign(appPath('/controller'));
}
export function becomeScreen(){location.assign(appPath('/?device=screen'));}
