"""Replace only the managed GameFlex experience API; preserve every other service."""
import importlib.util,subprocess
from pathlib import Path
R=Path(__file__).resolve().parents[2];spec=importlib.util.spec_from_file_location('deployment',R/'tools/pi-tv/deploy-local.py');d=importlib.util.module_from_spec(spec);spec.loader.exec_module(d)
old=d.inspect('gameflex-experience');assert old and old['Config']['Labels'].get('org.gameflex.project')==str(R)
# No active transfer/metadata task is interrupted.
import json
for pattern in ['downloads/*/*.json','metadata-jobs/*.json']:
 for p in (R/'.local/pi-tv').glob(pattern):
  if json.loads(p.read_text()).get('status') in ('queued','running'):raise SystemExit('Active GameFlex job: deployment deferred')
subprocess.run(['docker','tag',old['Image'],'gameflex-experience:before-tv'],check=True)
subprocess.run(['docker','tag','gameflex-experience:tv-candidate','gameflex-experience:pi-local'],check=True)
subprocess.run(['docker','stop','gameflex-experience'],check=True,stdout=subprocess.DEVNULL);subprocess.run(['docker','rm','gameflex-experience'],check=True,stdout=subprocess.DEVNULL)
d.run('gameflex-experience',['--init','--network','gameflex-local','--user','1000:1000','--cpus','1.5','--memory','1024m','--pids-limit','256','--shm-size','128m','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--tmpfs','/tmp:rw,size=192m,mode=1777','--mount','type=bind,src='+str(R)+',dst=/app,readonly','--mount','type=bind,src='+str(R/'.local/pi-tv')+',dst=/data','--mount','type=bind,src='+str(R/'.local/demo-library')+',dst=/library','gameflex-experience:pi-local','python3','/app/tools/pi-tv/server.py','--bind','0.0.0.0','--data','/data','--auth-base','http://gameflex-credential:8080','--library-host',str(R/'.local/demo-library'),'--library-mount','/library','--default-roms'])
