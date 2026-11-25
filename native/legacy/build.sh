#!/bin/sh
set -eu
# Run in the dedicated Android build environment; no host package installation.
root=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
out=${OUT:-$root/out}
ndk=${NDK:-${ANDROID_NDK_HOME:?set NDK or ANDROID_NDK_HOME}}
host=${NDK_HOST:-linux-x86_64}
tools=$ndk/toolchains/llvm/prebuilt/$host/bin
mkdir -p "$out"
out=$(CDPATH= cd -- "$out" && pwd)
archive=$out/ffmpeg-9.1.tar.xz
if [ ! -f "$archive" ]; then
  curl --fail --location https://ffmpeg.org/releases/ffmpeg-9.1.tar.xz -o "$archive.tmp"
  mv "$archive.tmp" "$archive"
fi
printf '%s  %s\n' 1a4d394ae63d190a526cd052eb4415c156b5c5e2df37efab41a7de0b8fde2433 "$archive" | sha256sum -c -
if [ ! -d "$out/ffmpeg-9.1" ]; then tar -xJf "$archive" -C "$out"; fi
src=$out/ffmpeg-9.1
mkdir -p "$out/build" "$out/install"
cd "$out/build"
config_hash=$(sha256sum "$root/build.sh" | cut -d " " -f 1)
if [ ! -f config.h ] || [ "$(cat config.sha256 2>/dev/null || true)" != "$config_hash" ]; then
  "$src/configure" --prefix="$out/install" --target-os=android --arch=arm --cpu=armv7-a \
    --enable-cross-compile --cc="$tools/armv7a-linux-androideabi28-clang" \
    --ar="$tools/llvm-ar" --ranlib="$tools/llvm-ranlib" --strip="$tools/llvm-strip" \
    --disable-autodetect --disable-everything --disable-programs --disable-doc \
    --disable-debug --disable-network --disable-avdevice --disable-avfilter \
    --disable-shared --enable-static --enable-pic --enable-neon \
    --disable-gpl --disable-nonfree --disable-version3 \
    --enable-avcodec --enable-avformat --enable-swscale --enable-swresample \
    --enable-demuxer=asf,avi,mpegps,mpegvideo,mp3,ac3,aac,h264,m4v,flv,mov,matroska \
    --enable-decoder=wmv1,wmv2,wmv3,mpeg4,msmpeg4v2,msmpeg4v3,mpeg1video,mpeg2video,vp6f,svq3,mjpeg,h264,mpegvideo,wmav2,mp3,mp2,pcm_u8,pcm_s16le,ac3,aac,qdm2 \
    --enable-parser=h264,mpeg4video,mpegvideo,mpegaudio,aac,ac3,vc1 \
    --enable-bsf=h264_mp4toannexb \
    --extra-cflags='-O2 -D_GNU_SOURCE -ffunction-sections -fdata-sections' \
    --extra-ldflags='-Wl,--gc-sections'
  printf '%s\n' "$config_hash" > config.sha256
fi
make -j"${JOBS:-2}" > "$out/compile.log" 2>&1
make install > "$out/install.log" 2>&1
"$tools/armv7a-linux-androideabi28-clang" -O2 -Wall -Wextra -Werror -D_GNU_SOURCE \
  -I"$out/install/include" "$root/decoder.c" -L"$out/install/lib" \
  -lavformat -lavcodec -lswresample -lswscale -lavutil -lm -lz -latomic \
  -Wl,--gc-sections -o "$out/echolocal-legacy"
"$tools/llvm-strip" "$out/echolocal-legacy"
cp "$src/COPYING.LGPLv2.1" "$out/FFmpeg.LICENSE"
cp "$out/build/config.h" "$out/FFmpeg.config.h"
cp "$out/build/ffbuild/config.mak" "$out/FFmpeg.config.mak"
printf 'FFmpeg 9.1, LGPL-2.1-or-later, GPL/nonfree/version3 disabled\n' > "$out/NOTICE"
wc -c "$out/echolocal-legacy"

cp "$root/decoder.c" "$out/decoder.c"
cp "$root/build.sh" "$out/build.sh"
cp "$root/NOTICE" "$out/NOTICE"
cp "$root/../../LICENSE" "$out/LICENSE"
cd "$out"
tar -czf legacy-corresponding-source.tar.gz -C "$root/../.." native/legacy/decoder.c native/legacy/build.sh native/legacy/NOTICE LICENSE
