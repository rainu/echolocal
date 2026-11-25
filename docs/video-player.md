# Standalone video playback

The **Video** page in the device's authenticated web interface plays a direct HTTP(S) media URL or a public YouTube / YouTube Music video URL on the Echo display and speakers. It does not use Google Cast discovery, device authentication, or a sender app.

## Use

1. Open the device web interface and sign in with its existing admin password.
2. Select **Video**.
3. Paste a media URL (a direct stream, a YouTube watch link, or a YouTube Music video) and choose **Play on device**.
4. Use **Pause / Resume**, **Stop**, or **Jump to (seconds)**. Seeking is available only when the source supports it. The Echo's existing fullscreen video controls remain available.

The browser is a remote control: the Echo fetches the media itself. A local media server must therefore be reachable from the Echo. URLs are not saved as settings. Stop playback before leaving the device unattended; closing the browser does not stop the video.

## Scope and limitations

- Start with MP4 containing H.264 video and AAC audio.
- The underlying player also has HLS support; codec, server and stream compatibility must be tested for the particular source.
- This is not a browser-tab or screen-mirroring receiver.
- Public individual YouTube videos are supported through the official YouTube IFrame Player API. Arbitrary website pages are not supported.
- DRM-protected streaming services and URL-embedded login credentials are not supported by this entry point.
- The existing Cast functionality is unchanged. This feature does not fix or bypass Google's Cast device trust checks.
- The Video page is offered only by display-capable builds.

The playback API is mounted under the existing authenticated admin API at `/api/v1/video/`: GET returns status; POST accepts `load` (with `url`), `play`, `pause`, `stop`, or `seek` (with a nonnegative `position` in seconds). Status contains no source URL or raw upstream error. Do not expose the admin API publicly.

Refs #68.

## Home Assistant (no HACS or custom integration)

Display-capable devices expose an additional **Video** media-player entity through the existing ESPHome native integration. Home Assistant discovers it when the device reconnects after this update.

Use the standard service/action with the new Video entity:

```yaml
action: media_player.play_media
target:
  entity_id: media_player.your_device_video
data:
  media_content_type: video
  media_content_id: https://example.com/video.mp4
```

Replace the example entity ID with the Video entity discovered for your device. `media_player.media_pause`, `media_player.media_play` and `media_player.media_stop` control the same playback as the web interface and native fullscreen controls. Its volume and mute controls address the existing media audio channel.

The original **Speaker** entity is unchanged, including Home Assistant's audio conversion and announcement handling. The separate Video entity deliberately advertises no audio-conversion format: Home Assistant otherwise turns MP4 into WAV and drops the picture. No REST login, extra service, HA configuration patch, HACS installation or `bypass_proxy` flag is required.

Pass direct HTTP(S) media URLs or the supported individual YouTube video links described below. Home Assistant's ESPHome native media protocol does not carry a seek command; seeking remains available on the device and web interface rather than being advertised as an unsupported HA feature. The Video entity does not handle announcements; send those to Speaker.


### Existing Speaker proxy prerequisite

The original Speaker entity still uses Home Assistant's WAV/announcement proxy. That proxy needs a Home Assistant internal URL which is reachable from the device; an automatically detected container-only address may not be reachable. If direct Video playback and a correctly formatted direct WAV work but normal Speaker conversion does not, check this HA URL setting separately. This update does not rewrite HA's global URL configuration.

## YouTube and YouTube Music URLs

The existing Video web page, authenticated `POST /api/v1/video/` load action,
Video media-player entity, and Node `audio.control.video` accept:

- `https://www.youtube.com/watch?v=M7lc1UVf-VE`
- `https://youtu.be/M7lc1UVf-VE`
- `https://music.youtube.com/watch?v=M7lc1UVf-VE`

The existing Speaker Node action `audio.control.speaker` also routes these URLs
to the same YouTube player. Its ordinary audio URLs and announcements remain on
the original audio path. This does not change Home Assistant's Speaker WAV proxy:
use the Video entity to pass the original YouTube URL through Home Assistant.
The standalone diagnostic `echod media` command still accepts WAV audio only.

Podcast episode links (`music.youtube.com/podcast/VIDEO_ID`), live links (`/live/VIDEO_ID`), legacy `/v/VIDEO_ID` and `/e/VIDEO_ID`, privacy-enhanced `youtube-nocookie.com/embed/VIDEO_ID`, trailing slashes, timestamp fragments (`#t=90`) and YouTube-owned share redirects are recognized without manual rewriting. Redirect wrappers are unwrapped locally (at most three levels), never fetched and never followed to non-YouTube sites.

Mobile watch links, `/embed/VIDEO_ID` and `/shorts/VIDEO_ID` also identify a
single video. Optional `t=90`, `t=1m30s` or `start=90` selects the initial
position (at most 24 hours). Playlist-only, channel, search, clip, credential-bearing,
nonstandard-port YouTube URLs are rejected before playback
changes. A watch link with a playlist still plays only its selected video.

YouTube normally runs its official embedded web player. If YouTube refuses that
embedding context with code 101/150, EchoLocal opens the ordinary official
YouTube watch page and bridges its standard media controls to the same device
decoders. This does not import cookies, automate sign-in, extract stream URLs,
remove advertising or make restricted content available. The website fallback
uses a browser identity; the separate Cast receiver retains its Cast identity.
The website DOM is not a versioned API and may need compatibility updates.
EchoLocal does not resolve watch
pages as direct files, extract signed stream URLs, bypass Cast trust, or add a
public YouTube-only playback endpoint. YouTube chooses the stream and ads;
the existing device media adapter supplies the hardware decoders.
Pause/resume, stop, seeking when available, and the media volume/mute channel
address that same player, including the native fullscreen controls.
Closing the controlling browser does not stop playback.

Loading is asynchronous. Node acceptance is not playback success. Inspect
`audio.status.video`, the existing Video API GET, or the web page for
loading/playing/paused/idle/error, actual decoder video frames, and a sanitized
error. Startup times out after 90 seconds rather than remaining silently loading.
Malformed links are rejected immediately; removed/private videos, disabled
embedding, player identification failures, autoplay blocks and decoder failures
produce distinct safe errors without exposing source URLs.

Limitations: display-capable builds only; no playlist
queue, guaranteed ad-free playback, restricted/age-gated/region-locked content,
or guarantee that every public video permits embedding or the device codecs.
YouTube Music links play the corresponding YouTube video, not a separate Music
account/session. Internet access to YouTube and its media hosts is required.
No playback URL or account settings are persisted. Service restart ends playback.

Refs #71.

### Diagnosing playback refusals

Error 101/150 reports refusal in the current embedded-player context; it is not
by itself proof that the uploader globally disabled embedding. Missing client
identification (153), sign-in/consent, regional and content restrictions are
different conditions. Test ordinary official-site playback in the actual device
network before attributing a refusal to missing cookies or recommending cookies.
A successful page/player state is not sufficient: verify decoder frames and
position, controls and the device output path. No cookie export is needed for
public content that the device can play anonymously.


### YouTube account connection

Manage the device connection under **Connected services → YouTube** in the web
frontend or **Settings → Connected services → YouTube** on the touchscreen.
A locally rendered QR code sits to the right of the activation website in both UIs.
It encodes only the public website URL, never the activation code, and needs no external QR service.
Sign in starts Google's official device activation: open `https://yt.be/activate`
on your phone/computer and confirm the short-lived code there. No Google password
is entered in EchoLocal. Status, retry/check and cancel are shared by both UIs.
Codes are memory-only, not settings or diagnostics; responses must not be logged.
The UI discards a displayed code after ten minutes (Google may expire it earlier).
Cancel closes the local flow but cannot undo a confirmation already made at Google.

A confirmed device connection routes existing YouTube and YouTube Music URL
requests through the official TV website using the same native decoder and media
controls. The normal website/embed and TV sessions are not interchangeable. The
connection status is checked on startup before selecting playback; a connection
error must be resolved or explicitly signed out rather than silently switching
an existing account to anonymous playback. Premium is reported only when the
signed-in first-party TV UI also shows its Premium logo. Benefits depend on the
membership and eligible content; EchoLocal does not remove ads or bypass access
restrictions. The TV website DOM is not a stable public API.

Session data remains in the device's existing persistent Chromium profile, which
is shared with its existing YouTube browser playback. The app never reads or
exports cookies, tokens, account identity or private browser-cookie files.
A service restart stops playback but must retain the connection. This is not a
separate Google account selector or a claim that all YouTube content is supported.

**Sign out on this device** asks for confirmation, stops playback and clears
YouTube-origin local site data (including consent state). It does not clear
unrelated Google-service data, delete the account/subscription, or revoke the
remote Google device authorization. The next sign-in may require a new code.
Account status/actions use owner-authenticated `/api/v1/youtube/`; this is a
connection-management endpoint, not a separate YouTube playback endpoint.
