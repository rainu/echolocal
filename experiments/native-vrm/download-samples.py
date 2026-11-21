#!/usr/bin/env python3
"""Fetch public samples into an explicit PRIVATE artifact directory, not Git.

Read README/THIRD_PARTY and the embedded model terms before use. This script
neither changes metadata nor implies permission for arbitrary avatar usage.
"""
import argparse
import hashlib
from pathlib import Path
import urllib.request

SPEC = "https://raw.githubusercontent.com/vrm-c/vrm-specification/94e82dd346fa6cf0337c4421728640e5252dd38e/samples/"
SAMPLES = [
    ("sample-vrm1.vrm", SPEC + "VRM1_Constraint_Twist_Sample/vrm/VRM1_Constraint_Twist_Sample.vrm",
     "12c2b97e95e700783a6a550dc0eee2d7880aeedccef9ae67bc4c5a2f0f2631a2"),
    ("Seed-san.vrm", SPEC + "Seed-san/vrm/Seed-san.vrm",
     "624d0d554bc205bbdc33e22a68a2c3c20edebb3e573011ead8878a65e5329b23"),
    ("alicia-vrm0.vrm", "https://raw.githubusercontent.com/vrm-c/UniVRM/efd20f62f1bf0ac3fcc34de1e497324c5197d146/Tests/Models/Alicia_vrm-0.51/AliciaSolid_vrm-0.51.vrm",
     "237bb02efadf8c13a114af91dd8e860173081457dee87017e51011c448d05dc2"),
]


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("private_output", type=Path)
    args = p.parse_args()
    args.private_output.mkdir(parents=True, exist_ok=True, mode=0o700)
    for name, url, expected in SAMPLES:
        target = args.private_output / name
        data = target.read_bytes() if target.exists() else urllib.request.urlopen(url, timeout=60).read(64 * 1024 * 1024 + 1)
        if hashlib.sha256(data).hexdigest() != expected:
            raise SystemExit("SHA256 mismatch: " + name)
        if not target.exists():
            target.write_bytes(data)
            target.chmod(0o600)
        print(name, "verified")


if __name__ == "__main__":
    main()
