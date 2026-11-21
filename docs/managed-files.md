# Managed user files

The owner web interface's **Files** page and `/api/v1/files` share one persistent,
flat user-files area. This is asset storage, not a general device filesystem
browser. No folders, rename, overwrite, previews or automatic activation.
Uploading a picture does not add a photo provider; uploading VRM/VRMA does not
import an avatar; custom tones continue to use their existing validated mechanism.

## Access and transport

The endpoints are mounted only in the existing owner-authenticated HTTPS API.
Use the normal owner login and secure SameSite=Strict session cookie, an existing
owner bearer session, or HTTP Basic with the existing frontend owner password.
They are not mounted on the plain HTTP, peer or Node command interfaces.
No new agent grants or configuration settings are introduced. Device configuration
therefore remains unchanged; selecting a computer file happens in the browser.
Cross-origin writes are rejected. Do not send credentials or file payloads over
plain HTTP. Use the device's trusted HTTPS certificate/normal browser trust flow.

## Limits and publication

- Maximum file: **128 MiB** (134217728 bytes).
- Total active user files: **512 MiB** (536870912 bytes), at most **256 files**.
- At least **128 MiB** filesystem availability is reserved for other device work.
  Reported capacity is the minimum of quota remaining, OS-available bytes
  (Bavail), and total free bytes (Bfree) minus this reserve. OS-reserved
  blocks remain untouched and count toward the total free-space reserve;
  they are not subtracted twice. It is a snapshot, not a reservation guarantee.
- One admitted upload at a time; other uploads immediately return 423.
  Metadata and downloads remain available. Each upload has a five-minute deadline.
- Names are 1–128 ASCII bytes: letters, digits, spaces, dots, dashes, underscores;
  no leading dot or leading/trailing spaces. URL-encode the name once as one path
  segment. Slashes, backslashes, control characters and path traversal are invalid.
- `PUT` is **create-only**. A duplicate returns 409; it never replaces bytes.
  Explicitly delete first or choose a different name.
- A known `Content-Length` is required; chunked/unknown-length uploads return 411.
  Browser File uploads supply this automatically. Content is copied incrementally,
  not decoded or buffered in full. A 64 KiB transfer buffer is used.
- Incoming bytes go to a private temporary file; a fully received, synced file is
  atomically linked into its final name without replacement. Rejected/interrupted
  transfers are removed; crash leftovers are removed on the next store opening.
  An HTTP disconnect after commit can lose the success response: refresh the list
  before retrying. Published data survives daemon restart.
- All public operations use a pinned `os.Root`, flat-name validation and regular-file
  checks; downloads additionally use no-follow/nonblocking opens. Symlinks, folders
  and special files are never returned as user content. Storage inconsistency
  fails closed; errors do not include filesystem paths or raw request values.
- Downloads are attachment-only `application/octet-stream`, with nosniff, sandbox
  and no-store headers. A browser downloads directly; it does not create a whole-file
  JavaScript blob. Single-range requests are supported by the HTTP content server.

The managed directory is `layout.StateDir + "/user-files"` (0700; files 0600).
Credentials, settings, app-private state and existing consumer stores are outside
that root. A rollback to older software leaves these files intact but unavailable
through the old API. This is not a backup or an untrusted local-root security boundary.

## API

| Request | Success |
| --- | --- |
| `GET /api/v1/files` | 200 inventory/capacity JSON |
| `PUT /api/v1/files/{name}` with raw bytes | 201 metadata JSON |
| `GET /api/v1/files/{name}` | 200 identical bytes (206 for a valid range) |
| `DELETE /api/v1/files/{name}` | 204, no body |

Metadata: `{"name":"sample.bin","size":3,"modified":"2026-01-01T00:00:00Z"}`.

Inventory: `{"files":[],"usedBytes":0,"availableBytes":536870912,
"budgetBytes":536870912,"maxFileBytes":134217728,"maxFiles":256}`.
`modified` is UTC RFC3339; sizes are bytes; names are sorted. Files are not executed,
rendered, MIME-sniffed or indexed by content.

After authentication, failures are JSON `{"error":"code"}`:

| HTTP | Code |
| --- | --- |
| 400 | `invalid_name`, `incomplete_upload` |
| 403 | `cross_origin` |
| 404 | `not_found` |
| 405 | `method_not_allowed` |
| 409 | `already_exists`, `unsafe_entry` |
| 411 | `length_required` |
| 413 | `file_too_large` |
| 423 | `transfer_busy` |
| 500 | `transfer_failed` (I/O or transport failure; refresh before retry) |
| 507 | `storage_full` (quota, file count, reserve, ENOSPC/EDQUOT) |

Unauthenticated requests use the existing API's 401 response. The UI shows upload
progress, finalization, cancellation, available capacity and actionable failures.
Deletion asks for confirmation. Refresh reconciles uncertain transfer outcomes.

### Script example

Set `DEVICE_URL` to the device's trusted HTTPS origin. These examples prompt for
the owner password; never put it in a URL or command history:

```sh
curl --fail-with-body --user owner "$DEVICE_URL/api/v1/files"
curl --fail-with-body --user owner --upload-file sample.bin \
  "$DEVICE_URL/api/v1/files/sample.bin"
curl --fail-with-body --user owner --output downloaded.bin \
  "$DEVICE_URL/api/v1/files/sample.bin"
cmp sample.bin downloaded.bin
curl --fail-with-body --user owner --request DELETE \
  "$DEVICE_URL/api/v1/files/sample.bin"
```

## Consumer contract

Consumers discover metadata with `(*userfiles.Store).List()`, select by `name`
(the stable flat identifier), and stream through `Open(name)`, closing the returned
descriptor. They must use the same process store instance, never concatenate a
user name into an absolute path, never expose its handler outside owner auth, and
never accept a request-supplied store root. `Metadata` is intentionally format-neutral.

The process-wide instance is exposed by `internal/feature/files.Get()` and used
by the owner web frontend. Consumers should receive that same instance by
dependency injection when their import workflow is implemented. For a long-lived
feature, explicitly validate and atomically import into feature-owned storage.
Keep format/size/security validation, activation and removal semantics there.
Deleting a user file must not break an already imported copy. No photo renderer,
avatar engine, tone importer, network-drive protocol or agent file tool is added
by this change.
