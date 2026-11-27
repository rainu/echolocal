# On-device Files app

Refs #84; builds on the accepted picker in #76.

## Shared browser and security

The Files app and settings picker use `internal/filepicker` for the trusted
user-location list, path policy, directory enumeration, parent navigation,
metadata, ordering and pinned regular-file opens. The two presentations are
separate: opening a file never confirms or changes an avatar/picker setting.

The picker still hides dot names, filters extensions, and requires explicit
Select or Cancel. Files shows regular files, supports an optional hidden-name
toggle, and opens supported media on tap. Symlinks and special files remain
excluded in both modes. Known credential names/directories, private Android app
storage and key files remain excluded even when hidden names are enabled.
These exclusions are defense in depth, not a content-based secret classifier:
only put intentionally browsable user files in the permitted user locations.
No application-state parent, arbitrary path input, new remote API, listener,
write operation, permission change or external file handler is added.

Every path component is opened without following symlinks. Metadata uses the
pinned directory descriptor; viewers retain a pinned regular-file descriptor.
The local player receives bytes through a single-file, in-process HTTP
transport with no socket or network fallback. Embedded external references and
playlists cannot obtain network or arbitrary filesystem access.

## Controls and persistence

Files is registered in Apps and directly in the right sidebar. Its fixed
toolbar offers Up, Refresh, list/grid, tile size, sort key, direction, hidden
visibility and Close. Tap the view/size/sort controls to cycle their labelled
states. Tile widths are 128/192/256 UI pixels. Sort by name, modified time, size
or extension/type in either direction; folders remain first and names break
ties deterministically. These five display preferences are stored in the
`files` configuration section; missing/invalid legacy values have safe defaults.

The location remains visible above an independently scrollable list/grid.
Large directories are sorted as one bounded snapshot and paged 64 at a time;
more than 65,536 raw entries produces an explicit unavailable/too-large state.
Navigation and metadata collection are asynchronous, with obsolete scans
cancelled. Empty/unavailable folders, unsupported media and failed saves have
explicit states. Failed navigation retains the previous usable folder.
Turning hidden visibility off inside a hidden folder returns to locations.

## Preview and player inventory

Existing reusable capabilities were inventoried before implementation:

- go-gui provides image rendering and a bounded memory-image registry;
- the existing URL player handles supported MP3/PCM/FLAC/AAC and MP4/WebM/Matroska
  streams, pause/stop/seek and playback errors;
- existing playout/video pages provide the native on-device video surface,
  media ownership and back/close controls;
- the avatar helper already has a local-only, resource-limited MP4/H.264
  first-frame decoder. A new thumbnail-only invocation returns that first frame
  without creating an avatar surface or decoding/playing audio.

Files reuses these capabilities, not Android intent handlers. Image formats:
PNG, JPEG, GIF and WebP (animated images show their first frame). Images are
read into a bounded immutable encoded buffer (16 MiB), validated before decode
(maximum 16 megapixels), and fitted without aspect distortion. Viewer output
is at most 1280×800; thumbnails fit inside 192×192.

One process-wide decode gate serializes image/thumbnail work. A folder/page
owns one worker, at most 64 queued/positive/negative preview results (under
9 MiB decoded thumbnail pixels), and no disk thumbnail cache. Navigation,
paging, visibility changes and explicit refresh retire the old cache and GUI
image keys. Entries are keyed by path, size and modification time, with a
fresh presentation generation on refresh; no duplicate full-size media is
stored. Failed/unavailable previews retain stable file-type icons.
The shared GUI registry also has its own 32 MiB budget.

Video thumbnails are currently limited to the existing helper's MP4/H.264
profile: at most 64 MiB, 1280×800, 30 fps and two minutes, with its existing
timestamp/pixel-aspect/rotation validation. Other containers/codecs can still
play through the normal player when supported, but show a video icon.
Unavailable/missing decoders or damaged media produce a clear playback error,
not an assumption that a system handler exists.

## Verification boundary

Unit tests cover shared policy and sorting, pinned-file/range transport,
bounded image decoding/cache, persistence and picker regressions.
DE/EN layout/input tests cover list/grid, long names, fixed controls, scrolling,
paging and preference changes at several panel sizes. The PR separately
records exact-commit native builds and actual device verification. Simulated
input is not physical-finger or subjective listening verification.
