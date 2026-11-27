#!/usr/bin/env python3
"""Compare GPU PPM snapshots using the deterministic layer fixtures."""
import argparse
import math
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
    ys = set(range(10, h - 10, 7))
    for boundary in [0.5, 0.75]:
        y = round(h * ((boundary - 0.5) / cy + 0.5))
        ys.update(range(max(0, y-2), min(h, y+3)))
    for y in sorted(ys):
        v = ((y + 0.5) / h - 0.5) * cy + 0.5
        key = "transparent" if v < 0.5 else "partial" if v < 0.75 else "opaque"
        texel = max(0, min(199, v * 200 - 0.5))
        low = math.floor(texel)
        fraction = texel - low

        def pixel(row):
            if row < 100:
                return (0, 0, 0, 0)
            if row < 150:
                return (120, 12, 24, 128)  # premultiplied once, before filtering
            return (96, 48, 24, 255)

        a, b = pixel(low), pixel(min(199, low+1))
        rgba = [a[c] * (1-fraction) + b[c] * fraction for c in range(4)]
        alpha = rgba[3] / 255
        for x in range(x0 + 10, w - 10, 11):
            if y > h - 100 and abs(x - w/2) < 100:
                continue  # fullscreen accessible controls intentionally cut through
            i = (y * w + x) * 3
            expected = [round(rgba[c] + base[i+c] * (1-alpha)) for c in range(3)]
            # GLES texture units may quantize bilinear weights (the target
            # hardware uses 1/16 steps). Keep strict checks away from transitions
            # and allow at most one such weight step at the two cutout edges.
            tolerance = 8 if low in (99, 149) else 2
            assert all(abs(actual[i+c] - expected[c]) <= tolerance for c in range(3)), (
                key, x, y, list(actual[i:i+3]), expected)
            counts[key] += 1
    assert all(counts.values()), counts
    print("LAYER_PIXELS=PASS", counts)


if __name__ == "__main__":
    main()
