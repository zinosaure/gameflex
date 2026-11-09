import hashlib
import json
import os
from pathlib import Path
import sys
import tempfile
import time
import unittest
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
from library import Library,LibraryError,APPROVED

class LibraryTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.base=Path(self.tmp.name);self.mount=self.base/'games';self.mount.mkdir();self.lib=Library(self.base/'data','/chosen/by/user',self.mount)
    def tearDown(self):self.lib.pool.shutdown(wait=True);self.tmp.cleanup()
    def completed(self,user='alice',game='palette-libre'):
        job=self.lib.start(user,game)
        for _ in range(100):
            result=next(x for x in self.lib.jobs(user) if x['id']==job['id'])
            if result['status'] in ('succeeded','failed'):return result
            time.sleep(.01)
        self.fail('Job did not finish')
    def test_setup_requires_explicit_mounted_folder(self):
        self.assertFalse(self.lib.status()['configured'])
        for path in ['/etc','../games','/unmounted/folder',None]:
            with self.assertRaises(LibraryError):self.lib.configure(path)
        with self.assertRaises(LibraryError):self.lib.start('alice','palette-libre')
        self.lib.configure('/chosen/by/user');self.assertTrue(self.lib.status()['configured'])
    def test_approved_content_atomic_install_and_private_jobs(self):
        self.lib.configure('/chosen/by/user');job=self.completed();self.assertEqual(job['status'],'succeeded')
        file=self.mount/'roms/nes/palette-libre.nes';self.assertEqual(hashlib.sha256(file.read_bytes()).hexdigest(),APPROVED['palette-libre']['sha256'])
        self.assertEqual(self.lib.jobs('bob'),[]);self.assertEqual(self.completed()['status'],'succeeded')
        self.assertEqual(list(file.parent.glob('*.part')),[])
    def test_unknown_game_or_url_cannot_be_downloaded(self):
        self.lib.configure('/chosen/by/user')
        for value in ['commercial-title','https://planetemu.net/rom/nes/game','../../outside',{'url':'https://example.com'}]:
            with self.assertRaises(LibraryError):self.lib.start('alice',value)
        self.assertFalse((self.mount/'roms').exists())
    def test_existing_personal_file_is_never_overwritten(self):
        self.lib.configure('/chosen/by/user');folder=self.mount/'roms/nes';folder.mkdir(parents=True);file=folder/'palette-libre.nes';file.write_bytes(b'personal file')
        self.assertEqual(self.completed()['status'],'failed');self.assertEqual(file.read_bytes(),b'personal file')
    def test_symlink_cannot_escape_library(self):
        self.lib.configure('/chosen/by/user');outside=self.base/'outside';outside.mkdir();(self.mount/'roms').symlink_to(outside,target_is_directory=True)
        self.assertEqual(self.completed()['status'],'failed');self.assertEqual(list(outside.iterdir()),[])
    def test_integrity_failure_does_not_publish(self):
        self.lib.configure('/chosen/by/user')
        with patch.dict(APPROVED,{'invalid':{**APPROVED['palette-libre'],'id':'invalid','sha256':'0'*64}}):
            self.assertEqual(self.completed(game='invalid')['status'],'failed')
        self.assertFalse((self.mount/'roms').exists())
    def test_interrupted_job_is_failed_after_restart(self):
        folder=self.lib.job_folder('alice');folder.mkdir(parents=True);(folder/'job.json').write_text(json.dumps({'id':'job','status':'running','createdAt':1}))
        other=Library(self.base/'data','/chosen/by/user',self.mount)
        try:self.assertEqual(other.jobs('alice')[0]['status'],'failed')
        finally:other.pool.shutdown()
    def test_browser_lists_directories_without_files_or_symlinks(self):
        (self.mount/'roms').mkdir();(self.mount/'saves').mkdir();(self.mount/'.private').mkdir();(self.mount/'secret.txt').write_text('private');(self.mount/'escape').symlink_to(self.base,target_is_directory=True)
        home=self.lib.browse();self.assertEqual(home['items'][0]['folder'],'/chosen/by/user');self.assertFalse(home['selectable'])
        folder=self.lib.browse('/chosen/by/user');self.assertEqual([x['name'] for x in folder['items']],['roms','saves']);self.assertTrue(folder['selectable']);self.assertEqual(folder['parent'],'')
        child=self.lib.browse('/chosen/by/user/roms');self.assertEqual(child['parent'],'/chosen/by/user');self.assertFalse(child['selectable']);self.assertEqual(child['items'],[])
        self.assertFalse(self.lib.status()['configured'])
    def test_browser_refuses_escape_and_symlink_components(self):
        outside=self.base/'outside';outside.mkdir();(self.mount/'escape').symlink_to(outside,target_is_directory=True)
        for path in ['/etc','/chosen/by/user/../outside','/chosen/by/user//roms','/chosen/by/user/.private','/chosen/by/user/escape','/chosen/by/userish']:
            with self.assertRaises((LibraryError,OSError)):self.lib.browse(path)
    def test_default_is_existing_roms_folder_without_double_nesting(self):
        (self.mount/'roms').mkdir()
        lib=Library(self.base/'default-data','/chosen/by/user',self.mount,default_roms=True)
        try:
            self.assertTrue(lib.status()['configured']);self.assertEqual(lib.status()['folder'],'/chosen/by/user/roms')
            self.assertEqual(lib.browse()['items'][0]['folder'],'/chosen/by/user/roms')
            self.assertTrue(lib.browse('/chosen/by/user/roms')['selectable'])
            lib.configure('/chosen/by/user/roms')
            game=APPROVED['palette-libre'];lib.write_game(game,(Path(__file__).resolve().parents[2]/game['sourceFile']).read_bytes())
            self.assertTrue((self.mount/'roms/nes/palette-libre.nes').is_file());self.assertFalse((self.mount/'roms/roms').exists())
        finally:lib.pool.shutdown()
