#!/usr/bin/env python3
"""Read-only local checks. No Docker daemon access, network or TV discovery."""
import json
from pathlib import Path
import platform
import shutil

def main():
    p = Path('/proc/device-tree/model')
    model = p.read_text().strip('\x00') if p.exists() else next((line.split(':',1)[1].strip() for line in Path('/proc/cpuinfo').read_text().splitlines() if line.startswith('Model')), 'non visible dans cet environnement')
    print(json.dumps({'architecture':platform.machine(),'model':model,'python':platform.python_version(),'dockerCli':bool(shutil.which('docker')),'driVisible':Path('/dev/dri').exists(),'uinputVisible':Path('/dev/uinput').exists(),'upstreamArmSupport':'non validé ; ne pas lancer launch.sh comme une installation ARM officielle','deviceVisibility':'Les périphériques absents du conteneur peuvent exister sur l’hôte ; aucun module chargé.', 'tv':'aucun accès effectué'},ensure_ascii=False,indent=2))
if __name__ == '__main__': main()
