#!/usr/bin/env python3
"""Original deterministic VRMA + PCM fixtures; no voice, model, or room capture."""
import argparse
import json
import math
from pathlib import Path
import struct
import wave


def animation(path: Path, mode: str):
    times = [0, 0.5, 1, 1.5, 2]
    angles = [0, 0.6, 0, -0.6, 0]
    rotations = [[0, math.sin(a / 2), 0, math.cos(a / 2)] for a in angles]
    positions = [[0, 1 + y, 0] for y in [0, 0.04, 0, 0.04, 0]]
    if mode == "CUBICSPLINE":
        rotations = [v for key in rotations for v in [[0] * 4, key, [0] * 4]]
        positions = [v for key in positions for v in [[0] * 3, key, [0] * 3]]
    blob = bytearray()
    views = []
    accessors = []

    def accessor(values, components):
        flat = [n for v in values for n in v] if components > 1 else values
        data = struct.pack("<" + "f" * len(flat), *flat)
        views.append({"buffer": 0, "byteOffset": len(blob), "byteLength": len(data)})
        blob.extend(data)
        accessors.append({"bufferView": len(views) - 1, "componentType": 5126,
                          "count": len(values), "type": {1: "SCALAR", 3: "VEC3", 4: "VEC4"}[components]})
        return len(accessors) - 1

    clock = accessor(times, 1)
    rot = accessor(rotations, 4)
    pos = accessor(positions, 3)
    scene = {
        "asset": {"version": "2.0", "generator": "EchoLocal native VRM authored test fixture"},
        "extensionsUsed": ["VRMC_vrm_animation"],
        "extensions": {"VRMC_vrm_animation": {"specVersion": "1.0", "humanoid": {
            "humanBones": {"hips": {"node": 0}, "head": {"node": 1}}}}},
        "nodes": [{"name": "hips", "translation": [0, 1, 0], "children": [1]},
                  {"name": "head", "translation": [0, 0.5, 0]}],
        "scenes": [{"nodes": [0]}], "scene": 0,
        "buffers": [{"byteLength": len(blob)}], "bufferViews": views, "accessors": accessors,
        "animations": [{"name": "Authored head turn and hips bounce", "samplers": [
            {"input": clock, "output": rot, "interpolation": mode},
            {"input": clock, "output": pos, "interpolation": mode}], "channels": [
            {"sampler": 0, "target": {"node": 1, "path": "rotation"}},
            {"sampler": 1, "target": {"node": 0, "path": "translation"}}]}],
    }
    raw = json.dumps(scene, separators=(",", ":")).encode()
    raw += b" " * (-len(raw) % 4)
    blob += b"\0" * (-len(blob) % 4)
    path.write_bytes(struct.pack("<III", 0x46546C67, 2, 28 + len(raw) + len(blob))
                     + struct.pack("<I4s", len(raw), b"JSON") + raw
                     + struct.pack("<I4s", len(blob), b"BIN\0") + blob)


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("output", type=Path)
    args = parser.parse_args()
    args.output.mkdir(parents=True, exist_ok=True)
    for mode in ["LINEAR", "STEP", "CUBICSPLINE"]:
        animation(args.output / (mode.lower() + ".vrma"), mode)
    (args.output / "malformed.vrm").write_bytes(b"not a GLB")
    (args.output / "truncated.wav").write_bytes(b"RIFF")
    valid = (args.output / "linear.vrma").read_bytes()
    json_length = struct.unpack_from("<I", valid, 12)[0]
    doc = json.loads(valid[20:20 + json_length])
    for resource in ["buffer", "image"]:
        test = json.loads(json.dumps(doc))
        if resource == "buffer":
            test["buffers"][0]["uri"] = "https://example.invalid/never-request.bin"
        else:
            test["images"] = [{"uri": "https://example.invalid/never-request.png"}]
        raw = json.dumps(test, separators=(",", ":")).encode()
        raw += b" " * (-len(raw) % 4)
        bin_chunk = valid[20 + json_length:]
        (args.output / ("external-" + resource + ".vrm")).write_bytes(
            struct.pack("<III", 0x46546C67, 2, 20 + len(raw) + len(bin_chunk))
            + struct.pack("<I4s", len(raw), b"JSON") + raw + bin_chunk)
    with wave.open(str(args.output / "reference-envelope.wav"), "wb") as wav:
        wav.setparams((1, 2, 16000, 0, "NONE", "not compressed"))
        pcm = []
        for i in range(16000 * 4):
            t = i / 16000
            level = 0 if t < 0.5 or 1.5 <= t < 2.5 or t >= 3.5 else 0.2
            pcm.append(int(32767 * level * math.sin(2 * math.pi * 220 * t)))
        wav.writeframes(struct.pack("<" + "h" * len(pcm), *pcm))


if __name__ == "__main__":
    main()
