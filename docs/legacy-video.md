# Legacy and current on-device video formats

Athena builds include an optional native FFmpeg fallback behind the existing
Video URL player. Use the same authenticated web API, Home Assistant Video
entity and Node video controls, with the same HTTP(S) and configured SMB sources.
No transcoding service, conversion of user files, format-specific endpoint,
settings, credential export or extra listening port is required.

The primary MP4/H.264/AAC-LC path is retained. In-band H.264/H.265
parameter sets take precedence over stale container configuration; missing sets
are still supplied from the container. Configured SMB sources retain at most
32 aligned 256 KiB source chunks (8 MiB) per ranged reader across seeks. This
bounded, per-playback working set avoids repeated signed SMB setup when a MOV
alternates between distant audio/video chunks; it is not whole-file caching or
transcoding. Cache contents are released on close and are never persisted.

Unsupported containers/codecs
reopen through the existing transport and a separate CGO-independent ARM helper.
H.264 and HEVC in fallback containers are demuxed to Annex-B for the existing
platform decoder; VP9 packets are passed through unchanged. No re-encoding is
performed, and a platform decoder is not necessarily hardware-accelerated.
Other selected video codecs decode locally to bounded
RGBA, scale while preserving display aspect ratio, and use the existing display
surface, fullscreen controls and audio-derived presentation clock. Audio is
locally decoded/downmixed/resampled to 48 kHz stereo through the existing media
volume/mute path. Pause/resume, stop and source-supported seek remain shared.

This iteration is **Athena/API 28 only**. Other board payloads and Cast/YouTube/
MSE receiver paths are unchanged. This does not add DRM support or promise that
every file bearing one of the following extensions will work.

## Compatibility matrix and limits

| Container | Video | Audio |
| --- | --- | --- |
| ASF/WMV | WMV1/2, WMV3 Main, MPEG-4 Part 2 | WMA v1/v2 |
| AVI | MPEG-4 SP/ASP, MSMPEG4 v2/v3, H.264, WMV3; H.263, Cinepak, Indeo 2/3/4/5, DV, HuffYUV/FFVHuff, MS Video 1, MS RLE | MP3/MP2, PCM u8/s16/s24/s32, A-law, MS/IMA ADPCM |
| MPEG Program Stream | MPEG-1/2 Video | MP2, AC-3 |
| FLV | H.264, VP6F, FLV1/Sorenson Spark | AAC, MP3 |
| MOV/MP4/3GP | SVQ1/SVQ3, RPZA, MPEG-4 Part 2, MJPEG, H.263, H.264 | QDM2, AC-3/E-AC-3, AAC-LC/HE-AAC, ALAC, IMA4, big-endian PCM |
| Matroska/WebM | H.264, HEVC Main8, VP9 profile0, VP8, FFV1, MPEG-4 Part 2 | AAC, DTS core, FLAC, Vorbis, Opus, float32/float64 PCM |
| Ogg | Theora | Vorbis |
| MPEG-TS | MPEG-2 Video, H.264, HEVC Main8 | MP2, AAC, E-AC-3 |

This is a family/combination guide, not a Cartesian product of every listed
codec and container. Some audio codecs already existed in the builtin player;
their native fallback combinations are additional coverage, not entirely new
standalone audio features. Codec identifiers absent from the fixture manifest
are not implied to have device validation.

A codec name alone is not a profile/resolution/bitrate guarantee. The generated
and hash-pinned public matrix records actual profiles, dimensions, frame/sample
rates, channels and sizes. The candidate PR records the actual device checks
and resource measurements; consult those instead of inferring universal support.

Admission limits for this first fallback:

- Software decode: at most 1280x720 pixels and approximately 30 fps; output at
  most 854x480, aspect-correct. It can still be too expensive for complex streams.
- Platform H.264/HEVC: at most 1920x1080 pixels and 60 fps; actual hardware profile,
  level, frame reordering and bitrate compatibility still apply.
- Platform VP9: profile 0 / 8-bit SDR, at most 1280x720 pixels and 30 fps.
  Athena selects `OMX.google.vp9.decoder` (software), not a hardware decoder.
  H.264 and HEVC select the MediaTek platform decoders. HEVC Main10 and VP9
  profile 2 are explicitly rejected; no HDR tone mapping is added.
- Decoder process: 512 MiB address-space ceiling, 64 MiB individual FFmpeg
  allocation ceiling, two decode threads; 4 MiB probe/5 s analysis budget and a
  30 s wall-clock startup deadline. Frame/packet and Go/native queues are bounded.
- Audio: 8–192 kHz, at most eight input channels, downmixed to stereo; PCM gaps
  over two seconds fail instead of allocating unbounded silence.
- One usable video and audio track are selected. Extra tracks, subtitles and
  non-media/timecode tracks are ignored, not a reason to reject usable A/V.
  Unsupported selected audio/video tracks or timestamps produce explicit errors.
- A seek requires transport byte ranges and demuxer support. Service restart
  stops playback; no URL or autoplay state is persisted.

Whitelisted diagnostics distinguish unsupported/damaged containers, missing
usable tracks, unsupported profiles, resource limits, missing timestamps,
source reads, startup timeouts, helper availability/protocol errors and decoding
failures. Status contains a safe diagnostic code, never the URL, file path,
server, account or raw FFmpeg output. VIDEO_RESOURCE_LIMIT means the stated
dimension/frame-rate admission limit, not proof of an out-of-memory condition.
A new load remains available after an error. Stop cancels/reaps the helper.

## Library choice, security and maintenance

FFmpeg 9.1 was selected after evaluating platform MediaCodec and GStreamer.
MediaCodec remains the preferred H.264 renderer but does not provide the missing
legacy container/codec coverage. GStreamer supports Android/ARM and could use
libav plugins, but its pipeline/plugin/Android integration adds a larger surface
for this small non-Java daemon. A bounded custom-AVIO FFmpeg executable provides
the needed demuxers, decoders and converters without changing the CGO-disabled
Go build or depending on Android framework/JNI playback.

The helper merges selected compressed-packet stream heads by DTS, with at most 512 pending packets / 8 MiB. This bounded lookahead prevents video bursts in MPEG-PS from blocking earlier audio behind a full raw-frame queue. DECODER_MEMORY also reports this compressed-lookahead limit. Implicit SBR is checked in an isolated initial AAC frame probe before selecting the fallback; later unsupported-profile changes fail explicitly.

The minimal build disables auto-discovered external libraries, programs, filters,
file/network protocols, GPL, version3 and nonfree components. Selected raw
demuxer probes are also necessary to identify elementary tracks inside MPEG-PS;
omitting them can produce an apparently unknown video codec. FFmpeg never gets
the source URL or credentials and cannot open referenced files or network URLs.
Existing Go range transport remains the only byte source. The display adapter
copies RGBA to an ANativeWindow; it does not encode the software frames again.
The stripped helper is approximately 4 MB; exact candidate size is in its PR.

FFmpeg contains complex parsers and must receive security maintenance. The source
version and SHA256 are fixed in native/legacy/build.sh. The initial 9.1 archive
was verified with the official release signing key
FCF986EA15E6E293A5644F10B4322F04D67658D8. To update: verify the new official
release signature, change version/hash together, rebuild the minimal
configuration, rerun the full matrix and admission/lifecycle tests, and repeat
the actual Athena output/resource/control checks. Do not silently use a system
FFmpeg, floating source branch, external binary download or broader codec build.

The helper source is AGPL-3.0-or-later and statically links unmodified FFmpeg
under LGPL-2.1-or-later (no optional GPL/nonfree/version3 code). Its source,
rebuild script and notices are available, and all binaries must be accompanied
on the same release download site by:

- ffmpeg-9.1.tar.xz (exact FFmpeg corresponding source)
- legacy-corresponding-source.tar.gz (helper source, build recipe and notices)

The release configuration uploads both archives. The app also installs the
helper's AGPL/LGPL licenses and attribution alongside it. A distributor must
retain these files and source/rebuild access; patent obligations are separate
from copyright licensing.

## Reproduce the fixtures and checks

On a development machine with FFmpeg/ffprobe and Python:

```sh
python3 native/legacy/fixtures.py fixtures --seconds 8 --public
```

The default synthetic black/white flash and tone-pulse fixtures are original,
redistributable test content. Add --motion for a more complex moving source.
The optional public sources are hash-pinned FFmpeg/FATE reference downloads;
their short excerpts are remuxed solely to construct codec combinations.
No public reference binaries are committed/rehosted or assumed freely licensed
for redistribution. Source URLs, expected hashes and transformations are in the
script and output manifest. Do not point this test at a personal media library.

Run libcountertop's opt-in TestNativeFixtureMatrix, TestNativeFixtureRouting,
TestNativeSeekAndStop and TestNativeBoundaries with the built helper. The tests
can be cross-compiled as a CGO-disabled ARM binary and run on Android. Synthetic
tests compare decoded pulse alignment, and bounds tests include oversized/
over-rate input and MP4 with a non-media timecode track. They are distinct from
actual display/speaker playback and do not establish subjective acoustic sync.

The candidate PR lists observed real-device frame/clock/control/resource results,
and any unverified physical listening or hardware-limit boundaries explicitly.

Refs #79.


## Additional-codec regression inputs

`python3 native/legacy/extended_fixtures.py extended --seconds 4 --public`
generates the extended synthetic matrix and hash-verifies five additional
public references (VC-1 Advanced and Indeo 2/3/4/5). Do not commit or rehost
public reference media; their download availability is not a redistribution
license. The generated `manifest.json` records actual codec/profile/pixel/sample
formats and hashes. Static flashes intentionally exercise Theora empty-packet
repeats; timing assertions must not be weakened to hide silence or missing
frames. Theora seek retains only the last bounded converted preroll picture.

The expanded opt-in native tests also cover platform-profile rejection and
Theora/HEVC/VP9 seek/stop/reaping. Decoding/routing is separate from actual
platform presentation, steady pacing, pause/resume and acoustic acceptance.
DV/interlaced material is not deinterlaced, and fixture success is not proof of
universal resolution/bitrate/profile support. AV1 is unavailable on this
platform and no external AV1 decoder is bundled. RealMedia was evaluated but
not enabled in this extension because the evaluated paths require additional
timestamp handling.

QuickTime IMA4 startup is covered by TestNativeSmallAudioStartup: the helper groups tiny PCM frames into 20ms records using a fixed 3840-byte buffer. Timestamp discontinuities flush partial records rather than being concealed; EOF retains the final partial block. This prevents packet-count backpressure from starving asynchronous first-presentation reporting.
