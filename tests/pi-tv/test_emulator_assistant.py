import sys,tempfile,unittest,struct,io,zipfile,threading
from pathlib import Path
from types import SimpleNamespace
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import emulator_assistant as a
class AssistantTests(unittest.TestCase):
 def setUp(self):
  self.temp=tempfile.TemporaryDirectory();self.root=Path(self.temp.name);self.lib=SimpleNamespace(mount=self.root/'library',data=self.root/'data');self.lib.mount.mkdir();a.JOBS.clear()
 def tearDown(self):self.temp.cleanup()
 def test_inventory_only_installed_library_and_machine(self):
  folder=self.lib.mount/'roms/gba';folder.mkdir(parents=True);(folder/'demo.gba').write_bytes(b'original demo');(folder/'manual.txt').write_text('not a game')
  with patch.object(a,'system_info',return_value={'architecture':'aarch64','platform':'linux/aarch64','ramGiB':8}):
   row=a.inventory(self.lib)['items'][0];self.assertEqual(row['games'],1);self.assertEqual(row['core'],'mgba');self.assertFalse(row['installed']);self.assertTrue(row['canInstall'])
  with patch.object(a,'system_info',return_value={'architecture':'unknown','platform':None,'ramGiB':8}):self.assertFalse(a.inventory(self.lib)['items'][0]['canInstall'])
 def test_n64_and_nds_use_the_registered_cores_on_supported_linux(self):
  for console,name in [('n64','demo.z64'),('nds','demo.nds')]:
   folder=self.lib.mount/'roms'/console;folder.mkdir(parents=True);(folder/name).write_bytes(b'fixture')
  with patch.object(a,'system_info',return_value={'os':'Linux','architecture':'aarch64','platform':'linux/aarch64','ramGiB':8,'execution':'server'}):
   rows={item['consoleId']:item for item in a.inventory(self.lib)['items']}
  self.assertEqual(rows['n64']['core'],'parallel_n64');self.assertEqual(rows['n64']['name'],'ParaLLEl N64');self.assertTrue(rows['n64']['canInstall'])
  self.assertEqual(rows['nds']['core'],'desmume');self.assertEqual(rows['nds']['name'],'DeSmuME');self.assertTrue(rows['nds']['canInstall'])
 def test_n64_download_selects_the_arm64_buildbot_archive(self):
  folder=self.lib.mount/'roms/n64';folder.mkdir(parents=True);(folder/'demo.z64').write_bytes(b'fixture')
  archive=io.BytesIO()
  with zipfile.ZipFile(archive,'w') as z:z.writestr('parallel_n64_libretro.so',b'validated fixture')
  response=io.BytesIO(archive.getvalue());response.headers={'Content-Length':str(len(archive.getvalue()))}
  with patch.object(a,'system_info',return_value={'os':'Linux','architecture':'aarch64','platform':'linux/aarch64','ramGiB':8,'execution':'server'}),patch.object(a,'validate_core'),patch.object(a.urllib.request,'build_opener') as opener:
   opener.return_value.open.return_value=response;a.JOBS['n64']={};a.install(self.lib,'n64')
  request=opener.return_value.open.call_args.args[0]
  self.assertEqual(request.full_url,'https://buildbot.libretro.com/nightly/linux/aarch64/latest/parallel_n64_libretro.so.zip')
  self.assertEqual(a.JOBS['n64']['status'],'succeeded');self.assertTrue((self.lib.mount/'cores/n64/parallel_n64_libretro.so').is_file())
 def test_wrong_machine_and_non_library_are_refused(self):
  p=self.root/'core.so';data=bytearray(64);data[:6]=b'\x7fELF\x02\x01';struct.pack_into('<HH',data,16,3,62);p.write_bytes(data)
  with self.assertRaises(a.EmulatorError):a.validate_core(p,183)
  p.write_bytes(b'not a core')
  with self.assertRaises(a.EmulatorError):a.elf_machine(p)
 def test_existing_core_preserved_and_invalid_zip_never_published(self):
  target=self.lib.mount/'cores/gba/mgba_libretro.so';target.parent.mkdir(parents=True);target.write_bytes(b'existing')
  a.JOBS['gba']={};a.install(self.lib,'gba');self.assertEqual(target.read_bytes(),b'existing');self.assertEqual(a.JOBS['gba']['status'],'failed');target.unlink()
  data=io.BytesIO()
  with zipfile.ZipFile(data,'w') as z:z.writestr('../../mgba_libretro.so',b'unsafe')
  response=io.BytesIO(data.getvalue());response.headers={'Content-Length':str(len(data.getvalue()))}
  with patch.object(a.urllib.request,'build_opener') as opener:
   opener.return_value.open.return_value=response;a.JOBS['gba']={};a.install(self.lib,'gba')
  self.assertEqual(a.JOBS['gba']['status'],'failed');self.assertFalse(target.exists());self.assertFalse(list(target.parent.glob('*.part')))
 def test_private_arbitrary_url_or_unknown_console_not_accepted(self):
  for console in ['../../x','ps5','https://example.org/emu']:
   with self.assertRaises(a.EmulatorError):a.start(self.lib,console)
if __name__=='__main__':unittest.main()

class AssistantApiTests(unittest.TestCase):
 def test_api_requires_account_and_admin_for_install(self):
  import test_api as endpoint
  from types import SimpleNamespace
  self.s=SimpleNamespace(data=Path('/tmp'),auth_base='http://127.0.0.1',library=None)
  with patch.object(endpoint.api,'verified_identity',return_value=None):
   self.assertEqual(endpoint.ApiTests.request(self,'/v1/emulators')[0],401)
   self.assertEqual(endpoint.ApiTests.request(self,'/v1/emulators/gba/install','POST',{},'fixture')[0],401)
  with patch.object(endpoint.api,'verified_identity',return_value={'username':'member','role_level':1}):
   self.assertEqual(endpoint.ApiTests.request(self,'/v1/emulators/gba/install','POST',{},'fixture')[0],403)
