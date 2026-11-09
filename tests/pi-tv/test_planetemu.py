import sys
import tempfile
import unittest
from pathlib import Path
from unittest.mock import patch
sys.path.insert(0,str(Path(__file__).resolve().parents[2]/'tools/pi-tv'))
import planetemu

class PlanetemuTests(unittest.TestCase):
    def test_only_catalogue_paths(self):
        for path in ['/rom/set/file','/php/roms/download.php','https://evil.test/roms/set','/roms/set.zip','/roms/set?page=A&download=1']:
            with self.assertRaises(ValueError):planetemu.read_html(path)
    def test_metadata_cache_excludes_bios_and_never_fetches_rom(self):
        paths=[]
        def html(path):
            paths.append(path)
            if path=='/robots.txt':return 'User-agent: *\nAllow: /'
            if path.startswith('/machine/'):return '<a href="/roms/games">Games</a><a href="/roms/bios">BIOS</a>'
            return '<a href="/rom/games/free-title">Free title</a><a href="/rom/games/boot">[BIOS] Boot ROM</a><a href="/rom/games/firmware">Firmware</a>'
        with tempfile.TemporaryDirectory() as folder,patch.object(planetemu,'read_html',side_effect=html),patch.object(planetemu.time,'sleep'):
            first=planetemu.catalogue(folder,'nes','F');n=len(paths)
            self.assertEqual([x['title'] for x in first['items']],['Free title'])
            self.assertEqual(first,planetemu.catalogue(folder,'nes','F'))
            self.assertEqual(n,len(paths));self.assertFalse(any(x.startswith('/rom/') for x in paths))
    def test_unsupported_console_and_invalid_letter(self):
        with tempfile.TemporaryDirectory() as folder:
            self.assertEqual(planetemu.catalogue(folder,'psp')['items'],[])
            with self.assertRaises(ValueError):planetemu.catalogue(folder,'nes','../')
    def test_source_ignoring_letter_is_filtered_locally(self):
        def html(path):
            if path=='/robots.txt':return 'User-agent: *\nAllow: /'
            if path.startswith('/machine/'):return '<a href="/roms/fds">FDS</a><a href="/roms/nes">NES</a>'
            return '<a href="/rom/fds/adian">Adian no Tsue (Japan)</a><a href="/rom/nes/mario">Mario Bros. (World)</a><a href="/rom/nes/metroid">Metroid (USA)</a>'
        with tempfile.TemporaryDirectory() as folder,patch.object(planetemu,'read_html',side_effect=html),patch.object(planetemu.time,'sleep'):
            result=planetemu.catalogue(folder,'nes','M')
            self.assertEqual(result['letter'],'M')
            self.assertEqual([x['title'] for x in result['items']],['Mario Bros. (World)','Metroid (USA)'])
    def test_numeric_and_accented_letters(self):
        self.assertTrue(planetemu.matches_letter('Évasion (France)','E'))
        self.assertTrue(planetemu.matches_letter('1942 (USA)','0'))
        self.assertFalse(planetemu.matches_letter('Adian (Japan)','M'))
