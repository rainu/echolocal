#!/usr/bin/env python3
"""Compare GPU PPM snapshots using the deterministic layer fixtures."""
import argparse
from pathlib import Path


def ppm(path):
    header, dims, maxval, data = Path(path).read_bytes().split(b"\n", 3)
    assert header == b"P6" and maxval == b"255"
    w, h = map(int, dims.split())
    assert len(data) == w * h * 3
    return w, h, data


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("baseline")
    parser.add_argument("composite")
    parser.add_argument("--split", action="store_true")
    args = parser.parse_args()
    w, h, base = ppm(args.baseline)
    cw, ch, actual = ppm(args.composite)
    assert (w, h) == (cw, ch)
    x0 = w // 2 if args.split else 0
    vw = w - x0
    aspect = 320 / 200
    cx, cy = (vw / h / aspect, 1) if aspect > vw / h else (1, aspect / (vw / h))
    counts = {"transparent": 0, "partial": 0, "opaque": 0}
    for y in range(10, h - 10, 7):
        v = ((y + 0.5) / h - 0.5) * cy + 0.5
        # Avoid bilinear transition rows and fullscreen accessible cutouts.
        if min(abs(v - 0.5), abs(v - 0.75)) < 0.02 or y > h - 100:
            continue
        key = "transparent" if v < 0.5 else "partial" if v < 0.75 else "opaque"
        alpha = 0 if key == "transparent" else 128 / 255 if key == "partial" else 1
        color = (0, 0, 0) if alpha == 0 else (240, 24, 48) if alpha < 1 else (96, 48, 24)
        for x in range(x0 + 10, w - 10, 11):
            i = (y * w + x) * 3
            expected = [round(color[c] * alpha + base[i+c] * (1-alpha)) for c in range(3)]
            assert all(abs(actual[i+c] - expected[c]) <= 2 for c in range(3)), (
                key, x, y, list(actual[i:i+3]), expected)
            counts[key] += 1
    assert all(counts.values()), counts
    print("LAYER_PIXELS=PASS", counts)


if __name__ == "__main__":
    main()
