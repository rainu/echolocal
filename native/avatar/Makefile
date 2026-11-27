# Athena native avatar helper. A separate AGPL-3.0 executable, not linked into Go.
NDK ?= $(ANDROID_NDK_HOME)
NDK_HOST ?= linux-x86_64
TOOLCHAIN := $(NDK)/toolchains/llvm/prebuilt/$(NDK_HOST)
RESOURCE := $(firstword $(wildcard $(TOOLCHAIN)/lib/clang/*))
AVATAR_CC ?= $(TOOLCHAIN)/bin/clang
AVATAR_CXX ?= $(TOOLCHAIN)/bin/clang++
HOST_CXX ?= c++
OUT ?= out
FFMPEG ?= ../legacy/out/install
TARGET := --target=armv7a-linux-androideabi28 --sysroot=$(TOOLCHAIN)/sysroot
WARN := -O2 -Wall -Wextra -Werror
CXXFLAGS := $(TARGET) -resource-dir $(RESOURCE) -fuse-ld=lld --rtlib=compiler-rt --unwindlib=libunwind -L$(RESOURCE)/lib/linux/arm -stdlib=libc++ -isystem $(TOOLCHAIN)/sysroot/usr/include/c++/v1 -L$(TOOLCHAIN)/sysroot/usr/lib/arm-linux-androideabi/28 -L$(TOOLCHAIN)/sysroot/usr/lib/arm-linux-androideabi -std=c++17 -pthread $(WARN) -Wno-unused-function -static-libstdc++
.PHONY: all vendor-check host-test
all: $(OUT)/echolocal-avatar
vendor-check:
	sha256sum -c vendor/SHA256SUMS
$(OUT):
	mkdir -p "$@"
$(OUT)/athena-window.o: athena-window.c | $(OUT)
	$(AVATAR_CC) $(TARGET) $(WARN) -c $< -o $@
$(OUT)/echolocal-avatar: renderer.cpp video.hpp layers.hpp pose.hpp control.hpp turntable.hpp viewport.hpp $(OUT)/athena-window.o $(wildcard vendor/*.h) vendor/SHA256SUMS | vendor-check
	$(AVATAR_CXX) $(CXXFLAGS) renderer.cpp $(OUT)/athena-window.o -I$(FFMPEG)/include -L$(FFMPEG)/lib -lavformat -lavcodec -lswresample -lswscale -lavutil -lm -lz -latomic -Wl,--gc-sections -lEGL -lGLESv3 -landroid -ldl -o $@
	$(TOOLCHAIN)/bin/llvm-strip $@
host-test: | $(OUT)
	$(HOST_CXX) -O2 -std=c++17 -Wall -Wextra -Werror control-test.cpp -o $(OUT)/control-test
	$(OUT)/control-test

	$(HOST_CXX) -O2 -std=c++17 -Wall -Wextra -Werror layers-test.cpp -o $(OUT)/layers-test
	$(OUT)/layers-test

$(OUT)/video-test: video-test.cpp video.hpp | $(OUT)
	$(AVATAR_CXX) $(CXXFLAGS) video-test.cpp -I$(FFMPEG)/include -L$(FFMPEG)/lib -lavformat -lavcodec -lswresample -lswscale -lavutil -lm -lz -latomic -Wl,--gc-sections -o $@
