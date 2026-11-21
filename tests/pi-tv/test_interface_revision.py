import re,sys,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
from server import validate_preferences
class InterfaceRevisionTests(unittest.TestCase):
 def test_all_registered_interfaces_can_sync(self):
  registry=Path(__file__).resolve().parents[2]/'src/webapp/src/app/gameflex/experience/interfaces.js'
  ids=re.findall(r"\bid\s*:\s*'([^']+)'",registry.read_text())
  self.assertEqual(len(ids),29) # One active appearance, 22 archives and six historical destinations.
  for mode in ids:
   with self.subTest(interface=mode):
    value={'interface':mode,'interfaceUpdatedAt':1794333000000,'favorites':['gba:a'],'history':['gba:a'],'collections':[{'id':'a','name':'Test','games':['gba:a']}]}
    self.assertEqual(validate_preferences(value),value)
 def test_archived_choice_survives_sync(self):
  value={'interface':'ps-home','archivedInterface':'shelf','favorites':['gba:a'],'history':[],'collections':[]}
  self.assertEqual(validate_preferences(value),value)
 def test_timestamp_round_trip_preserves_library(self):
  value={'interface':'shelf','interfaceUpdatedAt':1794333000000,'favorites':['gba:a'],'history':['gba:a'],'collections':[{'id':'a','name':'Test','games':['gba:a']}]}
  self.assertEqual(validate_preferences(value),value)
 def test_old_preferences_still_supported(self):
  self.assertEqual(validate_preferences({'interface':'station'})['interface'],'station')
 def test_bad_revision_refused(self):
  for value in [-1,True,1.5,'123',{},9007199254740992]:
   with self.assertRaises(ValueError):validate_preferences({'interface':'station','interfaceUpdatedAt':value})
if __name__=='__main__':unittest.main()
