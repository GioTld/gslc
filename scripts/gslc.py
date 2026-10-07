#!/usr/bin/env python3
"""Installed entry point; compilation behavior stays in the shared driver."""
import json
from pathlib import Path
import sys

root = Path(__file__).resolve().parents[1] / "lib/gslc"
sys.path.insert(0, str(root / "scripts"))

if sys.argv[1:] == ["--version"]:
    metadata = json.loads((root / "compiler.json").read_text())
    print(f"gslc {metadata['host']} {metadata['source_revision']}")
else:
    import gsl
    sys.exit(gsl.main(installed=True))
