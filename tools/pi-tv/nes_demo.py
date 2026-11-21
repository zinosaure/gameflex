#!/usr/bin/env python3
"""Generate an original CC0 NES cartridge; optionally test a local QuickNES core."""
import argparse
import ctypes as C
import hashlib
import json
from pathlib import Path
import platform
import struct
import time
import urllib.request
import zipfile
import io

ROOT = Path(__file__).resolve().parents[2]

def cartridge():
    code = bytearray(); labels = {}; fixups = []
    def emit(*values): code.extend(values)
    def label(name): labels[name] = 0x8000 + len(code)
    def branch(op, name): emit(op, 0); fixups.append((len(code)-1, name, True))
    def jump(name): emit(0x4c,0,0); fixups.append((len(code)-2,name,False))
    label('reset'); emit(0x78,0xd8,0xa2,0xff,0x9a,0xe8)  # SEI, CLD, LDX #$ff, TXS, INX
    emit(0x8e,0x00,0x20,0x8e,0x01,0x20)  # display off
    label('wait1'); emit(0x2c,0x02,0x20); branch(0x10,'wait1')
    label('wait2'); emit(0x2c,0x02,0x20); branch(0x10,'wait2')
    emit(0xa9,0x3f,0x8d,0x06,0x20,0xa9,0,0x8d,0x06,0x20,0xa2,0)
    label('palette'); emit(0xa9,0x21,0x8d,0x07,0x20,0xe8,0xe0,0x20); branch(0xd0,'palette')
    emit(0xa9,0x20,0x8d,0x06,0x20,0xa9,0,0x8d,0x06,0x20,0xa0,4,0xa2,0)
    label('fill'); emit(0xa9,1,0x8d,0x07,0x20,0xe8); branch(0xd0,'fill'); emit(0x88); branch(0xd0,'fill')
    # Clear attribute bytes, then write original GAMEFLEX glyphs.
    emit(0xa9,0x23,0x8d,0x06,0x20,0xa9,0xc0,0x8d,0x06,0x20,0xa2,0x40,0xa9,0)
    label('attributes'); emit(0x8d,0x07,0x20,0xca); branch(0xd0,'attributes')
    emit(0xa9,0x21,0x8d,0x06,0x20,0xa9,0xac,0x8d,0x06,0x20)
    for tile in range(2,10): emit(0xa9,tile,0x8d,0x07,0x20)
    emit(0xa9,0,0x85,0,0x8d,0x05,0x20,0x8d,0x05,0x20,0xa9,0x0a,0x8d,0x01,0x20)
    label('loop'); emit(0x2c,0x02,0x20); branch(0x10,'loop')
    emit(0xa9,0x3f,0x8d,0x06,0x20,0xa9,1,0x8d,0x06,0x20,0xe6,0,0xa5,0,0x29,0x2f,0x8d,0x07,0x20)
    # Reset nametable selection after writes to palette RAM.
    emit(0xa9,0x20,0x8d,0x06,0x20,0xa9,0,0x8d,0x06,0x20,0x8d,0x05,0x20,0x8d,0x05,0x20); jump('loop')
    label('interrupt'); emit(0x40)
    for at, name, relative in fixups:
        value = labels[name] - (0x8000 + at + 1) if relative else labels[name]
        if relative:
            assert -128 <= value <= 127; code[at] = value & 255
        else: code[at:at+2] = struct.pack('<H', value)
    prg = bytearray([0xea])*16384; prg[:len(code)] = code
    prg[-6:] = struct.pack('<HHH',labels['interrupt'],labels['reset'],labels['interrupt'])
    chr_data = bytearray(8192)
    chr_data[16:24] = bytes([0x81,0x42,0x24,0x18,0x18,0x24,0x42,0x81])
    glyphs = [[0x3c,0x42,0x40,0x4e,0x42,0x42,0x3c,0], [0x18,0x24,0x42,0x7e,0x42,0x42,0x42,0], [0x42,0x66,0x5a,0x5a,0x42,0x42,0x42,0], [0x7e,0x40,0x40,0x7c,0x40,0x40,0x7e,0], [0x7e,0x40,0x40,0x7c,0x40,0x40,0x40,0], [0x40,0x40,0x40,0x40,0x40,0x40,0x7e,0], [0x7e,0x40,0x40,0x7c,0x40,0x40,0x7e,0], [0x42,0x42,0x24,0x18,0x24,0x42,0x42,0]]
    for i, glyph in enumerate(glyphs, 2): chr_data[i*16:i*16+8] = bytes(glyph)
    return b'NES\x1a'+bytes([1,1,0,0])+bytes(8)+prg+chr_data

class GameInfo(C.Structure):
    _fields_=[('path',C.c_char_p),('data',C.c_void_p),('size',C.c_size_t),('meta',C.c_char_p)]


def run_core(core, rom, output):
    lib = C.CDLL(str(core.resolve()))
    fmt = [0]; frames = []; last = [None]
    env_t = C.CFUNCTYPE(C.c_bool,C.c_uint,C.c_void_p)
    video_t = C.CFUNCTYPE(None,C.c_void_p,C.c_uint,C.c_uint,C.c_size_t)
    sample_t = C.CFUNCTYPE(None,C.c_int16,C.c_int16)
    batch_t = C.CFUNCTYPE(C.c_size_t,C.POINTER(C.c_int16),C.c_size_t)
    poll_t = C.CFUNCTYPE(None)
    input_t = C.CFUNCTYPE(C.c_int16,C.c_uint,C.c_uint,C.c_uint,C.c_uint)
    directory = str(output.resolve()).encode()
    @env_t
    def env(cmd, ptr):
        if cmd == 10:
            proposed = C.cast(ptr,C.POINTER(C.c_int)).contents.value
            if proposed not in (0,1,2): return False
            fmt[0] = proposed; return True
        if cmd in (9,31): C.cast(ptr,C.POINTER(C.c_char_p))[0] = directory; return True
        if cmd in (2,17,49): C.cast(ptr,C.POINTER(C.c_bool))[0] = False; return True
        if cmd == 3: C.cast(ptr,C.POINTER(C.c_bool))[0] = True; return True
        if cmd == 39: C.cast(ptr,C.POINTER(C.c_uint))[0] = 0; return True
        return cmd in (8,11,16,18,35,37)
    @video_t
    def video(data,w,h,pitch):
        if not data or data == C.c_void_p(-1).value: return
        if w > 2048 or h > 2048 or pitch > 32768: return
        raw = C.string_at(data,pitch*h)
        frames.append(hashlib.sha256(raw).hexdigest()); last[0] = (w,h,pitch,raw,fmt[0])
    @sample_t
    def sample(l,r): pass
    @batch_t
    def batch(data,n): return n
    @poll_t
    def poll(): pass
    @input_t
    def inputs(port,device,index,ident): return 0
    for name,cb,tp in [('environment',env,env_t),('video_refresh',video,video_t),('audio_sample',sample,sample_t),('audio_sample_batch',batch,batch_t),('input_poll',poll,poll_t),('input_state',inputs,input_t)]:
        setter=getattr(lib,'retro_set_'+name);setter.argtypes=[tp];setter.restype=None;setter(cb)
    lib.retro_init.argtypes=[];lib.retro_init.restype=None;lib.retro_init()
    data = C.create_string_buffer(rom.read_bytes()); info=GameInfo(str(rom.resolve()).encode(),C.cast(data,C.c_void_p),len(data)-1,None)
    lib.retro_load_game.argtypes=[C.POINTER(GameInfo)];lib.retro_load_game.restype=C.c_bool
    if not lib.retro_load_game(C.byref(info)): raise RuntimeError('cartouche refusée')
    lib.retro_run.argtypes=[];lib.retro_run.restype=None
    started=time.monotonic()
    for _ in range(300): lib.retro_run()
    elapsed=time.monotonic()-started
    lib.retro_unload_game.argtypes=[];lib.retro_unload_game.restype=None;lib.retro_unload_game()
    lib.retro_deinit.argtypes=[];lib.retro_deinit.restype=None;lib.retro_deinit()
    if not last[0] or len(set(frames)) < 2: raise RuntimeError('aucune animation détectée')
    w,h,pitch,raw,pixel_format=last[0]; rgb=bytearray()
    for y in range(h):
        for x in range(w):
            size=4 if pixel_format==1 else 2
            pixel=int.from_bytes(raw[y*pitch+x*size:y*pitch+x*size+size],'little')
            if pixel_format==1: rgb.extend(((pixel>>16)&255,(pixel>>8)&255,pixel&255))
            elif pixel_format==2: rgb.extend((((pixel>>11)&31)*255//31,((pixel>>5)&63)*255//63,(pixel&31)*255//31))
            else: rgb.extend((((pixel>>10)&31)*255//31,((pixel>>5)&31)*255//31,(pixel&31)*255//31))
    (output/'nes-frame.ppm').write_bytes(f'P6\n{w} {h}\n255\n'.encode()+rgb)
    return {'status':'passed','frames':len(frames),'differentFrames':len(set(frames)),'seconds':round(elapsed,3),'resolution':[w,h],'coreSha256':hashlib.sha256(core.read_bytes()).hexdigest()}

def main():
    p=argparse.ArgumentParser(description=__doc__);p.add_argument('--core',type=Path);p.add_argument('--fetch-core',action='store_true');p.add_argument('--output',type=Path,default=ROOT/'output/nes-demo')
    a=p.parse_args();a.output.mkdir(parents=True,exist_ok=True)
    rom=a.output/'palette-libre.nes';rom.write_bytes(cartridge())
    result={'architecture':platform.machine(),'romSha256':hashlib.sha256(rom.read_bytes()).hexdigest(),'contentLicense':'CC0-1.0','content':'Original GameFlex Palette libre — no commercial ROM, BIOS, firmware or external game content','status':'prepared'}
    core=a.core
    if a.fetch_core:
        try:
            if platform.machine() != 'aarch64': raise RuntimeError('architecture non prise en charge par ce téléchargement')
            url='https://buildbot.libretro.com/nightly/linux/aarch64/latest/quicknes_libretro.so.zip'
            with urllib.request.urlopen(url,timeout=15) as r: archive=r.read(8*1024*1024+1)
            if len(archive)>8*1024*1024: raise RuntimeError('archive trop grosse')
            with zipfile.ZipFile(io.BytesIO(archive)) as z:
                candidates=[n for n in z.namelist() if Path(n).name=='quicknes_libretro.so']
                if len(candidates)!=1 or z.getinfo(candidates[0]).file_size>16*1024*1024: raise RuntimeError('archive invalide')
                core=ROOT/'.local/pi-tv/cores/quicknes_libretro.so';core.parent.mkdir(parents=True,exist_ok=True);core.write_bytes(z.read(candidates[0]))
            result.update(coreSource=url,archiveSha256=hashlib.sha256(archive).hexdigest(),supplyChain='HTTPS officiel ; empreinte locale, non comparée à une signature amont')
        except Exception:
            result.update(status='blocked',reason='Téléchargement du core impossible dans cet environnement ; aucun core exécuté.')
    if core:
        try: result.update(run_core(core,rom,a.output))
        except Exception as e: result.update(status='failed',reason=type(e).__name__)
    (a.output/'result.json').write_text(json.dumps(result,ensure_ascii=False,indent=2)+'\n')
    print(json.dumps(result,ensure_ascii=False,indent=2))
    if result['status'] in ('failed','blocked'): raise SystemExit(1)
if __name__=='__main__': main()
