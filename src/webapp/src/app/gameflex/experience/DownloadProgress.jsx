import React from 'react';
import {activeDownload,downloadPercent,downloadLabel,byteLabel} from './download-state.js';
import './download-progress.css';
export function DownloadOverlay({job,pending}){
 if(!job||(!activeDownload(job)&&job.status!=='failed'&&!pending))return null;
 const percent=downloadPercent(job),failed=job.status==='failed';
 return <div className={`gf-download-mask ${failed?'is-failed':''}`} role="progressbar" aria-label={downloadLabel(job)} aria-valuemin={0} aria-valuemax={100} aria-valuenow={percent??undefined} title={job.message}>
 <strong>{percent===null?'…':`${percent} %`}</strong><span>{downloadLabel(job)}</span>{job.bytesReceived>0&&<small>{byteLabel(job.bytesReceived)}{job.totalBytes?` / ${byteLabel(job.totalBytes)}`:' reçus'}</small>}<i className={percent===null?'is-indeterminate':''}><b style={{width:percent===null?'35%':`${percent}%`}}/></i></div>;
}
export function DownloadList({jobs=[]}){
 if(!jobs.length)return null;
 const ordered=[...jobs].sort((a,b)=>Number(activeDownload(b))-Number(activeDownload(a))||b.createdAt-a.createdAt).slice(0,8);
 return <section className="gf-download-list" aria-label="Suivi des téléchargements"><h2>Téléchargements</h2>{ordered.map(job=><article key={job.id} className={job.status==='failed'?'is-failed':''}><div><strong>{job.title}</strong><span>{downloadLabel(job)}</span><small>{job.status==='failed'?job.message:job.phase==='checking'?'En attente du fichier.':job.bytesReceived?`${byteLabel(job.bytesReceived)}${job.totalBytes?` / ${byteLabel(job.totalBytes)}`:' reçus'}`:job.message}</small></div><b>{downloadPercent(job)===null?'…':`${downloadPercent(job)} %`}</b></article>)}</section>;
}
