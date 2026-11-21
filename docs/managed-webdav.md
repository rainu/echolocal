# Decision: optional managed-file WebDAV subset (issue #39)

Status: implementation candidate; Windows interoperability **not verified**.
No cross-platform network-drive support claim. Owner acceptance and Windows
client evidence remain prerequisites for completing #39.

## Decision and evidence

Use a small **WebDAV transfer subset over existing owner HTTPS**, not SMB or SFTP.
The target is a constrained ARMv7 Echo with Android 9/API28, not a NAS. The shared
store is flat and create-only; a general filesystem server needs substantial
restrictions to preserve this boundary.

| Candidate | Go implementation / evidence checked 2026-11-21 | Target/client trade-off |
| --- | --- | --- |
| WebDAV | [golang.org/x/net/webdav](https://pkg.go.dev/golang.org/x/net/webdav), BSD-3-Clause, part of maintained Go net repository (activity 2026-11-19) | Reuses HTTPS, TLS certificate, owner authentication and streaming. Windows has WebClient; Linux has curl/davfs/GVfs. Actual compatibility depends on method, authentication and certificate expectations. |
| SMB2/3 | [hirochachacha/go-smb2](https://github.com/hirochachacha/go-smb2) is BSD-2-Clause but **client-only**. [macos-fuse-t/go-smb2](https://github.com/macos-fuse-t/go-smb2) is an AGPL-3.0 server fork, repository activity 2026-09-21 | Best native Windows network-drive fit. Requires a new TCP service, SMB session/NTLM and signing/encryption decisions, open-file/locking state and a larger server surface. AGPL obligations need separate review. No deployment/resource measurements claimed. |
| SFTP | [pkg/sftp](https://github.com/pkg/sftp), BSD-2-Clause, activity 2026-11-13; request-server can restrict operations without shell | Good dedicated clients, but no equivalent built-in Windows Explorer drive. Requires new SSH listener, host-key and authentication lifecycle. No clear benefit over existing HTTPS here. |

Activity is repository metadata, not proof of recent package-level security review.
Primary protocol reference: [RFC 4918](https://www.rfc-editor.org/rfc/rfc4918).

The full x/net handler is not used: its generic writable FileSystem and
Close-based lifecycle do not directly express the existing known-length,
single-admission, atomic create-only Upload operation. The adapter implements
bounded PROPFIND XML and delegates GET/HEAD, PUT and DELETE to the **existing
Store.Handler**. No new dependency, second copy, listener, root selector,
password or filesystem implementation. Standard encoding/xml provides escaped,
namespace-aware XML; parsing is bounded at 32 KiB/128 requested properties.
This is **not a full RFC 4918 class-1/2 server**; no DAV compliance header.

## Configuration and authority

- Off by default. Device: Network → Web access → Managed files via WebDAV (HTTPS).
  Web settings expose the same access/webdav toggle. Authenticated API:
  PUT /api/v1/settings/access/webdav with {"value":"on"} or {"value":"off"}.
- Endpoint https://<device>/api/v1/webdav/ uses the existing trusted owner TLS
  certificate. Do not disable verification to accommodate a client. No HTTP
  fallback, anonymous share, Node setting, shell or filesystem exposure.
- Existing owner password through HTTP Basic (username such as owner), or owner
  bearer/cookie session. Basic failures share the existing throttle. Existing
  password fields stay redacted; no new credential API.
- Disablement denies new DAV requests and cancels admitted transfers, interrupting
  blocked network I/O. The shared store checks cancellation before publication.
  Completed files remain. Refresh after uncertain outcomes. Other web APIs work
  independently.
- Owner system-filesystem browsing is separate and **not** exposed here.

## Exact subset and limits

| Method / feature | Behaviour |
| --- | --- |
| OPTIONS | 204, exact Allow list, no DAV class-1/2 advertisement |
| PROPFIND | 207, Depth 0/1 only; missing Depth/infinity returns finite-depth 403. allprop, propname, explicit properties; unknown properties get 404 propstat |
| Metadata | DAV displayname, resourcetype; regular files also getcontentlength, getlastmodified, getcontenttype. No invented creation time, ETag or mutable properties |
| Root | One immutable flat collection; no recursive scan or arbitrary path mapping |
| GET/HEAD | Existing attachment download, ranges, Last-Modified; application/octet-stream, no content execution |
| PUT | Shared create-only upload: known Content-Length, 128 MiB/file, 512 MiB total, 256 files, 128 MiB free-space reserve, one upload, five-minute read deadline, private staging and atomic completion |
| DELETE | Shared regular-file deletion. No root deletion. Open downloads retain their descriptor |
| Names | Shared 1–128 ASCII letters/digits/spaces/dots/dashes/underscores, no leading dot/outer spaces. No nested paths, traversal, symlinks or special files |
| Existing destination | 409; never overwrite. Choose another name. No rename/MOVE, COPY, MKCOL, PROPPATCH, LOCK or UNLOCK (405) |
| Preconditions | Lock If/Lock-Token and unsupported mutation preconditions return 412. PUT If-None-Match * remains create-only; an existing destination returns the shared API's 409 |
| Interruption/concurrency | Exact same upload mutex as web/API, 423 if busy. Failed/cancelled uploads remove staging; managed crash leftovers cleaned on startup. No locks advertised |

Clients requiring full DAV classes, writable folders, rename-based saves,
overwrite, custom properties, lock tokens or chunked uploads are not supported.
Native Windows WebClient, davfs2 and GVfs mounts are **not certified**.
Story #40 owns mounting/reconnect experience, not a second server.

Example with a trusted device URL and curl (password prompted, not in history):

~~~sh
curl --fail-with-body --user owner -X PROPFIND -H 'Depth: 1' "$DEVICE_URL/api/v1/webdav/"
curl --fail-with-body --user owner --upload-file sample.bin "$DEVICE_URL/api/v1/webdav/sample.bin"
curl --fail-with-body --user owner --output downloaded.bin "$DEVICE_URL/api/v1/webdav/sample.bin"
curl --fail-with-body --user owner -X DELETE "$DEVICE_URL/api/v1/webdav/sample.bin"
~~~

## Verification status

Test/build/device receipts and measured overhead belong to the issue/PR and the
tested commit. Linux evidence is not Windows evidence. Synthetic browser or HTTP
tests are not Explorer/WebClient certification. No Windows client was tested at
this decision's creation, so that acceptance criterion remains open.

## Settings coverage inventory\n\n| Bound field | Persisted source | Device | Web | Owner API | Node |\n| --- | --- | --- | --- | --- | --- |\n| access/webdav | frontend.webdav (default false) | Network/Web access toggle | Network/Web access toggle | GET/PUT /api/v1/settings/access/webdav | Explicitly excluded; owner administration only |\n\nThe endpoint and port are fixed, credentials are the existing protected owner\npassword, and store limits are shared constants rather than separate settings.\n