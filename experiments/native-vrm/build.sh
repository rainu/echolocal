#!/bin/sh
set -eu
cd "$(dirname "$0")"
: "${NDK_BIN:?set NDK_BIN to the existing NDK llvm/prebuilt/<host>/bin directory}"
OUT=${OUT:-out}
mkdir -p "$OUT"
sha256sum -c vendor/SHA256SUMS
"$NDK_BIN/armv7a-linux-androideabi28-clang" -O2 -Wall -Wextra -Werror \
  -c athena-window.c -o "$OUT/athena-window.o"
"$NDK_BIN/armv7a-linux-androideabi28-clang++" -std=c++17 -O2 \
  -Wall -Wextra -Werror -Wno-unused-function -static-libstdc++ \
  renderer.cpp "$OUT/athena-window.o" -lEGL -lGLESv3 -landroid -ldl \
  -o "$OUT/native-vrm-arm"
sha256sum "$OUT/native-vrm-arm"
