"""Library-driven installation of allowlisted libre cores, within the project volume.
Never fetches games/BIOS/firmware, never installs a system package.
"""
import io,json,os,platform,struct,subprocess,sys,tempfile,threading,time,urllib.request,zipfile,hashlib
from pathlib import Path
from concurrent.futures import ThreadPoolExecutor
from library import atomic_json
from rom_import import FORMATS
# These software-rendered cores require no proprietary BIOS for these systems.
CORES={'nes':('quicknes','QuickNES','GPL-2.0'),'gbc':('gambatte','Gambatte','GPL-2.0'),'gba':('mgba','mGBA','MPL-2.0'),'snes':('snes9x','Snes9x','Snes9x non-commercial'),'megadrive':('genesis_plus_gx','Genesis Plus GX','non-commercial'),'sms':('genesis_plus_gx','Genesis Plus GX','non-commercial'),'atari2600':('stella2014','Stella 2014','GPL-2.0'),'atari7800':('prosystem','ProSystem','GPL-2.0'),'pce':('mednafen_pce_fast','Beetle PCE Fast','GPL-2.0'),'lynx':('mednafen_lynx','Beetle Lynx','GPL-2.0'),'wswan':('mednafen_wswan','Beetle WonderSwan','GPL-2.0'),'ngp':('mednafen_ngp','Beetle NeoPop','GPL-2.0')}
# Automatic installation is intentionally limited to audited, libre engines.
INSTALLABLE={'nes','gbc','gba','atari2600','atari7800','pce','lynx','wswan','ngp'}
ARCH={'aarch64':('aarch64',183),'arm64':('aarch64',183),'x86_64':('x86_64',62)}
LOCK=threading.RLock();POOL=ThreadPoolExecutor(max_workers=1);JOBS={}
class EmulatorError(ValueError):pass
class NoRedirect(urllib.request.HTTPRedirectHandler):
 def redirect_request(self,*args,**kwargs):raise EmulatorError('Redirection du fournisseur refusée.')
def elf_machine(path):
 with Path(path).open('rb') as f:h=f.read(20)
 if len(h)!=20 or h[:4]!=b'\x7fELF' or h[4]!=2 or h[5]!=1 or struct.unpack_from('<H',h,16)[0]!=3:raise EmulatorError('Le core ne correspond pas à une bibliothèque Linux 64 bits.')
 return struct.unpack_from('<H',h,18)[0]
def validate_core(path,machine):
 if elf_machine(path)!=machine:raise EmulatorError('Architecture du core incompatible avec ce serveur.')
 probe="import ctypes,sys; c=ctypes.CDLL(sys.argv[1]); assert c.retro_api_version()==1; assert all(hasattr(c,n) for n in ['retro_init','retro_run','retro_load_game','retro_get_system_info'])"
 r=subprocess.run([sys.executable,'-c',probe,str(path)],capture_output=True,timeout=10)
 if r.returncode:raise EmulatorError('Core incompatible avec les bibliothèques du serveur. Aucun fichier installé.')
def system_info():
 machine=platform.machine().lower();arch=ARCH.get(machine)
 try:ram=int(next(line.split()[1] for line in Path('/proc/meminfo').read_text().splitlines() if line.startswith('MemTotal:')))*1024
 except (OSError,StopIteration,ValueError):ram=0
 return {'os':platform.system(),'architecture':machine,'platform':('linux/'+arch[0]) if platform.system()=='Linux' and arch else None,'ramGiB':round(ram/1024**3),'execution':'server'}
def inventory(library):
 info=system_info();expected=ARCH.get(info['architecture'],('',0))[1];items=[]
 for console,extensions in FORMATS.items():
  folder=library.mount/'roms'/console
  if not folder.is_dir() or folder.is_symlink():continue
  count=sum(p.is_file() and not p.is_symlink() and p.suffix.lower().lstrip('.') in extensions for p in folder.iterdir())
  if not count:continue
  core,name,license=CORES.get(console,('','Configuration manuelle',''))
  target=library.mount/'cores'/console/(core+'_libretro.so');installed=False
  if core and target.is_file() and not target.is_symlink():
   try:installed=elf_machine(target)==expected
   except (OSError,EmulatorError):pass
  with LOCK:job=dict(JOBS[console]) if console in JOBS else None
  items.append({'consoleId':console,'games':count,'core':core,'name':name,'license':license,'installed':installed,'canInstall':bool(info['platform'] and console in INSTALLABLE),'status':'installed' if installed else 'missing','job':job})
 return {'hardware':info,'items':items}
def start(library,console):
 info=system_info()
 if console not in INSTALLABLE or not info['platform']:raise EmulatorError('Installation automatique non vérifiée pour cette console ou cette architecture.')
 item=next((i for i in inventory(library)['items'] if i['consoleId']==console),None)
 if not item:raise EmulatorError('Aucun jeu installé pour cette console.')
 if item['installed']:return {'status':'succeeded','consoleId':console,'message':'Émulateur déjà installé.'}
 with LOCK:
  if console in JOBS and JOBS[console]['status']=='running':return dict(JOBS[console])
  JOBS[console]={'consoleId':console,'status':'running','phase':'queued','message':'Installation en attente…','bytesReceived':0,'totalBytes':None}
 POOL.submit(install,library,console);return dict(JOBS[console])
def install(library,console):
 def update(**values):
  with LOCK:JOBS[console].update(values)
 partial=None
 try:
  info=system_info();arch,machine=ARCH[info['architecture']];core,name,license=CORES[console]
  target_dir=library.mount/'cores'/console
  if (library.mount/'cores').is_symlink() or target_dir.is_symlink():raise EmulatorError('Dossier de cores refusé.')
  target_dir.mkdir(parents=True,exist_ok=True)
  target=target_dir/(core+'_libretro.so')
  if target.exists() or target.is_symlink():raise EmulatorError('Un core est déjà présent ; il a été conservé.')
  url='https://buildbot.libretro.com/nightly/linux/'+arch+'/latest/'+core+'_libretro.so.zip'
  update(phase='downloading',message='Téléchargement de '+name+' pour Linux '+arch+'…')
  req=urllib.request.Request(url,headers={'User-Agent':'GameFlex-Core-Assistant/1.0'})
  body=bytearray();deadline=time.monotonic()+120
  with urllib.request.build_opener(NoRedirect).open(req,timeout=20) as response:
   total=int(response.headers.get('Content-Length','0')) or None
   if total and total>32*1024*1024:raise EmulatorError('Archive de core trop volumineuse.')
   while True:
    if time.monotonic()>deadline:raise EmulatorError('Téléchargement trop lent. Réessayez.')
    block=response.read(65536)
    if not block:break
    body.extend(block)
    if len(body)>32*1024*1024:raise EmulatorError('Archive de core trop volumineuse.')
    update(bytesReceived=len(body),totalBytes=total)
  update(phase='validating',message='Contrôle ARM/ABI et dépendances…')
  with zipfile.ZipFile(io.BytesIO(body)) as archive:
   entries=[e for e in archive.infolist() if e.filename==core+'_libretro.so']
   if len(entries)!=1 or not 0<entries[0].file_size<=64*1024*1024:raise EmulatorError('Archive du core invalide.')
   space=os.statvfs(target_dir)
   if space.f_bavail*space.f_frsize<entries[0].file_size+10*1024*1024:raise EmulatorError('Espace disque insuffisant.')
   fd,tmp=tempfile.mkstemp(prefix='.core-',suffix='.part',dir=target_dir);partial=Path(tmp)
   with os.fdopen(fd,'wb') as out,archive.open(entries[0]) as source:
    written=0
    while block:=source.read(1024*1024):
     written+=len(block)
     if written>64*1024*1024:raise EmulatorError('Core décompressé trop volumineux.')
     out.write(block)
    out.flush();os.fsync(out.fileno())
  validate_core(partial,machine);digest=hashlib.sha256(partial.read_bytes()).hexdigest()
  os.chmod(partial,0o644);os.link(partial,target,follow_symlinks=False)
  atomic_json(library.data/'emulator-installations'/(console+'.json'),{'core':core,'platform':info['platform'],'sha256':digest,'source':url,'installedAt':time.time(),'license':license})
  update(status='succeeded',phase='completed',message=name+' installé.',sha256=digest)
 except EmulatorError as e:update(status='failed',phase='failed',message=str(e))
 except Exception:update(status='failed',phase='failed',message='Installation impossible. Le core existant est conservé ; réessayez.')
 finally:
  if partial:partial.unlink(missing_ok=True)
