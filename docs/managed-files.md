# Owner file browser and managed assets

**Files → Device filesystem** shows the actual device filesystem, not a virtual
asset tree. Start at `/`, open directories or enter an absolute path, download
regular files (including system/configuration files), and select the upload
destination directory and filename. Normal symlinks resolve to real targets;
the returned path and breadcrumbs show the canonical directory.

File sizes use appropriate B/KiB/MiB/GiB units; small scripts are not rounded
to zero MiB.

**Managed assets** retains the bounded asset/consumer workflow below. It is not
the boundary of the normal filesystem browser.

## Authentication and authority

Both APIs use the existing owner-authenticated HTTPS Pages API: normal owner
login with secure SameSite=Strict cookie, an existing bearer session, or HTTP
Basic with the frontend owner password. Never send credentials or file contents
over plain HTTP. No new anonymous, peer or Node/agent API, grants or settings.

Following the owner's explicit request, filesystem transfers have the daemon's
OS-level regular-file access, **including sensitive system and application
configuration**. Successful owner downloads return actual bytes without redaction
or a managed-directory restriction. Keep owner credentials private.

Cross-origin uploads are rejected. Responses are no-store/nosniff. Downloads are
attachment-only application/octet-stream with sandbox CSP, not HTML previews.
The browser downloads directly without a whole-file JavaScript blob.

## Real filesystem API

| Request | Result |
| --- | --- |
| `GET /api/v1/filesystem?path=%2F&offset=0` | Real directory metadata and target-filesystem capacity |
| `GET /api/v1/filesystem/file?path=%2Fsystem%2Fexample.conf` | Stream the requested regular file |
| `HEAD /api/v1/filesystem/file?path=...` | Authenticated download headers without a body |
| `PUT /api/v1/filesystem/file?path=%2Fchosen%2Fdirectory%2Fname.bin` | Stream bytes into that exact destination, create-only |

URL-encode the absolute path as a query parameter once. Format is not a permission
rule: pictures, models, binaries and configuration files are regular files.
Downloads have **no upload-size cap**, support byte ranges and stream
incrementally. Devices, FIFOs, sockets and directories are not downloadable
files. Kernel pseudo-files with nonstandard seek/size semantics are not
guaranteed to behave like ordinary stored files.

Illustrative listing, not a fabricated device inventory:

```json
{"path":"/","parent":"","entries":[
  {"name":"system","kind":"directory","size":4096,
   "modified":"2026-01-01T00:00:00Z","mode":"drwxr-xr-x"}
],"availableBytes":536870912,"maxUploadBytes":134217728,
"readOnly":false,"capacityKnown":true}
```

`parent` is empty at root. Entries are directory, file, symlink or special.
Resolvable file/directory symlinks also report `targetKind` and `targetPath`.
Broken/inaccessible links remain visible without an action.

Resolved paths are reopened component-by-component using pinned directory
descriptors and no-follow flags; a symlink swap after resolution cannot redirect
the open descriptor. Downloads open nonblocking and verify a regular file before
serving. Listings read the filesystem every time; no recursive scan, content
index or separately maintained inventory.

Pages have at most 256 entries. The complete bounded directory is sorted before
paging: directories (including symlinks to directories) first, then other entries,
each group alphabetically sorted case-insensitively with an exact-name tie-break.
Lightweight names/types are retained for sorting; full metadata is loaded only
for the current page. `nextOffset` advertises the
next page. Refresh from page one after external changes. Paging is bounded to
65,536 entries/directory; `limitReached:true` reports that bound.

### Upload destination and behavior

Navigate to a directory; **Upload destination directory** defaults to it. Keep
that path or edit it explicitly, select a local file and optionally edit
**Destination filename**. The destination is visible before transfer. On success
the browser opens the destination directory and shows the new file.

- Any **existing writable directory** may be chosen, not only the asset area.
- Create-only: an existing name of any type returns 409. Choose another name.
  No arbitrary delete, rename, directory creation or permission editor is added.
- **No automatic remount**. Read-only system partitions return 403, as do
  OS/SELinux permission denials. Readable files remain downloadable.
- Maximum upload **128 MiB**, known Content-Length, five-minute deadline and one
  filesystem upload at a time. A second upload returns 423.
- No managed-area 512 MiB/256-file quota on this filesystem endpoint. Capacity
  uses the **chosen filesystem**, retaining 128 MiB total free space without
  spending OS-reserved blocks. It is a snapshot, not a reservation against
  external writers; reserve is rechecked after staging.
- A 64 KiB buffer streams to a private random temporary file (0600) in that
  directory. Complete/synced bytes are atomically published by rename-no-replace,
  with no-replace hard-link fallback on older kernels. Filesystems supporting
  neither return an actionable 409; no unsafe overwrite/partial-file fallback.
- Failed, disconnected or cancelled uploads remove their staging file. A process
  or power failure may leave a private staging dotfile in the chosen directory;
  arbitrary directories are not recursively scanned or automatically cleaned.
  If a response is lost, a complete final file may already exist: refresh first.
- Direct owner filesystem writes bypass the managed consumer's filename/quota
  rules. Changes in application-owned directories can invalidate their
  assumptions, just as with other filesystem tools.

Success is 201 entry metadata. Failure JSON contains a bounded `error` code:

| HTTP | Code |
| --- | --- |
| 400 | invalid_path, invalid_offset, incomplete_upload |
| 401 | Existing owner authentication failure |
| 403 | permission_denied, read_only_filesystem, cross_origin |
| 404 | not_found |
| 405 | method_not_allowed |
| 409 | already_exists, not_directory, unsupported_file, atomic_publication_unsupported |
| 411 | length_required |
| 413 | file_too_large |
| 423 | transfer_busy |
| 500 | listing_failed (unclassified I/O failure; refresh before retry) |
| 507 | storage_full |

Paths must be absolute UTF-8 without dot segments, controls, backslashes or NUL.
Unicode names, spaces and leading dots are supported; OS filename limits apply.
Errors do not echo raw OS messages or credentials.

After authentication, use a trusted HTTPS origin:

```sh
curl --fail-with-body --user owner --get \
  --data-urlencode 'path=/system/example.conf' \
  --output example.conf "$DEVICE_URL/api/v1/filesystem/file"
curl --fail-with-body --user owner --upload-file asset.bin \
  "$DEVICE_URL/api/v1/filesystem/file?path=%2Fchosen%2Fdirectory%2Fasset.bin"
```

## Optional managed asset API

The **Managed assets** tab and `/api/v1/files` share the real persistent managed
directory `layout.StateDir + "/user-files"` (0700, files 0600). The normal browser's
**Show upload directory** shortcut opens that physical directory, not a virtual
filesystem.

| Request | Success |
| --- | --- |
| `GET /api/v1/files` | Inventory/capacity including actual `directory` path |
| `PUT /api/v1/files/{name}` | 201, streamed create-only file |
| `GET /api/v1/files/{name}` | Attachment download with range support |
| `DELETE /api/v1/files/{name}` | 204; asset UI confirms deletion |

The managed API and WebDAV share 128 MiB/file, 512 MiB total and 256 files/folders, with a
128 MiB free-space reserve. Capacity is min(quota remaining, OS-available bytes,
total free bytes minus reserve); OS-reserved blocks are not subtracted twice.
One managed upload at a time, with a five-minute deadline.

Managed relative paths support UTF-8 names (255 bytes/component, 1024 bytes/path,
16 components). Reserved `.upload-*`/`.dav-*` names and control characters are
rejected. The API remains create-only; the [WebDAV adapter](managed-webdav.md)
adds folders, replacement, rename, properties and locking in the same store.
Traversal, symlinks and special files are rejected in this **managed** API, unlike
normal symlink navigation in the separate filesystem browser. Complete files are
synced and atomically published; failed transfers are removed and managed crash
leftovers are cleaned on store opening. Files survive daemon restart; rollback
to older software leaves them intact.

Metadata: `{"name":"sample.bin","size":3,"modified":"2026-01-01T00:00:00Z"}`.
Inventory has `directory`, `files`, `usedBytes`, `availableBytes`, `budgetBytes`,
`maxFileBytes` and `maxFiles` and `entries` (files plus folders). Managed errors additionally include invalid_name
(400) and unsafe_entry (409). Refresh after uncertain outcomes. The managed quota
is not a device-wide filesystem sandbox.

## Consumer contract

Consumers discover with `files.Get().List()`, select by relative managed path and
stream through `Open(name)`, closing the descriptor. Inject the same process
store, never concatenate untrusted names or accept request-selected managed
roots. Validate formats and atomically import into feature-owned storage there.
Deleting a transferred file must not break an already imported copy.

Uploading pictures does not add a photo provider, uploading VRM/VRMA does not
import an avatar, and tones retain their existing validated workflow. Neither API
adds a renderer, avatar engine, tone importer, network-drive protocol or agent
file command.

## Optional standard-protocol adapter

See [managed WebDAV subset](managed-webdav.md) for the off-by-default protocol,
configuration, client restrictions and interoperability verification status.


## WebDAV device-tree parity (owner clarification, issue #39)

WebDAV now exports the same actual device filesystem as the Device filesystem tab, starting at `/`, not the managed asset inventory. Both share the device-file transfer backend and target-filesystem limits; the managed asset API retains its own quotas. See [device WebDAV](managed-webdav.md) for capabilities, URL/settings display and scope.
