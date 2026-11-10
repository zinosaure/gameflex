"""Personal imports in acknowledged 1 MiB blocks; no network ROM retrieval."""
import os,re,time,uuid,threading
from pathlib import Path
from library import LibraryError,atomic_json
from rom_import import validate,validate_content,install_stream
CHUNK=1024*1024
class UploadSessions:
 def __init__(self,slot):
  self.slot=slot;self.lock=threading.RLock();self.record=None
  threading.Thread(target=self.reaper,daemon=True).start()
 def reaper(self):
  while True:
   time.sleep(15)
   with self.lock:
    if self.record and time.monotonic()-self.record['touched']>900:self.end('failed','Import expiré. Réessayez.')
 def job(self,r,**changes):
  r['job'].update(**changes,updatedAt=time.time());atomic_json(r['library'].job_folder(r['owner'])/(r['id']+'.json'),r['job']);return dict(r['job'])
 def end(self,status,message):
  r=self.record
  if not r:return
  try:
   self.job(r,status=status,phase='completed' if status=='succeeded' else 'failed',message=message,**({'progress':100} if status=='succeeded' else {}))
  finally:
   r['path'].unlink(missing_ok=True);self.record=None;self.slot.release()
 def owned(self,user,id):
  r=self.record
  if not r or r['id']!=id:raise LibraryError('Import expiré ou terminé. Réessayez.')
  if r['owner']!=user:raise PermissionError('Cet import appartient à un autre compte.')
  r['touched']=time.monotonic();return r
 def start(self,library,user,data):
  if not isinstance(data,dict):raise LibraryError('Demande invalide.')
  name=validate(data.get('consoleId'),data.get('filename'));size=data.get('size')
  if type(size)!=int or size<=0:raise LibraryError('Taille du fichier invalide.')
  if not library.status()['configured']:raise LibraryError('Dossier de jeux indisponible.')
  with self.lock:
   if not self.slot.acquire(blocking=False):raise LibraryError('Un import est déjà en cours.')
   path=None
   try:
    folder=library.data/'upload-staging';folder.mkdir(mode=0o700,parents=True,exist_ok=True)
    if folder.is_symlink():raise LibraryError('Dossier temporaire refusé.')
    # Reboot cleanup: only our UUID-named temporary files, never library content.
    for old in folder.glob('*.part'):
     if re.fullmatch('[a-f0-9]{32}\\.part',old.name):old.unlink(missing_ok=True)
    for disk in (folder,library.mount):
     space=os.statvfs(disk)
     if space.f_bavail*space.f_frsize<size*2+10*1024*1024:raise LibraryError('Espace insuffisant pour cet import.')
    id=uuid.uuid4().hex;path=folder/(id+'.part');fd=os.open(path,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o600);os.close(fd)
    job={'id':id,'gameId':'personal:'+id,'title':Path(name).stem,'consoleId':data['consoleId'],'filename':name,'status':'running','phase':'uploading','progress':0,'bytesReceived':0,'totalBytes':size,'createdAt':time.time(),'updatedAt':time.time(),'message':'Envoi du fichier personnel…'}
    r={'id':id,'owner':user,'path':path,'size':size,'received':0,'library':library,'name':name,'console':data['consoleId'],'job':job,'touched':time.monotonic()};self.record=r;self.job(r)
    return {'id':id,'chunkSize':CHUNK,'bytesReceived':0}
   except Exception:
    if path:path.unlink(missing_ok=True)
    self.record=None;self.slot.release();raise
 def chunk(self,user,id,offset,source,size):
  with self.lock:
   r=self.owned(user,id)
   if offset!=r['received'] or not 0<size<=CHUNK or offset+size>r['size']:raise LibraryError('Bloc invalide. Relancez l’import.')
   body=bytearray()
   while len(body)<size:
    data=source.read(size-len(body))
    if not data:raise LibraryError('Envoi interrompu. Réessayez.')
    body.extend(data)
   if offset==0:validate_content(r['console'],r['name'],bytes(body[:512]))
   fd=os.open(r['path'],os.O_WRONLY|os.O_APPEND|os.O_NOFOLLOW)
   with os.fdopen(fd,'ab') as out:
    if os.fstat(out.fileno()).st_size!=offset:raise LibraryError('Fichier temporaire incomplet. Relancez l’import.')
    out.write(body);out.flush()
   r['received']+=size;r['touched']=time.monotonic();self.job(r,bytesReceived=r['received'],progress=min(98,int(r['received']*98/r['size'])))
   return {'bytesReceived':r['received'],'totalBytes':r['size']}
 def complete(self,user,id):
  with self.lock:
   r=self.owned(user,id)
   if r['received']!=r['size'] or r['path'].stat().st_size!=r['size']:raise LibraryError('Le fichier est incomplet.')
   self.job(r,phase='installing',progress=99,message='Vérification et rangement…')
   try:
    with r['path'].open('rb') as source:item=install_stream(r['library'],r['console'],r['name'],source,r['size'])
    self.end('succeeded','Jeu importé.');return {'item':item}
   except Exception:
    self.end('failed','Import impossible. Aucun succès confirmé.');raise
 def cancel(self,user,id):
  with self.lock:self.owned(user,id);self.end('failed','Import annulé.')
