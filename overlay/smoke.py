#!/usr/bin/env python3
"""Compat entrypoint — prefer: python3 -m overlay"""

from __future__ import annotations

import runpy
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
if str(ROOT) not in sys.path:
    sys.path.insert(0, str(ROOT))

runpy.run_module("overlay.main", run_name="__main__")
