import sys,unittest
from pathlib import Path
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
from server import validate_preferences
class InterfaceRevisionTests(unittest.TestCase):
 def test_timestamp_round_trip_preserves_library(self):
  value={'interface':'shelf','interfaceUpdatedAt':1794333000000,'favorites':['gba:a'],'history':['gba:a'],'collections':[{'id':'a','name':'Test','games':['gba:a']}]}
  self.assertEqual(validate_preferences(value),value)
 def test_old_preferences_still_supported(self):
  self.assertEqual(validate_preferences({'interface':'station'})['interface'],'station')
 def test_bad_revision_refused(self):
  for value in [-1,True,1.5,'123',{},9007199254740992]:
   with self.assertRaises(ValueError):validate_preferences({'interface':'station','interfaceUpdatedAt':value})
if __name__=='__main__':unittest.main()
