import importlib.util,os
from pathlib import Path
root=Path(__file__).resolve().parents[2]
s=importlib.util.spec_from_file_location('deploy',root/'tools/pi-tv/deploy-local.py');m=importlib.util.module_from_spec(s);s.loader.exec_module(m)
folder=root/'.local/host-relay';folder.mkdir(parents=True,exist_ok=True);os.chown(folder,1000,1000);os.chmod(folder,0o700)
common=['--user','1000:1000','--cpus','.5','--memory','96m','--pids-limit','64','--read-only','--cap-drop','ALL','--security-opt','no-new-privileges:true','--tmpfs','/tmp:rw,size=8m','--mount','type=bind,src='+str(root/'tools/pi-tv/host-relay.py')+',dst=/app/relay.py,readonly','--mount','type=bind,src='+str(folder)+',dst=/relay']
m.run('gameflex-host-service',['--network','host']+common+['python:3.11-slim','python3','/app/relay.py','host'])
m.run('gameflex-host-relay',['--network','gameflex-local']+common+['python:3.11-slim','python3','/app/relay.py','bridge'])
