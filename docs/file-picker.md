# On-device asset picker

Refs #76.

## Reuse evaluation

The pinned go-gui v0.94.0 already includes InWindowOpenDialog and
InWindowFolderDialog. Its built-in browser is desktop-sized (480 UI pixels wide,
220-pixel list), includes editable filesystem paths, and has no public filesystem
policy/root hook. Native Linux dialogs also depend on desktop dialog programs
which the embedded display does not provide. Reusing either as-is would expose
the root application's private filesystem and would not match the touch menus.

The implementation therefore reuses libcountertop's existing Page, Row,
PageFrame, touch interaction, wrapping text, scrolling list and pinned ActionBar.
No new GUI dependency or library fork is required. The common browser model is
in internal/filepicker; internal/ui/filepicker supplies the shared touch UI.

## Interaction and limits

- Choose a user-file location, tap folders to navigate, and use Parent folder to
  go up. At a location's root, Parent returns to the location chooser.
- Tap a supported file to mark it, then Select to apply. Selecting a folder
  navigates into it. Directory mode selects the current folder on Select.
- Cancel and the screen's back arrow return to the originating screen without
  saving. No selection is applied by tapping a row alone.
- Current location and selected path wrap instead of using the trailing value
  column. The selection remains visible when changing entry pages.
- Extensions are case-insensitive. Unsupported files, hidden names, symlinks and
  special files are consistently hidden. Directories remain visible.
- Directories are read only on open/navigation/refresh, never on every frame or
  during settings search. A bounded snapshot is sorted globally folder-first,
  then case-insensitively by name. Each scrollable page holds 64 entries;
  Previous/Next handles larger directories. More than 65,536 raw entries produces
  an explicit error rather than an arbitrary partial ordering.
- A missing stored path falls back to its nearest accessible ancestor inside a
  permitted location, with a notice and no replacement selection. An inaccessible
  location leaves the prior usable location/chooser intact. Empty directories
  display an explicit empty state.
- Select rechecks existence/type/readability. Caller validation/save errors keep
  the picker open and preserve the prior setting. Refresh reloads external edits.

## Access boundary

The Avatar callers expose only managed user files, local user/test files, and
shared media storage. They do not expose the application's parent state
directory, credentials, proc/sys/device nodes, a path-entry escape hatch, or
copy/delete/upload operations. An existing selection outside those locations
shows an unavailable notice; it is not silently cleared or made browsable.

Every directory component is opened with O_NOFOLLOW and pinned descriptors.
Final files are pinned with O_PATH, checked as regular before reopening for read,
and rejected if replaced by a symlink or special file. This reuses devicefs's
existing safe-open primitives. Filesystem permissions remain enforced by the OS;
there is no remount, chmod or permission escalation. Like any path-based setting,
a file can subsequently disappear or be changed externally; the existing asset
loader/validation remains authoritative.

## Touch-UI selector audit

| Surface | Result |
| --- | --- |
| Avatar Lab: VRM model | Shared picker; .vrm; existing path validation and model load behavior retained; default-model reset retained |
| Avatar Lab: background image | Shared picker; .png/.jpg/.jpeg; existing bounded image decoder validation and save retained; clear/reset retained |
| Fullscreen Talk: background image | Same image picker; same registry validation/save; per-image clear preserves independent framing; composition reset retained |
| Avatar renderer/preview/Talk | Consume the above persisted settings; no independent path input |
| Other native settings, including shared libcountertop settings | No additional local file/directory path input found |
| SMB server/share/folder credentials | Remote SMB location, not a local directory; intentionally unchanged |
| Photos/poster/radio/camera/YouTube and server connections | URL/entity/catalogue inputs, not local file selectors; intentionally unchanged |
| Tone/model catalogues and managed-file owner web UI | Catalogue selection / authenticated transfer interfaces, not on-device local path entry; intentionally unchanged |
| CLI/API/Node settings | Existing non-touch interfaces/validation retained; no web picker or new Node authority added |

Directory mode is implemented and tested as part of the reusable component; no
current on-device local-directory setting requires a caller.

## Verification

Automated coverage distinguishes filesystem/model checks, shared-widget layout
and simulated input, registry regressions, and actual installed-device checks.
Actual hardware results and the exact tested commit are recorded in the PR.
Physical finger input is not implied by injected device input.
