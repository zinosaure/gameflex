import sys,tempfile,threading,unittest,hashlib,json
from pathlib import Path
from io import BytesIO
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
from library import Library,LibraryError
from upload_sessions import UploadSessions,CHUNK
ROOT=Path(__file__).resolve().parents[2]
class UploadTests(unittest.TestCase):
 def setUp(self):
  self.temp=tempfile.TemporaryDirectory();p=Path(self.temp.name);self.mount=p/'library';self.mount.mkdir();self.lib=Library(p/'data','/library',self.mount);self.lib.configure('/library');self.manager=UploadSessions(threading.BoundedSemaphore(1));self.body=(ROOT/'output/nes-demo/palette-libre.nes').read_bytes()
 def tearDown(self):
  if self.manager.record:self.manager.cancel('alice',self.manager.record['id'])
  self.lib.pool.shutdown();self.temp.cleanup()
 def start(self,body=None,name='Demo.nes'):
  body=self.body if body is None else body
  return self.manager.start(self.lib,'alice',{'consoleId':'nes','filename':name,'size':len(body)})['id']
 def test_acknowledged_progress_over_64_mib_and_atomic_completion(self):
  body=self.body+b'\0'*(65*1024*1024);id=self.start(body);acks=[]
  for offset in range(0,len(body),CHUNK):
   part=body[offset:offset+CHUNK];r=self.manager.chunk('alice',id,offset,BytesIO(part),len(part));acks.append(r['bytesReceived'])
  jobs=self.lib.jobs('alice');self.assertEqual(jobs[0]['progress'],98);self.assertGreater(len(set(acks)),64)
  self.assertFalse((self.mount/'roms/nes/Demo.nes').exists())
  item=self.manager.complete('alice',id)['item'];self.assertEqual(item['sha256'],hashlib.sha256(body).hexdigest());self.assertEqual(self.lib.jobs('alice')[0]['progress'],100)
  self.assertEqual((self.mount/'roms/nes/Demo.nes').stat().st_size,len(body));self.assertIsNone(self.manager.record)
 def test_owner_offsets_cancel_no_overwrite_and_slot_reuse(self):
  id=self.start()
  with self.assertRaises(PermissionError):self.manager.cancel('bob',id)
  with self.assertRaises(LibraryError):self.manager.chunk('alice',id,1,BytesIO(self.body),len(self.body))
  with self.assertRaises(LibraryError):self.manager.complete('alice',id)
  self.manager.cancel('alice',id);self.assertEqual(self.lib.jobs('alice')[0]['status'],'failed')
  id=self.start();self.manager.chunk('alice',id,0,BytesIO(self.body),len(self.body));self.manager.complete('alice',id)
  bad=self.body+b'different';id=self.start(bad);self.manager.chunk('alice',id,0,BytesIO(bad),len(bad))
  with self.assertRaises(LibraryError):self.manager.complete('alice',id)
  self.assertEqual((self.mount/'roms/nes/Demo.nes').read_bytes(),self.body)
 def test_bad_header_and_large_block(self):
  id=self.start()
  with self.assertRaises(LibraryError):self.manager.chunk('alice',id,0,BytesIO(b'MZbad'),5)
  with self.assertRaises(LibraryError):self.manager.chunk('alice',id,0,BytesIO(),CHUNK+1)
if __name__=='__main__':unittest.main()
