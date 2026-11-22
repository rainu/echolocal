# Device filesystem over WebDAV (issue #39)

## Owner clarification

The endpoint must expose **the same actual device filesystem as the web file
browser**, starting at `/`. The former managed-only adapter and empty managed
root were not the intended scope. No user files are moved or migrated: the
managed asset directory remains at its existing physical location and is now
reachable as part of the real tree.

## Connection and controls

- HTTPS URL: `https://<device>/api/v1/webdav/` (existing HTTPS port).
- KDE/Dolphin: `webdavs://owner@<device>:443/api/v1/webdav/`.
- Username `owner`; existing EchoLocal admin password. Passwords are never put
  into the displayed/copied URL, connection-info response or examples.
- Default-off `access/webdav` uses the existing native/web/owner API and Node
  settings grant. Device Web access and web Network settings show the complete
  connection address **only while enabled**. The web view also provides copying,
  folder path and login guidance.
- Owner-authenticated `GET /api/v1/webdav-info` reports enabled state, URL, path,
  root and username. Its native address comes from trusted device address state,
  not a request Host header. It is informational, not a new writable setting.
- Disabling denies new DAV requests and cancels running transfers; ordinary web
  filesystem access stays independent. No new listener, anonymous share, shell,
  remount or change to Node grants is introduced.

## Shared filesystem backend

The web filesystem API and DAV share one device-filesystem service, safe pinned
directory/file opens, upload implementation, free-space policy and mutation
admission. Normal symlinks resolve as in the browser. Locks use canonical paths,
so an alias cannot bypass a lock through the browser upload route or DAV.
Metadata for special entries may be listed, but devices, pipes and sockets are
never opened as ordinary downloads or upload destinations. A pinned O_PATH
descriptor is inspected before opening a regular file for reading.

Web uploads remain create-only. DAV adds standard explicit replacement,
collections, COPY/MOVE, deletion and properties for OS-writable resources. The
device filesystem's permissions and read-only mounts are respected; no remount,
permission editor, executable content handler or arbitrary root selector exists.
Deleting/moving/replacing the exported root itself is prohibited. Mutation of a
symlink entry is rejected instead of unexpectedly deleting its target; browsing
and regular-file downloads through normal symlinks work.

There is **no managed-area 512 MiB/256-entry quota on the device filesystem**.
Uploads/COPY file contents share the browser's 128 MiB per-file and 128 MiB
physical free-space reserve on the selected target filesystem. Downloads do not
have that upload-size bound. One admitted mutation and a five-minute request
read deadline prevent competing or stalled writes from monopolizing resources.

## RFC 4918 classes 1 and 2

OPTIONS advertises `DAV: 1, 2`. Supported operations are PROPFIND, PROPPATCH,
MKCOL, GET/HEAD/ranges, PUT, DELETE, COPY/MOVE, LOCK/refresh/UNLOCK. Shared and
exclusive locks, full tagged/untagged If list evaluation, Not and ETag conditions,
HTTP write preconditions, default Overwrite semantics and lockdiscovery reuse the
tested protocol engine. Equivalent HTTPS authorities with default port 443 are
accepted without enabling cross-server COPY/MOVE.

PROPFIND supports Depth 0/1. Unbounded recursion is rejected with the standard
`propfind-finite-depth` error: blindly crawling `/proc`, mounts and symlink cycles
is not appropriate for a device-root export. A directory listing is bounded to
65,536 entries; oversized responses fail rather than silently pretending the
folder is empty. Control XML is bounded to 64 KiB and property/listing responses
to 4 MiB. Collection COPY is bounded and rejects repeated directory inodes.
Unrelated DAV ACL/versioning/calendar/synchronization extensions are not claimed.

Dead properties persist in application-owned metadata, not arbitrary xattrs or
sidecars in every device directory; the total is bounded to 1 MiB. Locks are
volatile across process restart, bounded to one hour and 512 active locks;
clients must reacquire them after restart.

## Publication and recovery limits

Uploads stage in the destination filesystem, sync complete bytes and publish
atomically. A failed/cancelled PUT never truncates its old destination. The API
keeps create-only publication; DAV replacement is explicit and checks the old
destination's admission identity. Optimized io.Copy paths cannot bypass byte
accounting. Regular-file replacement rollback requires safe hard-link support;
unsupported atomic publication fails rather than using an unsafe fallback.

Namespace writes isolate in-process readers and journal affected destinations
for request-error rollback. COPY/MOVE failure preserves existing destinations;
cross-filesystem MOVE is not converted into a destructive copy/delete fallback.
There is **no whole-device snapshot** or recursive device-root copy. Process/power
failure can leave hidden staging/recovery entries in affected directories;
ordinary protocol requests cannot address them. Do not recursively clean the
whole device or describe this as a full filesystem backup/ACID power-loss journal.
External local writers/admin filesystem changes are outside this in-process
coordination and may invalidate application-owned state, as with any root file
manager. Keep test cleanup restricted to exact test-owned paths.

## Verification

Keep receipts tied to the exact app/library/build commits. Include fixture
browser/DAV parity, symlinks/special-file safety, alias locks, byte identity,
replacement/COPY/MOVE/properties, abort/disable behavior, read-only mounts and
conditional address rendering/copying. Run mutating conformance clients
**sequentially** against a fresh isolated fixture, not against the live device
root and not in parallel against one store.

Linux litmus and davtest evidence is separate from actual KDE/Windows client
evidence. The old davtest source may need
`PERL_NET_HTTPS_SSL_SOCKET_CLASS=IO::Socket::SSL`; expected execution-probe
failures must not be mistaken for missing DAV support or actual script execution.
The device's existing certificate/trust configuration is unchanged. A public-key
pin test does not establish default CA/hostname trust or Windows compatibility.

Protocol decision remains WebDAV over existing owner HTTPS. SMB would introduce
a separate session/authentication/signing/listener stack (the investigated
Go SMB client is not a server; its investigated server fork is AGPL). SFTP adds
SSH transport/host-key lifecycle without native Explorer support. The existing
BSD-licensed Go WebDAV dependency is the smaller fit; this is not a NAS stack.

References: [RFC 4918](https://www.rfc-editor.org/rfc/rfc4918),
[Go WebDAV](https://pkg.go.dev/golang.org/x/net/webdav).
Both implementation PRs remain unmerged until owner acceptance.
