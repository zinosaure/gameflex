"""User-selected mounted library and bounded local/provider installation jobs.
No arbitrary URL, advertisement execution, system command or file overwrite.
"""
import http.client
import hashlib
import json
import os
from pathlib import Path
import stat
import threading
import time
import uuid
from concurrent.futures import ThreadPoolExecutor

ROOT=Path(__file__).resolve().parents[2]
APPROVED={
    'palette-libre': {'id':'palette-libre','title':'Palette libre','consoleId':'nes','filename':'palette-libre.nes','license':'CC0-1.0','author':'GameFlex','sourceFile':'output/nes-demo/palette-libre.nes','sha256':'18ea7f7eb5dce8ab18b15d84af42c53e6a021df51dd9e3916fa60b8521896abd','size':24592}
}
REMOTE_SLOT=threading.BoundedSemaphore(1)
class LibraryError(ValueError):pass

def atomic_json(path,obj):
    path.parent.mkdir(parents=True,exist_ok=True,mode=0o700)
    temp=path.with_name(path.name+'.'+uuid.uuid4().hex+'.tmp')
    fd=os.open(temp,os.O_WRONLY|os.O_CREAT|os.O_EXCL,0o600)
    try:
        with os.fdopen(fd,'w') as f:json.dump(obj,f,ensure_ascii=False)
        os.replace(temp,path)
    finally:temp.unlink(missing_ok=True)

class Library:
    def __init__(self,data,host_folder,mount,default_roms=False):
        self.data=Path(data);self.host_folder=host_folder;self.mount=Path(mount);self.default_roms=default_roms
        self.lock=threading.RLock();self.pool=ThreadPoolExecutor(max_workers=2,thread_name_prefix='game-import');self.pending={}
        # Interrupted jobs are reported as failures instead of staying pending forever.
        for file in (self.data/'downloads').glob('*/*.json'):
            try:
                job=json.loads(file.read_text())
                if job['status'] in ('queued','running'):
                    job.update(status='failed',phase='failed',message='Transfert interrompu. Aucun succès confirmé ; réessayez.',updatedAt=time.time());atomic_json(file,job)
            except (OSError,ValueError,KeyError):pass
    @property
    def destination(self):return self.host_folder+'/roms' if self.default_roms else self.host_folder
    def status(self):
        try:configured=json.loads((self.data/'library.json').read_text()).get('folder') in (self.host_folder,self.destination)
        except (OSError,ValueError):configured=False
        target=self.mount/'roms' if self.default_roms else self.mount
        available=bool(self.host_folder) and target.is_dir() and not target.is_symlink() and os.access(target,os.W_OK|os.X_OK)
        configured=(configured or self.default_roms) and available
        return {'configured':configured,'folder':self.destination if configured else '', 'availableFolders':[self.destination] if available else [],'organization':'<console>/<jeu>' if self.default_roms else 'roms/<console>/<jeu>','defaultFolder':self.destination}
    def browse(self,folder=''):
        available=self.status()['availableFolders']
        if not folder:
            return {'folder':'','parent':None,'breadcrumbs':[],'selectable':False,'items':[{'name':Path(path).name,'folder':path,'library':True} for path in available],'truncated':False}
        if not isinstance(folder,str) or len(folder)>2048 or not available:
            raise LibraryError('Dossier indisponible.')
        if folder==self.destination:parts=[]
        elif folder.startswith(self.destination+'/'):parts=folder[len(self.destination)+1:].split('/')
        else:raise LibraryError('Ce dossier n’est pas accessible à GameFlex.')
        if len(parts)>16 or any(not part or part.startswith('.') or len(part)>255 or any(ord(c)<32 or c=='\\' for c in part) for part in parts):
            raise LibraryError('Chemin de dossier refusé.')
        fd=os.open(self.mount/'roms' if self.default_roms else self.mount,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW)
        try:
            for part in parts:
                child=os.open(part,os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW,dir_fd=fd);os.close(fd);fd=child
            items=[];truncated=False
            with os.scandir(fd) as entries:
                for index,entry in enumerate(entries):
                    if index>=2000:truncated=True;break
                    if not entry.name.startswith('.') and entry.is_dir(follow_symlinks=False):
                        items.append({'name':entry.name,'folder':folder+'/'+entry.name,'library':False})
            items.sort(key=lambda item:item['name'].casefold())
            if len(items)>200:truncated=True;items=items[:200]
            crumbs=[{'name':Path(self.destination).name,'folder':self.destination}]
            for index,part in enumerate(parts):crumbs.append({'name':part,'folder':self.destination+'/'+'/'.join(parts[:index+1])})
            return {'folder':folder,'parent':self.destination+'/'+'/'.join(parts[:-1]) if len(parts)>1 else self.destination if parts else '', 'breadcrumbs':crumbs,'items':items,'selectable':not parts,'truncated':truncated,'selectionHint':'' if not parts else 'Choisissez le dossier roms pour conserver le classement par console.'}
        finally:os.close(fd)
    def configure(self,folder):
        if not isinstance(folder,str) or not self.host_folder or folder!=self.destination or not self.status()['availableFolders']:
            raise LibraryError('Dossier non accessible à GameFlex. Faites monter ce dossier sur le serveur avant de le sélectionner.')
        with self.lock:atomic_json(self.data/'library.json',{'folder':folder,'layoutVersion':2 if self.default_roms else 1})
        return self.status()
    def catalogue(self,console):
        return [{k:v for k,v in game.items() if k not in ('sourceFile','sha256','size','filename')}|{'downloadId':game['id']} for game in APPROVED.values() if game['consoleId']==console]
    def job_folder(self,user):return self.data/'downloads'/hashlib.sha256(user.encode()).hexdigest()
    def jobs(self,user):
        result=[]
        with self.lock:
            for file in self.job_folder(user).glob('*.json'):
                try:result.append(json.loads(file.read_text()))
                except (OSError,ValueError):pass
        return sorted(result,key=lambda x:(x.get('status') in ('queued','running'),x['createdAt']),reverse=True)[:32]
    def start(self,user,game_id,presentation=None):
        if isinstance(game_id,str) and game_id.startswith(('freeroms:','romspure:')):
            from providers import decode
            console,slug=decode(game_id);game={'id':game_id,'title':slug.replace('_',' '),'consoleId':console,'remote':True}
        elif isinstance(game_id,str) and game_id in APPROVED:game=APPROVED[game_id]
        else:raise LibraryError('Cette source ne fournit pas de lien de téléchargement intégré.')
        if not self.status()['configured']:raise LibraryError('Choisissez d’abord le dossier de jeux.')
        with self.lock:
            key=(user,game_id)
            if key in self.pending:return self.pending[key]
            if len(self.pending)>=8:raise LibraryError('La file est pleine. Réessayez dans un instant.')
            presentation=presentation if isinstance(presentation,dict) else {}
            if game.get('remote'):
                from providers import decode
                candidates=presentation.get('candidateIds',[])
                if not isinstance(candidates,list) or len(candidates)>4:raise LibraryError('Liste de sources invalide.')
                game={**game,'candidateIds':[game_id]}
                for candidate in candidates:
                    if not isinstance(candidate,str):raise LibraryError('Source invalide.')
                    candidate_console,_=decode(candidate)
                    if candidate_console!=game['consoleId']:raise LibraryError('Console de la source incompatible.')
                    if candidate not in game['candidateIds']:game['candidateIds'].append(candidate)
            title=presentation.get('displayTitle') or presentation.get('title')
            title=title.strip() if isinstance(title,str) and 0<len(title)<=200 else game['title']
            image=presentation.get('image','')
            if not isinstance(image,str) or not __import__('re').fullmatch(r'/experience-api/v1/(?:media/[a-f0-9]{64}|covers/[a-z0-9_-]+/[A-Za-z0-9_-]{1,800}|(?:freeroms-art/[a-z0-9_-]+/[a-z0-9_-]{1,200}|romspure-art/[a-z0-9_-]+/[a-z0-9~-]{1,301}))',image):image=''
            job={'id':uuid.uuid4().hex,'gameId':game_id,'title':title,'consoleId':game['consoleId'],'image':image,'attribution':str(presentation.get('attribution',''))[:240],'status':'queued','phase':'queued','progress':0,'bytesReceived':0,'totalBytes':None,'createdAt':time.time(),'updatedAt':time.time(),'message':'Demande acceptée · en attente.'}
            folder=self.job_folder(user);atomic_json(folder/(job['id']+'.json'),job)
            # Bound the persistent history for this account; pending jobs are retained.
            for old in self.jobs(user)[24:]:
                if old['status'] not in ('queued','running'):(folder/(old['id']+'.json')).unlink(missing_ok=True)
            self.pending[key]=dict(job);self.pool.submit(self.run,user,game,job);return dict(job)
    def run(self,user,game,job):
        path=self.job_folder(user)/(job['id']+'.json')
        last_write=[0.0]
        def update(**values):
            with self.lock:
                same_phase=values.get('phase',job.get('phase'))==job.get('phase')
                job.update(**values,updatedAt=time.time())
                self.pending[(user,game['id'])]=dict(job)
                now=time.monotonic()
                if same_phase and set(values)<= {'phase','bytesReceived','totalBytes','progress','message'} and now-last_write[0]<.25:return
                atomic_json(path,job);last_write[0]=now
        try:
            if game.get('remote'):
                from providers import payload
                with REMOTE_SLOT:
                    update(status='running',phase='checking',progress=0,message='Vérification du lien · aucun fichier reçu pour le moment.')
                    candidates=game.get('candidateIds',[game['id']])
                    for index,candidate in enumerate(candidates):
                        if index:update(phase='checking',progress=0,bytesReceived=0,totalBytes=None,message='Essai d’un autre accès compatible…')
                        try:
                            downloaded,body=payload(self.data,candidate,progress=update)
                            break
                        except (OSError,ValueError,http.client.HTTPException):
                            if index==len(candidates)-1:raise
                    if not self.status()['configured']:raise LibraryError('Le dossier de jeux est indisponible.')
                    update(phase='installing',progress=99,message='Rangement du jeu dans la bibliothèque…',filename=downloaded['filename'])
                    self.write_game(downloaded,body)
            else:
                update(status='running',phase='validating',progress=0,message='Vérification du contenu local…')
                source=(ROOT/game['sourceFile']).resolve()
                if not source.is_relative_to(ROOT) or not source.is_file():raise LibraryError('Contenu libre indisponible sur ce serveur.')
                body=source.read_bytes()
                if len(body)!=game['size'] or hashlib.sha256(body).hexdigest()!=game['sha256']:raise LibraryError('Le contrôle d’intégrité du jeu a échoué.')
                if not self.status()['configured']:raise LibraryError('Le dossier de jeux est indisponible.')
                update(phase='installing',progress=99,bytesReceived=len(body),totalBytes=len(body),filename=game['filename'],message='Rangement du jeu dans la bibliothèque…')
                self.write_game(game,body)
            update(status='succeeded',phase='completed',progress=100,message='Téléchargement terminé · jeu rangé dans votre bibliothèque.')
        except ValueError as e:update(status='failed',phase='failed',message=str(e) if isinstance(e,LibraryError) or e.__class__.__name__=='ProviderError' else 'Fichier reçu invalide.')
        except OSError:update(status='failed',phase='failed',message='Transfert ou écriture impossible : vérifiez la source, le dossier et l’espace disponible.')
        except Exception:update(status='failed',phase='failed',message='Préparation ou transfert interrompu. Aucun succès confirmé ; réessayez.')
        finally:
            with self.lock:self.pending.pop((user,game['id']),None)
    def write_game(self,game,body):
        from io import BytesIO
        def prepare(prefix,digest,size):
            if digest!=game['sha256'] or size!=game['size']:raise LibraryError('Le fichier reçu ne correspond pas aux métadonnées.')
            return game
        return self.receive_game(game['consoleId'],game['filename'],BytesIO(body),len(body),prepare)

    def receive_game(self,console_id,filename,source,size,prepare,validate_prefix=None):
        """Stream one authenticated import to disk, then publish without overwrite."""
        flags=os.O_RDONLY|os.O_DIRECTORY|os.O_NOFOLLOW
        root=os.open(self.mount,flags);roms=console=None;temp=None
        try:
            try:os.mkdir('roms',mode=0o750,dir_fd=root)
            except FileExistsError:pass
            roms=os.open('roms',flags,dir_fd=root)
            try:os.mkdir(console_id,mode=0o750,dir_fd=roms)
            except FileExistsError:pass
            console=os.open(console_id,flags,dir_fd=roms)
            space=os.fstatvfs(console)
            if space.f_bavail*space.f_frsize<size+10*1024*1024:raise LibraryError('Espace insuffisant pour installer ce jeu.')
            temp='.'+uuid.uuid4().hex+'.part';fd=os.open(temp,os.O_WRONLY|os.O_CREAT|os.O_EXCL|os.O_NOFOLLOW,0o640,dir_fd=console)
            digest=hashlib.sha256();prefix=b'';remaining=size;validated=False
            with os.fdopen(fd,'wb') as output:
                while remaining:
                    chunk=source.read(min(1024*1024,remaining))
                    if not chunk:raise LibraryError('Transfert incomplet. Le fichier partiel a été supprimé.')
                    if len(chunk)>remaining:raise LibraryError('Taille reçue invalide.')
                    if len(prefix)<512:prefix+=chunk[:512-len(prefix)]
                    digest.update(chunk);output.write(chunk);remaining-=len(chunk)
                    if not validated and (len(prefix)>=512 or remaining==0):
                        if validate_prefix:validate_prefix(prefix)
                        validated=True
                output.flush();os.fsync(output.fileno())
            game=prepare(prefix,digest.hexdigest(),size)
            try:
                existing=os.open(filename,os.O_RDONLY|os.O_NOFOLLOW|os.O_NONBLOCK,dir_fd=console)
                with os.fdopen(existing,'rb') as current:
                    if not stat.S_ISREG(os.fstat(current.fileno()).st_mode):raise LibraryError('Le fichier existant ne peut pas être remplacé.')
                    existing_digest=hashlib.sha256()
                    while True:
                        chunk=current.read(1024*1024)
                        if not chunk:break
                        existing_digest.update(chunk)
                if existing_digest.hexdigest()!=game['sha256']:raise LibraryError('Un fichier différent porte déjà ce nom. Il a été conservé.')
                return game
            except FileNotFoundError:pass
            os.link(temp,filename,src_dir_fd=console,dst_dir_fd=console,follow_symlinks=False)
            os.fsync(console)
            return game
        finally:
            if temp and console is not None:
                try:os.unlink(temp,dir_fd=console)
                except FileNotFoundError:pass
            for fd in (console,roms,root):
                if fd is not None:os.close(fd)
