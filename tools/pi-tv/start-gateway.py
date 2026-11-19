"""Compatibility entrypoint: the ARM64 adapter supersedes the QEMU experiment."""
import runpy
from pathlib import Path
runpy.run_path(str(Path(__file__).with_name('start-native-gateway.py')),run_name='__main__')
