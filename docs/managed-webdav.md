# Managed-file WebDAV (RFC 4918 classes 1 and 2)

Issue #39 owner clarification supersedes the earlier flat transfer-subset design.
The requirement is standard WebDAV clients, not just individually tested curl
methods. This implementation uses `golang.org/x/net/webdav` v0.60.0 (BSD-3-Clause,
already in the application dependency graph), with a constrained shared store
and explicit fixes for its missing shared locks, complete If-header evaluation,
lockdiscovery, MOVE overwrite default, write preconditions, XML namespace
validation and failure-safe publication. No dependency cache is patched.

## Protocol decision

| Candidate | Evidence and constrained-device trade-off |
| --- | --- |
| WebDAV | Existing HTTPS listener, owner authentication and Go dependency; Linux and Windows clients exist. RFC 4918 classes 1/2 cover ordinary file operations. Certificate trust and actual clients still require independent verification. |
| SMB | Native network-drive integration, but a separate listener/authentication/session/signing stack. `hirochachacha/go-smb2` is a client, not a server; the investigated server fork uses AGPL-3.0. Greater service and maintenance footprint. |
| SFTP | Maintained BSD-licensed `pkg/sftp`, but requires a separate SSH transport and host-key lifecycle without improving native Windows Explorer support. No shell is needed or exposed by DAV. |

References: [RFC 4918](https://www.rfc-editor.org/rfc/rfc4918),
[Go WebDAV](https://pkg.go.dev/golang.org/x/net/webdav),
[SMB client](https://github.com/hirochachacha/go-smb2),
[SMB server fork](https://github.com/macos-fuse-t/go-smb2),
[SFTP](https://github.com/pkg/sftp). Library licensing and repository activity are
not a security audit or a resource measurement.

## Configuration and authority

- URL: `https://<device>/api/v1/webdav/`; existing HTTPS port and certificate.
- Opt-in setting `access/webdav`, persisted as `frontend.webdav`, default off.
  Device Network/Web access, web Network settings, owner settings API, and
  OpenClaw Node settings discovery/get/set/import/export share this value.
  The Node requires its existing `settings` grant. Password and other access
  credentials remain excluded from Node access and redacted in owner API reads.
- HTTP Basic with the existing owner password (username such as `owner`), or
  existing owner bearer/cookie sessions. No anonymous access or second password.
- Turning off rejects new requests and cancels running transfers; cancellation
  is checked before publication. Other owner web/API routes remain independent.
- This endpoint exposes only the common managed tree, never the separate
  full-device owner filesystem browser. No arbitrary root setting or shell.
- Client certificate trust must be configured explicitly. No HTTP fallback or
  generic certificate-verification bypass is introduced.

## Operations and consistency

OPTIONS advertises `DAV: 1, 2` and `MS-Author-Via: DAV`. Supported operations:
PROPFIND (Depth 0/1/infinity, allprop/propname/explicit properties), PROPPATCH,
MKCOL, GET/HEAD, PUT, DELETE, COPY, MOVE, LOCK and UNLOCK. GET supports ranges and
HTTP cache validators. Files are attachment downloads, not executable content.

- Collections, UTF-8 names and nested paths are supported. Up to 255 bytes per
  component, 1024 bytes per relative path, and 16 components; no empty components,
  traversal, backslashes, control characters, symlinks or special files.
  `.upload-*` and `.dav-*` names are reserved; other dotfiles are supported.
- PUT atomically replaces a file, including chunked requests within quotas.
  HTTP If-Match/If-None-Match/If-Unmodified-Since and DAV If conditions protect
  writes. Existing destination replacement follows DAV Overwrite semantics;
  MOVE defaults to T as required by RFC 4918. Recursive self-destinations fail.
- Dead properties are persisted atomically, copied/moved/deleted with resources,
  and limited to 1 MiB for the whole store. Control XML is limited to 64 KiB;
  property/listing responses are buffered with a 4 MiB cap.
- Exclusive and shared write locks, depth 0/infinity, refresh, lockdiscovery and
  lock-token conditions are supported. Tokens are unpredictable absolute URIs.
  Timeout is bounded to one hour; at most 512 active locks. Locks are volatile
  and lost on process restart; clients must reacquire them. DELETE removes
  locks rooted in the deleted subtree; MOVE removes source-rooted locks rather
  than moving them. API writes honor DAV locks as well.
- Tagged and untagged If lists implement AND inside lists, OR between lists for
  a resource, and AND between tagged resource groups. Both state tokens and
  ETags, including Not, are evaluated; all affected locks must be authorized.

## Common store and bounds

There is one canonical `user-files` tree. Managed `/api/v1/files` inventory lists
all regular files recursively with relative names; its file download/upload/
delete endpoints accept those paths. Web uploads remain create-only and target
the root, whereas DAV provides explicit replacement/collection operations.
Consumers keep using the same `Store.List/Open` entry points.

Shared limits: 128 MiB per file, 512 MiB content, 256 total files **and folders**,
128 MiB physical free-space reserve, one admitted mutation, and a five-minute
write read-deadline. The API's `entries` count includes directories; `files`
contains only regular files. Metadata/staging are not public resources.

Every DAV mutation uses a disk-backed hard-linked snapshot under the same
trusted state-directory parent. Unchanged file contents are not duplicated;
all writes go to new inodes. Reads continue against the current complete tree.
Only successful operations publish the entire new namespace under the shared
mutex. A two-rename journal recovers to the whole old or new tree after a crash.
Existing open downloads retain their old descriptors. Failed uploads/COPY/MOVE
do not truncate or delete existing destinations. Capacity is checked before
writing and before publication; no file-sized RAM buffers are used.

The process owns one Store instance for this path. Direct owner filesystem
writes are an administrative escape hatch and, as documented separately, are
not governed by managed-file quotas/locks. Do not concurrently modify this
application-owned tree through that separate administrative endpoint.

## Verification and acceptance

The original curl-only transfer-subset evidence is insufficient for client
compatibility. Current receipts must name the tested commit and distinguish:

- Store/API/DAV regressions and race checks, including atomic failure, quota,
  namespace/symlink boundaries, properties/restart, lock/API parity, interrupted
  transfers, complete tagged conditions and journal recovery.
- Linux `litmus` suites basic/copymove/props/locks/http and `davtest` discovery/
  upload/cleanup. davtest's script-execution probes are **not** a feature: script
  payloads remain inert attachments; text/HTML echo matches are not execution.
- The stock davtest source loads obsolete Net::SSL. On the test workstation,
  `PERL_NET_HTTPS_SSL_SOCKET_CLASS=IO::Socket::SSL` selects the maintained backend.
  Certificate trust is separate from DAV capability detection.
- Synthetic loopback conformance tests are separate from real Echo TLS/device
  tests. A TLS-fixture skip of HTTP Expect/100-continue is not a passed test.
- Windows Explorer/WebClient interoperability is still unverified until an
  actual Windows client is tested with an appropriate trusted certificate.
  Class 1/2 support is not a claim for unrelated DAV extensions (ACL, versioning,
  calendar/address-book or synchronization protocols).

Owner acceptance and missing actual Windows evidence remain prerequisites for
merging/closing #39. No automatic merge or issue closure.
