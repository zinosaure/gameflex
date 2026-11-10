"""Compatibility entry point for the current unified-console journeys."""
import runpy
from pathlib import Path
runpy.run_path(str(Path(__file__).with_name("consoles-first-browser.py")),run_name="__main__")
