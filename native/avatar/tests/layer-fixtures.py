#!/usr/bin/env python3
"""Authored, deterministic desk/alpha fixtures; no external assets."""
import argparse
from pathlib import Path
import struct
import zlib


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    w, h = 320, 200
    for name in ["background", "clear", "desk"]:
        rows = []
        for y in range(h):
            row = bytearray()
            for x in range(w):
                if name == "background":
                    pixel = (24 + (x // 40) * 8, 120, 180, 255)
                elif name == "clear" or y < 100:
                    pixel = (0, 0, 0, 0)
                elif y < 150:
                    pixel = (240, 24, 48, 128)
                else:
                    pixel = (96, 48, 24, 255)
                row.extend(pixel)
            rows.append(bytes(row))

        def chunk(kind, payload):
            return (struct.pack(">I", len(payload)) + kind + payload
                    + struct.pack(">I", zlib.crc32(kind + payload) & 0xffffffff))
        png = (b"\x89PNG\r\n\x1a\n"
               + chunk(b"IHDR", struct.pack(">IIBBBBB", w, h, 8, 6, 0, 0, 0))
               + chunk(b"IDAT", zlib.compress(b"".join(b"\0" + row for row in rows)))
               + chunk(b"IEND", b""))
        (args.output / (name + ".png")).write_bytes(png)
        (args.output / (name + ".rgba")).write_bytes(
            b"BG01" + struct.pack("<II", w, h) + b"".join(rows))


if __name__ == "__main__":
    main()
