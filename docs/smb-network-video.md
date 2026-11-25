# SMB network videos

EchoLocal can read a video from one owner-configured SMB share. It does not mount
the share, expose a proxy port, enumerate other shares, or export device files.
This is unrelated to managed-file/WebDAV server features.

## Configure and play

On a panel device, open **Settings → Connected services → SMB network videos**.
The owner web interface has the same configuration under **Connected services → SMB**.
Set the server hostname (no URL), TCP port (normally 445), share name, username,
optional NTLM domain, and a password. Enable the source and select **Save settings**.

Use a dedicated account restricted on the server to the intended read-only
share. The complete configured share is the access boundary; no client-side
subdirectory sandbox is claimed. The server must enforce its share boundary,
including Unix symlinks and junctions. EchoLocal rejects advertised reparse
points and special files, traversal, alternate data streams, UNC paths and
cross-share references. It never follows DFS referrals to another server.
Use a separate share if only one directory should be accessible.

Play a URL such as `smb://media/folder/video.mp4` through the existing Video web
page or video media-player entity. The speaker entity also routes this source to
the shared video player. `media` is a fixed alias, not a server name. A file must
be addressed explicitly; this iteration does not browse or enumerate the share.
Use the existing pause/resume, seek and stop controls. SMB announcements are not
supported. Ordinary HTTP(S) and YouTube playback retain their existing paths.

Saving source configuration stops the active video so a stream cannot continue
with a mixture of credentials or share settings. An omitted password retains the
saved password; a nonempty replacement changes it. **Disconnect**
explicitly clears it and disables the source. The device and web UI use this same
persistent source of truth. Configuration is panel-only; no source configuration
or credentials are exposed through restricted Node settings.

## Test connection

Use **Test connection** in the device or owner web settings before saving.
It checks the current draft even if the source is disabled. An empty password
field uses the saved password; a typed password is used only for the test and
remains available for an explicit save. Testing never persists configuration,
enables the source, clears a password, or stops playback.

The test connects with the same SMB signing and NTLM policy as playback,
authenticates, mounts the selected share and reads its root metadata. It does
not list, create or modify files. Success is not a video-format or per-file
read-permission test. Connection, sign-in, share/access and timeout failures
have localized, credential-free messages. One test may run at a time; it has
a 15-second overall deadline and a five-second TCP connection deadline.
The device offers cancellation.

## Owner API

`GET /api/v1/smb/` and `PUT /api/v1/smb/` use the existing owner-authenticated
HTTPS API. Read responses are no-store and contain `enabled`, `host`, `port`,
`share`, `username`, `domain`, `password_set`, and localized `labels`.
They never contain the password.

`POST /api/v1/smb/test` accepts the same draft fields and optional password,
but not credential clearing. It uses the same owner authentication and no-store
policy and returns `{ok, code, message}`. A failed connection is a structured
result, not a configuration change. Malformed/oversized requests are rejected.

PUT takes the six configuration fields plus optional `password` (nonempty
replacement) or `clear_password: true` (with `enabled: false`). Omission preserves
the password. Invalid requests do not mutate the previous configuration. Raw
request values are not echoed in errors or logged. The password is stored in a
separate atomically replaced, mode-0600 device file, outside portable settings
exports. It is not placed in media URLs, playback snapshots, Node descriptors,
or decoder metadata. Local root access is outside this protection boundary.

Playback remains `POST /api/v1/video/` with
`{"action":"load","url":"smb://media/folder/video.mp4"}`, then the existing
`pause`, `play`, `seek` (position in seconds), and `stop` actions.
Loading is asynchronous: poll GET playback status for actual success or its
sanitized source/format error. A source URL being accepted is not playback proof.

## Protocol and dependency decision

The implementation pins CloudSoda/go-smb2 at
`v0.0.0-20261105190815-95be04a983b8` (BSD-2-Clause). It is an actively maintained
Go implementation of SMB 2.0.2/2.1/3.0/3.0.2/3.1.1 with NTLMv2 and SMB3
encryption support. Message signing is required; guest/anonymous sessions are
rejected. Encryption is honored when required by the server/share; signing alone
does not encrypt media. SMB1, NetBIOS transport, Kerberos configuration, QUIC,
guest access and DFS are not exposed in this first version.

The original hirochachacha/go-smb2 project and its current v2 API were also
evaluated. CloudSoda provides the needed context-aware TCP, signing and range
reads with a smaller/established API rather than introducing the v2 QUIC and
Kerberos facilities. Its pre-1.0, commit-pinned status is a maintenance tradeoff;
upgrades require the authentication, boundary and device regression tests.
A platform CIFS mount would need kernel/mount support and additional privileges
on Android, so it is not used. No custom SMB protocol implementation is included.

The transport opens a new bounded SMB session for each byte-range request. There
are at most four outstanding requests, a five-second TCP connect limit and a
15-second request deadline. It streams bytes directly into the existing bounded
HTTP range reader/decoder rather than downloading a whole video. Repeated SMB
authentication/metadata requests are the cost of simple, deterministic
cancellation and reconnect behavior; no permanent file/session pool is retained.
The native decoder, container/codec support and media volume controls are shared
with ordinary URL video playback.

## Playback diagnostics

SMB playback and connection-test failures show a localized explanation, the
failed step (connect, sign-in, share, path metadata, file open/stat, or data read)
and a safe technical code. Server NTSTATUS values are preserved, including
unknown values, rather than being replaced with a generic interruption message.
Known statuses distinguish missing paths, invalid paths, folders, read permission
denials, locks, expired sessions and unsupported requests. Wrapped network errors
distinguish DNS, refusal, reset/close and timeout failures.

Unknown failures explicitly remain unknown. Raw error strings, file paths,
server addresses, usernames and passwords are not included. This is a diagnosis
of the reported error, not proof of the underlying NAS/server configuration.
A connection test checks the share, not each file or codec.

An incomplete response is reported as a premature end of video data; normal
EOF is not an error. Intentional body closure does not create a playback error.
Empty files and invalid byte ranges retain the normal HTTP response semantics
but provide an explanatory playback diagnostic.

## Reconnect and limitations

Every subsequent range request reconnects and authenticates afresh. The existing
range reader has bounded retries; a short interruption may recover if a retry
succeeds before buffering is exhausted. Otherwise playback enters an error state:
restore connectivity and explicitly load the same URL again. It does not silently
restart a finished/failed video. Device/service restart retains configuration but
does not autoplay a previously loaded file. Stop closes in-flight reads.

Use a stable, regular file. Growing/live files, server-side path remapping and
files modified during playback are not supported. Unsupported video formats
produce a playback error; a subsequent HTTP(S) load remains usable. MP4 with
H.264 video and AAC audio is the recommended starting point. Codec support is
hardware-specific; no transcoding is added.

### Verification

The opt-in transport integration test uses an isolated SMB fixture and verifies
a byte-identical generated video, initial/middle/end ranges, missing file,
invalid credentials, invalid share, unreachable endpoint, reconnect, cancellation,
and unchanged ordinary HTTP fallback. Set `ECHOLOCAL_SMB_TEST_CONFIG` only to that
isolated fixture's protected configuration file. It must not point to a personal
media share. The generated test video is 24 seconds, 640×360, 24 fps, H.264/AAC.
Synthetic browser coverage checks four languages, replacement/retention/clear
semantics, refresh, source URL submission, and mobile layout. These are not claims
of physical interaction or successful playback on every NAS/codec combination.

Device-tested results and any remaining hardware limits are recorded with the
specific tested candidate in the pull request.
