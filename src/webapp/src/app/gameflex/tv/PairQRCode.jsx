import React,{useMemo} from 'react';
import qrcode from './vendor/qrcode-generator.js';
export default function PairQRCode({url}){
 const matrix=useMemo(()=>{const qr=qrcode(0,'M');qr.addData(url,'Byte');qr.make();const size=qr.getModuleCount();let path='';for(let y=0;y<size;y++)for(let x=0;x<size;x++)if(qr.isDark(y,x))path+=`M${x+4},${y+4}h1v1h-1z`;return {size:size+8,path};},[url]);
 return <svg className="gf-pair-qr" role="img" aria-label="QR code d’association" viewBox={`0 0 ${matrix.size} ${matrix.size}`} width="208" height="208" shapeRendering="crispEdges"><rect width={matrix.size} height={matrix.size} fill="#fff"/><path d={matrix.path} fill="#000"/></svg>;
}
