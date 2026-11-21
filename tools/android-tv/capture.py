#!/usr/bin/env python3
"""Capture only the foreground GameFlex window after association (no credentials)."""
import argparse,sys,re,time,xml.etree.ElementTree as ET
from pathlib import Path
from tv_device import TV
ROOT=Path(__file__).resolve().parents[2]
p=argparse.ArgumentParser();p.add_argument('label',choices=['tv-library','tv-details','tv-game','tv-return','tv-exit']);a=p.parse_args()
with TV() as tv:
 foreground=tv.shell('dumpsys activity activities').split('mResumedActivity:')[-1].splitlines()[0]
 if 'org.gameflex.tv/' not in foreground:raise SystemExit('Capture refused: GameFlex is not foreground')
 remote='/data/local/tmp/gameflex-capture-check.xml';tv.shell('uiautomator dump '+remote);raw=tv.shell('cat '+remote);tv.shell('rm '+remote)
 nodes=ET.fromstring(raw[raw.index('<?xml'):])
 if any(n.get('password')=='true' or re.fullmatch('[0-9]{6}',n.get('text','')) for n in nodes.iter('node')):raise SystemExit('Capture refused: pairing or credential UI')
 out=ROOT/'output/android-tv'/ (a.label+'.png')
 remote='/data/local/tmp/gameflex-screen-check.png'
 time.sleep(4) # let the transient UI-automation system toast disappear
 tv.shell('screencap -p '+remote,timeout=60)
 tv.device.pull(remote,str(out),transport_timeout_s=20,read_timeout_s=60)
 tv.shell('rm '+remote)
 if not out.read_bytes().startswith(b'\x89PNG\r\n\x1a\n'):raise SystemExit('Screenshot invalid')
 print(str(out))
