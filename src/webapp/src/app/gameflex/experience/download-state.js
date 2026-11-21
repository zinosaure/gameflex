import {gameIdentity} from './merge-games.js';
export const activeDownload=job=>Boolean(job&&(['submitting','queued','running'].includes(job.status)||job.phase==='indexing'));
export function downloadPercent(job){if(job.status==='succeeded')return 100;if(job.status==='failed')return null;if(job.progress===null)return null;return Number.isFinite(job.progress)?Math.max(0,Math.min(99,Math.floor(job.progress))):job.status==='queued'||job.status==='submitting'?0:null;}
export function downloadLabel(job){
 if(job.trackingLost)return 'Suivi interrompu · reconnexion…';
 if(job.syncWarning)return 'Téléchargé · analyse à relancer';
 if(job.status==='failed')return 'Échec du téléchargement';
 return ({submitting:'Demande en cours…',queued:'En attente',checking:'Vérification du lien…',downloading:'Téléchargement…',uploading:'Import en cours…',validating:'Vérification du fichier…',extracting:'Extraction…',installing:'Rangement…',indexing:'Actualisation…',completed:'Terminé'})[job.phase||job.status]||'En cours…';
}
export const byteLabel=bytes=>bytes>=1048576?`${(bytes/1048576).toFixed(1)} Mio`:bytes>=1024?`${Math.round(bytes/1024)} Kio`:`${bytes||0} o`;
export function freshTerminalJobs(jobs,seen){const fresh=jobs.filter(job=>['succeeded','failed'].includes(job.status)&&!seen.has(job.id));fresh.forEach(job=>seen.add(job.id));return fresh;}
export function jobForGame(game,jobs=[]){
 const ids=new Set([game.downloadId,...(game.variants||[]).map(v=>v.downloadId)].filter(Boolean));
 const candidates=jobs.filter(job=>ids.has(job.gameId)||(job.consoleId===game.consoleId&&(gameIdentity({consoleId:job.consoleId,title:job.title})===gameIdentity(game)||(job.filename&&gameIdentity({consoleId:job.consoleId,title:job.filename})===gameIdentity(game)))));
 return candidates.sort((a,b)=>Number(activeDownload(b))-Number(activeDownload(a))||(b.updatedAt||b.createdAt)-(a.updatedAt||a.createdAt))[0];
}
export function libraryWithDownloads(games,jobs=[]){
 const items=games.map(game=>{const download=jobForGame(game,jobs);return {...game,title:download?.filename&&download.status==='succeeded'?download.title:game.title,download};});
 const newest=new Map();
 for(const job of jobs){const key=gameIdentity({consoleId:job.consoleId,title:job.title});if(!newest.has(key))newest.set(key,job);}
 for(const job of newest.values()){
  const freshSuccess=job.status==='succeeded'&&Date.now()/1000-job.updatedAt<30;
  if(!activeDownload(job)&&!freshSuccess)continue;
  if(items.some(game=>game.consoleId===job.consoleId&&(gameIdentity(game)===gameIdentity({consoleId:job.consoleId,title:job.title})||gameIdentity(game)===gameIdentity({consoleId:job.consoleId,title:job.filename||''}))))continue;
  items.push({title:job.title,consoleId:job.consoleId,consoleName:job.consoleId.toUpperCase(),downloadId:job.gameId,image:job.image,attribution:job.attribution||'Image de la bibliothèque',pendingDownload:true,download:job});
 }
 return items;
}
