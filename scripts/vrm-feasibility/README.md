# VRM rendering feasibility probes

Related: [EchoLocal #34](https://github.com/rainu/echolocal/issues/34).
Results and limitations: [measurement report](../../docs/vrm-feasibility/report.md).

These probes **do not load a VRM** or implement an avatar renderer. They test
prerequisites without replacing the UI, opening a Talk session, using microphone
input, changing browser flags, or restarting any service.

## Browser probe

Requires the existing EchoLocal Cast engine to be running, ADB access to the
test device, and the repository's Go toolchain/dependencies. Do not start or
reconfigure the Cast engine just to hide an unavailable prerequisite.

Set `ADB` to an ADB executable/wrapper and `SERIAL` to your test device locally.
Do not commit either value. First check existing forwards and ensure port 19334
is unused; do not replace someone else's forward.

```sh
"$ADB" forward --list
"$ADB" -s "$SERIAL" forward --no-rebind tcp:19334 tcp:9222
go run ./scripts/vrm-feasibility/cdp-probe.go > cdp-capabilities.json
"$ADB" -s "$SERIAL" forward --remove tcp:19334
```

Only use loopback. CDP has browser-wide authority and no authentication.
The program creates its own blank window, attaches only to that target, checks
Canvas2D/AudioContext/WebGL contexts, samples three seconds of **empty-page**
requestAnimationFrame callbacks, then closes the target. It reads browser GPU
feature status but not existing page contents, URLs, audio or credentials.
It never closes an existing page or the browser.

On an interrupted run, inspect the local CDP target list and close only the
new blank target you created. Do not close the Cast receiver page. A timeout,
missing engine, failed context or cleanup failure is not a successful VRM test.

## Native offscreen probe (Athena, Android API 28 / ARMv7)

Use an existing Android NDK compiler. `NDK_CC` is the API-28 ARMv7 clang executable,
not the system compiler. No package installation or Android framework startup
is required.

```sh
"$NDK_CC" -O2 -Wall -Wextra -Werror \
  scripts/vrm-feasibility/native/egl-probe.c -lEGL -lGLESv3 -o egl-probe-arm
"$ADB" -s "$SERIAL" push egl-probe-arm /data/local/tmp/issue34-egl-probe
"$ADB" -s "$SERIAL" shell chmod 700 /data/local/tmp/issue34-egl-probe
"$ADB" -s "$SERIAL" shell /data/local/tmp/issue34-egl-probe > egl-capabilities.txt
```

This creates a 64x64 offscreen pbuffer, verifies one RGBA pixel, performs 120
clear/glFinish operations, and destroys its context/surface. The duration is a
driver sanity check, **not** avatar FPS, a frame-time distribution, or a sustained
GPU benchmark. It opens no display layer and plays no audio.

Retain the host binary and its SHA-256. Verify the device copy matches, then
remove only that temporary probe file (never broad device cleanup). Check that
the existing clock/idle view is still topmost, the daemon is running and Talk is
inactive as before. Do not initiate a new Talk connection for this test.
